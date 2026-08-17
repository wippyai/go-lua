package stdlib

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// luaTypeName spells the eight family names as literals rather than projecting
// them, because this signature table is a leaf of module lookup and the one
// place the analyzer declares those names is the sealed structural catalog,
// which carries the whole program declaration graph behind it. This law is what
// keeps the literals honest: it derives the expected result domain from that
// declaration and states the two are the same type, so a family renamed,
// added, or removed in the vocabulary is a rejected build here rather than a
// call result that silently disagrees with what type(v) can yield.

func declaredFamilyNames(t *testing.T) []typ.Type {
	t.Helper()
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural vocabulary")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	names := make([]typ.Type, 0, int(runtimekind.Count)-1)
	for kind := runtimekind.Invalid + 1; kind < runtimekind.Count; kind++ {
		entry, ok := table.At(structure.CategoryRuntimeKind, uint16(kind))
		if !ok {
			t.Fatalf("runtime family %d names no member of the sealed vocabulary", kind)
		}
		if !entry.Key().Available() {
			t.Fatalf("runtime family %d is declared without a name", kind)
		}
		names = append(names, typ.LiteralString(string(entry.Key())))
	}
	return names
}

// TestLuaTypeNameIsTheDeclaredFamilyVocabulary states the result domain of
// type(v) is exactly the declared family names, no member held back and no
// name invented beside them.
func TestLuaTypeNameIsTheDeclaredFamilyVocabulary(t *testing.T) {
	names := declaredFamilyNames(t)
	if want := int(runtimekind.Count) - 1; len(names) != want {
		t.Fatalf("declaration yields %d family names, but the closed vocabulary has %d", len(names), want)
	}
	if !typ.TypeEquals(luaTypeName, typ.MaterializeUnion(names)) {
		t.Fatalf("type(v) result domain %v is not the declared family vocabulary %v", luaTypeName, typ.MaterializeUnion(names))
	}
}

// TestLuaTypeNameRejectsAnUndeclaredName states the law above discriminates:
// a result domain carrying a name the vocabulary does not declare fails it,
// so the law cannot pass by accident on any union of string literals.
func TestLuaTypeNameRejectsAnUndeclaredName(t *testing.T) {
	names := declaredFamilyNames(t)
	if typ.TypeEquals(luaTypeName, typ.MaterializeUnion(append(names, typ.LiteralString("integer")))) {
		t.Fatal("type(v) result domain admitted a name the family vocabulary does not declare")
	}
	if typ.TypeEquals(luaTypeName, typ.MaterializeUnion(names[:len(names)-1])) {
		t.Fatal("type(v) result domain matched a vocabulary missing a declared family")
	}
}
