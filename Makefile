.PHONY: help fmt lint test build smoke tidy

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-10s %s\n", $$1, $$2}'

fmt: ## Format code (gofumpt if available, else gofmt)
	@command -v gofumpt >/dev/null 2>&1 && gofumpt -w . || gofmt -w .

lint: ## Run golangci-lint (must be installed)
	golangci-lint run

test: ## Run tests with race detector and coverage
	go test -race -coverprofile=coverage.out ./...

build: ## Build all binaries into ./bin
	go build -o bin/ ./...

smoke: ## Run the auth/connectivity smoke test (needs OMADA_* env)
	go run ./cmd/omada-smoke

tidy: ## Tidy go.mod/go.sum
	go mod tidy
