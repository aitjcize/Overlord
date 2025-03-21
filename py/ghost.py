#!/usr/bin/env python
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import argparse
import binascii
import codecs
import contextlib
import ctypes
import ctypes.util
import fcntl
from functools import reduce
import hashlib
import json
import logging
import os
import platform
import queue
import re
import select
import signal
import socket
import ssl
import stat
import struct
import subprocess
import sys
import termios
import threading
import time
import tty
import urllib.request
import uuid
from ws4py.client import WebSocketBaseClient
from xmlrpc.client import ServerProxy
from xmlrpc.server import SimpleXMLRPCServer


_DEBUG = False

_GHOST_RPC_PORT = int(os.getenv('GHOST_RPC_PORT', '4500'))

_OVERLORD_LAN_DISCOVERY_PORT = int(os.getenv('OVERLORD_LD_PORT', '4456'))
_DEFAULT_HTTP_PORT = 80
_DEFAULT_HTTPS_PORT = 443

_BUFSIZE = 8192
_RETRY_INTERVAL = 2
_SEPARATOR = b'\r\n'
_PING_TIMEOUT = 10
_PING_INTERVAL = 5
_REQUEST_TIMEOUT_SECS = 60
_SHELL = os.getenv('SHELL', '/bin/sh')
_DEFAULT_BIND_ADDRESS = '127.0.0.1'

_BLOCK_SIZE = 4096
_CONNECT_TIMEOUT = 3

_USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36"

# Stream control
_STDIN_CLOSED = '##STDIN_CLOSED##'

SUCCESS = 'success'
FAILED = 'failed'
DISCONNECTED = 'disconnected'

_DEFAULT_FORWARD_HOST = '127.0.0.1'

_ESCAPE_SEQ_RE = re.compile(r'\x1b\[([0-9;?]*)([A-Za-z])')


class PingTimeoutError(Exception):
  pass


class RequestError(Exception):
  pass


class BufferedSocket:
  """A buffered socket that supports unrecv.

  Allow putting back data back to the socket for the next recv() call.
  """
  def __init__(self, sock):
    self.sock = sock
    self._buf = b''

  def fileno(self):
    return self.sock.fileno()

  def Recv(self, bufsize, flags=0):
    if self._buf:
      if len(self._buf) >= bufsize:
        ret = self._buf[:bufsize]
        self._buf = self._buf[bufsize:]
        return ret
      ret = self._buf
      self._buf = b''
      return ret + self.sock.recv(bufsize - len(ret), flags)
    return self.sock.recv(bufsize, flags)

  def UnRecv(self, buf):
    self._buf = buf + self._buf

  def Send(self, *args, **kwargs):
    return self.sock.send(*args, **kwargs)

  def RecvBuf(self):
    """Only recive from buffer."""
    ret = self._buf
    self._buf = b''
    return ret

  def Close(self):
    self.sock.close()


class TLSSettings:
  def __init__(self, tls_cert_file, verify):
    """Constructor.

    Args:
      tls_cert_file: TLS certificate in PEM format.
      enable_tls_without_verify: enable TLS but don't verify certificate.
    """
    self._enabled = False
    self._tls_cert_file = tls_cert_file
    self._verify = verify
    self._tls_context = None

  def _UpdateContext(self):
    if not self._enabled:
      self._tls_context = None
      return

    self._tls_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    self._tls_context.verify_mode = ssl.CERT_REQUIRED

    if self._verify:
      if self._tls_cert_file:
        self._tls_context.check_hostname = True
        try:
          self._tls_context.load_verify_locations(self._tls_cert_file)
          logging.info('TLSSettings: using user-supplied ca-certificate')
        except IOError as e:
          logging.error('TLSSettings: %s: %s', self._tls_cert_file, e)
          sys.exit(1)
      else:
        self._tls_context = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
        self._tls_context.check_hostname = True
        self._tls_context.verify_mode = ssl.CERT_REQUIRED
        logging.info('TLSSettings: using built-in ca-certificates')
    else:
      self._tls_context.check_hostname = False
      self._tls_context.verify_mode = ssl.CERT_NONE
      logging.info('TLSSettings: skipping TLS verification!!!')

  def SetEnabled(self, enabled):
    logging.info('TLSSettings: enabled: %s', enabled)

    if self._enabled != enabled:
      self._enabled = enabled
      self._UpdateContext()

  def Enabled(self):
    return self._enabled

  def CertFilePath(self):
    return self._tls_cert_file

  def Context(self):
    return self._tls_context


class GhostWebsocketClient(WebSocketBaseClient):
  def __init__(self, tls_settings, *args, **kwargs):
    super().__init__(ssl_context=tls_settings.Context(), *args, **kwargs)


class Ghost:
  """Ghost implements the client protocol of Overlord.

  Ghost provide terminal/shell/logcat functionality and manages the client
  side connectivity.
  """
  NONE, AGENT, TERMINAL, SHELL, LOGCAT, FILE, FORWARD = range(7)

  MODE_NAME = {
      NONE: 'NONE',
      AGENT: 'Agent',
      TERMINAL: 'Terminal',
      SHELL: 'Shell',
      LOGCAT: 'Logcat',
      FILE: 'File',
      FORWARD: 'Forward'
  }

  RANDOM_MID = '##random_mid##'

  def __init__(self, overlord_addrs, tls_settings=None, mode=AGENT, mid=None,
               sid=None, allowlist=None, prop_file=None, terminal_sid=None,
               tty_device=None, command=None, file_op=None, host=None, port=None,
               tls_mode=None):
    """Constructor.

    Args:
      overlord_addrs: a list of possible address of overlord.
      tls_settings: a TLSSetting object.
      mode: client mode, either AGENT, SHELL or LOGCAT
      mid: a str to set for machine ID. If mid equals Ghost.RANDOM_MID, machine
        id is randomly generated.
      sid: session ID. If the connection is requested by overlord, sid should
        be set to the corresponding session id assigned by overlord.
      allowlist: comma-separated list of users/groups that can access this ghost.
      prop_file: properties file filename.
      terminal_sid: the terminal session ID associate with this client. This is
        use for file download.
      tty_device: the terminal device to open, if tty_device is None, as pseudo
        terminal will be opened instead.
      command: the command to execute when we are in SHELL mode.
      file_op: a tuple (action, filepath, perm). action is either 'download' or
        'upload'. perm is the permission to set for the file.
      port: port number to forward.
      tls_mode: can be [True, False, None]. if not None, skip detection of
        TLS and assume whether server use TLS or not.
    """
    assert mode in [Ghost.AGENT, Ghost.TERMINAL, Ghost.SHELL, Ghost.FILE,
                    Ghost.FORWARD]
    if mode == Ghost.SHELL:
      assert command is not None
    if mode == Ghost.FILE:
      assert file_op is not None

    self._platform = platform.system()
    self._overlord_addrs = overlord_addrs
    self._connected_addr = None
    self._tls_settings = tls_settings
    self._mid = mid
    self._sock = None
    self._mode = mode
    if self._mid == Ghost.RANDOM_MID:
      self._machine_id = str(uuid.uuid4())
    else:
        self._machine_id = self.GetMachineID()
    self._session_id = sid if sid is not None else str(uuid.uuid4())
    self._terminal_session_id = terminal_sid
    self._ttyname_to_sid = {}
    self._terminal_sid_to_pid = {}
    self._allowlist = allowlist
    self._prop_file = prop_file
    self._properties = {}
    self._register_status = DISCONNECTED
    self._reset = threading.Event()
    self._tls_mode = tls_mode

    # RPC
    self._requests = {}
    self._queue = queue.Queue()

    # Protocol specific
    self._last_ping = 0
    self._tty_device = tty_device
    self._shell_command = command
    self._file_op = file_op
    self._download_queue = queue.Queue()
    self._host = host
    self._port = port

  def SetIgnoreChild(self, status):
    # Only ignore child for Agent since only it could spawn child Ghost.
    if self._mode == Ghost.AGENT:
      signal.signal(signal.SIGCHLD,
                    signal.SIG_IGN if status else signal.SIG_DFL)

  def GetFileSha1(self, filename):
    with open(filename, 'rb') as f:
      return hashlib.sha1(f.read()).hexdigest()

  def TLSEnabled(self, host, port):
    """Determine if TLS is enabled on given server address."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
      # Allow any certificate since we only want to check if server talks TLS.
      context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
      context.check_hostname = False
      context.verify_mode = ssl.CERT_NONE

      sock = context.wrap_socket(sock, server_hostname=host)
      sock.settimeout(_CONNECT_TIMEOUT)
      sock.connect((host, port))
      return True
    except ssl.SSLError:
      return False
    except socket.timeout:
      return False
    except socket.error:  # Connect refused or timeout
      raise
    except Exception:
      return False  # For whatever reason above failed, assume False

  def Upgrade(self):
    logging.info('Upgrade: initiating upgrade sequence...')

    try:
      https_enabled = self.TLSEnabled(self._connected_addr[0],
                                      self._connected_addr[1])
    except socket.error:
      logging.error('Upgrade: failed to connect to Overlord HTTP server, '
                    'abort')
      return

    if self._tls_settings.Enabled() and not https_enabled:
      logging.error('Upgrade: TLS enforced but found Overlord HTTP server '
                    'without TLS enabled! Possible mis-configuration or '
                    'DNS/IP spoofing detected, abort')
      return

    scriptpath = os.path.abspath(sys.argv[0])
    url = 'http%s://%s:%d/upgrade/ghost.py' % (
        's' if https_enabled else '',
        self._connected_addr[0], self._connected_addr[1])

    # Download sha1sum for ghost.py for verification
    try:
      request = urllib.request.Request(url + '.sha1')
      request.add_header('User-Agent', _USER_AGENT)
      with contextlib.closing(
          urllib.request.urlopen(request, timeout=_CONNECT_TIMEOUT,
                                 context=self._tls_settings.Context())) as f:
        if f.getcode() != 200:
          raise RuntimeError('HTTP status %d' % f.getcode())
        sha1sum = f.read().strip().decode('utf-8')
    except (ssl.SSLError, ssl.CertificateError) as e:
      logging.error('Upgrade: %s: %s', e.__class__.__name__, e)
      return
    except Exception as e:
      logging.error('Upgrade: failed to download sha1sum file, abort')
      return

    if self.GetFileSha1(scriptpath) == sha1sum:
      logging.info('Upgrade: ghost is already up-to-date, skipping upgrade')
      return

    # Download upgrade version of ghost.py
    try:
      request = urllib.request.Request(url)
      request.add_header('User-Agent', _USER_AGENT)
      with contextlib.closing(
          urllib.request.urlopen(request, timeout=_CONNECT_TIMEOUT,
                                 context=self._tls_settings.Context())) as f:
        if f.getcode() != 200:
          raise RuntimeError('HTTP status %d' % f.getcode())
        data = f.read()
    except (ssl.SSLError, ssl.CertificateError) as e:
      logging.error('Upgrade: %s: %s', e.__class__.__name__, e)
      return
    except Exception:
      logging.error('Upgrade: failed to download upgrade, abort')
      return

    # Compare SHA1 sum
    if hashlib.sha1(data).hexdigest() != sha1sum:
      logging.error('Upgrade: sha1sum mismatch, abort')
      return

    try:
      with open(scriptpath, 'wb') as f:
        f.write(data)
    except Exception:
      logging.error('Upgrade: failed to write upgrade onto disk, abort')
      return

    logging.info('Upgrade: restarting ghost...')
    self.CloseSockets()
    self.SetIgnoreChild(False)
    os.execve(scriptpath, [scriptpath] + sys.argv[1:], os.environ)

  def LoadProperties(self):
    """Load properties from file and always set the allowlist."""
    self._properties = {}

    try:
      if self._prop_file:
        with open(self._prop_file, 'r') as f:
          self._properties = json.loads(f.read())
    except Exception as e:
      logging.error('LoadProperties: %s', e)

    if self._allowlist:
      if 'allowlist' in self._properties and self._properties['allowlist']:
        logging.warning('Overwriting existing allowlist from properties file with '
                        'command line allowlist value')

      allowed_entities = []
      for entity in self._allowlist.split(','):
        trimmed_entity = entity.strip()
        if trimmed_entity:
          # Support the new format (u/username or g/groupname)
          # If no prefix is provided, assume it's a username and add "u/" prefix
          if '/' not in trimmed_entity:
            trimmed_entity = 'u/' + trimmed_entity
          allowed_entities.append(trimmed_entity)
      self._properties['allowlist'] = allowed_entities
    elif 'allowlist' not in self._properties or len(self._properties['allowlist']) == 0:
      # Default allowlist to current user
      self._properties['allowlist'] = ['u/' + self.GetCurrentUser()]

  def GetCurrentUser(self):
    """Gets the current user's username."""
    return os.getenv('USER', 'root')

  def CloseSockets(self):
    # Close sockets opened by parent process, since we don't use it anymore.
    if self._platform == 'Linux':
      for fd in os.listdir('/proc/self/fd/'):
        try:
          real_fd = os.readlink('/proc/self/fd/%s' % fd)
          if real_fd.startswith('socket'):
            os.close(int(fd))
        except Exception:
          pass

  def SpawnGhost(self, mode, sid=None, terminal_sid=None, tty_device=None,
                 command=None, file_op=None, host=None, port=None):
    """Spawn a child ghost with specific mode.

    Returns:
      The spawned child process pid.
    """
    # Restore the default signal handler, so our child won't have problems.
    self.SetIgnoreChild(False)

    pid = os.fork()
    if pid == 0:
      self.CloseSockets()
      g = Ghost([self._connected_addr], tls_settings=self._tls_settings,
                mode=mode, mid=Ghost.RANDOM_MID, sid=sid,
                allowlist=self._allowlist, terminal_sid=terminal_sid,
                tty_device=tty_device, command=command, file_op=file_op,
                host=host, port=port)
      g.Start()
      sys.exit(0)
    else:
      self.SetIgnoreChild(True)
      return pid

  def Timestamp(self):
    return int(time.time())

  def GetGateWayIP(self):
    if self._platform == 'Darwin':
      output = subprocess.check_output(['route', '-n', 'get', 'default'])
      ret = re.search('gateway: (.*)', output.decode('utf-8'))
      if ret:
        return [ret.group(1)]
      return []
    if self._platform == 'Linux':
      with open('/proc/net/route', 'r') as f:
        lines = f.readlines()

      ips = []
      for line in lines:
        parts = line.split('\t')
        if parts[2] == '00000000':
          continue

        try:
          h = codecs.decode(parts[2], 'hex')
          ips.append('.'.join([str(x) for x in reversed(h)]))
        except (TypeError, binascii.Error):
          pass

      return ips

    logging.warning('GetGateWayIP: unsupported platform')
    return []

  def GetFactoryServerIP(self):
    try:
      from cros.factory.test import server_proxy

      url = server_proxy.GetServerURL()
      match = re.match(r'^https?://(.*):.*$', url)
      if match:
        return [match.group(1)]
    except Exception:
      pass
    return []

  def GetMachineID(self):
    """Generates machine-dependent ID string for a machine.
    There are many ways to generate a machine ID:
    Linux:
      1. factory device_id
      2. /sys/class/dmi/id/product_uuid (only available on intel machines)
      3. MAC address
      We follow the listed order to generate machine ID, and fallback to the
      next alternative if the previous doesn't work.

    Darwin:
      All Darwin system should have the IOPlatformSerialNumber attribute.
    """
    if self._mid:
      return self._mid

    # Darwin
    if self._platform == 'Darwin':
      output = subprocess.check_output(['ioreg', '-rd1', '-c',
                                        'IOPlatformExpertDevice'])
      ret = re.search('"IOPlatformSerialNumber" = "(.*)"',
                      output.decode('utf-8'))
      if ret:
        return ret.group(1)

    # Try factory device id
    try:
      from cros.factory.test import session
      return session.GetDeviceID()
    except Exception:
      pass

    # Try DMI product UUID
    try:
      with open('/sys/class/dmi/id/product_uuid', 'r') as f:
        return f.read().strip()
    except Exception:
      pass

    # Use MAC address if non is available
    try:
      macs = []
      ifaces = sorted(os.listdir('/sys/class/net'))
      for iface in ifaces:
        if iface == 'lo':
          continue

        with open('/sys/class/net/%s/address' % iface, 'r') as f:
          macs.append(f.read().strip())

      return ';'.join(macs)
    except Exception:
      pass

    return str(uuid.uuid4())

  def GetProcessWorkingDirectory(self, pid):
    if self._platform == 'Linux':
      return os.readlink('/proc/%d/cwd' % pid)
    if self._platform == 'Darwin':
      PROC_PIDVNODEPATHINFO = 9
      proc_vnodepathinfo_size = 2352
      vid_path_offset = 152

      proc = ctypes.cdll.LoadLibrary(ctypes.util.find_library('libproc'))
      buf = ctypes.create_string_buffer('\0' * proc_vnodepathinfo_size)
      proc.proc_pidinfo(pid, PROC_PIDVNODEPATHINFO, 0,
                        ctypes.byref(buf), proc_vnodepathinfo_size)
      buf = buf.raw[vid_path_offset:]
      n = buf.index('\0')
      return buf[:n]
    raise RuntimeError('GetProcessWorkingDirectory: unsupported platform')

  def Reset(self):
    """Reset state and clear request handlers."""
    if self._sock is not None:
      self._sock.Close()
      self._sock = None
    self._reset.clear()
    self._last_ping = 0
    self._requests = {}
    self.LoadProperties()
    self._register_status = DISCONNECTED

  def SendMessage(self, msg):
    """Serialize the message and send it through the socket."""
    self._sock.Send(json.dumps(msg).encode('utf-8') + _SEPARATOR)

  def SendRequest(self, name, args, handler=None,
                  timeout=_REQUEST_TIMEOUT_SECS):
    if handler and not callable(handler):
      raise RequestError('Invalid request handler for msg "%s"' % name)

    rid = str(uuid.uuid4())
    msg = {'rid': rid, 'timeout': timeout, 'name': name, 'payload': args}
    if timeout >= 0:
      self._requests[rid] = [self.Timestamp(), timeout, handler]
    self.SendMessage(msg)

  def SendResponse(self, omsg, status, payload=None):
    msg = {'rid': omsg['rid'], 'status': status, 'payload': payload}
    self.SendMessage(msg)

  def SendErrorResponse(self, omsg, error):
    msg = {'rid': omsg['rid'], 'status': FAILED, 'payload': {'error': error}}
    self.SendMessage(msg)

  def HandleTTYControl(self, fd, buffer):
    """Handle terminal control sequences.

    Args:
      fd: File descriptor of the terminal
      control_str: Control string to process

    Returns:
      index of the next character after the control sequence
    """
    match = _ESCAPE_SEQ_RE.search(buffer.decode('utf-8'))
    if not match:
      # Consume the first two bytes so we won't process it again.
      os.write(fd, buffer[:2])
      return 2

    args = match.group(1)
    command = match.group(2)

    if command == 't':
      try:
        params = args.split(';')
        if len(params) >= 3 and params[0] == '8':
          rows = int(params[1])
          cols = int(params[2])
          logging.info('Terminal resize request received: rows=%d, cols=%d', rows, cols)
          winsize = struct.pack('HHHH', rows, cols, 0, 0)
          fcntl.ioctl(fd, termios.TIOCSWINSZ, winsize)
          return len(match.group(0))
      except Exception as e:
        logging.warning('Error handling terminal control: %s', e)

    os.write(fd, match.group(0).encode('utf-8'))
    return len(match.group(0))

  def SpawnTTYServer(self, unused_var):
    """Spawn a TTY server and forward I/O to the TCP socket."""
    logging.info('SpawnTTYServer: started')

    try:
      if self._tty_device is None:
        pid, fd = os.forkpty()

        if pid == 0:
          ttyname = os.ttyname(sys.stdout.fileno())
          try:
            server = GhostRPCServer()
            server.RegisterTTY(self._session_id, ttyname)
            server.RegisterSession(self._session_id, os.getpid())
          except Exception:
            # If ghost is launched without RPC server, the call will fail but we
            # can ignore it.
            pass

          # The directory that contains the current running ghost script
          script_dir = os.path.dirname(os.path.abspath(sys.argv[0]))

          env = os.environ.copy()
          env['USER'] = os.getenv('USER', 'root')
          env['HOME'] = os.getenv('HOME', '/root')
          env['PATH'] = os.getenv('PATH') + ':%s' % script_dir
          os.chdir(env['HOME'])
          os.execve(_SHELL, [_SHELL], env)
      else:
        fd = os.open(self._tty_device, os.O_RDWR)
        tty.setraw(fd)

        attrs = termios.tcgetattr(fd)
        attrs[0] &= ~(termios.IXON | termios.IXOFF)  # Disable software flow control
        attrs[2] |= termios.CLOCAL                   # Ignore modem control lines
        attrs[2] &= ~termios.CRTSCTS                 # Disable hardware flow control
        termios.tcsetattr(fd, termios.TCSANOW, attrs)

      def _ProcessBuffer(buf):
        while True:
          pos = buf.find(b'\x1b[')
          if pos == -1:
            break

          os.write(fd, buf[:pos])
          consumed = self.HandleTTYControl(fd, buf[pos:])
          buf = buf[pos + consumed:]

        os.write(fd, buf)

      # Initial buffer processing
      _ProcessBuffer(self._sock.RecvBuf())

      while True:
        rd, unused_wd, unused_xd = select.select([self._sock, fd], [], [])

        if fd in rd:
          data = os.read(fd, _BUFSIZE)
          if data == b'':
            raise RuntimeError('connection terminated')
          self._sock.Send(data)

        if self._sock in rd:
          buf = self._sock.Recv(_BUFSIZE)
          _ProcessBuffer(buf)
    except Exception as e:
      if _DEBUG:
        logging.exception('SpawnTTYServer: %s', e)
      else:
        logging.error('SpawnTTYServer: %s', e)
    finally:
      self._sock.Close()

    logging.info('SpawnTTYServer: terminated')
    os._exit(0)  # pylint: disable=protected-access

  def SpawnShellServer(self, unused_var):
    """Spawn a shell server and forward input/output from/to the TCP socket."""
    logging.info('SpawnShellServer: started')

    # Add ghost executable to PATH
    script_dir = os.path.dirname(os.path.abspath(sys.argv[0]))
    env = os.environ.copy()
    env['PATH'] = '%s:%s' % (script_dir, os.getenv('PATH'))

    # Execute shell command from HOME directory
    os.chdir(os.getenv('HOME', '/tmp'))

    p = subprocess.Popen(self._shell_command, stdin=subprocess.PIPE,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                         shell=True, env=env)

    def make_non_block(fd):
      fl = fcntl.fcntl(fd, fcntl.F_GETFL)
      fcntl.fcntl(fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

    make_non_block(p.stdout)
    make_non_block(p.stderr)

    try:
      p.stdin.write(self._sock.RecvBuf())

      while True:
        rd, unused_wd, unused_xd = select.select(
            [p.stdout, p.stderr, self._sock], [], [])
        if p.stdout in rd:
          self._sock.Send(p.stdout.read(_BUFSIZE))

        if p.stderr in rd:
          self._sock.Send(p.stderr.read(_BUFSIZE))

        if self._sock in rd:
          ret = self._sock.Recv(_BUFSIZE)
          if not ret:
            raise RuntimeError('connection terminated')

          try:
            idx = ret.index(_STDIN_CLOSED * 2)
            p.stdin.write(ret[:idx])
            p.stdin.close()
          except ValueError:
            p.stdin.write(ret)
        p.poll()
        if p.returncode is not None:
          break
    except Exception as e:
      if _DEBUG:
        logging.exception('SpawnShellServer: %s', e)
      else:
        logging.error('SpawnShellServer: %s', e)
    finally:
      # Check if the process is terminated. If not, Send SIGTERM to process,
      # then wait for 1 second. Send another SIGKILL to make sure the process is
      # terminated.
      p.poll()
      if p.returncode is None:
        try:
          p.terminate()
          time.sleep(1)
          p.kill()
        except Exception:
          pass

      p.wait()
      self._sock.Close()

    logging.info('SpawnShellServer: terminated')
    os._exit(0)  # pylint: disable=protected-access

  def InitiateFileOperation(self, unused_var):
    if self._file_op[0] == 'download':
      try:
        size = os.stat(self._file_op[1]).st_size
      except OSError as e:
        logging.error('InitiateFileOperation: download: %s', e)
        sys.exit(1)

      self.SendRequest('request_to_download',
                       {'terminal_sid': self._terminal_session_id,
                        'filename': os.path.basename(self._file_op[1]),
                        'size': size})
    elif self._file_op[0] == 'upload':
      self.SendRequest('clear_to_upload', {}, timeout=-1)
      self.StartUploadServer()
    else:
      logging.error('InitiateFileOperation: unknown file operation, ignored')

  def StartDownloadServer(self):
    logging.info('StartDownloadServer: started')

    try:
      with open(self._file_op[1], 'rb') as f:
        while True:
          data = f.read(_BLOCK_SIZE)
          if not data:
            break
          self._sock.Send(data)
    except Exception as e:
      logging.error('StartDownloadServer: %s', e)
    finally:
      self._sock.Close()

    logging.info('StartDownloadServer: terminated')
    os._exit(0)  # pylint: disable=protected-access

  def StartUploadServer(self):
    logging.info('StartUploadServer: started')
    try:
      filepath = self._file_op[1]
      dirname = os.path.dirname(filepath)
      if not os.path.exists(dirname):
        try:
          os.makedirs(dirname)
        except Exception:
          pass

      with open(filepath, 'wb') as f:
        if self._file_op[2]:
          os.fchmod(f.fileno(), self._file_op[2])

        f.write(self._sock.RecvBuf())

        while True:
          rd, unused_wd, unused_xd = select.select([self._sock], [], [])
          if self._sock in rd:
            buf = self._sock.Recv(_BLOCK_SIZE)
            if not buf:
              break
            f.write(buf)
    except socket.error as e:
      logging.error('StartUploadServer: socket error: %s', e)
    except Exception as e:
      logging.error('StartUploadServer: %s', e)
    finally:
      self._sock.Close()

    logging.info('StartUploadServer: terminated')
    os._exit(0)  # pylint: disable=protected-access

  def SpawnPortForwardServer(self, unused_var):
    """Spawn a port forwarding server and forward I/O to the TCP socket."""
    logging.info('SpawnPortForwardServer: started')

    src_sock = None
    try:
      src_sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
      src_sock.settimeout(_CONNECT_TIMEOUT)
      src_sock.connect((self._host, self._port))

      src_sock.send(self._sock.RecvBuf())

      while True:
        rd, unused_wd, unused_xd = select.select([self._sock, src_sock], [], [])

        if self._sock in rd:
          data = self._sock.Recv(_BUFSIZE)
          if not data:
            raise RuntimeError('connection terminated')
          src_sock.send(data)

        if src_sock in rd:
          data = src_sock.recv(_BUFSIZE)
          if not data:
            continue
          self._sock.Send(data)
    except Exception as e:
      logging.error('SpawnPortForwardServer: %s', e)
    finally:
      if src_sock:
        src_sock.close()
      self._sock.Close()

    logging.info('SpawnPortForwardServer: terminated')
    os._exit(0)  # pylint: disable=protected-access

  def Ping(self):
    def timeout_handler(x):
      if x is None:
        raise PingTimeoutError

    self._last_ping = self.Timestamp()
    self.SendRequest('ping', {}, timeout_handler, _PING_TIMEOUT)

  def _Fstat(self, path):
    if not os.path.isabs(path):
      raise RuntimeError(f"absolute path required: {path}")

    entry = {'exists': os.path.exists(path)}
    if entry['exists']:
      stat_info = os.lstat(path)
      entry['path'] = path
      entry['perm'] = stat_info.st_mode
      entry['size'] = stat_info.st_size
      entry['mtime'] = int(stat_info.st_mtime)
      entry['is_dir'] = os.path.isdir(path)
      entry['is_symlink'] = stat.S_ISLNK(stat_info.st_mode)

      if entry['is_symlink']:
        entry['link_target'] = os.readlink(path)
        entry['is_dir'] = False

    return entry

  def HandleListTreeRequest(self, msg):
    """Handle a request to list directory contents recursively."""
    payload = msg['payload']
    path = payload.get('path')

    if not os.path.isabs(path):
      path = os.path.join('~', path)

    path = os.path.expanduser(path)

    try:
      if not os.path.exists(path):
        raise RuntimeError(f"No such file or directory: {path}")

      entries = []
      entries.append(self._Fstat(path))
      for root, dirs, files in os.walk(path):
        for file in files:
          file_path = os.path.join(root, file)
          try:
            entries.append(self._Fstat(file_path))
          except OSError as e:
            logging.exception(e)
            pass

        for dir in dirs:
          dir_path = os.path.join(root, dir)
          try:
            entries.append(self._Fstat(dir_path))
          except OSError:
            pass

      self.SendResponse(msg, SUCCESS, entries)
    except Exception as e:
      self.SendErrorResponse(msg, str(e))

  def HandleFstatRequest(self, msg):
    """Handle a request to get file/directory status.

    Args:
      msg: The request message.
    """
    payload = msg['payload']
    path = payload.get('path')

    if not os.path.isabs(path):
      path = os.path.join('~', path)

    path = os.path.expanduser(path)

    try:
      self.SendResponse(msg, SUCCESS, self._Fstat(path))
    except Exception as e:
      self.SendErrorResponse(msg, str(e))

  def HandleFileDownloadRequest(self, msg):
    payload = msg['payload']
    filepath = payload['filename']
    if not os.path.isabs(filepath):
      filepath = os.path.join(os.getenv('HOME', '/tmp'), filepath)

    try:
      with open(filepath, 'rb', encoding=None):
        pass
    except Exception as e:
      self.SendErrorResponse(msg, str(e))
      return

    self.SpawnGhost(self.FILE, payload['sid'],
                    file_op=('download', filepath))
    self.SendResponse(msg, SUCCESS)

  def HandleFileUploadRequest(self, msg):
    payload = msg['payload']

    # Resolve upload filepath
    filename = payload['filename']
    dest_path = filename

    # If dest is specified, use it first
    dest_path = payload.get('dest', '')
    if dest_path:
      if not os.path.isabs(dest_path):
        dest_path = os.path.join(os.getenv('HOME', '/tmp'), dest_path)

      if os.path.isdir(dest_path):
        dest_path = os.path.join(dest_path, filename)
    else:
      target_dir = os.getenv('HOME', '/tmp')

      # Terminal session ID found, upload to it's current working directory
      if 'terminal_sid' in payload:
        pid = self._terminal_sid_to_pid.get(payload['terminal_sid'], None)
        if pid:
          try:
            target_dir = self.GetProcessWorkingDirectory(pid)
          except Exception as e:
            logging.error(e)

      dest_path = os.path.join(target_dir, filename)

    try:
      os.makedirs(os.path.dirname(dest_path))
    except Exception:
      pass

    try:
      with open(dest_path, 'wb', encoding=None):
        pass
    except Exception as e:
      self.SendErrorResponse(msg, str(e))
      return

    # If not check_only, spawn FILE mode ghost agent to handle upload
    if not payload.get('check_only', False):
      self.SpawnGhost(self.FILE, payload['sid'],
                      file_op=('upload', dest_path, payload.get('perm', None)))
    self.SendResponse(msg, SUCCESS)

  def HandleCreateSymlinkRequest(self, msg):
    """Handle create_symlink request."""
    payload = msg['payload']
    target = payload['target']
    dest = payload['dest']

    try:
      # Create parent directories if they don't exist
      os.makedirs(os.path.dirname(dest), exist_ok=True)

      # Remove existing file/link if it exists
      if os.path.exists(dest):
        os.remove(dest)

      # Create the symlink
      os.symlink(target, dest)
      self.SendResponse(msg, SUCCESS)
    except Exception as e:
      self.SendErrorResponse(msg, str(e))

  def HandleMkdirRequest(self, msg):
    """Handle mkdir request."""
    payload = msg['payload']
    path = payload['path']
    perm = payload['perm']

    try:
      os.makedirs(path, exist_ok=True)
      os.chmod(path, perm)
      self.SendResponse(msg, SUCCESS)
    except Exception as e:
      self.SendErrorResponse(msg, str(e))

  def HandleRequest(self, msg):
    """Handle request from overlord."""
    command = msg['name']
    payload = msg['payload']

    if command == 'upgrade':
      self.Upgrade()
    elif command == 'terminal':
      self.SpawnGhost(self.TERMINAL, payload['sid'],
                      tty_device=payload['tty_device'])
      self.SendResponse(msg, SUCCESS)
    elif command == 'shell':
      self.SpawnGhost(self.SHELL, payload['sid'], command=payload['command'])
      self.SendResponse(msg, SUCCESS)
    elif command == 'list_tree':
      self.HandleListTreeRequest(msg)
    elif command == 'fstat':
      self.HandleFstatRequest(msg)
    elif command == 'file_download':
      self.HandleFileDownloadRequest(msg)
    elif command == 'clear_to_download':
      self.StartDownloadServer()
    elif command == 'file_upload':
      self.HandleFileUploadRequest(msg)
    elif command == 'create_symlink':
      self.HandleCreateSymlinkRequest(msg)
    elif command == 'mkdir':
      self.HandleMkdirRequest(msg)
    elif command == 'forward':
      self.SpawnGhost(self.FORWARD, payload['sid'],
                      host=payload.get('host', _DEFAULT_FORWARD_HOST),
                      port=payload['port'])
      self.SendResponse(msg, SUCCESS)

  def HandleResponse(self, response):
    rid = str(response['rid'])
    if rid in self._requests:
      handler = self._requests[rid][2]
      del self._requests[rid]
      if callable(handler):
        handler(response)
    else:
      logging.warning('Received unsolicited response, ignored')

  def ParseMessage(self, buf, single=True):
    if single:
      try:
        index = buf.index(_SEPARATOR)
      except ValueError:
        self._sock.UnRecv(buf)
        return

      msgs_json = [buf[:index]]
      self._sock.UnRecv(buf[index + 2:])
    else:
      msgs_json = buf.split(_SEPARATOR)
      self._sock.UnRecv(msgs_json.pop())

    for msg_json in msgs_json:
      try:
        msg = json.loads(msg_json)
      except ValueError:
        # Ignore mal-formed message.
        logging.error('mal-formed JSON request, ignored')
        continue

      if 'name' in msg:
        self.HandleRequest(msg)
      elif 'status' in msg:
        self.HandleResponse(msg)
      else:  # Ingnore mal-formed message.
        logging.error('mal-formed JSON request, ignored')

  def ScanForTimeoutRequests(self):
    """Scans for pending requests which have timed out.

    If any timed-out requests are discovered, their handler is called with the
    special response value of None.
    """
    for rid in list(self._requests):
      request_time, timeout, handler = self._requests[rid]
      if self.Timestamp() - request_time > timeout:
        if callable(handler):
          handler(None)
        else:
          logging.error('Request %s timeout', rid)
        del self._requests[rid]

  def InitiateDownload(self):
    ttyname, filename = self._download_queue.get()
    sid = self._ttyname_to_sid[ttyname]
    self.SpawnGhost(self.FILE, terminal_sid=sid,
                    file_op=('download', filename))

  def Listen(self):
    try:
      while True:
        rds, unused_wd, unused_xd = select.select([self._sock], [], [],
                                                  _PING_INTERVAL // 2)

        if self._sock in rds:
          data = self._sock.Recv(_BUFSIZE)

          # Socket is closed
          if not data:
            break

          self.ParseMessage(data, self._register_status != SUCCESS)

        if (self._mode == self.AGENT and
            self.Timestamp() - self._last_ping > _PING_INTERVAL):
          self.Ping()
        self.ScanForTimeoutRequests()

        if not self._download_queue.empty():
          self.InitiateDownload()

        if self._reset.is_set():
          break
    except socket.error:
      raise RuntimeError('Connection dropped') from None
    except PingTimeoutError:
      raise RuntimeError('Connection timeout') from None
    finally:
      self.Reset()

    self._queue.put('resume')

    if self._mode != Ghost.AGENT:
      sys.exit(1)

  def Register(self):
    non_local = {}
    for addr in self._overlord_addrs:
      non_local['addr'] = addr
      def registered(response):
        if response is None:
          self._reset.set()
          raise RuntimeError('Register request timeout')

        self._register_status = response['status']
        if response['status'] != SUCCESS:
          self._reset.set()
          raise RuntimeError('Register: ' + response['status'])

        logging.info('Registered with Overlord at %s:%d', *non_local['addr'])
        self._connected_addr = non_local['addr']
        self.Upgrade()  # Check for upgrade
        self._queue.put('pause', True)

      try:
        logging.info('Trying %s:%d ...', *addr)
        self.Reset()

        # Check if server has TLS enabled. Only check if self._tls_mode is
        # None.
        # Only control channel needs to determine if TLS is enabled. Other mode
        # should use the TLSSettings passed in when it was spawned.
        if self._mode == Ghost.AGENT:
          self._tls_settings.SetEnabled(
              self.TLSEnabled(*addr) if self._tls_mode is None
              else self._tls_mode)

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(_CONNECT_TIMEOUT)

        try:
          if self._tls_settings.Enabled():
            tls_context = self._tls_settings.Context()
            sock = tls_context.wrap_socket(sock, server_hostname=addr[0])

          sock.connect(addr)
        except (ssl.SSLError, ssl.CertificateError) as e:
          logging.error('%s: %s', e.__class__.__name__, e)
          continue
        except IOError as e:
          if e.errno == 2:  # No such file or directory
            logging.error('%s: %s', e.__class__.__name__, e)
            continue
          raise

        logging.info('Connection established, registering...')
        handler = {
            Ghost.AGENT: registered,
            Ghost.TERMINAL: self.SpawnTTYServer,
            Ghost.SHELL: self.SpawnShellServer,
            Ghost.FILE: self.InitiateFileOperation,
            Ghost.FORWARD: self.SpawnPortForwardServer,
        }[self._mode]

        uri = '%s://%s:%s/connect' % (
            ('wss' if self._tls_settings.Enabled() else 'ws',) + addr)
        self._ws = GhostWebsocketClient(self._tls_settings, uri)
        self._ws.connect()

        # Hijack the raw socket from the websocket, we do not want to use the
        # websocket protocol.
        self._sock = BufferedSocket(self._ws.sock)

        # Machine ID may change if MAC address is used (USB-ethernet dongle
        # plugged/unplugged)
        self._machine_id = self.GetMachineID()
        self.SendRequest('register',
                         {'mode': self._mode, 'mid': self._machine_id,
                          'sid': self._session_id,
                          'properties': self._properties}, handler)
      except socket.error:
        pass
      except Exception as e:
        logging.error('Register: %s', e)
      else:
        sock.settimeout(None)
        self.Listen()

    raise RuntimeError('Cannot connect to any server')

  def Reconnect(self):
    logging.info('Received reconnect request from RPC server, reconnecting...')
    self._reset.set()

  def GetStatus(self):
    status = self._register_status
    if self._register_status == SUCCESS:
      ip, port = self._sock.sock.getpeername()
      status += ' %s:%d' % (ip, port)
    return status

  def AddToDownloadQueue(self, ttyname, filename):
    self._download_queue.put((ttyname, filename))

  def RegisterTTY(self, session_id, ttyname):
    self._ttyname_to_sid[ttyname] = session_id

  def RegisterSession(self, session_id, process_id):
    self._terminal_sid_to_pid[session_id] = process_id

  def StartLanDiscovery(self):
    """Start to listen to LAN discovery packet at
    _OVERLORD_LAN_DISCOVERY_PORT."""

    def thread_func():
      s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
      s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
      s.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
      try:
        s.bind(('0.0.0.0', _OVERLORD_LAN_DISCOVERY_PORT))
      except socket.error as e:
        logging.error('LAN discovery: %s, abort', e)
        return

      logging.info('LAN Discovery: started')
      while True:
        rd, unused_wd, unused_xd = select.select([s], [], [], 1)

        if s in rd:
          data, source_addr = s.recvfrom(_BUFSIZE)
          parts = data.split()
          if parts[0] == 'OVERLORD':
            ip, port = parts[1].split(':')
            if not ip:
              ip = source_addr[0]
            self._queue.put((ip, int(port)), True)

        try:
          obj = self._queue.get(False)
        except queue.Empty:
          pass
        else:
          if not isinstance(obj, str):
            self._queue.put(obj)
          elif obj == 'pause':
            logging.info('LAN Discovery: paused')
            while obj != 'resume':
              obj = self._queue.get(True)
            logging.info('LAN Discovery: resumed')

    t = threading.Thread(target=thread_func)
    t.daemon = True
    t.start()

  def StartRPCServer(self):
    logging.info('RPC Server: started')
    rpc_server = SimpleXMLRPCServer((_DEFAULT_BIND_ADDRESS, _GHOST_RPC_PORT),
                                     logRequests=False, allow_none=True)
    rpc_server.register_function(self.Reconnect, 'Reconnect')
    rpc_server.register_function(self.GetStatus, 'GetStatus')
    rpc_server.register_function(self.RegisterTTY, 'RegisterTTY')
    rpc_server.register_function(self.RegisterSession, 'RegisterSession')
    rpc_server.register_function(self.AddToDownloadQueue, 'AddToDownloadQueue')
    t = threading.Thread(target=rpc_server.serve_forever)
    t.daemon = True
    t.start()

  def ScanServer(self):
    for meth in [self.GetGateWayIP, self.GetFactoryServerIP]:
      addrs = reduce(lambda x, y: x + y,
                     [[(h, _DEFAULT_HTTPS_PORT), (h, _DEFAULT_HTTP_PORT)]
                      for h in meth()], [])
      for addr in addrs:
        if addr not in self._overlord_addrs:
          self._overlord_addrs.append(addr)

  def Start(self, lan_disc=False, rpc_server=False):
    logging.info('%s started', self.MODE_NAME[self._mode])
    logging.info('MID: %s', self._machine_id)
    logging.info('SID: %s', self._session_id)

    # We don't care about child process's return code, not wait is needed.  This
    # is used to prevent zombie process from lingering in the system.
    self.SetIgnoreChild(True)

    if lan_disc:
      self.StartLanDiscovery()

    if rpc_server:
      self.StartRPCServer()

    try:
      while True:
        try:
          addr = self._queue.get(False)
        except queue.Empty:
          pass
        else:
          if isinstance(addr, tuple) and addr not in self._overlord_addrs:
            logging.info('LAN Discovery: got overlord address %s:%d', *addr)
            self._overlord_addrs.append(addr)

        try:
          self.ScanServer()
          self.Register()
        # Don't show stack trace for RuntimeError, which we use in this file for
        # plausible and expected errors (such as can't connect to server).
        except RuntimeError as e:
          logging.info('%s, retrying in %ds', str(e), _RETRY_INTERVAL)
          time.sleep(_RETRY_INTERVAL)
        except Exception as e:
          logging.info('%s: %s, retrying in %ds',
                       e.__class__.__name__, str(e), _RETRY_INTERVAL)
          time.sleep(_RETRY_INTERVAL)

        self.Reset()
    except KeyboardInterrupt:
      logging.error('Received keyboard interrupt, quit')
      sys.exit(0)


def GhostRPCServer():
  """Returns handler to Ghost's JSON RPC server."""
  return ServerProxy('http://127.0.0.1:%d' % _GHOST_RPC_PORT)


def ForkToBackground():
  """Fork process to run in background."""
  pid = os.fork()
  if pid != 0:
    logging.info('Ghost(%d) running in background.', pid)
    sys.exit(0)


def DownloadFile(filename):
  """Initiate a client-initiated file download."""
  filepath = os.path.abspath(filename)
  if not os.path.exists(filepath):
    logging.error('file `%s\' does not exist', filename)
    sys.exit(1)

  # Check if we actually have permission to read the file
  if not os.access(filepath, os.R_OK):
    logging.error('can not open %s for reading', filepath)
    sys.exit(1)

  server = GhostRPCServer()
  server.AddToDownloadQueue(os.ttyname(0), filepath)
  sys.exit(0)


def main():
  parser = argparse.ArgumentParser()
  parser.add_argument('--fork', dest='fork', action='store_true', default=False,
                      help='fork procecess to run in background')
  parser.add_argument('--mid', metavar='MID', dest='mid', action='store',
                      default=None, help='use MID as machine ID')
  parser.add_argument('--rand-mid', dest='mid', action='store_const',
                      const=Ghost.RANDOM_MID, help='use random machine ID')
  parser.add_argument('--no-lan-disc', dest='lan_disc', action='store_false',
                      default=True, help='disable LAN discovery')
  parser.add_argument('--no-rpc-server', dest='rpc_server',
                      action='store_false', default=True,
                      help='disable RPC server')
  parser.add_argument('--tls', dest='tls_mode', default='detect',
                      choices=('y', 'n', 'detect'),
                      help="specify 'y' or 'n' to force enable/disable TLS")
  parser.add_argument('--tls-cert-file', metavar='TLS_CERT_FILE',
                      dest='tls_cert_file', type=str, default=None,
                      help='file containing the server TLS certificate in PEM '
                           'format')
  parser.add_argument('--tls-no-verify', dest='tls_no_verify',
                      action='store_true', default=False,
                      help='do not verify certificate if TLS is enabled')
  parser.add_argument('--prop-file', metavar='PROP_FILE', dest='prop_file',
                      type=str, default=None,
                      help='file containing the JSON representation of client '
                           'properties')
  parser.add_argument('--download', metavar='FILE', dest='download', type=str,
                      default=None, help='file to download')
  parser.add_argument('--reset', dest='reset', default=False,
                      action='store_true',
                      help='reset ghost and reload all configs')
  parser.add_argument('--status', dest='status', default=False,
                      action='store_true',
                      help='show status of the client')
  parser.add_argument('--allowlist', dest='allowlist',
                      help='comma-separated list of users/groups that can access this ghost')
  parser.add_argument('overlord_addr', metavar='OVERLORD_ADDR', type=str,
                      nargs='*',
                      help='overlord server address in format: host:port')
  args = parser.parse_args()

  if args.status:
    print(GhostRPCServer().GetStatus())
    sys.exit()

  if args.fork:
    ForkToBackground()

  if args.reset:
    GhostRPCServer().Reconnect()
    sys.exit()

  if args.download:
    DownloadFile(args.download)

  addrs = []

  for addr in args.overlord_addr:
    if ':' not in addr:
      addrs += [(addr, _DEFAULT_HTTPS_PORT), (addr, _DEFAULT_HTTP_PORT)]
    else:
      host, port = addr.split(':')
      addrs.append((host, int(port)))

  addrs += [('127.0.0.1', _DEFAULT_HTTPS_PORT),
            ('127.0.0.1', _DEFAULT_HTTP_PORT)]

  prop_file = os.path.abspath(args.prop_file) if args.prop_file else None

  tls_settings = TLSSettings(args.tls_cert_file, not args.tls_no_verify)
  tls_mode = args.tls_mode
  tls_mode = {'y': True, 'n': False, 'detect': None}[tls_mode]
  g = Ghost(addrs, tls_settings, Ghost.AGENT, args.mid, None,
            allowlist=args.allowlist, prop_file=prop_file, tls_mode=tls_mode)
  g.Start(args.lan_disc, args.rpc_server)


def try_main():
  # Setup logging format
  logger = logging.getLogger()
  logger.setLevel(logging.INFO)
  handler = logging.StreamHandler()
  formatter = logging.Formatter('%(asctime)s %(message)s', '%Y/%m/%d %H:%M:%S')
  handler.setFormatter(formatter)
  logger.addHandler(handler)

  try:
    main()
  except Exception as e:
    if _DEBUG:
      logging.exception(e)
    else:
      logging.error(e)


if __name__ == '__main__':
  try_main()
