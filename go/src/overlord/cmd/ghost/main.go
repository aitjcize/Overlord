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
var enableTLSWithoutVerify = flag.Bool("enable-tls-without-verify", false,
	"Enable TLS but don't verify certificate")
var download = flag.String("download", "", "file to download")
var reset = flag.Bool("reset", false, "reset ghost and reload all configs")

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
		finalMid = overlord.RANDOM_MID
	} else {
		finalMid = *mid
	}

	overlord.StartGhost(args, finalMid, *noLanDisc, *noRPCServer, *tlsCertFile,
	  *enableTLSWithoutVerify, *propFile, *download, *reset)
}
