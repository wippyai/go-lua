package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestLifecycleViewExpiresEveryTypedProjectionAfterCommit(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	construction := finalizer.View()
	if !construction.Available() || construction.Types().Primitives().Count() != 1 {
		t.Fatal("claimed construction View did not expose its authored component")
	}
	if construction.References().Count() != 0 || construction.Declarations().Aliases().Count() != 0 ||
		construction.Publications().Count() != 0 || construction.Operands().Claims().Count() != 0 ||
		construction.Operators().TypeOfs().Count() != 0 || construction.Signatures().TypeFunctions().Count() != 0 ||
		construction.Contracts().Functions().Count() != 0 {
		t.Fatal("construction View exposed unexpected rows")
	}
	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if construction.Available() || construction.Types().Primitives().Count() != 0 ||
		construction.References().Count() != 0 || construction.Declarations().Aliases().Count() != 0 ||
		construction.Publications().Count() != 0 || construction.Operands().Claims().Count() != 0 ||
		construction.Operators().TypeOfs().Count() != 0 || construction.Signatures().TypeFunctions().Count() != 0 ||
		construction.Contracts().Functions().Count() != 0 {
		t.Fatal("expired construction View retained a typed projection")
	}
	if !component.View().Available() || component.View().Types().Primitives().Count() != 1 {
		t.Fatal("published Component View lost its typed projection")
	}
}

// TestPoolBackedQueriesFailClosedOnAnUnavailableView proves every pool-backed
// cursor refuses a view that resolves to no component, rather than reaching
// into one that is not there.
func TestPoolBackedQueriesFailClosedOnAnUnavailableView(t *testing.T) {
	view := View{}
	term := func(family keyspace.Family) keyspace.Term { return keyspace.MakeTerm(family, 1) }
	for _, probe := range []struct {
		name string
		call func() bool
	}{
		{"reference source", func() bool { _, ok := view.References().SourceAt(term(keyspace.FamilyTypeRef), 0); return ok }},
		{"reference canonical", func() bool { _, ok := view.References().CanonicalAt(term(keyspace.FamilyTypeRef), 0); return ok }},
		{"alias param", func() bool {
			_, ok := view.Declarations().Aliases().ParamAt(term(keyspace.FamilyTypeAlias), 0)
			return ok
		}},
		{"interface extend", func() bool {
			_, ok := view.Declarations().Interfaces().ExtendAt(term(keyspace.FamilyTypeInterface), 0)
			return ok
		}},
		{"interface member", func() bool {
			_, ok := view.Declarations().Interfaces().MemberAt(term(keyspace.FamilyTypeInterface), 0)
			return ok
		}},
		{"union member", func() bool { _, ok := view.Types().Unions().MemberAt(term(keyspace.FamilyTypeUnion), 0); return ok }},
		{"intersection member", func() bool {
			_, ok := view.Types().Intersections().MemberAt(term(keyspace.FamilyTypeIntersection), 0)
			return ok
		}},
		{"generic arg", func() bool { _, ok := view.Types().Generics().ArgAt(term(keyspace.FamilyTypeGeneric), 0); return ok }},
		{"record field", func() bool { _, ok := view.Types().Records().FieldAt(term(keyspace.FamilyTypeRecord), 0); return ok }},
		{"signature type param", func() bool {
			_, ok := view.Signatures().TypeFunctions().TypeParamAt(term(keyspace.FamilyTypeFunction), 0)
			return ok
		}},
		{"signature parameter", func() bool {
			_, ok := view.Signatures().TypeFunctions().ParameterAt(term(keyspace.FamilyTypeFunction), 0)
			return ok
		}},
		{"signature return", func() bool {
			_, ok := view.Signatures().TypeFunctions().ReturnAt(term(keyspace.FamilyTypeFunction), 0)
			return ok
		}},
		{"contract type param", func() bool {
			_, ok := view.Contracts().Functions().TypeParamAt(term(keyspace.FamilyFunction), 0)
			return ok
		}},
		{"contract return", func() bool {
			_, ok := view.Contracts().Functions().ReturnAt(term(keyspace.FamilyFunction), 0)
			return ok
		}},
		{"call type argument", func() bool {
			_, ok := view.Contracts().Calls().TypeArgumentAt(term(keyspace.FamilyCall), 0)
			return ok
		}},
		{"annotation term", func() bool {
			_, ok := view.Operands().Annotations().ForAt(term(keyspace.FamilyTypePrimitive), 0)
			return ok
		}},
	} {
		t.Run(probe.name, func(t *testing.T) {
			if probe.call() {
				t.Fatal("pool-backed query accepted an unavailable view")
			}
		})
	}
}

func TestContractsQueryRootEnumeratesCanonicalFunctionAndCallTerms(t *testing.T) {
	component := staticContentComponent(t, contractsFixture(t))
	view := component.View().Contracts()
	if got, ok := view.Functions().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyFunction, 1) {
		t.Fatalf("Functions.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.Calls().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyCall, 1) {
		t.Fatalf("Calls.At(0) = %v/%v", got, ok)
	}
	if _, ok := view.Functions().At(-1); ok {
		t.Fatal("Functions.At accepted negative index")
	}
	if _, ok := view.Calls().At(1); ok {
		t.Fatal("Calls.At accepted out-of-range index")
	}
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

func TestOperandsQueryRootPreservesSparseAndDenseOrdinals(t *testing.T) {
	component := staticContentComponent(t, operandsFixture(t))
	view := component.View().Operands()
	if got, ok := view.Claims().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyValueClaim, 1) {
		t.Fatalf("Claims.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.TypeValues().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeValue, 1) {
		t.Fatalf("TypeValues.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.Annotations().At(1); !ok || got != keyspace.MakeTerm(keyspace.FamilyAnnotation, 2) {
		t.Fatalf("Annotations.At(1) = %v/%v", got, ok)
	}
	if _, ok := view.Claims().At(-1); ok {
		t.Fatal("Claims.At accepted negative index")
	}
	if _, ok := view.Annotations().At(2); ok {
		t.Fatal("Annotations.At accepted out-of-range index")
	}
}

func TestOperatorsQueryRootEnumeratesAllTypedColumns(t *testing.T) {
	component := staticContentComponent(t, operatorFixture())
	view := component.View().Operators()
	if view.TypeOfs().Count() != 2 || view.KeyOfs().Count() != 1 ||
		view.IndexAccesses().Count() != 1 || view.Conditionals().Count() != 1 {
		t.Fatalf("operator column counts = typeof:%d keyof:%d index:%d conditional:%d",
			view.TypeOfs().Count(), view.KeyOfs().Count(), view.IndexAccesses().Count(), view.Conditionals().Count())
	}
	if got, ok := view.Conditionals().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1) {
		t.Fatalf("Conditionals.At(0) = %v/%v", got, ok)
	}
	if _, _, ok := view.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("TypeOfs.Get accepted a non-TypeOf family")
	}
}

func TestPublicationsQueryRootReturnsCanonicalTermAndPair(t *testing.T) {
	component := staticContentComponent(t, publicationFixture(t))
	view := component.View().Publications()
	term, ok := view.At(0)
	if !ok || term != keyspace.MakeTerm(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publications.At(0) = %v/%v", term, ok)
	}
	assign, pair, target, ok := view.Get(term)
	if !ok || assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || pair != 0 ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("Publications.Get() = %v/%d/%v/%v", assign, pair, target, ok)
	}
	if _, ok := view.At(1); ok {
		t.Fatal("Publications.At accepted out-of-range index")
	}
}

func TestSignaturesQueryRootReturnsFunctionAndAssertionRows(t *testing.T) {
	component := staticContentComponent(t, signatureFixture(t))
	view := component.View().Signatures()
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	if got, ok := view.TypeFunctions().At(0); !ok || got != function {
		t.Fatalf("TypeFunctions.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.Assertions().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1) {
		t.Fatalf("Assertions.At(0) = %v/%v", got, ok)
	}
	if count, ok := view.TypeFunctions().ParameterCount(function); !ok || count != 1 {
		t.Fatalf("TypeFunctions.ParameterCount() = %d/%v", count, ok)
	}
	if _, ok := view.Assertions().At(1); ok {
		t.Fatal("Assertions.At accepted out-of-range index")
	}
}

func TestStaticQueryRootProjectsOwnedVerticalsAndFailsClosed(t *testing.T) {
	component := staticContentComponent(t, publicationFixture(t))
	view := component.View()
	if !view.Available() || view.Types().Primitives().Count() != 1 ||
		view.References().Count() != 2 || view.Declarations().Aliases().Count() != 1 ||
		view.Publications().Count() != 1 {
		t.Fatal("View did not project the authored top-level verticals")
	}
	var nilComponent *Component
	empty := nilComponent.View()
	if empty.Available() || empty.Types().Primitives().Count() != 0 ||
		empty.References().Count() != 0 || empty.Declarations().Aliases().Count() != 0 ||
		empty.Publications().Count() != 0 {
		t.Fatal("nil Component View did not fail closed")
	}
}
