package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// structural_rule_family_law_test.go states the A form's admission: a Program
// whose publication is structural reaches the same cold row, the same bind, and
// the same family seam as one that writes a fact.
//
// The A form computes no fact. Its output is the activation row set its
// candidate branches mount into the construct topology, so every place the
// engine used to read "the Factor this rule writes" has to read the rule's
// DECLARED geometry instead. These laws hold that reading to one answer: the
// cold row carries the activation family and no write, the bound cell carries
// the structural disposition, and the family claim is fenced by the axis the
// rule's rows are indexed by.

func structuralLawFixture(t testing.TB, spare int) generatedBindingLawFixture {
	t.Helper()
	return openGeneratedBindingSpareLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawActivation, generatedRuleLawRuleRole), spare)
}

// TestTheActivationFormReachesAColdStructuralRow is the declaration half of the
// cut. A structural publication names no output Factor and no write, because
// there is no Factor cell for one to address; what it names instead is the
// activation family its branches are admitted under, which is the cold
// capability the composition validates a structural rule by.
func TestTheActivationFormReachesAColdStructuralRow(t *testing.T) {
	fixture := structuralLawFixture(t, 0)
	ordinal, ordinalOK := fixture.slot.Ordinal()
	if !ordinalOK {
		t.Fatal("activation rule ordinal")
	}
	shape, shapeOK := fixture.schema.ruleShapeAt(ordinal)
	if !shapeOK {
		t.Fatal("activation rule shape")
	}
	if shape.OutputKind != composition.StructuralOutput {
		t.Fatalf("output kind = %d, want the structural row", shape.OutputKind)
	}
	if shape.Output.Available() || shape.WriteCount != 0 || shape.CarryCount != 0 {
		t.Fatalf("a structural row published a fact: output=%v writes=%d carries=%d", shape.Output.Available(), shape.WriteCount, shape.CarryCount)
	}
	if shape.ActivationCount != 1 || !shape.ActivationFamily.Available() {
		t.Fatalf("activation capability = %d/%v", shape.ActivationCount, shape.ActivationFamily.Available())
	}
	// The A form declares ONE read: the exact read at the trigger coordinate.
	// Its branch set is a relation the declaration names and the issuance pass
	// enumerates through that relation's owner - a branch carries no fact any
	// judgment consumes and has no coordinate of its own to be read at, so a
	// second read here would deliver the trigger's own cell once per branch.
	if shape.ReadCount != 1 {
		t.Fatalf("declared reads = %d, want only the trigger read", shape.ReadCount)
	}
	exact, exactOK := fixture.schema.ruleReadShapeAt(ordinal, 0)
	if !exactOK || exact.Kind != composition.ReadExact {
		t.Fatalf("read kind = %d, want the exact trigger read", exact.Kind)
	}
	descriptor, descriptorOK := fixture.schema.generatedProgramAt(0)
	if !descriptorOK {
		t.Fatal("the sealed structural descriptor")
	}
	if branch, declared := descriptor.ActivationBranch(); !declared || branch.Branch.Member == 0 {
		t.Fatalf("branch relation = %+v declared=%t, want the relation its branches are members of", branch.Branch, declared)
	}
}

// TestTheSealedActivationDescriptorDeclaresTheActivationForm holds the cold row
// and the execution form table to one declaration. DeclaredForm derives the
// form from the publication mode, the transport vector and the read
// vocabulary; if the slot sealed a descriptor that classified as anything else,
// the row that binds and the row that executes would be two rules.
func TestTheSealedActivationDescriptorDeclaresTheActivationForm(t *testing.T) {
	fixture := structuralLawFixture(t, 0)
	descriptor, descriptorOK := fixture.schema.generatedProgramAt(0)
	if !descriptorOK || !descriptor.Available() {
		t.Fatalf("sealed descriptor = %+v/%t", descriptor, descriptorOK)
	}
	mode, modeOK := descriptor.OutputMode()
	if !modeOK || mode != ruleprogram.ModeStructural {
		t.Fatalf("descriptor mode = %d/%t", mode, modeOK)
	}
	if descriptor.TransportCount() == 0 {
		t.Fatal("a structural descriptor carries the vector its candidate routes instantiate")
	}
	row, declared := execution.DeclaredForm(descriptor)
	if !declared || row.Form != execution.FormActivation {
		t.Fatalf("declared form = %d/%t, want the activation form", row.Form, declared)
	}
}

// TestBindGeneratedRuleSeatsAStructuralRow is the bind half. The generated arm
// used to refuse a structural row by its output kind, before any family could
// be installed against it; the disposition it takes instead is the one the
// declaration states, which is that this rule publishes no fact.
func TestBindGeneratedRuleSeatsAStructuralRow(t *testing.T) {
	fixture := structuralLawFixture(t, 0)
	cell := generatedLawCell(t, fixture)
	if cell.directRuleWriteMode() != directRuleWriteStructural {
		t.Fatalf("write mode = %d, want the structural disposition", cell.directRuleWriteMode())
	}
	if cell.directRuleRouteRead() != 0 {
		t.Fatalf("route read = %d, want none on a rule that publishes no fact", cell.directRuleRouteRead())
	}
	if !cell.schemaRuleComplete() {
		t.Fatal("a seated structural row is not complete")
	}
}

// TestAStructuralFamilyInstallsThroughTheOneSeam is the cut itself. A rule
// whose output is an activation row set has no Output semantic for a family
// claim to be fenced by, and it reaches execution through the same
// RuleFamilies table as every other rule. What fences the claim instead is the
// axis its rows are indexed by - the axis whose typed plane the ladder builds
// them on - so one seam serves both declared geometries.
func TestAStructuralFamilyInstallsThroughTheOneSeam(t *testing.T) {
	fixture := structuralLawFixture(t, 0)
	if !BindRuleFamily[uint64](fixture.binding, fixture.slot, fixture.factors[0].Ref(), lawRuleFamilyInstaller{}) {
		t.Fatal("a structural rule could not install the family of its own ordinal")
	}
	if fixture.binding.Poisoned() {
		t.Fatal("an admitted structural family claim poisoned the binding")
	}
	if BindRuleFamily[uint64](fixture.binding, fixture.slot, fixture.factors[0].Ref(), lawRuleFamilyInstaller{}) || !fixture.binding.Poisoned() {
		t.Fatal("a second claim on one structural rule ordinal crossed the one-shot fence")
	}
}

// TestAStructuralFamilyClaimNamesTheAxisItsRowsAreIndexedBy keeps the
// structural fence exact. A claim against any other bound Factor is typed at a
// coordinate the rule's rows are not built on, and the plane that would resolve
// it never sees them.
func TestAStructuralFamilyClaimNamesTheAxisItsRowsAreIndexedBy(t *testing.T) {
	fixture := structuralLawFixture(t, 1)
	if len(fixture.factors) < 2 {
		t.Fatal("the spare Factor")
	}
	output, outputOK := fixture.factors[0].Ordinal()
	spare, spareOK := fixture.factors[1].Ordinal()
	if !outputOK || !spareOK || output == spare {
		t.Fatal("spare Factor ordinal")
	}
	if BindRuleFamily[uint64](fixture.binding, fixture.slot, fixture.factors[1].Ref(), lawRuleFamilyInstaller{}) || !fixture.binding.Poisoned() {
		t.Fatal("a structural family claim against an axis the rule's rows are not indexed by was admitted")
	}
}
