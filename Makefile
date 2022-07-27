# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

GO=go
BIN=$(CURDIR)/bin
BUILD=$(CURDIR)/build
DEPS?=true
STATIC?=false
LDFLAGS=

ifeq ($(STATIC), true)
	LDFLAGS=-a -tags netgo -installsuffix netgo \
		-ldflags '-extldflags "-static"'
endif

all: ghost overlordd

deps:
	mkdir -p $(BIN)
	if $(DEPS); then \
		cd $(CURDIR)/overlord; \
		$(GO) get -d .; \
	fi

overlordd: deps
	GOBIN=$(BIN) $(GO) install $(LDFLAGS) $(CURDIR)/cmd/$@
	rm -f $(BIN)/app
	ln -s $(CURDIR)/overlord/app $(BIN)

ghost: deps
	GOBIN=$(BIN) $(GO) install $(LDFLAGS) $(CURDIR)/cmd/$@

py-bin:
	mkdir -p $(BUILD)
	# Create virtualenv environment
	rm -rf $(BUILD)/.venv
	python -m venv $(BUILD)/.venv
	# Build ovl binary with pyinstaller
	cd $(BUILD); \
	. $(BUILD)/.venv/bin/activate; \
	pip install ws4py pyyaml pyinstaller; \
	pyinstaller --onefile $(CURDIR)/scripts/ovl.py; \
	pyinstaller --onefile $(CURDIR)/scripts/ghost.py
	# Move built binary to bin
	mv $(BUILD)/dist/ovl $(BIN)/ovl.py.bin
	mv $(BUILD)/dist/ghost $(BIN)/ghost.py.bin

clean:
	rm -rf $(BIN)/ghost $(BIN)/overlordd $(BUILD) \
		$(BIN)/ghost.py.bin $(BIN)/ovl.py.bin
