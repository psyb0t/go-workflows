APP_NAME := go-workflows
PROJECT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
DEV_IMAGE ?= $(APP_NAME)-dev
UI_IMAGE ?= $(APP_NAME)-ui-dev
UI_RUN := docker run --rm --init --user $(shell id -u):$(shell id -g) --cap-drop ALL \
	--security-opt no-new-privileges:true --memory 2g --cpus 2 --pids-limit 256 \
	-v $(PROJECT_DIR):/work -w /work/diag/app $(UI_IMAGE)
RUN_UID := $(shell id -u)
RUN_GID := $(shell id -g)
# Unit-only tool containers do not mount the Docker socket.
DOCKER_GID := $(shell stat -c '%g' /var/run/docker.sock 2>/dev/null)
PKG ?= ./...
GOFLAGS ?= -mod=vendor
export PKG GOFLAGS RELEASE_TAG
MODULE_DIR ?= .
export MODULE_DIR
TEST_TIMEOUT ?= 10m
TEST_COUNT ?= 1
MIN_TEST_COVERAGE ?= 62
MIN_ANALYZER_COVERAGE ?= 90
export TEST_TIMEOUT TEST_COUNT MIN_TEST_COVERAGE MIN_ANALYZER_COVERAGE

DEV_ENV := --user $(RUN_UID):$(RUN_GID) --cap-drop ALL \
	--security-opt no-new-privileges:true --memory 8g --cpus 8 --pids-limit 512 \
	-e HOME=/tmp -e CGO_ENABLED=1 -e GOTOOLCHAIN=local \
	-e GOPATH=/tmp/go -e GOCACHE=/tmp/go-cache -e PKG -e GOFLAGS \
	-e MIN_TEST_COVERAGE -e MIN_ANALYZER_COVERAGE -e TEST_TIMEOUT -e TEST_COUNT -e MODULE_DIR \
	-e RELEASE_TAG -e GITHUB_REF_NAME
DEV_CACHE := -v $(PROJECT_DIR)/.cache/go:/tmp/go -v $(PROJECT_DIR)/.cache/build:/tmp/go-cache \
	-v $(PROJECT_DIR)/.cache/trivy:/tmp/trivy-cache -e TRIVY_CACHE_DIR=/tmp/trivy-cache
DEV_RUN := docker run --rm --init $(DEV_ENV) $(DEV_CACHE) \
	-v $(PROJECT_DIR):/work -w /work $(DEV_IMAGE)
DEV_RUN_DIND := docker run --rm --init $(DEV_ENV) $(DEV_CACHE) \
	--group-add $(DOCKER_GID) \
	--network host -e TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1 \
	-v /var/run/docker.sock:/var/run/docker.sock \
	-v $(PROJECT_DIR):$(PROJECT_DIR) -w $(PROJECT_DIR) $(DEV_IMAGE)

.PHONY: help all dev-image shell dep pkg-lock pkg-add pkg-add-tool pkg-update pkg-upgrade pkg-remove \
	format fmt lint lint-fix build generate ui-image ui-lock build-ui test test-unit test-integration test-coverage \
	test-analyzer test-redis test-mysql test-postgres test-sqlite test-monoprocess test-backends sec audit run analyzer-plugin clean check-release-tag

help: ## List supported project commands
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_ -]+:.*## / {n=split($$1, names, " "); for (i=1; i<=n; i++) printf "%-24s %s\n", names[i], $$2}' $(MAKEFILE_LIST)

all: lint build test ## Lint, build and run all tests

check-release-tag: dev-image ## Validate RELEASE_TAG as SemVer for its Go module
	$(DEV_RUN) bash scripts/check-release-tag.sh

dev-image: ## Build the pinned development toolchain
	@mkdir -p .cache/go .cache/build .cache/trivy
	docker build -f Dockerfile.dev -t $(DEV_IMAGE) $(PROJECT_DIR)

shell: dev-image ## Open a development shell without Docker socket access
	$(DEV_RUN) bash

dep: pkg-lock ## Synchronize module checksums and vendored dependencies

pkg-lock: dev-image ## Tidy and vendor existing pins, optionally MODULE_DIR=analyzer
	$(DEV_RUN) bash scripts/deps.sh lock

pkg-add: dev-image ## Add a pinned dependency, PKG=module@version
	$(DEV_RUN) bash scripts/deps.sh add

pkg-add-tool: dev-image ## Add a pinned Go tool, PKG=module/cmd/tool@version
	$(DEV_RUN) bash scripts/deps.sh tool

pkg-update: dev-image ## Update one dependency after its release-age check, PKG=module
	$(DEV_RUN) bash scripts/deps.sh update

pkg-upgrade: dev-image ## Resolve and age-check every direct dependency before updating
	$(DEV_RUN) bash scripts/deps.sh upgrade

pkg-remove: dev-image ## Remove a dependency, PKG=module
	$(DEV_RUN) bash scripts/deps.sh remove

format: dev-image ## Format first-party Go code in both modules
	$(DEV_RUN) bash scripts/checks.sh format

fmt: format ## Alias for format

lint: dev-image ## Lint both modules with locked Go tools
	$(DEV_RUN) bash scripts/checks.sh lint

lint-fix: dev-image ## Apply formatter and linter fixes to first-party source
	$(DEV_RUN) bash scripts/checks.sh lint-fix

build: dev-image build-ui ## Compile the diagnostics UI, library, examples and analyzer
	$(DEV_RUN) bash scripts/checks.sh build

ui-image: ## Build the pinned diagnostics UI toolchain
	docker build -f Dockerfile.ui -t $(UI_IMAGE) $(PROJECT_DIR)

ui-lock: ui-image ## Resolve diagnostics dependencies under the release-age gate
	$(UI_RUN) pnpm install --lockfile-only --ignore-scripts

build-ui: ui-image ## Typecheck and rebuild the embedded diagnostics UI
	$(UI_RUN) pnpm install --frozen-lockfile --ignore-scripts
	$(UI_RUN) pnpm build

generate: dev-image ## Regenerate declared package-local artifacts
	$(DEV_RUN) bash scripts/checks.sh generate

test: dev-image ## Run the complete race-enabled suite against isolated real backends
	$(DEV_RUN_DIND) bash scripts/checks.sh test

test-unit: dev-image ## Run race-enabled unit tests without external backend services
	$(DEV_RUN) bash scripts/checks.sh unit

test-integration: test ## Run the complete integration suite without silent skips

test-analyzer: dev-image ## Test the nested analyzer module with the race detector
	$(DEV_RUN) bash scripts/checks.sh analyzer

analyzer-plugin: dev-image ## Build the custom golangci-lint binary with the workflow analyzer
	$(DEV_RUN) bash -c 'binary=$$(go tool -n golangci-lint); GOFLAGS=-mod=mod "$$binary" custom'

test-coverage: dev-image ## Measure unit and real-backend coverage and enforce the threshold
	$(DEV_RUN_DIND) bash scripts/test-coverage.sh

test-redis test-mysql test-postgres test-sqlite test-monoprocess: dev-image ## Test one real backend
	$(DEV_RUN_DIND) go test -mod=vendor -race -count=1 -timeout=$(TEST_TIMEOUT) ./backend/$(patsubst test-%,%,$@)

test-backends: test ## Run all backend contracts and end-to-end scenarios

sec: dev-image ## Run dependency and source security checks and emit SARIF
	$(DEV_RUN) bash scripts/sec.sh

audit: sec ## Alias for source and dependency security checks

run: dev-image ## Run the persistent SQLite quick start and print result 42
	$(DEV_RUN) go run ./samples/quickstart

clean: ## Remove only generated coverage and scanner reports
	@$(RM) coverage.txt coverage-analyzer.txt coverage-percent.txt sec.sarif

