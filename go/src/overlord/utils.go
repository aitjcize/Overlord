// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"C"
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

func B64Encode(buffer string) []byte {
	n := len(buffer)
	out := make([]byte, base64.StdEncoding.EncodedLen(n))
	base64.StdEncoding.Encode(out, []byte(buffer))
	return out
}

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

	interfaces, err := ioutil.ReadDir("/sys/class/net")
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
		return mid, nil
	}
	return "", errors.New("can't generate machine ID")
}

func ToVTNewLine(text string) string {
	return strings.Replace(text, "\n", "\r\n", -1)
}

func GetExecutablePath() (string, error) {
	path, err := os.Readlink("/proc/self/exe")
	return path, err
}

// Return the PtsName for a given tty master file descriptor.
func PtsName(f *os.File) (string, error) {
	var n C.uint
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGPTN,
		uintptr(unsafe.Pointer(&n)))
	if err != 0 {
		return "", err
	}
	return "/dev/pts/" + strconv.Itoa(int(n)), nil
}

// Return the TTY name of a given file descriptor.
func TtyName(f *os.File) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), f.Fd()))
}

// Return machine architecture string.
// For ARM platform, return armvX, where X is ARM version.
func GetArchString() string {
	if runtime.GOARCH == "arm" {
		return fmt.Sprintf("armv%s", os.Getenv("GOARM"))
	}
	return runtime.GOARCH
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
