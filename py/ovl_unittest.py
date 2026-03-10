#!/usr/bin/env python3

import argparse
import os
import sys
import unittest
from unittest.mock import patch

# Add scripts directory to Python path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# pylint: disable=wrong-import-position
from ovl import OverlordCliClient, FileEntry
# pylint: enable=wrong-import-position


class FileSystemTestCase(unittest.TestCase):

  """Base class for file system related test cases."""

  def setUp(self):
    self.cli = OverlordCliClient()
    self.results = []

    # Define fixed paths for testing instead of using actual filesystem paths
    self.mock_root = "/mock/root"
    self.remote_root = "/remote"

    # Define user home directory for relative paths
    self.user_home = "/home/user"

    # Define a pseudo file system structure for both push and pull tests
    self.pseudo_fs = {
        "some_dir": {
            "is_dir": True,
            "children": {
                "a": {
                    "is_dir": True,
                    "children": {
                        "b": {
                            "is_dir": False,
                            "content": "file b content"
                        },
                        "c": {
                            "is_dir": True,
                            "children": {
                                "d": {
                                    "is_dir": False,
                                    "content": "file d content",
                                }
                            },
                        },
                    },
                }
            },
        },
        "single_file": {
            "is_dir": False,
            "content": "single file content"
        },
    }

  def _build_file_entries(self, base_path, structure, path_prefix=""):
    """Helper to build FileEntry objects from pseudo file system structure."""
    entries = []

    for name, info in structure.items():
      full_path = os.path.join(path_prefix, name)
      abs_path = os.path.join(base_path, full_path)

      if info["is_dir"]:
        entries.append(FileEntry(path=abs_path, is_dir=True))
        if "children" in info:
          entries.extend(
              self._build_file_entries(base_path, info["children"], full_path))
      else:
        entries.append(FileEntry(path=abs_path, is_dir=False))

    return entries

  def _get_all_files(self, structure, path_prefix=""):
    """Helper to get all file paths from the pseudo file system structure."""
    files = []

    for name, info in structure.items():
      full_path = os.path.join(path_prefix, name)

      # Check if info is a dictionary and has 'is_dir' key
      is_dir = False
      if isinstance(info, dict) and "is_dir" in info:
        is_dir = info["is_dir"]

      if is_dir:
        if isinstance(info, dict) and "children" in info:
          files.extend(self._get_all_files(info["children"], full_path))
      else:
        files.append(full_path)

    return files


class PushUnittest(FileSystemTestCase):

  """Test cases for push command."""

  def setUp(self):
    super().setUp()
    self.cli.CheckClient = lambda: None
    self.cli._PushSingle = self.PushSingle
    self.cli._LocalLsTree = self.LocalLsTree
    self.cli._Fstat = self.Fstat
    self.cli._Mkdir = self.Mkdir

  def PushSingle(self, src, dst):
    """Mock function for _PushSingle."""
    self.results.append((src, dst))

  def Mkdir(self, path, perm=0o755):  # pylint: disable=unused-argument
    """Mock function for _Mkdir."""

  def Fstat(self, path):
    """Mock function for _Fstat."""
    # Convert relative paths to absolute
    if not os.path.isabs(path):
      path = os.path.join(self.user_home, path)

    if path == os.path.join(self.user_home, "some_dir"):
      if self.remote_exists and self.remote_is_dir:
        return {
            "exists": True,
            "path": path,
            "is_dir": True,
            "mode": 0o755,
            "size": 4096,
            "mtime": "2023-01-01T00:00:00Z",
        }
      elif self.remote_exists and not self.remote_is_dir:
        return {
            "exists": True,
            "path": path,
            "is_dir": False,
            "mode": 0o644,
            "size": 1024,
            "mtime": "2023-01-01T00:00:00Z",
        }
      else:
        return {"exists": False}
    elif path == os.path.join(self.user_home, "single_file"):
      return {
          "exists": True,
          "path": path,
          "is_dir": False,
          "mode": 0o644,
          "size": 1024,
          "mtime": "2023-01-01T00:00:00Z",
      }
    return {"exists": False}

  def LocalLsTree(self, path):
    """Mock function for _LocalLsTree using the pseudo file system."""
    # Convert to absolute path if relative
    if not os.path.isabs(path):
      path = os.path.join(self.mock_root, path)

    base_name = os.path.basename(path)
    if base_name == "some_dir":
      return self._build_file_entries(os.path.dirname(path),
                                      {"some_dir": self.pseudo_fs["some_dir"]})
    elif base_name == "single_file":
      fs = {"single_file": self.pseudo_fs["single_file"]}
      return self._build_file_entries(os.path.dirname(path), fs)
    return []

  def test_push_abs_dir_to_abs_dir_remote_exists(self):
    """Test push absolute src dir to absolute remote dir
    when remote dir exists."""
    self.remote_exists = True
    self.remote_is_dir = True

    dst = "/home/user/some_dir"
    src_path = os.path.join(self.mock_root, "some_dir")
    args = argparse.Namespace(srcs=[src_path], dst=dst)

    # Mock file system checks
    with (
        patch("os.path.exists", return_value=True),
        patch("os.access", return_value=True),
        patch(
            "os.path.isdir",
            lambda path: (path == src_path or "some_dir" in path),
        ),
    ):

      self.cli.Push(args)

      self.assertEqual(
          sorted(self.results),
          sorted([
              (
                  os.path.join(src_path, "a/b"),
                  os.path.join(dst, "some_dir/a/b"),
              ),
              (
                  os.path.join(src_path, "a/c/d"),
                  os.path.join(dst, "some_dir/a/c/d"),
              ),
          ]),
      )

  def test_push_abs_dir_to_abs_dir_remote_exists_not_dir(self):
    """Test push absolute src dir to absolute remote path
    when remote exists but is not a directory."""
    self.remote_exists = True
    self.remote_is_dir = False

    dst = "/home/user/some_dir"
    src_path = os.path.join(self.mock_root, "some_dir")
    args = argparse.Namespace(srcs=[src_path], dst=dst)

    # Mock file system checks
    with (
        patch("os.path.exists", return_value=True),
        patch("os.access", return_value=True),
        patch(
            "os.path.isdir",
            lambda path: (path == src_path or "some_dir" in path),
        ),
    ):

      self.assertRaises(RuntimeError, self.cli.Push, args)

  def test_push_abs_dir_to_abs_dir_remote_not_exists(self):
    """Test push absolute src dir to absolute remote dir
    when remote dir does not exist."""
    self.remote_exists = False
    self.remote_is_dir = False

    dst = "/home/user/some_dir"
    src_path = os.path.join(self.mock_root, "some_dir")
    args = argparse.Namespace(srcs=[src_path], dst=dst)

    # Mock file system checks
    with (
        patch("os.path.exists", return_value=True),
        patch("os.access", return_value=True),
        patch(
            "os.path.isdir",
            lambda path: (path == src_path or "some_dir" in path),
        ),
    ):

      self.cli.Push(args)

      self.assertEqual(
          sorted(self.results),
          sorted([
              (
                  os.path.join(src_path, "a/b"),
                  os.path.join(dst, "a/b"),
              ),
              (
                  os.path.join(src_path, "a/c/d"),
                  os.path.join(dst, "a/c/d"),
              ),
          ]),
      )

  def test_push_rel_dir_to_rel_dir_remote_exists(self):
    """Test push relative src dir to relative remote dir
    when remote dir exists."""
    self.remote_exists = True
    self.remote_is_dir = True

    # Mock os.chdir instead of actually changing directories
    dst = "some_dir"
    rel_src = "some_dir"
    args = argparse.Namespace(srcs=[rel_src], dst=dst)

    def mock_abspath(path):
      if path == rel_src:
        return os.path.join(self.mock_root, path)
      if path == dst:
        return os.path.join(self.user_home, path)
      return path

    # Mock file system checks
    with (
        patch("os.path.exists", return_value=True),
        patch("os.access", return_value=True),
        patch("os.path.isdir", lambda path: "some_dir" in path),
        patch("os.path.abspath", mock_abspath),
        patch("os.chdir"),
    ):

      self.cli.Push(args)

      # The test expects relative paths in the results
      self.assertEqual(
          sorted(self.results),
          sorted([
              (
                  os.path.join(self.mock_root, "some_dir/a/b"),
                  os.path.join("some_dir", "some_dir/a/b"),
              ),
              (
                  os.path.join(self.mock_root, "some_dir/a/c/d"),
                  os.path.join("some_dir", "some_dir/a/c/d"),
              ),
          ]),
      )

  def test_push_rel_dir_to_rel_dir_remote_not_exists(self):
    """Test push relative src dir to relative remote dir
    when remote dir does not exist."""
    self.remote_exists = False
    self.remote_is_dir = False

    # Mock os.chdir instead of actually changing directories
    dst = "some_dir"
    rel_src = "some_dir"
    args = argparse.Namespace(srcs=[rel_src], dst=dst)

    def mock_abspath(path):
      if path == rel_src:
        return os.path.join(self.mock_root, path)
      if path == dst:
        return os.path.join(self.user_home, path)
      return path

    # Mock file system checks
    with (
        patch("os.path.exists", return_value=True),
        patch("os.access", return_value=True),
        patch("os.path.isdir", lambda path: "some_dir" in path),
        patch("os.path.abspath", mock_abspath),
        patch("os.chdir"),
    ):

      self.cli.Push(args)

      # The test expects relative paths in the results
      self.assertEqual(
          sorted(self.results),
          sorted([
              (
                  os.path.join(self.mock_root, "some_dir/a/b"),
                  os.path.join("some_dir", "a/b"),
              ),
              (
                  os.path.join(self.mock_root, "some_dir/a/c/d"),
                  os.path.join("some_dir", "a/c/d"),
              ),
          ]),
      )

  def test_push_abs_dir_to_rel_dir_remote_exists(self):
    """Test push absolute src dir to relative remote dir
    when remote dir exists."""
    self.remote_exists = True
    self.remote_is_dir = True

    dst = "some_dir"
    src_path = os.path.join(self.mock_root, "some_dir")
    args = argparse.Namespace(srcs=[src_path], dst=dst)

    def mock_abspath(path):
      if os.path.isabs(path):
        return path
      return os.path.join(self.user_home, path)

    # Mock file system checks
    with (
        patch("os.path.exists", return_value=True),
        patch("os.access", return_value=True),
        patch(
            "os.path.isdir",
            lambda path: (path == src_path or "some_dir" in path),
        ),
        patch("os.path.abspath", mock_abspath),
    ):

      self.cli.Push(args)

      self.assertEqual(
          sorted(self.results),
          sorted([
              (
                  os.path.join(src_path, "a/b"),
                  os.path.join(dst, "some_dir/a/b"),
              ),
              (
                  os.path.join(src_path, "a/c/d"),
                  os.path.join(dst, "some_dir/a/c/d"),
              ),
          ]),
      )

  def test_push_rel_dir_to_abs_dir_remote_exists(self):
    """Test push relative src dir to absolute remote dir
    when remote dir exists."""
    self.remote_exists = True
    self.remote_is_dir = True

    # Mock os.chdir instead of actually changing directories
    dst = "/home/user/some_dir"
    rel_src = "some_dir"
    args = argparse.Namespace(srcs=[rel_src], dst=dst)

    def mock_abspath(path):
      if path == rel_src:
        return os.path.join(self.mock_root, path)
      return path

    # Mock file system checks
    with (
        patch("os.path.exists", return_value=True),
        patch("os.access", return_value=True),
        patch("os.path.isdir", lambda path: "some_dir" in path),
        patch("os.path.abspath", mock_abspath),
        patch("os.chdir"),
    ):

      self.cli.Push(args)

      self.assertEqual(
          sorted(self.results),
          sorted([
              (
                  os.path.join(self.mock_root, "some_dir/a/b"),
                  os.path.join(dst, "some_dir/a/b"),
              ),
              (
                  os.path.join(self.mock_root, "some_dir/a/c/d"),
                  os.path.join(dst, "some_dir/a/c/d"),
              ),
          ]),
      )


class PullUnittest(FileSystemTestCase):

  """Test cases for pull command."""

  def setUp(self):
    super().setUp()
    self.cli.CheckClient = lambda: None
    self.cli._PullSingle = self.PullSingle
    self.cli._LsTree = self.LsTree

  def PullSingle(self, entry, dst):
    """Mock function for _Pull."""
    self.results.append((entry.path, dst))

  def LsTree(self, path):
    """Mock function for _LsTree using the pseudo file system."""
    # Convert relative paths to absolute
    if not os.path.isabs(path):
      path = os.path.join(self.remote_root, path)

    if path == os.path.join(self.remote_root, "single_file"):
      return [FileEntry(path=path, is_dir=False)]
    elif path == os.path.join(self.remote_root, "some_dir"):
      # Create a list to hold all entries
      entries = []

      entries.append(FileEntry(path=path, is_dir=True))

      entries.append(FileEntry(path=os.path.join(path, "a/b"), is_dir=False))

      entries.append(FileEntry(path=os.path.join(path, "a"), is_dir=True))

      entries.append(FileEntry(path=os.path.join(path, "a/c"), is_dir=True))

      entries.append(FileEntry(path=os.path.join(path, "a/c/d"), is_dir=False))
      return entries
    else:
      raise RuntimeError("ls: No such file or directory")

  def test_pull_abs_file_to_abs_file(self):
    """Test pulling an absolute file path to an absolute file path."""
    src = "/remote/single_file"
    dst = "/absolute/local_file"
    args = argparse.Namespace(src=src, dst=dst)

    # Add the patches for os.chmod and os.makedirs
    with patch("os.chmod"), patch("os.makedirs"):
      self.cli.Pull(args)

    self.assertEqual(self.results, [(src, dst)])

  def test_pull_abs_file_to_rel_file(self):
    """Test pulling an absolute file path to a relative file path."""
    args = argparse.Namespace()
    args.src = "/remote/single_file"
    args.dst = "local_file"

    # Add the patches for os.chmod and os.makedirs
    with patch("os.chmod"), patch("os.makedirs"):
      self.cli.Pull(args)

    # The destination path should be converted to absolute
    self.assertEqual(
        self.results,
        [(args.src, os.path.join(self.user_home, args.dst))],
    )

  def test_pull_rel_file_to_abs_file(self):
    """Test pulling a relative file path to an absolute file path."""
    args = argparse.Namespace()
    args.src = "single_file"
    args.dst = "/absolute/local_file"

    # Add the patches for os.chmod and os.makedirs
    with patch("os.chmod"), patch("os.makedirs"):
      self.cli.Pull(args)

    # The source path should be converted to absolute
    self.assertEqual(
        self.results,
        [(os.path.join(self.remote_root, args.src), args.dst)],
    )

  def test_pull_rel_file_to_rel_file(self):
    """Test pulling a relative file path to a relative file path."""
    args = argparse.Namespace()
    args.src = "single_file"
    args.dst = "local_file"

    # Add the patches for os.chmod and os.makedirs
    with patch("os.chmod"), patch("os.makedirs"):
      self.cli.Pull(args)

    # Both paths should be converted to absolute
    self.assertEqual(
        self.results,
        [(
            os.path.join(self.remote_root, args.src),
            os.path.join(self.user_home, args.dst),
        )],
    )

  def test_pull_abs_file_to_abs_dir_exists(self):
    """Test pulling an absolute file path to an absolute
    directory that exists."""
    args = argparse.Namespace()
    args.src = "/remote/single_file"
    args.dst = "/absolute/local_dir"

    # Add the patches for os.chmod and os.makedirs
    with patch("os.chmod"), patch("os.makedirs"), patch(
        "os.path.exists", lambda path: path == args.dst):
      self.cli.Pull(args)

    self.assertEqual(
        self.results,
        [(args.src, os.path.join(args.dst, "single_file"))],
    )

  def test_pull_abs_dir_to_abs_dir_not_exists(self):
    """Test pulling an absolute directory path to an
    absolute directory that does not exist."""
    args = argparse.Namespace()
    args.src = "/remote/some_dir"
    args.dst = "/absolute/local_dir"

    # Add the patches for os.chmod and os.makedirs
    with (
        patch("os.chmod"),
        patch("os.makedirs"),
        patch("os.path.exists", return_value=False),
    ):
      self.cli.Pull(args)

    self.assertEqual(
        sorted(self.results),
        sorted([
            (
                os.path.join(args.src, "a/b"),
                os.path.join(args.dst, "a/b"),
            ),
            (
                os.path.join(args.src, "a/c/d"),
                os.path.join(args.dst, "a/c/d"),
            ),
        ]),
    )

  def test_pull_abs_dir_to_abs_dir_exists(self):
    """Test pulling an absolute directory path to an
    absolute directory that exists."""
    args = argparse.Namespace()
    args.src = "/remote/some_dir"
    args.dst = "/absolute/local_dir"

    # Add the patches for os.chmod and os.makedirs
    with patch("os.chmod"), patch("os.makedirs"), patch(
        "os.path.exists", lambda path: path == args.dst):
      self.cli.Pull(args)

    self.assertEqual(
        sorted(self.results),
        sorted([
            (
                os.path.join(args.src, "a/b"),
                os.path.join(args.dst, "some_dir/a/b"),
            ),
            (
                os.path.join(args.src, "a/c/d"),
                os.path.join(args.dst, "some_dir/a/c/d"),
            ),
        ]),
    )

  def test_pull_abs_dir_to_rel_dir_not_exists(self):
    """Test pulling an absolute directory path to a
    relative directory that does not exist."""
    args = argparse.Namespace()
    args.src = "/remote/some_dir"
    args.dst = "local_dir"

    def mock_abspath(path):
      if not path.startswith("/"):
        return os.path.join(self.user_home, path)
      return path

    # Add the patches for os.chmod and os.makedirs
    with (
        patch("os.chmod"),
        patch("os.makedirs"),
        patch("os.path.exists", return_value=False),
        patch("os.path.abspath", mock_abspath),
    ):
      self.cli.Pull(args)

    self.assertEqual(
        sorted(self.results),
        sorted([
            (
                os.path.join(args.src, "a/b"),
                os.path.join(self.user_home, args.dst, "a/b"),
            ),
            (
                os.path.join(args.src, "a/c/d"),
                os.path.join(self.user_home, args.dst, "a/c/d"),
            ),
        ]),
    )

  def test_pull_rel_dir_to_abs_dir_not_exists(self):
    """Test pulling a relative directory path to an
    absolute directory that does not exist."""
    args = argparse.Namespace()
    args.src = "some_dir"
    args.dst = "/absolute/local_dir"

    # Add the patches for os.chmod and os.makedirs
    with (
        patch("os.chmod"),
        patch("os.makedirs"),
        patch("os.path.exists", return_value=False),
    ):
      self.cli.Pull(args)

    self.assertEqual(
        sorted(self.results),
        sorted([
            (
                os.path.join(self.remote_root, args.src, "a/b"),
                os.path.join(args.dst, "a/b"),
            ),
            (
                os.path.join(self.remote_root, args.src, "a/c/d"),
                os.path.join(args.dst, "a/c/d"),
            ),
        ]),
    )

  def test_pull_rel_dir_to_rel_dir_not_exists(self):
    """Test pulling a relative directory path to a
    relative directory that does not exist."""
    args = argparse.Namespace()
    args.src = "some_dir"
    args.dst = "local_dir"

    def mock_abspath(path):
      if not path.startswith("/"):
        return os.path.join(self.user_home, path)
      return path

    # Add the patches for os.chmod and os.makedirs
    with (
        patch("os.chmod"),
        patch("os.makedirs"),
        patch("os.path.exists", return_value=False),
        patch("os.path.abspath", mock_abspath),
    ):
      self.cli.Pull(args)

    self.assertEqual(
        sorted(self.results),
        sorted([
            (
                os.path.join(self.remote_root, args.src, "a/b"),
                os.path.join(self.user_home, args.dst, "a/b"),
            ),
            (
                os.path.join(self.remote_root, args.src, "a/c/d"),
                os.path.join(self.user_home, args.dst, "a/c/d"),
            ),
        ]),
    )

  def test_pull_rel_dir_to_abs_dir_exists(self):
    """Test pulling a relative directory path to an
    absolute directory that exists."""
    args = argparse.Namespace()
    args.src = "some_dir"
    args.dst = "/absolute/local_dir"

    # Add the patches for os.chmod and os.makedirs
    with patch("os.chmod"), patch("os.makedirs"), patch(
        "os.path.exists", lambda path: path == args.dst):
      self.cli.Pull(args)

    self.assertEqual(
        sorted(self.results),
        sorted([
            (
                os.path.join(self.remote_root, args.src, "a/b"),
                os.path.join(args.dst, "some_dir/a/b"),
            ),
            (
                os.path.join(self.remote_root, args.src, "a/c/d"),
                os.path.join(args.dst, "some_dir/a/c/d"),
            ),
        ]),
    )

  def test_pull_rel_dir_to_rel_dir_exists(self):
    """Test pulling a relative directory path to a
    relative directory that exists."""
    args = argparse.Namespace()
    args.src = "some_dir"
    args.dst = "local_dir"

    def mock_exists(path):
      return path == os.path.join(self.user_home, args.dst)

    def mock_abspath(path):
      if not path.startswith("/"):
        return os.path.join(self.user_home, path)
      return path

    # Add the patches for os.chmod and os.makedirs
    with (
        patch("os.chmod"),
        patch("os.makedirs"),
        patch("os.path.exists", mock_exists),
        patch("os.path.abspath", mock_abspath),
    ):
      self.cli.Pull(args)

    self.assertEqual(
        sorted(self.results),
        sorted([
            (
                os.path.join(self.remote_root, args.src, "a/b"),
                os.path.join(self.user_home, args.dst, "some_dir/a/b"),
            ),
            (
                os.path.join(self.remote_root, args.src, "a/c/d"),
                os.path.join(self.user_home, args.dst, "some_dir/a/c/d"),
            ),
        ]),
    )


if __name__ == "__main__":
  unittest.main()
