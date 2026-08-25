package generated

import (
	"testing"

	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// summaryMemberSetRead is a Summary read over a self-provided nested member
// set: the vector is correlated by the ordinal its parent addresses it at, so
// the read declares a parent restatement and no selection predicate.
func summaryMemberSetRead() ReadPlan {
	return ReadPlan{
		Input: 0, Factor: 1, Axis: 0,
		Relation: ruleplan.RelationAddr{Axis: 0, Member: 1},
		Key:      ruleplan.ProjectionAddr{Axis: 0, Member: 0},
		Parent:   ruleplan.RelationAddr{Axis: 0, Member: 2}, ParentPresent: true,
		// A vector read over a nested member set is indexed by the PARENT's
		// directory, which is the one the owner resolves the row in.
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 2}, AddressingPresent: true,
		Form: ruleprogram.Summary, PointBound: ruleprogram.PointBound,
		Contract: ruleplan.ReadContract{
			Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
			OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityMany,
		},
		Denominator: ruleplan.DenominatorAddr{Ordinal: 0, Present: true},
		RowCapacity: 1, CellCapacity: 1,
	}
}

// A Summary read whose relation is a self-provided nested member set seals
// through the generated path. Its ordinal position IS the correlation, so
// requiring a predicate beside it would demand a second tagging authority for
// the same row - a law this package no longer states on its own account, and
// which the declaration's own normal form already settled.
func TestGeneratedSealAdmitsSummaryOverASelfProvidedMemberSet(t *testing.T) {
	read := summaryMemberSetRead()
	descriptor, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1))
	if !ok || !descriptor.Available() {
		t.Fatalf("summary over a nested member set refused: %+v/%t", descriptor, ok)
	}
	parent, present, parentOK := descriptor.ReadParentAt(0)
	if !parentOK || !present || parent != read.Parent {
		t.Fatalf("sealed parent = %+v present=%t ok=%t, want %+v", parent, present, parentOK, read.Parent)
	}
	form, formOK := descriptor.ReadFormAt(0)
	if !formOK || form != ruleprogram.Summary {
		t.Fatalf("sealed form = %v/%t, want Summary", form, formOK)
	}
}

// A Summary read that is addressed by neither an owner-issued predicate nor a
// parent ordinal correlates its cells with nothing, and does not seal.
func TestGeneratedSealRefusesAnUncorrelatedSummary(t *testing.T) {
	read := summaryMemberSetRead()
	read.Parent, read.ParentPresent = ruleplan.RelationAddr{}, false
	if descriptor, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1)); ok || descriptor.Available() {
		t.Fatal("a summary correlated by nothing sealed")
	}
}

// The absent parent carries the zero address, and a declared one carries a
// valid address. A read holding an address it does not declare present, or
// declaring a parent at no valid address, is a descriptor disagreeing with
// itself.
//
// The zero address is NOT the disagreement: relation 0 of axis 0 is a real
// relation, and a member set hanging off the first relation an axis declares
// is the ordinary case. Presence is declared and never inferred from the
// value, which is the encoding ReadAddressingShape states for the third
// address and this one is held to as well.
func TestGeneratedSealRefusesADisagreeingParentEncoding(t *testing.T) {
	read := summaryMemberSetRead()
	read.ParentPresent = false
	if _, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1)); ok {
		t.Fatal("an undeclared parent address sealed")
	}
	read = summaryMemberSetRead()
	read.Parent = ruleplan.RelationAddr{Axis: ^uint32(0), Member: ^uint32(0)}
	if _, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1)); ok {
		t.Fatal("a declared parent at no valid address sealed")
	}
	read = summaryMemberSetRead()
	read.Parent = ruleplan.RelationAddr{}
	if _, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1)); !ok {
		t.Fatal("a member set whose parent is the first relation of the first axis was refused")
	}
}

// An exact lookup and a closed complete vector select nothing. The predicate
// law they are held to is the declaration's own, so the two paths cannot
// disagree about which reads carry one.
func TestGeneratedSealRefusesAPredicateOnAClosedVector(t *testing.T) {
	read := summaryMemberSetRead()
	read.Form = ruleprogram.Complete
	read.Parent, read.ParentPresent = ruleplan.RelationAddr{}, false
	read.Predicate, read.PredicatePresent = ruleplan.ProjectionAddr{Axis: 0, Member: 3}, true
	if _, ok := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1)); ok {
		t.Fatal("a complete vector carrying a selection predicate sealed")
	}
}
