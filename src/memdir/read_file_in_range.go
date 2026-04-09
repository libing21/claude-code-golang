package memdir

import (
	"bufio"
	"context"
	"os"
	"strings"
)

// Port of TS utils/readFileInRange + its usage by extractMemories in attachments.ts.

type readFileResult struct {
	Content          string
	LineCount        int // number of lines included in Content
	TotalLines       int // best-effort; in this Go port equals LineCount when early-stop
	TruncatedByBytes bool
}

func readFileInRange(ctx context.Context, path string, maxLines int, maxBytes int, truncateOnByteLimit bool) (readFileResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return readFileResult{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lines := make([]string, 0, maxLines)
	totalBytes := 0
	truncatedByBytes := false

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return readFileResult{}, ctx.Err()
		default:
		}
		line := sc.Text()
		if maxBytes > 0 {
			// +1 for newline
			if totalBytes+len(line)+1 > maxBytes {
				if truncateOnByteLimit {
					remaining := maxBytes - totalBytes
					if remaining > 0 {
						// Keep a partial line if needed.
						if remaining-1 > 0 && remaining-1 <= len(line) {
							lines = append(lines, line[:remaining-1])
						}
					}
					truncatedByBytes = true
					break
				}
			}
		}
		lines = append(lines, line)
		totalBytes += len(line) + 1
		if maxLines > 0 && len(lines) >= maxLines {
			break
		}
	}
	// Note: we intentionally do not scan the rest of the file for TotalLines
	// to keep syscall/IO costs bounded (mirrors TS rationale).

	content := strings.Join(lines, "\n")
	return readFileResult{
		Content:          content,
		LineCount:        len(lines),
		TotalLines:       len(lines),
		TruncatedByBytes: truncatedByBytes,
	}, nil
}

