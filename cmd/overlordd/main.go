// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aitjcize/Overlord/overlord"
)

var bindAddr = flag.String("bind", "0.0.0.0", "specify alternate bind address")
var port = flag.Int("port", 0,
	"alternate port listen instead of standard ports (http:80, https:443)")
var lanDiscInterface = flag.String("lan-disc-iface", "",
	"the network interface used for broadcasting LAN discovery packets")
var noLanDisc = flag.Bool("no-lan-disc", false,
	"disable LAN discovery broadcasting")
var tlsCerts = flag.String("tls", "",
	"TLS certificates in the form of 'cert.pem,key.pem'. Empty to disable.")
var noLinkTLS = flag.Bool("no-link-tls", false,
	"disable TLS between ghost and overlord. Only valid when TLS is enabled.")
var htpasswdPath = flag.String("htpasswd-path", "overlord.htpasswd",
	"the path to the .htpasswd file. Required for authentication.")
var jwtSecretPath = flag.String("jwt-secret-path", "jwt-secret",
	"Path to the file containing the JWT secret. Required for authentication.")

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: overlordd [OPTIONS]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	// Validate required flags
	if *htpasswdPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -htpasswd-path is required\n")
		usage()
	}
	if *jwtSecretPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -jwt-secret-path is required\n")
		usage()
	}

	overlord.StartOverlord(*bindAddr, *port, *lanDiscInterface, !*noLanDisc,
		*tlsCerts, !*noLinkTLS, *htpasswdPath, *jwtSecretPath)
}
