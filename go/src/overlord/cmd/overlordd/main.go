// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"overlord"
)

var lanDiscInterface = flag.String("lan-disc-iface", "",
	"the network interface used for broadcasting LAN discovery packets")
var noLanDisc = flag.Bool("no-lan-disc", false,
	"disable LAN discovery broadcasting")
var noAuth = flag.Bool("noauth", false, "disable authentication")
var tlsCerts = flag.String("tls", "",
	"TLS certificates in the form of 'cert.pem,key.pem'. Empty to disable.")
var disableLinkTLS = flag.Bool("disable-link-tls", false,
	"disable TLS between ghost and overlord. Only valid when TLS is enabled.")

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: overlordd [OPTIONS]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	overlord.StartOverlord(*lanDiscInterface, *noLanDisc, *noAuth,
		*tlsCerts, *disableLinkTLS)
}
