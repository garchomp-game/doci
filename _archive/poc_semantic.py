#!/usr/bin/env python3
"""
Semantic search for file-indexer-go using Ollama + nomic-embed-text.
Usage:
  ./semantic.py embed          # Generate embeddings for all chunks
  ./semantic.py search "query" # Semantic search
  ./semantic.py compare "query" # Compare FTS5 vs Semantic results
"""

import json
import os
import sqlite3
import struct
import sys
import time
import urllib.request

import numpy as np

DB_PATH = os.path.expanduser("~/file-indexer-go/file_index.db")
OLLAMA_URL = "http://localhost:11434/api/embed"
MODEL = "nomic-embed-text"
BATCH_SIZE = 20  # chunks per API call

# =============================================================================
# Embedding helpers
# =============================================================================

def ollama_embed(texts: list[str]) -> list[list[float]]:
    """Call Ollama embedding API."""
    payload = json.dumps({"model": MODEL, "input": texts}).encode()
    req = urllib.request.Request(OLLAMA_URL, data=payload,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=60) as resp:
        data = json.loads(resp.read())
    return data["embeddings"]


def vec_to_blob(vec: list[float]) -> bytes:
    """Pack float list into binary blob for SQLite storage."""
    return struct.pack(f"{len(vec)}f", *vec)


def blob_to_vec(blob: bytes) -> np.ndarray:
    """Unpack binary blob to numpy array."""
    n = len(blob) // 4
    return np.array(struct.unpack(f"{n}f", blob), dtype=np.float32)


def cosine_sim(a: np.ndarray, b: np.ndarray) -> float:
    dot = np.dot(a, b)
    norm = np.linalg.norm(a) * np.linalg.norm(b)
    return float(dot / norm) if norm > 0 else 0.0

# =============================================================================
# Commands
# =============================================================================

def cmd_embed():
    """Generate embeddings for all chunks and store in SQLite."""
    print("🧠 Embedding生成: nomic-embed-text via Ollama\n")
    
    db = sqlite3.connect(DB_PATH)
    
    # Create embeddings table
    db.execute("""
        CREATE TABLE IF NOT EXISTS embeddings (
            snippet_id INTEGER PRIMARY KEY,
            file_id    INTEGER NOT NULL,
            embedding  BLOB NOT NULL
        )
    """)
    db.execute("DELETE FROM embeddings")
    db.commit()
    
    # Get all chunks
    rows = db.execute("""
        SELECT s.id, s.file_id, s.content
        FROM snippets s
        ORDER BY s.id
    """).fetchall()
    
    total = len(rows)
    print(f"  📄 {total} チャンク\n")
    
    start = time.time()
    processed = 0
    dim = None
    
    for i in range(0, total, BATCH_SIZE):
        batch = rows[i:i+BATCH_SIZE]
        texts = [r[2][:2000] for r in batch]  # truncate long chunks
        
        try:
            embeddings = ollama_embed(texts)
        except Exception as e:
            print(f"\n  ⚠️ Ollama error at batch {i}: {e}")
            continue
        
        if dim is None:
            dim = len(embeddings[0])
            print(f"  🔢 次元数: {dim}\n")
        
        insert_data = []
        for j, (sid, fid, _) in enumerate(batch):
            if j < len(embeddings):
                insert_data.append((sid, fid, vec_to_blob(embeddings[j])))
        
        db.executemany(
            "INSERT INTO embeddings (snippet_id, file_id, embedding) VALUES (?, ?, ?)",
            insert_data
        )
        
        processed += len(batch)
        elapsed = time.time() - start
        rate = processed / elapsed if elapsed > 0 else 0
        print(f"\r  🧠 {processed}/{total} ({rate:.0f} chunks/s) | ⏱ {elapsed:.1f}s", 
              end="", flush=True)
    
    db.commit()
    
    # Create index
    db.execute("CREATE INDEX IF NOT EXISTS idx_emb_file ON embeddings(file_id)")
    db.commit()
    db.close()
    
    elapsed = time.time() - start
    print(f"\n\n✅ Embedding完了: {processed} チャンク, {dim}次元, {elapsed:.1f}秒")


def cmd_search(query: str, limit: int = 10):
    """Semantic search using cosine similarity."""
    print(f"\n🔮 セマンティック検索: \"{query}\"\n")
    
    start = time.time()
    
    # Embed query
    q_vec = np.array(ollama_embed([query])[0], dtype=np.float32)
    embed_time = time.time() - start
    
    db = sqlite3.connect(DB_PATH)
    
    # Load all embeddings
    rows = db.execute("""
        SELECT e.snippet_id, e.file_id, e.embedding, s.content
        FROM embeddings e
        JOIN snippets s ON s.id = e.snippet_id
    """).fetchall()
    
    # Compute similarities
    results = []
    for sid, fid, blob, content in rows:
        vec = blob_to_vec(blob)
        sim = cosine_sim(q_vec, vec)
        results.append((sim, sid, fid, content))
    
    results.sort(reverse=True)
    
    search_time = time.time() - start
    
    # Get file paths
    file_cache = {}
    for row in db.execute("SELECT id, path FROM files"):
        file_cache[row[0]] = row[1]
    
    home = os.path.expanduser("~")
    
    # Deduplicate by file (show best chunk per file)
    seen_files = set()
    shown = 0
    
    print(f"  ⏱ クエリembed: {embed_time*1000:.0f}ms | 類似度計算: {(search_time-embed_time)*1000:.1f}ms\n")
    
    for sim, sid, fid, content in results:
        if fid in seen_files:
            continue
        seen_files.add(fid)
        
        path = file_cache.get(fid, "?").replace(home, "~")
        snippet = content.replace("\n", " ")[:100]
        
        bar = "█" * int(sim * 20) + "░" * (20 - int(sim * 20))
        print(f"  {sim:.3f} |{bar}| {path}")
        print(f"         {snippet}...")
        print()
        
        shown += 1
        if shown >= limit:
            break
    
    db.close()
    print(f"  📊 {shown} 件 ({search_time*1000:.1f}ms)")


def cmd_compare(query: str, limit: int = 5):
    """Compare FTS5 keyword search vs Semantic search."""
    print(f"\n{'='*60}")
    print(f"🔍 比較: \"{query}\"")
    print(f"{'='*60}")
    
    db = sqlite3.connect(DB_PATH)
    home = os.path.expanduser("~")
    
    # --- FTS5 ---
    print(f"\n📝 FTS5 (キーワード一致):")
    fts_start = time.time()
    try:
        fts_rows = db.execute("""
            SELECT f.path, snippet(snippets_fts, 0, '>>>', '<<<', '...', 30)
            FROM snippets_fts
            JOIN snippets s ON s.id = snippets_fts.rowid
            JOIN files f ON f.id = s.file_id
            WHERE snippets_fts MATCH ?
            ORDER BY rank LIMIT ?
        """, (query, limit * 3)).fetchall()
    except Exception as e:
        fts_rows = []
        print(f"  ⚠️ FTS5エラー: {e}")
    fts_time = time.time() - fts_start
    
    seen = set()
    count = 0
    for path, snip in fts_rows:
        if path in seen:
            continue
        seen.add(path)
        p = path.replace(home, "~")
        s = (snip or "").replace("\n", " ")[:80]
        print(f"  {p}")
        print(f"    {s}")
        count += 1
        if count >= limit:
            break
    print(f"  → {count} 件 ({fts_time*1000:.1f}ms)")
    
    # --- Semantic ---
    print(f"\n🔮 セマンティック (意味類似):")
    sem_start = time.time()
    q_vec = np.array(ollama_embed([query])[0], dtype=np.float32)
    
    emb_rows = db.execute("""
        SELECT e.file_id, e.embedding, s.content
        FROM embeddings e
        JOIN snippets s ON s.id = e.snippet_id
    """).fetchall()
    
    file_cache = {}
    for row in db.execute("SELECT id, path FROM files"):
        file_cache[row[0]] = row[1]
    
    results = []
    for fid, blob, content in emb_rows:
        vec = blob_to_vec(blob)
        sim = cosine_sim(q_vec, vec)
        results.append((sim, fid, content))
    
    results.sort(reverse=True)
    sem_time = time.time() - sem_start
    
    seen = set()
    count = 0
    for sim, fid, content in results:
        if fid in seen:
            continue
        seen.add(fid)
        p = file_cache.get(fid, "?").replace(home, "~")
        s = content.replace("\n", " ")[:80]
        print(f"  {sim:.3f} {p}")
        print(f"        {s}")
        count += 1
        if count >= limit:
            break
    
    print(f"  → {count} 件 ({sem_time*1000:.1f}ms)")
    
    db.close()
    print(f"\n{'='*60}")


# =============================================================================
# Main
# =============================================================================

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)
    
    cmd = sys.argv[1]
    
    if cmd == "embed":
        cmd_embed()
    elif cmd == "search":
        if len(sys.argv) < 3:
            print("Usage: semantic.py search \"query\"")
            sys.exit(1)
        cmd_search(sys.argv[2])
    elif cmd == "compare":
        if len(sys.argv) < 3:
            print("Usage: semantic.py compare \"query\"")
            sys.exit(1)
        cmd_compare(sys.argv[2])
    else:
        print(f"Unknown command: {cmd}")
        print(__doc__)
        sys.exit(1)
