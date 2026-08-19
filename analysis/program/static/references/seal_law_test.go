package references

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// sealCounts is the external denominator this vertical measures against. The
// declaration families appear only as bounds: References owns no row in them.
func sealCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyTypeRef] = 3
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeInterface] = 1
	counts[keyspace.FamilyTypeParam] = 1
	counts[keyspace.FamilyCell] = 1
	return counts
}

// TestReferencesPreserveAuthoredBinderDisposition proves each authored
// disposition survives the seal with its exclusive anchors and both key paths.
func TestReferencesPreserveAuthoredBinderDisposition(t *testing.T) {
	table, err := Build(Input{TypeRef: []TypeRef{
		{Resolution: Declaration, Source: []keyspace.Key{1}, Target: term(keyspace.FamilyTypeAlias, 1)},
		{Resolution: CanonicalPath, Source: []keyspace.Key{2, 3}, Root: term(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7, 8}},
		{Resolution: Unresolved, Source: []keyspace.Key{4, 5}, Root: term(keyspace.FamilyCell, 1)},
	}}, sealCounts())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	references := table.View()
	if got := references.Count(); got != 3 {
		t.Fatalf("Count() = %d, want 3", got)
	}
	declaration := term(keyspace.FamilyTypeRef, 1)
	if kind, target, root, ok := references.Get(declaration); !ok || kind != Declaration ||
		target != term(keyspace.FamilyTypeAlias, 1) || root != 0 {
		t.Fatalf("declaration Get() = (%v, %v, %v, %v)", kind, target, root, ok)
	}
	canonical := term(keyspace.FamilyTypeRef, 2)
	if source, ok := references.SourceCount(canonical); !ok || source != 2 {
		t.Fatalf("SourceCount() = (%d, %v), want 2", source, ok)
	}
	if key, ok := references.SourceAt(canonical, 1); !ok || key != 3 {
		t.Fatalf("SourceAt() = (%d, %v), want 3", key, ok)
	}
	if canonicalCount, ok := references.CanonicalCount(canonical); !ok || canonicalCount != 2 {
		t.Fatalf("CanonicalCount() = (%d, %v), want 2", canonicalCount, ok)
	}
	if key, ok := references.CanonicalAt(canonical, 0); !ok || key != 7 {
		t.Fatalf("CanonicalAt() = (%d, %v), want 7", key, ok)
	}
	unresolved := term(keyspace.FamilyTypeRef, 3)
	if kind, target, root, ok := references.Get(unresolved); !ok || kind != Unresolved ||
		target != 0 || root != term(keyspace.FamilyCell, 1) {
		t.Fatalf("unresolved Get() = (%v, %v, %v, %v)", kind, target, root, ok)
	}
	if length, ok := references.CanonicalCount(unresolved); !ok || length != 0 {
		t.Fatalf("unresolved CanonicalCount() = (%d, %v), want 0", length, ok)
	}
}

// TestReferencesRejectInvalidXORRootAndArity proves the disposition's
// exclusive-anchor law and the qualification law admit exactly the authored
// shapes a binder can produce.
func TestReferencesRejectInvalidXORRootAndArity(t *testing.T) {
	cell := term(keyspace.FamilyCell, 1)
	alias := term(keyspace.FamilyTypeAlias, 1)
	interfaceType := term(keyspace.FamilyTypeInterface, 1)
	param := term(keyspace.FamilyTypeParam, 1)
	for _, test := range []struct {
		name string
		row  TypeRef
		ok   bool
	}{
		{"unqualified unresolved", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1}}, true},
		{"qualified unresolved", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1, 2}, Root: cell}, true},
		{"alias target", TypeRef{Resolution: Declaration, Source: []keyspace.Key{1}, Target: alias}, true},
		{"interface target", TypeRef{Resolution: Declaration, Source: []keyspace.Key{1}, Target: interfaceType}, true},
		{"param target", TypeRef{Resolution: Declaration, Source: []keyspace.Key{1}, Target: param}, true},
		{"canonical", TypeRef{Resolution: CanonicalPath, Source: []keyspace.Key{1, 2}, Root: cell, Canonical: []keyspace.Key{3}}, true},
		{"empty source", TypeRef{Resolution: Unresolved}, false},
		{"zero source key", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{0}}, false},
		{"unqualified root", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1}, Root: cell}, false},
		{"qualified missing root", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1, 2}}, false},
		{"qualified noncell root", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1, 2}, Root: alias}, false},
		{"unresolved target", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1}, Target: alias}, false},
		{"unresolved canonical", TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1}, Canonical: []keyspace.Key{2}}, false},
		{"declaration missing target", TypeRef{Resolution: Declaration, Source: []keyspace.Key{1}}, false},
		{"declaration canonical", TypeRef{Resolution: Declaration, Source: []keyspace.Key{1}, Target: alias, Canonical: []keyspace.Key{2}}, false},
		{"declaration type ref target", TypeRef{Resolution: Declaration, Source: []keyspace.Key{1}, Target: term(keyspace.FamilyTypeRef, 1)}, false},
		{"canonical target", TypeRef{Resolution: CanonicalPath, Source: []keyspace.Key{1}, Target: alias, Canonical: []keyspace.Key{2}}, false},
		{"canonical missing path", TypeRef{Resolution: CanonicalPath, Source: []keyspace.Key{1}}, false},
		{"canonical zero path key", TypeRef{Resolution: CanonicalPath, Source: []keyspace.Key{1}, Canonical: []keyspace.Key{0}}, false},
		{"unknown disposition", TypeRef{Resolution: 99, Source: []keyspace.Key{1}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := sealCounts()
			counts[keyspace.FamilyTypeRef] = 1
			_, err := Build(Input{TypeRef: []TypeRef{test.row}}, counts)
			if (err == nil) != test.ok {
				t.Fatalf("Build() error = %v, want accepted=%v", err, test.ok)
			}
		})
	}
}

// TestReferencesCopyFenceAndBounds proves the seal takes a copy of both key
// paths, every column read is total, and the hot queries allocate nothing.
func TestReferencesCopyFenceAndBounds(t *testing.T) {
	counts := sealCounts()
	counts[keyspace.FamilyTypeRef] = 1
	source := []keyspace.Key{1, 2}
	canonical := []keyspace.Key{3, 4}
	table, err := Build(Input{TypeRef: []TypeRef{{
		Resolution: CanonicalPath,
		Source:     source,
		Root:       term(keyspace.FamilyCell, 1),
		Canonical:  canonical,
	}}}, counts)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	source[0], canonical[0] = 99, 99

	references := table.View()
	reference := term(keyspace.FamilyTypeRef, 1)
	if got, ok := references.SourceAt(reference, 0); !ok || got != 1 {
		t.Fatalf("source copy fence = (%d, %v)", got, ok)
	}
	if got, ok := references.CanonicalAt(reference, 0); !ok || got != 3 {
		t.Fatalf("canonical copy fence = (%d, %v)", got, ok)
	}
	for _, index := range []int{-1, 2} {
		if _, ok := references.SourceAt(reference, index); ok {
			t.Fatalf("SourceAt(%d) accepted out-of-bounds index", index)
		}
		if _, ok := references.CanonicalAt(reference, index); ok {
			t.Fatalf("CanonicalAt(%d) accepted out-of-bounds index", index)
		}
	}
	if _, ok := references.At(1); ok {
		t.Fatal("At() accepted out-of-bounds ordinal")
	}
	if _, _, _, ok := references.Get(term(keyspace.FamilyTypeRef, 2)); ok {
		t.Fatal("Get() accepted an unknown TypeRef")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		references.Get(reference)
		references.SourceCount(reference)
		references.SourceAt(reference, 1)
		references.CanonicalCount(reference)
		references.CanonicalAt(reference, 1)
	}); allocations != 0 {
		t.Fatalf("Reference queries allocated %.2f times", allocations)
	}
}

// TestZeroViewFailsClosed proves an unavailable view answers nothing rather
// than reaching into a table it does not have.
func TestZeroViewFailsClosed(t *testing.T) {
	var view View
	reference := term(keyspace.FamilyTypeRef, 1)
	if view.Available() || view.Count() != 0 {
		t.Fatal("zero View reported availability or rows")
	}
	if _, ok := view.At(0); ok {
		t.Fatal("zero View minted a term")
	}
	if _, _, _, ok := view.Get(reference); ok {
		t.Fatal("zero View returned a row")
	}
	for _, probe := range []func() (int, bool){
		func() (int, bool) { return view.SourceCount(reference) },
		func() (int, bool) { return view.CanonicalCount(reference) },
	} {
		if count, ok := probe(); ok || count != 0 {
			t.Fatalf("zero View column count = %d/%v", count, ok)
		}
	}
	if _, ok := view.SourceAt(reference, 0); ok {
		t.Fatal("zero View read a source key")
	}
	if _, ok := view.CanonicalAt(reference, 0); ok {
		t.Fatal("zero View read a canonical key")
	}
}
