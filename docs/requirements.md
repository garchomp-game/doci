---
title: "doci — AI-Native Document Indexer"
description: プロジェクト概要と要件定義
---

## プロダクトビジョン

**doci** は、開発プロジェクトのドキュメントを SQLite にインデックスし、AIエージェントが高速に検索・参照できるようにする **Go 製 CLI ツール**。

FTS5（キーワード検索）と Embedding（セマンティック検索）のハイブリッドにより、「何を読むべきか」の判断を **2ms以下** に短縮し、AI コーディングアシスタント（Claude Code / Cursor 等）の精度を向上させる。

### 解決する課題

| # | 課題 | 現状 | doci での解決 |
|---|------|------|--------------|
| 1 | AIが関連ファイルを見つけるのが遅い | grep/find でO(n)走査 | SQLite FTS5 で O(log n)、2ms |
| 2 | キーワードがわからないと検索できない | 「ログインの仕組み」では何もヒットしない | セマンティック検索で意味類似を発見 |
| 3 | node_modules等のノイズが混入 | 手動で除外パターンを指定 | `.gitignore` 自動パース |
| 4 | ドキュメントのメタデータが活用されない | ファイル内容のみ | frontmatter tags/title で絞り込み |
| 5 | 大規模プロジェクトでリビルドが遅い | 毎回全ファイルを処理 | 差分インデックスで変更分のみ |

### 既存ツールとの差別化

| 機能 | sem | qmd | sff | **doci** |
|------|-----|-----|-----|---------|
| 言語 | Python | Node.js | Python | **Go（爆速）** |
| FTS5 | ❌ | ✅(BM25) | ❌ | **✅** |
| Embedding | ✅ | ✅ | ✅ | **✅** |
| .gitignore | △(Git前提) | ❌ | ❌ | **✅** |
| frontmatter tags | ❌ | ❌ | ❌ | **✅** |
| 差分インデックス | ❌ | ❌ | ❌ | **✅** |
| watch モード | ❌ | ❌ | ❌ | **✅** |
| AI向け context 出力 | ❌ | ❌ | ❌ | **✅** |
| ハイブリッドランキング | ❌ | △ | ❌ | **✅(RRF)** |

---

## スコープ

### IN（対象範囲）

| 機能 | 概要 | 優先度 |
|------|------|--------|
| **index** | ディレクトリ走査 → SQLite FTS5 インデックス構築 | **Must** |
| **search** | FTS5 キーワード検索 | **Must** |
| **--use-gitignore** | `.gitignore` パースによる自動除外 | **Must** |
| **--tag** | frontmatter tags 絞り込み | **Must** |
| **--tree** | ドキュメントツリー出力 | **Must** |
| **--context** | チャンク本文の stdout 出力（パイプ向け） | **Must** |
| **--incremental** | 差分インデックス（変更ファイルのみ） | **Should** |
| **--watch** | ファイル変更監視 → 自動再インデックス | **Should** |
| **embed** | Ollama embedding 生成 | **Should** |
| **semantic** | セマンティック検索（cosine similarity） | **Should** |
| **--hybrid** | FTS5 + Semantic の RRF 統合ランキング | **Could** |
| **--related** | ファイル類似度一覧 | **Could** |
| **--diff** | git diff 連動の影響範囲分析 | **Could** |
| **--mcp** | MCP サーバーモード | **Could** |
| **--export** | JSON/YAML/Markdown エクスポート | **Could** |

### OUT（対象外）

- クラウドストレージ連携
- Web UI / ダッシュボード
- 自然言語による要約生成（LLM呼び出し）
- バイナリファイルの解析（画像/動画等）

---

## CLI 設計

### コマンド体系

```bash
doci [target] [flags]              # デフォルト: index + search
doci index [target] [flags]        # インデックス構築
doci search [query] [flags]        # 検索
doci embed [flags]                 # embedding 生成
doci semantic [query] [flags]      # セマンティック検索
doci tree [target] [flags]         # ツリー表示
doci stats [flags]                 # 統計情報
doci watch [target] [flags]        # ファイル監視
```

### 主要フラグ

```bash
# グローバル
--db <path>              DB パス (default: ./.doci.db)
--verbose                詳細ログ

# index
--use-gitignore          .gitignore を尊重
--incremental            差分のみ再インデックス
--reset                  DB削除して再構築
--exclude <pattern>      追加除外パターン (glob)
--ext <extensions>       対象拡張子 (default: md,mdx,txt,yaml,json,ts,go...)

# search / semantic
--tag <tag>              frontmatter tag で絞り込み (複数指定可)
--limit <n>              表示件数 (default: 10)
--context                チャンク本文を出力（パイプ向け）
--json                   JSON 出力

# embed
--model <name>           embedding モデル (default: nomic-embed-text)
--ollama-url <url>       Ollama エンドポイント

# watch
--interval <duration>    ポーリング間隔 (default: 5s)
--on-change <command>    変更時に実行するコマンド
```

### 使用例

```bash
# 基本: ドキュメントをインデックスして検索
$ doci index ./src/content --use-gitignore
$ doci search "認証の仕組み" --tag architecture

# パイプ: 検索結果をAIに直接渡す
$ doci search "REQ-B01" --context | claude "このチケットの実装計画を作って"

# ツリー: AIにフォルダ構成を把握させる
$ doci tree ./src/content

# ワンライナー: インデックス → セマンティック検索
$ doci index ./docs --use-gitignore && doci semantic "ログインの仕組み"

# watch: 保存するたびに自動再インデックス
$ doci watch ./src/content --use-gitignore --incremental
```

---

## アーキテクチャ

### プロジェクト構成

```
doci/
├── cmd/                          # Cobra コマンド定義
│   ├── root.go                   # グローバルフラグ、DB パス
│   ├── index.go                  # index サブコマンド
│   ├── search.go                 # search サブコマンド
│   ├── embed.go                  # embed サブコマンド
│   ├── semantic.go               # semantic サブコマンド
│   ├── tree.go                   # tree サブコマンド
│   ├── stats.go                  # stats サブコマンド
│   └── watch.go                  # watch サブコマンド
├── internal/
│   ├── indexer/
│   │   ├── crawler.go            # fastwalk ディレクトリ走査
│   │   ├── chunker.go            # テキスト → チャンク分割
│   │   ├── frontmatter.go        # YAML frontmatter パーサー
│   │   ├── gitignore.go          # .gitignore パーサー
│   │   └── incremental.go        # 差分検出ロジック
│   ├── store/
│   │   ├── sqlite.go             # SQLite 接続・PRAGMA管理
│   │   ├── schema.go             # テーブル定義・マイグレーション
│   │   ├── writer.go             # バッチ INSERT
│   │   └── reader.go             # 検索クエリ
│   ├── embedding/
│   │   ├── ollama.go             # Ollama API クライアント
│   │   ├── vector.go             # ベクトル操作（cosine sim等）
│   │   └── hybrid.go             # RRF ハイブリッドランキング
│   └── output/
│       ├── tree.go               # ツリー表示
│       ├── context.go            # --context 出力
│       └── formatter.go          # テーブル/JSON 出力
├── main.go                       # エントリポイント
├── go.mod
└── go.sum
```

### 依存ライブラリ

| ライブラリ | 用途 |
|-----------|------|
| `spf13/cobra` | CLI フレームワーク |
| `mattn/go-sqlite3` | SQLite ドライバ (CGo, FTS5) |
| `charlievieth/fastwalk` | 並列 FS 走査 |
| `sabhiram/go-gitignore` | .gitignore パーサー |
| `fsnotify/fsnotify` | ファイル変更監視 (watch) |
| `go-yaml/yaml` | frontmatter パーサー |

### DB 設計

```sql
-- メタデータ
CREATE TABLE files (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  path       TEXT NOT NULL UNIQUE,
  filename   TEXT NOT NULL,
  extension  TEXT,
  size_bytes INTEGER,
  modified   REAL,           -- Unix timestamp
  lang       TEXT,
  title      TEXT,           -- frontmatter title
  tags       TEXT,           -- frontmatter tags (JSON array)
  indexed_at REAL            -- インデックス時刻（差分検出用）
);

-- チャンク
CREATE TABLE snippets (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id  INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  chunk_no INTEGER NOT NULL,
  content  TEXT NOT NULL
);

-- FTS5 全文検索
CREATE VIRTUAL TABLE snippets_fts USING fts5(
  content,
  content=snippets,
  content_rowid=id
);

-- Embedding ベクトル
CREATE TABLE embeddings (
  snippet_id INTEGER PRIMARY KEY REFERENCES snippets(id) ON DELETE CASCADE,
  file_id    INTEGER NOT NULL,
  embedding  BLOB NOT NULL   -- float32[] packed binary
);

-- メタ情報
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT
);
-- meta examples: last_indexed_at, db_version, embedding_model, embedding_dim
```

### 差分インデックスの仕組み

```
1. meta テーブルから last_indexed_at を取得
2. fastwalk でファイル走査
3. 各ファイルの modified と DB の modified を比較
4. 変更/新規 → INSERT OR REPLACE
5. DB にあるがFSにないファイル → DELETE
6. 変更チャンクの embedding のみ再計算
7. FTS5 rebuild
8. meta.last_indexed_at を更新
```

---

## 成功指標

| 指標 | 目標値 |
|------|--------|
| 72ファイルのフルインデックス | < 0.5秒 |
| 3万ファイルのフルインデックス | < 10秒 |
| 差分インデックス（1ファイル変更時） | < 0.1秒 |
| FTS5 検索レスポンス | < 5ms |
| セマンティック検索レスポンス | < 100ms (embedding cached) |
| メモリ使用量（3万ファイル） | < 500MB |
| バイナリサイズ | < 15MB |
