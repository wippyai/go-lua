package architecture

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGeneratedPromptmapArtifactsStayOutOfRepoRoot(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"repo_map.md", "nodes.jsonl"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("generated promptmap artifact %s must not live at the repo root; write it under /tmp or a scoped artifact directory", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
