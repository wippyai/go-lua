package rule

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	suspensionowner "github.com/wippyai/go-lua/analysis/domain/suspension/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// TestCompletionConsumesOnlyTheExactSuccessfulGeneration proves successful
// completion is a Suspension-only lifecycle conclusion. Module's ReadySubject
// is used solely to prove success through Link, not retained as a new fact.
func TestCompletionConsumesOnlyTheExactSuccessfulGeneration(t *testing.T) {
	source := initRuleLink(t)
	generations, generationsOK := suspension.NewSchema(source)
	outcome, outcomeOK := moduleInitReadyCompletion(source)
	if !generationsOK || !outcomeOK {
		t.Fatal("successful ModuleInit outcome fixture")
	}
	operand, operandOK := NewCompletionOperand(source, generations, outcome)
	want, wantOK := source.Module().Outcomes().ID(outcome)
	if !operandOK || !wantOK || operand.OutcomeID() != want {
		t.Fatal("completion operand did not retain exact Link outcome identity")
	}

	generation, _, _, provenanceOK := source.Module().Outcomes().Provenance(outcome)
	key, keyOK := generations.KeyForModuleInitGeneration(generation)
	live, liveOK := generations.Live(key, materialization.Recent)
	consumed, consumedOK := consumeLiveCompletion(generations, key, live, true)
	if !provenanceOK || !keyOK || !liveOK || !consumedOK || consumed.MayBeLive() || !consumed.MayBeConsumed() || !generations.Admits(key, consumed) {
		t.Fatal("successful completion did not consume its exact live generation")
	}
	if _, ok := consumeLiveCompletion(generations, key, consumed, true); ok {
		t.Fatal("successful completion fabricated a second consumption")
	}

	composition := engine.NewComposition()
	owner, ownerOK := suspensionowner.Declare(composition, initRuleKey(81), generations)
	declaration, declarationOK := DeclareCompletion(composition, initRuleKey(82), initRuleKey(83), initRuleKey(84), owner)
	if !ownerOK || !declarationOK || declaration == nil || declaration.rule == nil {
		t.Fatal("successful completion declaration")
	}
	if instance, ok := declaration.NewInstance(operand); !ok || instance == nil {
		t.Fatal("successful completion did not bind its exact generation")
	}
}

func TestCompletionOperandRejectsForeignAndNonReadyOutcomes(t *testing.T) {
	left := initRuleLink(t)
	right := initRuleLink(t)
	leftSchema, leftOK := suspension.NewSchema(left)
	rightSchema, rightOK := suspension.NewSchema(right)
	outcome, outcomeOK := moduleInitReadyCompletion(left)
	if !leftOK || !rightOK || !outcomeOK || left.ContentID() != right.ContentID() {
		t.Fatal("replay completion fixture")
	}
	if _, ok := NewCompletionOperand(left, leftSchema, outcome); !ok {
		t.Fatal("left completion operand")
	}
	if _, ok := NewCompletionOperand(left, rightSchema, outcome); ok {
		t.Fatal("completion operand accepted foreign replay schema")
	}
	if _, ok := NewCompletionOperand(left, leftSchema, linkmodule.ModuleInitOutcome{}); ok {
		t.Fatal("completion operand accepted unsealed outcome")
	}
}

func TestCompletionDeclarationFencesEvidenceToOneRuleSemantic(t *testing.T) {
	source := initRuleLink(t)
	generations, generationsOK := suspension.NewSchema(source)
	if !generationsOK {
		t.Fatal("completion generation schema")
	}
	composition := engine.NewComposition()
	owner, ownerOK := suspensionowner.Declare(composition, initRuleKey(91), generations)
	if !ownerOK {
		t.Fatal("completion Suspension owner")
	}
	for _, keys := range [][3]engine.SemanticKey{
		{initRuleKey(92), initRuleKey(92), initRuleKey(93)},
		{initRuleKey(94), initRuleKey(95), initRuleKey(94)},
		{initRuleKey(96), initRuleKey(97), initRuleKey(97)},
	} {
		if declaration, ok := DeclareCompletion(composition, keys[0], keys[1], keys[2], owner); ok || declaration != nil {
			t.Fatalf("completion declaration accepted aliased semantic identities: %#v", keys)
		}
	}
	semantic := initRuleKey(98)
	declaration, declarationOK := DeclareCompletion(composition, semantic, initRuleKey(99), initRuleKey(100), owner)
	if !declarationOK || declaration == nil || !declaration.matchesSemantic(semantic) {
		t.Fatal("completion declaration did not retain exact Rule semantic")
	}
	if declaration.matchesSemantic(initRuleKey(101)) {
		t.Fatal("forged same-shaped Rule semantic could reuse completion evidence")
	}
}

func moduleInitReadyCompletion(source *link.Link) (linkmodule.ModuleInitOutcome, bool) {
	if source == nil {
		return linkmodule.ModuleInitOutcome{}, false
	}
	for generationIndex := 0; generationIndex < source.Module().Generations().Count(); generationIndex++ {
		generation, generationOK := source.Module().Generations().At(generationIndex)
		if !generationOK {
			return linkmodule.ModuleInitOutcome{}, false
		}
		for outcomeIndex := 0; outcomeIndex < source.Module().Outcomes().Count(generation); outcomeIndex++ {
			outcome, outcomeOK := source.Module().Outcomes().At(generation, outcomeIndex)
			if !outcomeOK {
				return linkmodule.ModuleInitOutcome{}, false
			}
			if _, ready := source.Module().Outcomes().ReadySubject(outcome); ready {
				return outcome, true
			}
		}
	}
	return linkmodule.ModuleInitOutcome{}, false
}
