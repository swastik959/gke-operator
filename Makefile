TARGETS := $(shell ls scripts)

ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BIN_DIR := $(abspath $(ROOT_DIR)/bin)
GO_INSTALL = ./scripts/go_install.sh

MOCKGEN_VER := v1.6.0
MOCKGEN_BIN := mockgen
MOCKGEN := $(BIN_DIR)/$(MOCKGEN_BIN)-$(MOCKGEN_VER)
MOCKGEN_PKG := github.com/golang/mock/mockgen

$(MOCKGEN):
	GOBIN=$(BIN_DIR) $(GO_INSTALL) $(MOCKGEN_PKG) $(MOCKGEN_BIN) $(MOCKGEN_VER)

.dapper:
	@echo Downloading dapper
	@curl -sL https://releases.rancher.com/dapper/latest/dapper-`uname -s`-`uname -m` > .dapper.tmp
	@@chmod +x .dapper.tmp
	@./.dapper.tmp -v
	@mv .dapper.tmp .dapper

.PHONY: $(TARGETS)
$(TARGETS): .dapper
	./.dapper $@

.PHONY: generate-go
generate-go: $(MOCKGEN)
	go generate ./pkg/gke/...

.PHONY: generate-crd
generate-crd: $(MOCKGEN)
	go generate main.go

.PHONY: generate
generate:
	$(MAKE) generate-go
	$(MAKE) generate-crd

ALL_VERIFY_CHECKS = generate

.PHONY: verify
verify: $(addprefix verify-,$(ALL_VERIFY_CHECKS))

.PHONY: verify-generate
verify-generate: generate
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

.PHONY: clean
clean:
	rm -rf build bin dist