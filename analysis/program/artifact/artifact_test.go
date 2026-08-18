package artifact_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

const frozenArtifactFixtureCount = 1178

// TestArtifactFrozenCorpusRoundTrip proves persistence against the complete
// frozen parser-valid source denominator. It checks source semantics, not Go
// package layout: every Program is rebuilt through the only Seal path and
// must retain its identity and canonical artifact bytes.
func TestArtifactFrozenCorpusRoundTrip(t *testing.T) {
	contract := mustProfile(t)
	fixtures := artifactFixturePaths(t)
	if len(fixtures) != frozenArtifactFixtureCount {
		t.Fatalf("frozen artifact corpus = %d files, want %d", len(fixtures), frozenArtifactFixtureCount)
	}
	for _, fixture := range fixtures {
		source, err := os.ReadFile(fixture.absolute)
		if err != nil {
			t.Fatalf("read %s: %v", fixture.relative, err)
		}
		original, err := lower.Lower(lower.Source{Name: fixture.relative, Text: source})
		if err != nil {
			t.Fatalf("lower %s: %v", fixture.relative, err)
		}
		encoded, err := artifact.Encode(original, contract, artifact.Metadata{Provenance: fixture.relative})
		if err != nil {
			t.Fatalf("encode %s: %v", fixture.relative, err)
		}
		replayed, metadata, err := artifact.Decode(encoded, contract, nil)
		if err != nil {
			t.Fatalf("decode %s: %v", fixture.relative, err)
		}
		if replayed.ContentID() != original.ContentID() {
			t.Fatalf("ContentID changed for %s", fixture.relative)
		}
		reencoded, err := artifact.Encode(replayed, contract, metadata)
		if err != nil || !bytes.Equal(encoded, reencoded) {
			t.Fatalf("artifact bytes changed for %s: %v", fixture.relative, err)
		}
	}
}

type artifactFixturePath struct{ absolute, relative string }

func artifactFixturePaths(t *testing.T) []artifactFixturePath {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate artifact corpus test")
	}
	repository := artifactRepositoryRoot(t, filepath.Dir(file))
	root := filepath.Join(repository, "testdata", "fixtures")
	var paths []artifactFixturePath
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		relative, err := filepath.Rel(repository, path)
		if err != nil {
			return err
		}
		paths = append(paths, artifactFixturePath{absolute: path, relative: filepath.ToSlash(relative)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Slice(paths, func(left, right int) bool { return paths[left].relative < paths[right].relative })
	return paths
}

func artifactRepositoryRoot(t *testing.T, start string) string {
	t.Helper()
	for directory := filepath.Clean(start); ; {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && !info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("locate repository root")
		}
		directory = parent
	}
}
