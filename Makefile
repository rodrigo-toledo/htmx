# htmx kanban — task runner
#
# Run `make` (or `make help`) to list targets. Works with the make that
# ships on macOS/Linux; no extra tooling required.

GO    ?= go
BIN   := bin/kanban
E2E   := e2e/run.sh

.DEFAULT_GOAL := help

.PHONY: help build run test cover e2e e2e-install vet fmt check clean

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-13s\033[0m %s\n", $$1, $$2}'

build: ## Compile the server into bin/kanban
	$(GO) build -o $(BIN) .

run: ## Run the server (http://localhost:8080)
	$(GO) run .

test: ## Run Go unit + handler tests
	$(GO) test ./...

cover: ## Run tests with a coverage report printed to the terminal
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
	@echo "  (view as HTML: go tool cover -html=coverage.out)"

e2e: ## Run the browser end-to-end suites (auto-installs JS deps once)
	@test -d e2e/node_modules || $(MAKE) e2e-install
	$(E2E)

e2e-install: ## Install the e2e JavaScript dependencies
	cd e2e && npm install

vet: ## Run go vet static analysis
	$(GO) vet ./...

fmt: ## Format Go source
	$(GO) fmt ./...

check: vet test e2e ## Run the full gate: vet + tests + e2e

clean: ## Remove build artifacts
	rm -rf bin coverage.out
