package toolruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DiscoveredToolsStore persists the discovered deferred-tools set so defer_loading
// can survive restarts/compaction/replay.
type DiscoveredToolsStore interface {
	Load() ([]string, error)
	Save(names []string) error
}

type FileDiscoveredToolsStore struct {
	Path string
}

type discoveredToolsFile struct {
	Version int      `json:"version"`
	Names   []string `json:"names"`
}

func NewFileDiscoveredToolsStore(path string) *FileDiscoveredToolsStore {
	return &FileDiscoveredToolsStore{Path: path}
}

func (s *FileDiscoveredToolsStore) Load() ([]string, error) {
	if s == nil || s.Path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f discoveredToolsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f.Names, nil
}

func (s *FileDiscoveredToolsStore) Save(names []string) error {
	if s == nil || s.Path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(discoveredToolsFile{Version: 1, Names: names}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

