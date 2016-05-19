#!/bin/bash
#
# Copyright 2016 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

SCRIPT_DIR="$(dirname $0)"

cd $SCRIPT_DIR

echo 'Installing noVNC ...'
git clone https://github.com/kanaka/noVNC
