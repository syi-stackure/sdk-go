STATICCHECK_VERSION := 2025.1.1
GOLANGCI_LINT_VERSION := v2.1.6

GO := go
GOBIN := $(shell $(GO) env GOPATH)/bin

.PHONY: all build fmt fmt-check vet lint staticcheck staticcheck-install golangci-lint-install ci clean

all: ci

build:
	$(GO) build ./...

fmt:
	$(GO) fmt ./...

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

staticcheck-install:
	$(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

staticcheck: staticcheck-install
	$(GOBIN)/staticcheck ./...

golangci-lint-install:
	@which golangci-lint > /dev/null || $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint: golangci-lint-install
	golangci-lint run ./...

ci: fmt-check vet staticcheck lint build

clean:
	$(GO) clean ./...
