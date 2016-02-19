// Copyright 2016 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetMachineID(t *testing.T) {
	mid, err := GetMachineID()
	if err != nil {
		t.Fatal(err)
	}

	if mid == "" {
		t.Fatal("Machine ID is empty")
	}
}

func TestGetProcessWorkingDirectory(t *testing.T) {
	testPath := filepath.Join(os.TempDir(), "a/b/c")

	if err := os.MkdirAll(testPath, 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(testPath); err != nil {
		t.Fatal(err)
	}

	wd, err := GetProcessWorkingDirectory(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}

	testPath, err = filepath.EvalSymlinks(testPath)
	if err != nil {
		t.Fatal(err)
	}

	if wd != testPath {
		t.Fatalf("Working directory differs: (%s, %s)", testPath, wd)
	}

	os.RemoveAll(filepath.Join(os.TempDir(), "a"))
}

func TestGetExecutablePath(t *testing.T) {
	path, err := GetExecutablePath()
	if err != nil {
		t.Fatal(err)
	}

	ans, err := filepath.EvalSymlinks(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}

	if ans != path {
		t.Fatalf("Executable path differs: (%s, %s)", ans, path)
	}
}
