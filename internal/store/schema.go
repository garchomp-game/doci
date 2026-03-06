package store

// InitSchema creates all tables and indexes.
func (d *DB) InitSchema() error {
	_, err := d.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			path       TEXT NOT NULL,
			filename   TEXT NOT NULL,
			extension  TEXT,
			size_bytes INTEGER,
			modified   REAL,
			lang       TEXT,
			title      TEXT,
			tags       TEXT,
			indexed_at REAL
		);

		CREATE TABLE IF NOT EXISTS file_tags (
			file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			tag     TEXT NOT NULL,
			PRIMARY KEY (file_id, tag)
		);

		CREATE TABLE IF NOT EXISTS snippets (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id  INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			chunk_no INTEGER NOT NULL,
			content  TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS embeddings (
			snippet_id INTEGER PRIMARY KEY REFERENCES snippets(id) ON DELETE CASCADE,
			file_id    INTEGER NOT NULL,
			embedding  BLOB NOT NULL
		);

		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	return err
}

// CreateIndexes adds secondary indexes on files and snippets.
func (d *DB) CreateIndexes() error {
	stmts := []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_files_path ON files(path)",
		"CREATE INDEX IF NOT EXISTS idx_files_ext ON files(extension)",
		"CREATE INDEX IF NOT EXISTS idx_files_lang ON files(lang)",
		"CREATE INDEX IF NOT EXISTS idx_files_filename ON files(filename)",
		"CREATE INDEX IF NOT EXISTS idx_files_modified ON files(modified)",
		"CREATE INDEX IF NOT EXISTS idx_files_tags ON files(tags)",
		"CREATE INDEX IF NOT EXISTS idx_file_tags_tag ON file_tags(tag)",
		"CREATE INDEX IF NOT EXISTS idx_snippets_file ON snippets(file_id)",
		"CREATE INDEX IF NOT EXISTS idx_emb_file ON embeddings(file_id)",
	}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// CreateFTS builds the FTS5 full-text search index with unicode61 tokenizer.
// unicode61 handles CJK characters better than the default ASCII tokenizer.
// remove_diacritics=2 removes all diacritical marks for broader matching.
func (d *DB) CreateFTS() error {
	// Drop old FTS table to ensure tokenizer change takes effect
	d.Exec(`DROP TABLE IF EXISTS snippets_fts`)

	if _, err := d.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS snippets_fts USING fts5(
		content,
		content=snippets,
		content_rowid=id,
		tokenize='unicode61 remove_diacritics 2'
	)`); err != nil {
		return err
	}
	_, err := d.Exec(`INSERT INTO snippets_fts(snippets_fts) VALUES('rebuild')`)
	return err
}
