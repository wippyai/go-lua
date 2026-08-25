package generated

import (
	"testing"

	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// zero_address_presence_law_test.go states the encoding law this package
// already spells for one of its two address kinds and owed to the other.
//
// ReadAddressingShape says it exactly: "the zero relation address is a real
// address, and a read that names none must carry it zero." Presence is the
// declaration's own statement, and the address is checked against it. Deriving
// presence from whether the value happens to be non-zero says instead that
// relation 0 of axis 0 does not exist, which silently makes the first relation
// an axis declares unusable as a parent or a predicate.

// zeroAddressParentRead is a Summary over a nested member set whose parent is
// the FIRST relation of the FIRST axis - a perfectly ordinary declaration, and
// the one whose compiled address is all zeroes.
func zeroAddressParentRead() ReadPlan {
	return ReadPlan{
		Input: 0, Factor: 1, Axis: 0,
		Relation: ruleplan.RelationAddr{Axis: 0, Member: 1},
		Key:      ruleplan.ProjectionAddr{Axis: 0, Member: 2},
		Parent:   ruleplan.RelationAddr{Axis: 0, Member: 0}, ParentPresent: true,
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
		Form: ruleprogram.Summary, PointBound: ruleprogram.PointBound,
		Contract: ruleplan.ReadContract{
			Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
			OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityMany,
		},
		Denominator: ruleplan.DenominatorAddr{Ordinal: 0, Present: true},
		RowCapacity: 1, CellCapacity: 1,
	}
}

func zeroAddressCarry() *CarryPlan {
	return &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}
}

// TestAMemberSetMayHangOffTheFirstRelationAnAxisDeclares is the defect this
// law exists for. An axis's first relation is the one a rule is most likely to
// declare its candidate row in, and a branch set hanging off that row is the
// ordinary case, not an edge one.
func TestAMemberSetMayHangOffTheFirstRelationAnAxisDeclares(t *testing.T) {
	descriptor, sealed := NewPlanCompiledRule(planLawSpec([]ReadPlan{zeroAddressParentRead()}, zeroAddressCarry(), 1))
	if !sealed || !descriptor.Available() {
		t.Fatal("a member set whose parent is relation 0 of axis 0 did not seal")
	}
	parent, present, parentOK := descriptor.ReadParentAt(0)
	if !parentOK || !present || parent != (ruleplan.RelationAddr{Axis: 0, Member: 0}) {
		t.Fatalf("sealed parent = %+v present=%t ok=%t, want the zero address held present", parent, present, parentOK)
	}
}

// TestAnAbsentParentCarriesTheZeroAddress is the other half of the encoding,
// and the reason presence cannot simply be dropped: a read that declares no
// parent must carry none, so an address left behind by an earlier edit is
// refused rather than read as a set.
func TestAnAbsentParentCarriesTheZeroAddress(t *testing.T) {
	read := zeroAddressParentRead()
	read.Form = ruleprogram.Exact
	read.ParentPresent = false
	read.Denominator = ruleplan.DenominatorAddr{}
	read.Contract.Multiplicity = ruleprogram.MultiplicityOne
	if _, sealed := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, zeroAddressCarry(), 1)); !sealed {
		t.Fatal("an exact read carrying no parent and the zero address did not seal")
	}
	read.Parent = ruleplan.RelationAddr{Axis: 0, Member: 3}
	if _, sealed := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, zeroAddressCarry(), 1)); sealed {
		t.Fatal("a read declaring no parent while carrying an address sealed")
	}
}

// TestAPredicateMayNameTheFirstProjectionAnAxisDeclares holds the sibling
// address to the same law. A selection predicate at projection 0 of axis 0 is
// as ordinary as a parent at relation 0, and it was unrepresentable for the
// same reason.
func TestAPredicateMayNameTheFirstProjectionAnAxisDeclares(t *testing.T) {
	read := zeroAddressParentRead()
	read.Form = ruleprogram.Selected
	read.Parent, read.ParentPresent = ruleplan.RelationAddr{}, false
	read.Addressing, read.AddressingPresent = ruleplan.RelationAddr{}, false
	read.Predicate, read.PredicatePresent = ruleplan.ProjectionAddr{Axis: 0, Member: 0}, true
	read.Contract.Order = ruleprogram.OrderByTag
	descriptor, sealed := NewPlanCompiledRule(planLawSpec([]ReadPlan{read}, zeroAddressCarry(), 1))
	if !sealed || !descriptor.Available() {
		t.Fatal("a selection whose predicate is projection 0 of axis 0 did not seal")
	}
}
