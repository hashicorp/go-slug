package copyUtil

import (
	"errors"
	"io"
)

const (
	chunkSize = 4 * 1024 * 1024 // 4MB per chunk
	maxChunks = 100             // Up to 400MB
)

// copyWithLimit copies from a tar.Reader to a file in limited-size chunks
func CopyWithLimit(dst io.Writer, src io.Reader) error {
	var totalChunks int

	for {
		if totalChunks >= maxChunks {
			return errors.New("copy limit exceeded")
		}

		n, err := io.CopyN(dst, src, chunkSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break // Copy complete
			}
			return err
		}

		if n < chunkSize {
			break // No more data left
		}

		totalChunks++
	}

	return nil
}
