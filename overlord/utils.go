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
	"os/exec"
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

// NewPollableProcess creates a new PollableProcess from an os.Process.
func NewPollableProcess(p *os.Process) *PollableProcess {
	return &PollableProcess{Process: p}
}

// PollableProcess is a os.Process which supports the polling for it's status.
type PollableProcess struct {
	*os.Process
}

// Poll polls the process for it's execution status.
func (p *PollableProcess) Poll() (uint32, error) {
	var wstatus syscall.WaitStatus
	pid, err := syscall.Wait4(p.Process.Pid, &wstatus, syscall.WNOHANG, nil)
	if err == nil && p.Process.Pid == pid {
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

// runWithSudo executes a command with sudo privileges
func runWithSudo(args ...string) error {
	cmd := exec.Command("sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// cpWithSudo copies a file to a destination that requires sudo
func cpWithSudo(src, dst string) error {
	// Copy the file with sudo
	err := runWithSudo("cp", src, dst)
	if err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}
	return nil
}

// chmodWithSudo changes file permissions using sudo
func chmodWithSudo(mode, path string) error {
	err := runWithSudo("chmod", mode, path)
	if err != nil {
		return fmt.Errorf("failed to set permissions: %v", err)
	}
	return nil
}

// getCurrentUser gets the current user name with multiple fallbacks
func getCurrentUser() string {
	if envUser := os.Getenv("USER"); envUser != "" {
		return envUser
	}

	if logName := os.Getenv("LOGNAME"); logName != "" {
		return logName
	}

	return "unknown"
}

// isGhostRunning checks if ghost is running by calling the RPC GetStatus method
func isGhostRunning() bool {
	// isGhostRunning checks if the ghost service is already running by attempting an RPC connection
	client, err := ghostRPCStubServer()
	if err != nil {
		return false
	}
	defer client.Close()

	// Try to call GetStatus to verify the service is responsive
	var status string
	err = client.Call("GhostRPCStub.GetStatus", "", &status)
	return err == nil
}

// getServiceCommand constructs the command line arguments for the service
// by filtering out the --install flag from os.Args
func getServiceCommand() []string {
	var cmdParts []string

	// Skip program name (os.Args[0]) and filter out --install flag
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--install" || arg == "-install" {
			continue // Skip the install flag
		}
		cmdParts = append(cmdParts, arg)
	}

	return cmdParts
}
