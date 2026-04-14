.PHONY: build test lint run-example clean tools check

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
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

## help: print available targets
help:
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/  /'
