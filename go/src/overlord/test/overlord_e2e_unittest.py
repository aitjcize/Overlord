#!/usr/bin/python -u
# -*- coding: utf-8 -*-
#
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import base64
import json
import os
import subprocess
import time
import unittest
import urllib

from ws4py.client import WebSocketBaseClient


_HOST = '127.0.0.1:9000'
_INCREMENT = 42


class TestError(Exception):
  pass


class CloseWebSocket(Exception):
  pass


class TestOverlord(unittest.TestCase):
  def setUp(self):
    self.basedir = os.path.dirname(__file__)
    bindir = os.path.normpath(os.path.join(self.basedir, '../../../bin'))
    factorydir = os.path.normpath(os.path.join(self.basedir, '../../../..'))

    # Build overlord
    subprocess.call('make -C %s' % os.path.join(self.basedir, '..'),
                    shell=True)

    env = os.environ.copy()
    env['SHELL'] = os.path.join(os.getcwd(), self.basedir, 'test_shell.sh')

    # Launch overlord
    self.ovl = subprocess.Popen(['%s/overlordd' % bindir, '-noauth'])

    # Launch go implementation of ghost
    self.goghost = subprocess.Popen(['%s/ghost' % bindir, '-rand-mid',
                                     '-no-lan-disc'], env=env)

    # Launch python implementation of ghost
    self.pyghost = subprocess.Popen(['%s/py/tools/ghost.py' % factorydir,
                                     '--rand-mid', '--no-lan-disc',
                                     '--no-rpc-server'], env=env)
    time.sleep(1)

  def tearDown(self):
    self.ovl.kill()
    self.goghost.kill()
    self.pyghost.kill()

  def _GetJSON(self, path):
    return json.loads(urllib.urlopen('http://' + _HOST + path).read())

  def testWebAPI(self):
    # Test /api/app/list
    appdir = os.path.join(self.basedir, '../app')
    apps = [x for x in os.listdir(appdir)
            if os.path.isdir(os.path.join(appdir, x)) and x != 'common']
    res = self._GetJSON('/api/apps/list')
    assert len(res['apps']) == len(apps)

    # Test /api/agents/list
    assert len(self._GetJSON('/api/agents/list')) == 2

    # Test /api/logcats/list. TODO(wnhuang): test this properly
    assert len(self._GetJSON('/api/logcats/list')) == 0

    # Test /api/agent/properties/mid
    for client in self._GetJSON('/api/agents/list'):
      assert self._GetJSON('/api/agent/properties/%s' % client['mid']) != None

  def testShellCommand(self):
    clients = self._GetJSON('/api/agents/list')

    class TestClient(WebSocketBaseClient):
      def __init__(self, *args, **kwargs):
        super(TestClient, self).__init__(*args, **kwargs)
        self.answer = subprocess.check_output(['uname', '-r'])

      def handshake_ok(self):
        pass

      def received_message(self, msg):
        msg_text = base64.b64decode(msg.data)
        assert msg_text == self.answer
        raise CloseWebSocket

    for client in clients:
      ws = TestClient('ws://' + _HOST + '/api/agent/shell/%s' %
                      urllib.quote(client['mid']) + '?command=' +
                      urllib.quote('uname -r'))
      ws.connect()
      try:
        ws.run()
      except TestError as e:
        raise e
      except CloseWebSocket:
        ws.close()

  def testTerminalCommand(self):
    clients = self._GetJSON('/api/agents/list')

    class TestClient(WebSocketBaseClient):
      NONE, PROMPT, RESPONSE = range(0, 3)

      def __init__(self, *args, **kwargs):
        super(TestClient, self).__init__(*args, **kwargs)
        self.state = self.NONE
        self.answer = 0

      def handshake_ok(self):
        pass

      def received_message(self, msg):
        msg_text = base64.b64decode(msg.data)
        if self.state == self.NONE:
          if msg_text.startswith('TEST-SHELL-CHALLENGE'):
            self.state = self.PROMPT
            challenge_number = int(msg_text.split()[1])
            self.answer = challenge_number + _INCREMENT
            self.send('%d\n' % self.answer)
        elif self.state == self.PROMPT:
          msg_text = msg_text.strip()
          if msg_text == 'SUCCESS':
            raise CloseWebSocket
          elif msg_text == 'FAILED':
            raise TestError('Challange failed')
          elif msg_text and int(msg_text) == self.answer:
            pass
          else:
            raise TestError('Unexpected response: %s' % msg_text)

    for client in clients:
      ws = TestClient('ws://' + _HOST + '/api/agent/pty/%s' %
                      urllib.quote(client['mid']))
      ws.connect()
      try:
        ws.run()
      except TestError as e:
        raise e
      except CloseWebSocket:
        ws.close()


if __name__ == '__main__':
  unittest.main()
