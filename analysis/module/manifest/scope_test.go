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

func TestScopeTypeResolvesNestedLocalDefinitionReferences(t *testing.T) {
	entryID := typetable.NewRecord().Field("value", typ.String).Build()
	entry := typetable.NewRecord().Field("id", typ.NewRef("", "EntryID")).Build()
	m := New("registry")
	m.DefineType("EntryID", entryID)
	m.DefineType("Entry", entry)

	scoped := m.ScopeType(typ.Func().Returns(typ.NewRef("", "Entry")).Build())
	fn, ok := scoped.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("ScopeType(function) = %T %[1]v, want one-return function", scoped)
	}
	resolvedEntry, ok := fn.Returns[0].(*typ.Record)
	if !ok || resolvedEntry.GetField("id") == nil {
		t.Fatalf("first return = %T %[1]v, want Entry record with id", fn.Returns[0])
	}
	if !typ.TypeEquals(resolvedEntry.GetField("id").Type, entryID) {
		t.Fatalf("Entry.id = %v, want registry EntryID %v", resolvedEntry.GetField("id").Type, entryID)
	}
}

func TestScopeTypeBreaksNamedReferenceCyclesAtModuleIdentity(t *testing.T) {
	m := New("registry")
	m.DefineType("Entry", typetable.NewRecord().Field("next", typ.NewRef("", "Entry")).Build())

	scoped := m.ScopeType(typ.Func().Returns(typ.NewRef("", "Entry")).Build())
	fn, ok := scoped.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("ScopeType(function) = %T %[1]v, want one-return function", scoped)
	}
	entry, ok := fn.Returns[0].(*typ.Record)
	if !ok || entry.GetField("next") == nil {
		t.Fatalf("first return = %T %[1]v, want Entry record with next", fn.Returns[0])
	}
	want := typ.NewRef("registry", "Entry")
	if !typ.TypeEquals(entry.GetField("next").Type, want) {
		t.Fatalf("Entry.next = %v, want module-qualified recursive identity %v", entry.GetField("next").Type, want)
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

func TestScopeTypeReattachesStructuralRecursiveGraphToDefiningType(t *testing.T) {
	provider := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
	decoded := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
	m := New("tree")
	m.DefineType("Node", provider)

	scoped := m.ScopeType(typ.Func().Returns(decoded).Build())
	fn, ok := scoped.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || fn.Returns[0] != provider {
		t.Fatalf("ScopeType(structural Node) = %v, want provider identity %v", scoped, provider)
	}
}
