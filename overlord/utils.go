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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// ToVTNewLine replace the newline character to VT100 newline control.
func ToVTNewLine(text string) string {
	return strings.ReplaceAll(text, "\n", "\r\n")
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

// getCurrentUser gets the current user name with multiple fallbacks
func getCurrentUser() string {
	if envUser := os.Getenv("USER"); envUser != "" {
		return envUser
	}

	if logName := os.Getenv("LOGNAME"); logName != "" {
		return logName
	}

	return UnknownStr
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

// installBinaryToUserLocal installs the ghost binary to ~/.local/bin
func installBinaryToUserLocal() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %v", err)
	}

	homeDir := getCurrentUserHomeDir()
	targetPath := filepath.Join(homeDir, ".local", "bin", "ghost")
	binDir := filepath.Join(homeDir, ".local", "bin")

	// Create the ~/.local/bin directory if it doesn't exist
	err = os.MkdirAll(binDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create ~/.local/bin directory: %v", err)
	}

	// Copy the binary
	srcFile, err := os.Open(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to create target file: %v", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return "", fmt.Errorf("failed to copy ghost binary: %v", err)
	}

	// Set executable permissions
	err = os.Chmod(targetPath, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to set executable permissions: %v", err)
	}

	fmt.Printf("Ghost binary installed to %s\n", targetPath)
	return targetPath, nil
}

// getUserShell returns the user's current shell, with fallback to /bin/bash
func getUserShell() string {
	// Try to get shell from environment variable first
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}

	// Ultimate fallback
	return "/bin/bash"
}

// getShellFromUserDB tries to get the user's shell from the user database
func getShellFromUserDB(username string) string {
	// Try using getent command (works on most Unix systems)
	cmd := exec.Command("getent", "passwd", username)
	output, err := cmd.Output()
	if err == nil {
		// Parse passwd entry: username:x:uid:gid:gecos:home:shell
		fields := strings.Split(strings.TrimSpace(string(output)), ":")
		if len(fields) >= 7 && fields[6] != "" {
			return fields[6]
		}
	}

	return ""
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
