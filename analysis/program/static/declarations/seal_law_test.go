package declarations

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func sealTable(t *testing.T, input Input) Table {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return table
}

// TestDeclarationsPreserveTypedOwnershipAndOrder proves every authored
// declaration relation survives the seal with its owner, name, coordinate,
// and ordered columns intact.
func TestDeclarationsPreserveTypedOwnershipAndOrder(t *testing.T) {
	declarations := sealTable(t, ledgerInput(t)).View()
	alias := term(keyspace.FamilyTypeAlias, 1)
	if owner, target, name, coordinate, ok := declarations.Aliases().Get(alias); !ok ||
		owner != term(keyspace.FamilyBody, 1) || target != term(keyspace.FamilyTypePrimitive, 1) ||
		name != 1 || coordinate == (source.Coordinate{}) {
		t.Fatalf("alias relation = (%v, %v, %v, %v, %v)", owner, target, name, coordinate, ok)
	}
	param := term(keyspace.FamilyTypeParam, 1)
	if count, ok := declarations.Aliases().ParamCount(alias); !ok || count != 1 {
		t.Fatalf("alias parameter count = (%d, %v)", count, ok)
	}
	if got, ok := declarations.Aliases().ParamAt(alias, 0); !ok || got != param {
		t.Fatalf("alias parameter = (%v, %v)", got, ok)
	}
	if owner, name, constraint, ok := declarations.TypeParams().Get(param); !ok || owner != alias || name != 3 ||
		constraint != term(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("type parameter = (%v, %v, %v, %v)", owner, name, constraint, ok)
	}
	iface := term(keyspace.FamilyTypeInterface, 1)
	if count, ok := declarations.Interfaces().ExtendCount(iface); !ok || count != 2 {
		t.Fatalf("interface extends = (%d, %v)", count, ok)
	}
	if member, ok := declarations.Interfaces().MemberAt(iface, 1); !ok || member.Kind != InterfaceMethod ||
		member.Field != 0 || member.Name != 6 || member.Signature != term(keyspace.FamilyTypeFunction, 1) {
		t.Fatalf("method member = (%+v, %v)", member, ok)
	}
	if member, ok := declarations.Interfaces().MemberAt(iface, 0); !ok || member.Kind != InterfaceField ||
		member.Field != term(keyspace.FamilyTypeField, 1) || member.Name != 0 ||
		member.NameCoordinate != (source.Coordinate{}) || member.Signature != 0 {
		t.Fatalf("field member = (%+v, %v)", member, ok)
	}
}

// TestDeclarationsRejectTotalityXORAndCoordinates proves the admissions this
// vertical owns. The combined containment forest and the TypeParam ownership
// law span other verticals and belong to the enclosing owner's joint laws --
// including what an alias parameter column may name, because that claim is
// only meaningful against the signature and contract claimants.
func TestDeclarationsRejectTotalityXORAndCoordinates(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, *Input)
	}{
		{"field member with a name", func(_ *testing.T, in *Input) { in.Interface[0].Members[0].Name = 9 }},
		{"field member with a signature", func(_ *testing.T, in *Input) {
			in.Interface[0].Members[0].Signature = term(keyspace.FamilyTypeFunction, 1)
		}},
		{"field member without a field", func(_ *testing.T, in *Input) { in.Interface[0].Members[0].Field = 0 }},
		{"method member with a field", func(_ *testing.T, in *Input) {
			in.Interface[0].Members[1].Field = term(keyspace.FamilyTypeField, 1)
		}},
		{"method missing coordinate", func(_ *testing.T, in *Input) {
			in.Interface[0].Members[1].NameCoordinate = source.Coordinate{}
		}},
		{"method missing name", func(_ *testing.T, in *Input) { in.Interface[0].Members[1].Name = 0 }},
		{"method non-callable signature", func(_ *testing.T, in *Input) {
			in.Interface[0].Members[1].Signature = term(keyspace.FamilyTypePrimitive, 1)
		}},
		{"unknown member kind", func(_ *testing.T, in *Input) { in.Interface[0].Members[0].Kind = 9 }},
		{"alias absent coordinate", func(_ *testing.T, in *Input) {
			in.Alias[0].NameCoordinate = source.Coordinate{}
		}},
		{"alias missing name", func(_ *testing.T, in *Input) { in.Alias[0].Name = 0 }},
		{"alias non-body owner", func(_ *testing.T, in *Input) {
			in.Alias[0].Owner = term(keyspace.FamilyCell, 1)
		}},
		{"alias non-node target", func(_ *testing.T, in *Input) {
			in.Alias[0].Target = term(keyspace.FamilyBody, 1)
		}},
		{"interface non-body owner", func(_ *testing.T, in *Input) {
			in.Interface[0].Owner = term(keyspace.FamilyCell, 1)
		}},
		{"interface absent coordinate", func(_ *testing.T, in *Input) {
			in.Interface[0].NameCoordinate = source.Coordinate{}
		}},
		{"interface non-reference extends", func(_ *testing.T, in *Input) {
			in.Interface[0].Extends[0] = term(keyspace.FamilyTypePrimitive, 1)
		}},
		{"type parameter missing name", func(_ *testing.T, in *Input) { in.TypeParam[0].Name = 0 }},
		{"type parameter invalid owner", func(_ *testing.T, in *Input) {
			in.TypeParam[0].Owner = term(keyspace.FamilyBody, 1)
		}},
		{"type parameter non-node constraint", func(_ *testing.T, in *Input) {
			in.TypeParam[0].Constraint = term(keyspace.FamilyBody, 1)
		}},
		{"declared type non-cell", func(_ *testing.T, in *Input) {
			in.DeclaredType[0].Cell = term(keyspace.FamilyBody, 1)
		}},
		{"declared type non-node target", func(_ *testing.T, in *Input) {
			in.DeclaredType[0].Target = term(keyspace.FamilyBody, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledgerInput(t)
			test.edit(t, &input)
			if _, err := Build(input, ledgerCounts()); err == nil {
				t.Fatal("Build() accepted an invalid declaration relation")
			}
		})
	}
}

// TestDeclarationsCopyFencesBoundsAndQueriesDoNotAllocate proves the seal takes
// a copy of every column, each read is total, and hot queries allocate nothing.
func TestDeclarationsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := ledgerInput(t)
	table := sealTable(t, input)
	input.Alias[0].Params[0] = 0
	input.Interface[0].Extends[0] = 0
	input.Interface[0].Members[1].Name = 99

	declarations := table.View()
	alias := term(keyspace.FamilyTypeAlias, 1)
	iface := term(keyspace.FamilyTypeInterface, 1)
	param := term(keyspace.FamilyTypeParam, 1)
	if got, ok := declarations.Aliases().ParamAt(alias, 0); !ok || got == 0 {
		t.Fatalf("alias copy fence = (%v, %v)", got, ok)
	}
	if got, ok := declarations.Interfaces().ExtendAt(iface, 0); !ok || got == 0 {
		t.Fatalf("interface extension copy fence = (%v, %v)", got, ok)
	}
	if got, ok := declarations.Interfaces().MemberAt(iface, 1); !ok || got.Name != 6 {
		t.Fatalf("interface member copy fence = (%+v, %v)", got, ok)
	}
	if _, ok := declarations.Aliases().ParamAt(alias, -1); ok {
		t.Fatal("ParamAt accepted negative index")
	}
	if _, ok := declarations.Interfaces().MemberAt(iface, 2); ok {
		t.Fatal("MemberAt accepted out-of-range index")
	}
	if _, _, _, _, ok := declarations.Aliases().Get(term(keyspace.FamilyTypeAlias, 9)); ok {
		t.Fatal("Aliases.Get accepted unknown term")
	}
	if _, _, _, _, ok := declarations.Aliases().Get(iface); ok {
		t.Fatal("Aliases.Get accepted foreign family")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		declarations.Aliases().Get(alias)
		declarations.Aliases().ParamCount(alias)
		declarations.Aliases().ParamAt(alias, 0)
		declarations.TypeParams().Get(param)
		declarations.Interfaces().Get(iface)
		declarations.Interfaces().ExtendAt(iface, 0)
		declarations.Interfaces().MemberAt(iface, 1)
		declarations.DeclaredTypes().ForCell(term(keyspace.FamilyCell, 1))
	}); allocations != 0 {
		t.Fatalf("declaration queries allocated %.2f times", allocations)
	}
}

// TestDeclaredTypesPreserveExactCellTargetRelationAndOrder proves the authored
// Cell-to-type relation keeps its row order and resolves by canonical term.
func TestDeclaredTypesPreserveExactCellTargetRelationAndOrder(t *testing.T) {
	declared := sealTable(t, ledgerInput(t)).View().DeclaredTypes()
	if declared.Count() != 2 {
		t.Fatalf("DeclaredTypes.Count() = %d, want 2", declared.Count())
	}
	for index, want := range []struct {
		cell   keyspace.Term
		target keyspace.Term
	}{
		{cell: term(keyspace.FamilyCell, 1), target: term(keyspace.FamilyTypePrimitive, 1)},
		{cell: term(keyspace.FamilyCell, 2), target: term(keyspace.FamilyTypePrimitive, 2)},
	} {
		declaration, ok := declared.At(index)
		if !ok || declaration != term(keyspace.FamilyDeclaredType, uint32(index+1)) {
			t.Fatalf("DeclaredTypes.At(%d) = %v/%v", index, declaration, ok)
		}
		cell, target, ok := declared.Get(declaration)
		if !ok || cell != want.cell || target != want.target {
			t.Fatalf("DeclaredTypes.Get(%v) = (%v, %v, %v)", declaration, cell, target, ok)
		}
	}
	if _, ok := declared.At(2); ok {
		t.Fatal("DeclaredTypes.At accepted out-of-range index")
	}
	if _, _, ok := declared.Get(term(keyspace.FamilyDeclaredType, 9)); ok {
		t.Fatal("DeclaredTypes.Get accepted unknown term")
	}
	if _, _, ok := declared.Get(term(keyspace.FamilyCell, 1)); ok {
		t.Fatal("DeclaredTypes.Get accepted foreign family")
	}
}

// TestDecoderRetainsTypedRowsAndMembers proves the decoded rows map each wire
// field back to the relation it names, including both member kinds.
func TestDecoderRetainsTypedRowsAndMembers(t *testing.T) {
	decoded, err := Decode(sectionReader(t, sectionBytes(t, ledgerInput(t))))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded.Alias) != 2 || len(decoded.TypeParam) != 2 ||
		len(decoded.Interface) != 2 || len(decoded.DeclaredType) != 2 {
		t.Fatalf("decoded counts = (%d, %d, %d, %d)", len(decoded.Alias), len(decoded.TypeParam),
			len(decoded.Interface), len(decoded.DeclaredType))
	}
	alias := decoded.Alias[0]
	if alias.Owner != term(keyspace.FamilyBody, 1) || alias.Name != 1 ||
		len(alias.Params) != 1 || alias.Params[0] != term(keyspace.FamilyTypeParam, 1) ||
		alias.NameCoordinate == (source.Coordinate{}) {
		t.Fatalf("decoded alias = %+v", alias)
	}
	members := decoded.Interface[0].Members
	if len(members) != 2 {
		t.Fatalf("decoded member count = %d, want 2", len(members))
	}
	if members[0].Kind != InterfaceField || members[0].Field != term(keyspace.FamilyTypeField, 1) ||
		members[0].Name != 0 || members[0].NameCoordinate != (source.Coordinate{}) || members[0].Signature != 0 {
		t.Fatalf("decoded field member = %+v", members[0])
	}
	if members[1].Kind != InterfaceMethod || members[1].Field != 0 || members[1].Name != 6 ||
		members[1].Signature != term(keyspace.FamilyTypeFunction, 1) {
		t.Fatalf("decoded method member = %+v", members[1])
	}
	if decoded.DeclaredType[1].Cell != term(keyspace.FamilyCell, 2) {
		t.Fatalf("decoded declared type = %+v", decoded.DeclaredType[1])
	}
}

// TestZeroViewsFailClosed proves an unavailable view answers nothing.
func TestZeroViewsFailClosed(t *testing.T) {
	var view View
	alias := term(keyspace.FamilyTypeAlias, 1)
	iface := term(keyspace.FamilyTypeInterface, 1)
	if view.Available() || view.Aliases().Count() != 0 || view.TypeParams().Count() != 0 ||
		view.Interfaces().Count() != 0 || view.DeclaredTypes().Count() != 0 {
		t.Fatal("zero View reported availability or rows")
	}
	if _, _, _, _, ok := view.Aliases().Get(alias); ok {
		t.Fatal("zero View returned an alias row")
	}
	if _, ok := view.Aliases().ParamCount(alias); ok {
		t.Fatal("zero View counted alias parameters")
	}
	if _, ok := view.Interfaces().MemberAt(iface, 0); ok {
		t.Fatal("zero View read an interface member")
	}
	if _, ok := view.DeclaredTypes().ForCell(term(keyspace.FamilyCell, 1)); ok {
		t.Fatal("zero View resolved a declared cell")
	}
}
