package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// structural_publication_width_law_test.go states one reading for how wide a
// rule's publication is on the runtime path.
//
// A structural rule computes no fact. Its graph members carry no write, its
// catalog rows open no Factor patch slot, and its family's Run holds none. The
// runtime used to read all three off the fact-publishing shape - one write per
// member, one output handle per row, at least one output column per family -
// because until the A form every rule published a fact. A rule that publishes
// structurally then fails the plane bind, so no member of the whole program is
// ever bound, while the compile path - which builds none of these tables -
// reports the same source complete. These laws hold each width to the rule's
// own declared disposition instead.

// TestEveryPublicationDispositionStatesItsGraphMemberWriteArity is the
// exhaustive half. The graph binding catalog validates a member against the
// arity its rule declares, so every disposition the sealed cell vocabulary can
// carry must name one here; a later disposition with no arm is refused rather
// than admitted under the writing shape.
func TestEveryPublicationDispositionStatesItsGraphMemberWriteArity(t *testing.T) {
	for _, expectation := range []struct {
		mode  directRuleWriteMode
		count int
		named bool
	}{
		{mode: directRuleWriteExact, count: 1, named: true},
		{mode: directRuleWriteRoute, count: 1, named: true},
		{mode: directRuleWriteStructural, count: 0, named: true},
		{mode: directRuleWriteStructural + 1},
		{mode: 0},
	} {
		rule := writeModeLawGeometry{mode: expectation.mode}
		count, named := graphMemberWriteArity(rule)
		if named != expectation.named || count != expectation.count {
			t.Fatalf("disposition %d has arity %d/%t, want %d/%t", expectation.mode, count, named, expectation.count, expectation.named)
		}
		if structural := structuralGraphMember(rule); structural != (expectation.named && expectation.count == 0) {
			t.Fatalf("disposition %d classifies structural=%t", expectation.mode, structural)
		}
	}
	if _, named := graphMemberWriteArity(nil); named {
		t.Fatal("an absent rule named a member write arity")
	}
}

// TestAStructuralRowOpensNoPatchSlot pins the invocation width to the same
// declaration. The catalog row's output handles and the family's Run are sized
// independently - the row here, the emitted family there - so they agree only
// while both read the publication mode. A structural row that still claimed one
// handle would be refused at Issue by the zero-width Run its own family
// declares, which is where the whole solve stopped.
func TestAStructuralRowOpensNoPatchSlot(t *testing.T) {
	fixture := structuralLawFixture(t, 0)
	descriptor, descriptorOK := fixture.schema.generatedProgramAt(0)
	if !descriptorOK {
		t.Fatal("the sealed structural descriptor")
	}
	mode, modeOK := descriptor.OutputMode()
	if !modeOK || mode != ruleprogram.ModeStructural {
		t.Fatalf("descriptor mode = %d/%t, want the structural publication", mode, modeOK)
	}
	width, widthOK := generatedRowOutputWidth(descriptor)
	if !widthOK || width != 0 {
		t.Fatalf("structural row width = %d/%t, want no patch slot", width, widthOK)
	}
	// The Run a zero-width family mints is a live invocation lane, not an
	// absent one: it holds no patch slot and issues all the same.
	if run := execution.NewRun(1, int(width)); run == nil {
		t.Fatal("a zero-width family minted no Run")
	}
}

// TestAFactPublicationStillOpensItsOneSlot keeps the structural arm from
// widening into the fact lane: a rule that concludes a fact publishes through
// exactly the one slot its output addresses.
func TestAFactPublicationStillOpensItsOneSlot(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawSelected, generatedRuleLawRuleRole))
	descriptor, descriptorOK := fixture.schema.generatedProgramAt(0)
	if !descriptorOK {
		t.Fatal("the sealed fact descriptor")
	}
	if mode, modeOK := descriptor.OutputMode(); !modeOK || mode == ruleprogram.ModeStructural {
		t.Fatalf("descriptor mode = %d/%t, want a fact publication", mode, modeOK)
	}
	width, widthOK := generatedRowOutputWidth(descriptor)
	if !widthOK || width != 1 {
		t.Fatalf("fact row width = %d/%t, want its one patch slot", width, widthOK)
	}
}

// writeModeLawGeometry is a sealed rule geometry that answers only for its
// publication disposition. The arity law asks nothing else of a rule, so this
// keeps the enumeration over the vocabulary itself rather than over whichever
// cells a fixture happens to seal.
type writeModeLawGeometry struct {
	sealedRuleGeometry
	mode directRuleWriteMode
}

func (geometry writeModeLawGeometry) directRuleWriteMode() directRuleWriteMode { return geometry.mode }
