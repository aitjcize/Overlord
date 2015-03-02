#!/bin/sh
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Overlord LAN discovery port
OVERLORD_LD_PORT=4456

/sbin/iptables -A INPUT -p udp --dport ${OVERLORD_LD_PORT} -j ACCEPT
