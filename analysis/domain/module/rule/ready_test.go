package rule

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/module"
	moduleowner "github.com/wippyai/go-lua/analysis/domain/module/owner"
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	suspensionowner "github.com/wippyai/go-lua/analysis/domain/suspension/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

func TestReadyRulePublishesOnlyTheExactLiveGenerationAndSubject(t *testing.T) {
	source := moduleRuleLink(t, `return 1`)
	modules, modulesOK := module.NewSchema(source)
	generations, generationsOK := suspension.NewSchema(source)
	outcome, outcomeOK := moduleInitReadyOutcome(source)
	if !modulesOK || !generationsOK || !outcomeOK {
		t.Fatal("successful ModuleInit fixture")
	}
	operand, operandOK := NewReadyOperand(source, modules, generations, outcome)
	wantID, wantIDOK := source.Module().Outcomes().ID(outcome)
	if !operandOK || !wantIDOK || operand.OutcomeID() != wantID {
		t.Fatal("ready operand did not retain exact Link completion identity")
	}

	generation, _, _, provenanceOK := source.Module().Outcomes().Provenance(outcome)
	_, coordinate, _, _, entryOK := source.Module().Generations().Entry(generation)
	moduleKey, moduleKeyOK := modules.KeyForCoordinate(coordinate)
	suspensionKey, suspensionKeyOK := generations.KeyForModuleInitGeneration(generation)
	pending, pendingOK := modules.Pending(moduleKey, generation, materialization.Recent)
	live, liveOK := generations.Live(suspensionKey, materialization.Recent)
	published, publishedOK := publishReady(modules, pending, operand, live)
	if !provenanceOK || !entryOK || !moduleKeyOK || !suspensionKeyOK || !pendingOK || !liveOK || !publishedOK || published.PendingCount() != 0 || published.ReadyCount() != 1 || !modules.Admits(moduleKey, published) {
		t.Fatal("ready publication did not consume precisely the live pending alternative")
	}
	readyGeneration, subject, readyOK := published.ReadyAt(0)
	if !readyOK || readyGeneration != generation || subject != operand.subject {
		t.Fatal("ready publication changed Link-selected generation or subject")
	}

	consumed, consumedOK := generations.ConsumeLive(live, suspensionKey, materialization.Recent)
	if !consumedOK {
		t.Fatal("consumed premise")
	}
	if _, ok := publishReady(modules, pending, operand, consumed); ok {
		t.Fatal("consumed generation published Ready")
	}

	summaryPending, summaryPendingOK := modules.ReplacePending(pending, moduleKey, generation, materialization.Recent, generation, materialization.Summary)
	summaryLive, summaryLiveOK := generations.Materialize(live, suspensionKey)
	summaryPublished, summaryPublishedOK := publishReady(modules, summaryPending, operand, summaryLive)
	if !summaryPendingOK || !summaryLiveOK || !summaryPublishedOK || summaryPublished.ReadyCount() != 1 || summaryPublished.PendingCount() != 0 {
		t.Fatal("same generation summary lifecycle was not published")
	}
	if _, ok := publishReady(modules, summaryPending, operand, live); ok {
		t.Fatal("mismatched materialization age published Ready")
	}
}

func TestReadyOperandRejectsNonSuccessAndForeignSchema(t *testing.T) {
	success := moduleRuleLink(t, `return 1`)
	replayed := moduleRuleLink(t, `return 1`)
	failure := moduleRuleLink(t, `error("boom")`)
	successModules, successModulesOK := module.NewSchema(success)
	successSuspensions, successSuspensionsOK := suspension.NewSchema(success)
	replayedModules, replayedModulesOK := module.NewSchema(replayed)
	replayedSuspensions, replayedSuspensionsOK := suspension.NewSchema(replayed)
	failureModules, failureModulesOK := module.NewSchema(failure)
	failureSuspensions, failureSuspensionsOK := suspension.NewSchema(failure)
	successOutcome, successOutcomeOK := moduleInitReadyOutcome(success)
	if !successModulesOK || !successSuspensionsOK || !replayedModulesOK || !replayedSuspensionsOK || !failureModulesOK || !failureSuspensionsOK || !successOutcomeOK || success.ContentID() != replayed.ContentID() {
		t.Fatal("ready operand fixtures")
	}
	if _, ok := NewReadyOperand(success, failureModules, successSuspensions, successOutcome); ok {
		t.Fatal("ready operand accepted a foreign Module schema")
	}
	if _, ok := NewReadyOperand(success, successModules, failureSuspensions, successOutcome); ok {
		t.Fatal("ready operand accepted a foreign Suspension schema")
	}
	if _, ok := NewReadyOperand(success, replayedModules, successSuspensions, successOutcome); ok {
		t.Fatal("ready operand accepted a replayed foreign Module schema")
	}
	if _, ok := NewReadyOperand(success, successModules, replayedSuspensions, successOutcome); ok {
		t.Fatal("ready operand accepted a replayed foreign Suspension schema")
	}
	throw, throwOK := moduleInitOutcomeOfKind(failure, flowkind.OutcomeThrow)
	if !throwOK {
		t.Fatal("throw ModuleInit outcome")
	}
	if _, ok := NewReadyOperand(failure, failureModules, failureSuspensions, throw); ok {
		t.Fatal("ready operand accepted Throw ModuleInit completion")
	}
}

func TestReadyDeclarationFencesSemanticIdentity(t *testing.T) {
	source := moduleRuleLink(t, `return 1`)
	modules, modulesOK := module.NewSchema(source)
	generations, generationsOK := suspension.NewSchema(source)
	outcome, outcomeOK := moduleInitReadyOutcome(source)
	if !modulesOK || !generationsOK || !outcomeOK {
		t.Fatal("ready declaration fixture")
	}
	operand, operandOK := NewReadyOperand(source, modules, generations, outcome)
	composition := engine.NewComposition()
	moduleFactor, moduleOK := moduleowner.Declare(composition, moduleRuleKey(61), modules)
	suspensionFactor, suspensionOK := suspensionowner.Declare(composition, moduleRuleKey(62), generations)
	if !operandOK || !moduleOK || !suspensionOK {
		t.Fatal("ready declaration prerequisites")
	}
	for _, keys := range [][3]engine.SemanticKey{
		{moduleRuleKey(63), moduleRuleKey(63), moduleRuleKey(64)},
		{moduleRuleKey(65), moduleRuleKey(66), moduleRuleKey(65)},
		{moduleRuleKey(67), moduleRuleKey(68), moduleRuleKey(68)},
	} {
		if declaration, ok := DeclarePublishReady(composition, keys[0], keys[1], keys[2], moduleFactor, suspensionFactor); ok || declaration != nil {
			t.Fatalf("ready declaration accepted aliased semantic identities: %#v", keys)
		}
	}
	declaration, declarationOK := DeclarePublishReady(composition, moduleRuleKey(69), moduleRuleKey(70), moduleRuleKey(71), moduleFactor, suspensionFactor)
	if !declarationOK || declaration == nil || declaration.rule == nil {
		t.Fatal("ready declaration")
	}
	if instance, ok := declaration.NewInstance(operand); !ok || instance == nil {
		t.Fatal("ready instance")
	}
	forgedSubject := operand
	forgedSubject.subject = linkmodule.ModuleReadySubject{}
	if instance, ok := declaration.NewInstance(forgedSubject); ok || instance != nil {
		t.Fatal("ready instance accepted a forged Link subject")
	}
	forgedOutcome := operand
	forgedOutcome.outcome = linkmodule.ModuleInitOutcome{}
	if instance, ok := declaration.NewInstance(forgedOutcome); ok || instance != nil {
		t.Fatal("ready instance accepted a forged Link completion")
	}
	if evidence, accepted := declaration.check(engine.RuleDerivation[module.Value, ReadyOperand]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged empty derivation passed ready evidence")
	}
}

func moduleInitReadyOutcome(source *link.Link) (linkmodule.ModuleInitOutcome, bool) {
	if source == nil {
		return linkmodule.ModuleInitOutcome{}, false
	}
	generation, generationOK := source.Module().Generations().At(0)
	if !generationOK {
		return linkmodule.ModuleInitOutcome{}, false
	}
	for index := 0; index < source.Module().Outcomes().Count(generation); index++ {
		outcome, ok := source.Module().Outcomes().At(generation, index)
		if !ok {
			return linkmodule.ModuleInitOutcome{}, false
		}
		if _, ok := source.Module().Outcomes().ReadySubject(outcome); ok {
			return outcome, true
		}
	}
	return linkmodule.ModuleInitOutcome{}, false
}

func moduleInitOutcomeOfKind(source *link.Link, wanted flowkind.OutcomeKind) (linkmodule.ModuleInitOutcome, bool) {
	if source == nil {
		return linkmodule.ModuleInitOutcome{}, false
	}
	generation, generationOK := source.Module().Generations().At(0)
	if !generationOK {
		return linkmodule.ModuleInitOutcome{}, false
	}
	for index := 0; index < source.Module().Outcomes().Count(generation); index++ {
		outcome, ok := source.Module().Outcomes().At(generation, index)
		kind, kindOK := source.Module().Outcomes().Kind(outcome)
		if ok && kindOK && kind == wanted {
			return outcome, true
		}
	}
	return linkmodule.ModuleInitOutcome{}, false
}
