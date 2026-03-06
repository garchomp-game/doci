package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite connection with mode-specific configurations.
type DB struct {
	*sql.DB
	Path string
}

// OpenFast opens DB optimized for bulk writes (journal off, sync off).
func OpenFast(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=OFF&_synchronous=OFF&_locking_mode=EXCLUSIVE&_cache_size=-256000&_mmap_size=536870912")
	if err != nil {
		return nil, fmt.Errorf("open fast DB: %w", err)
	}
	db.SetMaxOpenConns(1)
	return &DB{DB: db, Path: path}, nil
}

// OpenSafe opens DB for index/FTS creation (WAL mode).
func OpenSafe(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-128000&_mmap_size=268435456")
	if err != nil {
		return nil, fmt.Errorf("open safe DB: %w", err)
	}
	db.SetMaxOpenConns(1)
	return &DB{DB: db, Path: path}, nil
}

// OpenRead opens DB for read-only queries.
func OpenRead(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_mmap_size=268435456&mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open read DB: %w", err)
	}
	return &DB{DB: db, Path: path}, nil
}

// SetMeta writes a key-value pair to the meta table.
func (d *DB) SetMeta(key, value string) error {
	_, err := d.Exec("INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)", key, value)
	return err
}

// GetMeta reads a value from the meta table.
func (d *DB) GetMeta(key string) (string, error) {
	var val string
	err := d.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// SetLastIndexed updates the last_indexed_at timestamp.
func (d *DB) SetLastIndexed() error {
	return d.SetMeta("last_indexed_at", fmt.Sprintf("%f", float64(time.Now().UnixMilli())/1000.0))
}
