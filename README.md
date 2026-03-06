# doci — AI-Native Document Indexer

ドキュメントを SQLite にインデックスし、AIエージェントが高速検索できる Go 製 CLI。

FTS5（キーワード検索）+ Embedding（セマンティック検索）のハイブリッドで、
「何を読むべきか」の判断を **2ms以下** に短縮します。

## インストール

```bash
CGO_ENABLED=1 go build -tags fts5 -o doci .
```

## 使い方

```bash
# インデックス構築
doci index ./src/content --use-gitignore --reset

# FTS5 キーワード検索
doci search "Guard 認証"

# タグで絞り込み
doci search "Guard" --tag architecture

# ツリー表示
doci tree ./src/content --depth 3

# 統計情報
doci stats

# AIにパイプ（チャンク本文出力）
doci search "REQ-B01" --context | your-ai-command
```

## パフォーマンス

| 指標 | 実測値 |
|------|--------|
| 72ファイルのインデックス | **0.04秒** |
| FTS5 検索 | **1.1ms** |
| DBサイズ | 1.1MB |

## 依存ライブラリ

- [Cobra](https://cobra.dev/) — CLI フレームワーク
- [fastwalk](https://github.com/charlievieth/fastwalk) — 並列FS走査
- [go-sqlite3](https://github.com/mattn/go-sqlite3) — SQLite (FTS5)
- [go-gitignore](https://github.com/sabhiram/go-gitignore) — .gitignore パーサー

## ドキュメント

- [要件定義・設計書](docs/requirements.md)
