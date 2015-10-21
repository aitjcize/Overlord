// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

const (
	DEBUG = false
)

const (
	OVERLORD_PORT      = 4455 // Socket server port
	OVERLORD_LD_PORT   = 4456 // LAN discovery port
	OVERLORD_HTTP_PORT = 9000 // Overlord HTTP server port
)

// ConnServer Client mode
const (
	NONE = iota
	AGENT
	TERMINAL
	SHELL
	LOGCAT
	FILE
	FORWARD
)

// Logcat format
const (
	TEXT = iota
	TERM
)

const (
	SUCCESS      = "success"
	FAILED       = "failed"
	DISCONNECTED = "disconnected"
)

// Terminal resize control
const (
	CONTROL_NONE  = 255 // Control State None
	CONTROL_START = 128 // Control Start Code
	CONTROL_END   = 129 // Control End Code
)

func ModeStr(mode int) string {
	return map[int]string{
		NONE:     "None",
		AGENT:    "Agent",
		TERMINAL: "Terminal",
		SHELL:    "Shell",
		LOGCAT:   "Logcat",
		FILE:     "File",
		FORWARD:  "Forward",
	}[mode]
}
