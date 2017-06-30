// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"overlord"
)

var mid = flag.String("mid", "", "machine ID to set")
var randMid = flag.Bool("rand-mid", false, "use random machine ID")
var noLanDisc = flag.Bool("no-lan-disc", false, "disable LAN discovery")
var noRPCServer = flag.Bool("no-rpc-server", false, "disable RPC server")
var propFile = flag.String("prop-file", "",
	"file containing the JSON representation of client properties")
var tlsCertFile = flag.String("tls-cert-file", "",
	"file containing the server TLS certificate in PEM format")
var tlsNoVerify = flag.Bool("tls-no-verify", false,
	"do not verify certificate if TLS is enabled")
var tlsModeFlag = flag.String("tls", "detect",
	"specify 'y' or 'n' to force enable/disable TLS")
var download = flag.String("download", "", "file to download")
var reset = flag.Bool("reset", false, "reset ghost and reload all configs")
var status = flag.Bool("status", false, "show status of the client")

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: ghost OVERLORD_ADDR\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	var finalMid string

	if *randMid && *mid != "" {
		log.Fatalf("Conflict options. Both mid and rand-mid flag are assgined.")
	}

	if *randMid {
		finalMid = overlord.RandomMID
	} else {
		finalMid = *mid
	}

	tlsMode := overlord.TLSDetect
	if *tlsModeFlag == "detect" {
		tlsMode = overlord.TLSDetect
	} else if *tlsModeFlag == "y" {
		tlsMode = overlord.TLSForceEnable
	} else if *tlsModeFlag == "n" {
		tlsMode = overlord.TLSForceDisable
	}

	overlord.StartGhost(args, finalMid, *noLanDisc, *noRPCServer, *tlsCertFile,
		!*tlsNoVerify, *propFile, *download, *reset, *status, tlsMode)
}
