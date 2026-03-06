package indexer

import (
	"strings"
)

const (
	DefaultChunkLines   = 30
	DefaultOverlapLines = 10
)

// Chunk represents a text fragment from a file.
type Chunk struct {
	FileID  int64
	ChunkNo int
	Content string
}

// ChunkText splits text content into overlapping chunks.
// Each chunk contains chunkLines lines, with overlapLines of overlap
// from the previous chunk to prevent token splitting at boundaries.
func ChunkText(content string, fileID int64, chunkLines int) []Chunk {
	lines := strings.Split(content, "\n")
	var chunks []Chunk

	overlap := DefaultOverlapLines
	if overlap >= chunkLines {
		overlap = chunkLines / 3
	}

	step := chunkLines - overlap
	if step < 1 {
		step = 1
	}

	chunkNo := 0
	for i := 0; i < len(lines); i += step {
		end := i + chunkLines
		if end > len(lines) {
			end = len(lines)
		}
		chunk := strings.Join(lines[i:end], "\n")
		if strings.TrimSpace(chunk) != "" {
			chunks = append(chunks, Chunk{
				FileID:  fileID,
				ChunkNo: chunkNo,
				Content: chunk,
			})
			chunkNo++
		}
		// If this chunk reached the end, stop
		if end >= len(lines) {
			break
		}
	}
	return chunks
}
