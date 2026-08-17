package census

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDriftGuardRejectsChangedParserAction is the census's own catch arm. The
// checked-in rows are only evidence while they still describe the parser that
// ships, so a semantic action edited without regenerating the census must be
// refused rather than joined against. The mutation is applied to a real copy of
// the parser and AST sources, so the guard is exercised through the same
// derivation the generator uses and not through a hand-built value.
func TestDriftGuardRejectsChangedParserAction(t *testing.T) {
	root := moduleRoot(t)
	copied := copyParserSources(t, root)
	// The copy is the control: the checked-in census must validate against it,
	// so the rejection below is attributable to the mutation alone.
	if err := Generated.Validate(copied); err != nil {
		t.Fatalf("census rejected an unmodified copy of the parser sources: %v", err)
	}

	grammarPath := filepath.Join(copied, "compiler", "parse", "parser.go.y")
	contents, err := os.ReadFile(grammarPath)
	if err != nil {
		t.Fatal(err)
	}
	const original = "$$ = []ast.AnnotationExpr{$1}"
	const edited = "$$ = append([]ast.AnnotationExpr(nil), $1)"
	if strings.Count(string(contents), original) != 1 {
		t.Fatalf("parser.go.y does not state %q exactly once", original)
	}
	mutated := strings.Replace(string(contents), original, edited, 1)
	if err := os.WriteFile(grammarPath, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Generated.Validate(copied); err == nil {
		t.Fatal("census accepted a parser action it was not generated from")
	}
}

// TestDriftGuardRejectsStaleRows states the same guard from the other side: a
// census whose rows were edited without the parser changing is rejected too, so
// the checked-in file cannot be corrected by hand into agreement.
func TestDriftGuardRejectsStaleRows(t *testing.T) {
	root := moduleRoot(t)
	stale := clone(Generated)
	if len(stale.Productions) == 0 {
		t.Fatal("census states no productions")
	}
	stale.Productions = stale.Productions[:len(stale.Productions)-1]
	stale.Digest = digest(stale)
	if err := stale.Validate(root); err == nil {
		t.Fatal("census accepted a row set the parser does not produce")
	}
}

// copyParserSources materializes the exact source set the census derives from:
// the yacc grammar, the generated parser the AST discovery reads, and the AST
// declarations themselves.
func copyParserSources(t *testing.T, root string) string {
	t.Helper()
	target := t.TempDir()
	copyTree(t, filepath.Join(root, "compiler", "ast"), filepath.Join(target, "compiler", "ast"))
	parseSource := filepath.Join(root, "compiler", "parse")
	parseTarget := filepath.Join(target, "compiler", "parse")
	if err := os.MkdirAll(parseTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"parser.go.y", "parser.go"} {
		copyFile(t, filepath.Join(parseSource, name), filepath.Join(parseTarget, name))
	}
	return target
}

func copyTree(t *testing.T, source, target string) {
	t.Helper()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		copyFile(t, filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name()))
	}
}

func copyFile(t *testing.T, source, target string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
