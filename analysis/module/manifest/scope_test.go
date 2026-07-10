package manifest

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestScopeTypeResolvesBareReferencesInOwningManifest(t *testing.T) {
	entry := typetable.NewRecord().Field("id", typ.String).Build()
	errType := typ.NewInterface("Error", nil)
	m := New("registry")
	m.DefineType("Entry", entry)
	m.DefineType("Error", errType)

	scoped := m.ScopeType(typ.Func().
		Returns(typ.NewRef("", "Entry"), typeexpr.Optional(typ.NewRef("", "Error"))).
		Build())
	fn, ok := scoped.(*typ.Function)
	if !ok || len(fn.Returns) != 2 {
		t.Fatalf("ScopeType(function) = %T %[1]v, want two-return function", scoped)
	}
	if !typ.TypeEquals(fn.Returns[0], entry) {
		t.Fatalf("first return = %v, want registry Entry %v", fn.Returns[0], entry)
	}
	if !typ.TypeEquals(fn.Returns[1], typeexpr.Optional(errType)) {
		t.Fatalf("second return = %v, want registry Error? %v", fn.Returns[1], typeexpr.Optional(errType))
	}
}

func TestScopeTypeKeepsUnknownBareReferenceModuleQualified(t *testing.T) {
	m := New("registry")
	scoped := m.ScopeType(typ.NewRef("", "Missing"))
	want := typ.NewRef("registry", "Missing")
	if !typ.TypeEquals(scoped, want) {
		t.Fatalf("ScopeType(Missing) = %v, want module-qualified unresolved reference %v", scoped, want)
	}
}

func TestScopeTypePreservesRecursiveLocalIdentityAcrossLookups(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Build()
	})
	m := New("tree")
	m.DefineType("Node", node)

	scoped := m.ScopeType(typ.Func().Returns(typ.NewRef("", "Node")).Build())
	fn, ok := scoped.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || fn.Returns[0] != node {
		t.Fatalf("ScopeType(return Node) = %v, want manifest Node recursive identity %v", scoped, node)
	}

	rescoped := m.ScopeType(scoped)
	fn, ok = rescoped.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || fn.Returns[0] != node {
		t.Fatalf("second ScopeType(return Node) = %v, want the same recursive identity %v", rescoped, node)
	}
}
