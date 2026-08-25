package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// setOwner is a minimal owner whose one occurrence carries three candidates
// and whose nested member set carries two ports per candidate. It exists to
// state the surface's cardinality law, not to model an axis.
type setOwner struct {
	occurrence identity.ContentID
	candidates []uint32
	ports      map[uint32][]uint32
	attributes map[uint32]uint32
}

func (owner *setOwner) CandidateCount(relationOrdinal uint32, mount, occurrence identity.ContentID) (int, bool) {
	if relationOrdinal != 0 || mount.Available() || occurrence != owner.occurrence {
		return 0, false
	}
	return len(owner.candidates), true
}

func (owner *setOwner) CandidateAt(relationOrdinal uint32, mount, occurrence identity.ContentID, index int) (uint32, bool) {
	if relationOrdinal != 0 || mount.Available() || occurrence != owner.occurrence || index < 0 || index >= len(owner.candidates) {
		return 0, false
	}
	return owner.candidates[index], true
}

func (owner *setOwner) MemberCount(relationOrdinal, parentCandidateOrdinal uint32) (int, bool) {
	if relationOrdinal != 1 {
		return 0, false
	}
	ports, held := owner.ports[parentCandidateOrdinal]
	if !held {
		return 0, false
	}
	return len(ports), true
}

func (owner *setOwner) MemberAt(relationOrdinal, parentCandidateOrdinal uint32, ordinal int) (uint32, bool) {
	if relationOrdinal != 1 || ordinal < 0 {
		return 0, false
	}
	ports, held := owner.ports[parentCandidateOrdinal]
	if !held || ordinal >= len(ports) {
		return 0, false
	}
	return ports[ordinal], true
}

func (owner *setOwner) Project(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (uint32, bool) {
	if relationOrdinal != 0 || projectionOrdinal != 0 {
		return 0, false
	}
	local, held := owner.attributes[candidateOrdinal]
	return local, held
}

func newSetOwner() *setOwner {
	occurrence := identity.ContentID{}
	occurrence[0] = 7
	return &setOwner{
		occurrence: occurrence,
		candidates: []uint32{4, 9, 11},
		ports:      map[uint32][]uint32{4: {0, 1}, 9: {2, 3}, 11: {4, 5}},
		attributes: map[uint32]uint32{
			4:  uint32(structure.Concrete.Ordinal()),
			9:  uint32(structure.NoSelection.Ordinal()),
			11: uint32(structure.AuthenticatedOpaque.Ordinal()),
		},
	}
}

// TestOneOccurrenceCarriesEveryCandidateItsRelationHolds is the cardinality
// law the activation relation needs: an occurrence's candidates are a set, not
// a single dense row. A body route per admitted body times an activation edge
// per module pair is a variable-width answer, and a surface that can only say
// "the candidate" cannot state it.
func TestOneOccurrenceCarriesEveryCandidateItsRelationHolds(t *testing.T) {
	var owner memberrelation.Owner = newSetOwner()
	source := newSetOwner()
	count, countOK := owner.CandidateCount(0, identity.ContentID{}, source.occurrence)
	if !countOK || count != 3 {
		t.Fatalf("candidate census = %d (%t), want the complete set", count, countOK)
	}
	seen := make(map[uint32]struct{}, count)
	for index := 0; index < count; index++ {
		candidate, ok := owner.CandidateAt(0, identity.ContentID{}, source.occurrence, index)
		if !ok {
			t.Fatalf("candidate %d of a published census must resolve", index)
		}
		if _, duplicate := seen[candidate]; duplicate {
			t.Fatalf("candidate %d repeats a dense ordinal", index)
		}
		seen[candidate] = struct{}{}
	}
	if _, ok := owner.CandidateAt(0, identity.ContentID{}, source.occurrence, count); ok {
		t.Fatal("a candidate beyond the published census is not a row of the relation")
	}
}

// TestANestedMemberSetIsAddressedByOrdinalUnderItsParentCandidate states the
// port address. An export port k of one candidate is a row of a nested set
// keyed by its ordinal, so a Program names it without a projection that must
// return a variable-length answer.
func TestANestedMemberSetIsAddressedByOrdinalUnderItsParentCandidate(t *testing.T) {
	var owner memberrelation.Owner = newSetOwner()
	count, countOK := owner.MemberCount(1, 9)
	if !countOK || count != 2 {
		t.Fatalf("port census = %d (%t)", count, countOK)
	}
	first, firstOK := owner.MemberAt(1, 9, 0)
	second, secondOK := owner.MemberAt(1, 9, 1)
	if !firstOK || !secondOK || first == second {
		t.Fatal("each ordinal of a nested member set addresses its own row")
	}
	if _, ok := owner.MemberAt(1, 9, count); ok {
		t.Fatal("an ordinal beyond the published census addresses no port")
	}
	if _, ok := owner.MemberCount(1, 12); ok {
		t.Fatal("a nested set has no members under a candidate its parent never issued")
	}
}

// TestABranchSettlesExactlyOneOutcome is FG-6 law (c) over the sealed column:
// the per-branch disposition is read off an attribute projection as one member
// of the five-valued vocabulary, every branch settles one, and a local outside
// the vocabulary settles none rather than defaulting to a member.
func TestABranchSettlesExactlyOneOutcome(t *testing.T) {
	var owner memberrelation.Owner = newSetOwner()
	source := newSetOwner()
	settled := make(map[uint32]structure.ReductionOutcome, len(source.candidates))
	for _, candidate := range source.candidates {
		local, projected := owner.Project(0, 0, candidate)
		if !projected {
			t.Fatalf("candidate %d carries no outcome column", candidate)
		}
		outcome, resolved := memberrelation.Outcome(local)
		if !resolved || !outcome.Available() {
			t.Fatalf("candidate %d settles no declared outcome", candidate)
		}
		if prior, repeated := settled[candidate]; repeated && prior != outcome {
			t.Fatalf("candidate %d settles two outcomes", candidate)
		}
		settled[candidate] = outcome
	}
	if len(settled) != len(source.candidates) {
		t.Fatalf("settled %d branches of %d", len(settled), len(source.candidates))
	}
	for _, outcome := range []structure.ReductionOutcome{
		structure.Refuse, structure.NoSelection, structure.NoCandidate,
		structure.Concrete, structure.AuthenticatedOpaque,
	} {
		recovered, ok := memberrelation.Outcome(uint32(outcome.Ordinal()))
		if !ok || recovered != outcome {
			t.Fatalf("outcome %d does not round-trip through its attribute local", outcome)
		}
	}
	if _, ok := memberrelation.Outcome(0); ok {
		t.Fatal("zero is the absent local, not a settled outcome")
	}
	if _, ok := memberrelation.Outcome(uint32(structure.AuthenticatedOpaque.Ordinal()) + 1); ok {
		t.Fatal("a local outside the vocabulary settles no outcome")
	}
}

func (owner *setOwner) KeyVectorCount(relationOrdinal, candidateOrdinal uint32) (int, bool) {
	return 0, false
}

func (owner *setOwner) KeyVectorAt(relationOrdinal, candidateOrdinal uint32, ordinal int) (uint32, bool) {
	return 0, false
}
