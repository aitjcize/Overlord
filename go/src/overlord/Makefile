# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

GO=go
FACTORY=$(PWD)/../../..
GOPATH=$(FACTORY)/build/go:$(FACTORY)/go

all: ghost overlordd

overlord:
	GOPATH=$(GOPATH) $(GO) get $@

overlordd: overlord
	GOPATH=$(GOPATH) $(GO) install $</cmd/$@

ghost: overlord
	GOPATH=$(GOPATH) $(GO) install $</cmd/$@
