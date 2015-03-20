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

var noAuth = flag.Bool("noauth", false, "disable authentication")
var enableTLS = flag.String("tls", "", `enable TLS. The argument should be in the form of "cert.pem,key.pem"`)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: overlord [OPTIONS]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	overlord.StartOverlord(*noAuth, *enableTLS)
}
