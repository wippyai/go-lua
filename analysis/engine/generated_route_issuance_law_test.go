package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// openGeneratedRouteLaw seals the routed generated rule: one exact join, one
// selected join fed by it, and a routed publication over that selected join.
func openGeneratedRouteLaw(t testing.TB) generatedBindingLawFixture {
	t.Helper()
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawRouteOutput, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("routed generated binding seal")
	}
	return fixture
}

// TestARoutedGeneratedRuleDeclaresItsOwnReadGeometry states what a generated
// rule now carries. It has one sealed read row per declared join, in the form
// its own row states, and it names the publication disposition its Plan
// declared - so a routed rule is admitted by the same completeness check that
// admits an exact one, rather than being refused for not being exact.
func TestARoutedGeneratedRuleDeclaresItsOwnReadGeometry(t *testing.T) {
	fixture := openGeneratedRouteLaw(t)
	cell := generatedLawCell(t, fixture)
	if !cell.schemaRuleComplete() {
		t.Fatal("a routed generated rule was not admitted as complete")
	}
	if len(cell.reads) != cell.generated.program.ReadCount() || len(cell.reads) != 2 {
		t.Fatalf("the rule declares %d joins and carries %d read rows", cell.generated.program.ReadCount(), len(cell.reads))
	}
	if cell.reads[0].kind != composition.ReadExact || cell.reads[1].kind != composition.ReadSelect {
		t.Fatalf("read kinds = %d/%d, want the exact join then the selected one", cell.reads[0].kind, cell.reads[1].kind)
	}
	for index, row := range cell.reads {
		if row.owner != cell || row.ownerOrdinal != cell.ordinal || row.readOrdinal != uint64(index) {
			t.Fatalf("read row %d is not owned by the rule that declared it", index)
		}
		if cell.schemaRuleReadAt(uint64(index)) != row {
			t.Fatalf("read row %d is not the row the rule answers with", index)
		}
	}
	if len(cell.reads[1].dependencies) != 1 || cell.reads[1].dependencies[0] != 0 {
		t.Fatalf("the selected join depends on %v, want the exact join that feeds it", cell.reads[1].dependencies)
	}
	if cell.writeMode != directRuleWriteRoute || cell.routeRead != 2 {
		t.Fatalf("write mode = %d route read = %d, want a routed publication over join 1", cell.writeMode, cell.routeRead)
	}
	if cell.directRuleWriteMode() != directRuleWriteRoute || cell.directRuleRouteRead() != 2 {
		t.Fatal("the rule does not answer with the disposition it declared")
	}
}

// generatedRouteLawSurfaces mints one issuance's surfaces against a canonical
// anchor. It calls exactly what the issuance arm calls, so what is proven here
// is the surface minting itself rather than the Batch admission around it.
func generatedRouteLawSurfaces(t testing.TB, fixture generatedBindingLawFixture) ([]RuleReadSurface, ruleWriteSurface) {
	t.Helper()
	cell := generatedLawCell(t, fixture)
	semantic, _, semanticOK := generatedRuleSchema(cell)
	if !semanticOK {
		t.Fatal("routed generated rule semantic")
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(semantic)
	if !ruleSemanticOK {
		t.Fatal("routed generated rule identity")
	}
	anchor := lawCanonicalRuleAnchor(t)
	descriptor := cell.generated.program
	candidate, candidateOK := fixture.owner.CandidateAt(descriptor.CandidateRelation().Member, fixture.mount, fixture.occurrence, 0)
	if !candidateOK {
		t.Fatal("routed generated candidate")
	}
	reads, _, readsOK := declareGeneratedReadSurfaces(fixture.binding.state, cell, descriptor, anchor, ruleSemantic, OperandCoords{Mount: fixture.mount, Occurrence: fixture.occurrence}, candidate)
	if !readsOK {
		t.Fatal("routed generated read surfaces")
	}
	write, writeOK := declareGeneratedWriteSurface(fixture.binding.state, cell, descriptor, anchor, ruleSemantic, candidate)
	if !writeOK {
		t.Fatal("routed generated write surface")
	}
	return reads, write
}

// TestARoutedIssuanceSealsItsReadsAndDefersItsDestination is the issuance half
// of the seam. A routed issuance seals everything that is decided when the row
// is declared - the candidate, and every prior read's coordinate - and seals
// NO destination, because a routed row's destinations are the members of the
// relation its selected join derives, which exist only per invocation.
//
// What stands in for the destination is an anchored surface: the routed write
// is identified by the occurrence and operand this issuance minted, so the
// members published later are attributable to this exact row rather than to
// the rule in general.
func TestARoutedIssuanceSealsItsReadsAndDefersItsDestination(t *testing.T) {
	fixture := openGeneratedRouteLaw(t)
	reads, write := generatedRouteLawSurfaces(t, fixture)
	if len(reads) != 2 {
		t.Fatalf("issuance declared %d reads, want one per join", len(reads))
	}
	exact := reads[0].value
	if exact.Form != equation.SurfaceReadExact || exact.Mode != equation.TargetModeNone || exact.Local == 0 {
		t.Fatalf("the exact join sealed %+v, want a resolved exact coordinate", exact)
	}
	selected := reads[1].value
	if selected.Form != equation.SurfaceReadSelect || selected.Local != 0 || selected.Semantic != selected.Factor {
		t.Fatalf("the selected join sealed %+v, want an anchored selection with no static coordinate", selected)
	}
	if write.value.Form != equation.SurfaceWriteRoute || write.value.Local != 0 || write.value.Mode != equation.TargetModeNone {
		t.Fatalf("the routed write sealed %+v, want a deferred destination", write.value)
	}
	if !write.anchored || !reads[1].anchored {
		t.Fatal("a deferred surface is not anchored to the issuance that minted it")
	}
	if reads[0].anchored {
		t.Fatal("a resolved coordinate was anchored as though it were deferred")
	}
	if selected.Content == write.value.Content {
		t.Fatal("the selection and the publication share one anchored identity")
	}
}

// TestAnExactIssuanceStillResolvesItsDestination holds the other side of the
// same switch. Generalizing the arm must not turn an exact publication into a
// deferred one: a rule that names one destination projection resolves it while
// the row is declared, exactly as before.
func TestAnExactIssuanceStillResolvesItsDestination(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("exact generated binding seal")
	}
	cell := generatedLawCell(t, fixture)
	if cell.writeMode != directRuleWriteExact || cell.routeRead != 0 {
		t.Fatalf("write mode = %d route read = %d, want an exact publication", cell.writeMode, cell.routeRead)
	}
	reads, write := generatedRouteLawSurfaces(t, fixture)
	if len(reads) != 1 || reads[0].value.Form != equation.SurfaceReadExact {
		t.Fatalf("exact rule declared %d reads", len(reads))
	}
	if write.value.Form != equation.SurfaceWriteExact || write.value.Mode != equation.TargetModeStrong || write.value.Local == 0 {
		t.Fatalf("the exact write sealed %+v, want its resolved destination", write.value)
	}
	if write.anchored {
		t.Fatal("a resolved destination was anchored as though it were deferred")
	}
}

// TestATransformedCarryGeneratedRuleStaysComplete fences the one fact the cold
// carry row does NOT carry. Which transform a carried fact passes through is
// the descriptor's statement, resolved by the family that installs the fold,
// so the cold row names none even for a rule whose carry transforms. A
// completeness check that expected the cold row to agree with the descriptor
// about the transform would refuse every transformed carry - which is the
// shape heap's empty constructor has.
func TestATransformedCarryGeneratedRuleStaysComplete(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawTransformedCarry, generatedRuleLawRuleRole))
	cell := generatedLawCell(t, fixture)
	if _, present := cell.generated.program.CarryTransform(); !present {
		t.Fatal("the fixture rule declares no carry transform")
	}
	carry, carryOK := cell.schema.ruleCarryShapeAt(cell.ordinal, 0)
	if !carryOK || carry.Transform.Available() {
		t.Fatalf("the cold carry row names transform %+v, which is the descriptor's statement", carry.Transform)
	}
	if !cell.schemaRuleComplete() {
		t.Fatal("a transformed-carry generated rule was not admitted as complete")
	}
}
