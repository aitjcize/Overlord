#!/usr/bin/python -u
# -*- coding: utf-8 -*-
#
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import argparse
import fcntl
import json
import logging
import os
import Queue
import re
import select
import socket
import subprocess
import sys
import threading
import time
import uuid

import jsonrpclib
from jsonrpclib.SimpleJSONRPCServer import SimpleJSONRPCServer


_GHOST_RPC_PORT = 4499

_OVERLORD_PORT = 4455
_OVERLORD_LAN_DISCOVERY_PORT = 4456

_BUFSIZE = 8192
_RETRY_INTERVAL = 2
_SEPARATOR = '\r\n'
_PING_TIMEOUT = 3
_PING_INTERVAL = 5
_REQUEST_TIMEOUT_SECS = 60
_SHELL = os.getenv('SHELL', '/bin/bash')
_DEFAULT_BIND_ADDRESS = '0.0.0.0'

RESPONSE_SUCCESS = 'success'
RESPONSE_FAILED = 'failed'


class PingTimeoutError(Exception):
  pass


class RequestError(Exception):
  pass


class Ghost(object):
  """Ghost implements the client protocol of Overlord.

  Ghost provide terminal/shell/logcat functionality and manages the client
  side connectivity.
  """
  NONE, AGENT, TERMINAL, SHELL, LOGCAT = range(5)

  MODE_NAME = {
      NONE: 'NONE',
      AGENT: 'Agent',
      TERMINAL: 'Terminal',
      SHELL: 'Shell',
      LOGCAT: 'Logcat'
      }

  RANDOM_MID = '##random_mid##'

  def __init__(self, overlord_addrs, mode=AGENT, mid=None, sid=None,
               command=None):
    """Constructor.

    Args:
      overlord_addrs: a list of possible address of overlord.
      mode: client mode, either AGENT, SHELL or LOGCAT
      mid: a str to set for machine ID. If mid equals Ghost.RANDOM_MID, machine
        id is randomly generated.
      sid: session id. If the connection is requested by overlord, sid should
        be set to the corresponding session id assigned by overlord.
      shell: the command to execute when we are in SHELL mode.
    """
    assert mode in [Ghost.AGENT, Ghost.TERMINAL, Ghost.SHELL]
    if mode == Ghost.SHELL:
      assert command is not None

    self._overlord_addrs = overlord_addrs
    self._connected_addr = None
    self._mode = mode
    self._mid = mid
    self._sock = None
    self._machine_id = self.GetMachineID()
    self._client_id = sid if sid is not None else str(uuid.uuid4())
    self._properties = {}
    self._shell_command = command
    self._buf = ''
    self._requests = {}
    self._reset = threading.Event()
    self._last_ping = 0
    self._queue = Queue.Queue()

  def LoadPropertiesFromFile(self, filename):
    try:
      with open(filename, 'r') as f:
        self._properties = json.loads(f.read())
    except Exception as e:
      logging.exception('LoadPropertiesFromFile: ' + str(e))

  def SpawnGhost(self, mode, sid, command=None):
    """Spawn a child ghost with specific mode.

    Returns:
      The spawned child process pid.
    """
    pid = os.fork()
    if pid == 0:
      g = Ghost([self._connected_addr], mode, Ghost.RANDOM_MID, sid, command)
      g.Start()
      sys.exit(0)
    else:
      return pid

  def Timestamp(self):
    return int(time.time())

  def GetGateWayIP(self):
    with open('/proc/net/route', 'r') as f:
      lines = f.readlines()

    ips = []
    for line in lines:
      parts = line.split('\t')
      if parts[2] == '00000000':
        continue

      try:
        h = parts[2].decode('hex')
        ips.append('%d.%d.%d.%d' % tuple(ord(x) for x in reversed(h)))
      except TypeError:
        pass

    return ips

  def GetShopfloorIP(self):
    try:
      import factory_common  # pylint: disable=W0612
      from cros.factory.test import shopfloor

      url = shopfloor.get_server_url()
      match = re.match(r'^https?://(.*):.*$', url)
      if match:
        return [match.group(1)]
    except Exception:
      pass
    return []

  def GetMachineID(self):
    """Generates machine-dependent ID string for a machine.
    There are many ways to generate a machine ID:
    1. factory device-data
    2. factory device_id
    3. /sys/class/dmi/id/product_uuid (only available on intel machines)
    4. MAC address
    We follow the listed order to generate machine ID, and fallback to the next
    alternative if the previous doesn't work.
    """
    if self._mid == Ghost.RANDOM_MID:
      return str(uuid.uuid4())
    elif self._mid:
      return self._mid

    # Try factory device data
    try:
      p = subprocess.Popen('factory device-data | grep mlb_serial_number | '
                           'cut -d " " -f 2', stdout=subprocess.PIPE,
                           stderr=subprocess.PIPE, shell=True)
      stdout, _ = p.communicate()
      if stdout == '':
        raise RuntimeError('empty mlb number')
      return stdout.strip()
    except Exception:
      pass

    # Try factory device id
    try:
      import factory_common  # pylint: disable=W0612
      from cros.factory.test import event_log
      with open(event_log.DEVICE_ID_PATH) as f:
        return f.read()
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

    raise RuntimeError("can't generate machine ID")

  def Reset(self):
    """Reset state and clear request handlers."""
    self._reset.clear()
    self._buf = ""
    self._last_ping = 0
    self._requests = {}

  def SendMessage(self, msg):
    """Serialize the message and send it through the socket."""
    self._sock.send(json.dumps(msg) + _SEPARATOR)

  def SendRequest(self, name, args, handler=None,
                  timeout=_REQUEST_TIMEOUT_SECS):
    if handler and not callable(handler):
      raise RequestError('Invalid requiest handler for msg "%s"' % name)

    rid = str(uuid.uuid4())
    msg = {'rid': rid, 'timeout': timeout, 'name': name, 'params': args}
    self._requests[rid] = [self.Timestamp(), timeout, handler]
    self.SendMessage(msg)

  def SendResponse(self, omsg, status, params=None):
    msg = {'rid': omsg['rid'], 'response': status, 'params': params}
    self.SendMessage(msg)

  def SpawnPTYServer(self, _):
    """Spawn a PTY server and forward I/O to the TCP socket."""
    logging.info('SpawnPTYServer: started')

    pid, fd = os.forkpty()
    if pid == 0:
      env = os.environ.copy()
      env['USER'] = os.getenv('USER', 'root')
      env['HOME'] = os.getenv('HOME', '/root')
      os.chdir(env['HOME'])
      os.execve(_SHELL, [_SHELL], env)
    else:
      try:
        while True:
          rd, _, _ = select.select([self._sock, fd], [], [])

          if fd in rd:
            self._sock.send(os.read(fd, _BUFSIZE))

          if self._sock in rd:
            ret = self._sock.recv(_BUFSIZE)
            if len(ret) == 0:
              raise RuntimeError("socket closed")
            os.write(fd, ret)
      except (OSError, socket.error, RuntimeError):
        self._sock.close()
        logging.info('SpawnPTYServer: terminated')
        sys.exit(0)

  def SpawnShellServer(self, _):
    """Spawn a shell server and forward input/output from/to the TCP socket."""
    logging.info('SpawnShellServer: started')

    p = subprocess.Popen(self._shell_command, stdin=subprocess.PIPE,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                         shell=True)

    def make_non_block(fd):
      fl = fcntl.fcntl(fd, fcntl.F_GETFL)
      fcntl.fcntl(fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

    make_non_block(p.stdout)
    make_non_block(p.stderr)

    try:
      while True:
        rd, _, _ = select.select([p.stdout, p.stderr, self._sock], [], [])
        p.poll()

        if p.returncode != None:
          raise RuntimeError("process complete")

        if p.stdout in rd:
          self._sock.send(p.stdout.read(_BUFSIZE))

        if p.stderr in rd:
          self._sock.send(p.stderr.read(_BUFSIZE))

        if self._sock in rd:
          ret = self._sock.recv(_BUFSIZE)
          if len(ret) == 0:
            raise RuntimeError("socket closed")
          p.stdin.write(ret)
    except (OSError, socket.error, RuntimeError):
      self._sock.close()
      logging.info('SpawnShellServer: terminated')
      sys.exit(0)


  def Ping(self):
    def timeout_handler(x):
      if x is None:
        raise PingTimeoutError

    self._last_ping = self.Timestamp()
    self.SendRequest('ping', {}, timeout_handler, 5)

  def HandleRequest(self, msg):
    if msg['name'] == 'terminal':
      self.SpawnGhost(self.TERMINAL, msg['params']['sid'])
      self.SendResponse(msg, RESPONSE_SUCCESS)
    elif msg['name'] == 'shell':
      self.SpawnGhost(self.SHELL, msg['params']['sid'],
                      msg['params']['command'])
      self.SendResponse(msg, RESPONSE_SUCCESS)

  def HandleResponse(self, response):
    rid = str(response['rid'])
    if rid in self._requests:
      handler = self._requests[rid][2]
      del self._requests[rid]
      if callable(handler):
        handler(response)
    else:
      print(response, self._requests.keys())
      logging.warning('Recvied unsolicited response, ignored')

  def ParseMessage(self):
    msgs_json = self._buf.split(_SEPARATOR)
    self._buf = msgs_json.pop()

    for msg_json in msgs_json:
      try:
        msg = json.loads(msg_json)
      except ValueError:
        # Ignore mal-formed message.
        continue

      if 'name' in msg:
        self.HandleRequest(msg)
      elif 'response' in msg:
        self.HandleResponse(msg)
      else:  # Ingnore mal-formed message.
        pass

  def ScanForTimeoutRequests(self):
    for rid in self._requests.keys()[:]:
      request_time, timeout, handler = self._requests[rid]
      if self.Timestamp() - request_time > timeout:
        handler(None)
        del self._requests[rid]

  def Listen(self):
    try:
      while True:
        rds, _, _ = select.select([self._sock], [], [], _PING_INTERVAL / 2)

        if self._sock in rds:
          self._buf += self._sock.recv(_BUFSIZE)
          self.ParseMessage()

        if self.Timestamp() - self._last_ping > _PING_INTERVAL:
          self.Ping()
        self.ScanForTimeoutRequests()

        if self._reset.is_set():
          self.Reset()
          break
    except socket.error:
      raise RuntimeError('Connection dropped')
    except PingTimeoutError:
      raise RuntimeError('Connection timeout')
    finally:
      self._sock.close()

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
        logging.info('Registered with Overlord at %s:%d', *non_local['addr'])
        self._queue.put("pause", True)

      try:
        logging.info('Trying %s:%d ...', *addr)
        self.Reset()
        self._sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._sock.settimeout(_PING_TIMEOUT)
        self._sock.connect(addr)

        logging.info('Connection established, registering...')
        handler = {
            Ghost.AGENT: registered,
            Ghost.TERMINAL: self.SpawnPTYServer,
            Ghost.SHELL: self.SpawnShellServer
            }[self._mode]

        # Machine ID may change if MAC address is used (USB-ethernet dongle
        # plugged/unplugged)
        self._machine_id = self.GetMachineID()
        self.SendRequest('register',
                         {'mode': self._mode, 'mid': self._machine_id,
                          'cid': self._client_id,
                          'properties': self._properties}, handler)
      except socket.error:
        pass
      else:
        self._sock.settimeout(None)
        self._connected_addr = addr
        self.Listen()

    raise RuntimeError("Cannot connect to any server")

  def Reconnect(self):
    logging.info('Received reconnect request from RPC server, reconnecting...')
    self._reset.set()

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
        logging.error("LAN discovery: %s, abort", e)
        return

      logging.info('LAN Discovery: started')
      while True:
        rd, _, _ = select.select([s], [], [], 1)

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
        except Queue.Empty:
          pass
        else:
          if type(obj) is not str:
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
    rpc_server = SimpleJSONRPCServer((_DEFAULT_BIND_ADDRESS, _GHOST_RPC_PORT),
                                     logRequests=False)
    rpc_server.register_function(self.Reconnect, 'Reconnect')
    t = threading.Thread(target=rpc_server.serve_forever)
    t.daemon = True
    t.start()

  def ScanServer(self):
    for meth in [self.GetGateWayIP, self.GetShopfloorIP]:
      for addr in [(x, _OVERLORD_PORT) for x in meth()]:
        if addr not in self._overlord_addrs:
          self._overlord_addrs.append(addr)

  def Start(self, lan_disc=False, rpc_server=False):
    logging.info('%s started', self.MODE_NAME[self._mode])
    logging.info('MID: %s', self._machine_id)
    logging.info('CID: %s', self._client_id)

    if lan_disc:
      self.StartLanDiscovery()

    if rpc_server:
      self.StartRPCServer()

    try:
      while True:
        try:
          addr = self._queue.get(False)
        except Queue.Empty:
          pass
        else:
          if type(addr) == tuple and addr not in self._overlord_addrs:
            logging.info('LAN Discovery: got overlord address %s:%d', *addr)
            self._overlord_addrs.append(addr)

        try:
          self.ScanServer()
          self.Register()
        except Exception as e:
          logging.info(str(e) + ', retrying in %ds' % _RETRY_INTERVAL)
          time.sleep(_RETRY_INTERVAL)

        self.Reset()
    except KeyboardInterrupt:
      logging.error('Received keyboard interrupt, quit')
      sys.exit(0)


def GhostRPCServer():
  return jsonrpclib.Server('http://localhost:%d' % _GHOST_RPC_PORT)


def main():
  logger = logging.getLogger()
  logger.setLevel(logging.INFO)

  parser = argparse.ArgumentParser()
  parser.add_argument('--mid', metavar='MID', dest='mid', action='store',
                      default=None, help='use MID as machine ID')
  parser.add_argument('--rand-mid', dest='mid', action='store_const',
                      const=Ghost.RANDOM_MID, help='use random machine ID')
  parser.add_argument('--no-lan-disc', dest='lan_disc', action='store_false',
                      default=True, help='disable LAN discovery')
  parser.add_argument('--no-rpc-server', dest='rpc_server',
                      action='store_false', default=True,
                      help='disable RPC server')
  parser.add_argument("--prop-file", dest="prop_file", type=str, default=None,
                      help='file containing the JSON representation of client '
                           'properties')
  parser.add_argument('overlord_ip', metavar='OVERLORD_IP', type=str,
                      nargs='*', help='overlord server address')
  args = parser.parse_args()

  addrs = [('localhost', _OVERLORD_PORT)]
  addrs += [(x, _OVERLORD_PORT) for x in args.overlord_ip]

  g = Ghost(addrs, Ghost.AGENT, args.mid)
  if args.prop_file:
    g.LoadPropertiesFromFile(args.prop_file)
  g.Start(args.lan_disc, args.rpc_server)


if __name__ == '__main__':
  main()
