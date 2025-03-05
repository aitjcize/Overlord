#!/usr/bin/env python3
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from contextlib import closing
import json
import os
import shutil
import socket
import subprocess
import tempfile
import time
import unittest
import urllib.parse
import urllib.request

from ws4py.client import WebSocketBaseClient


# Constants.
_HOST = '127.0.0.1'
_INCREMENT = 42


class TestError(Exception):
  pass


class CloseWebSocket(Exception):
  pass


def FindUnusedPort():
  with closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as s:
    s.bind(('', 0))
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    return s.getsockname()[1]


class TestOverlord(unittest.TestCase):
  @classmethod
  def setUpClass(cls):
    # Build overlord, only do this once over all tests.
    gitroot = os.path.normpath(os.path.join(os.path.dirname(__file__),
                               '..'))
    cls.bindir = tempfile.mkdtemp()
    subprocess.call('make -C %s BIN=%s overlordd ghost' %
                    (gitroot, cls.bindir), shell=True)

  @classmethod
  def tearDownClass(cls):
    if os.path.isdir(cls.bindir):
      shutil.rmtree(cls.bindir)

  def setUp(self):
    self.basedir = os.path.dirname(__file__)
    bindir = self.__class__.bindir
    scriptdir = os.path.normpath(os.path.join(self.basedir, '../scripts'))

    env = os.environ.copy()
    env['SHELL'] = os.path.join(os.getcwd(), self.basedir, 'test_shell.sh')

    # set ports for overlord to bind
    overlord_http_port = FindUnusedPort()
    self.host = '%s:%d' % (_HOST, overlord_http_port)
    env['OVERLORD_LD_PORT'] = str(FindUnusedPort())
    env['GHOST_RPC_PORT'] = str(FindUnusedPort())

    # Launch overlord
    self.ovl = subprocess.Popen(['%s/overlordd' % bindir, '-no-auth',
                                 '-port', str(overlord_http_port)], env=env)

    # Launch go implementation of ghost
    self.goghost = subprocess.Popen(['%s/ghost' % bindir,
                                     '-mid=go', '-no-lan-disc',
                                     '-no-rpc-server', '-tls=n',
                                     'localhost:%d' % overlord_http_port],
                                    env=env)

    # Launch python implementation of ghost
    self.pyghost = subprocess.Popen(['%s/ghost.py' % scriptdir,
                                     '--mid=python', '--no-lan-disc',
                                     '--no-rpc-server', '--tls=n',
                                     'localhost:%d' % overlord_http_port],
                                    env=env)

    def CheckClient():
      try:
        clients = self._GetJSON('/api/agents/list')
        return len(clients) == 2
      except IOError:
        # overlordd is not ready yet.
        return False

    # Wait for clients to connect
    try:
      for unused_i in range(30):
        if CheckClient():
          return
        time.sleep(1)
      raise RuntimeError('client not connected')
    except Exception:
      self.tearDown()
      raise

  def tearDown(self):
    self.goghost.kill()
    self.goghost.wait()

    # Python implementation uses process instead of goroutine, also kill those
    with subprocess.Popen('pkill -P %d' % self.pyghost.pid, shell=True) as p:
      p.wait()

    self.pyghost.kill()
    self.pyghost.wait()

    self.ovl.kill()
    self.ovl.wait()

  def _GetJSON(self, path):
    return json.loads(urllib.request.urlopen(
        'http://' + self.host + path).read())

  def testWebAPI(self):
    # Test /api/app/list
    appsdir = os.path.join(self.basedir, '../webroot/apps')
    apps = os.listdir(appsdir)
    res = self._GetJSON('/api/apps/list')
    assert len(res['apps']) == len(apps)

    # Test /api/agents/list
    assert len(self._GetJSON('/api/agents/list')) == 2

    # Test /api/logcats/list. TODO(wnhuang): test this properly
    assert not self._GetJSON('/api/logcats/list')

    # Test /api/agent/properties/mid
    for client in self._GetJSON('/api/agents/list'):
      assert self._GetJSON(
          '/api/agent/properties/%s' % client['mid']) is not None

  def testShellCommand(self):
    class TestClient(WebSocketBaseClient):
      def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.message = b''

      def handshake_ok(self):
        pass

      def received_message(self, message):
        self.message += message.data

    clients = self._GetJSON('/api/agents/list')
    self.assertTrue(clients)
    answer = subprocess.check_output(['uname', '-r'])

    for client in clients:
      ws = TestClient('ws://' + self.host + '/api/agent/shell/%s' %
                      urllib.parse.quote(client['mid']) + '?command=' +
                      urllib.parse.quote('uname -r'))
      ws.connect()
      ws.run()
      self.assertEqual(ws.message, answer)

  def testTerminalCommand(self):
    class TestClient(WebSocketBaseClient):
      NONE, PROMPT, RESPONSE = range(0, 3)

      def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.state = self.NONE
        self.answer = 0
        self.test_run = False
        self.buffer = b''

      def handshake_ok(self):
        pass

      def closed(self, code, reason=None):
        if not self.test_run:
          raise RuntimeError('test exit before being run: %s' % reason)

      def received_message(self, message):
        if message.is_text:
          # Ignore control messages.
          return

        self.buffer += message.data
        if b'\r\n' not in self.buffer:
          return

        self.test_run = True
        msg_text, self.buffer = self.buffer.split(b'\r\n', 1)
        if self.state == self.NONE:
          if msg_text.startswith(b'TEST-SHELL-CHALLENGE'):
            self.state = self.PROMPT
            challenge_number = int(msg_text.split()[1])
            self.answer = challenge_number + _INCREMENT
            self.send('%d\n' % self.answer)
        elif self.state == self.PROMPT:
          msg_text = msg_text.strip()
          if msg_text == b'SUCCESS':
            raise CloseWebSocket
          if msg_text == b'FAILED':
            raise TestError('Challange failed')
          if msg_text and int(msg_text) == self.answer:
            pass
          else:
            raise TestError('Unexpected response: %r' % msg_text)

    clients = self._GetJSON('/api/agents/list')
    assert clients

    for client in clients:
      ws = TestClient('ws://' + self.host + '/api/agent/tty/%s' %
                      urllib.parse.quote(client['mid']))
      ws.connect()
      try:
        ws.run()
      except TestError as e:
        raise e
      except CloseWebSocket:
        ws.close()


if __name__ == '__main__':
  unittest.main()
