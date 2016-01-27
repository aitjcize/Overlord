// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	"C"
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

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

func TcGetAttr(fd uintptr) (*syscall.Termios, error) {
	var termios syscall.Termios
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS,
		uintptr(unsafe.Pointer(&termios))); err != 0 {
		return nil, err
	}
	return &termios, nil
}

func TcSetAttr(fd uintptr, termios *syscall.Termios) error {
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCSETS,
		uintptr(unsafe.Pointer(termios))); err != 0 {
		return err
	}
	return nil
}

func CfMakeRaw(termios *syscall.Termios) {
	termios.Iflag &^= (syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK |
		syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON)
	termios.Oflag &^= syscall.OPOST
	termios.Lflag &^= (syscall.ECHO | syscall.ECHONL | syscall.ICANON |
		syscall.ISIG | syscall.IEXTEN)
	termios.Cflag &^= (syscall.CSIZE | syscall.PARENB)
	termios.Cflag |= syscall.CS8
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

// A buffered net.TCPConn that supports UnRead.
//Allow putting back data back to the socket for the next Read() call.
type BufferedTCPConn struct {
	*net.TCPConn
	buf []byte
}

func NewBufferedTCPConn(self *net.TCPConn) *BufferedTCPConn {
	return &BufferedTCPConn{TCPConn: self}
}

func (self *BufferedTCPConn) UnRead(b []byte) {
	self.buf = append(b, self.buf...)
}

func (self *BufferedTCPConn) Read(b []byte) (n int, err error) {
	bufsize := len(b)

	if self.buf != nil {
		if len(self.buf) >= bufsize {
			copy(b, self.buf[:bufsize])
			self.buf = self.buf[bufsize:]
			return bufsize, nil
		} else {
			copy(b, self.buf)
			copied_size := len(self.buf)
			n, err := self.TCPConn.Read(b[copied_size:])
			self.buf = nil
			return n + copied_size, err
		}
	} else {
		return self.TCPConn.Read(b)
	}
}
