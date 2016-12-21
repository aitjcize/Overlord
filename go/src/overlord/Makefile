# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

GO=go
FACTORY=$(CURDIR)/../../..
BUILDDIR=$(FACTORY)/build
PACKAGEDIR=$(BUILDDIR)/overlord
PACKAGE_OUTPUT_PATH?=$(BUILDDIR)/overlord.tar.gz
BINDIR=$(FACTORY)/go/bin
GOPATH=$(FACTORY)/build/go:$(FACTORY)/go
OVERLORD=overlord
DEPS?=true
STATIC?=false
LDFLAGS=

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
	rm -rf $(BINDIR)/app
	ln -s $(CURDIR)/app $(BINDIR)

ghost: deps
	(cd $(BINDIR) && GOPATH=$(GOPATH) $(GO) build $(LDFLAGS) \
		$(OVERLORD)/cmd/$@)

package: overlordd ghost
	rm -rf $(PACKAGEDIR)
	mkdir $(PACKAGEDIR)
	cp -r app $(PACKAGEDIR)
	cp -r $(foreach target,$^,$(BINDIR)/$(target)) $(PACKAGEDIR)
	cp setup.sh $(PACKAGEDIR)
	tar -C $(BUILDDIR) -zcf $(PACKAGE_OUTPUT_PATH) overlord
	@echo "Please find the tarball at $(PACKAGE_OUTPUT_PATH)"

clean:
	rm -rf $(FACTORY)/go/pkg $(FACTORY)/go/bin
