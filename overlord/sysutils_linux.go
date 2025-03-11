// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
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
