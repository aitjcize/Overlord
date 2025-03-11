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
DIST_APPS_DIR=$(WEBROOT_DIR)/apps
GO_DIRS=./overlord/... ./cmd/...
PY_FILES=$(shell find scripts -name "*.py")

# Supported architectures for ghost binary
GHOST_ARCHS=amd64 386 arm64 arm
GHOST_BINS=$(addprefix $(BIN)/ghost.linux., $(GHOST_ARCHS))

# Get list of apps with package.json
APP_DIRS=$(shell find apps -maxdepth 1 -mindepth 1 \
	   -type d -exec test -f '{}/package.json' \; -print | sed 's|apps/||')
APP_TARGETS=$(addprefix $(DIST_APPS_DIR)/,$(APP_DIRS))

# Output formatting
cmd_msg = @echo "  $(1)  $(2)"

ifeq ($(STATIC), true)
	LDFLAGS=-a -tags netgo -installsuffix netgo \
		-ldflags '-s -w -extldflags "-static"'
else
	LDFLAGS=-ldflags '-s -w'
endif

.PHONY: all \
	build build-go build-py build-apps \
	ghost ghost-all overlordd \
	go-fmt go-lint \
	py-lint py-format \
	clean clean-apps \
	install

all: build

build: build-go build-py build-apps

deps:
	@mkdir -p $(BIN)
	@if $(DEPS); then \
		cd $(CURDIR); \
		$(GO) get ./...; \
	fi

overlordd: deps
	$(call cmd_msg,GO,cmd/$@)
	@GOBIN=$(BIN) $(GO) install $(LDFLAGS) $(CURDIR)/cmd/$@
	@rm -f $(BIN)/webroot
	@ln -s $(WEBROOT_DIR) $(BIN)/webroot

ghost: deps
	$(call cmd_msg,GO,cmd/$@)
	@GOBIN=$(BIN) $(GO) install $(LDFLAGS) $(CURDIR)/cmd/$@

$(BIN)/ghost.linux.%.sha1: $(BIN)/ghost.linux.%
	$(call cmd_msg,SHA1,$(notdir $<))
	@cd $(BIN) && sha1sum $(notdir $<) | awk '{ print $$1 }' > $(notdir $@)

$(BIN)/ghost.linux.%:
	$(call cmd_msg,GO,$(notdir $@))
	@GOOS=linux GOARCH=$* $(GO) build $(LDFLAGS) -o $@ $(CURDIR)/cmd/ghost

ghost-all: $(GHOST_BINS) $(GHOST_BINS:=.sha1)

build-go: overlordd ghost ghost-all

build-py:
	@ln -sf ../scripts/ghost.py bin
	$(call cmd_msg,SHA1,ghost.py)
	@sha1sum scripts/ghost.py > bin/ghost.py.sha1

	@mkdir -p $(BUILD)
	$(call cmd_msg,VENV,creating virtualenv)
	@rm -rf $(BUILD)/.venv
	@python -m venv $(BUILD)/.venv
	$(call cmd_msg,PIP,installing requirements)
	@cd $(BUILD); \
	. $(BUILD)/.venv/bin/activate; \
	pip install -q -r $(CURDIR)/requirements.txt; \
	pip install -q pyinstaller

	$(call cmd_msg,GEN,ovl.pybin)
	@cd $(BUILD); . $(BUILD)/.venv/bin/activate; \
	pyinstaller --onefile $(CURDIR)/scripts/ovl.py > /dev/null;
	@mv $(BUILD)/dist/ovl $(BIN)/ovl.pybin
	$(call cmd_msg,SHA1,ovl.pybin)
	@cd $(BIN) && sha1sum ovl.pybin > ovl.pybin.sha1

	$(call cmd_msg,GEN,ghost.pybin)
	@cd $(BUILD); . $(BUILD)/.venv/bin/activate; \
	pyinstaller --onefile $(CURDIR)/scripts/ghost.py > /dev/null
	@mv $(BUILD)/dist/ghost $(BIN)/ghost.pybin
	$(call cmd_msg,SHA1,ghost.pybin)
	@cd $(BIN) && sha1sum ghost.pybin > ghost.pybin.sha1

go-fmt:
	$(call cmd_msg,FMT,$(GO_DIRS))
	@$(GO) fmt $(GO_DIRS)

go-lint:
	$(call cmd_msg,VET,$(GO_DIRS))
	@$(GO) vet $(GO_DIRS)
	$(call cmd_msg,GO,installing golint)
	@$(GO) install golang.org/x/lint/golint@latest
	$(call cmd_msg,LINT,$(GO_DIRS))
	@golint -set_exit_status $(GO_DIRS)

py-lint:
	$(call cmd_msg,PYLINT,$(PY_FILES))
	@pylint --rcfile=.pylintrc $(PY_FILES)

py-format:
	$(call cmd_msg,YAPF,$(PY_FILES))
	@yapf -i $(PY_FILES)

# Pattern rule for building individual apps
$(DIST_APPS_DIR)/%:
	$(call cmd_msg,NPM,$*)
	@mkdir -p $(DIST_APPS_DIR)
	@cd apps/$* && npm install --silent && npm run build --silent
	@cp -r apps/$*/dist $(DIST_APPS_DIR)/$*

build-apps: $(APP_TARGETS)
	@cp $(DIST_APPS_DIR)/dashboard/index.html $(WEBROOT_DIR)

clean-apps:
	$(call cmd_msg,RM,apps)
	@rm -rf $(DIST_APPS_DIR)
	@rm -rf $(WEBROOT_DIR)/index.html

clean: clean-apps
	$(call cmd_msg,RM,build artifacts)
	@rm -rf $(BIN)/ghost* $(BIN)/overlordd $(BUILD) \
		$(BIN)/ghost.pybin $(BIN)/ovl.pybin \
		$(DIST_APPS_DIR)
