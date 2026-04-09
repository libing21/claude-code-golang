package memdir

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Port of TS src/memdir/memoryScan.ts

type MemoryHeader struct {
	Filename    string  `json:"filename"` // relative path under memoryDir
	FilePath    string  `json:"filePath"` // absolute
	MtimeMs     int64   `json:"mtimeMs"`
	Description *string `json:"description,omitempty"`
	Type        *string `json:"type,omitempty"`
}

const (
	MAX_MEMORY_FILES      = 200
	FRONTMATTER_MAX_LINES = 30
)

func scanMemoryFiles(ctx context.Context, memoryDir string) ([]MemoryHeader, error) {
	var headers []MemoryHeader

	err := filepath.WalkDir(memoryDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		if d.Name() == ENTRYPOINT_NAME {
			return nil
		}
		rel, err := filepath.Rel(memoryDir, path)
		if err != nil {
			return nil
		}

		content, mtimeMs, err := readFileFirstLines(path, FRONTMATTER_MAX_LINES)
		if err != nil {
			return nil
		}
		fm := parseFrontmatter(content)
		var desc *string
		if s := strings.TrimSpace(fm["description"]); s != "" {
			desc = &s
		}
		var typ *string
		if s := strings.TrimSpace(fm["type"]); s != "" {
			// Keep the raw string; TS parses into MemoryType enum.
			typ = &s
		}

		headers = append(headers, MemoryHeader{
			Filename:    filepath.ToSlash(rel),
			FilePath:    path,
			MtimeMs:     mtimeMs,
			Description: desc,
			Type:        typ,
		})
		return nil
	})
	if err != nil && err != context.Canceled {
		// best effort: ignore scan errors
	}

	sort.Slice(headers, func(i, j int) bool { return headers[i].MtimeMs > headers[j].MtimeMs })
	if len(headers) > MAX_MEMORY_FILES {
		headers = headers[:MAX_MEMORY_FILES]
	}
	return headers, nil
}

func readFileFirstLines(path string, maxLines int) (string, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := make([]string, 0, maxLines)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) >= maxLines {
			break
		}
	}
	mtimeMs := info.ModTime().UnixNano() / int64(time.Millisecond)
	return strings.Join(lines, "\n"), mtimeMs, nil
}

func parseFrontmatter(content string) map[string]string {
	// Minimal YAML frontmatter parser: reads between leading --- ... ---.
	out := map[string]string{}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for i := 1; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if l == "---" {
			break
		}
		col := strings.Index(l, ":")
		if col <= 0 {
			continue
		}
		k := strings.TrimSpace(l[:col])
		v := strings.TrimSpace(l[col+1:])
		v = strings.Trim(v, `"'`)
		out[k] = v
	}
	return out
}

func formatMemoryManifest(memories []MemoryHeader) string {
	var b strings.Builder
	for i, m := range memories {
		tag := ""
		if m.Type != nil && strings.TrimSpace(*m.Type) != "" {
			tag = "[" + strings.TrimSpace(*m.Type) + "] "
		}
		ts := time.Unix(0, m.MtimeMs*int64(time.Millisecond)).UTC().Format(time.RFC3339Nano)
		if m.Description != nil && strings.TrimSpace(*m.Description) != "" {
			b.WriteString("- " + tag + m.Filename + " (" + ts + "): " + strings.TrimSpace(*m.Description))
		} else {
			b.WriteString("- " + tag + m.Filename + " (" + ts + ")")
		}
		if i != len(memories)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

