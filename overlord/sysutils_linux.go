// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	uuid "github.com/satori/go.uuid"
)

// GetGateWayIP return the IPs of the gateways.
func GetGateWayIP() ([]string, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var ips []string
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 {
			gatewayHex := fields[2]
			if gatewayHex != "00000000" {
				h, err := hex.DecodeString(gatewayHex)
				if err != nil {
					continue
				}
				ip := fmt.Sprintf("%d.%d.%d.%d", h[3], h[2], h[1], h[0])
				ips = append(ips, ip)
			}
		}
	}

	return ips, nil
}

// GetMachineID generates machine-dependent ID string for a machine.
// There are many ways to generate a machine ID:
// 1. /sys/class/dmi/id/product_uuid (only available on intel machines)
// 2. MAC address
// We follow the listed order to generate machine ID, and fallback to the next
// alternative if the previous doesn't work.
func GetMachineID() (string, error) {
	buf := make([]byte, 64)
	f, err := os.Open("/sys/class/dmi/id/product_uuid")
	if err == nil {
		if n, err := f.Read(buf); err == nil {
			return strings.TrimSpace(string(buf[:n])), nil
		}
	}

	interfaces, err := os.ReadDir("/sys/class/net")
	if err == nil {
		mid := ""
		for _, iface := range interfaces {
			if iface.Name() == "lo" {
				continue
			}
			addrPath := fmt.Sprintf("/sys/class/net/%s/address", iface.Name())
			f, err := os.Open(addrPath)
			if err != nil {
				break
			}
			if n, err := f.Read(buf); err == nil {
				mid += strings.TrimSpace(string(buf[:n])) + ";"
			}
		}
		mid = strings.Trim(mid, ";")

		if mid == "" {
			mid = uuid.NewV4().String()
		}
		return mid, nil
	}

	// Fallback to a random UUID.
	return uuid.NewV4().String(), nil
}

// GetProcessWorkingDirectory returns the current working directory of a process.
func GetProcessWorkingDirectory(pid int) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
}

// Ttyname returns the TTY name of a given file descriptor.
func Ttyname(fd uintptr) (string, error) {
	// Get the process ID
	pid := os.Getpid()

	// Try to read the symlink for the file descriptor
	ttyPath, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%d", pid, fd))
	if err != nil {
		return "", fmt.Errorf("failed to get tty name: %v", err)
	}

	return ttyPath, nil
}

// getCurrentUserHomeDir gets the home directory for the current user on Linux
func getCurrentUserHomeDir() string {
	// Fallback to HOME environment variable
	if homeDir := os.Getenv("HOME"); homeDir != "" {
		return homeDir
	}

	// Linux-specific fallback using /etc/passwd
	if username := getCurrentUser(); username != "unknown" {
		// Try to read from /etc/passwd
		if file, err := os.Open("/etc/passwd"); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				fields := strings.Split(line, ":")
				if len(fields) >= 6 && fields[0] == username {
					return fields[5] // Home directory is the 6th field
				}
			}
		}
	}

	return "/home/" + getCurrentUser()
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

// Install installs and configures the ghost service for automatic startup on Linux
func Install() error {
	// Install binary to filesystem
	targetPath, err := installBinaryToUserLocal()
	if err != nil {
		return fmt.Errorf("failed to install binary: %v", err)
	}

	homeDir := getCurrentUserHomeDir()
	currentUser := getCurrentUser()

	// Install service
	cmdParts := getServiceCommand()
	cmdArgs := strings.Join(cmdParts, " ")

	serviceContent := fmt.Sprintf(`[Unit]
Description=Overlord Ghost Client
After=network-online.target local-fs.target
Wants=network-online.target
RequiresMountsFor=%s

[Service]
Type=simple
User=%s
Environment=SHELL=/bin/bash
Environment=HOME=%s
Environment=TERM=xterm-256color
ExecStart=%s %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, homeDir, currentUser, homeDir, targetPath, cmdArgs)

	serviceFilePath, err := getSystemdServicePath()
	if err != nil {
		return fmt.Errorf("failed to determine systemd directory: %v", err)
	}

	tempServiceFile, err := os.CreateTemp("", "ghost-*.service")
	if err != nil {
		return fmt.Errorf("failed to create temp service file: %v", err)
	}
	defer os.Remove(tempServiceFile.Name())

	_, err = tempServiceFile.WriteString(serviceContent)
	if err != nil {
		return fmt.Errorf("failed to write temp service file: %v", err)
	}
	tempServiceFile.Close()

	err = cpWithSudo(tempServiceFile.Name(), serviceFilePath)
	if err != nil {
		return fmt.Errorf("failed to install systemd service file: %v", err)
	}

	fmt.Printf("Systemd service installed at %s\n", serviceFilePath)

	// Reload systemd daemon
	err = runWithSudo("systemctl", "daemon-reload")
	if err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}
	fmt.Printf("Systemd daemon reloaded\n")

	// Enable service
	err = runWithSudo("systemctl", "enable", "ghost")
	if err != nil {
		return fmt.Errorf("failed to enable ghost service: %v", err)
	}
	fmt.Printf("Ghost service enabled for automatic startup\n")

	// Start service
	if !isGhostRunning() {
		fmt.Printf("Starting ghost service...\n")
		err = runWithSudo("systemctl", "start", "ghost")
		if err != nil {
			fmt.Printf("Warning: failed to start ghost service: %v\n", err)
			fmt.Printf("You can start it manually with: sudo systemctl start ghost\n")
		} else {
			fmt.Printf("Ghost service started successfully\n")
		}
	} else {
		fmt.Printf("Ghost service is already running\n")
	}

	fmt.Printf("Ghost service installation completed successfully!\n")
	fmt.Printf("The ghost service will start automatically on system boot.\n")
	fmt.Printf("To check service status, run: sudo systemctl status ghost\n")

	return nil
}

// getSystemdServicePath determines the best systemd directory for service installation
func getSystemdServicePath() (string, error) {
	// Systemd service directories in order of preference:
	// 1. /etc/systemd/system - Local configuration (highest priority, for admin-installed services)
	// 2. /usr/lib/systemd/system - Package manager installed services (RHEL/CentOS/Fedora)
	// 3. /lib/systemd/system - Package manager installed services (Debian/Ubuntu)

	serviceDirs := []string{
		"/etc/systemd/system",
		"/usr/lib/systemd/system",
		"/lib/systemd/system",
	}

	// Try to find an existing systemd directory
	for _, dir := range serviceDirs {
		if _, err := os.Stat(dir); err == nil {
			return filepath.Join(dir, "ghost.service"), nil
		}
	}

	// If no existing directory found, try to create /etc/systemd/system (preferred for admin installs)
	preferredDir := "/etc/systemd/system"
	err := runWithSudo("mkdir", "-p", preferredDir)
	if err != nil {
		return "", fmt.Errorf("failed to create systemd directory %s: %v", preferredDir, err)
	}

	return filepath.Join(preferredDir, "ghost.service"), nil
}
