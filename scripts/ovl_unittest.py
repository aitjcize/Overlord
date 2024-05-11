#!/usr/bin/env python3

import argparse
import os
import unittest

from ovl import OverlordCliClient


class PushUnittest(unittest.TestCase):
  """Test cases for push command."""
  def setUp(self):
    self.cli = OverlordCliClient()
    self.results = []
    self.git_root = os.path.normpath(os.path.join(
      os.path.dirname(os.path.abspath(__file__)), '..'))

    self.cli.CheckClient = lambda: None
    self.cli._PushSingleFile = self.PushSingleFile

  def PushSingleFile(self, src, dst):
    """Mock function for _PushSingleFile."""
    self.results.append((src, dst))

  def test_push_src_abs_dir_remote_exists_dir(self):
    """Test push src dir to remote dir when remote dir exists."""

    dst = '/home/aitjcize/some_dir'
    self.cli._RemoteDirExists = lambda path: True
    self.cli._RemoteGetPathType = lambda path: 'directory'
    args = argparse.Namespace(srcs=[os.path.join(self.git_root, 'cmd')], dst=dst)
    self.cli.Push(args)

    self.assertEqual(sorted(self.results), sorted([
      (os.path.join(self.git_root, 'cmd/ghost/main.go'),
       os.path.join(dst, 'cmd/ghost/main.go')),
      (os.path.join(self.git_root, 'cmd/overlordd/main.go'),
       os.path.join(dst, 'cmd/overlordd/main.go')),
    ]))

  def test_push_src_abs_dir_remote_exists_but_not_dir(self):
    """Test push src dir to remote dir when remote path exists but not a
    directory."""

    dst = '/home/aitjcize/some_dir'
    self.cli._RemoteDirExists = lambda path: True
    self.cli._RemoteGetPathType = lambda path: 'file'
    args = argparse.Namespace(srcs=[os.path.join(self.git_root, 'cmd')], dst=dst)

    self.assertRaises(RuntimeError, self.cli.Push, args)

  def test_push_src_abs_dir_remote_does_not_exist(self):
    """Test push src dir to remote dir when remote dir does not exist."""

    dst = '/home/aitjcize/some_dir'
    self.cli._RemoteDirExists = lambda path: False
    self.cli._RemoteGetPathType = lambda path: 'directory'
    args = argparse.Namespace(srcs=[os.path.join(self.git_root, 'cmd')], dst=dst)
    self.cli.Push(args)

    self.assertEqual(sorted(self.results), sorted([
      (os.path.join(self.git_root, 'cmd/ghost/main.go'),
       os.path.join(dst, 'ghost/main.go')),
      (os.path.join(self.git_root, 'cmd/overlordd/main.go'),
       os.path.join(dst, 'overlordd/main.go')),
    ]))

  def test_push_rel_dir_remote_rel_dir(self):
    """Test push relative src dir to relative remote dir."""

    os.chdir(self.git_root)
    dst = 'some_dir'
    self.cli._RemoteDirExists = lambda path: True
    self.cli._RemoteGetPathType = lambda path: 'directory'
    args = argparse.Namespace(srcs=['cmd'], dst=dst)
    self.cli.Push(args)

    self.assertEqual(sorted(self.results), sorted([
      (os.path.join(self.git_root, 'cmd/ghost/main.go'),
       os.path.join(dst, 'cmd/ghost/main.go')),
      (os.path.join(self.git_root, 'cmd/overlordd/main.go'),
       os.path.join(dst, 'cmd/overlordd/main.go')),
    ]))

  def test_push_rel_dir_remote_rel_does_not_exist(self):
    """Test push relative src dir to relative remote dir when remote dir does
    not exist."""

    os.chdir(self.git_root)
    dst = 'some_dir'
    self.cli._RemoteDirExists = lambda path: False
    self.cli._RemoteGetPathType = lambda path: 'directory'
    args = argparse.Namespace(srcs=['cmd'], dst=dst)
    self.cli.Push(args)

    self.assertEqual(sorted(self.results), sorted([
      (os.path.join(self.git_root, 'cmd/ghost/main.go'),
       os.path.join(dst, 'ghost/main.go')),
      (os.path.join(self.git_root, 'cmd/overlordd/main.go'),
       os.path.join(dst, 'overlordd/main.go')),
    ]))

if __name__ == '__main__':
  unittest.main()
