// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

const (
	DEBUG = false
)

const (
	OVERLORD_PORT    = 4455 // Socket server port
	OVERLORD_LD_PORT = 4456 // LAN discovery port
)

// ConnServer Client mode
const (
	NONE = iota
	AGENT
	TERMINAL
	LOGCAT
	SLOGCAT
)

// Logcat format
const (
	TEXT = iota
	TERM
)

const (
	SUCCESS = "success"
	FAILED  = "failed"
)

func ModeStr(mode int) string {
	return map[int]string{
		NONE:     "None",
		AGENT:    "Agent",
		TERMINAL: "Terminal",
		LOGCAT:   "Logcat",
		SLOGCAT:  "Simple-Logcat",
	}[mode]
}
