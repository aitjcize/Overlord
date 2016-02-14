// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

// Overlord server ports.
const (
	OverlordPort     = 4455 // Socket server port
	OverlordLDPort   = 4456 // LAN discovery port
	OverlordHTTPPort = 9000 // Overlord HTTP server port
)

const (
	pingTimeout = 10
)

// ConnServer Client mode
const (
	ModeNone = iota
	ModeControl
	ModeTerminal
	ModeShell
	ModeLogcat
	ModeFile
	ModeForward
)

// Logcat format
const (
	logcatTypeText = iota
	logcatTypeVT100
)

// RPC states
const (
	Success = "success"
	Failed  = "failed"
)

// Stream control
const (
	StdinClosed = "##STDIN_CLOSED##"
)

// ModeStr translate client mode to string.
func ModeStr(mode int) string {
	return map[int]string{
		ModeNone:     "None",
		ModeControl:  "Agent",
		ModeTerminal: "Terminal",
		ModeShell:    "Shell",
		ModeLogcat:   "Logcat",
		ModeFile:     "File",
		ModeForward:  "ModeForward",
	}[mode]
}
