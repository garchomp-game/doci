package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"
	_ "github.com/mattn/go-sqlite3"
)

// =============================================================================
// 設定
// =============================================================================

var dbPath string

const chunkLines = 50
const maxFileSize = 512 * 1024 // 512KB

var excludePatterns = []string{
	".cache",
	".local/share/Trash",
	".local/share/baloo",
	"snap/",
	".npm/_cacache",
	".bun/install/cache",
	".gradle/caches",
	"__pycache__",
	".git/objects",
	".git/modules",
	".wine",
	".steam",
	".vscode-server",
	"node_modules/.cache",
}

var textExtensions = map[string]bool{
	".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".mjs": true, ".cjs": true, ".mts": true, ".cts": true,
	".html": true, ".css": true, ".scss": true, ".vue": true, ".svelte": true,
	".py": true, ".pyi": true, ".go": true, ".rs": true,
	".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cc": true,
	".java": true, ".kt": true, ".kts": true,
	".rb": true, ".php": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
	".md": true, ".rst": true, ".txt": true,
	".sql": true, ".lua": true, ".mbt": true,
	".graphql": true, ".gql": true, ".astro": true, ".scm": true,
}

var langMap = map[string]string{
	".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript", ".mts": "TypeScript", ".cts": "TypeScript",
	".py": "Python", ".pyi": "Python", ".go": "Go", ".rs": "Rust",
	".c": "C", ".h": "C", ".cpp": "C++", ".hpp": "C++", ".cc": "C++",
	".java": "Java", ".kt": "Kotlin", ".kts": "Kotlin",
	".rb": "Ruby", ".php": "PHP",
	".sh": "Shell", ".bash": "Shell", ".zsh": "Shell", ".fish": "Shell",
	".html": "HTML", ".css": "CSS", ".scss": "SCSS",
	".json": "JSON", ".yaml": "YAML", ".yml": "YAML",
	".toml": "TOML", ".xml": "XML", ".sql": "SQL",
	".md": "Markdown", ".lua": "Lua", ".mbt": "MoonBit",
	".vue": "Vue", ".svelte": "Svelte", ".astro": "Astro",
	".graphql": "GraphQL", ".gql": "GraphQL",
}

// =============================================================================
// データ型
// =============================================================================

type FileInfo struct {
	Path string
	ID   int64
	Size int64
}

type ChunkBatch struct {
	Chunks []Chunk
	Errors int
	Files  int
}

type Chunk struct {
	FileID  int64
	ChunkNo int
	Content string
}

// =============================================================================
// DB
// =============================================================================

func openFastDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=OFF&_synchronous=OFF&_locking_mode=EXCLUSIVE&_cache_size=-256000&_mmap_size=536870912")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			path       TEXT NOT NULL,
			filename   TEXT NOT NULL,
			extension  TEXT,
			size_bytes INTEGER,
			modified   REAL,
			lang       TEXT
		);
		CREATE TABLE IF NOT EXISTS snippets (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id    INTEGER NOT NULL,
			chunk_no   INTEGER NOT NULL,
			content    TEXT NOT NULL
		);
	`)
	return db, err
}

func openSafeDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-128000&_mmap_size=268435456")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, err
}

func openReadDB(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path+"?_mmap_size=268435456&mode=ro")
}

func createIndexes(db *sql.DB) {
	fmt.Println("\n🔨 インデックスを作成中...")
	start := time.Now()
	for _, q := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_files_path ON files(path)",
		"CREATE INDEX IF NOT EXISTS idx_files_ext ON files(extension)",
		"CREATE INDEX IF NOT EXISTS idx_files_lang ON files(lang)",
		"CREATE INDEX IF NOT EXISTS idx_files_filename ON files(filename)",
		"CREATE INDEX IF NOT EXISTS idx_files_modified ON files(modified)",
		"CREATE INDEX IF NOT EXISTS idx_snippets_file ON snippets(file_id)",
	} {
		if _, err := db.Exec(q); err != nil {
			fmt.Printf("  ⚠️ %v\n", err)
		}
	}
	fmt.Printf("✅ インデックス作成完了 (%.1fs)\n", time.Since(start).Seconds())
}

func createFTS(db *sql.DB) {
	fmt.Println("🔨 FTS5全文検索インデックスを構築中...")
	start := time.Now()
	if _, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS snippets_fts USING fts5(content, content=snippets, content_rowid=id)`); err != nil {
		fmt.Printf("⚠️ FTS5テーブル作成失敗: %v\n", err)
		return
	}
	if _, err := db.Exec(`INSERT INTO snippets_fts(snippets_fts) VALUES('rebuild')`); err != nil {
		fmt.Printf("⚠️ FTS5リビルド失敗: %v\n", err)
		return
	}
	fmt.Printf("✅ FTS5構築完了 (%.1fs)\n", time.Since(start).Seconds())
}

// =============================================================================
// Phase 1: fastwalk 並列クロール → メタデータ登録
// =============================================================================

func shouldExclude(path string) bool {
	for _, p := range excludePatterns {
		if strings.Contains(path, p) {
			return true
		}
	}
	return false
}

type fileMeta struct {
	path string
	name string
	ext  string
	size int64
	mod  float64
	lang string
}

func phase1(db *sql.DB, targetDir string, batchSize int) int64 {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("⚡ Phase 1: fastwalk 並列クロール → メタデータ登録")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	crawlStart := time.Now()

	// fastwalk で並列FS走査
	var fileList []fileMeta
	var mu sync.Mutex

	conf := fastwalk.Config{Follow: false}

	fastwalk.Walk(&conf, targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldExclude(path) {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		name := d.Name()
		ext := strings.ToLower(filepath.Ext(name))

		fm := fileMeta{
			path: path,
			name: name,
			ext:  ext,
			size: info.Size(),
			mod:  float64(info.ModTime().UnixMilli()) / 1000.0,
			lang: langMap[ext],
		}

		mu.Lock()
		fileList = append(fileList, fm)
		mu.Unlock()
		return nil
	})

	fmt.Printf("  📂 fastwalk クロール完了: %s ファイル (%.2fs)\n",
		fmtInt(int64(len(fileList))), time.Since(crawlStart).Seconds())

	// バッチ INSERT
	insertStart := time.Now()
	tx, _ := db.Begin()
	stmt, _ := tx.Prepare("INSERT INTO files (path, filename, extension, size_bytes, modified, lang) VALUES (?, ?, ?, ?, ?, ?)")

	for i, fm := range fileList {
		var langPtr, extPtr *string
		if fm.lang != "" {
			langPtr = &fm.lang
		}
		if fm.ext != "" {
			extPtr = &fm.ext
		}
		stmt.Exec(fm.path, fm.name, extPtr, fm.size, fm.mod, langPtr)

		if (i+1)%batchSize == 0 {
			tx.Commit()
			tx, _ = db.Begin()
			stmt.Close()
			stmt, _ = tx.Prepare("INSERT INTO files (path, filename, extension, size_bytes, modified, lang) VALUES (?, ?, ?, ?, ?, ?)")
			elapsed := time.Since(insertStart).Seconds()
			fmt.Printf("\r  💾 %s / %s | ⏱ %.1fs",
				fmtInt(int64(i+1)), fmtInt(int64(len(fileList))), elapsed)
		}
	}
	stmt.Close()
	tx.Commit()

	totalElapsed := time.Since(crawlStart).Seconds()
	fmt.Printf("\r  💾 %s files | ⏱ %.1fs (%s/s)                    \n",
		fmtInt(int64(len(fileList))), totalElapsed, fmtInt(int64(float64(len(fileList))/totalElapsed)))
	fmt.Println("✅ Phase 1 完了\n")

	fileList = nil
	runtime.GC()

	var count int64
	db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	return count
}

// =============================================================================
// Phase 2: goroutine並列 → チャンク登録（v8と同じ安定版）
// =============================================================================

func phase2(db *sql.DB, numWorkers int, workerBatch int) int64 {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("⚡ Phase 2: %d goroutine 並列読み取り → チャンク登録\n", numWorkers)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	start := time.Now()

	rows, err := db.Query(`
		SELECT id, path, size_bytes FROM files
		WHERE size_bytes > 0 AND size_bytes <= ?
		ORDER BY size_bytes ASC
	`, maxFileSize)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return 0
	}

	var textFiles []FileInfo
	for rows.Next() {
		var f FileInfo
		rows.Scan(&f.ID, &f.Path, &f.Size)
		if textExtensions[strings.ToLower(filepath.Ext(f.Path))] {
			textFiles = append(textFiles, f)
		}
	}
	rows.Close()

	fmt.Printf("\n  📄 対象テキストファイル: %s\n", fmtInt(int64(len(textFiles))))
	fmt.Printf("  🧵 %d goroutine\n\n", numWorkers)

	// チャネル（最小バッファ）
	fileCh := make(chan []FileInfo, 2)
	resultCh := make(chan ChunkBatch, 2)

	var totalChunks atomic.Int64
	var totalProcessed atomic.Int64
	var totalErrors atomic.Int64

	// Workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range fileCh {
				var chunks []Chunk
				errs := 0
				for _, f := range batch {
					data, err := os.ReadFile(f.Path)
					if err != nil {
						errs++
						continue
					}
					lines := strings.Split(string(data), "\n")
					for j := 0; j < len(lines); j += chunkLines {
						end := j + chunkLines
						if end > len(lines) {
							end = len(lines)
						}
						chunk := strings.Join(lines[j:end], "\n")
						if strings.TrimSpace(chunk) != "" {
							chunks = append(chunks, Chunk{f.ID, j / chunkLines, chunk})
						}
					}
				}
				resultCh <- ChunkBatch{Chunks: chunks, Errors: errs, Files: len(batch)}
			}
		}()
	}

	// Writer（SQLiteシングルライター）
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare("INSERT INTO snippets (file_id, chunk_no, content) VALUES (?, ?, ?)")
		bc := 0

		for batch := range resultCh {
			nChunks := int64(len(batch.Chunks))
			for _, c := range batch.Chunks {
				stmt.Exec(c.FileID, c.ChunkNo, c.Content)
				bc++
			}
			batch.Chunks = nil

			totalChunks.Add(nChunks)
			totalProcessed.Add(int64(batch.Files))
			totalErrors.Add(int64(batch.Errors))

			if bc >= 5000 {
				tx.Commit()
				tx, _ = db.Begin()
				stmt.Close()
				stmt, _ = tx.Prepare("INSERT INTO snippets (file_id, chunk_no, content) VALUES (?, ?, ?)")
				bc = 0
			}

			proc := totalProcessed.Load()
			if proc%500 < int64(workerBatch) {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				elapsed := time.Since(start).Seconds()
				fmt.Printf("\r  📝 %s / %s → %s chunks | 🧠 %dMB | ⏱ %.1fs",
					fmtInt(proc), fmtInt(int64(len(textFiles))), fmtInt(totalChunks.Load()),
					m.Sys/(1024*1024), elapsed)
			}
		}
		stmt.Close()
		tx.Commit()
	}()

	// Dispatcher
	go func() {
		for i := 0; i < len(textFiles); i += workerBatch {
			end := i + workerBatch
			if end > len(textFiles) {
				end = len(textFiles)
			}
			fileCh <- textFiles[i:end]
		}
		close(fileCh)
	}()

	wg.Wait()
	close(resultCh)
	<-writerDone

	elapsed := time.Since(start).Seconds()
	fmt.Printf("\r  📝 %s files → %s chunks | ⚠️ %d errors | ⏱ %.1fs                    \n",
		fmtInt(totalProcessed.Load()), fmtInt(totalChunks.Load()), totalErrors.Load(), elapsed)
	fmt.Println("✅ Phase 2 完了\n")
	return totalChunks.Load()
}

// =============================================================================
// 検索
// =============================================================================

func cmdSearch(args []string) {
	f := flag.NewFlagSet("search", flag.ExitOnError)
	name := f.String("name", "", "ファイル名で検索")
	ext := f.String("ext", "", "拡張子で絞り込み")
	lang := f.String("lang", "", "言語で絞り込み")
	stats := f.Bool("stats", false, "統計情報")
	large := f.Int("large", 0, "ファイルサイズTOP N")
	recent := f.Int("recent", 0, "直近N日以内")
	limit := f.Int("limit", 30, "表示件数")
	f.Parse(args)

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "❌ DB未作成: %s\n   → file-indexer-go index -reset\n", dbPath)
		os.Exit(1)
	}

	db, err := openReadDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ DB接続エラー: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	start := time.Now()

	if *stats {
		printStats(db)
		return
	}
	if *large > 0 {
		fmt.Printf("\n\033[1m📦 サイズ TOP %d:\033[0m\n\n", *large)
		printQuery(db, "SELECT path, size_bytes, modified, lang FROM files ORDER BY size_bytes DESC LIMIT ?", *large)
		printElapsed(start)
		return
	}
	if *recent > 0 && *name == "" && *ext == "" && *lang == "" {
		cutoff := float64(time.Now().Unix()) - float64(*recent)*86400
		fmt.Printf("\n\033[1m🕐 直近%d日:\033[0m\n\n", *recent)
		printQuery(db, "SELECT path, size_bytes, modified, lang FROM files WHERE modified > ? ORDER BY modified DESC LIMIT ?", cutoff, *limit)
		printElapsed(start)
		return
	}

	query := f.Arg(0)
	if query != "" {
		fmt.Printf("\n\033[1m🔍 全文検索: \"%s\"\033[0m\n\n", query)
		rows, err := db.Query(`
			SELECT f.path, f.size_bytes, f.modified, f.lang,
			       snippet(snippets_fts, 0, '>>>', '<<<', '...', 40) AS snip
			FROM snippets_fts
			JOIN snippets s ON s.id = snippets_fts.rowid
			JOIN files f ON f.id = s.file_id
			WHERE snippets_fts MATCH ?
			ORDER BY rank LIMIT ?`, query, *limit)
		if err != nil {
			fmt.Printf("  ⚠️ FTS5エラー: %v\n  → index -reset で再構築\n", err)
			return
		}
		printRowsWithSnippet(rows)
		printElapsed(start)
		return
	}

	if *name != "" || *ext != "" || *lang != "" || *recent > 0 {
		var conds []string
		var params []interface{}
		var labels []string

		if *ext != "" {
			e := *ext
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			conds = append(conds, "extension = ?")
			params = append(params, e)
			labels = append(labels, "拡張子="+*ext)
		}
		if *lang != "" {
			conds = append(conds, "lang = ?")
			params = append(params, *lang)
			labels = append(labels, "言語="+*lang)
		}
		if *name != "" {
			conds = append(conds, "filename LIKE ?")
			params = append(params, "%"+*name+"%")
			labels = append(labels, "名前="+*name)
		}
		if *recent > 0 {
			cutoff := float64(time.Now().Unix()) - float64(*recent)*86400
			conds = append(conds, "modified > ?")
			params = append(params, cutoff)
			labels = append(labels, fmt.Sprintf("直近%d日", *recent))
		}

		fmt.Printf("\n\033[1m🔍 %s\033[0m\n\n", strings.Join(labels, " + "))
		params = append(params, *limit)
		printQuery(db, "SELECT path, size_bytes, modified, lang FROM files WHERE "+strings.Join(conds, " AND ")+" ORDER BY modified DESC LIMIT ?", params...)
		printElapsed(start)
		return
	}

	fmt.Println(`
📎 file-indexer-go search

  search "keyword"          FTS5全文検索
  search -name "Button"     ファイル名
  search -ext tsx            拡張子
  search -lang TypeScript    言語
  search -recent 7           直近7日
  search -large 10           サイズTOP
  search -stats              統計`)
}

// =============================================================================
// 表示
// =============================================================================

func printQuery(db *sql.DB, q string, args ...interface{}) {
	rows, err := db.Query(q, args...)
	if err != nil {
		fmt.Printf("  ⚠️ %v\n", err)
		return
	}
	printRows(rows)
}

func printRows(rows *sql.Rows) {
	if rows == nil {
		return
	}
	defer rows.Close()
	home, _ := os.UserHomeDir()
	n := 0
	for rows.Next() {
		var path string
		var size int64
		var mod float64
		var lang sql.NullString
		rows.Scan(&path, &size, &mod, &lang)
		p := strings.Replace(path, home, "~", 1)
		l := ""
		if lang.Valid {
			l = fmt.Sprintf("\033[35m[%s]\033[0m", lang.String)
		}
		fmt.Printf("  \033[32m%s\033[0m %s \033[36m%s\033[0m \033[2m%s\033[0m\n", p, l, fmtSize(size), fmtTime(mod))
		n++
	}
	fmt.Printf("\n  \033[1m%d 件表示\033[0m\n", n)
}

func printRowsWithSnippet(rows *sql.Rows) {
	if rows == nil {
		return
	}
	defer rows.Close()
	home, _ := os.UserHomeDir()
	n := 0
	for rows.Next() {
		var path string
		var size int64
		var mod float64
		var lang, snip sql.NullString
		rows.Scan(&path, &size, &mod, &lang, &snip)
		p := strings.Replace(path, home, "~", 1)
		l := ""
		if lang.Valid {
			l = fmt.Sprintf("\033[35m[%s]\033[0m", lang.String)
		}
		fmt.Printf("  \033[32m%s\033[0m %s \033[36m%s\033[0m \033[2m%s\033[0m\n", p, l, fmtSize(size), fmtTime(mod))
		if snip.Valid {
			s := strings.ReplaceAll(snip.String, "\n", " ")
			if len(s) > 120 {
				s = s[:120] + "..."
			}
			fmt.Printf("    \033[2m%s\033[0m\n", s)
		}
		n++
	}
	fmt.Printf("\n  \033[1m%d 件表示\033[0m\n", n)
}

func printStats(db *sql.DB) {
	var fc, ts, cc int64
	db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fc)
	db.QueryRow("SELECT COALESCE(SUM(size_bytes),0) FROM files").Scan(&ts)
	db.QueryRow("SELECT COUNT(*) FROM snippets").Scan(&cc)

	fmt.Println("\n\033[1m" + strings.Repeat("═", 60) + "\033[0m")
	fmt.Println("\033[1m📊 インデックス統計\033[0m")
	fmt.Println("\033[1m" + strings.Repeat("═", 60) + "\033[0m")
	fmt.Printf("  ファイル: \033[32m%s\033[0m  サイズ: \033[36m%s\033[0m  チャンク: \033[33m%s\033[0m\n", fmtInt(fc), fmtSize(ts), fmtInt(cc))

	rows, _ := db.Query("SELECT extension, COUNT(*), SUM(size_bytes) FROM files WHERE extension IS NOT NULL GROUP BY extension ORDER BY COUNT(*) DESC LIMIT 15")
	if rows != nil {
		fmt.Println("\n  🔹 拡張子別 TOP 15:")
		for rows.Next() {
			var ext string
			var cnt, sz int64
			rows.Scan(&ext, &cnt, &sz)
			fmt.Printf("    %-10s %8s files (%8s)\n", ext, fmtInt(cnt), fmtSize(sz))
		}
		rows.Close()
	}
	if fi, err := os.Stat(dbPath); err == nil {
		fmt.Printf("\n  DB: \033[36m%s\033[0m\n", fmtSize(fi.Size()))
	}
	fmt.Println("\033[1m" + strings.Repeat("═", 60) + "\033[0m")
}

func printElapsed(s time.Time) {
	fmt.Printf("  \033[2m⏱ %.1fms\033[0m\n", float64(time.Since(s).Microseconds())/1000)
}

func fmtSize(b int64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%dB", b)
	}
	d, e := int64(u), 0
	for n := b / u; n >= u; n /= u {
		d *= u
		e++
	}
	return fmt.Sprintf("%.1f%s", float64(b)/float64(d), []string{"KB", "MB", "GB", "TB"}[e])
}

func fmtTime(ts float64) string {
	return time.Unix(int64(ts), 0).Format("2006-01-02 15:04")
}

func fmtInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var r []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			r = append(r, ',')
		}
		r = append(r, byte(c))
	}
	return string(r)
}

// =============================================================================
// メイン
// =============================================================================

func main() {
	home, _ := os.UserHomeDir()
	dbPath = filepath.Join(home, "file-indexer-go", "file_index.db")

	if len(os.Args) < 2 {
		fmt.Println("使い方: file-indexer-go <index|search>")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "index":
		cmdIndex(os.Args[2:])
	case "search":
		cmdSearch(os.Args[2:])
	default:
		fmt.Printf("不明: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdIndex(args []string) {
	f := flag.NewFlagSet("index", flag.ExitOnError)
	target := f.String("target", "", "対象ディレクトリ")
	metaOnly := f.Bool("meta-only", false, "メタデータのみ")
	reset := f.Bool("reset", false, "DB削除して再構築")
	f.Parse(args)

	home, _ := os.UserHomeDir()
	if *target == "" {
		*target = home
	}

	debug.SetMemoryLimit(1 * 1024 * 1024 * 1024)

	cores := runtime.NumCPU()
	numWorkers := cores / 2
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > 6 {
		numWorkers = 6
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ⚡ File Indexer v10 - Go Stable + fastwalk               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("   DB: %s\n   対象: %s\n", dbPath, *target)
	fmt.Printf("\n🖥️  CPU: %d コア → %d goroutine\n", cores, numWorkers)
	fmt.Println("🔒 GOMEMLIMIT: 1GiB")

	if *reset {
		for _, p := range []string{dbPath, filepath.Join(home, "file_index.db"), filepath.Join(home, "file-indexer", "file_index.db")} {
			for _, s := range []string{"", "-wal", "-shm"} {
				os.Remove(p + s)
			}
		}
		fmt.Println("   🗑️  既存DB削除済み")
	}

	totalStart := time.Now()

	fastDB, err := openFastDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	phase1(fastDB, *target, 10000)

	if !*metaOnly {
		phase2(fastDB, numWorkers, 50)
	} else {
		fmt.Println("📌 --meta-only: チャンクスキップ")
	}

	fastDB.Close()
	fmt.Println("🔄 安全モードでDB再オープン...")

	safeDB, err := openSafeDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	createIndexes(safeDB)
	if !*metaOnly {
		createFTS(safeDB)
	}

	totalElapsed := time.Since(totalStart).Seconds()
	var fileCount, chunkCount int64
	safeDB.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
	safeDB.QueryRow("SELECT COUNT(*) FROM snippets").Scan(&chunkCount)
	safeDB.Close()

	fi, _ := os.Stat(dbPath)
	fmt.Println("\n" + strings.Repeat("═", 60))
	fmt.Println("📊 インデックス構築完了")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("   ファイル数:   %s\n", fmtInt(fileCount))
	fmt.Printf("   チャンク数:   %s\n", fmtInt(chunkCount))
	if fi != nil {
		fmt.Printf("   DBサイズ:     %s\n", fmtSize(fi.Size()))
	}
	fmt.Printf("   総所要時間:   %.1f秒\n", totalElapsed)
	fmt.Printf("   処理速度:     %s files/s\n", fmtInt(int64(float64(fileCount)/totalElapsed)))
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("\n🔍 検索: ./file-indexer-go search -stats")
}
