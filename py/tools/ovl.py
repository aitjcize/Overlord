#!/usr/bin/env python
# -*- coding: utf-8 -*-
#
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from __future__ import print_function

import argparse
import ast
import base64
import fcntl
import getpass
import hashlib
import httplib
import json
import jsonrpclib
import logging
import os
import re
import select
import signal
import socket
import ssl
import StringIO
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time
import tty
import urllib2
import urlparse
import unicodedata  # required by pyinstaller, pylint: disable=W0611

from jsonrpclib.SimpleJSONRPCServer import SimpleJSONRPCServer
from jsonrpclib.config import Config
from ws4py.client import WebSocketBaseClient


_CERT_DIR = os.path.expanduser('~/.config/ovl')

_ESCAPE = '~'
_BUFSIZ = 8192
_OVERLORD_PORT = 4455
_OVERLORD_HTTP_PORT = 9000
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

Do you want to trust this certificate and proceed? [Y/n] """

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


def GetVersionDigest():
  """Return the sha1sum of the current executing script."""
  # Check python script by default
  filename = __file__

  # If we are running from a frozen binary, we should calculate the checksum
  # against that binary instead of the python script.
  # See: https://pyinstaller.readthedocs.io/en/stable/runtime-information.html
  if getattr(sys, 'frozen', False):
    filename = sys.executable

  with open(filename, 'r') as f:
    return hashlib.sha1(f.read()).hexdigest()


def GetTLSCertPath(host):
  return os.path.join(_CERT_DIR, '%s.cert' % host)


def UrlOpen(state, url):
  """Wrapper for urllib2.urlopen.

  It selects correct HTTP scheme according to self._state.ssl, add HTTP
  basic auth headers, and add specify correct SSL context.
  """
  url = MakeRequestUrl(state, url)
  request = urllib2.Request(url)
  if state.username is not None and state.password is not None:
    request.add_header(*BasicAuthHeader(state.username, state.password))
  return urllib2.urlopen(request, timeout=_DEFAULT_HTTP_TIMEOUT,
                         context=state.ssl_context)


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
  credential = base64.b64encode('%s:%s' % (user, password))
  return ('Authorization', 'Basic %s' % credential)


def GetTerminalSize():
  """Retrieve terminal window size."""
  ws = struct.pack('HHHH', 0, 0, 0, 0)
  ws = fcntl.ioctl(0, termios.TIOCGWINSZ, ws)
  lines, columns, unused_x, unused_y = struct.unpack('HHHH', ws)
  return lines, columns


def MakeRequestUrl(state, url):
  return 'http%s://%s' % ('s' if state.ssl else '', url)


class ProgressBar(object):
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
      value = size_in_bytes / 1024.0
    elif size_in_bytes < 1024 ** 3:
      unit = 'MiB'
      value = size_in_bytes / (1024.0 ** 2)
    elif size_in_bytes < 1024 ** 4:
      unit = 'GiB'
      value = size_in_bytes / (1024.0 ** 3)
    return ' %6.1f %3s' % (value, unit)

  def _SpeedToHuman(self, speed_in_bs):
    if speed_in_bs < 1024:
      unit = 'B'
      value = speed_in_bs
    elif speed_in_bs < 1024 ** 2:
      unit = 'K'
      value = speed_in_bs / 1024.0
    elif speed_in_bs < 1024 ** 3:
      unit = 'M'
      value = speed_in_bs / (1024.0 ** 2)
    elif speed_in_bs < 1024 ** 4:
      unit = 'G'
      value = speed_in_bs / (1024.0 ** 3)
    return ' %6.1f%s/s' % (value, unit)

  def _DurationToClock(self, duration):
    return ' %02d:%02d' % (duration / 60, duration % 60)

  def SetProgress(self, percentage, size=None):
    current_width = GetTerminalSize()[1]
    if self._width != current_width:
      self._CalculateSize()

    if size is not None:
      self._size = size

    elapse_time = time.time() - self._start_time
    speed = self._size / float(elapse_time)

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


class DaemonState(object):
  """DaemonState is used for storing Overlord state info."""
  def __init__(self):
    self.version_sha1sum = GetVersionDigest()
    self.host = None
    self.port = None
    self.ssl = False
    self.ssl_self_signed = False
    self.ssl_context = ssl.SSLContext(ssl.PROTOCOL_TLSv1_2)
    self.ssh = False
    self.orig_host = None
    self.ssh_pid = None
    self.username = None
    self.password = None
    self.selected_mid = None
    self.forwards = {}
    self.listing = []
    self.last_list = 0


class OverlordClientDaemon(object):
  """Overlord Client Daemon."""
  def __init__(self):
    self._state = DaemonState()
    self._server = None

  def Start(self):
    self.StartRPCServer()

  def StartRPCServer(self):
    self._server = SimpleJSONRPCServer(_OVERLORD_CLIENT_DAEMON_RPC_ADDR,
                                       logRequests=False)
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
      self._server.serve_forever()

  @staticmethod
  def GetRPCServer():
    """Returns the Overlord client daemon RPC server."""
    server = jsonrpclib.Server('http://%s:%d' %
                               _OVERLORD_CLIENT_DAEMON_RPC_ADDR)
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
      context = ssl.SSLContext(ssl.PROTOCOL_TLSv1_2)
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

      # Save SSLContext for future use.
      self._state.ssl_context = context
      return True

    # First try connect with built-in certificates
    tls_context = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    if _DoConnect(tls_context):
      return True

    # Try with already saved certificate, if any.
    tls_context = ssl.SSLContext(ssl.PROTOCOL_TLSv1_2)
    tls_context.verify_mode = ssl.CERT_REQUIRED
    tls_context.check_hostname = True

    tls_cert_path = GetTLSCertPath(self._state.host)
    if os.path.exists(tls_cert_path):
      tls_context.load_verify_locations(tls_cert_path)
      self._state.ssl_self_signed = True

    return _DoConnect(tls_context)

  def Connect(self, host, port=_OVERLORD_HTTP_PORT, ssh_pid=None,
              username=None, password=None, orig_host=None):
    self._state.username = username
    self._state.password = password
    self._state.host = host
    self._state.port = port
    self._state.ssl = False
    self._state.ssl_self_signed = False
    self._state.orig_host = orig_host
    self._state.ssh_pid = ssh_pid
    self._state.selected_mid = None

    tls_enabled = self._TLSEnabled()
    if tls_enabled:
      result = self._CheckTLSCertificate()
      if not result:
        if self._state.ssl_self_signed:
          return ('SSLCertificateChanged', ssl.get_server_certificate(
              (self._state.host, self._state.port)))
        else:
          return ('SSLVerifyFailed', ssl.get_server_certificate(
              (self._state.host, self._state.port)))

    try:
      self._state.ssl = tls_enabled
      UrlOpen(self._state, '%s:%d' % (host, port))
    except urllib2.HTTPError as e:
      return ('HTTPError', e.getcode(), str(e), e.read().strip())
    except Exception as e:
      return str(e)
    else:
      return True

  def Clients(self):
    if time.time() - self._state.last_list <= _LIST_CACHE_TIMEOUT:
      return self._state.listing

    mids = [client['mid'] for client in self._GetJSON('/api/agents/list')]
    self._state.listing = sorted(list(set(mids)))
    self._state.last_list = time.time()
    return self._state.listing

  def SelectClient(self, mid):
    self._state.selected_mid = mid

  def AddForward(self, mid, remote, local, pid):
    self._state.forwards[local] = (mid, remote, pid)

  def RemoveForward(self, local_port):
    try:
      unused_mid, unused_remote, pid = self._state.forwards[local_port]
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
    cafile = ssl.get_default_verify_paths().openssl_cafile
    # For some system / distribution, python can not detect system cafile path.
    # In such case we fallback to the default path.
    if not os.path.exists(cafile):
      cafile = '/etc/ssl/certs/ca-certificates.crt'

    if state.ssl_self_signed:
      cafile = GetTLSCertPath(state.host)

    ssl_options = {
        'cert_reqs': ssl.CERT_REQUIRED,
        'ca_certs': cafile
    }
    # ws4py does not allow you to specify SSLContext, but rather passing in the
    # argument of ssl.wrap_socket
    super(SSLEnabledWebSocketBaseClient, self).__init__(
        ssl_options=ssl_options, *args, **kwargs)


class TerminalWebSocketClient(SSLEnabledWebSocketBaseClient):
  def __init__(self, state, mid, escape, *args, **kwargs):
    super(TerminalWebSocketClient, self).__init__(state, *args, **kwargs)
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
        payload = chr(_CONTROL_START) + json.dumps(control) + chr(_CONTROL_END)
        nonlocals['size'] = size
        try:
          self.send(payload, binary=True)
        except Exception:
          pass

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
    termios.tcsetattr(self._stdin_fd, termios.TCSANOW, self._old_termios)
    print('Connection to %s closed.' % self._mid)

  def received_message(self, msg):
    if msg.is_binary:
      sys.stdout.write(msg.data)
      sys.stdout.flush()


class ShellWebSocketClient(SSLEnabledWebSocketBaseClient):
  def __init__(self, state, output, *args, **kwargs):
    """Constructor.

    Args:
      output: output file object.
    """
    self.output = output
    super(ShellWebSocketClient, self).__init__(state, *args, **kwargs)

  def handshake_ok(self):
    pass

  def opened(self):
    def _FeedInput():
      try:
        while True:
          data = sys.stdin.read(1)

          if len(data) == 0:
            self.send(_STDIN_CLOSED * 2)
            break
          self.send(data, binary=True)
      except (KeyboardInterrupt, RuntimeError):
        pass

    t = threading.Thread(target=_FeedInput)
    t.daemon = True
    t.start()

  def closed(self, code, reason=None):
    pass

  def received_message(self, msg):
    if msg.is_binary:
      self.output.write(msg.data)
      self.output.flush()


class ForwarderWebSocketClient(SSLEnabledWebSocketBaseClient):
  def __init__(self, state, sock, *args, **kwargs):
    super(ForwarderWebSocketClient, self).__init__(state, *args, **kwargs)
    self._sock = sock
    self._stop = threading.Event()

  def handshake_ok(self):
    pass

  def opened(self):
    def _FeedInput():
      try:
        self._sock.setblocking(False)
        while True:
          rd, unused_w, unused_x = select.select([self._sock], [], [], 0.5)
          if self._stop.is_set():
            break
          if self._sock in rd:
            data = self._sock.recv(_BUFSIZ)
            if len(data) == 0:
              self.close()
              break
            self.send(data, binary=True)
      except Exception:
        pass
      finally:
        self._sock.close()

    t = threading.Thread(target=_FeedInput)
    t.daemon = True
    t.start()

  def closed(self, code, reason=None):
    self._stop.set()
    sys.exit(0)

  def received_message(self, msg):
    if msg.is_binary:
      self._sock.send(msg.data)


def Arg(*args, **kwargs):
  return (args, kwargs)


def Command(command, help_msg=None, args=None):
  """Decorator for adding argparse parameter for a method."""
  if args is None:
    args = []
  def WrapFunc(func):
    def Wrapped(*args, **kwargs):
      return func(*args, **kwargs)
    # pylint: disable=W0212
    Wrapped.__arg_attr = {'command': command, 'help': help_msg, 'args': args}
    return Wrapped
  return WrapFunc


def ParseMethodSubCommands(cls):
  """Decorator for a class using the @Command decorator.

  This decorator retrieve command info from each method and append it in to the
  SUBCOMMANDS class variable, which is later used to construct parser.
  """
  for unused_key, method in cls.__dict__.iteritems():
    if hasattr(method, '__arg_attr'):
      cls.SUBCOMMANDS.append(method.__arg_attr)  # pylint: disable=W0212
  return cls


@ParseMethodSubCommands
class OverlordCLIClient(object):
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
    subparsers = root_parser.add_subparsers(help='sub-command')

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
    elif command == 'kill-server':
      self.KillServer()
      return

    self.CheckDaemon()
    if command == 'status':
      self.Status()
      return
    elif command == 'connect':
      self.Connect(args)
      return

    # The following command requires connection to the server
    self.CheckConnection()

    if args.select_mid_before_action:
      self.SelectClient(store=False)

    if command == 'select':
      self.SelectClient(args)
    elif command == 'ls':
      self.ListClients()
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
    parse = urlparse.urlparse(url)

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
      h = httplib.HTTP(parse.netloc)
    else:
      h = httplib.HTTPS(parse.netloc, context=self._state.ssl_context)

    post_path = url[url.index(parse.netloc) + len(parse.netloc):]
    h.putrequest('POST', post_path)
    h.putheader('Content-Length', content_length)
    h.putheader('Content-Type', 'multipart/form-data; boundary=%s' % boundary)

    if user and passwd:
      h.putheader(*BasicAuthHeader(user, passwd))
    h.endheaders()
    h.send(part_header)

    count = 0
    with open(filename, 'r') as f:
      while True:
        data = f.read(_BUFSIZ)
        if not data:
          break
        count += len(data)
        if progress:
          progress(int(count * 100.0 / size), count)
        h.send(data)

    h.send(end_part)
    progress(100)

    if count != size:
      logging.warning('file changed during upload, upload may be truncated.')

    errcode, unused_x, unused_y = h.getreply()
    return errcode == 200

  def CheckDaemon(self):
    self._server = OverlordClientDaemon.GetRPCServer()
    if self._server is None:
      print('* daemon not running, starting it now on port %d ... *' %
            _OVERLORD_CLIENT_DAEMON_PORT)
      self.StartServer()

    self._state = self._server.State()
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
    case we can SSH forward the port to localhost.
    """

    control_file = self.GetSSHControlFile(host)
    try:
      os.unlink(control_file)
    except Exception:
      pass

    subprocess.Popen([
        'ssh', '-Nf',
        '-M',  # Enable master mode
        '-S', control_file,
        '-L', '9000:localhost:9000',
        '-p', str(port),
        '%s%s' % (user + '@' if user else '', host)
    ]).wait()

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
      raise RuntimeError('remote server disconnected, abort')

    if self._state.ssh_pid is not None:
      ret = subprocess.Popen(['kill', '-0', str(self._state.ssh_pid)],
                             stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE).wait()
      if ret != 0:
        raise RuntimeError('ssh tunnel disconnected, please re-connect')

  def CheckClient(self):
    if self._selected_mid is None:
      if self._state.selected_mid is None:
        raise RuntimeError('No client is selected')
      self._selected_mid = self._state.selected_mid

    if self._selected_mid not in self._server.Clients():
      raise RuntimeError('client %s disappeared' % self._selected_mid)

  def CheckOutput(self, command):
    headers = []
    if self._state.username is not None and self._state.password is not None:
      headers.append(BasicAuthHeader(self._state.username,
                                     self._state.password))

    scheme = 'ws%s://' % ('s' if self._state.ssl else '')
    sio = StringIO.StringIO()
    ws = ShellWebSocketClient(self._state, sio,
                              scheme + '%s:%d/api/agent/shell/%s?command=%s' %
                              (self._state.host, self._state.port,
                               self._selected_mid, urllib2.quote(command)),
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
      Arg('host', metavar='HOST', type=str, default='localhost',
          help='Overlord hostname/IP'),
      Arg('port', metavar='PORT', type=int,
          default=_OVERLORD_HTTP_PORT, help='Overlord port'),
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
          help='Overlord HTTP auth password')])
  def Connect(self, args):
    ssh_pid = None
    host = args.host
    orig_host = args.host

    if args.ssh_forward:
      # Kill previous SSH tunnel
      self.KillSSHTunnel()

      ssh_pid = self.SSHTunnel(args.ssh_login, args.host, args.ssh_port)
      host = 'localhost'

    username_provided = args.user is not None
    password_provided = args.passwd is not None
    prompt = False

    for unused_i in range(3):
      try:
        if prompt:
          if not username_provided:
            args.user = raw_input('Username: ')
          if not password_provided:
            args.passwd = getpass.getpass('Password: ')

        ret = self._server.Connect(host, args.port, ssh_pid, args.user,
                                   args.passwd, orig_host)
        if isinstance(ret, list):
          if ret[0].startswith('SSL'):
            cert_pem = ret[1]
            fp = GetTLSCertificateSHA1Fingerprint(cert_pem)
            fp_text = ':'.join([fp[i:i+2] for i in range(0, len(fp), 2)])

          if ret[0] == 'SSLCertificateChanged':
            print(_TLS_CERT_CHANGED_WARNING % (fp_text, GetTLSCertPath(host)))
            return
          elif ret[0] == 'SSLVerifyFailed':
            print(_TLS_CERT_FAILED_WARNING % (fp_text), end='')
            response = raw_input()
            if response.lower() in ['y', 'ye', 'yes']:
              self._SaveTLSCertificate(host, cert_pem)
              print('TLS host Certificate trusted, you will not be prompted '
                    'next time.\n')
              continue
            else:
              print('connection aborted.')
              return
          elif ret[0] == 'HTTPError':
            code, except_str, body = ret[1:]
            if code == 401:
              print('connect: %s' % body)
              prompt = True
              if not username_provided or not password_provided:
                continue
              else:
                break
            else:
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

    self._state = self._server.State()

    # Kill SSH Tunnel
    self.KillSSHTunnel()

    # Kill server daemon
    KillGraceful(self._server.GetPid())

  def KillSSHTunnel(self):
    if self._state.ssh_pid is not None:
      KillGraceful(self._state.ssh_pid)

  @Command('ls', 'list all clients')
  def ListClients(self):
    for client in self._server.Clients():
      print(client)

  @Command('select', 'select default client', [
      Arg('mid', metavar='mid', nargs='?', default=None)])
  def SelectClient(self, args=None, store=True):
    clients = self._server.Clients()

    mid = args.mid if args is not None else None
    if mid is None:
      print('Select from the following clients:')
      for i, client in enumerate(clients):
        print('    %d. %s' % (i + 1, client))

      print('\nSelection: ', end='')
      try:
        choice = int(raw_input()) - 1
        mid = clients[choice]
      except ValueError:
        raise RuntimeError('select: invalid selection')
      except IndexError:
        raise RuntimeError('select: selection out of range')
    else:
      if mid not in clients:
        raise RuntimeError('select: client %s does not exist' % mid)

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
    if len(command) == 0:
      ws = TerminalWebSocketClient(self._state, self._selected_mid,
                                   self._escape,
                                   scheme + '%s:%d/api/agent/tty/%s' %
                                   (self._state.host, self._state.port,
                                    self._selected_mid), headers=headers)
    else:
      cmd = ' '.join(command)
      ws = ShellWebSocketClient(self._state, sys.stdout,
                                scheme + '%s:%d/api/agent/shell/%s?command=%s' %
                                (self._state.host, self._state.port,
                                 self._selected_mid, urllib2.quote(cmd)),
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
             (self._state.host, self._state.port, self._selected_mid, dst,
              mode))
      try:
        UrlOpen(self._state, url + '&filename=%s' % src_base)
      except urllib2.HTTPError as e:
        msg = json.loads(e.read()).get('error', None)
        raise RuntimeError('push: %s' % msg)

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
    def _pull(src, dst, ftype, perm=0644, link=None):
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
             (self._state.host, self._state.port, self._selected_mid,
              urllib2.quote(src)))
      try:
        h = UrlOpen(self._state, url)
      except urllib2.HTTPError as e:
        msg = json.loads(e.read()).get('error', 'unkown error')
        raise RuntimeError('pull: %s' % msg)
      except KeyboardInterrupt:
        return

      pbar = ProgressBar(src_base)
      with open(dst, 'w') as f:
        os.fchmod(f.fileno(), perm)
        total_size = int(h.headers.get('Content-Length'))
        downloaded_size = 0

        while True:
          data = h.read(_BUFSIZ)
          if len(data) == 0:
            break
          downloaded_size += len(data)
          pbar.SetProgress(float(downloaded_size) * 100 / total_size,
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
      if len(self._state.forwards):
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
          scheme + '%s:%d/api/agent/forward/%s?port=%d' %
          (self._state.host, self._state.port, self._selected_mid, remote),
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

  # Add DaemonState to JSONRPC lib classes
  Config.instance().classes.add(DaemonState)

  ovl = OverlordCLIClient()
  try:
    ovl.Main()
  except KeyboardInterrupt:
    print('Ctrl-C received, abort')
  except Exception as e:
    print('error: %s' % e)


if __name__ == '__main__':
  main()
