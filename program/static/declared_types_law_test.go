package static

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func declaredTypeFixture(t *testing.T) Input {
	t.Helper()
	input := declarationFixture(t)
	input.Counts[keyspace.FamilyCell] = 2
	input.Counts[keyspace.FamilyDeclaredType] = 1
	input.Counts[keyspace.FamilyTypePrimitive] = 4
	input.Types.Primitive = append(input.Types.Primitive, Primitive{Kind: PrimitiveBoolean})
	input.Declarations.DeclaredType = []DeclaredType{{
		Cell:   keyspace.MakeTerm(keyspace.FamilyCell, 1),
		Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4),
	}}
	return input
}

func TestDeclaredTypesPreserveExactCellTargetRelation(t *testing.T) {
	draft, err := Build(declaredTypeFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	declared := component.View().Declarations().DeclaredTypes()
	term, ok := declared.At(0)
	if !ok {
		t.Fatal("DeclaredTypes.At(0) rejected the sole row")
	}
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	target := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4)
	if gotCell, gotTarget, ok := declared.Get(term); !ok || gotCell != cell || gotTarget != target {
		t.Fatalf("DeclaredTypes.Get() = (%v, %v, %v), want (%v, %v, true)", gotCell, gotTarget, ok, cell, target)
	}
	if got, ok := declared.ForCell(cell); !ok || got != term {
		t.Fatalf("DeclaredTypes.ForCell() = (%v, %v), want (%v, true)", got, ok, term)
	}
	// A counted Cell can be globally shaped (or otherwise nonlexical) at this
	// vertical. Static preserves its authored relation but has no Flow/Source
	// role authority to accept it as lexical; joint sealing must do that later.
	if got, ok := declared.ForCell(keyspace.MakeTerm(keyspace.FamilyCell, 2)); ok || got != 0 {
		t.Fatalf("DeclaredTypes.ForCell() invented relation for valid unbound Cell: (%v, %v)", got, ok)
	}
}

func TestDeclaredTypesPreserveAuthoredRowOrder(t *testing.T) {
	input := declaredTypeFixture(t)
	input.Counts[keyspace.FamilyDeclaredType] = 2
	input.Counts[keyspace.FamilyTypePrimitive] = 5
	input.Types.Primitive = append(input.Types.Primitive, Primitive{Kind: PrimitiveNil})
	input.Declarations.DeclaredType = append(input.Declarations.DeclaredType, DeclaredType{
		Cell:   keyspace.MakeTerm(keyspace.FamilyCell, 2),
		Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5),
	})
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	declared := component.View().Declarations().DeclaredTypes()
	for index, want := range []struct {
		cell, target keyspace.Term
	}{
		{keyspace.MakeTerm(keyspace.FamilyCell, 1), keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4)},
		{keyspace.MakeTerm(keyspace.FamilyCell, 2), keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5)},
	} {
		term, ok := declared.At(index)
		if !ok {
			t.Fatalf("DeclaredTypes.At(%d) rejected authored row", index)
		}
		cell, target, ok := declared.Get(term)
		if !ok || cell != want.cell || target != want.target {
			t.Fatalf("DeclaredTypes.Get(At(%d)) = (%v, %v, %v), want (%v, %v, true)", index, cell, target, ok, want.cell, want.target)
		}
		if inverse, ok := declared.ForCell(want.cell); !ok || inverse != term {
			t.Fatalf("DeclaredTypes.ForCell(%v) = (%v, %v), want (%v, true)", want.cell, inverse, ok, term)
		}
	}
}

func TestDeclaredTypesRejectCountCellTargetAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing declared type count", func(input *Input) { input.Counts[keyspace.FamilyDeclaredType] = 0 }},
		{"extra declared type count", func(input *Input) { input.Counts[keyspace.FamilyDeclaredType] = 2 }},
		{"non-cell anchor", func(input *Input) {
			input.Declarations.DeclaredType[0].Cell = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"foreign cell anchor", func(input *Input) {
			input.Declarations.DeclaredType[0].Cell = keyspace.MakeTerm(keyspace.FamilyCell, 3)
		}},
		{"nonstatic target", func(input *Input) {
			input.Declarations.DeclaredType[0].Target = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"shared target", func(input *Input) {
			input.Declarations.DeclaredType[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := declaredTypeFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid declared type relation")
			}
		})
	}

	t.Run("duplicate cell", func(t *testing.T) {
		input := declaredTypeFixture(t)
		input.Counts[keyspace.FamilyDeclaredType] = 2
		input.Counts[keyspace.FamilyTypePrimitive] = 5
		input.Types.Primitive = append(input.Types.Primitive, Primitive{Kind: PrimitiveNil})
		input.Declarations.DeclaredType = append(input.Declarations.DeclaredType, DeclaredType{
			Cell: keyspace.MakeTerm(keyspace.FamilyCell, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5),
		})
		if _, err := Build(input); err == nil {
			t.Fatal("Build() accepted duplicate declared type Cell")
		}
	})

	t.Run("cyclic target remains rejected by local forest", func(t *testing.T) {
		input := declaredTypeFixture(t)
		input.Counts[keyspace.FamilyTypeOptional] = 1
		input.Types.Optional = []Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)}}
		input.Declarations.DeclaredType[0].Target = keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)
		if _, err := Build(input); err == nil {
			t.Fatal("Build() accepted declared type with cyclic static target")
		}
	})
}

func TestDeclaredTypesCopyFenceBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := declaredTypeFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Declarations.DeclaredType[0] = DeclaredType{}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	declared := component.View().Declarations().DeclaredTypes()
	term := keyspace.MakeTerm(keyspace.FamilyDeclaredType, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if gotCell, gotTarget, ok := declared.Get(term); !ok || gotCell != cell || gotTarget == 0 {
		t.Fatalf("declared type copy fence = (%v, %v, %v)", gotCell, gotTarget, ok)
	}
	if _, _, ok := declared.Get(0); ok {
		t.Fatal("DeclaredTypes.Get accepted zero term")
	}
	if _, _, ok := declared.Get(keyspace.MakeTerm(keyspace.FamilyDeclaredType, 2)); ok {
		t.Fatal("DeclaredTypes.Get accepted foreign ordinal")
	}
	if _, ok := declared.ForCell(0); ok {
		t.Fatal("DeclaredTypes.ForCell accepted zero Cell")
	}
	if _, ok := declared.ForCell(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("DeclaredTypes.ForCell accepted non-Cell anchor")
	}
	if _, ok := declared.ForCell(keyspace.MakeTerm(keyspace.FamilyCell, 3)); ok {
		t.Fatal("DeclaredTypes.ForCell accepted foreign Cell")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		declared.Count()
		declared.At(0)
		declared.Get(term)
		declared.ForCell(cell)
		declared.ForCell(keyspace.MakeTerm(keyspace.FamilyCell, 2))
	}); allocations != 0 {
		t.Fatalf("declared type queries allocated %.2f times", allocations)
	}
}
