package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func analysisSourcePath(t testing.TB, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source path unavailable")
	}
	return filepath.Join(filepath.Dir(file), name)
}

func TestBuildDetachedArtifactResultHasNoArtifactTraversal(t *testing.T) {
	source, err := os.ReadFile(analysisSourcePath(t, "artifact_result.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func buildDetachedArtifactResult(")
	if start < 0 {
		t.Fatal("buildDetachedArtifactResult boundary unavailable")
	}
	end := strings.Index(text[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("buildDetachedArtifactResult end unavailable")
	}
	body := text[start : start+end]
	for _, forbidden := range []string{
		"mountedProgramArtifact",
		"mount.artifact",
		"BodyCount()",
		"BodyAt(",
		"OccurrenceCount()",
		"OccurrenceAt(",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("detached result still traverses artifact geometry: %q", forbidden)
		}
	}
}

func TestAnalysisHasNoTransformerFacadeImports(t *testing.T) {
	root := analysisSourcePath(t, ".")
	const forbiddenImport = "analysis/internal/transformer"
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "__legacy" || entry.Name() == "_reference" || entry.Name() == "transformer" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == "artifact_result_static_law_test.go" || filepath.Ext(path) != ".go" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), forbiddenImport) || strings.Contains(string(data), "package transformer") {
			t.Fatalf("transformer facade reference remains in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
