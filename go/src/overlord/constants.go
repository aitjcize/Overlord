// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

const (
	DEBUG = false
)

const (
	OVERLORD_PORT         = 4455  // Socket server port
	OVERLORD_LD_PORT      = 4456  // LAN discovery port
	OVERLORD_HTTP_PORT    = 9000  // Overlord HTTP server port
	TARGET_SSH_PORT_START = 50000 // First port for SSH forwarding
	TARGET_SSH_PORT_END   = 55000 // Last port for SSH forwarding
)

// ConnServer Client mode
const (
	NONE = iota
	AGENT
	TERMINAL
	SHELL
	LOGCAT
	FILE
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
	}[mode]
}
