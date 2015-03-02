#!/bin/sh
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

FACTORY_BASE="/usr/local/factory"

${FACTORY_BASE}/bin/ghost > /dev/null 2>&1 &
