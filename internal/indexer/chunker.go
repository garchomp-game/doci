package indexer

import (
	"strings"
)

const DefaultChunkLines = 50

// Chunk represents a text fragment from a file.
type Chunk struct {
	FileID  int64
	ChunkNo int
	Content string
}

// ChunkText splits text content into chunks of chunkLines lines each.
func ChunkText(content string, fileID int64, chunkLines int) []Chunk {
	lines := strings.Split(content, "\n")
	var chunks []Chunk

	for i := 0; i < len(lines); i += chunkLines {
		end := i + chunkLines
		if end > len(lines) {
			end = len(lines)
		}
		chunk := strings.Join(lines[i:end], "\n")
		if strings.TrimSpace(chunk) != "" {
			chunks = append(chunks, Chunk{
				FileID:  fileID,
				ChunkNo: i / chunkLines,
				Content: chunk,
			})
		}
	}
	return chunks
}
