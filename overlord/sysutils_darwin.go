// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	// #cgo LDFLAGS: -lproc
	// #include <libproc.h>
	// #include <unistd.h>
	"C"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"unsafe"
)

// GetGateWayIP return the IPs of the gateways.
func GetGateWayIP() ([]string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err == nil {
		re := regexp.MustCompile("gateway: (.*)")
		ret := re.FindStringSubmatch(string(out))
		if len(ret) == 2 {
			return ret[1:], nil
		}
	}
	return nil, err
}

// GetMachineID generates machine-dependent ID string for a machine.
// All Darwin system should have the IOPlatformSerialNumber attribute.
func GetMachineID() (string, error) {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err == nil {
		re := regexp.MustCompile("\"IOPlatformSerialNumber\" = \"(.*)\"")
		ret := re.FindStringSubmatch(string(out))
		if len(ret) == 2 {
			return ret[1], nil
		}
	}
	return "", errors.New("can't generate machine ID")
}

// GetProcessWorkingDirectory returns the current working directory of a process.
func GetProcessWorkingDirectory(pid int) (string, error) {
	const (
		procVnodepathinfoSize = 2352
		vidPathOffset         = 152
	)

	buf := make([]byte, procVnodepathinfoSize)
	ret := C.proc_pidinfo(C.int(pid), C.int(C.PROC_PIDVNODEPATHINFO),
		C.uint64_t(0), unsafe.Pointer(&buf[0]), C.int(procVnodepathinfoSize))
	if ret == 0 {
		return "", fmt.Errorf("proc_pidinfo returned %d", ret)
	}
	buf = buf[vidPathOffset : vidPathOffset+C.MAXPATHLEN]
	n := bytes.Index(buf, []byte{0})

	return string(buf[:n]), nil
}

// Ttyname returns the TTY name of a given file descriptor.
func Ttyname(fd uintptr) (string, error) {
	var ttyname *C.char
	ttyname = C.ttyname(C.int(fd))
	if ttyname == nil {
		return "", errors.New("ttyname returned NULL")
	}
	return C.GoString(ttyname), nil
}

// getCurrentUserHomeDir gets the home directory for the current user on macOS
func getCurrentUserHomeDir() string {
	// Fallback to HOME environment variable
	if homeDir := os.Getenv("HOME"); homeDir != "" {
		return homeDir
	}

	// macOS-specific fallback using dscl (Directory Service Command Line)
	if username := getCurrentUser(); username != "unknown" {
		out, err := exec.Command("dscl", ".", "-read", "/Users/"+username, "NFSHomeDirectory").Output()
		if err == nil {
			re := regexp.MustCompile("NFSHomeDirectory: (.*)")
			ret := re.FindStringSubmatch(string(out))
			if len(ret) == 2 {
				return ret[1]
			}
		}
	}

	// Ultimate fallback for macOS
	return "/Users/" + getCurrentUser()
}

// Install installs and configures the ghost service for automatic startup on macOS
func Install() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	homeDir := getCurrentUserHomeDir()
	targetPath := filepath.Join(homeDir, ".local", "bin", "ghost")
	binDir := filepath.Join(homeDir, ".local", "bin")

	err = os.MkdirAll(binDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create ~/.local/bin directory: %v", err)
	}

	srcFile, err := os.Open(execPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %v", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy ghost binary: %v", err)
	}

	err = os.Chmod(targetPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to set executable permissions: %v", err)
	}

	cmdParts := getServiceCommand()
	var programArgs []string
	programArgs = append(programArgs, targetPath)
	programArgs = append(programArgs, cmdParts...)

	argsXML := ""
	for _, arg := range programArgs {
		if arg != "" { // Skip empty strings
			argsXML += fmt.Sprintf("\t\t<string>%s</string>\n", arg)
		}
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.overlord.ghost</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>SHELL</key>
		<string>/bin/bash</string>
		<key>HOME</key>
		<string>%s</string>
		<key>TERM</key>
		<string>xterm-256color</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/var/log/ghost.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/ghost.log</string>
</dict>
</plist>
`, argsXML, homeDir)

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	plistFilePath := filepath.Join(launchAgentsDir, "com.overlord.ghost.plist")
	err = os.MkdirAll(launchAgentsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %v", err)
	}

	plistFile, err := os.Create(plistFilePath)
	if err != nil {
		return fmt.Errorf("failed to create plist file: %v", err)
	}
	defer plistFile.Close()

	_, err = plistFile.WriteString(plistContent)
	if err != nil {
		return fmt.Errorf("failed to write plist file: %v", err)
	}

	fmt.Printf("Launchd service installed at %s\n", plistFilePath)

	cmd := exec.Command("launchctl", "load", plistFilePath)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to load ghost service: %v", err)
	}
	fmt.Printf("Ghost service loaded and enabled for automatic startup\n")

	if !isGhostRunning() {
		fmt.Printf("Starting ghost service...\n")
		cmd = exec.Command("launchctl", "start", "com.overlord.ghost")
		err = cmd.Run()
		if err != nil {
			fmt.Printf("Warning: failed to start ghost service: %v\n", err)
			fmt.Printf("The service should start automatically on next login\n")
		} else {
			fmt.Printf("Ghost service started successfully\n")
		}
	} else {
		fmt.Printf("Ghost service is already running\n")
	}

	fmt.Printf("Ghost service installation completed successfully!\n")
	fmt.Printf("The ghost service will start automatically on user login.\n")
	fmt.Printf("To check if service is loaded, run: launchctl list | grep ghost\n")
	fmt.Printf("To unload the service, run: launchctl unload %s\n", plistFilePath)

	return nil
}
