package analysis

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestEngineProductionImportsRemainDomainBlind makes the engine/domain
// direction an executable architecture law. The root analysis package may
// compose domains with Engine; Engine itself may depend only on its generic
// implementation, the shared lattice, and analysis-internal codecs.
func TestEngineProductionImportsRemainDomainBlind(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	engineRoot := filepath.Join(filepath.Dir(current), "engine")
	const module = "github.com/wippyai/go-lua"
	allowed := func(path string) bool {
		return path == module+"/analysis/lattice" ||
			strings.HasPrefix(path, module+"/analysis/engine") ||
			strings.HasPrefix(path, module+"/analysis/internal/")
	}
	err := filepath.WalkDir(engineRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imported := range parsed.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if strings.HasPrefix(value, module+"/") && !allowed(value) {
				t.Errorf("engine production source %s imports semantic owner %s", filepath.Base(path), value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
