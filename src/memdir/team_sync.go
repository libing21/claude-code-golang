package memdir

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SyncTeamMemOnce is a minimal local-only analog of TS team memory sync.
// TS syncs with a remote service; in this Go port we optionally "pull" from a
// local remote directory for learnability and deterministic behavior.
//
// Enable with:
// - CLAUDE_GO_TEAMMEM_SYNC=1
// - CLAUDE_GO_TEAMMEM_REMOTE_DIR=/abs/path/to/remote/root
func SyncTeamMemOnce(cwd string, teamDir string) error {
	if !envTruthy("CLAUDE_GO_TEAMMEM_SYNC") {
		return nil
	}
	remoteRoot := strings.TrimSpace(os.Getenv("CLAUDE_GO_TEAMMEM_REMOTE_DIR"))
	if remoteRoot == "" {
		return nil
	}
	remoteRoot = filepath.Clean(remoteRoot)
	_ = cwd

	// Mirror only the team directory files from remoteRoot/<projectSlug>/team -> teamDir.
	projectSlug := sanitizeProjectSlug(cwd)
	remoteTeam := filepath.Join(remoteRoot, projectSlug, "team")
	if _, err := os.Stat(remoteTeam); err != nil {
		return nil
	}
	return copyDir(remoteTeam, teamDir)
}

func copyDir(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

