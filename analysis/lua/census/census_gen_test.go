package census

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
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
	declarations, err := parsersource.Discover(root)
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

// TestProductionsAttributeHelperConstructedForms states the attribution law a
// disposition join depends on: a production row's constructor column is the
// complete set of AST forms its reduction builds, whether the action writes the
// composite literal itself or delegates to a parser.go.y helper. Without it an
// empty column would mean "builds nothing" and "builds something through a
// helper" alike, and a structural disposition could be claimed for a reduction
// that constructs a semantic form.
func TestProductionsAttributeHelperConstructedForms(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string][]string, len(value.Productions))
	for _, production := range value.Productions {
		byKey[production.Key] = production.Constructors
	}
	// Method-call syntax builds its call through the callExpr helper, and an
	// annotation builds its value through the annotationExpr helper. Neither
	// action contains an ast composite literal for the form it produces.
	for key, want := range map[string]string{
		"functioncall#3": "FuncCallExpr",
		"functioncall#4": "FuncCallExpr",
		"annotation#1":   "AnnotationExpr",
		"annotation#3":   "AnnotationExpr",
	} {
		constructors, known := byKey[key]
		if !known {
			t.Fatalf("production %s is absent from the census", key)
		}
		if !contains(constructors, want) {
			t.Fatalf("production %s constructs %s through a helper; census cites %v", key, want, constructors)
		}
	}
	// A reduction which only threads an already-built value constructs nothing,
	// so the column stays empty and the structural disposition remains provable.
	for _, key := range []string{"expr#7", "typeexpr#1", "exprlist#2", "chunk1#1"} {
		constructors, known := byKey[key]
		if !known {
			t.Fatalf("production %s is absent from the census", key)
		}
		if len(constructors) != 0 {
			t.Fatalf("pass-through production %s cites constructors %v", key, constructors)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate parser census source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
