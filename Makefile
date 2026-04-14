.PHONY: build test lint run-example clean tools check release-dry release-check

BUILD_DIR  := ./bin
BINARY     := $(BUILD_DIR)/ruptor
COVERAGE   := coverage.out

## build: compile the binary
build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BINARY) ./cmd/ruptor
	@echo "✓ Built $(BINARY)"

## test: run all tests with race detector and coverage
test:
	go test ./... -race -cover -coverprofile=$(COVERAGE)
	@go tool cover -func=$(COVERAGE) | tail -1

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## check: build + test + vet + lint (run before every commit)
check: build test lint
	go vet ./...
	@echo "✓ All checks passed"

## run-example: run chaos example
run-example: build
	@mkdir -p ./reports
	$(BINARY) run configs/chaos.example.yaml --output ./reports/example.html

## run-simulate-example: run simulate example
run-simulate-example: build
	@mkdir -p ./reports
	$(BINARY) simulate configs/simulate.example.yaml --output ./reports/simulate.html

## clean: remove build artifacts
clean:
	rm -rf $(BUILD_DIR) ./reports $(COVERAGE)
	@echo "✓ Cleaned"

## tools: install dev tools
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/goreleaser/goreleaser/v2@latest
	@echo "Install cosign separately per your OS: https://docs.sigstore.dev/cosign/installation/"

## release-check: validate the goreleaser config without building anything
release-check:
	goreleaser check

## release-dry: build all release artifacts locally, skip publish and cosign
release-dry:
	goreleaser release --snapshot --clean --skip=publish,sign

## help: print available targets
help:
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/  /'
