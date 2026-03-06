package indexer

import (
	"encoding/json"
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
	"github.com/garchomp-game/doci/internal/store"
)

// Config holds indexing configuration.
type Config struct {
	Target       string
	UseGitIgnore bool
	Exclude      []string
	Extensions   []string
	Reset        bool
	Incremental  bool
	ChunkLines   int
	MaxFileSize  int64
	Verbose      bool
}

// FileMeta holds metadata for a single file.
type FileMeta struct {
	Path     string
	Name     string
	Ext      string
	Size     int64
	Modified float64
	Lang     string
	Title    string
	Tags     string   // JSON array
	TagsList []string // parsed tags for file_tags table
}

// Result holds indexing statistics.
type Result struct {
	FileCount  int64
	ChunkCount int64
	ErrorCount int64
	Duration   time.Duration
	DBSize     int64
}

const defaultMaxFileSize int64 = 512 * 1024 // 512KB

var textExtensions = map[string]bool{
	".md": true, ".mdx": true, ".txt": true, ".rst": true,
	".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".mjs": true, ".cjs": true, ".mts": true, ".cts": true,
	".html": true, ".css": true, ".scss": true, ".vue": true, ".svelte": true,
	".py": true, ".pyi": true, ".go": true, ".rs": true,
	".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cc": true,
	".java": true, ".kt": true, ".kts": true,
	".rb": true, ".php": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
	".sql": true, ".lua": true, ".mbt": true,
	".graphql": true, ".gql": true, ".astro": true,
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
	".md": "Markdown", ".mdx": "Markdown", ".lua": "Lua", ".mbt": "MoonBit",
	".vue": "Vue", ".svelte": "Svelte", ".astro": "Astro",
	".graphql": "GraphQL", ".gql": "GraphQL",
}

// Run executes the full indexing pipeline.
func Run(cfg Config, dbPath string) (*Result, error) {
	debug.SetMemoryLimit(1 * 1024 * 1024 * 1024)
	start := time.Now()

	if cfg.ChunkLines == 0 {
		cfg.ChunkLines = DefaultChunkLines
	}
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = defaultMaxFileSize
	}

	cores := runtime.NumCPU()
	numWorkers := cores / 2
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > 6 {
		numWorkers = 6
	}

	// Reset
	if cfg.Reset {
		for _, s := range []string{"", "-wal", "-shm"} {
			os.Remove(dbPath + s)
		}
	}

	// Phase 1: Crawl + metadata
	fastDB, err := store.OpenFast(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open DB: %w", err)
	}
	if err := fastDB.InitSchema(); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	files, err := crawl(cfg)
	if err != nil {
		return nil, fmt.Errorf("crawl: %w", err)
	}

	if err := insertFiles(fastDB, files); err != nil {
		return nil, fmt.Errorf("insert files: %w", err)
	}

	var fileCount int64
	fastDB.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)

	// Phase 2: Chunks
	chunkCount, errCount := processChunks(fastDB, numWorkers, cfg.ChunkLines)
	fastDB.Close()

	// Phase 3: Indexes + FTS
	safeDB, err := store.OpenSafe(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open safe DB: %w", err)
	}
	safeDB.CreateIndexes()
	safeDB.CreateFTS()
	safeDB.SetLastIndexed()
	safeDB.Close()

	// Calculate DB size
	var dbSize int64
	if fi, err := os.Stat(dbPath); err == nil {
		dbSize = fi.Size()
	}

	return &Result{
		FileCount:  fileCount,
		ChunkCount: chunkCount,
		ErrorCount: errCount,
		Duration:   time.Since(start),
		DBSize:     dbSize,
	}, nil
}

func crawl(cfg Config) ([]FileMeta, error) {
	var gi *GitIgnore
	if cfg.UseGitIgnore {
		var err error
		gi, err = LoadGitIgnore(cfg.Target)
		if err != nil {
			return nil, err
		}
	}

	var files []FileMeta
	var mu sync.Mutex

	conf := fastwalk.Config{Follow: false}
	fastwalk.Walk(&conf, cfg.Target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(cfg.Target, path)

		if d.IsDir() {
			if gi != nil && gi.ShouldIgnore(relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		if gi != nil && gi.ShouldIgnore(relPath) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		name := d.Name()
		ext := strings.ToLower(filepath.Ext(name))

		// Parse frontmatter for md/mdx
		var title string
		var tags string
		var tagsList []string
		if ext == ".md" || ext == ".mdx" {
			if data, err := os.ReadFile(path); err == nil {
				parsed, _ := ParseFrontmatter(string(data))
				if parsed != nil {
					title = parsed.Title
					tagsList = parsed.Tags
					if len(parsed.Tags) > 0 {
						if b, err := json.Marshal(parsed.Tags); err == nil {
							tags = string(b)
						}
					}
				}
			}
		}

		fileMeta := FileMeta{
			Path:     path,
			Name:     name,
			Ext:      ext,
			Size:     info.Size(),
			Modified: float64(info.ModTime().UnixMilli()) / 1000.0,
			Lang:     langMap[ext],
			Title:    title,
			Tags:     tags,
			TagsList: tagsList,
		}

		mu.Lock()
		files = append(files, fileMeta)
		mu.Unlock()
		return nil
	})

	return files, nil
}

func insertFiles(db *store.DB, files []FileMeta) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO files (path, filename, extension, size_bytes, modified, lang, title, tags, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	tagStmt, err := tx.Prepare(`INSERT OR IGNORE INTO file_tags (file_id, tag) VALUES (?, ?)`)
	if err != nil {
		return err
	}

	now := float64(time.Now().UnixMilli()) / 1000.0
	for i, f := range files {
		var langPtr, extPtr, titlePtr, tagsPtr *string
		if f.Lang != "" {
			langPtr = &f.Lang
		}
		if f.Ext != "" {
			extPtr = &f.Ext
		}
		if f.Title != "" {
			titlePtr = &f.Title
		}
		if f.Tags != "" {
			tagsPtr = &f.Tags
		}
		result, _ := stmt.Exec(f.Path, f.Name, extPtr, f.Size, f.Modified, langPtr, titlePtr, tagsPtr, now)

		// Insert normalized tags
		if len(f.TagsList) > 0 {
			fileID, _ := result.LastInsertId()
			for _, tag := range f.TagsList {
				tagStmt.Exec(fileID, tag)
			}
		}

		if (i+1)%10000 == 0 {
			tx.Commit()
			tx, _ = db.Begin()
			stmt.Close()
			tagStmt.Close()
			stmt, _ = tx.Prepare(`INSERT INTO files (path, filename, extension, size_bytes, modified, lang, title, tags, indexed_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
			tagStmt, _ = tx.Prepare(`INSERT OR IGNORE INTO file_tags (file_id, tag) VALUES (?, ?)`)
		}
	}
	stmt.Close()
	tagStmt.Close()
	return tx.Commit()
}

func processChunks(db *store.DB, numWorkers, chunkLines int) (int64, int64) {
	rows, err := db.Query(`SELECT id, path, size_bytes FROM files WHERE size_bytes > 0 AND size_bytes <= ? ORDER BY size_bytes ASC`, defaultMaxFileSize)
	if err != nil {
		return 0, 0
	}

	type fileEntry struct {
		ID   int64
		Path string
		Size int64
	}

	var textFiles []fileEntry
	for rows.Next() {
		var f fileEntry
		rows.Scan(&f.ID, &f.Path, &f.Size)
		if textExtensions[strings.ToLower(filepath.Ext(f.Path))] {
			textFiles = append(textFiles, f)
		}
	}
	rows.Close()

	type chunkBatch struct {
		Chunks []Chunk
		Errors int
		Files  int
	}

	fileCh := make(chan []fileEntry, 2)
	resultCh := make(chan chunkBatch, 2)
	var totalChunks, totalErrors atomic.Int64

	workerBatch := 50
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
					chunks = append(chunks, ChunkText(string(data), f.ID, chunkLines)...)
				}
				resultCh <- chunkBatch{Chunks: chunks, Errors: errs, Files: len(batch)}
			}
		}()
	}

	// Writer
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		tx, _ := db.Begin()
		stmt, _ := tx.Prepare("INSERT INTO snippets (file_id, chunk_no, content) VALUES (?, ?, ?)")
		bc := 0
		for batch := range resultCh {
			for _, c := range batch.Chunks {
				stmt.Exec(c.FileID, c.ChunkNo, c.Content)
				bc++
			}
			totalChunks.Add(int64(len(batch.Chunks)))
			totalErrors.Add(int64(batch.Errors))
			if bc >= 5000 {
				tx.Commit()
				tx, _ = db.Begin()
				stmt.Close()
				stmt, _ = tx.Prepare("INSERT INTO snippets (file_id, chunk_no, content) VALUES (?, ?, ?)")
				bc = 0
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

	return totalChunks.Load(), totalErrors.Load()
}
