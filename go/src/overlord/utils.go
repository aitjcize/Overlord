// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// ToVTNewLine replace the newline character to VT100 newline control.
func ToVTNewLine(text string) string {
	return strings.Replace(text, "\n", "\r\n", -1)
}

// GetPlatformString returns machine platform string.
// Platform stream has the format of GOOS.GOARCH
func GetPlatformString() string {
	return fmt.Sprintf("%s.%s", runtime.GOOS, runtime.GOARCH)
}

// GetFileSha1 return the sha1sum of a file.
func GetFileSha1(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PollableProcess is a os.Process which supports the polling for it's status.
type PollableProcess os.Process

// Poll polls the process for it's execution status.
func (p *PollableProcess) Poll() (uint32, error) {
	var wstatus syscall.WaitStatus
	pid, err := syscall.Wait4(p.Pid, &wstatus, syscall.WNOHANG, nil)
	if err == nil && p.Pid == pid {
		return uint32(wstatus), nil
	}
	return 0, errors.New("Wait4 failed")
}

// GetenvInt parse an integer from environment variable, and return default
// value when error.
func GetenvInt(key string, defaultValue int) int {
	env := os.Getenv(key)
	value, err := strconv.Atoi(env)
	if err != nil {
		return defaultValue
	}
	return value
}
