.PHONY: help build install install-system install-man uninstall clean fmt test

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

BINARY  = doci
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -s -w -X main.version=$(VERSION)

build: ## Build binary
	CGO_ENABLED=1 go build -tags fts5 -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: build ## Install to ~/.local/bin (no sudo)
	mkdir -p $(HOME)/.local/bin
	install -m 755 $(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "✅ $(BINARY) installed to ~/.local/bin/"

install-system: build ## Install to /usr/local/bin (sudo)
	sudo install -m 755 $(BINARY) /usr/local/bin/$(BINARY)
	@echo "✅ $(BINARY) installed to /usr/local/bin/"

install-man: build ## Generate and install man pages
	./$(BINARY) man

uninstall: ## Remove doci from PATH
	rm -f $(HOME)/.local/bin/$(BINARY)
	sudo rm -f /usr/local/bin/$(BINARY)
	@echo "🗑️  $(BINARY) removed"

clean: ## Remove binary and DB files
	rm -f $(BINARY)
	rm -f *.db *.db-wal *.db-shm

fmt: ## Run gofmt
	gofmt -w .

test: ## Run tests
	CGO_ENABLED=1 go test -tags fts5 ./...
