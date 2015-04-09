# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

GO=go
FACTORY=$(CURDIR)/../../..
GOPATH=$(FACTORY)/build/go:$(FACTORY)/go
OVERLORD=overlord
DEPS?=true

all: ghost overlordd

deps:
	if $(DEPS); then \
		GOPATH=$(GOPATH) $(GO) get $(OVERLORD); \
	fi

overlordd: deps
	GOPATH=$(GOPATH) $(GO) install $(OVERLORD)/cmd/$@
	rm -rf $(FACTORY)/go/bin/app
	ln -s $(CURDIR)/app $(FACTORY)/go/bin

ghost: deps
	GOPATH=$(GOPATH) $(GO) install $(OVERLORD)/cmd/$@

clean:
	rm -rf $(FACTORY)/go/pkg $(FACTORY)/go/bin
