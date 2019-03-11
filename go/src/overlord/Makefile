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

all: ghost overlordd overlord_lan_disc

deps:
	mkdir -p $(BINDIR)
	if $(DEPS); then \
		GO111MODULE=on GOPATH=$(GOPATH) $(GO) get $(OVERLORD); \
	fi

overlordd: deps
	(GO111MODULE=on GOPATH=$(GOPATH) $(GO) build $(LDFLAGS) \
		$(OVERLORD)/cmd/$@)
	mv -f $@ $(BINDIR)
	ln -sf $(CURDIR)/app $(BINDIR)

overlord_lan_disc: deps
	(GO111MODULE=on GOPATH=$(GOPATH) $(GO) build $(LDFLAGS) \
		$(OVERLORD)/cmd/$@)
	mv -f $@ $(BINDIR)

ghost: deps
	(GO111MODULE=on GOPATH=$(GOPATH) $(GO) build $(LDFLAGS) \
		$(OVERLORD)/cmd/$@)
	mv -f $@ $(BINDIR)

package: overlordd overlord_lan_disc ghost
	rm -rf $(PACKAGEDIR)
	mkdir $(PACKAGEDIR)
	cp -r app $(PACKAGEDIR)
	cp -r $(foreach target,$^,$(BINDIR)/$(target)) $(PACKAGEDIR)
	cp setup.sh $(PACKAGEDIR)
	tar -C $(BUILDDIR) -zcf $(PACKAGE_OUTPUT_PATH) overlord
	@echo "Please find the tarball at $(PACKAGE_OUTPUT_PATH)"

clean:
	rm -rf $(FACTORY)/go/pkg $(FACTORY)/go/bin
