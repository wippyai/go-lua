package transformer

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func TestFormalValuesCarrierPreservesOwnedSymbolicTerm(t *testing.T) {
	program, constant, _ := formalTupleConstantInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	_, _, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("missing formal authority")
	}
	group := authority.body.factors.values
	if len(group.valueSlots) == 0 {
		t.Fatal("fixture has no Values slots")
	}
	term := authority.terms.Root(Root{Kind: RootParam, Index: 0})
	binding := formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: term}}
	leaf, err := authority.internBinding(binding)
	if err != nil {
		t.Fatal(err)
	}
	leaves := make([]decisionLeaf, len(group.members))
	leaves[group.valueSlots[0].position] = leaf
	carrier, err := algebra.materializeFormalValuesGroup(authority, group, leaves)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := carrier.Values[group.valueSlots[0].slot]
	if !ok || !value.isSymbolic || value.symbolicLeaf != leaf {
		t.Fatalf("symbolic Values carrier = %#v", carrier)
	}
	if _, err := formalConcreteValuesFactor(authority, carrier); err == nil {
		t.Fatal("symbolic Values carrier crossed concrete boundary")
	}
	if _, err := formalPublicationConcreteValues(algebra, authority, group, leaves); !errors.Is(err, errFormalPublicationSymbolicValues) {
		t.Fatalf("publication symbolic Values assertion = %v", err)
	}
	factored, err := algebra.factorFormalValuesGroup(authority, group, carrier)
	if err != nil {
		t.Fatal(err)
	}
	if got := factored[group.valueSlots[0].position]; got != leaf {
		t.Fatalf("symbolic Values factor leaf = %d, want interned %d", got, leaf)
	}
	tuple, err := algebra.instantiateConstant(constant)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := algebra.writeFormalValuesFactor(tuple, formalValuesFiberGroup{descriptor: group}, carrier)
	if err != nil {
		t.Fatalf("canonical symbolic Values seed: %v", err)
	}
	if err := algebra.validateTuple(seeded); err != nil {
		t.Fatalf("canonical symbolic Values seed validation: %v", err)
	}
}

func TestFormalValuesCarrierJoinsConcreteAndSymbolicWithoutGrowingArena(t *testing.T) {
	program, _, _ := formalTupleConstantInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	_, _, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("missing formal authority")
	}
	term := authority.terms.Root(Root{Kind: RootParam, Index: 0})
	symbolic, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: term}})
	if err != nil {
		t.Fatal(err)
	}
	concrete, err := authority.internGroundValue(product.Top())
	if err != nil {
		t.Fatal(err)
	}
	before := len(authority.terms.values)
	joined, err := authority.joinFormalValueLeaves(concrete, symbolic)
	if err != nil {
		t.Fatal(err)
	}
	if len(authority.terms.values) != before {
		t.Fatalf("symbolic Values join grew sealed term arena: %d -> %d", before, len(authority.terms.values))
	}
	terminal, err := authority.terminal(joined)
	if err != nil || terminal.kind != formalComponentSymbolicValue || !terminal.symbolicValue.hasGround || len(terminal.symbolicValue.bindings) != 1 {
		t.Fatalf("mixed Values join = %#v, %v", terminal, err)
	}
	value, err := formalValueFromLeaf(authority, joined)
	if err != nil || !value.isSymbolic || value.symbolicLeaf != joined {
		t.Fatalf("mixed Values carrier = %#v, %v", value, err)
	}
}

func TestFormalValuesCarrierJoinsSymbolicTermsInArena(t *testing.T) {
	program, _, _ := formalTupleConstantInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	_, _, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("missing formal authority")
	}
	left := authority.terms.Root(Root{Kind: RootParam, Index: 0})
	var right ValueTerm
	for candidate := ValueTerm(1); int(candidate) < len(authority.terms.values); candidate++ {
		if candidate != left && authority.validFormalValue(candidate) {
			right = candidate
			break
		}
	}
	if right == 0 {
		t.Fatal("fixture has no second sealed symbolic value term")
	}
	leftLeaf, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: left}})
	if err != nil {
		t.Fatal(err)
	}
	rightLeaf, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: right}})
	if err != nil {
		t.Fatal(err)
	}
	joinedLeaf, err := authority.combine(context.Background(), formalComponentJoin, leftLeaf, rightLeaf)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := formalValueFromLeaf(authority, joinedLeaf)
	if err != nil || !joined.isSymbolic {
		t.Fatalf("symbolic join carrier = %#v, %v", joined, err)
	}
	if joined.symbolicLeaf != joinedLeaf {
		t.Fatalf("symbolic join leaf = %d, want interned %d", joined.symbolicLeaf, joinedLeaf)
	}
}

func TestFormalValuesCarrierOrdersSymbolicSumWithoutConcreteMaterialization(t *testing.T) {
	program, _, _ := formalTupleConstantInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	_, _, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("missing formal authority")
	}
	group := authority.body.factors.values
	if len(group.valueSlots) == 0 {
		t.Fatal("fixture has no Values slot")
	}
	binding, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{
		owner: authority.variable, arena: authority.terms, term: authority.terms.Root(Root{Kind: RootParam}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	ground, err := authority.internGroundValue(product.Top())
	if err != nil {
		t.Fatal(err)
	}
	joined, err := authority.joinFormalValueLeaves(binding, ground)
	if err != nil {
		t.Fatal(err)
	}
	slot := group.valueSlots[0].slot
	left := formalValuesFactor{Values: map[FormalSlot]formalValue{slot: formalSymbolicValue(binding)}}
	right := formalValuesFactor{Values: map[FormalSlot]formalValue{slot: formalSymbolicValue(joined)}}
	less, err := formalValuesFactorRelation(authority, left, right, true)
	if err != nil || !less {
		t.Fatalf("symbolic Values subset = %t, %v", less, err)
	}
	equal, err := formalValuesFactorRelation(authority, left, right, false)
	if err != nil || equal {
		t.Fatalf("distinct symbolic Values equality = %t, %v", equal, err)
	}
}
