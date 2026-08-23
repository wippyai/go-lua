package keymatch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keymatch "github.com/wippyai/go-lua/domain/heap/keymatch"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestSealedSelectorProjectionEqualsAPerBindingBuild is the equality half of
// the seal: the projection is a pure function of the two sealed schemas, so
// one seal handed to every reader denotes exactly what each reader would have
// built for itself. The law compares two independent builds over the same
// pair on all three observations - the selector image, the full heap-observable
// class quotient, and the coarser payload quotient - over Top and over every
// singleton the sealed atom universe contains.
func TestSealedSelectorProjectionEqualsAPerBindingBuild(t *testing.T) {
	heap, values, _ := fixture(t, "keymatch_selector_projection_seal", selectorProjectionSource(8))
	sealed, sealedOK := keymatch.NewSelectorProjection(heap, values)
	perBinding, perBindingOK := keymatch.NewSelectorProjection(heap, values)
	if !sealedOK || !perBindingOK {
		t.Fatal("selector projection")
	}

	subjects := []valuedomain.Value{values.Top()}
	var atoms []valuedomain.Atom
	if !values.VisitSupport(values.Top(), func(atom valuedomain.Atom) { atoms = append(atoms, atom) }) {
		t.Fatal("Top support")
	}
	if len(atoms) == 0 {
		t.Fatal("sealed atom universe is empty")
	}
	for _, atom := range atoms {
		singleton, ok := values.Singleton(atom)
		if !ok {
			t.Fatal("singleton relation")
		}
		subjects = append(subjects, singleton)
	}

	for index, subject := range subjects {
		if got, want := projectedSelectors(t, sealed, subject), projectedSelectors(t, perBinding, subject); !sameSelectorSequence(got, want) {
			t.Fatalf("subject %d: sealed selector image differs from a per-binding build", index)
		}
		if got, want := visitedClasses(t, sealed, subject, false), visitedClasses(t, perBinding, subject, false); !sameAtomSequence(got, want) {
			t.Fatalf("subject %d: sealed class quotient differs from a per-binding build", index)
		}
		if got, want := visitedClasses(t, sealed, subject, true), visitedClasses(t, perBinding, subject, true); !sameAtomSequence(got, want) {
			t.Fatalf("subject %d: sealed payload quotient differs from a per-binding build", index)
		}
	}
}

// TestSealedSelectorProjectionRefusesAForeignPair states the fence the seal
// replaces a rebuild with: a reader that receives a projection proves it
// belongs to the exact schema pair it is about to read, so handing one seal to
// many readers cannot silently cross a Link.
func TestSealedSelectorProjectionRefusesAForeignPair(t *testing.T) {
	heap, values, _ := fixture(t, "keymatch_selector_projection_fence", selectorProjectionSource(4))
	foreignHeap, foreignValues, _ := fixture(t, "keymatch_selector_projection_fence_foreign", selectorProjectionSource(3))
	projection, ok := keymatch.NewSelectorProjection(heap, values)
	if !ok {
		t.Fatal("selector projection")
	}
	if !projection.FencedTo(heap, values) {
		t.Fatal("sealed projection refused its own schema pair")
	}
	if projection.FencedTo(foreignHeap, values) {
		t.Fatal("sealed projection admitted a foreign Heap schema")
	}
	if projection.FencedTo(heap, foreignValues) {
		t.Fatal("sealed projection admitted a foreign Value schema")
	}
	var absent *keymatch.SelectorProjection
	if absent.FencedTo(heap, values) {
		t.Fatal("an absent projection authenticated a schema pair")
	}
}

// TestSelectorProjectionIsConstructedOnceInTheComposition is the placement
// half. The projection is a derivation of the sealed Link, so exactly one
// non-test declaration in the module constructs one, and it is the mount
// phase's own derivation step. Every rule that quotients Value alternatives by
// what Heap observes reads that seal.
//
// It is a source law because where a value is constructed is not something the
// value can state about itself.
func TestSelectorProjectionIsConstructedOnceInTheComposition(t *testing.T) {
	root := moduleRoot(t)
	var sites []string
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".goseam", "testdata", "vendor":
				return filepath.SkipDir
			}
			if strings.HasPrefix(entry.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fileSet := token.NewFileSet()
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "NewSelectorProjection" {
				return true
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relative = path
			}
			sites = append(sites, filepath.ToSlash(relative))
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if len(sites) != 1 || sites[0] != "domain/composite/mount_table.go" {
		t.Fatalf("selector projection is constructed at %v, want exactly domain/composite/mount_table.go", sites)
	}
}

func visitedClasses(t testing.TB, projection *keymatch.SelectorProjection, value valuedomain.Value, payloadRole bool) []valuedomain.Atom {
	t.Helper()
	var atoms []valuedomain.Atom
	visit := projection.VisitClasses
	if payloadRole {
		visit = projection.VisitPayloadClasses
	}
	if !visit(value, func(atom valuedomain.Atom) bool {
		atoms = append(atoms, atom)
		return true
	}) {
		t.Fatal("class quotient")
	}
	return atoms
}

func sameAtomSequence(left, right []valuedomain.Atom) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func moduleRoot(t testing.TB) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("module root was not found")
		}
		directory = parent
	}
}
