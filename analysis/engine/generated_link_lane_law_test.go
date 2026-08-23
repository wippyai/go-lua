package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
)

// generatedLinkLawDirectory is the owner double for a globally addressed
// candidate relation: it resolves candidates from an occurrence alone and
// publishes the occurrence directory that inventory is derived from. It is the
// smallest owner that can stand in for a sealed axis with a Link relation.
type generatedLinkLawDirectory struct {
	generatedBindingLawOwner
	occurrences []identity.ContentID
}

var _ memberrelation.Owner = (*generatedLinkLawDirectory)(nil)
var _ memberrelation.OccurrenceDirectory = (*generatedLinkLawDirectory)(nil)

func (owner *generatedLinkLawDirectory) candidateFor(relationOrdinal uint32, mount, occurrence identity.ContentID) (uint32, bool) {
	if owner == nil || !owner.acceptCandidate || relationOrdinal != owner.relation || mount.Available() || !occurrence.Available() {
		return 0, false
	}
	for index, id := range owner.occurrences {
		if id == occurrence {
			return uint32(index), true
		}
	}
	return 0, false
}

func (owner *generatedLinkLawDirectory) CandidateCount(relationOrdinal uint32, mount, occurrence identity.ContentID) (int, bool) {
	if _, ok := owner.candidateFor(relationOrdinal, mount, occurrence); !ok {
		return 0, false
	}
	return 1, true
}

func (owner *generatedLinkLawDirectory) CandidateAt(relationOrdinal uint32, mount, occurrence identity.ContentID, index int) (uint32, bool) {
	if index != 0 {
		return 0, false
	}
	return owner.candidateFor(relationOrdinal, mount, occurrence)
}

func (owner *generatedLinkLawDirectory) OccurrenceCount(relationOrdinal uint32) (int, bool) {
	if owner == nil || relationOrdinal != owner.relation {
		return 0, false
	}
	return len(owner.occurrences), true
}

func (owner *generatedLinkLawDirectory) OccurrenceIDAt(relationOrdinal uint32, index int) (identity.ContentID, bool) {
	if owner == nil || relationOrdinal != owner.relation || index < 0 || index >= len(owner.occurrences) {
		return identity.ContentID{}, false
	}
	return owner.occurrences[index], true
}

func generatedLinkLawOccurrences(t testing.TB) []identity.ContentID {
	t.Helper()
	ids := make([]identity.ContentID, 0, 3)
	for _, seed := range []string{"first", "second", "third"} {
		id, ok := identity.DeriveContentID("go-lua/generated-link-law/occurrence", []byte(seed))
		if !ok {
			t.Fatal("generated Link law occurrence identity")
		}
		ids = append(ids, id)
	}
	return ids
}

// TestLinkGeneratedSlotIssuesTheLinkLaneCapability states the Link twin of the
// mounted generated issuance: the same sealed plan slot, registered through the
// Link handoff, carries a Link capability and never a mounted one. Lane is the
// only degree of freedom - there is no second generated lifecycle.
func TestLinkGeneratedSlotIssuesTheLinkLaneCapability(t *testing.T) {
	fixture := openGeneratedBindingLaneLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole), true)
	if !fixture.cap.Link() || fixture.cap.Mounted() || fixture.cap.MountedPoint() || fixture.cap.Activation() {
		t.Fatalf("Link generated capability lanes link=%t mounted=%t mountedPoint=%t activation=%t",
			fixture.cap.Link(), fixture.cap.Mounted(), fixture.cap.MountedPoint(), fixture.cap.Activation())
	}
	// The slot is one declaration and holds one lane: a second registration of
	// either lane must be refused rather than re-issuing the capability.
	if _, ok := RegisterLinkGeneratedSlot(fixture.binding, fixture.slot); ok {
		t.Fatal("a generated Link slot was registered twice")
	}
	if _, ok := RegisterMountedGeneratedSlot(fixture.binding, fixture.slot); ok {
		t.Fatal("a generated Link slot also took the mounted lane")
	}
}

// TestGeneratedLinkOccurrenceInventoryIsTheCandidateDirectory states where a
// generated Link rule's occurrences come from: the candidate relation the
// sealed plan names, read through the axis owner's own occurrence directory.
// No rule callback and no owner-issued catalog participates.
func TestGeneratedLinkOccurrenceInventoryIsTheCandidateDirectory(t *testing.T) {
	fixture := openGeneratedBindingLaneLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole), true)
	base := generatedBindingLawOwnerForDescriptor(t, fixture)
	occurrences := generatedLinkLawOccurrences(t)
	directory := &generatedLinkLawDirectory{generatedBindingLawOwner: *base, occurrences: occurrences}
	if !BindRelationOwner(fixture.binding, fixture.factors[0], directory) {
		t.Fatal("generated Link law relation owner")
	}
	if !fixture.binding.Seal() {
		t.Fatal("generated Link law binding seal")
	}
	inventory, inventoryOK := GeneratedOccurrenceCatalog(fixture.binding, fixture.slot)
	if !inventoryOK || inventory.Count() != len(occurrences) {
		t.Fatalf("generated Link inventory count = %d/%t, want %d", inventory.Count(), inventoryOK, len(occurrences))
	}
	for index, want := range occurrences {
		got, ok := inventory.IDAt(index)
		if !ok || got != want {
			t.Fatalf("generated Link inventory row %d = %v/%t", index, got, ok)
		}
	}
	if _, ok := inventory.IDAt(len(occurrences)); ok {
		t.Fatal("generated Link inventory admitted a row past its census")
	}
	if _, ok := inventory.IDAt(-1); ok {
		t.Fatal("generated Link inventory admitted a negative row")
	}
}

// TestGeneratedOccurrenceInventoryRefusesAMountedCandidateRelation is the
// nearest negative: a mounted candidate relation publishes no occurrence
// directory, and its occurrences are the artifact's rows. Deriving an
// inventory for it would let a rule admit occurrences no artifact declared.
func TestGeneratedOccurrenceInventoryRefusesAMountedCandidateRelation(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("generated mounted law binding seal")
	}
	if _, ok := GeneratedOccurrenceCatalog(fixture.binding, fixture.slot); ok {
		t.Fatal("a mounted candidate relation published an occurrence inventory")
	}
}

// TestGeneratedOccurrenceInventoryRequiresASealedBinding keeps the phase fence
// adjacent to the positive law: the relation owner is an axis authority only
// once the binding is terminal.
func TestGeneratedOccurrenceInventoryRequiresASealedBinding(t *testing.T) {
	fixture := openGeneratedBindingLaneLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole), true)
	base := generatedBindingLawOwnerForDescriptor(t, fixture)
	directory := &generatedLinkLawDirectory{generatedBindingLawOwner: *base, occurrences: generatedLinkLawOccurrences(t)}
	if !BindRelationOwner(fixture.binding, fixture.factors[0], directory) {
		t.Fatal("generated Link law relation owner")
	}
	if _, ok := GeneratedOccurrenceCatalog(fixture.binding, fixture.slot); ok {
		t.Fatal("an open binding published a generated occurrence inventory")
	}
	if !fixture.binding.Seal() {
		t.Fatal("generated Link law binding seal")
	}
	if _, ok := GeneratedOccurrenceCatalog(fixture.binding, fixture.slot); !ok {
		t.Fatal("the sealed binding refused its own generated occurrence inventory")
	}
}
