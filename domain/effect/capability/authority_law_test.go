package capability_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/effect/capability"
)

// TestEachCapabilityIDHasOneAuthoringSite states that a capability ID is
// authored once. The dotted string is the audited name of a capability, and the
// catalog is where it is spelled; any second production spelling is a copy that
// can drift away from the catalog without a compiler or a test noticing.
func TestEachCapabilityIDHasOneAuthoringSite(t *testing.T) {
	root := moduleRoot(t)
	sources := productionSources(t, root)

	for _, desc := range capability.All() {
		needle := `"` + desc.ID + `"`
		sites := []string{}
		for _, source := range sources {
			count := strings.Count(source.text, needle)
			for i := 0; i < count; i++ {
				sites = append(sites, source.path)
			}
		}
		if len(sites) != 1 {
			t.Errorf("capability ID %s is authored at %d production sites, want 1: %v", desc.ID, len(sites), sites)
		}
	}
}

type goSource struct {
	path string
	text string
}

// productionSources reads every non-test Go file the module builds from.
func productionSources(t *testing.T, root string) []goSource {
	t.Helper()

	var sources []goSource
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if info.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		sources = append(sources, goSource{path: rel, text: string(data)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no production Go sources found")
	}
	return sources
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above working directory")
		}
		dir = parent
	}
}
