// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

import (
	// #cgo LDFLAGS: -lc
	// #include <unistd.h>
	"C"
	"errors"
)

// Ttyname retuns the TTY name of a given file descriptor.
func Ttyname(fd uintptr) (string, error) {
	var ttyname *C.char
	ttyname = C.ttyname(C.int(fd))
	if ttyname == nil {
		return "", errors.New("ttyname returned NULL")
	}
	return C.GoString(ttyname), nil
}
