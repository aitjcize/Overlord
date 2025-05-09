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
from io import BytesIO
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

_DEBUG = False
_DEBUG_NO_KILL = False
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


def MakeRequestUrl(state, url):
  """Create a full URL with the correct protocol and host."""
  if url.startswith('http://') or url.startswith('https://'):
    return url

  return 'http%s://%s:%d%s' % (
      's' if state.ssl else '',
      state.host,
      state.port,
      url if url.startswith('/') else '/' + url)


def UrlOpen(state, url, headers=[], data=None, method='GET'):
  """Open a URL with proper headers.

  Args:
    state: DaemonState object.
    url: URL to open.
    headers: Additional headers to add.
    data: Data to send for POST requests (will be JSON encoded).
    method: HTTP method to use (GET or POST).

  Returns:
    urllib.response.addinfourl: Response from the server.
  """
  url = MakeRequestUrl(state, url)
  headers = list(headers)  # Make a copy to avoid modifying the original
  headers.append(('User-Agent', _USER_AGENT))
  if state.jwt_token:
    headers.append(JWTAuthHeader(state.jwt_token))
  if data is not None:
    headers.append(('Content-Type', 'application/json'))
    data = json.dumps(data).encode('utf-8')

  req = urllib.request.Request(
      url=url,
      data=data,
      headers=dict(headers),
      method=method)
  return urllib.request.urlopen(req, context=state.SSLContext())


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
          if _DEBUG:
            logging.exception(e)
          print('error: %s: %s: retrying ...' % (args[0], e))
        else:
          break
      else:
        print('error: failed to %s %s' % (action_name, args[0]))
    return Loop
  return Wrap


def JWTAuthHeader(token):
  """Return HTTP JWT auth header."""
  return ('Authorization', 'Bearer %s' % token)


def GetTerminalSize():
  """Retrieve terminal window size."""
  try:
    ws = struct.pack('HHHH', 0, 0, 0, 0)
    ws = fcntl.ioctl(0, termios.TIOCGWINSZ, ws)
    lines, columns, unused_x, unused_y = struct.unpack('HHHH', ws)
    return lines, columns
  except (IOError, struct.error, ValueError):
    # Default values if we can't get the terminal size
    return 24, 80


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
    self.jwt_token = None
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
      if not _DEBUG:
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

  def _GetJSON(self, url):
    try:
      response = UrlOpen(self._state, url)
      return json.loads(response.read().decode('utf-8')).get('data', [])
    except urllib.error.HTTPError as e:
      error = json.loads(e.read()).get('data', 'unknown error')
      raise RuntimeError('GET %s: %s' % (url, error)) from e

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

  def GetJWTToken(self):
    """Get JWT token from server.

    Returns:
      JWT token string or None if authentication fails.
    """
    url = '/api/auth/login'
    data = json.dumps({
      'username': self._state.username,
      'password': self._state.password
    }).encode('utf-8')

    headers = {
      'Content-Type': 'application/json',
      'User-Agent': _USER_AGENT
    }

    full_url = MakeRequestUrl(self._state, url)
    request = urllib.request.Request(full_url, data=data, headers=headers)

    try:
      context = self._state.SSLContext()
      with urllib.request.urlopen(request, timeout=_DEFAULT_HTTP_TIMEOUT,
                                  context=context) as response:
        result = json.loads(response.read().decode('utf-8')).get('data', {})
        return result.get('token')
    except urllib.error.HTTPError as e:
      error = json.loads(e.read().decode('utf-8')).get('data', 'unknown error')
      raise RuntimeError(error) from e

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
      self._state.jwt_token = self.GetJWTToken()
      self._GetJSON('/api/agents')
    except urllib.error.HTTPError as e:
      return ('HTTPError', e.getcode(), str(e), e.read().strip())
    except Exception as e:
      return str(e)
    else:
      return True

  def Clients(self):
    if time.time() - self._state.last_list <= _LIST_CACHE_TIMEOUT:
      return self._state.listing

    self._state.listing = self._GetJSON('/api/agents')
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
      del self._state.forwards[str(local_port)]
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
    self._last_size = None
    self._old_sigwinch_handler = None

  def handshake_ok(self):
    pass

  def _handle_sigwinch(self, signum, frame):
    """Handle terminal resize events."""
    rows, cols = GetTerminalSize()
    if self._last_size != (rows, cols):
      self._last_size = (rows, cols)
      self.send(f"\x1b[8;{rows};{cols}t")

  def opened(self):
    def _FeedInput():
      self._old_termios = termios.tcgetattr(self._stdin_fd)
      tty.setraw(self._stdin_fd)

      # Send initial terminal size
      rows, cols = GetTerminalSize()
      self._last_size = (rows, cols)
      self.send(f"\x1b[8;{rows};{cols}t")

      READY, ENTER_PRESSED, ESCAPE_PRESSED = range(3)

      try:
        state = READY
        while True:
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

    # Set up SIGWINCH handler
    self._old_sigwinch_handler = signal.signal(signal.SIGWINCH, self._handle_sigwinch)

  def closed(self, code, reason=None):
    del code, reason  # Unused.
    # Restore original terminal settings
    termios.tcsetattr(self._stdin_fd, termios.TCSANOW, self._old_termios)
    # Restore original signal handler
    if self._old_sigwinch_handler:
      signal.signal(signal.SIGWINCH, self._old_sigwinch_handler)
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

        if sys.stdin in rd:
          data = sys.stdin.buffer.read()
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
      self._output.write(message.data)
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


class FileEntry:
  """Class to represent a file entry with its metadata."""

  def __init__(self, path, perm=0o644, is_symlink=False, link_target='', is_dir=False):
    self.path = path
    self.perm = perm
    self.is_symlink = is_symlink
    self.link_target = link_target
    self.is_dir = is_dir

  def __repr__(self):
    return f"{self.ftype} {self.path} {oct(self.perm)[2:]} {self.link_target}"

  @property
  def name(self):
    return os.path.basename(self.path.rstrip('/'))

  @property
  def ftype(self):
    if self.is_dir:
      return 'd'
    return 'l' if self.is_symlink else 'f'

@ParseMethodSubCommands
class OverlordCliClient:
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
    elif command == 'admin':
      self.Admin(args)

  def _SaveTLSCertificate(self, host, cert_pem):
    try:
      os.makedirs(_CERT_DIR)
    except Exception:
      pass
    with open(GetTLSCertPath(host), 'w') as f:
      f.write(cert_pem)

  def _HTTPPostFile(self, url, filename, progress=None):
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
    h.putheader(*JWTAuthHeader(self._state.jwt_token))

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
    if resp.status != 200:
      raise RuntimeError(f"Failed to upload file: {resp.read()}")

  def CheckDaemon(self):
    self._server = OverlordClientDaemon.GetRPCServer()
    if self._server is None:
      print('* daemon not running, starting it now on port %d ... *' %
            _OVERLORD_CLIENT_DAEMON_PORT)
      self.StartServer()

    self._state = DaemonState.FromDict(self._server.State())
    sha1sum = GetVersionDigest()

    if sha1sum != self._state.version_sha1sum and not _DEBUG_NO_KILL:
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
    except Exception as e:
      raise RuntimeError('remote server disconnected, abort') from e

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
    scheme = 'ws%s://' % ('s' if self._state.ssl else '')
    bio = BytesIO()
    ws = ShellWebSocketClient(
        self._state, bio,
        scheme + '%s:%d/api/agents/%s/shell?command=%s&token=%s' % (
            self._state.host, self._state.port,
            urllib.parse.quote(self._selected_mid),
            urllib.parse.quote(command),
            urllib.parse.quote(self._state.jwt_token)))
    ws.connect()
    ws.run()
    return bio.getvalue().decode('utf-8')

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

    for unused_i in range(3):  # pylint: disable=too-many-nested-blocks
      try:
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
              res = json.loads(str(body))
              print('connect: %s' % res['data'])
              if not username_provided or not password_provided:
                continue
              break
            logging.error('%s; %s', except_str, body)

        if ret is not True:
          print('\nFailed to connect to %s: %s' % (host, ret))
        else:
          print('\nConnection to %s:%d established.' % (host, args.port))
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

  def _SizeToHuman(self, size_in_bytes):
    """Convert size in bytes to human readable format.

    Args:
      size_in_bytes: Size in bytes.

    Returns:
      Human readable string representation of the size.
    """
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
    return '%6.1f %3s' % (value, unit)

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
      raise RuntimeError('select: no clients found')
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
      except ValueError as e:
        raise RuntimeError('select: invalid selection') from e
      except IndexError as e:
        raise RuntimeError('select: selection out of range') from e

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

    scheme = 'ws%s://' % ('s' if self._state.ssl else '')
    if command:
      cmd = ' '.join(command)
      ws = ShellWebSocketClient(
          self._state, sys.stdout.buffer,
          scheme + '%s:%d/api/agents/%s/shell?command=%s&token=%s' % (
              self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid),
              urllib.parse.quote(cmd),
              urllib.parse.quote(self._state.jwt_token)))
    else:
      ws = TerminalWebSocketClient(
          self._state, self._selected_mid, self._escape,
          scheme + '%s:%d/api/agents/%s/tty?token=%s' % (
              self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid),
              urllib.parse.quote(self._state.jwt_token)))
    try:
      ws.connect()
      ws.run()
    except socket.error as e:
      if e.errno == 32:  # Broken pipe
        pass
      else:
        raise

  def _LsTree(self, path):
    """Get a recursive directory listing using the Overlord API.

    This method uses the Overlord API to get a directory listing recursively,
    including all subdirectories and files, similar to os.walk.

    Args:
      path: The path to list.

    Returns:
      A list of FileEntry objects representing all files and directories.

    Raises:
      RuntimeError: If the API call fails.
    """
    url = ('/api/agents/%s/fs?op=lstree&path=%s' %
           (urllib.parse.quote(self._selected_mid),
            urllib.parse.quote(path)))
    try:
      response = UrlOpen(self._state, url)
      data = json.loads(response.read().decode('utf-8')).get('data', [])
    except urllib.error.HTTPError as e:
      error = json.loads(e.read()).get('data', 'unknown error')
      raise RuntimeError('lstree: %s' % error) from e

    entries = []
    for entry in data:
      try:
        perm = entry.get('perm', 0o644)
      except (ValueError, TypeError):
        perm = 0o644

      is_dir = entry.get('is_dir', False)
      file_path = entry.get('path', '')

      # Add trailing slash to directories for consistency
      if is_dir and not file_path.endswith('/'):
        file_path += '/'

      is_symlink = entry.get('is_symlink', False)
      link_target = entry.get('link_target', '')

      entries.append(FileEntry(
        path=file_path,
        perm=perm,
        is_symlink=is_symlink,
        link_target=link_target,
        is_dir=is_dir
      ))

    return entries

  def _Fstat(self, path):
    """Get file or directory status using the Overlord API.

    Args:
      path: The path to get status for.

    Returns:
      A dictionary containing file/directory status information.

    Raises:
      RuntimeError: If the API call fails.
    """
    url = ('/api/agents/%s/fs?op=fstat&path=%s' %
           (urllib.parse.quote(self._selected_mid),
            urllib.parse.quote(path)))
    try:
      response = UrlOpen(self._state, url)
      return json.loads(response.read().decode('utf-8'))['data']
    except urllib.error.HTTPError as e:
      error = json.loads(e.read()).get('data', 'unknown error')
      raise RuntimeError('fstat: %s' % error) from e

  def _Mkdir(self, path, perm=0o755):
    """Create a directory with specific permissions using the Overlord API.

    Args:
      path: The path to create.
      perm: The permissions to set (in octal).

    Raises:
      RuntimeError: If the API call fails.
    """
    url = ('/api/agents/%s/fs/directories?path=%s&perm=%d' %
           (urllib.parse.quote(self._selected_mid),
            urllib.parse.quote(path),
            perm))
    try:
      UrlOpen(self._state, url, method='POST')
    except urllib.error.HTTPError as e:
      error = json.loads(e.read()).get('data', 'unknown error')
      raise RuntimeError('mkdir: %s' % error) from e

  @AutoRetry('pull', _RETRY_TIMES)
  def _PullSingle(self, entry, dst):
    """Pull a single file or symlink.

    Args:
      entry: A FileEntry object representing the file to pull.
      dst: The destination path.
    """
    try:
      os.makedirs(os.path.dirname(dst))
    except Exception:
      pass

    if entry.is_symlink:
      pbar = ProgressBar(entry.name)
      if os.path.exists(dst):
        os.remove(dst)
      os.symlink(entry.link_target, dst)
      pbar.End()
      return

    url = ('/api/agents/%s/file?filename=%s' %
           (urllib.parse.quote(self._selected_mid),
            urllib.parse.quote(entry.path)))
    try:
      h = UrlOpen(self._state, url)
    except urllib.error.HTTPError as e:
      msg = json.loads(e.read()).get('data', 'unknown error')
      raise RuntimeError('pull: %s' % msg) from e
    except KeyboardInterrupt:
      return

    pbar = ProgressBar(entry.name)
    with open(dst, 'wb') as f:
      os.fchmod(f.fileno(), entry.perm)
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

      os.fchmod(f.fileno(), entry.perm)

    pbar.End()

  @Command('pull', 'pull a file or directory from remote', [
      Arg('src', metavar='SOURCE'),
      Arg('dst', metavar='DESTINATION', default='.', nargs='?')])
  def Pull(self, args):
    self.CheckClient()

    # Get directory listing using the API
    entries = self._LsTree(args.src)

    if args.dst == '.':
      args.dst = os.path.basename(args.src)

    args.dst = os.path.abspath(args.dst)

    # Since the entries are in absolute path. We need to find the common prefix
    # with args.src
    common_prefix = os.path.commonpath([entry.path for entry in entries])

    if not os.path.isabs(common_prefix):
      raise RuntimeError('common_prefix is not absolute: %s' % common_prefix)

    while os.path.basename(common_prefix) != os.path.basename(args.src):
      common_prefix = os.path.dirname(common_prefix)
      if common_prefix == '/':
        break

    pull_spec = []
    for entry in entries:
      # If dst path exists, the dst path should be dst(dir) + rest of the path
      # w/o common prefix
      if os.path.exists(args.dst):
        rel_path = os.path.relpath(entry.path, common_prefix)
        dst = os.path.join(args.dst, os.path.basename(common_prefix))
        if rel_path != '.':
          dst = os.path.join(dst, rel_path)
      else:
        # Otherwise, the dst path should be dst + rest of the path w/ common
        # prefix
        rel_path = os.path.relpath(entry.path, common_prefix)
        if rel_path == '.':
          dst = args.dst
        else:
          dst = os.path.join(args.dst, rel_path)

      pull_spec.append((entry, dst))

    for spec in pull_spec:
      entry, dst = spec
      if entry.is_dir:
        os.makedirs(dst, exist_ok=True)
        os.chmod(dst, entry.perm)
      else:
        self._PullSingle(entry, dst)

  @AutoRetry('push', _RETRY_TIMES)
  def _PushSingle(self, src, dst):
    src_base = os.path.basename(src)

    if os.path.islink(src):
      pbar = ProgressBar(src_base)
      link_path = os.readlink(src)

      url = ('/api/agents/%s/fs/symlinks?target=%s&dest=%s' %
             (urllib.parse.quote(self._selected_mid),
              urllib.parse.quote(link_path),
              urllib.parse.quote(dst)))

      try:
        UrlOpen(self._state, url, method='POST')
      except urllib.error.HTTPError as e:
        msg = json.loads(e.read()).get('data', 'unknown error')
        raise RuntimeError(f'push: {msg}')

      pbar.End()
      return

    mode = '0%o' % (0x1FF & os.stat(src).st_mode)
    url = ('/api/agents/%s/file?dest=%s&perm=%s' %
           (urllib.parse.quote(self._selected_mid),
            urllib.parse.quote(dst),
            mode))

    pbar = ProgressBar(src_base)
    try:
      self._HTTPPostFile(url, src, pbar.SetProgress)
    except Exception as e:
      raise RuntimeError(f'push: {str(e)}')
    pbar.End()

  @Command('push', 'push a file or directory to remote', [
      Arg('srcs', nargs='+', metavar='SOURCE'),
      Arg('dst', metavar='DESTINATION')])
  def Push(self, args):
    self.CheckClient()

    def _Push(src, dst):
      if os.path.isdir(src):
        src = os.path.abspath(src)
        base_dir_name = os.path.basename(src)

        # Use the API to check if the destination exists and is a directory
        try:
          stat_info = self._Fstat(dst)
          dst_exists = stat_info.get('exists', False)
          dst_is_dir = stat_info.get('is_dir', False) if dst_exists else False
        except RuntimeError as e:
          # Some error occurred
          raise

        if dst_exists and not dst_is_dir:
          raise RuntimeError('push: %s: Not a directory' % dst)

        # Get all files in the source directory tree
        entries = self._LocalLsTree(src)

        # Process each file entry
        for entry in entries:
          rel_path = os.path.relpath(entry.path, src)

          if dst_exists:
            dest_path = os.path.join(dst, base_dir_name, rel_path)
          else:
            dest_path = os.path.join(dst, rel_path)

          if entry.is_dir:
            self._Mkdir(dest_path, entry.perm)
            continue

          self._PushSingle(entry.path, dest_path)
      else:
        self._PushSingle(src, dst)

    if len(args.srcs) > 1:
      # Use the API to check if the destination is a directory
      try:
        stat_info = self._Fstat(args.dst)
        dst_exists = stat_info.get('exists', False)
        dst_is_dir = stat_info.get('is_dir', False)

        if not dst_exists or not dst_is_dir:
          raise RuntimeError('push: %s: Not a directory' % args.dst)
      except RuntimeError as e:
        # If we can't stat it, it's not a directory or doesn't exist
        raise RuntimeError('push: %s: No such file or directory' % args.dst)

    for src in args.srcs:
      if not os.path.exists(src):
        raise RuntimeError('push: can not stat "%s": no such file or directory'
                           % src)
      if not os.access(src, os.R_OK):
        raise RuntimeError('push: can not open "%s" for reading' % src)

      _Push(src, args.dst)

  @Command('forward', 'forward remote port to local port', [
      Arg('--list', dest='list_all', action='store_true', default=False,
          help='list all port forwarding sessions'),
      Arg('--remove', metavar='LOCAL_PORT', dest='remove', type=int,
          default=None,
          help='remove port forwarding for local port LOCAL_PORT'),
      Arg('--remove-all', dest='remove_all', action='store_true',
          default=False, help='remove all port forwarding'),
      Arg('remote', metavar='[HOST:]REMOTE_PORT', type=str, nargs='?'),
      Arg('local_port', metavar='LOCAL_PORT', type=int, nargs='?')])
  def Forward(self, args):
    if args.list_all:
      max_len = 10
      if self._state.forwards:
        max_len = max([len(v[0]) for v in self._state.forwards.values()])

      print('%-*s   %-23s  %-8s' % (max_len, 'Client', 'Remote', 'Local'))
      for local in sorted(self._state.forwards.keys()):
        value = self._state.forwards[local]
        print('%-*s   %-23s  %-8s' % (max_len, value[0], value[1], local))
      return

    if args.remove_all:
      self._server.RemoveAllForward()
      return

    if args.remove:
      self._server.RemoveForward(args.remove)
      return

    self.CheckClient()

    if args.remote is None:
      raise RuntimeError('remote target not specified')

    remote_parts = args.remote.split(':')
    if len(remote_parts) == 1:
      try:
        remote_host = '127.0.0.1'
        remote_port = int(remote_parts[0])
      except ValueError:
        raise RuntimeError('invalid remote port')
    elif len(remote_parts) == 2:
      remote_host = remote_parts[0]
      remote_port = int(remote_parts[1])
    else:
      raise RuntimeError('invalid remote target')

    if args.local_port is None:
      args.local_port = remote_port

    remote = remote_port

    def HandleConnection(conn):
      scheme = 'ws%s://' % ('s' if self._state.ssl else '')
      ws = ForwarderWebSocketClient(
          self._state, conn,
          scheme + '%s:%d/api/agents/%s/forward?host=%s&port=%d&token=%s' % (
              self._state.host, self._state.port,
              urllib.parse.quote(self._selected_mid),
              remote_host, remote_port,
              urllib.parse.quote(self._state.jwt_token)))
      try:
        ws.connect()
        ws.run()
      except Exception as e:
        print('error: %s' % e)
      finally:
        ws.close()

    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(('0.0.0.0', args.local_port))
    server.listen(5)

    pid = os.fork()
    if pid == 0:
      while True:
        conn, unused_addr = server.accept()
        t = threading.Thread(target=HandleConnection, args=(conn,))
        t.daemon = True
        t.start()
    else:
      self._server.AddForward(self._selected_mid, args.remote, args.local_port,
                              pid)

  def _LocalLsTree(self, path):
    """Get a recursive directory listing of local files.

    This method recursively traverses a local directory and returns FileEntry objects
    for all files and directories found, similar to os.walk but with a unified return format.

    Args:
      path: The local path to list.

    Returns:
      A list of FileEntry objects representing all files and directories.

    Raises:
      RuntimeError: If the path doesn't exist or can't be accessed.
    """
    if not os.path.exists(path):
      raise RuntimeError('local lstree: %s: No such file or directory' % path)

    if not os.access(path, os.R_OK):
      raise RuntimeError('local lstree: %s: Permission denied' % path)

    results = []

    # If it's a file, return a single FileEntry
    if os.path.isfile(path):
      stat_info = os.stat(path)
      perm = stat_info.st_mode & 0o777
      is_symlink = os.path.islink(path)
      link_target = os.readlink(path) if is_symlink else ''

      return [FileEntry(
          path=path,
          perm=perm,
          is_symlink=is_symlink,
          link_target=link_target,
          is_dir=False
      )]

    # For directories, walk the tree
    for root, dirs, files in os.walk(path):
      # Add directory entries
      for dir_name in dirs:
        dir_path = os.path.join(root, dir_name)
        stat_info = os.stat(dir_path)
        perm = stat_info.st_mode & 0o777
        is_symlink = os.path.islink(dir_path)
        link_target = os.readlink(dir_path) if is_symlink else ''

        results.append(FileEntry(
            path=dir_path,
            perm=perm,
            is_symlink=is_symlink,
            link_target=link_target,
            is_dir=False if is_symlink else True
        ))

      # Add file entries
      for file_name in files:
        file_path = os.path.join(root, file_name)
        stat_info = os.stat(file_path)
        perm = stat_info.st_mode & 0o777
        is_symlink = os.path.islink(file_path)
        link_target = os.readlink(file_path) if is_symlink else ''

        results.append(FileEntry(
            path=file_path,
            perm=perm,
            is_symlink=is_symlink,
            link_target=link_target,
            is_dir=False
        ))

    return results

  @Command('admin', 'manage users and groups', [
      Arg('action', metavar='ACTION',
          choices=['list-users', 'add-user', 'del-user', 'change-password',
                   'list-groups', 'add-group', 'del-group', 'add-user-to-group',
                   'del-user-from-group', 'list-group-users'],
          help='admin action to perform: list-users, add-user, del-user, '
               'change-password, list-groups, add-group, del-group, '
               'add-user-to-group, del-user-from-group, list-group-users'),
      Arg('args', metavar='ARGS', nargs='*', help='arguments for the action')])
  def Admin(self, args):
    """Manage users and groups using the Overlord API."""
    self.CheckConnection()

    action = args.action
    action_args = args.args

    # User management
    if action == 'list-users':
      self._AdminListUsers()
    elif action == 'add-user':
      if len(action_args) < 2:
        raise RuntimeError('Usage: admin add-user USERNAME PASSWORD [is_admin]')
      username = action_args[0]
      password = action_args[1]
      is_admin = False
      if (len(action_args) >= 3 and
          action_args[2].lower() in ('true', 'yes', '1', 'y')):
        is_admin = True
      self._AdminAddUser(username, password, is_admin)
    elif action == 'del-user':
      if len(action_args) < 1:
        raise RuntimeError('Usage: admin del-user USERNAME')
      self._AdminDelUser(action_args[0])
    elif action == 'change-password':
      if len(action_args) < 2:
        raise RuntimeError('Usage: admin change-password USERNAME NEW_PASSWORD')
      self._AdminChangePassword(action_args[0], action_args[1])

    # Group management
    elif action == 'list-groups':
      self._AdminListGroups()
    elif action == 'add-group':
      if len(action_args) < 1:
        raise RuntimeError('Usage: admin add-group GROUP_NAME')
      self._AdminAddGroup(action_args[0])
    elif action == 'del-group':
      if len(action_args) < 1:
        raise RuntimeError('Usage: admin del-group GROUP_NAME')
      self._AdminDelGroup(action_args[0])

    # User-group management
    elif action == 'add-user-to-group':
      if len(action_args) < 2:
        raise RuntimeError('Usage: admin add-user-to-group USERNAME GROUP_NAME')
      self._AdminAddUserToGroup(action_args[0], action_args[1])
    elif action == 'del-user-from-group':
      if len(action_args) < 2:
        raise RuntimeError('Usage: admin del-user-from-group USERNAME GROUP_NAME')
      self._AdminDelUserFromGroup(action_args[0], action_args[1])
    elif action == 'list-group-users':
      if len(action_args) < 1:
        raise RuntimeError('Usage: admin list-group-users GROUP_NAME')
      self._AdminListGroupUsers(action_args[0])
    else:
      raise RuntimeError(f'Unknown admin action: {action}')

  def _AdminApiCall(self, url, method='GET', data=None):
    """Make an API call to the admin endpoints."""
    try:
      if method == 'GET':
        response = UrlOpen(self._state, url)
        return json.loads(response.read().decode('utf-8')).get('data', [])
      else:
        response = UrlOpen(self._state, url, data=data, method=method)
        return json.loads(response.read().decode('utf-8')).get('data', {})
    except urllib.error.HTTPError as e:
      try:
        error_data = json.loads(e.read().decode('utf-8'))
        error_msg = error_data.get('data', str(e))
      except:
        error_msg = str(e)
      raise RuntimeError(error_msg)

  def _AdminListUsers(self):
    """List all users."""
    users = self._AdminApiCall('/api/users')

    # Format output as a table
    print(f"{'USERNAME':<20} {'ADMIN':<10} {'GROUPS'}")
    print("-" * 50)

    for user in users:
      username = user.get('username', '')
      is_admin = 'Yes' if user.get('is_admin', False) else 'No'
      groups = ', '.join(user.get('groups', []))
      print(f"{username:<20} {is_admin:<10} {groups}")

  def _AdminAddUser(self, username, password, is_admin=False):
    """Add a new user."""
    data = {
      'username': username,
      'password': password,
      'is_admin': is_admin
    }
    self._AdminApiCall('/api/users', method='POST', data=data)
    print(f"User '{username}' created successfully")

    if is_admin:
      # Add user to admin group if specified
      self._AdminAddUserToGroup(username, 'admin')

  def _AdminDelUser(self, username):
    """Delete a user."""
    self._AdminApiCall(f'/api/users/{username}', method='DELETE')
    print(f"User '{username}' deleted successfully")

  def _AdminChangePassword(self, username, new_password):
    """Change a user's password."""
    data = {'password': new_password}
    self._AdminApiCall(f'/api/users/{username}/password', method='PUT',
                       data=data)
    print(f"Password for user '{username}' updated successfully")

  def _AdminListGroups(self):
    """List all groups."""
    groups = self._AdminApiCall('/api/groups')

    # Format output as a table
    print(f"{'GROUP NAME':<20} {'USER COUNT'}")
    print("-" * 30)

    for group in groups:
      name = group.get('name', '')
      user_count = group.get('user_count', 0)
      print(f"{name:<20} {user_count}")

  def _AdminAddGroup(self, group_name):
    """Add a new group."""
    data = {'name': group_name}
    self._AdminApiCall('/api/groups', method='POST', data=data)
    print(f"Group '{group_name}' created successfully")

  def _AdminDelGroup(self, group_name):
    """Delete a group."""
    self._AdminApiCall(f'/api/groups/{group_name}', method='DELETE')
    print(f"Group '{group_name}' deleted successfully")

  def _AdminAddUserToGroup(self, username, group_name):
    """Add a user to a group."""
    data = {'username': username}
    self._AdminApiCall(f'/api/groups/{group_name}/users', method='POST',
                       data=data)
    print(f"User '{username}' added to group '{group_name}' successfully")

  def _AdminDelUserFromGroup(self, username, group_name):
    """Remove a user from a group."""
    self._AdminApiCall(f'/api/groups/{group_name}/users/{username}',
                       method='DELETE')
    print(f"User '{username}' removed from group '{group_name}' successfully")

  def _AdminListGroupUsers(self, group_name):
    """List all users in a group."""
    users = self._AdminApiCall(f'/api/groups/{group_name}/users')

    if not users:
      print(f"No users in group '{group_name}'")
      return

    print(f"Users in group '{group_name}':")
    for username in users:
      print(f"  - {username}")


def main():
  # Setup logging format
  logger = logging.getLogger()
  logger.setLevel(logging.INFO)
  handler = logging.StreamHandler()
  formatter = logging.Formatter('%(asctime)s %(message)s', '%Y/%m/%d %H:%M:%S')
  handler.setFormatter(formatter)
  logger.addHandler(handler)

  ovl = OverlordCliClient()
  try:
    ovl.Main()
  except KeyboardInterrupt:
    print('Ctrl-C received, abort')
  except Exception as e:
    if _DEBUG:
      logging.exception(e)
    print(f'error: {str(e)}')


if __name__ == '__main__':
  main()
