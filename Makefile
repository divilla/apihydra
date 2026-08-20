PKG := "apih"
PKG_LIST := $(shell go list ${PKG}/... | grep -Ev '^apih/skeleton(/|$$)')

.PHONY: check init lint vet test tooling-test integration-test coverage race benchmark help test_version

.DEFAULT_GOAL := check
check: lint vet race tooling-test integration-test ## Check project

init:
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install golang.org/x/lint/golint@latest
	@go install honnef.co/go/tools/cmd/staticcheck@latest

lint: ## Lint the files
	@set -e; for package in ${PKG_LIST}; do \
		package_dir="$$(go list -f '{{.Dir}}' "$$package")"; \
		goimports -w "$$package_dir"; \
	done
	@if [ -n "${PKG_LIST}" ]; then staticcheck ${PKG_LIST}; fi
	@if [ -n "${PKG_LIST}" ]; then golint ${PKG_LIST}; fi

vet: ## Vet the files
	@if [ -n "${PKG_LIST}" ]; then go vet ${PKG_LIST}; fi

test: tooling-test ## Run tests
	@if [ -n "${PKG_LIST}" ]; then go test -short ${PKG_LIST}; fi

integration-test: ## Run black-box CLI integration tests with production coverage
	@go test -tags=integration ./int-tests -count=1

tooling-test:
	@scripts/makefile_test.sh
	@scripts/codex-review-loop_unit_test.pl
	@scripts/codex-review-loop_test.sh

coverage: ## Display test coverage
	@if [ -n "${PKG_LIST}" ]; then go test -cover ${PKG_LIST}; fi

race: ## Run tests with data race detector
	@if [ -n "${PKG_LIST}" ]; then go test -race ${PKG_LIST}; fi

benchmark: ## Run benchmarks
	@if [ -n "${PKG_LIST}" ]; then go test -run="-" -benchmem -bench=".*" ${PKG_LIST}; fi

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

goversion ?= "1.25"
test_version: ## Run tests inside Docker with given version (defaults to 1.25 oldest supported). Example: make test_version goversion=1.25
	@docker run --rm -it -v $(shell pwd):/project golang:$(goversion) /bin/sh -c "cd /project && make init check"
