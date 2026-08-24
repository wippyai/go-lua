package generated

import (
	"testing"

	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// exactAddressedRead is the ordinary candidate-addressed read: an exact keyed
// lookup indexed by the ordinal the rule resolved in the directory it names.
func exactAddressedRead() ReadPlan {
	return ReadPlan{
		Input: 0, Factor: 1, Axis: 0,
		Relation:   ruleplan.RelationAddr{Axis: 0, Member: 0},
		Key:        ruleplan.ProjectionAddr{Axis: 0, Member: 0},
		Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
		Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound,
		Contract: ruleplan.ReadContract{
			Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
			OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne,
		},
		RowCapacity: 1, CellCapacity: 1,
	}
}

func addressingLawSpec(read ReadPlan) CompiledRuleSpec {
	return planLawSpec([]ReadPlan{read}, &CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true}, 1)
}

// TestASealedCandidateAddressedReadNamesTheDirectoryItIsIndexedBy is the
// resolution law of a sealed read.
//
// The rule resolves one ordinal in one directory. Every read indexed by that
// ordinal has to say which directory it belongs to, because two directories
// addressed by one occurrence enumerate their rows independently: a read that
// carried no directory would be resolvable only by assuming its owner numbers
// its rows the way the candidate's owner does.
func TestASealedCandidateAddressedReadNamesTheDirectoryItIsIndexedBy(t *testing.T) {
	read := exactAddressedRead()
	read.Addressing, read.AddressingPresent = ruleplan.RelationAddr{}, false
	if descriptor, ok := NewPlanCompiledRule(addressingLawSpec(read)); ok || descriptor.Available() {
		t.Fatal("an exact read that names no addressing directory sealed")
	}
	if descriptor, ok := NewPlanCompiledRule(addressingLawSpec(exactAddressedRead())); !ok || !descriptor.Available() {
		t.Fatalf("a read naming its directory was refused: %+v/%t", descriptor, ok)
	}
}

// TestASealedReadTheCandidateOrdinalDoesNotIndexNamesNoDirectory is the other
// half. A selected read is addressed by the selection its own family resolves,
// so a directory declared beside it would be a second addressing authority for
// rows the rule's ordinal never reaches.
func TestASealedReadTheCandidateOrdinalDoesNotIndexNamesNoDirectory(t *testing.T) {
	spec := heterogeneousPlanLawSpec()
	selected := spec.Reads[1]
	if selected.Form != ruleprogram.Selected || selected.AddressingPresent {
		t.Fatalf("fixture read 1 = form %v addressing present %t, want an unaddressed selection", selected.Form, selected.AddressingPresent)
	}
	spec.Reads = append([]ReadPlan(nil), spec.Reads...)
	spec.Reads[1].Addressing, spec.Reads[1].AddressingPresent = ruleplan.RelationAddr{Axis: 0, Member: 0}, true
	if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
		t.Fatal("a selected read carrying an addressing directory sealed")
	}
}

// TestAnIssuedCandidateRuleNamesNoAddressingDirectory states the issued arm.
// An issued candidate is a Program row, not a directory row: there is no axis
// order for a read to be indexed in, and the ordinal reaches the runtime on the
// mounted placement instead. A read that named a directory under it would claim
// a correspondence to an order that does not exist.
func TestAnIssuedCandidateRuleNamesNoAddressingDirectory(t *testing.T) {
	spec := addressingLawSpec(exactAddressedRead())
	spec.IssuedCandidate = true
	spec.Candidate = ruleplan.RelationAddr{}
	if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
		t.Fatal("an issued-candidate rule sealed a read naming an addressing directory")
	}
}

// TestTheAbsentAddressingEncodingIsTheZeroAddress keeps the dense encoding
// honest. The zero relation address is a real address, so absence is stated by
// the presence flag and the address is required to be zero beside it; a
// non-zero address left behind an absent flag is metadata disagreeing with
// itself.
func TestTheAbsentAddressingEncodingIsTheZeroAddress(t *testing.T) {
	spec := heterogeneousPlanLawSpec()
	spec.Reads = append([]ReadPlan(nil), spec.Reads...)
	spec.Reads[1].Addressing = ruleplan.RelationAddr{Axis: 0, Member: 3}
	if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
		t.Fatal("an absent addressing carrying a non-zero address sealed")
	}
}

// TestASealedAddressingIsAnsweredByOrdinal proves the descriptor publishes the
// directory it sealed rather than making a consumer re-derive it from the
// read's own relation, which is exactly the derivation a foreign directory
// makes wrong.
func TestASealedAddressingIsAnsweredByOrdinal(t *testing.T) {
	read := exactAddressedRead()
	read.Relation = ruleplan.RelationAddr{Axis: 1, Member: 2}
	read.Key = ruleplan.ProjectionAddr{Axis: 1, Member: 2}
	read.Factor, read.Axis = 1, 1
	descriptor, ok := NewPlanCompiledRule(addressingLawSpec(read))
	if !ok || !descriptor.Available() {
		t.Fatalf("a foreign-relation read naming the candidate directory was refused: %+v/%t", descriptor, ok)
	}
	addressing, present, addressingOK := descriptor.ReadAddressingAt(0)
	if !addressingOK || !present || addressing != read.Addressing {
		t.Fatalf("sealed addressing = %+v present=%t ok=%t, want %+v", addressing, present, addressingOK, read.Addressing)
	}
	if addressing == read.Relation {
		t.Fatal("the sealed addressing directory was recovered from the read's own relation")
	}
}
