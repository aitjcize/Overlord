// Copyright 2017 The Chromium OS Authors. All rights reserved.
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

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: overlord_lan_disc [OPTIONS]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	ovl := overlord.NewOverlord(*lanDiscInterface, true, true, "", true, "")
	ovl.StartUDPBroadcast(overlord.OverlordLDPort)
}
