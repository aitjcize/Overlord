// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
)

func ToVTNewLine(text string) string {
	return strings.Replace(text, "\n", "\r\n", -1)
}

// Return machine architecture string.
// For ARM platform, return armvX, where X is ARM version.
func GetArchString() string {
	if runtime.GOARCH == "arm" {
		return fmt.Sprintf("armv%s", os.Getenv("GOARM"))
	}
	return runtime.GOARCH
}

// Return machine platform string.
// Platform stream has the format of GOOS.GOARCH
func GetPlatformString() string {
	return fmt.Sprintf("%s.%s", runtime.GOOS, GetArchString())
}

func GetFileSha1(filename string) (string, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return "", err
	}

	var buffer bytes.Buffer
	_, err = buffer.ReadFrom(fd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", sha1.Sum(buffer.Bytes())), nil
}

type PollableProcess os.Process

func (p *PollableProcess) Poll() (uint32, error) {
	var wstatus syscall.WaitStatus
	pid, err := syscall.Wait4(p.Pid, &wstatus, syscall.WNOHANG, nil)
	if err == nil && p.Pid == pid {
		return uint32(wstatus), nil
	}
	return 0, errors.New("Wait4 failed")
}
