package heap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestPublishedHeapRowsRetainNoLinkCoordinates is a recursive publication
// gate.  Heap's builder may consume Link authorities while sealing, but the
// published schema type graph itself has no Project, Boundary, or Host path.
// OwnerCapability is deliberately atomic: it
// is Link's detached owner fence and its private state contains only a scalar
// content ID.
func TestPublishedHeapRowsRetainNoLinkCoordinates(t *testing.T) {
	roots := []reflect.Type{
		reflect.TypeOf(schema{}),
		reflect.TypeOf(ArtifactMount{}),
		reflect.TypeOf(rootRow{}),
		reflect.TypeOf(allocationSource{}),
		reflect.TypeOf(slotRow{}),
		reflect.TypeOf(indexAccessRow{}),
		reflect.TypeOf(Key{}),
		reflect.TypeOf(ExactKey{}),
		reflect.TypeOf(Slot{}),
		reflect.TypeOf(Reference{}),
		reflect.TypeOf(IndexAccessReceipt{}),
		reflect.TypeOf(AllocationReceipt{}),
	}
	seen := make(map[reflect.Type]bool)
	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, path string) {
		if typ == nil || seen[typ] {
			return
		}
		seen[typ] = true
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
			switch typ.Kind() {
			case reflect.Map:
				walk(typ.Key(), path+"[key]")
				typ = typ.Elem()
			default:
				typ = typ.Elem()
			}
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		if typ.PkgPath() == "github.com/wippyai/go-lua/analysis/program/link" && typ.Name() == "OwnerCapability" {
			return
		}
		if strings.Contains(typ.PkgPath(), "/program/link/") {
			t.Fatalf("published Heap carrier retains Link coordinate at %s: %s", path, typ)
		}
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			walk(field.Type, path+"."+field.Name)
		}
	}
	for _, root := range roots {
		walk(root, root.String())
	}
}

// TestHeapSealHasNoGlobalAuthorityRegistry prevents a future convenience
// cache from turning local construction authority into process-global Link
// retention.  The only admissible cold owner is heapBuilder's stack-local
// value in SealWithArtifacts.
func TestHeapSealHasNoGlobalAuthorityRegistry(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate Heap source")
	}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(testFile), "schema.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse Heap schema: %v", err)
	}
	forbidden := map[string]struct{}{
		"heapBuildContexts":   {},
		"registerHeapBuild":   {},
		"unregisterHeapBuild": {},
		"heapBuildFor":        {},
	}
	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if _, banned := forbidden[ident.Name]; banned {
			t.Fatalf("Heap sealing must remain stack-confined; found %s", ident.Name)
		}
		return true
	})
}
