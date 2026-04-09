package query

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"claude-code-running-go/src/services/api"
)

type TranscriptStore interface {
	Load() ([]api.Message, error)
	Append(msgs []api.Message) error
}

type FileTranscriptStore struct {
	Path string
}

func defaultTranscriptPath() string {
	if p := strings.TrimSpace(os.Getenv("CLAUDE_GO_TRANSCRIPT_FILE")); p != "" {
		return p
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ".claude-go", "transcript.jsonl")
}

func NewFileTranscriptStore(path string) *FileTranscriptStore {
	if strings.TrimSpace(path) == "" {
		path = defaultTranscriptPath()
	}
	return &FileTranscriptStore{Path: path}
}

func (s *FileTranscriptStore) Load() ([]api.Message, error) {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return nil, nil
	}
	f, err := os.Open(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	out := make([]api.Message, 0, 128)
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 10*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m api.Message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *FileTranscriptStore) Append(msgs []api.Message) error {
	if s == nil || strings.TrimSpace(s.Path) == "" || len(msgs) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return nil
}

