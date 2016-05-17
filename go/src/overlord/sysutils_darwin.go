// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	// #cgo LDFLAGS: -lproc
	// #include <libproc.h>
	"C"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// GetExecutablePath return the executable path of the current process.
func GetExecutablePath() (string, error) {
	buf := make([]byte, C.MAXPATHLEN*4)
	length := C.proc_pidpath(C.int(os.Getpid()), unsafe.Pointer(&buf[0]),
		C.uint32_t(C.MAXPATHLEN*4))

	if length == 0 {
		return "", fmt.Errorf("proc_pidpath returned %d", length)
	}
	return string(buf[:length]), nil
}
