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

// TestInitOperandAndRuleUseOneCanonicalGeneration proves the Suspension child
// neither reconstructs a cache/generation pair nor reads a Module Rule patch.
// This is deliberately a cold-schema test: no Program body, engine Assembly,
// or Solver is created.
func TestInitOperandAndRuleUseOneCanonicalGeneration(t *testing.T) {
	source := initRuleLink(t)
	modules, modulesOK := module.NewSchema(source)
	generations, generationsOK := suspension.NewSchema(source)
	generation, generationOK := source.Module().Generations().At(0)
	if !modulesOK || !generationsOK || !generationOK {
		t.Fatal("sealed ModuleInit fixture")
	}
	operand, operandOK := NewInitOperand(source, modules, generations, generation)
	want, wantOK := source.Module().Generations().ID(generation)
	if !operandOK || !wantOK || operand.GenerationID() != want {
		t.Fatal("operand did not retain the exact canonical generation evidence")
	}
	if _, ok := NewInitOperand(source, modules, generations, linkmodule.ModuleInitGeneration{}); ok {
		t.Fatal("operand accepted an unsealed generation handle")
	}

	composition := engine.NewComposition()
	moduleFactor, moduleOK := moduleowner.Declare(composition, initRuleKey(1), modules)
	suspensionFactor, suspensionOK := suspensionowner.Declare(composition, initRuleKey(2), generations)
	declaration, declarationOK := DeclareInit(composition, initRuleKey(3), initRuleKey(4), initRuleKey(5), moduleFactor, suspensionFactor)
	if !moduleOK || !suspensionOK || !declarationOK || declaration == nil || declaration.rule == nil {
		t.Fatal("cold Suspension ModuleInit Rule declaration")
	}
	if instance, ok := declaration.NewInstance(operand); !ok || instance == nil {
		t.Fatal("cold Rule did not retain its typed instance constructor")
	}
}

// TestInitRejectsSameContentForeignSchemas keeps replay identity separate from
// the live Link capability. Independently sealed same-content schemas may be
// replay-equivalent, but they cannot supply one another's Rule coordinates.
func TestInitRejectsSameContentForeignSchemas(t *testing.T) {
	left := initRuleLink(t)
	right := initRuleLink(t)
	leftModules, leftModulesOK := module.NewSchema(left)
	leftGenerations, leftGenerationsOK := suspension.NewSchema(left)
	rightModules, rightModulesOK := module.NewSchema(right)
	rightGenerations, rightGenerationsOK := suspension.NewSchema(right)
	generation, generationOK := left.Module().Generations().At(0)
	if !leftModulesOK || !leftGenerationsOK || !rightModulesOK || !rightGenerationsOK || !generationOK || left.ContentID() != right.ContentID() {
		t.Fatal("same-content ModuleInit schemas")
	}
	if _, ok := NewInitOperand(left, rightModules, leftGenerations, generation); ok {
		t.Fatal("init operand accepted a same-content foreign Module schema")
	}
	if _, ok := NewInitOperand(left, leftModules, rightGenerations, generation); ok {
		t.Fatal("init operand accepted a same-content foreign Suspension schema")
	}

	composition := engine.NewComposition()
	leftModuleOwner, leftModuleOK := moduleowner.Declare(composition, initRuleKey(81), leftModules)
	rightSuspensionOwner, rightSuspensionOK := suspensionowner.Declare(composition, initRuleKey(82), rightGenerations)
	if !leftModuleOK || !rightSuspensionOK {
		t.Fatal("foreign owner fixtures")
	}
	if declaration, ok := DeclareInit(composition, initRuleKey(83), initRuleKey(84), initRuleKey(85), leftModuleOwner, rightSuspensionOwner); ok || declaration != nil {
		t.Fatal("init declaration accepted independently sealed foreign owners")
	}

	localSuspensionOwner, localSuspensionOK := suspensionowner.Declare(composition, initRuleKey(86), leftGenerations)
	if !localSuspensionOK {
		t.Fatal("local Suspension owner")
	}
	declaration, declarationOK := DeclareInit(composition, initRuleKey(87), initRuleKey(88), initRuleKey(89), leftModuleOwner, localSuspensionOwner)
	if !declarationOK || declaration == nil {
		t.Fatal("local init declaration rejected")
	}
	operand, operandOK := NewInitOperand(left, leftModules, leftGenerations, generation)
	if !operandOK {
		t.Fatal("local init operand rejected")
	}
	if instance, ok := declaration.NewInstance(operand); !ok || instance == nil {
		t.Fatal("local init replay rejected")
	}
}

// TestInitDeclarationFencesEvidenceToItsExactRuleSemantic makes the Module
// predecessor/Suspension-output shape insufficient to replay initiation
// evidence.  Its identities are independent and a forged same-shaped Rule is
// rejected before any transfer conclusion can be admitted.
func TestInitDeclarationFencesEvidenceToItsExactRuleSemantic(t *testing.T) {
	source := initRuleLink(t)
	modules, modulesOK := module.NewSchema(source)
	generations, generationsOK := suspension.NewSchema(source)
	if !modulesOK || !generationsOK {
		t.Fatal("ModuleInit schemas")
	}
	composition := engine.NewComposition()
	moduleFactor, moduleOK := moduleowner.Declare(composition, initRuleKey(61), modules)
	suspensionFactor, suspensionOK := suspensionowner.Declare(composition, initRuleKey(62), generations)
	if !moduleOK || !suspensionOK {
		t.Fatal("ModuleInit factors")
	}
	for _, keys := range [][3]engine.SemanticKey{
		{initRuleKey(63), initRuleKey(63), initRuleKey(64)},
		{initRuleKey(65), initRuleKey(66), initRuleKey(65)},
		{initRuleKey(67), initRuleKey(68), initRuleKey(68)},
	} {
		if declaration, ok := DeclareInit(composition, keys[0], keys[1], keys[2], moduleFactor, suspensionFactor); ok || declaration != nil {
			t.Fatalf("ModuleInit declaration accepted aliased semantic identities: %#v", keys)
		}
	}
	semantic := initRuleKey(69)
	declaration, declarationOK := DeclareInit(composition, semantic, initRuleKey(70), initRuleKey(71), moduleFactor, suspensionFactor)
	if !declarationOK || declaration == nil || !declaration.matchesSemantic(semantic) {
		t.Fatal("ModuleInit declaration did not retain exact Rule semantic")
	}
	if declaration.matchesSemantic(initRuleKey(72)) {
		t.Fatal("forged same-shaped Rule semantic could reuse ModuleInit evidence")
	}
}

// TestInitLivePreservesColdInsideTop guards the exact may-law used by both
// transfer and evidence: Top includes Cold, while a known non-Cold cache
// relation must not mint a fresh generation.
func TestInitLivePreservesColdInsideTop(t *testing.T) {
	source := initRuleLink(t)
	modules, modulesOK := module.NewSchema(source)
	generations, generationsOK := suspension.NewSchema(source)
	generation, generationOK := source.Module().Generations().At(0)
	operand, operandOK := NewInitOperand(source, modules, generations, generation)
	if !modulesOK || !generationsOK || !generationOK || !operandOK {
		t.Fatal("ModuleInit input")
	}
	want, wantOK := generations.Live(operand.suspensionKey, materialization.Recent)
	if !wantOK {
		t.Fatal("exact fresh generation")
	}
	for name, current := range map[string]module.Value{
		"cold": mustModuleDefault(t, modules),
		"top":  mustModuleTop(t, modules),
	} {
		got, ok := initLive(modules, operand.moduleKey, generations, operand.suspensionKey, current, true)
		if !ok || !generations.Equal(got, want) {
			t.Fatalf("%s Module predecessor did not retain its Cold -> live consequence", name)
		}
	}
	suspensionGeneration, suspensionGenerationOK := operand.suspensionKey.ModuleInitGeneration()
	pending, pendingOK := modules.Pending(operand.moduleKey, suspensionGeneration, materialization.Recent)
	if !suspensionGenerationOK || !pendingOK || pending.HasCold() {
		t.Fatal("known non-Cold pending predecessor")
	}
	if got, ok := initLive(modules, operand.moduleKey, generations, operand.suspensionKey, pending, true); ok || got.Valid() {
		t.Fatal("known non-Cold predecessor minted a fresh live generation")
	}
	if got, ok := initLive(modules, operand.moduleKey, generations, operand.suspensionKey, mustModuleDefault(t, modules), false); ok || got.Valid() {
		t.Fatal("absent Module predecessor minted a fresh live generation")
	}
}

func mustModuleDefault(t testing.TB, schema module.Schema) module.Value {
	t.Helper()
	value, ok := schema.Default()
	if !ok {
		t.Fatal("Module Cold")
	}
	return value
}

func mustModuleTop(t testing.TB, schema module.Schema) module.Value {
	t.Helper()
	value, ok := schema.Top()
	if !ok {
		t.Fatal("Module Top")
	}
	return value
}

func initRuleLink(t testing.TB) *link.Link {
	t.Helper()
	main, err := lower.Lower(lower.Source{Name: "main.lua", Text: []byte(`require("dependency")`)})
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := lower.Lower(lower.Source{Name: "dependency.lua", Text: []byte(`return 1`)})
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

func initRuleKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("suspension rule test semantic key")
	}
	return key
}
