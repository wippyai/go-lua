package analysis

import (
	"context"
	"reflect"
	"strings"
	"testing"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
	"github.com/wippyai/go-lua/program/target/profile"
)

func TestCanonicalCompositionSealsSixteenRules(t *testing.T) {
	declared, ok := newProgramAnalysis(directFieldHostileLink(t, `return 1`))
	if !ok || declared == nil || declared.composition == nil || !declared.composition.Sealed() {
		t.Fatal("production composition did not seal")
	}
	inventory, ok := declared.composition.RuleAdmissionInventory()
	if !ok || !inventory.ID.Available() || len(inventory.Rules) != 16 {
		t.Fatalf("rule inventory = %t/%d", ok, len(inventory.Rules))
	}
	report, ok := declared.composition.SemanticReport()
	if !ok || len(report.Rules) != 16 || len(report.Queries) != 2 {
		t.Fatalf("report = %t rules:%d queries:%d", ok, len(report.Rules), len(report.Queries))
	}
	factors := 0
	for _, component := range report.Components {
		factors += len(component.Factors)
	}
	if factors != 5 {
		t.Fatalf("factor components = %d, want 5", factors)
	}
}

func TestCanonicalTargetProfileAdmitsLiteralReturn(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "literal.lua", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil || result.BodyCount() != 1 {
		t.Fatalf("literal = %d/%v", status, result)
	}
	body, ok := result.BodyAt(0)
	present, top, effectOK := body.EffectDisposition()
	if !ok || !effectOK || present || top || body.EffectCount() != 0 || body.ValueCount() == 0 {
		t.Fatal("literal detached projection")
	}
}

func TestCanonicalIndexedAssignmentAdmitsPublicAnalyze(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `
type Profile = {
    id: string,
    count: number,
    flag: boolean,
    label: string?,
    tags: {[string]: string},
}

type Admin = {
    kind: "admin",
    id: string,
    level: number,
}

type Guest = {
    kind: "guest",
    id: string,
    expires: number,
}

type Principal = Admin | Guest
type Result<T> = {ok: true, value: T} | {ok: false, error: string}
local function profile(id: string, count: number, label: string?): Profile
    local tags: {[string]: string} = {}
    tags["source"] = id
    return {id = id, count = count, flag = count > 0, label = label, tags = tags}
end
return "ok"
`, contract)
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil || result.BodyCount() == 0 {
		t.Fatalf("indexed assignment = %d/%v", status, result)
	}
}

func TestCanonicalGlobalOperationEffectRunsThroughPublicAnalyze(t *testing.T) {
	linked := mustLink(t, `_G.emit()`, oracleEffectTarget(t))
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil || result.BodyCount() != 1 {
		t.Fatalf("global = %d/%v", status, result)
	}
	body, ok := result.BodyAt(0)
	present, top, effectOK := body.EffectDisposition()
	if !ok || !effectOK || !present || top || body.EffectCount() != 1 {
		t.Fatal("global effect projection")
	}
}

// Host owns Heap bootstrap at BootRoot scope. Two mounted Programs can each
// publish a GlobalBinding for the same actor-local root; that must not cause
// the canonical source traversal to issue the same Heap operand twice.
func TestCanonicalHeapBootstrapOncePerSharedBootRoot(t *testing.T) {
	first, err := lower.Lower(lower.Source{Name: "first-global.lua", Text: []byte(`return _G`)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := lower.Lower(lower.Source{Name: "second-global.lua", Text: []byte(`return _G`)})
	if err != nil {
		t.Fatal(err)
	}
	contract := oracleEffectTarget(t)
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{
		{Name: "first", Program: first},
		{Name: "second", Program: second},
	}})
	if err != nil {
		t.Fatal(err)
	}
	globals := linked.Host().Globals()
	if globals.Count() != 2 || linked.Host().BootRoots().Count() != 1 {
		t.Fatalf("shared-root fixture globals=%d boot-roots=%d", globals.Count(), linked.Host().BootRoots().Count())
	}
	firstGlobal, firstGlobalOK := globals.At(0)
	secondGlobal, secondGlobalOK := globals.At(1)
	_, firstBoot, _, _, _, _, firstMappingOK := globals.Mapping(firstGlobal)
	_, secondBoot, _, _, _, _, secondMappingOK := globals.Mapping(secondGlobal)
	firstBootID, firstBootIDOK := linked.Host().BootRoots().ID(firstBoot)
	secondBootID, secondBootIDOK := linked.Host().BootRoots().ID(secondBoot)
	if !firstGlobalOK || !secondGlobalOK || !firstMappingOK || !secondMappingOK || !firstBootIDOK || !secondBootIDOK || firstBootID != secondBootID {
		t.Fatal("globals did not share one owner-issued BootRoot")
	}
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil || result.BodyCount() != 2 {
		t.Fatalf("shared-root bootstrap = %d/%v", status, result)
	}
}

// This distinguishes BodyCall from direct external selection: the outer body
// has no _G access and receives the atom from the callee body in one solver.
func TestBodyCallTransportsEffectThroughPublicAnalyze(t *testing.T) {
	linked := mustLink(t, `
local function effectful()
	_G.emit()
end
effectful()
`, oracleEffectTarget(t))
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil || result.BodyCount() != 2 {
		t.Fatalf("body call = %d/%v", status, result)
	}
	for index := 0; index < result.BodyCount(); index++ {
		body, ok := result.BodyAt(index)
		present, top, effectOK := body.EffectDisposition()
		if !ok || !effectOK || !present || top || body.EffectCount() != 1 {
			t.Fatalf("body %d effect", index)
		}
	}
}

// This exercises the same BodyCall path through a self-recursive callee. The
// emit atom must converge once in the callee and remain deduplicated when it
// reaches the caller through the one recurrent solver site.
func TestRecursiveBodyCallConvergesThroughPublicAnalyze(t *testing.T) {
	linked := mustLink(t, `
local function effectful()
	_G.emit()
	effectful()
end
effectful()
`, oracleEffectTarget(t))
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil || result.BodyCount() != 2 {
		t.Fatalf("recursive body call = %d/%v", status, result)
	}
	for index := 0; index < result.BodyCount(); index++ {
		body, ok := result.BodyAt(index)
		present, top, effectOK := body.EffectDisposition()
		if !ok || !effectOK || !present || top || body.EffectCount() != 1 {
			t.Fatalf("body %d recursive effect", index)
		}
		atom, atomOK := body.EffectAt(0)
		if !atomOK || !atom.Available() {
			t.Fatalf("body %d recursive atom", index)
		}
	}
}

func TestCanonicalBodiesShareOneAssemblyAndDetachedQueries(t *testing.T) {
	direct, err := lower.Lower(lower.Source{Name: "direct.lua", Text: []byte(`return ({ field = 1 }).field`)})
	if err != nil {
		t.Fatal(err)
	}
	effect, err := lower.Lower(lower.Source{Name: "effect.lua", Text: []byte(`_G.emit()`)})
	if err != nil {
		t.Fatal(err)
	}
	literal, err := lower.Lower(lower.Source{Name: "literal.lua", Text: []byte(`return "ok"`)})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: oracleEffectTarget(t), Modules: []linkproject.Module{{Name: "direct", Program: direct}, {Name: "effect", Program: effect}, {Name: "literal", Program: literal}}})
	if err != nil {
		t.Fatal(err)
	}
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil || result.BodyCount() != 3 || !result.ContentID().Available() || result.SourceID() != linked.ContentID() {
		t.Fatalf("mixed = %d/%v", status, result)
	}
	effectRows := 0
	for index := 0; index < result.BodyCount(); index++ {
		body, ok := result.BodyAt(index)
		if !ok {
			t.Fatal("missing body")
		}
		id, idOK := body.ID()
		if !idOK || !id.Available() || body.RootCount() == 0 {
			t.Fatal("body row")
		}
		for rootIndex := 0; rootIndex < body.RootCount(); rootIndex++ {
			root, rootOK := body.RootAt(rootIndex)
			rootID, rootIDOK := root.ID()
			if !rootOK || !rootIDOK || !rootID.Available() || root.Family() == keyspace.FamilyInvalid {
				t.Fatal("root row")
			}
		}
		for valueIndex := 0; valueIndex < body.ValueCount(); valueIndex++ {
			id, _, valueOK := body.ValueAt(valueIndex)
			if !valueOK || !id.Available() {
				t.Fatal("value row")
			}
		}
		present, top, effectOK := body.EffectDisposition()
		if !effectOK || top {
			t.Fatal("effect disposition")
		}
		if present {
			effectRows++
			if body.EffectCount() != 1 {
				t.Fatal("effect count")
			}
		}
	}
	if effectRows != 1 {
		t.Fatalf("effect rows = %d, want 1", effectRows)
	}
}

func TestPublishedResultRetainsNoEngineDomainOrLinkAuthority(t *testing.T) {
	forbidden := []string{"github.com/wippyai/go-lua/analysis/engine", "github.com/wippyai/go-lua/analysis/domain/", "github.com/wippyai/go-lua/program/link"}
	seen := make(map[reflect.Type]bool)
	var inspect func(reflect.Type)
	inspect = func(value reflect.Type) {
		if value == nil || seen[value] {
			return
		}
		seen[value] = true
		for _, prefix := range forbidden {
			if strings.HasPrefix(value.PkgPath(), prefix) {
				t.Fatalf("published result retains %v", value)
			}
		}
		switch value.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			inspect(value.Elem())
		case reflect.Struct:
			for index := 0; index < value.NumField(); index++ {
				inspect(value.Field(index).Type)
			}
		}
	}
	inspect(reflect.TypeOf(Result{}))
	inspect(reflect.TypeOf(Body{}))
	inspect(reflect.TypeOf(Root{}))
}

func mustLink(t testing.TB, text string, contract *target.Contract) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "analysis.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	return linked
}

func oracleEffectTarget(t testing.TB) *target.Contract {
	t.Helper()
	emit := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"emit"}}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{
			{Bindings: []target.BindingSpec{emit}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Occurrences: []target.EffectSpec{{Target: 2}}, Tail: target.RowClosed}},
			{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"record"}}}, Input: target.ValuesSpec{Tail: target.ValuesClosed}, Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed}},
		},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: oracleLiteral("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: oracleLiteral("emit"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: emit}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: oracleLiteral("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: oracleLiteral("_G")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func oracleLiteral(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}
