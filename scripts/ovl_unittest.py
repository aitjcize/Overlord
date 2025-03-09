#!/usr/bin/env python3

import argparse
import os
import sys
import unittest
from unittest.mock import MagicMock

# Add scripts directory to Python path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

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
    self.cli._GetDirectoryListing = self.GetDirectoryListing

  def PushSingleFile(self, src, dst):
    """Mock function for _PushSingleFile."""
    self.results.append((src, dst))

  def GetDirectoryListing(self, path):
    """Mock function for _GetDirectoryListing."""
    if path == '/home/aitjcize/some_dir' or path == 'some_dir':
      if self.remote_exists and self.remote_is_dir:
        return [self.cli.FileEntry(path=path, is_dir=True)]
      elif self.remote_exists and not self.remote_is_dir:
        return [self.cli.FileEntry(path=path, is_dir=False)]
      else:
        raise RuntimeError('ls: No such file or directory')
    return []

  def test_push_src_abs_dir_remote_exists_dir(self):
    """Test push src dir to remote dir when remote dir exists."""
    self.remote_exists = True
    self.remote_is_dir = True

    dst = '/home/aitjcize/some_dir'
    args = argparse.Namespace(srcs=[os.path.join(self.git_root, 'cmd')], dst=dst)
    self.cli.Push(args)

    self.assertEqual(sorted(self.results), sorted([
      (os.path.join(self.git_root, 'cmd/ghost/main.go'),
       os.path.join(dst, 'cmd/ghost/main.go')),
      (os.path.join(self.git_root, 'cmd/overlordd/main.go'),
       os.path.join(dst, 'cmd/overlordd/main.go')),
    ]))

  def test_push_src_abs_dir_remote_exists_but_not_dir(self):
    """Test push src dir to remote dir when remote path exists but not a directory."""
    self.remote_exists = True
    self.remote_is_dir = False

    dst = '/home/aitjcize/some_dir'
    args = argparse.Namespace(srcs=[os.path.join(self.git_root, 'cmd')], dst=dst)

    self.assertRaises(RuntimeError, self.cli.Push, args)

  def test_push_src_abs_dir_remote_does_not_exist(self):
    """Test push src dir to remote dir when remote dir does not exist."""
    self.remote_exists = False
    self.remote_is_dir = False

    dst = '/home/aitjcize/some_dir'
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
    self.remote_exists = True
    self.remote_is_dir = True

    os.chdir(self.git_root)
    dst = 'some_dir'
    args = argparse.Namespace(srcs=['cmd'], dst=dst)
    self.cli.Push(args)

    self.assertEqual(sorted(self.results), sorted([
      (os.path.join(self.git_root, 'cmd/ghost/main.go'),
       os.path.join(dst, 'cmd/ghost/main.go')),
      (os.path.join(self.git_root, 'cmd/overlordd/main.go'),
       os.path.join(dst, 'cmd/overlordd/main.go')),
    ]))

  def test_push_rel_dir_remote_rel_does_not_exist(self):
    """Test push relative src dir to relative remote dir when remote dir does not exist."""
    self.remote_exists = False
    self.remote_is_dir = False

    os.chdir(self.git_root)
    dst = 'some_dir'
    args = argparse.Namespace(srcs=['cmd'], dst=dst)
    self.cli.Push(args)

    self.assertEqual(sorted(self.results), sorted([
      (os.path.join(self.git_root, 'cmd/ghost/main.go'),
       os.path.join(dst, 'ghost/main.go')),
      (os.path.join(self.git_root, 'cmd/overlordd/main.go'),
       os.path.join(dst, 'overlordd/main.go')),
    ]))


class PullUnittest(unittest.TestCase):
  """Test cases for pull command."""
  def setUp(self):
    self.cli = OverlordCliClient()
    self.results = []
    self.cli.CheckClient = lambda: None
    self._Pull = self.cli._Pull
    self.cli._Pull = self.PullMock
    self.cli._GetDirectoryListing = self.GetDirectoryListing

  def PullMock(self, entry, dst):
    """Mock function for _Pull."""
    self.results.append((entry.path, dst))

  def GetDirectoryListing(self, path):
    """Mock function for _GetDirectoryListing."""
    if path == '/remote/testdir':
      return [
        self.cli.FileEntry(path='/remote/testdir/a'),
        self.cli.FileEntry(path='/remote/testdir/b'),
        self.cli.FileEntry(path='/remote/testdir/c', is_dir=True),
      ]
    elif path == '/remote/testdir/c':
      return [
        self.cli.FileEntry(path='/remote/testdir/c/d'),
        self.cli.FileEntry(path='/remote/testdir/c/e', is_dir=True),
      ]
    elif path == '/remote/testdir/c/e':
      return [
        self.cli.FileEntry(path='/remote/testdir/c/e/f'),
      ]
    elif path == '/remote/single_file':
      return [self.cli.FileEntry(path='/remote/single_file')]
    return []

  def test_pull_single_file(self):
    """Test pulling a single file."""
    args = argparse.Namespace(src='/remote/single_file', dst='local_file')
    self.cli.Pull(args)

    self.assertEqual(self.results, [
      ('/remote/single_file', 'local_file'),
    ])

  def test_pull_single_file_to_dir(self):
    """Test pulling a single file to a directory."""
    args = argparse.Namespace(src='/remote/single_file', dst='local_dir')
    os.makedirs('local_dir', exist_ok=True)
    self.cli.Pull(args)

    self.assertEqual(self.results, [
      ('/remote/single_file', 'local_dir/single_file'),
    ])

  def test_pull_directory(self):
    """Test pulling a directory recursively."""
    args = argparse.Namespace(src='/remote/testdir', dst='local_dir')
    self.cli.Pull(args)

    self.assertEqual(sorted(self.results), sorted([
      ('/remote/testdir/a', 'local_dir/testdir/a'),
      ('/remote/testdir/b', 'local_dir/testdir/b'),
      ('/remote/testdir/c/d', 'local_dir/testdir/c/d'),
      ('/remote/testdir/c/e/f', 'local_dir/testdir/c/e/f'),
    ]))

  def test_pull_directory_to_existing_dir(self):
    """Test pulling a directory to an existing directory."""
    args = argparse.Namespace(src='/remote/testdir', dst='existing_dir')
    os.makedirs('existing_dir', exist_ok=True)
    self.cli.Pull(args)

    self.assertEqual(sorted(self.results), sorted([
      ('/remote/testdir/a', 'existing_dir/testdir/a'),
      ('/remote/testdir/b', 'existing_dir/testdir/b'),
      ('/remote/testdir/c/d', 'existing_dir/testdir/c/d'),
      ('/remote/testdir/c/e/f', 'existing_dir/testdir/c/e/f'),
    ]))


if __name__ == '__main__':
  unittest.main()
