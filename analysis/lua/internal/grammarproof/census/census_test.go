package census

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/grammar"
)

func TestGeneratedCensusIsCurrent(t *testing.T) {
	root := moduleRoot(t)
	if _, err := Current(root); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedCensusCoversParserAndASTSources(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Productions) == 0 || len(value.Constructors) == 0 {
		t.Fatalf("incomplete parser census: productions=%d constructors=%d", len(value.Productions), len(value.Constructors))
	}
	declarations, err := grammar.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	known := make(map[string]bool, len(declarations.Declarations))
	for _, declaration := range declarations.Declarations {
		known[declaration.Name] = true
	}
	for _, declaration := range declarations.Types {
		known[declaration.Name] = true
	}
	for _, constructor := range value.Constructors {
		if !known[constructor.Name] {
			t.Fatalf("constructor %s is absent from the AST declaration census", constructor.Name)
		}
		for index, field := range constructor.Fields {
			if field.Ordinal != index || field.Name == "" || field.Type == "" {
				t.Fatalf("invalid constructor field: %#v", field)
			}
		}
	}
	for _, production := range value.Productions {
		for _, constructor := range production.Constructors {
			if !known[constructor] {
				t.Fatalf("production %s cites undeclared AST constructor %s", production.Key, constructor)
			}
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate parser census source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", ".."))
}
