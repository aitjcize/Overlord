#!/usr/bin/env python3
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from contextlib import closing
import bcrypt
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
    scriptdir = os.path.normpath(os.path.join(self.basedir, '../py'))

    env = os.environ.copy()
    env['SHELL'] = os.path.join(os.getcwd(), self.basedir, 'test_shell.sh')

    # set ports for overlord to bind
    overlord_http_port = FindUnusedPort()
    self.host = '%s:%d' % (_HOST, overlord_http_port)
    env['OVERLORD_LD_PORT'] = str(FindUnusedPort())
    env['GHOST_RPC_PORT'] = str(FindUnusedPort())

    # Create temporary auth files
    self.temp_dir = tempfile.mkdtemp()

    # Create JWT secret file
    self.jwt_secret_path = os.path.join(self.temp_dir, 'jwt-secret')
    with open(self.jwt_secret_path, 'w') as f:
      f.write('test-secret-key-for-jwt-authentication')

    # Create htpasswd file with test user
    self.htpasswd_path = os.path.join(self.temp_dir, 'overlord.htpasswd')
    password = 'testpassword'

    # Create a bcrypt hash with a supported prefix (2b) and then manually
    # replace it with 2y Use a low cost factor (5) for faster tests
    hashed = bcrypt.hashpw(password.encode('utf-8'),
                           bcrypt.gensalt(rounds=5)).decode('utf-8')
    # Replace the prefix with $2y$ which is what the server expects
    hashed = hashed.replace('$2b$', '$2y$')

    with open(self.htpasswd_path, 'w') as f:
      f.write(f'testuser:{hashed}\n')

    # Store test credentials
    self.test_username = 'testuser'
    self.test_password = 'testpassword'

    # Launch overlord with proper authentication parameters
    self.ovl = subprocess.Popen([
        '%s/overlordd' % bindir,
        '-port', str(overlord_http_port),
        '-htpasswd-path', self.htpasswd_path,
        '-jwt-secret-path', self.jwt_secret_path,
        '-no-lan-disc'  # Disable LAN discovery to avoid network issues
    ], env=env)

    # Wait for the server to start
    time.sleep(2)

    # Launch go implementation of ghost
    self.goghost = subprocess.Popen([
        '%s/ghost' % bindir,
        '-mid=go',
        '-no-lan-disc',
        '-no-rpc-server',
        '-tls=n',
        '-allowlist=testuser',
        'localhost:%d' % overlord_http_port
    ], env=env)

    # Launch python implementation of ghost
    self.pyghost = subprocess.Popen([
        '%s/ghost.py' % scriptdir,
        '--mid=python',
        '--no-lan-disc',
        '--no-rpc-server',
        '--tls=n',
        '--allowlist=testuser',
        'localhost:%d' % overlord_http_port
    ], env=env)

    # Get JWT token for authentication
    self.token = self._GetJWTToken()
    if not self.token:
        self.fail("Failed to get authentication token")

    def CheckClient():
      try:
        clients = self._GetJSON('/api/agents')
        return len(clients) == 2
      except Exception as e:
        print(f"Error checking clients: {e}")
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

  def _GetJWTToken(self):
    """Get a JWT token for API authentication."""
    # Prepare the login request
    login_url = f'http://{self.host}/api/auth/login'
    headers = {'Content-Type': 'application/json'}
    data = json.dumps({'username': self.test_username,
                       'password': self.test_password}).encode()

    # Make the login request
    req = urllib.request.Request(login_url, data=data, headers=headers,
                                 method='POST')

    try:
      with urllib.request.urlopen(req) as response:
        response_data = json.loads(response.read().decode('utf-8'))
        return response_data['data']['token']
    except urllib.error.HTTPError as e:
      response_data = json.loads(e.read().decode('utf-8'))
      raise TestError(f"Authentication failed: {response_data['data']}")

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

    # Clean up temp directory
    shutil.rmtree(self.temp_dir)

  def _GetJSON(self, path):
    url = f'http://{self.host}{path}'
    headers = {'Authorization': f'Bearer {self.token}'}

    req = urllib.request.Request(url, headers=headers)
    try:
      with urllib.request.urlopen(req) as f:
        response_data = json.loads(f.read().decode('utf-8'))
        return response_data['data']
    except urllib.error.HTTPError as e:
      response_data = json.loads(e.read().decode('utf-8'))
      raise TestError(response_data['data'])

  def testWebAPI(self):
    # Test /api/apps
    response = self._GetJSON('/api/apps')
    self.assertIsInstance(response, list)

    # Test /api/agents
    response = self._GetJSON('/api/agents')
    self.assertIsInstance(response, list)
    self.assertEqual(len(response), 2)
    self.assertTrue(any(agent['mid'] == 'go' for agent in response))
    self.assertTrue(any(agent['mid'] == 'python' for agent in response))

    # Test /api/agents properties
    for agent in response:
      response = self._GetJSON(f'/api/agents/{agent["mid"]}/properties')
      self.assertIsInstance(response, dict)

  def testShellCommand(self):
    class TestClient(WebSocketBaseClient):
      def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.message = b''

      def handshake_ok(self):
        pass

      def received_message(self, message):
        self.message += message.data

    clients = self._GetJSON('/api/agents')
    self.assertTrue(clients)
    answer = subprocess.check_output(['uname', '-r'])

    for client in clients:
      ws = TestClient('ws://' + self.host + '/api/agents/%s/shell' %
                      urllib.parse.quote(client['mid']) + '?command=' +
                      urllib.parse.quote('uname -r') + '&token=' + self.token)
      ws.connect()
      ws.run()
      self.assertEqual(ws.message, answer)

  def testTerminalCommand(self):
    class TestClient(WebSocketBaseClient):
      NONE, PROMPT, RESPONSE = range(0, 3)

      def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.state = self.NONE
        self.message = ''

      def handshake_ok(self):
        pass

      def closed(self, code, reason=None):
        del code, reason  # Unused.
        raise CloseWebSocket()

      def received_message(self, message):
        msg_text = message.data.decode('utf-8')
        if self.state == self.NONE:
          if msg_text.startswith('$ '):
            self.state = self.PROMPT
            self.send('echo %d\n' % _INCREMENT)
          else:
            raise TestError('Unexpected response: %r' % msg_text)
        elif self.state == self.PROMPT:
          if msg_text == 'echo %d\r\n' % _INCREMENT:
            self.state = self.RESPONSE
          else:
            raise TestError('Unexpected response: %r' % msg_text)
        elif self.state == self.RESPONSE:
          if msg_text == '%d\r\n' % _INCREMENT:
            raise CloseWebSocket()
          else:
            raise TestError('Unexpected response: %r' % msg_text)

    clients = self._GetJSON('/api/agents')
    assert clients

    for client in clients:
      ws = TestClient('ws://' + self.host + '/api/agents/%s/tty?token=%s' %
                      (urllib.parse.quote(client['mid']), self.token))
      try:
        ws.connect()
        ws.run()
      except CloseWebSocket:
        pass


if __name__ == '__main__':
  unittest.main()
