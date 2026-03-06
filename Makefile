.PHONY: build install install-system install-man uninstall clean fmt test

BINARY  = doci
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -s -w -X main.version=$(VERSION)

build:
	CGO_ENABLED=1 go build -tags fts5 -ldflags "$(LDFLAGS)" -o $(BINARY) .

# ユーザーローカル（sudo不要）
install: build
	mkdir -p $(HOME)/.local/bin
	install -m 755 $(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "✅ $(BINARY) を ~/.local/bin/ にインストールしました"

# システム全体（sudo必要）
install-system: build
	sudo install -m 755 $(BINARY) /usr/local/bin/$(BINARY)
	@echo "✅ $(BINARY) を /usr/local/bin/ にインストールしました"

# manページ生成・インストール
install-man: build
	./$(BINARY) man
	@echo "   確認: man doci"

uninstall:
	rm -f $(HOME)/.local/bin/$(BINARY)
	sudo rm -f /usr/local/bin/$(BINARY)
	@echo "🗑️  $(BINARY) を削除しました"

clean:
	rm -f $(BINARY)
	rm -f *.db *.db-wal *.db-shm

fmt:
	gofmt -w .

test:
	CGO_ENABLED=1 go test -tags fts5 ./...
