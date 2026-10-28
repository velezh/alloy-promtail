## Build targets of alloy-promtail, the log collection distribution of Grafana Alloy.
## Run from the repository root as `make -C promtail <target>`.
##
## Targets:
##   build        Build $(BINARY) for GOOS/GOARCH.
##   check        Run build, deps, test, e2e and size.
##   overlay      Generate the go build overlay from overlay/manifest.txt.
##   deps         Fail when a denylisted package or a module missing from modules.txt is linked.
##   test         Run unit tests of promtail/ with the overlay.
##   e2e          Run the end-to-end test against $(BINARY) (host platform only).
##   size         Print the binary size.
##   image        Build the container image $(IMAGE).
##   patch-start  Apply the overlay patch of FILE to the working tree for editing.
##   patch-save   Store the working tree changes of FILE as its overlay patch and restore FILE.
##   clean        Remove $(BUILD_DIR).
##
## Variables:
##   COMPONENT_GROUPS  Optional component groups: k8s cloud secretfilter.
##   GOOS, GOARCH      Target platform (default: host).
##   CGO_ENABLED       Cgo is required by loki.source.journal (default 1).
##   GO                Go command used for the target build (e.g. xx-go).
##   VERSION           Version reported by the binary (default: git describe).
##   BUILD_DIR         Build output directory (default build/promtail).
##   BINARY            Output path (default $(BUILD_DIR)/alloy-promtail).
##   IMAGE             Image reference for `make image`.
##   FILE              Upstream file for patch-start/patch-save.
##   CONTEXT           Diff context lines for patch-save (default 3).

SHELL       := bash
.SHELLFLAGS := -o pipefail -ec

ROOT_DIR     := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/..)
PROMTAIL_DIR := $(ROOT_DIR)/promtail

# Relative BUILD_DIR and BINARY are resolved against the repository root.
root_path = $(abspath $(if $(filter /%,$(1)),$(1),$(ROOT_DIR)/$(1)))

BUILD_DIR    ?= $(ROOT_DIR)/build/promtail
override BUILD_DIR := $(call root_path,$(BUILD_DIR))
# `clean` removes BUILD_DIR: it must not contain the repository or the fork sources.
ifneq ($(filter $(BUILD_DIR:%/=%)/%,$(ROOT_DIR)/ $(PROMTAIL_DIR)/)$(filter $(PROMTAIL_DIR)/%,$(BUILD_DIR)/),)
$(error BUILD_DIR must be a dedicated output directory, got "$(BUILD_DIR)")
endif
OVERLAY_DIR  := $(BUILD_DIR)/overlay
OVERLAY      := $(OVERLAY_DIR)/overlay.json
BINARY       ?= $(BUILD_DIR)/alloy-promtail
override BINARY := $(call root_path,$(BINARY))
MAIN_PKG     := ./promtail/cmd/alloy-promtail
IMAGE        ?= alloy-promtail:dev

KNOWN_GROUPS     := $(filter-out base,$(basename $(notdir $(wildcard $(PROMTAIL_DIR)/components/*.go))))
COMPONENT_GROUPS ?=
ifneq ($(filter-out $(KNOWN_GROUPS),$(COMPONENT_GROUPS)),)
$(error unknown COMPONENT_GROUPS "$(filter-out $(KNOWN_GROUPS),$(COMPONENT_GROUPS))", known groups: $(KNOWN_GROUPS))
endif
GO               ?= go
CGO_ENABLED      ?= 1
GOOS             ?= $(shell go env GOOS)
GOARCH           ?= $(shell go env GOARCH)
GO_TAGS          := netgo gore2regex promtail_journal_enabled $(addprefix promtail_,$(COMPONENT_GROUPS))

# Tools run on the build host even when the target build is cross-compiled.
GO_HOST := env -u GOOS -u GOARCH -u GOARM CGO_ENABLED=0 go

# Ref names may contain shell metacharacters; derived values end up in recipes.
GIT          := git -C $(ROOT_DIR)
SAFE         := tr -c 'A-Za-z0-9._/+@\n-' '_'
VERSION      ?= $(shell $(GIT) describe --tags --always --dirty --match 'v[0-9]*' 2>/dev/null | $(SAFE))
GIT_REVISION ?= $(shell $(GIT) rev-parse --short HEAD 2>/dev/null)
GIT_BRANCH   ?= $(shell $(GIT) rev-parse --abbrev-ref HEAD 2>/dev/null | $(SAFE))
BUILD_USER   ?= $(shell printf '%s@%s' "$$(whoami)" "$$(hostname)" | $(SAFE))
BUILD_DATE   ?= $(shell TZ=UTC $(GIT) log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ 2>/dev/null)

VPREFIX       := github.com/grafana/alloy/internal/build
VPREFIXSYNTAX := github.com/grafana/alloy/syntax/internal/stdlib
GO_LDFLAGS    := -s -w                                 \
                 -X $(VPREFIX).Branch=$(GIT_BRANCH)     \
                 -X $(VPREFIX).Version=$(VERSION)       \
                 -X $(VPREFIXSYNTAX).Version=$(VERSION) \
                 -X $(VPREFIX).Revision=$(GIT_REVISION) \
                 -X $(VPREFIX).BuildUser=$(BUILD_USER)  \
                 -X $(VPREFIX).BuildDate=$(BUILD_DATE)
GO_FLAGS      := -overlay=$(OVERLAY) -trimpath -tags "$(GO_TAGS)"
TARGET_ENV    := GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED)

.DEFAULT_GOAL := build

.PHONY: help
help:
	@sed -n 's/^##//p' $(lastword $(MAKEFILE_LIST))

.PHONY: overlay
overlay:
	@cd $(ROOT_DIR) && $(GO_HOST) run ./promtail/tools/overlay -root . -dir promtail/overlay -out $(OVERLAY_DIR)

.PHONY: build
build: overlay
	cd $(ROOT_DIR) && $(TARGET_ENV) $(GO) build $(GO_FLAGS) -ldflags "$(GO_LDFLAGS)" -o $(BINARY) $(MAIN_PKG)

.PHONY: deps
deps: overlay
	@cd $(ROOT_DIR) && $(TARGET_ENV) $(GO) list $(GO_FLAGS) -deps -f '{{.ImportPath}}{{with .Module}} @{{.Path}}{{end}}{{range .Imports}} {{.}}{{end}}' $(MAIN_PKG) \
		| $(GO_HOST) run ./promtail/tools/depcheck -denylist promtail/denylist.txt -modules promtail/modules.txt \
			-groups "$(COMPONENT_GROUPS)" -root github.com/grafana/alloy/promtail/cmd/alloy-promtail

.PHONY: test
test: overlay
	cd $(ROOT_DIR) && CGO_ENABLED=$(CGO_ENABLED) go test $(GO_FLAGS) ./promtail/...

.PHONY: e2e
e2e: build
	cd $(ROOT_DIR) && ALLOY_PROMTAIL_BINARY=$(BINARY) go test -count=1 ./promtail/test/e2e/...

.PHONY: check
check: build deps test e2e
	@$(MAKE) --no-print-directory size

.PHONY: size
size:
	@ls -l $(BINARY) | awk '{ printf "%s: %d bytes (%.1f MiB)\n", "$(notdir $(BINARY))", $$5, $$5 / 1048576 }'

.PHONY: image
image:
	docker buildx build --load -t $(IMAGE) -f $(PROMTAIL_DIR)/Dockerfile \
		--build-arg 'VERSION=$(VERSION)' --build-arg 'GIT_REVISION=$(GIT_REVISION)' \
		--build-arg 'GIT_BRANCH=$(GIT_BRANCH)' --build-arg 'BUILD_DATE=$(BUILD_DATE)' \
		--build-arg 'COMPONENT_GROUPS=$(COMPONENT_GROUPS)' $(ROOT_DIR)

.PHONY: patch-start
patch-start:
	@$(PROMTAIL_DIR)/scripts/patch.sh start "$(FILE)"

.PHONY: patch-save
patch-save:
	@CONTEXT=$(CONTEXT) $(PROMTAIL_DIR)/scripts/patch.sh save "$(FILE)"

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
