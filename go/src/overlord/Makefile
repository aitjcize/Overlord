# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

GO ?= go
FACTORY = $(CURDIR)/../../..
override GOPATH := $(GOPATH):$(FACTORY)/build/go:$(FACTORY)/go
BINDIR = $(FACTORY)/go/bin
OVERLORD = overlord
DEPS ?= true
STATIC ?= false
LDFLAGS =

ifeq ($(STATIC), true)
	LDFLAGS=-a -tags netgo -installsuffix netgo
endif

all: ghost overlordd

deps:
	mkdir -p $(BINDIR)
	if $(DEPS); then \
		GOPATH=$(GOPATH) $(GO) get $(OVERLORD); \
	fi

overlordd: deps
	(cd $(BINDIR) && GOPATH=$(GOPATH) $(GO) build $(LDFLAGS) \
		$(OVERLORD)/cmd/$@)
	rm -rf $(FACTORY)/go/bin/app
	ln -s $(CURDIR)/app $(FACTORY)/go/bin

ghost: deps
	(cd $(BINDIR) && GOPATH=$(GOPATH) $(GO) build $(LDFLAGS) \
		$(OVERLORD)/cmd/$@)

clean:
	rm -rf $(FACTORY)/go/pkg $(FACTORY)/go/bin
