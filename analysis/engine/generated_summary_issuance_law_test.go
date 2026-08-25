package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// openGeneratedSummaryLaw seals the vector-read generated rule: one Summary
// join over the owner-issued member set of the relation it names.
func openGeneratedSummaryLaw(t testing.TB, members []uint32) generatedBindingLawFixture {
	t.Helper()
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawSummary, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	cell, cellOK := fixture.binding.state.rules[0].(*generatedRuleBindingCell)
	if !cellOK || cell == nil {
		t.Fatal("summary generated cell")
	}
	descriptor := cell.generated.program
	join := descriptor.JoinRelation().Member
	key := descriptor.KeyProjection().Member
	if members != nil {
		fixture.owner.members = map[[2]uint32][]uint32{{join, fixture.owner.candidate}: members}
		fixture.owner.memberProjections = make(map[[3]uint32]uint32, len(members))
		for index, member := range members {
			fixture.owner.memberProjections[[3]uint32{join, key, member}] = uint32(index)
		}
	}
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("summary generated binding seal")
	}
	return fixture
}

// TestAGeneratedVectorReadIsDeliveredOverTheFactorsOwnSummaryForm states the
// half of a vector read that belongs to the Factor.
//
// A whole denominator is not delivered because a rule asked for it: it is
// delivered over a form the Factor published, and that form's semantic keys
// the cold row and every summary surface minted against it. A rule whose join
// reads a Factor declaring no summary form therefore has no vector read to
// declare, and the sealed row says Summary rather than being flattened into
// the exact read it is not.
func TestAGeneratedVectorReadIsDeliveredOverTheFactorsOwnSummaryForm(t *testing.T) {
	fixture := openGeneratedSummaryLaw(t, []uint32{7, 9})
	cell := generatedLawCell(t, fixture)
	if !cell.schemaRuleComplete() {
		t.Fatal("a generated rule with a vector join was not admitted as complete")
	}
	if len(cell.reads) != 1 || cell.reads[0].kind != composition.ReadSummary {
		t.Fatalf("read kind = %v, want the declared vector read", cell.reads)
	}
	row := cell.reads[0]
	if !row.semantic.Available() || row.semantic != row.normalizer {
		t.Fatalf("vector read row semantic=%v normalizer=%v, want the Factor's one form", row.semantic, row.normalizer)
	}
	if len(row.dependencies) != 0 {
		t.Fatal("a vector read over a closed denominator declared a dependency")
	}
}

// TestAGeneratedVectorReadTakesItsKeysFromTheOwnerIssuedMemberSet is the
// clause a generated rule needs and an authored one gets from its operand.
//
// A generated rule has no operand, so the coordinates a vector read spans come
// from where they actually live: the relation the join names publishes the
// ordered member set one candidate row carries, and each member is projected
// through that join's own key projection. Nothing is invented here - the
// owner's own rows are the vector.
func TestAGeneratedVectorReadTakesItsKeysFromTheOwnerIssuedMemberSet(t *testing.T) {
	fixture := openGeneratedSummaryLaw(t, []uint32{7, 9})
	cell := generatedLawCell(t, fixture)
	descriptor := cell.generated.program
	plan, planOK := descriptor.ReadAt(0)
	if !planOK {
		t.Fatal("vector join")
	}
	keys, keysOK := generatedSummaryKeys(fixture.binding.state, plan, fixture.owner.candidate)
	if !keysOK {
		t.Fatal("the owner published no key vector for the candidate row")
	}
	if keys.SummaryKeyCount() != 2 {
		t.Fatalf("key vector width = %d, want one key per declared member", keys.SummaryKeyCount())
	}
	for index, want := range []uint64{0, 1} {
		key, present := keys.SummaryKeyAt(index)
		if !present || key != want {
			t.Fatalf("key %d = %d/%t, want the local its member projects to", index, key, present)
		}
	}

	reads, _, readsOK := declareGeneratedReadSurfaces(fixture.binding.state, cell, descriptor,
		lawCanonicalRuleAnchor(t), generatedSummaryLawSemantic(t, cell), OperandCoords{Mount: fixture.mount, Occurrence: fixture.occurrence}, fixture.owner.candidate)
	if !readsOK || len(reads) != 1 {
		t.Fatal("a generated vector read did not issue")
	}
	surface := reads[0].value
	if surface.Form != equation.SurfaceReadSummary || surface.Local != 0 {
		t.Fatalf("vector read sealed %+v, want a summary surface with no scalar coordinate", surface)
	}
	if surface.Semantic != surface.Normalizer || !surface.Semantic.Available() {
		t.Fatalf("vector read sealed semantic=%v normalizer=%v", surface.Semantic, surface.Normalizer)
	}
	if reads[0].summary == nil {
		t.Fatal("the issued vector read carries no key mapping")
	}
}

// TestAGeneratedVectorReadRefusesAnOwnerThatPublishesNoMemberSet keeps the
// vector honest at its source. A relation that publishes no ordered member set
// under the candidate has not said which coordinates the read spans, and a
// vector read over an unstated denominator is not a narrower read - it is a
// read whose width nothing declared.
func TestAGeneratedVectorReadRefusesAnOwnerThatPublishesNoMemberSet(t *testing.T) {
	fixture := openGeneratedSummaryLaw(t, nil)
	cell := generatedLawCell(t, fixture)
	descriptor := cell.generated.program
	plan, planOK := descriptor.ReadAt(0)
	if !planOK {
		t.Fatal("vector join")
	}
	if _, keysOK := generatedSummaryKeys(fixture.binding.state, plan, fixture.owner.candidate); keysOK {
		t.Fatal("an owner publishing no member set answered a key vector")
	}
	if _, _, readsOK := declareGeneratedReadSurfaces(fixture.binding.state, cell, descriptor,
		lawCanonicalRuleAnchor(t), generatedSummaryLawSemantic(t, cell), OperandCoords{Mount: fixture.mount, Occurrence: fixture.occurrence}, fixture.owner.candidate); readsOK {
		t.Fatal("a vector read issued over an unstated denominator")
	}
}

// TestAGeneratedVectorReadRefusesAnUnorderedMemberSet states the order clause
// at the boundary that owns it. The vector's cell positions ARE the
// denominator, so members answering one coordinate, or arriving out of the
// owner's ascending order, would renumber every later cell of every reader.
func TestAGeneratedVectorReadRefusesAnUnorderedMemberSet(t *testing.T) {
	fixture := openGeneratedSummaryLaw(t, []uint32{7, 9})
	cell := generatedLawCell(t, fixture)
	descriptor := cell.generated.program
	plan, planOK := descriptor.ReadAt(0)
	if !planOK {
		t.Fatal("vector join")
	}
	// The same two members, projected so the second names a coordinate before
	// the first: the owner's set is the same size and its order is not one.
	fixture.owner.memberProjections[[3]uint32{plan.Relation.Member, plan.Key.Member, 7}] = 1
	fixture.owner.memberProjections[[3]uint32{plan.Relation.Member, plan.Key.Member, 9}] = 0
	if _, _, readsOK := declareGeneratedReadSurfaces(fixture.binding.state, cell, descriptor,
		lawCanonicalRuleAnchor(t), generatedSummaryLawSemantic(t, cell), OperandCoords{Mount: fixture.mount, Occurrence: fixture.occurrence}, fixture.owner.candidate); readsOK {
		t.Fatal("a vector read issued over a member set that is not in coordinate order")
	}
}

func generatedSummaryLawSemantic(t testing.TB, cell *generatedRuleBindingCell) identity.SemanticKey {
	t.Helper()
	semantic, _, ok := generatedRuleSchema(cell)
	if !ok {
		t.Fatal("summary generated rule semantic")
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(semantic)
	if !ruleSemanticOK {
		t.Fatal("summary generated rule identity")
	}
	return ruleSemantic
}
