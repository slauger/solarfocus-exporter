BINARY  := solarfocus-exporter
IMAGE   ?= ghcr.io/slauger/solarfocus-exporter
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO_LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test cover lint vet vulncheck fmt tidy image run ci help

help: ## Show targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

build: ## Build static binary to bin/
	CGO_ENABLED=0 go build -trimpath $(GO_LDFLAGS) -o bin/$(BINARY) ./cmd/$(BINARY)

test: ## Run tests with race detector
	go test -race ./...

cover: ## Tests with coverage threshold (80%)
	go test -race -coverprofile=cover.out ./...
	./hack/check-coverage.sh cover.out 80

lint: ## golangci-lint
	golangci-lint run

vet: ## go vet
	go vet ./...

vulncheck: ## govulncheck
	go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...

fmt: ## gofmt
	gofmt -s -w .

tidy: ## go mod tidy
	go mod tidy

image: ## Build container image (scratch)
	docker build -f Containerfile --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

run: build ## Run against SOLARFOCUS_MODBUS_ADDRESS
	./bin/$(BINARY)

ci: tidy vet lint vulncheck cover ## All checks
