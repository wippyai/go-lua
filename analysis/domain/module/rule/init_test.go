package rule

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/module"
	moduleowner "github.com/wippyai/go-lua/analysis/domain/module/owner"
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	suspensionowner "github.com/wippyai/go-lua/analysis/domain/suspension/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestInitDeclarationBindsOnlyItsExactColdCacheCoordinate(t *testing.T) {
	source := moduleRuleLink(t, `return 1`)
	modules, modulesOK := module.NewSchema(source)
	generation, generationOK := source.Module().Generations().At(0)
	if !modulesOK || !generationOK {
		t.Fatal("Module init fixture")
	}
	operand, operandOK := NewInitOperand(source, modules, generation)
	want, wantOK := source.Module().Generations().ID(generation)
	if !operandOK || !wantOK || operand.GenerationID() != want {
		t.Fatal("init operand did not retain its exact Link generation")
	}
	if _, ok := NewInitOperand(source, modules, linkmodule.ModuleInitGeneration{}); ok {
		t.Fatal("init operand accepted an unsealed generation")
	}

	composition := engine.NewComposition()
	owner, ownerOK := moduleowner.Declare(composition, moduleRuleKey(1), modules)
	declaration, declarationOK := DeclareInit(composition, moduleRuleKey(2), moduleRuleKey(3), moduleRuleKey(4), owner)
	if !ownerOK || !declarationOK || declaration == nil || declaration.rule == nil {
		t.Fatal("Module init cold declaration")
	}
	if instance, ok := declaration.NewInstance(operand); !ok || instance == nil {
		t.Fatal("Module init cold instance")
	}
}

func TestModuleRuleOperandsFenceForeignSchemasAndRebindExactLinkIdentity(t *testing.T) {
	left := moduleRuleLink(t, `error("boom")`)
	right := moduleRuleLink(t, `error("boom")`)
	leftModules, leftModulesOK := module.NewSchema(left)
	leftSuspensions, leftSuspensionsOK := suspension.NewSchema(left)
	rightModules, rightModulesOK := module.NewSchema(right)
	rightSuspensions, rightSuspensionsOK := suspension.NewSchema(right)
	if !leftModulesOK || !leftSuspensionsOK || !rightModulesOK || !rightSuspensionsOK || left.ContentID() != right.ContentID() {
		t.Fatal("replay-equivalent Module schemas")
	}
	generation, generationOK := left.Module().Generations().At(0)
	terminal, terminalOK := left.Module().Terminals().At(0)
	if !generationOK || !terminalOK {
		t.Fatal("Module Rule Link operands")
	}
	if _, ok := NewInitOperand(left, rightModules, generation); ok {
		t.Fatal("init operand accepted a foreign/replayed schema")
	}
	if _, ok := NewRestoreOperand(left, rightModules, leftSuspensions, terminal); ok {
		t.Fatal("restore operand accepted a foreign Module schema")
	}
	if _, ok := NewRestoreOperand(left, leftModules, rightSuspensions, terminal); ok {
		t.Fatal("restore operand accepted a foreign Suspension schema")
	}

	generationRef, generationRefOK := left.Module().Generations().Ref(generation)
	terminalRef, terminalRefOK := left.Module().Terminals().Ref(terminal)
	if !generationRefOK || !terminalRefOK {
		t.Fatal("cold Link identities")
	}
	reboundGeneration, reboundGenerationOK := right.Module().Generations().FindRef(generationRef)
	reboundTerminal, reboundTerminalOK := right.Module().Terminals().FindRef(terminalRef)
	if !reboundGenerationOK || !reboundTerminalOK {
		t.Fatal("replay Link did not rebind exact operands")
	}
	if operand, ok := NewInitOperand(right, rightModules, reboundGeneration); !ok || operand.GenerationID() != mustGenerationID(t, right, reboundGeneration) {
		t.Fatal("rebound init operand")
	}
	restore, restoreOK := NewRestoreOperand(right, rightModules, rightSuspensions, reboundTerminal)
	if !restoreOK || restore.TerminalID() != mustTerminalID(t, right, reboundTerminal) {
		t.Fatal("rebound terminal operand")
	}
	_, generation, coordinate, _, provenanceOK := right.Module().Terminals().Provenance(reboundTerminal)
	moduleKey, moduleKeyOK := rightModules.KeyForCoordinate(coordinate)
	suspensionKey, suspensionKeyOK := rightSuspensions.KeyForModuleInitGeneration(generation)
	pending, pendingOK := rightModules.Pending(moduleKey, generation, materialization.Recent)
	live, liveOK := rightSuspensions.Live(suspensionKey, materialization.Recent)
	restored, restoredOK := restoreTerminal(rightModules, pending, restore, live)
	if !provenanceOK || !moduleKeyOK || !suspensionKeyOK || !pendingOK || !liveOK || !restoredOK || !restored.HasCold() || restored.PendingCount() != 0 {
		t.Fatal("exact terminal did not restore only its matching pending cache state")
	}

	composition := engine.NewComposition()
	moduleFactor, moduleOK := moduleowner.Declare(composition, moduleRuleKey(11), rightModules)
	suspensionFactor, suspensionOK := suspensionowner.Declare(composition, moduleRuleKey(12), rightSuspensions)
	declaration, declarationOK := DeclareRestoreCold(composition, moduleRuleKey(13), moduleRuleKey(14), moduleRuleKey(15), moduleFactor, suspensionFactor)
	if !moduleOK || !suspensionOK || !declarationOK || declaration == nil || declaration.rule == nil {
		t.Fatal("Module terminal cold declaration")
	}
	if instance, ok := declaration.NewInstance(restore); !ok || instance == nil {
		t.Fatal("Module terminal cold instance")
	}
}

func mustGenerationID(t testing.TB, source *link.Link, generation linkmodule.ModuleInitGeneration) keyspace.ContentID {
	t.Helper()
	id, ok := source.Module().Generations().ID(generation)
	if !ok {
		t.Fatal("generation identity")
	}
	return id
}

func mustTerminalID(t testing.TB, source *link.Link, terminal linkmodule.ModuleInitTerminal) keyspace.ContentID {
	t.Helper()
	id, ok := source.Module().Terminals().ID(terminal)
	if !ok {
		t.Fatal("terminal identity")
	}
	return id
}

func moduleRuleLink(t testing.TB, dependencySource string) *link.Link {
	t.Helper()
	main, err := lower.Lower(lower.Source{Name: "main.lua", Text: []byte(`require("dependency")`)})
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := lower.Lower(lower.Source{Name: "dependency.lua", Text: []byte(dependencySource)})
	if err != nil {
		t.Fatal(err)
	}
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	literal := func(text string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
	}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{require}, Input: target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed},
		}},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: literal("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: literal("__module_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: literal("require"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: require}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: literal("_G")}, {Name: "__module_absent", Root: "GlobalEnvRoot", Key: literal("__module_absent")}, {Name: "require", Root: "GlobalEnvRoot", Key: literal("require")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	imported, ok := main.Module().ImportAt(0)
	if !ok {
		t.Fatal("source import")
	}
	source, err := link.Seal(&link.Spec{
		Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}},
		Module: linkmodule.Spec{Actors: []linkmodule.ActorSpec{{Name: "actor"}},
			ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{{Actor: "actor", Instances: []string{"cache-main", "cache-dependency"}, Representative: "cache-main"}},
			AnalysisRoots:      []linkmodule.AnalysisRootSpec{{Name: "main-root", Module: "main", Actor: "actor", Instance: "cache-main"}, {Name: "dependency-root", Module: "dependency", Actor: "actor", Instance: "cache-dependency"}},
			ModuleCacheEntries: []linkmodule.ModuleCacheEntrySpec{{Module: "main", Import: imported.Term, FromRoot: "main-root", ToRoot: "dependency-root"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func moduleRuleKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("module rule semantic key")
	}
	return key
}
