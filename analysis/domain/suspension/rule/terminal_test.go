package rule

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
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

// TestCancelOperandAndRuleConsumeOnlyTheExactModuleInitGeneration proves that
// Cancel is a Suspension-owned transition.  In particular, it does not use a
// cache coordinate, accepts no Throw terminal, and preserves the exact
// consumed premise required by Module's sibling RestoreCold Rule.
func TestCancelOperandAndRuleConsumeOnlyTheExactModuleInitGeneration(t *testing.T) {
	source := cancelTerminalLink(t)
	generations, generationsOK := suspension.NewSchema(source)
	terminal, terminalOK := moduleInitTerminalOfKind(source, flowkind.OutcomeCancel)
	throwTerminal, throwOK := moduleInitTerminalOfKind(source, flowkind.OutcomeThrow)
	if !generationsOK || !terminalOK || !throwOK {
		t.Fatal("cancel and throw terminal fixture")
	}
	operand, operandOK := NewCancelOperand(source, generations, terminal)
	want, wantOK := source.Module().Terminals().ID(terminal)
	if !operandOK || !wantOK || operand.TerminalID() != want {
		t.Fatal("Cancel operand did not retain exact terminal identity")
	}
	if _, ok := NewCancelOperand(source, generations, throwTerminal); ok {
		t.Fatal("Cancel Rule admitted a Throw terminal")
	}
	if _, ok := NewCancelOperand(source, generations, linkmodule.ModuleInitTerminal{}); ok {
		t.Fatal("Cancel Rule admitted an unsealed terminal")
	}

	_, generation, _, kind, provenanceOK := source.Module().Terminals().Provenance(terminal)
	key, keyOK := generations.KeyForModuleInitGeneration(generation)
	live, liveOK := generations.Live(key, materialization.Recent)
	consumed, consumedOK := consumeLiveCompletion(generations, key, live, true)
	if !provenanceOK || kind != flowkind.OutcomeCancel || !keyOK || !liveOK || !consumedOK || consumed.MayBeLive() || !consumed.MayBeConsumed() || !generations.Admits(key, consumed) {
		t.Fatal("Cancel did not establish exactly the consumed Suspension premise")
	}
	if _, ok := consumeLiveCompletion(generations, key, consumed, true); ok {
		t.Fatal("Cancel fabricated a second consume conclusion")
	}
	if _, ok := consumeLiveCompletion(generations, key, live, false); ok {
		t.Fatal("absent input fabricated a consumption")
	}
	summary, summaryOK := generations.Materialize(live, key)
	summaryConsumed, summaryConsumedOK := consumeLiveCompletion(generations, key, summary, true)
	role, stillLive, consumedAtSummary, _, lifecycleOK := summaryConsumed.LifecycleAt(0)
	if !summaryOK || !summaryConsumedOK || !lifecycleOK || role != materialization.Summary || stillLive || !consumedAtSummary {
		t.Fatal("Cancel did not consume the same generation after materialization")
	}

	composition := engine.NewComposition()
	owner, ownerOK := suspensionowner.Declare(composition, initRuleKey(31), generations)
	declaration, declarationOK := DeclareCancel(composition, initRuleKey(32), initRuleKey(33), initRuleKey(34), owner)
	if !ownerOK || !declarationOK || declaration == nil || declaration.rule == nil {
		t.Fatal("cold Suspension Cancel Rule declaration")
	}
	if instance, ok := declaration.NewInstance(operand); !ok || instance == nil {
		t.Fatal("Cancel Rule did not bind its exact generation to itself")
	}
}

// TestCancelOperandFencesForeignSuspensionSchemas makes the generation source
// and the owner capability one Link-local relation, even for content-equal
// replayed Links.
func TestCancelOperandFencesForeignSuspensionSchemas(t *testing.T) {
	left := cancelTerminalLink(t)
	right := cancelTerminalLink(t)
	leftSchema, leftOK := suspension.NewSchema(left)
	rightSchema, rightOK := suspension.NewSchema(right)
	terminal, terminalOK := moduleInitTerminalOfKind(left, flowkind.OutcomeCancel)
	if !leftOK || !rightOK || !terminalOK || left.ContentID() != right.ContentID() {
		t.Fatal("replay cancel fixture")
	}
	if _, ok := NewCancelOperand(left, leftSchema, terminal); !ok {
		t.Fatal("left Cancel operand")
	}
	if _, ok := NewCancelOperand(left, rightSchema, terminal); ok {
		t.Fatal("Cancel operand accepted a foreign replay schema")
	}
	ref, refOK := left.Module().Terminals().Ref(terminal)
	rebound, reboundOK := right.Module().Terminals().FindRef(ref)
	if !refOK || !reboundOK {
		t.Fatal("cancel terminal rebinding")
	}
	if operand, ok := NewCancelOperand(right, rightSchema, rebound); !ok || operand.TerminalID() != mustCancelTerminalID(t, right, rebound) {
		t.Fatal("rebound Cancel operand")
	}
}

// TestCancelDeclarationFencesEvidenceToOneRuleSemantic proves the local
// admission theorem cannot be replayed by another Rule with the same read,
// write, and operand shapes.  The three declaration identities also must be
// distinct, so a rule identity cannot double as its operand or evidence
// family.
func TestCancelDeclarationFencesEvidenceToOneRuleSemantic(t *testing.T) {
	source := cancelTerminalLink(t)
	generations, generationsOK := suspension.NewSchema(source)
	if !generationsOK {
		t.Fatal("cancel generation schema")
	}
	composition := engine.NewComposition()
	owner, ownerOK := suspensionowner.Declare(composition, initRuleKey(41), generations)
	if !ownerOK {
		t.Fatal("Suspension owner")
	}
	for _, keys := range [][3]engine.SemanticKey{
		{initRuleKey(42), initRuleKey(42), initRuleKey(43)},
		{initRuleKey(44), initRuleKey(45), initRuleKey(44)},
		{initRuleKey(46), initRuleKey(47), initRuleKey(47)},
	} {
		if declaration, ok := DeclareCancel(composition, keys[0], keys[1], keys[2], owner); ok || declaration != nil {
			t.Fatalf("Cancel declaration accepted aliased semantic identities: %#v", keys)
		}
	}
	semantic := initRuleKey(48)
	declaration, declarationOK := DeclareCancel(composition, semantic, initRuleKey(49), initRuleKey(50), owner)
	if !declarationOK || declaration == nil || !declaration.matchesSemantic(semantic) {
		t.Fatal("Cancel declaration did not retain its exact Rule semantic")
	}
	if declaration.matchesSemantic(initRuleKey(51)) {
		t.Fatal("forged same-shaped Rule semantic could reuse Cancel evidence")
	}
}

func moduleInitTerminalOfKind(source *link.Link, wanted flowkind.OutcomeKind) (linkmodule.ModuleInitTerminal, bool) {
	for index := 0; index < source.Module().Terminals().Count(); index++ {
		terminal, ok := source.Module().Terminals().At(index)
		if !ok {
			return linkmodule.ModuleInitTerminal{}, false
		}
		_, _, _, kind, ok := source.Module().Terminals().Provenance(terminal)
		if ok && kind == wanted {
			return terminal, true
		}
	}
	return linkmodule.ModuleInitTerminal{}, false
}

func mustCancelTerminalID(t testing.TB, source *link.Link, terminal linkmodule.ModuleInitTerminal) keyspace.ContentID {
	t.Helper()
	id, ok := source.Module().Terminals().ID(terminal)
	if !ok {
		t.Fatal("cancel terminal identity")
	}
	return id
}

func cancelTerminalLink(t testing.TB) *link.Link {
	t.Helper()
	main, err := lower.Lower(lower.Source{Name: "main.lua", Text: []byte(`require("dependency")`)})
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := lower.Lower(lower.Source{Name: "dependency.lua", Text: []byte(`cancel()`)})
	if err != nil {
		t.Fatal(err)
	}
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	cancel := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"cancel"}}
	literal := func(text string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
	}
	closed := target.ValuesSpec{Tail: target.ValuesClosed}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{
			{Bindings: []target.BindingSpec{require}, Input: closed, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed}}, Effects: target.RowSpec{Tail: target.RowClosed}},
			{Bindings: []target.BindingSpec{cancel}, Input: closed, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeCancel, Values: closed}}, Effects: target.RowSpec{Tail: target.RowClosed}},
		},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: literal("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: literal("__module_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: literal("require"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: require}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: literal("cancel"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: cancel}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: literal("_G")},
			{Name: "__module_absent", Root: "GlobalEnvRoot", Key: literal("__module_absent")},
			{Name: "require", Root: "GlobalEnvRoot", Key: literal("require")},
			{Name: "cancel", Root: "GlobalEnvRoot", Key: literal("cancel")},
		},
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
