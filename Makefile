GO ?= go
BIN ?= sandbox-runner
DIST_DIR ?= dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
CURRENT_GOOS ?= $(shell $(GO) env GOOS)
CURRENT_GOARCH ?= $(shell $(GO) env GOARCH)
CURRENT_GOARM ?= $(shell $(GO) env GOARM)
SHA256 ?= shasum -a 256
BUILD_TAGS ?=

# 文档强制要求的最小发布矩阵 + 常见 Windows / ARM 变体。
PLATFORMS ?= darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 linux/arm/7 windows/amd64 windows/arm64

.DEFAULT_GOAL := build

platform_parts = $(subst /, ,$1)
platform_goos = $(word 1,$(call platform_parts,$1))
platform_goarch = $(word 2,$(call platform_parts,$1))
platform_goarm = $(word 3,$(call platform_parts,$1))
platform_suffix = $(call platform_goos,$1)-$(call platform_goarch,$1)$(if $(call platform_goarm,$1),-v$(call platform_goarm,$1),)
platform_ext = $(if $(filter windows,$(call platform_goos,$1)),.exe,)
platform_binary = $(DIST_DIR)/$(BIN)-$(call platform_suffix,$1)$(call platform_ext,$1)

define build_ldflags
-s -w \
-X 'github.com/lotosli/sandbox-runner/internal/cli.versionValue=$(VERSION)' \
-X 'github.com/lotosli/sandbox-runner/internal/cli.gitSHAValue=$(GIT_SHA)' \
-X 'github.com/lotosli/sandbox-runner/internal/cli.buildTimeValue=$(BUILD_TIME)' \
-X 'github.com/lotosli/sandbox-runner/internal/cli.buildTargetOS=$(1)' \
-X 'github.com/lotosli/sandbox-runner/internal/cli.buildTargetArch=$(2)$(if $(3),/v$(3),)'
endef

build_tags_flag = $(if $(strip $(BUILD_TAGS)),-tags "$(BUILD_TAGS)",)

LOCAL_BIN := $(BIN)$(if $(filter windows,$(CURRENT_GOOS)),.exe,)
DIST_BINARIES := $(foreach platform,$(PLATFORMS),$(call platform_binary,$(platform)))

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=$(CURRENT_GOOS) GOARCH=$(CURRENT_GOARCH)$(if $(CURRENT_GOARM), GOARM=$(CURRENT_GOARM),) \
	$(GO) build $(build_tags_flag) -trimpath -ldflags "$(call build_ldflags,$(CURRENT_GOOS),$(CURRENT_GOARCH),$(CURRENT_GOARM))" -o $(LOCAL_BIN) ./cmd/sandbox-runner

.PHONY: test
test:
	$(GO) test ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -f $(BIN) $(BIN).exe
	rm -rf $(DIST_DIR)

.PHONY: dist clean-dist checksums
dist: clean-dist $(DIST_BINARIES) checksums

clean-dist:
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)

checksums: $(DIST_BINARIES)
	cd $(DIST_DIR) && $(SHA256) $(notdir $(DIST_BINARIES)) > SHA256SUMS

define build_platform_target
$(call platform_binary,$1): | $(DIST_DIR)/.mkdir
	CGO_ENABLED=0 GOOS=$(call platform_goos,$1) GOARCH=$(call platform_goarch,$1)$(if $(call platform_goarm,$1), GOARM=$(call platform_goarm,$1),) \
	$(GO) build $(build_tags_flag) -trimpath -ldflags "$(call build_ldflags,$(call platform_goos,$1),$(call platform_goarch,$1),$(call platform_goarm,$1))" -o $$@ ./cmd/sandbox-runner
endef

$(foreach platform,$(PLATFORMS),$(eval $(call build_platform_target,$(platform))))

$(DIST_DIR)/.mkdir:
	mkdir -p $(DIST_DIR)
	touch $@
