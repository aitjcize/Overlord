#!/usr/bin/env python
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import argparse
import ast
import base64
import fcntl
import functools
import getpass
import hashlib
import http.client
from io import StringIO
import json
import logging
import os
import re
import select
import signal
import socket
import ssl
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time
import tty
import unicodedata  # required by pyinstaller, pylint: disable=unused-import
import urllib.error
import urllib.parse
import urllib.request
from xmlrpc.client import ServerProxy
from xmlrpc.server import SimpleXMLRPCServer

from ws4py.client import WebSocketBaseClient
import yaml


_CERT_DIR = os.path.expanduser('~/.config/ovl')

_ESCAPE = '~'
_BUFSIZ = 8192
_DEFAULT_HTTPS_PORT = 443
_OVERLORD_CLIENT_DAEMON_PORT = 4488
_OVERLORD_CLIENT_DAEMON_RPC_ADDR = ('127.0.0.1', _OVERLORD_CLIENT_DAEMON_PORT)

_CONNECT_TIMEOUT = 3
_DEFAULT_HTTP_TIMEOUT = 30
_LIST_CACHE_TIMEOUT = 2
_DEFAULT_TERMINAL_WIDTH = 80
_RETRY_TIMES = 3

# echo -n overlord | md5sum
_HTTP_BOUNDARY_MAGIC = '9246f080c855a69012707ab53489b921'

# Terminal resize control
_CONTROL_START = 128
_CONTROL_END = 129

# Stream control
_STDIN_CLOSED = '##STDIN_CLOSED##'

_SSH_CONTROL_SOCKET_PREFIX = os.path.join(tempfile.gettempdir(),
                                          'ovl-ssh-control-')

_TLS_CERT_FAILED_WARNING = """
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@ WARNING: REMOTE HOST VERIFICATION HAS FAILED! @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!
Someone could be eavesdropping on you right now (man-in-the-middle attack)!
It is also possible that the server is using a self-signed certificate.
The fingerprint for the TLS host certificate sent by the remote host is

%s

Do you want to trust this certificate and proceed? [y/N] """

_TLS_CERT_CHANGED_WARNING = """
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!
Someone could be eavesdropping on you right now (man-in-the-middle attack)!
It is also possible that the TLS host certificate has just been changed.
The fingerprint for the TLS host certificate sent by the remote host is

%s

Remove '%s' if you still want to proceed.
SSL Certificate verification failed."""

_USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36"


def GetVersionDigest():
  """Return the sha1sum of the current executing script."""
  # Check python script by default
  filename = __file__

  # If we are running from a frozen binary, we should calculate the checksum
  # against that binary instead of the python script.
  # See: https://pyinstaller.readthedocs.io/en/stable/runtime-information.html
  if getattr(sys, 'frozen', False):
    filename = sys.executable

  with open(filename, 'rb') as f:
    return hashlib.sha1(f.read()).hexdigest()


def GetTLSCertPath(host):
  return os.path.join(_CERT_DIR, '%s.cert' % host)


def UrlOpen(state, url):
  """Wrapper for urllib.request.urlopen.

  It selects correct HTTP scheme according to self._state.ssl, add HTTP
  basic auth headers, and add specify correct SSL context.
  """
  url = MakeRequestUrl(state, url)
  request = urllib.request.Request(url)
  if state.username is not None and state.password is not None:
    request.add_header(*BasicAuthHeader(state.username, state.password))
  request.add_header('User-Agent', _USER_AGENT)
  return urllib.request.urlopen(request, timeout=_DEFAULT_HTTP_TIMEOUT,
                                context=state.SSLContext())


def GetTLSCertificateSHA1Fingerprint(cert_pem):
  beg = cert_pem.index('\n')
  end = cert_pem.rindex('\n', 0, len(cert_pem) - 2)
  cert_pem = cert_pem[beg:end]  # Remove BEGIN/END CERTIFICATE boundary
  cert_der = base64.b64decode(cert_pem)
  return hashlib.sha1(cert_der).hexdigest()


def KillGraceful(pid, wait_secs=1):
  """Kill a process gracefully by first sending SIGTERM, wait for some time,
  then send SIGKILL to make sure it's killed."""
  try:
    os.kill(pid, signal.SIGTERM)
    time.sleep(wait_secs)
    os.kill(pid, signal.SIGKILL)
  except OSError:
    pass


def AutoRetry(action_name, retries):
  """Decorator for retry function call."""
  def Wrap(func):
    @functools.wraps(func)
    def Loop(*args, **kwargs):
      for unused_i in range(retries):
        try:
          func(*args, **kwargs)
        except Exception as e:
          print('error: %s: %s: retrying ...' % (args[0], e))
        else:
          break
      else:
        print('error: failed to %s %s' % (action_name, args[0]))
    return Loop
  return Wrap


def BasicAuthHeader(user, password):
  """Return HTTP basic auth header."""
  credential = base64.b64encode(
      b'%s:%s' % (user.encode('utf-8'), password.encode('utf-8')))
  return ('Authorization', 'Basic %s' % credential.decode('utf-8'))


def GetTerminalSize():
  """Retrieve terminal window size."""
  ws = struct.pack('HHHH', 0, 0, 0, 0)
  ws = fcntl.ioctl(0, termios.TIOCGWINSZ, ws)
  lines, columns, unused_x, unused_y = struct.unpack('HHHH', ws)
  return lines, columns


def MakeRequestUrl(state, url):
  return 'http%s://%s' % ('s' if state.ssl else '', url)


class ProgressBar:
  SIZE_WIDTH = 11
  SPEED_WIDTH = 10
  DURATION_WIDTH = 6
  PERCENTAGE_WIDTH = 8

  def __init__(self, name):
    self._start_time = time.time()
    self._name = name
    self._size = 0
    self._width = 0
    self._name_width = 0
    self._name_max = 0
    self._stat_width = 0
    self._max = 0
    self._CalculateSize()
    self.SetProgress(0)

  def _CalculateSize(self):
    self._width = GetTerminalSize()[1] or _DEFAULT_TERMINAL_WIDTH
    self._name_width = int(self._width * 0.3)
    self._name_max = self._name_width
    self._stat_width = self.SIZE_WIDTH + self.SPEED_WIDTH + self.DURATION_WIDTH
    self._max = (self._width - self._name_width - self._stat_width -
                 self.PERCENTAGE_WIDTH)

  def _SizeToHuman(self, size_in_bytes):
    if size_in_bytes < 1024:
      unit = 'B'
      value = size_in_bytes
    elif size_in_bytes < 1024 ** 2:
      unit = 'KiB'
      value = size_in_bytes / 1024
    elif size_in_bytes < 1024 ** 3:
      unit = 'MiB'
      value = size_in_bytes / (1024 ** 2)
    elif size_in_bytes < 1024 ** 4:
      unit = 'GiB'
      value = size_in_bytes / (1024 ** 3)
    return ' %6.1f %3s' % (value, unit)

  def _SpeedToHuman(self, speed_in_bs):
    if speed_in_bs < 1024:
      unit = 'B'
      value = speed_in_bs
    elif speed_in_bs < 1024 ** 2:
      unit = 'K'
      value = speed_in_bs / 1024
    elif speed_in_bs < 1024 ** 3:
      unit = 'M'
      value = speed_in_bs / (1024 ** 2)
    elif speed_in_bs < 1024 ** 4:
      unit = 'G'
      value = speed_in_bs / (1024 ** 3)
    return ' %6.1f%s/s' % (value, unit)

  def _DurationToClock(self, duration):
    return ' %02d:%02d' % (duration // 60, duration % 60)

  def SetProgress(self, percentage, size=None):
    current_width = GetTerminalSize()[1]
    if self._width != current_width:
      self._CalculateSize()

    if size is not None:
      self._size = size

    elapse_time = time.time() - self._start_time
    speed = self._size / elapse_time

    size_str = self._SizeToHuman(self._size)
    speed_str = self._SpeedToHuman(speed)
    elapse_str = self._DurationToClock(elapse_time)

    width = int(self._max * percentage / 100.0)
    sys.stdout.write(
        '%*s' % (- self._name_max,
                 self._name if len(self._name) <= self._name_max else
                 self._name[:self._name_max - 4] + ' ...') +
        size_str + speed_str + elapse_str +
        ((' [' + '#' * width + ' ' * (self._max - width) + ']' +
          '%4d%%' % int(percentage)) if self._max > 2 else '') + '\r')
    sys.stdout.flush()

  def End(self):
    self.SetProgress(100.0)
    sys.stdout.write('\n')
    sys.stdout.flush()


class DaemonState:
  """DaemonState is used for storing Overlord state info."""
  def __init__(self):
    self.version_sha1sum = GetVersionDigest()
    self.host = None
    self.port = None
    self.ssl = False
    self.ssl_self_signed = False
    self.ssl_verify = True
    self.ssl_check_hostname = True
    self.ssh = False
    self.orig_host = None
    self.ssh_pid = None
    self.username = None
    self.password = None
    self.selected_mid = None
    self.forwards = {}
    self.listing = []
    self.last_list = 0

  def SSLContext(self):
    # No verify.
    if not self.ssl_verify:
      context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
      context.check_hostname = False
      context.verify_mode = ssl.CERT_NONE
      return context

    context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    context.check_hostname = self.ssl_check_hostname
    context.verify_mode = ssl.CERT_REQUIRED

    # Check if self signed certificate exists.
    ssl_cert_path = GetTLSCertPath(self.host)
    if os.path.exists(ssl_cert_path):
      context.load_verify_locations(ssl_cert_path)
      self.ssl_self_signed = True
      return context

    return ssl.create_default_context(ssl.Purpose.SERVER_AUTH)


  @staticmethod
  def FromDict(kw):
    state = DaemonState()

    for k, v in kw.items():
      setattr(state, k, v)
    return state


class OverlordClientDaemon:
  """Overlord Client Daemon."""
  def __init__(self):
    self._state = DaemonState()
    self._server = None

  def Start(self):
    self.StartRPCServer()

  def StartRPCServer(self):
    self._server = SimpleXMLRPCServer(_OVERLORD_CLIENT_DAEMON_RPC_ADDR,
                                      logRequests=False, allow_none=True)
    exports = [
        (self.State, 'State'),
        (self.Ping, 'Ping'),
        (self.GetPid, 'GetPid'),
        (self.Connect, 'Connect'),
        (self.Clients, 'Clients'),
        (self.SelectClient, 'SelectClient'),
        (self.AddForward, 'AddForward'),
        (self.RemoveForward, 'RemoveForward'),
        (self.RemoveAllForward, 'RemoveAllForward'),
    ]
    for func, name in exports:
      self._server.register_function(func, name)

    pid = os.fork()
    if pid == 0:
      for fd in range(3):
        os.close(fd)
      self._server.serve_forever()

  @staticmethod
  def GetRPCServer():
    """Returns the Overlord client daemon RPC server."""
    server = ServerProxy('http://%s:%d' % _OVERLORD_CLIENT_DAEMON_RPC_ADDR,
                         allow_none=True)
    try:
      server.Ping()
    except Exception:
      return None
    return server

  def State(self):
    return self._state

  def Ping(self):
    return True

  def GetPid(self):
    return os.getpid()

  def _GetJSON(self, path):
    url = '%s:%d%s' % (self._state.host, self._state.port, path)
    return json.loads(UrlOpen(self._state, url).read())

  def _TLSEnabled(self):
    """Determine if TLS is enabled on given server address."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
      # Allow any certificate since we only want to check if server talks TLS.
      context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
      context.check_hostname = False
      context.verify_mode = ssl.CERT_NONE

      sock = context.wrap_socket(sock, server_hostname=self._state.host)
      sock.settimeout(_CONNECT_TIMEOUT)
      sock.connect((self._state.host, self._state.port))
      return True
    except ssl.SSLError:
      return False
    except socket.error:  # Connect refused or timeout
      raise
    except Exception:
      return False  # For whatever reason above failed, assume False

  def _CheckTLSCertificate(self):
    """Check TLS certificate.

    Returns:
      A tupple (check_result, if_certificate_is_loaded)
    """
    def _DoConnect(context):
      sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
      try:
        sock.settimeout(_CONNECT_TIMEOUT)
        sock = context.wrap_socket(sock, server_hostname=self._state.host)
        sock.connect((self._state.host, self._state.port))
      except ssl.SSLError:
        return False
      finally:
        sock.close()

      return True

    return _DoConnect(self._state.SSLContext())

  def Connect(self, host, port, ssh_pid=None,
              username=None, password=None, orig_host=None,
              ssl_verify=True, ssl_check_hostname=True):
    self._state.username = username
    self._state.password = password
    self._state.host = host
    self._state.port = port
    self._state.ssl = False
    self._state.ssl_self_signed = False
    self._state.orig_host = orig_host
    self._state.ssh_pid = ssh_pid
    self._state.selected_mid = None
    self._state.ssl_verify = ssl_verify
    self._state.ssl_check_hostname = ssl_check_hostname

    ssl_enabled = self._TLSEnabled()
    if ssl_enabled:
      result = self._CheckTLSCertificate()
      if not result:
        if self._state.ssl_self_signed:
          return ('SSLCertificateChanged', ssl.get_server_certificate(
              (self._state.host, self._state.port)))
        return ('SSLVerifyFailed', ssl.get_server_certificate(
            (self._state.host, self._state.port)))

    try:
      self._state.ssl = ssl_enabled
      UrlOpen(self._state, '%s:%d' % (host, port))
    except urllib.error.HTTPError as e:
      return ('HTTPError', e.getcode(), str(e), e.read().strip())
    except Exception as e:
      return str(e)
    else:
      return True

  def Clients(self):
    if time.time() - self._state.last_list <= _LIST_CACHE_TIMEOUT:
      return self._state.listing

    self._state.listing = self._GetJSON('/api/agents/list')
    self._state.last_list = time.time()
    return self._state.listing

  def SelectClient(self, mid):
    self._state.selected_mid = mid

  def AddForward(self, mid, remote, local, pid):
    self._state.forwards[str(local)] = (mid, remote, pid)

  def RemoveForward(self, local_port):
    try:
      unused_mid, unused_remote, pid = self._state.forwards[str(local_port)]
      KillGraceful(pid)
      del self._state.forwards[local_port]
    except (KeyError, OSError):
      pass

  def RemoveAllForward(self):
    for unused_mid, unused_remote, pid in self._state.forwards.values():
      try:
        KillGraceful(pid)
      except OSError:
        pass
    self._state.forwards = {}


class SSLEnabledWebSocketBaseClient(WebSocketBaseClient):
  def __init__(self, state, *args, **kwargs):
    super().__init__(ssl_context=state.SSLContext(), *args, **kwargs)


class TerminalWebSocketClient(SSLEnabledWebSocketBaseClient):
  def __init__(self, state, mid, escape, *args, **kwargs):
    super().__init__(state, *args, **kwargs)
    self._mid = mid
    self._escape = escape
    self._stdin_fd = sys.stdin.fileno()
    self._old_termios = None

  def handshake_ok(self):
    pass

  def opened(self):
    nonlocals = {'size': (80, 40)}

    def _ResizeWindow():
      size = GetTerminalSize()
      if size != nonlocals['size']:  # Size not changed, ignore
        control = {'command': 'resize', 'params': list(size)}
        payload = (_CONTROL_START.to_bytes(1, 'big') +
                   json.dumps(control).encode('utf-8') +
                   _CONTROL_END.to_bytes(1, 'big'))
        nonlocals['size'] = size
        try:
          self.send(payload, binary=True)
        except Exception as e:
          logging.exception(e)

    def _FeedInput():
      self._old_termios = termios.tcgetattr(self._stdin_fd)
      tty.setraw(self._stdin_fd)

      READY, ENTER_PRESSED, ESCAPE_PRESSED = range(3)

      try:
        state = READY
        while True:
          # Check if terminal is resized
          _ResizeWindow()

          ch = sys.stdin.read(1)

          # Scan for escape sequence
          if self._escape:
            if state == READY:
              state = ENTER_PRESSED if ch == chr(0x0d) else READY
            elif state == ENTER_PRESSED:
              state = ESCAPE_PRESSED if ch == self._escape else READY
            elif state == ESCAPE_PRESSED:
              if ch == '.':
                self.close()
                break
            else:
              state = READY

          self.send(ch)
      except (KeyboardInterrupt, RuntimeError):
        pass

    t = threading.Thread(target=_FeedInput)
    t.daemon = True
    t.start()

  def closed(self, code, reason=None):
    del code, reason  # Unused.
    termios.tcsetattr(self._stdin_fd, termios.TCSANOW, self._old_termios)
    print('\nConnection to %s closed.' % self._mid)

  def received_message(self, message):
    if message.is_binary:
      sys.stdout.buffer.write(message.data)
      sys.stdout.flush()


class ShellWebSocketClient(SSLEnabledWebSocketBaseClient):
  def __init__(self, state, output, *args, **kwargs):
    """Constructor.

    Args:
      output: output file object.
    """
    super().__init__(state, *args, **kwargs)
    self._output = output
    self._input_thread = threading.Thread(target=self._FeedInput)
    self._stop = threading.Event()

  def handshake_ok(self):
    pass

  def _FeedInput(self):
    try:
      while True:
        rd, unused_w, unused_x = select.select([sys.stdin], [], [], 0.5)
        if self._stop.is_set():
          break

        data = sys.stdin.read()
        if not data:
          self.send(_STDIN_CLOSED * 2)
          break
        self.send(data, binary=True)
    except (KeyboardInterrupt, RuntimeError):
      pass

  def opened(self):
    self._input_thread.start()

  def closed(self, code, reason=None):
    self._stop.set()
    self._input_thread.join()

  def received_message(self, message):
    if message.is_binary:
      self._output.write(message.data.decode('utf-8'))
      self._output.flush()


class ForwarderWebSocketClient(SSLEnabledWebSocketBaseClient):
  def __init__(self, state, sock, *args, **kwargs):
    super().__init__(state, *args, **kwargs)
    self._sock = sock
    self._input_thread = threading.Thread(target=self._FeedInput)
    self._stop = threading.Event()

  def handshake_ok(self):
    pass

  def _FeedInput(self):
    try:
      self._sock.setblocking(False)
      while True:
        rd, unused_w, unused_x = select.select([self._sock], [], [], 0.5)
        if self._stop.is_set():
          break
        if self._sock in rd:
          data = self._sock.recv(_BUFSIZ)
          if not data:
            self.close()
            break
          self.send(data, binary=True)
    except Exception:
      pass
    finally:
      self._sock.close()


  def opened(self):
    self._input_thread.start()

  def closed(self, code, reason=None):
    del code, reason  # Unused.
    self._stop.set()
    self._input_thread.join()
    sys.exit(0)

  def received_message(self, message):
    if message.is_binary:
      self._sock.send(message.data)


def Arg(*args, **kwargs):
  return (args, kwargs)


def Command(command, help_msg=None, args=None):
  """Decorator for adding argparse parameter for a method."""
  if args is None:
    args = []
  def WrapFunc(func):
    @functools.wraps(func)
    def Wrapped(*args, **kwargs):
      return func(*args, **kwargs)
    # pylint: disable=protected-access
    Wrapped.__arg_attr = {'command': command, 'help': help_msg, 'args': args}
    return Wrapped
  return WrapFunc


def ParseMethodSubCommands(cls):
  """Decorator for a class using the @Command decorator.

  This decorator retrieve command info from each method and append it in to the
  SUBCOMMANDS class variable, which is later used to construct parser.
  """
  for unused_key, method in cls.__dict__.items():
    if hasattr(method, '__arg_attr'):
      # pylint: disable=protected-access
      cls.SUBCOMMANDS.append(method.__arg_attr)
  return cls


@ParseMethodSubCommands
class OverlordCLIClient:
  """Overlord command line interface client."""

  SUBCOMMANDS = []

  def __init__(self):
    self._parser = self._BuildParser()
    self._selected_mid = None
    self._server = None
    self._state = None
    self._escape = None

  def _BuildParser(self):
    root_parser = argparse.ArgumentParser(prog='ovl')
    subparsers = root_parser.add_subparsers(title='subcommands',
                                            dest='subcommand')
    subparsers.required = True

    root_parser.add_argument('-s', dest='selected_mid', action='store',
                             default=None,
                             help='select target to execute command on')
    root_parser.add_argument('-S', dest='select_mid_before_action',
                             action='store_true', default=False,
                             help='select target before executing command')
    root_parser.add_argument('-e', dest='escape', metavar='ESCAPE_CHAR',
                             action='store', default=_ESCAPE, type=str,
                             help='set shell escape character, \'none\' to '
                             'disable escape completely')

    for attr in self.SUBCOMMANDS:
      parser = subparsers.add_parser(attr['command'], help=attr['help'])
      parser.set_defaults(which=attr['command'])
      for arg in attr['args']:
        parser.add_argument(*arg[0], **arg[1])

    return root_parser

  def Main(self):
    # We want to pass the rest of arguments after shell command directly to the
    # function without parsing it.
    try:
      index = sys.argv.index('shell')
    except ValueError:
      args = self._parser.parse_args()
    else:
      args = self._parser.parse_args(sys.argv[1:index + 1])

    command = args.which
    self._selected_mid = args.selected_mid

    if args.escape and args.escape != 'none':
      self._escape = args.escape[0]

    if command == 'start-server':
      self.StartServer()
      return
    if command == 'kill-server':
      self.KillServer()
      return

    self.CheckDaemon()
    if command == 'status':
      self.Status()
      return
    if command == 'connect':
      self.Connect(args)
      return

    # The following command requires connection to the server
    self.CheckConnection()

    if args.select_mid_before_action:
      self.SelectClient(store=False)

    if command == 'select':
      self.SelectClient(args)
    elif command == 'ls':
      self.ListClients(args)
    elif command == 'shell':
      command = sys.argv[sys.argv.index('shell') + 1:]
      self.Shell(command)
    elif command == 'push':
      self.Push(args)
    elif command == 'pull':
      self.Pull(args)
    elif command == 'forward':
      self.Forward(args)

  def _SaveTLSCertificate(self, host, cert_pem):
    try:
      os.makedirs(_CERT_DIR)
    except Exception:
      pass
    with open(GetTLSCertPath(host), 'w') as f:
      f.write(cert_pem)

  def _HTTPPostFile(self, url, filename, progress=None, user=None, passwd=None):
    """Perform HTTP POST and upload file to Overlord.

    To minimize the external dependencies, we construct the HTTP post request
    by ourselves.
    """
    url = MakeRequestUrl(self._state, url)
    size = os.stat(filename).st_size
    boundary = '-----------%s' % _HTTP_BOUNDARY_MAGIC
    CRLF = '\r\n'
    parse = urllib.parse.urlparse(url)

    part_headers = [
        '--' + boundary,
        'Content-Disposition: form-data; name="file"; '
        'filename="%s"' % os.path.basename(filename),
        'Content-Type: application/octet-stream',
        '', ''
    ]
    part_header = CRLF.join(part_headers)
    end_part = CRLF + '--' + boundary + '--' + CRLF

    content_length = len(part_header) + size + len(end_part)
    if parse.scheme == 'http':
      h = http.client.HTTPConnection(parse.netloc)
    else:
      h = http.client.HTTPSConnection(parse.netloc,
                                      context=self._state.SSLContext())

    post_path = url[url.index(parse.netloc) + len(parse.netloc):]
    h.putrequest('POST', post_path)
    h.putheader('Content-Length', content_length)
    h.putheader('Content-Type', 'multipart/form-data; boundary=%s' % boundary)

    if user and passwd:
      h.putheader(*BasicAuthHeader(user, passwd))
    h.endheaders()
    h.send(part_header.encode('utf-8'))

    count = 0
    with open(filename, 'rb') as f:
      while True:
        data = f.read(_BUFSIZ)
        if not data:
          break
        count += len(data)
        if progress:
          progress(count * 100 // size, count)
        h.send(data)

    h.send(end_part.encode('utf-8'))
    progress(100)

    if count != size:
      logging.warning('file changed during upload, upload may be truncated.')

    resp = h.getresponse()
    return resp.status == 200

  def CheckDaemon(self):
    self._server = OverlordClientDaemon.GetRPCServer()
    if self._server is None:
      print('* daemon not running, starting it now on port %d ... *' %
            _OVERLORD_CLIENT_DAEMON_PORT)
      self.StartServer()

    self._state = DaemonState.FromDict(self._server.State())
    sha1sum = GetVersionDigest()

    if sha1sum != self._state.version_sha1sum:
      print('ovl server is out of date.  killing...')
      KillGraceful(self._server.GetPid())
      self.StartServer()

  def GetSSHControlFile(self, host):
    return _SSH_CONTROL_SOCKET_PREFIX + host

  def SSHTunnel(self, user, host, port):
    """SSH forward the remote overlord server.

    Overlord server may not have port 9000 open to the public network, in such
    case we can SSH forward the port to 127.0.0.1.
    """

    control_file = self.GetSSHControlFile(host)
    try:
      os.unlink(control_file)
    except Exception:
      pass

    with subprocess.Popen([
        'ssh', '-Nf', '-M', '-S', control_file, '-L', '9000:127.0.0.1:9000',
        '-p',
        str(port),
        '%s%s' % (user + '@' if user else '', host)
    ]):
      pass

    p = subprocess.Popen([
        'ssh',
        '-S', control_file,
        '-O', 'check', host,
    ], stderr=subprocess.PIPE)
    unused_stdout, stderr = p.communicate()

    s = re.search(r'pid=(\d+)', stderr)
    if s:
      return int(s.group(1))

    raise RuntimeError('can not establish ssh connection')

  def CheckConnection(self):
    if self._state.host is None:
      raise RuntimeError('not connected to any server, abort')

    try:
      self._server.Clients()
    except Exception:
      raise RuntimeError('remote server disconnected, abort') from None

    if self._state.ssh_pid is not None:
      with subprocess.Popen(
          ['kill', '-0', str(self._state.ssh_pid)], stdout=subprocess.PIPE,
          stderr=subprocess.PIPE) as p:
        pass
      if p.returncode != 0:
        raise RuntimeError('ssh tunnel disconnected, please re-connect')

  def CheckClient(self):
    if self._selected_mid is None:
      if self._state.selected_mid is None:
        raise RuntimeError('No client is selected')
      self._selected_mid = self._state.selected_mid

    if not any(client['mid'] == self._selected_mid
               for client in self._server.Clients()):
      raise RuntimeError('client %s disappeared' % self._selected_mid)

  def CheckOutput(self, command):
    headers = []
    if self._state.username is not None and self._state.password is not None:
      headers.append(BasicAuthHeader(self._state.username,
                                     self._state.password))

    scheme = 'ws%s://' % ('s' if self._state.ssl else '')
    sio = StringIO()
    ws = ShellWebSocketClient(
        self._state, sio, scheme + '%s:%d/api/agent/shell/%s?command=%s' % (
            self._state.host, self._state.port,
            urllib.parse.quote(self._selected_mid),
            urllib.parse.quote(command)),
        headers=headers)
    ws.connect()
    ws.run()
    return sio.getvalue()

  @Command('status', 'show Overlord connection status')
  def Status(self):
    if self._state.host is None:
      print('Not connected to any host.')
    else:
      if self._state.ssh_pid is not None:
        print('Connected to %s with SSH tunneling.' % self._state.orig_host)
      else:
        print('Connected to %s:%d.' % (self._state.host, self._state.port))

    if self._selected_mid is None:
      self._selected_mid = self._state.selected_mid

    if self._selected_mid is None:
      print('No client is selected.')
    else:
      print('Client %s selected.' % self._selected_mid)

  @Command('connect', 'connect to Overlord server', [
      Arg('host', metavar='HOST', type=str, default='127.0.0.1',
          help='Overlord hostname/IP'),
      Arg('port', metavar='PORT', type=int, nargs='?',
          default=_DEFAULT_HTTPS_PORT, help='Overlord port'),
      Arg('-f', '--forward', dest='ssh_forward', default=False,
          action='store_true',
          help='connect with SSH forwarding to the host'),
      Arg('-p', '--ssh-port', dest='ssh_port', default=22,
          type=int, help='SSH server port for SSH forwarding'),
      Arg('-l', '--ssh-login', dest='ssh_login', default='',
          type=str, help='SSH server login name for SSH forwarding'),
      Arg('-u', '--user', dest='user', default=None,
          type=str, help='Overlord HTTP auth username'),
      Arg('-w', '--passwd', dest='passwd', default=None, type=str,
          help='Overlord HTTP auth password'),
      Arg('--ssl-no-verify', dest='ssl_verify',
          default=True, action='store_false',
          help='Ignore SSL cert verification'),
      Arg('--ssl-no-check-hostname', dest='ssl_check_hostname',
          default=True, action='store_false',
          help='Ignore SSL cert hostname check')])
  def Connect(self, args):
    ssh_pid = None
    host = args.host
    orig_host = args.host

    if args.ssh_forward:
      # Kill previous SSH tunnel
      self.KillSSHTunnel()

      ssh_pid = self.SSHTunnel(args.ssh_login, args.host, args.ssh_port)
      host = '127.0.0.1'

    username_provided = args.user is not None
    password_provided = args.passwd is not None
    prompt = False

    for unused_i in range(3):  # pylint: disable=too-many-nested-blocks
      try:
        if prompt:
          if not username_provided:
            args.user = input('Username: ')
          if not password_provided:
            args.passwd = getpass.getpass('Password: ')

        ret = self._server.Connect(host, args.port, ssh_pid, args.user,
                                   args.passwd, orig_host,
                                   args.ssl_verify, args.ssl_check_hostname)
        if isinstance(ret, list):
          if ret[0].startswith('SSL'):
            cert_pem = ret[1]
            fp = GetTLSCertificateSHA1Fingerprint(cert_pem)
            fp_text = ':'.join([fp[i:i+2] for i in range(0, len(fp), 2)])

          if ret[0] == 'SSLCertificateChanged':
            print(_TLS_CERT_CHANGED_WARNING % (fp_text, GetTLSCertPath(host)))
            return
          if ret[0] == 'SSLVerifyFailed':
            print(_TLS_CERT_FAILED_WARNING % (fp_text), end='')
            response = input()
            if response.lower() in ['y', 'ye', 'yes']:
              self._SaveTLSCertificate(host, cert_pem)
              print('TLS host Certificate trusted, you will not be prompted '
                    'next time.\n')
              continue
            print('connection aborted.')
            return
          if ret[0] == 'HTTPError':
            code, except_str, body = ret[1:]
            if code == 401:
              print('connect: %s' % body)
              prompt = True
              if not username_provided or not password_provided:
                continue
              break
            logging.error('%s; %s', except_str, body)

        if ret is not True:
          print('can not connect to %s: %s' % (host, ret))
        else:
          print('connection to %s:%d established.' % (host, args.port))
      except Exception as e:
        logging.error(e)
      else:
        break

  @Command('start-server', 'start overlord CLI client server')
  def StartServer(self):
    self._server = OverlordClientDaemon.GetRPCServer()
    if self._server is None:
      OverlordClientDaemon().Start()
      time.sleep(1)
      self._server = OverlordClientDaemon.GetRPCServer()
      if self._server is not None:
        print('* daemon started successfully *\n')

  @Command('kill-server', 'kill overlord CLI client server')
  def KillServer(self):
    self._server = OverlordClientDaemon.GetRPCServer()
    if self._server is None:
      return

    self._state = DaemonState.FromDict(self._server.State())

    # Kill SSH Tunnel
    self.KillSSHTunnel()

    # Kill server daemon
    KillGraceful(self._server.GetPid())

  def KillSSHTunnel(self):
    if self._state.ssh_pid is not None:
      KillGraceful(self._state.ssh_pid)

  def _FilterClients(self, clients, prop_filters, mid=None):
    def _ClientPropertiesMatch(client, key, regex):
      try:
        return bool(re.search(regex, client['properties'][key]))
      except KeyError:
        return False

    for prop_filter in prop_filters:
      key, sep, regex = prop_filter.partition('=')
      if not sep:
        # The filter doesn't contains =.
        raise ValueError('Invalid filter condition %r' % filter)
      clients = [c for c in clients if _ClientPropertiesMatch(c, key, regex)]

    if mid is not None:
      client = next((c for c in clients if c['mid'] == mid), None)
      if client:
        return [client]
      clients = [c for c in clients if c['mid'].startswith(mid)]
    return clients

  @Command('ls', 'list clients', [
      Arg('-f', '--filter', default=[], dest='filters', action='append',
          help=('Conditions to filter clients by properties. '
                'Should be in form "key=regex", where regex is the regular '
                'expression that should be found in the value. '
                'Multiple --filter arguments would be ANDed.')),
      Arg('-v', '--verbose', default=False, action='store_true',
          help='Print properties of each client.')
  ])
  def ListClients(self, args):
    clients = self._FilterClients(self._server.Clients(), args.filters)
    for client in clients:
      if args.verbose:
        print(yaml.safe_dump(client, default_flow_style=False))
      else:
        print(client['mid'])

  @Command('select', 'select default client', [
      Arg('-f', '--filter', default=[], dest='filters', action='append',
          help=('Conditions to filter clients by properties. '
                'Should be in form "key=regex", where regex is the regular '
                'expression that should be found in the value. '
                'Multiple --filter arguments would be ANDed.')),
      Arg('mid', metavar='mid', nargs='?', default=None)])
  def SelectClient(self, args=None, store=True):
    mid = args.mid if args is not None else None
    filters = args.filters if args is not None else []
    clients = self._FilterClients(self._server.Clients(), filters, mid=mid)

    if not clients:
      raise RuntimeError('select: client not found')
    if len(clients) == 1:
      mid = clients[0]['mid']
    else:
      # This case would not happen when args.mid is specified.
      print('Select from the following clients:')
      for i, client in enumerate(clients):
        print('    %d. %s' % (i + 1, client['mid']))

      print('\nSelection: ', end='')
      try:
        choice = int(input()) - 1
        mid = clients[choice]['mid']
      except ValueError:
        raise RuntimeError('select: invalid selection') from None
      except IndexError:
        raise RuntimeError('select: selection out of range') from None

    self._selected_mid = mid
    if store:
      self._server.SelectClient(mid)
      print('Client %s selected' % mid)

  @Command('shell', 'open a shell or execute a shell command', [
      Arg('command', metavar='CMD', nargs='?', help='command to execute')])
  def Shell(self, command=None):
    if command is None:
      command = []
    self.CheckClient()

    headers = []
    if self._state.username is not None and self._state.password is not None:
      headers.append(BasicAuthHeader(self._state.username,
                                     self._state.password))

    scheme = 'ws%s://' % ('s' if self._state.ssl else '')
    if command:
      cmd = ' '.join(command)
      ws = ShellWebSocketClient(
          self._state, sys.stdout,
          scheme + '%s:%d/api/agent/shell/%s?command=%s' % (
              self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid), urllib.parse.quote(cmd)),
          headers=headers)
    else:
      ws = TerminalWebSocketClient(
          self._state, self._selected_mid, self._escape,
          scheme + '%s:%d/api/agent/tty/%s' % (
              self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid)),
          headers=headers)
    try:
      ws.connect()
      ws.run()
    except socket.error as e:
      if e.errno == 32:  # Broken pipe
        pass
      else:
        raise

  @Command('push', 'push a file or directory to remote', [
      Arg('srcs', nargs='+', metavar='SOURCE'),
      Arg('dst', metavar='DESTINATION')])
  def Push(self, args):
    self.CheckClient()

    @AutoRetry('push', _RETRY_TIMES)
    def _push(src, dst):
      src_base = os.path.basename(src)

      # Local file is a link
      if os.path.islink(src):
        pbar = ProgressBar(src_base)
        link_path = os.readlink(src)
        self.CheckOutput('mkdir -p %(dirname)s; '
                         'if [ -d "%(dst)s" ]; then '
                         'ln -sf "%(link_path)s" "%(dst)s/%(link_name)s"; '
                         'else ln -sf "%(link_path)s" "%(dst)s"; fi' %
                         dict(dirname=os.path.dirname(dst),
                              link_path=link_path, dst=dst,
                              link_name=src_base))
        pbar.End()
        return

      mode = '0%o' % (0x1FF & os.stat(src).st_mode)
      url = ('%s:%d/api/agent/upload/%s?dest=%s&perm=%s' %
             (self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid), dst, mode))
      try:
        UrlOpen(self._state, url + '&filename=%s' % src_base)
      except urllib.error.HTTPError as e:
        msg = json.loads(e.read()).get('error', None)
        raise RuntimeError('push: %s' % msg) from None

      pbar = ProgressBar(src_base)
      self._HTTPPostFile(url, src, pbar.SetProgress,
                         self._state.username, self._state.password)
      pbar.End()

    def _push_single_target(src, dst):
      if os.path.isdir(src):
        dst_exists = ast.literal_eval(self.CheckOutput(
            'stat %s >/dev/null 2>&1 && echo True || echo False' % dst))
        for root, unused_x, files in os.walk(src):
          # If destination directory does not exist, we should strip the first
          # layer of directory. For example: src_dir contains a single file 'A'
          #
          # push src_dir dest_dir
          #
          # If dest_dir exists, the resulting directory structure should be:
          #   dest_dir/src_dir/A
          # If dest_dir does not exist, the resulting directory structure should
          # be:
          #   dest_dir/A
          dst_root = root if dst_exists else root[len(src):].lstrip('/')
          for name in files:
            _push(os.path.join(root, name),
                  os.path.join(dst, dst_root, name))
      else:
        _push(src, dst)

    if len(args.srcs) > 1:
      dst_type = self.CheckOutput('stat \'%s\' --printf \'%%F\' '
                                  '2>/dev/null' % args.dst).strip()
      if not dst_type:
        raise RuntimeError('push: %s: No such file or directory' % args.dst)
      if dst_type != 'directory':
        raise RuntimeError('push: %s: Not a directory' % args.dst)

    for src in args.srcs:
      if not os.path.exists(src):
        raise RuntimeError('push: can not stat "%s": no such file or directory'
                           % src)
      if not os.access(src, os.R_OK):
        raise RuntimeError('push: can not open "%s" for reading' % src)

      _push_single_target(src, args.dst)

  @Command('pull', 'pull a file or directory from remote', [
      Arg('src', metavar='SOURCE'),
      Arg('dst', metavar='DESTINATION', default='.', nargs='?')])
  def Pull(self, args):
    self.CheckClient()

    @AutoRetry('pull', _RETRY_TIMES)
    def _pull(src, dst, ftype, perm=0o644, link=None):
      try:
        os.makedirs(os.path.dirname(dst))
      except Exception:
        pass

      src_base = os.path.basename(src)

      # Remote file is a link
      if ftype == 'l':
        pbar = ProgressBar(src_base)
        if os.path.exists(dst):
          os.remove(dst)
        os.symlink(link, dst)
        pbar.End()
        return

      url = ('%s:%d/api/agent/download/%s?filename=%s' %
             (self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid), urllib.parse.quote(src)))
      try:
        h = UrlOpen(self._state, url)
      except urllib.error.HTTPError as e:
        msg = json.loads(e.read()).get('error', 'unkown error')
        raise RuntimeError('pull: %s' % msg) from None
      except KeyboardInterrupt:
        return

      pbar = ProgressBar(src_base)
      with open(dst, 'wb') as f:
        os.fchmod(f.fileno(), perm)
        total_size = int(h.headers.get('Content-Length'))
        downloaded_size = 0

        while True:
          data = h.read(_BUFSIZ)
          if not data:
            break
          downloaded_size += len(data)
          pbar.SetProgress(downloaded_size * 100 / total_size,
                           downloaded_size)
          f.write(data)
      pbar.End()

    # Use find to get a listing of all files under a root directory. The 'stat'
    # command is used to retrieve the filename and it's filemode.
    output = self.CheckOutput(
        'cd $HOME; '
        'stat "%(src)s" >/dev/null && '
        'find "%(src)s" \'(\' -type f -o -type l \')\' '
        '-printf \'%%m\t%%p\t%%y\t%%l\n\''
        % {'src': args.src})

    # We got error from the stat command
    if output.startswith('stat: '):
      sys.stderr.write(output)
      return

    entries = output.strip('\n').split('\n')
    common_prefix = os.path.dirname(args.src)

    if len(entries) == 1:
      entry = entries[0]
      perm, src_path, ftype, link = entry.split('\t', -1)
      if os.path.isdir(args.dst):
        dst = os.path.join(args.dst, os.path.basename(src_path))
      else:
        dst = args.dst
      _pull(src_path, dst, ftype, int(perm, base=8), link)
    else:
      if not os.path.exists(args.dst):
        common_prefix = args.src

      for entry in entries:
        perm, src_path, ftype, link = entry.split('\t', -1)
        rel_dst = src_path[len(common_prefix):].lstrip('/')
        _pull(src_path, os.path.join(args.dst, rel_dst), ftype,
              int(perm, base=8), link)

  @Command('forward', 'forward remote port to local port', [
      Arg('--list', dest='list_all', action='store_true', default=False,
          help='list all port forwarding sessions'),
      Arg('--remove', metavar='LOCAL_PORT', dest='remove', type=int,
          default=None,
          help='remove port forwarding for local port LOCAL_PORT'),
      Arg('--remove-all', dest='remove_all', action='store_true',
          default=False, help='remove all port forwarding'),
      Arg('remote', metavar='REMOTE_PORT', type=int, nargs='?'),
      Arg('local', metavar='LOCAL_PORT', type=int, nargs='?')])
  def Forward(self, args):
    if args.list_all:
      max_len = 10
      if self._state.forwards:
        max_len = max([len(v[0]) for v in self._state.forwards.values()])

      print('%-*s   %-8s  %-8s' % (max_len, 'Client', 'Remote', 'Local'))
      for local in sorted(self._state.forwards.keys()):
        value = self._state.forwards[local]
        print('%-*s   %-8s  %-8s' % (max_len, value[0], value[1], local))
      return

    if args.remove_all:
      self._server.RemoveAllForward()
      return

    if args.remove:
      self._server.RemoveForward(args.remove)
      return

    self.CheckClient()

    if args.remote is None:
      raise RuntimeError('remote port not specified')

    if args.local is None:
      args.local = args.remote
    remote = int(args.remote)
    local = int(args.local)

    def HandleConnection(conn):
      headers = []
      if self._state.username is not None and self._state.password is not None:
        headers.append(BasicAuthHeader(self._state.username,
                                       self._state.password))

      scheme = 'ws%s://' % ('s' if self._state.ssl else '')
      ws = ForwarderWebSocketClient(
          self._state, conn,
          scheme + '%s:%d/api/agent/forward/%s?port=%d' % (
              self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid), remote),
          headers=headers)
      try:
        ws.connect()
        ws.run()
      except Exception as e:
        print('error: %s' % e)
      finally:
        ws.close()

    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(('0.0.0.0', local))
    server.listen(5)

    pid = os.fork()
    if pid == 0:
      while True:
        conn, unused_addr = server.accept()
        t = threading.Thread(target=HandleConnection, args=(conn,))
        t.daemon = True
        t.start()
    else:
      self._server.AddForward(self._selected_mid, remote, local, pid)


def main():
  # Setup logging format
  logger = logging.getLogger()
  logger.setLevel(logging.INFO)
  handler = logging.StreamHandler()
  formatter = logging.Formatter('%(asctime)s %(message)s', '%Y/%m/%d %H:%M:%S')
  handler.setFormatter(formatter)
  logger.addHandler(handler)

  ovl = OverlordCLIClient()
  try:
    ovl.Main()
  except KeyboardInterrupt:
    print('Ctrl-C received, abort')
  except Exception as e:
    print(f'error: {e}')


if __name__ == '__main__':
  main()
