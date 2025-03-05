# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

GO=go
BIN=$(CURDIR)/bin
BUILD=$(CURDIR)/build
DEPS?=true
STATIC?=false
LDFLAGS=
WEBROOT_DIR=$(CURDIR)/webroot
APPS_DIR=$(WEBROOT_DIR)/apps

ifeq ($(STATIC), true)
	LDFLAGS=-a -tags netgo -installsuffix netgo \
		-ldflags '-extldflags "-static"'
endif

.PHONY: all build build-bin build-apps clean clean-apps install

all: build

build: build-bin build-apps

deps:
	mkdir -p $(BIN)
	if $(DEPS); then \
		cd $(CURDIR)/overlord; \
		$(GO) get -d .; \
	fi

overlordd: deps
	GOBIN=$(BIN) $(GO) install $(LDFLAGS) $(CURDIR)/cmd/$@
	rm -f $(BIN)/webroot
	ln -s $(WEBROOT_DIR) $(BIN)/webroot

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
	pip install -r $(CURDIR)/requirements.txt; \
	pip install pyinstaller; \
	pyinstaller --onefile $(CURDIR)/scripts/ovl.py; \
	pyinstaller --onefile $(CURDIR)/scripts/ghost.py
	# Move built binary to bin
	mv $(BUILD)/dist/ovl $(BIN)/ovl.py.bin
	mv $(BUILD)/dist/ghost $(BIN)/ghost.py.bin

build-bin: overlordd ghost py-bin

# Build all apps that have a package.json
build-apps:
	@echo "Building apps..."
	@mkdir -p $(APPS_DIR)
	@cd apps && \
	for dir in */; do \
		if [ ! -f "$$dir/package.json" ]; then \
			continue; \
		fi; \
		echo "Building $$dir..."; \
		(cd "$$dir" && npm install && npm run build); \
		if [ -d "$$dir/dist" ]; then \
			echo "Copying $$dir dist to apps directory..."; \
			mkdir -p $(APPS_DIR)/"$${dir%/}"; \
			cp -r "$$dir/dist/"* $(APPS_DIR)/"$${dir%/}"/; \
			if [ "$$dir" = "dashboard/" ]; then \
				echo "Copying dashboard to webroot..."; \
				cp $(APPS_DIR)/dashboard/index.html \
					$(CURDIR)/webroot/index.html; \
			fi; \
		else \
			echo "Error: No dist directory found for $$dir"; \
			exit 1; \
		fi; \
	done

# Install the built apps to the system directory
install: build
	@echo "Installing apps..."
	mkdir -p /usr/local/share/overlord/apps
	cp -r $(APPS_DIR)/* /usr/local/share/overlord/apps/
	@echo "Installation complete"

clean-apps:
	rm -rf $(APPS_DIR)
	rm -rf $(WEBROOT_DIR)/index.html

clean: clean-apps
	rm -rf $(BIN)/ghost $(BIN)/overlordd $(BUILD) \
		$(BIN)/ghost.py.bin $(BIN)/ovl.py.bin \
		$(APPS_DIR)
