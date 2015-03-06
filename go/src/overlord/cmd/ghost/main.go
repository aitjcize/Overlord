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

var noLanDisc = flag.Bool("no-lan-disc", false, "disable LAN discovery")
var propFile = flag.String("prop-file", "", "file containing the JSON representation of client properties")

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: ghost OVERLORD_ADDR\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	overlord.StartGhost(args, *noLanDisc, *propFile)
}
