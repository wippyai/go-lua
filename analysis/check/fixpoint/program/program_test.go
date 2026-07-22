package program

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/projection"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestProgramProductionUsesPreparedBodyAPI(t *testing.T) {
	src, err := os.ReadFile("program.go")
	if err != nil {
		t.Fatalf("ReadFile(program.go): %v", err)
	}
	if strings.Contains(string(src), "body.CheckBound") {
		t.Fatalf("program.go uses the removed direct body checker instead of the relation program")
	}
}

func TestRunChunkRecordsFreezeArchitectureTelemetry(t *testing.T) {
	stmts := parseChunk(t, `
local record = {}
record.value = "ok"
local function read()
    return record.value
end
local value: string = read()
`)
	stats := &Stats{}
	if _, err := RunChunk(stmts, Config{Check: body.Config{Registry: standard.Registry()}, Stats: stats}); err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	for name, phase := range map[string]transformer.FreezePhaseStats{
		"input validation":    stats.Freeze.InputValidation,
		"local syntax":        stats.Freeze.LocalSyntax,
		"SCC closure/linking": stats.Freeze.SCCClosureLinking,
		"region/WTO":          stats.Freeze.RegionWTO,
		"coordinate closure":  stats.Freeze.CoordinateClosure,
		"fiber layout":        stats.Freeze.FiberLayout,
		"observable quotient": stats.Freeze.ObservableQuotient,
		"template binding":    stats.Freeze.TemplateBinding,
	} {
		if phase.Calls == 0 || phase.Elapsed <= 0 {
			t.Errorf("%s telemetry = %+v, want completed timed phase", name, phase)
		}
	}
	if stats.Freeze.PathDependencyPlanning.Calls == 0 || stats.Freeze.PathDependencyPlanning.Elapsed <= 0 {
		t.Errorf("path dependency telemetry = %+v, want completed timed phase", stats.Freeze.PathDependencyPlanning)
	}
	t.Logf("freeze telemetry raw: input=%+v local=%+v scc=%+v region=%+v coordinate=%+v path=%+v fiber=%+v quotient=%+v template=%+v", stats.Freeze.InputValidation, stats.Freeze.LocalSyntax, stats.Freeze.SCCClosureLinking, stats.Freeze.RegionWTO, stats.Freeze.CoordinateClosure, stats.Freeze.PathDependencyPlanning, stats.Freeze.FiberLayout, stats.Freeze.ObservableQuotient, stats.Freeze.TemplateBinding)
}

func TestRunChunkNestedFunctionReturnSlotEvaluatesRuntimeKindLengthComparison(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function has_checkpoint_bindings(bindings: any): boolean
    if type(bindings) ~= "table" then
        return false
    end
    if type(bindings.checkpoint) == "table" then
        bindings = bindings.checkpoint
    end
    return #bindings > 0
end
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{
		Registry: reg,
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
		},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	children := root.FunctionResults()
	if len(children) != 1 {
		t.Fatalf("nested function results = %d, want 1", len(children))
	}
	exit, ok := children[0].ExitState()
	if !ok {
		t.Fatal("nested function exit state missing")
	}
	got, ok := typevalue.TypeOf(reg, exit.ReadReturnSlot(reg, 0))
	if !ok || !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("nested return slot type = %v/%v, want boolean", got, ok)
	}
}

func TestRunChunkAnnotatedRequireSourceUsesManifestExport(t *testing.T) {
	reg := standard.Registry()
	exportType := typetable.NewRecord().Field("run", typ.Func().Build()).Build()
	m := manifest.New("pkg")
	m.SetExport(exportType)
	m.DefineFunctionSignature("pkg.run", signature.Function{Type: typ.Func().Build()})
	stmts := parseChunk(t, `
local pkg: {run: () -> ()}? = require("pkg")
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{
		Registry: reg,
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
			Manifests:     []*manifest.Manifest{m},
		},
		ModuleExports: importlookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	var point cfg.Point
	var sourceFound bool
	var source factflow.ValueSource
	for _, candidate := range root.Graph().RPO() {
		fact, ok := root.LocalAssignment(candidate)
		if !ok || fact.Name != "pkg" {
			continue
		}
		lowered, ok := root.LoweredLocalAssignment(candidate)
		if !ok {
			t.Fatal("missing lowered local assignment for pkg")
		}
		point = candidate
		source = lowered.Source()
		sourceFound = true
		break
	}
	if !sourceFound {
		t.Fatal("local assignment pkg not found")
	}
	value, ok := root.SourceValueAtBoundary(point, source)
	if !ok {
		t.Fatal("missing annotated require source value")
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, exportType) {
		t.Fatalf("annotated require source type = %v/%v, want manifest export %v", got, ok, exportType)
	}
	semanticFact, ok := root.LocalAssignment(point)
	if !ok {
		t.Fatal("missing semantic local assignment for pkg")
	}
	sourceType, ok := readmodel.New(root).SourceType(point, semanticFact.Source)
	if !ok || !typ.TypeEquals(sourceType, exportType) {
		t.Fatalf("program readmodel source type = %v/%v, want manifest export %v", sourceType, ok, exportType)
	}
	exprValue, ok := root.ExpressionValueAtBoundary(point, semanticFact.Expr)
	if !ok {
		t.Fatal("missing program expression value for annotated require")
	}
	exprType, ok := typevalue.TypeOf(reg, exprValue)
	if !ok || !typ.TypeEquals(exprType, exportType) {
		t.Fatalf("program expression type = %v/%v, want manifest export %v", exprType, ok, exportType)
	}
}

type entriesOnlySummaryReader struct {
	entries []summary.EntrySummary
}

func (r entriesOnlySummaryReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	for _, entry := range r.entries {
		if entry.Key == key {
			return entry.Summary.Clone(), true
		}
	}
	return summary.Summary{}, false
}

func (r entriesOnlySummaryReader) EntriesOwnedNormalized() []summary.EntrySummary {
	return r.entries
}

func TestSetmetatableConstructorSummaryKeepsLiteralFields(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local table = table
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
	return setmetatable({
		node_order = table.create(4, 0),
	}, flow_graph_mt)
end
`)
	result, err := RunChunk(stmts, Config{Check: body.Config{
		Registry: reg,
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
		},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	children := root.FunctionResults()
	if len(children) != 1 {
		t.Fatalf("function results = %d, want constructor only", len(children))
	}
	constructor := children[0]
	sum := summaryprojection.FromResult(constructor)
	if len(sum.Returns) != 1 {
		t.Fatalf("summary returns = %d, want one", len(sum.Returns))
	}
	gotType, ok := typevalue.TypeOf(reg, sum.Returns[0])
	if !ok {
		t.Fatalf("constructor return has no type: %#v", sum.Returns[0])
	}
	rec, ok := gotType.(*typ.Record)
	if !ok {
		t.Fatalf("constructor return type = %v, want record", gotType)
	}
	field := rec.GetField("node_order")
	if field == nil {
		t.Fatalf("constructor return type = %v, want node_order field", gotType)
	}
	if typ.IsUnknown(field.Type) || typ.IsAny(field.Type) {
		t.Fatalf("node_order type = %v, want concrete table field", field.Type)
	}
}

type countingSummaryReader struct {
	snapshot    summary.Snapshot
	publicReads int
	ownedReads  int
}

func (r *countingSummaryReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	r.publicReads++
	return r.snapshot.Read(key)
}

func (r *countingSummaryReader) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	r.ownedReads++
	return r.snapshot.ReadOwnedNormalized(key)
}

func TestFunctionTypeFromSummaryUsesOwnedNormalizedRead(t *testing.T) {
	reg := standard.Registry()
	key := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(42)))
	ret := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	reader := &countingSummaryReader{
		snapshot: summary.NewSnapshotOwnedNormalized(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.NormalizeOwned(reg, summary.Summary{
				Returns: []product.Value{ret},
			}),
		}),
	}
	declared := typ.Func().Param("input", typ.String).Build()
	got, ok := functionTypeFromSummary(reg, reader, key, declared)
	if !ok || got == nil {
		t.Fatalf("functionTypeFromSummary = %#v/%v, want function", got, ok)
	}
	if reader.publicReads != 0 || reader.ownedReads == 0 {
		t.Fatalf("summary reads = public:%d owned:%d, want owned-only", reader.publicReads, reader.ownedReads)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want string", got.Returns)
	}
}

func TestFunctionTypeFromSummaryKeepsOwnedGenericReturnOpen(t *testing.T) {
	reg := standard.Registry()
	key := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(43)))
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	ret := typevalue.WithWitness(reg, typevalue.FromType(reg, errorCase), errorCase)
	reader := &countingSummaryReader{
		snapshot: summary.NewSnapshotOwnedNormalized(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.NormalizeOwned(reg, summary.Summary{
				Returns: []product.Value{ret},
			}),
		}),
	}
	param := typ.NewTypeParam("T", nil)
	resultType := typ.NewGeneric("Result", []*typ.TypeParam{param}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", param).
			Build(),
		errorCase,
	))
	declared := typ.Func().
		TypeParamRef(param).
		Param("message", typ.String).
		Returns(typ.Instantiate(resultType, param)).
		Build()

	got, ok := functionTypeFromSummary(reg, reader, key, declared)
	if !ok || got == nil {
		t.Fatalf("functionTypeFromSummary = %#v/%v, want function", got, ok)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], declared.Returns[0]) {
		t.Fatalf("returns = %v, want owned generic return to stay %v", got.Returns, declared.Returns[0])
	}
}

func TestRunBoundChunkUsesSuppliedBindIdentityForLocalCallee(t *testing.T) {
	reg := standard.Registry()
	want := product.Top()
	stmts := parseChunk(t, `
local x = 0
local f = function()
	return x + 1
end
return f()
`)
	local := stmts[1].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	fTarget := mustBoundLocalAt(t, bindings, local, 0)
	origin := onlyFunctionOrigin(t, bindings)
	if !origin.HasTargetSymbol || origin.TargetSymbol != fTarget {
		t.Fatalf("function origin target = %d/%v, want local symbol %d", origin.TargetSymbol, origin.HasTargetSymbol, fTarget)
	}

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{
			Registry:        reg,
			ExpressionValue: fixedExpressionValue(want),
		},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	targetKey, ok := result.TargetKey(fTarget)
	if !ok {
		t.Fatalf("TargetKey(%d) missing", fTarget)
	}
	if wantKey := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol)); targetKey != wantKey {
		t.Fatalf("TargetKey(%d) = %#v, want %#v", fTarget, targetKey, wantKey)
	}
	assertSummaryReturn(t, reg, result.Snapshot(), result.RootKey(), want)
	assertSummaryReturn(t, reg, result.Snapshot(), targetKey, want)
}

func TestRunChunkReexportsChainedWrapperNormalReturnParam(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local requireValue = function(x: string?)
	assert(x)
end
local requireAgain = function(x: string?)
	requireValue(x)
end
`)
	firstLocal := stmts[0].(*ast.LocalAssignStmt)
	secondLocal := stmts[1].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	requireValue := mustBoundLocalAt(t, bindings, firstLocal, 0)
	requireAgain := mustBoundLocalAt(t, bindings, secondLocal, 0)

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	valueKey, ok := result.TargetKey(requireValue)
	if !ok {
		t.Fatalf("TargetKey(requireValue) missing")
	}
	againKey, ok := result.TargetKey(requireAgain)
	if !ok {
		t.Fatalf("TargetKey(requireAgain) missing")
	}
	assertSummaryNormalReturnParam(t, reg, result.Snapshot(), valueKey, 0, presence.Present(), runtimekind.Singleton(runtimekind.String))
	assertSummaryNormalReturnParam(t, reg, result.Snapshot(), againKey, 0, presence.Present(), runtimekind.Singleton(runtimekind.String))
}

func TestRunChunkDoesNotReexportNormalReturnParamAfterCallerReassignment(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local requireValue = function(x: string?)
	assert(x)
end
local requireReassigned = function(x: string?)
	x = "replacement"
	requireValue(x)
end
`)
	secondLocal := stmts[1].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	requireReassigned := mustBoundLocalAt(t, bindings, secondLocal, 0)

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	key, ok := result.TargetKey(requireReassigned)
	if !ok {
		t.Fatalf("TargetKey(requireReassigned) missing")
	}
	got, ok := result.Snapshot().Read(key)
	if !ok || len(got.NormalReturnParams) != 1 || !product.Equal(reg, got.NormalReturnParams[0], product.Top()) {
		t.Fatalf("reassigned wrapper normal-return params = %#v, want one unconstrained parameter", got.NormalReturnParams)
	}
}

func TestRunChunkLexicalSummaryDoesNotDependOnIncomingCallRoute(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local called = function(x: string?)
	assert(x)
end
local uncalled = function(x: string?)
	assert(x)
end
called("value")
`)
	firstLocal := stmts[0].(*ast.LocalAssignStmt)
	secondLocal := stmts[1].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{})
	called := mustBoundLocalAt(t, bindings, firstLocal, 0)
	uncalled := mustBoundLocalAt(t, bindings, secondLocal, 0)

	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	calledKey, calledOK := result.TargetKey(called)
	uncalledKey, uncalledOK := result.TargetKey(uncalled)
	calledSummary, calledPresent := result.Snapshot().Read(calledKey)
	uncalledSummary, uncalledPresent := result.Snapshot().Read(uncalledKey)
	if !calledOK || !uncalledOK || !calledPresent || !uncalledPresent {
		t.Fatal("called/uncalled lexical summaries are incomplete")
	}
	if !summary.Equal(reg, calledSummary, uncalledSummary) {
		t.Fatalf("lexically identical summaries differ by incoming call route:\ncalled:   %#v\nuncalled: %#v", calledSummary, uncalledSummary)
	}
}

func TestRunChunkAppliesDirectErrorReturnPresenceRelation(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type DB = {
	release: (self: DB) -> (),
}

local function raw_get(dsn: string): (DB?, string?)
	if dsn == "" then
		return nil, "missing dsn"
	end
	return {}, nil
end

local function run(dsn: string): ()
	local db, err = raw_get(dsn)
	if err then
		return
	end
	db:release()
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	child, releasePoint, dbPath := findNestedReceiverCall(t, root, "release")
	rawGetPoint := requireCallByCalleeName(t, child, "raw_get")
	outcome, ok := child.CallOutcomeAt(rawGetPoint)
	if !ok {
		t.Fatalf("raw_get call outcome missing at %d", rawGetPoint)
	}
	if len(outcome.ReturnPresenceRelations) == 0 {
		t.Fatalf("raw_get return presence relations missing: %#v", outcome)
	}
	rawOrigin := functionOriginByName(t, bindings, "raw_get")
	var rawResult *body.Result
	for _, functionResult := range root.FunctionResults() {
		if functionResult.Function() == rawOrigin.Func {
			rawResult = functionResult
			break
		}
	}
	if rawResult == nil {
		t.Fatal("raw_get function result missing")
	}
	rawExit, ok := rawResult.ExitState()
	if !ok {
		t.Fatal("raw_get exit state missing")
	}
	rawReturnValue := rawResult.KeySpace().FromPath(path.Path{Root: "ret[0]"})
	rawReturnError := rawResult.KeySpace().FromPath(path.Path{Root: "ret[1]"})
	if !rawExit.HasPathPresenceImplication(pathevidence.NewPathPresenceImplication(
		rawReturnError, presence.Absent(), rawReturnValue, presence.Present(),
	)) {
		t.Fatal("raw_get exit lost ret[1]-absent => ret[0]-present correlation")
	}
	site, ok := child.CallSiteView(rawGetPoint)
	if !ok {
		t.Fatalf("raw_get call site missing at %d", rawGetPoint)
	}
	var dbSymbol, errSymbol symbol.ID
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		switch target.ResultIndex() {
		case 0:
			dbSymbol = target.TargetSymbol()
		case 1:
			errSymbol = target.TargetSymbol()
		}
		return true
	})
	hasDirectCorrelation := func(value state.State) bool {
		for _, implication := range value.PathPresenceImplicationsSnapshot(child.KeySpace()).Implications {
			if implication.Trigger.Sym == errSymbol && implication.Target.Sym == dbSymbol &&
				presence.Equal(implication.TriggerPresence, presence.Absent()) &&
				presence.Equal(implication.TargetPresence, presence.Present()) {
				return true
			}
		}
		return false
	}
	callBoundary, ok := child.StateAtBoundary(rawGetPoint)
	if !ok {
		t.Fatalf("raw_get call boundary missing at %d", rawGetPoint)
	}
	if !hasDirectCorrelation(callBoundary) {
		t.Fatal("raw_get call boundary lost err-absent => db-present correlation")
	}
	var branchPoint cfg.Point
	for _, point := range child.Graph().RPO() {
		if child.Graph().IsBranch(point) {
			branchPoint = point
			break
		}
	}
	if branchPoint == 0 {
		t.Fatal("raw_get caller branch missing")
	}
	branchInput, ok := child.StateAt(branchPoint)
	if !ok {
		t.Fatalf("raw_get branch input missing at %d", branchPoint)
	}
	if !hasDirectCorrelation(branchInput) {
		t.Fatal("raw_get branch input lost err-absent => db-present correlation")
	}
	value, ok := child.PathValueAtBoundary(releasePoint, dbPath)
	if !ok {
		t.Fatalf("db boundary value missing at release point %d path %s", releasePoint, dbPath.String())
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		t.Fatalf("db presence at release = %s (%#v), want present", got, value)
	}
}

func TestRunChunkPairsKeyUsesCallerMapType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function sorted_keys(t)
    local keys: string[] = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    return keys
end

local function grouped(entries)
    local suites: {[string]: any[]} = {}
    return suites
end

local suites = grouped({})
local suite_names = sorted_keys(suites)
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	child := requireLexicalResultByName(t, root, "sorted_keys")
	sawKey := false
	for _, point := range child.Graph().RPO() {
		fact, ok := child.GenericFor(point)
		if !ok || !fact.HasSymbols || fact.VariableIndex != 0 || len(fact.Names) == 0 || fact.Names[0] != "k" {
			continue
		}
		if fact.VariableIndex >= len(fact.Symbols) {
			t.Fatalf("generic-for symbols = %#v missing index %d", fact.Symbols, fact.VariableIndex)
		}
		value, ok := child.SymbolValueAtBoundary(point, fact.Symbols[fact.VariableIndex])
		if !ok {
			t.Fatal("generic-for key value missing from lexical result")
		}
		got, ok := typevalue.TypeOf(reg, value)
		sawKey = true
		if !ok || !typ.TypeEquals(got, typ.String) {
			t.Fatalf("generic-for key type = %v/%v, want string", got, ok)
		}
	}
	if !sawKey {
		root := result.RootResult()
		callPoint := requireCallByCalleeName(t, root, "sorted_keys")
		site, siteOK := root.CallSiteView(callPoint)
		var argValue product.Value
		var argType typ.Type
		argOK, typeOK := false, false
		if siteOK {
			if source, ok := site.ArgumentSourceAt(0); ok {
				argValue, argOK = root.SourceValueAtBoundary(callPoint, source)
				if argOK {
					argType, typeOK = typevalue.TypeOf(reg, argValue)
				}
			}
		}
		t.Fatalf("sorted_keys lexical result missing generic-for key (site=%v, argOK=%v, argType=%v/%v)", siteOK, argOK, argType, typeOK)
	}
}

func TestCollectKeysRejectsStaticPathForReassignedGlobalFunction(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
function handler()
    return "stale"
end

function replacement()
    return 1
end

handler = replacement
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	keys := collectKeys(bindings, summary.DefaultSummaryKey(ref.Root()), reg, nil, importlookup.Source{}, stmts)
	handler, ok := bindings.GlobalSymbol("handler")
	if !ok {
		t.Fatal("handler global symbol missing")
	}
	if len(bindings.WriteIdents(handler)) <= 1 {
		t.Fatalf("handler writes = %d, want function declaration plus reassignment", len(bindings.WriteIdents(handler)))
	}
	handlerPath := path.NewPath(handler, "handler")
	handlerKey, ok := factflow.CalleePathKeyFromPath(handlerPath)
	if !ok {
		t.Fatal("handler callee path key missing")
	}
	if _, ok := keys.pathKeys[handlerKey]; ok {
		t.Fatalf("handler static path key exists for reassigned global; calls must use current value identity")
	}
}

func TestRunChunkScalarLiteralSelectsReturnShape(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function select_shape(kind)
	if kind == "auto" then
		return { mode = "AUTO" }
	end
	return nil
end

local config = select_shape("auto")
local mode: string = config.mode
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	child := requireLexicalResultByName(t, root, "select_shape")
	entry, ok := child.EntryState()
	if !ok {
		t.Fatal("select_shape lexical result missing entry state")
	}
	fn := child.Function()
	slots := bindings.ParamSlots(fn)
	if len(slots) == 0 || slots[0].Symbol == 0 {
		t.Fatalf("select_shape param slots = %#v", slots)
	}
	entryValue := entry.ReadValue(reg, statekey.SymbolValue(slots[0].Symbol))
	entryType, entryTypeOK := typevalue.TypeOf(reg, entryValue)
	if !entryTypeOK || !typ.TypeEquals(entryType, typ.LiteralString("auto")) {
		t.Fatalf("select_shape entry param type = %v/%v, want \"auto\"", entryType, entryTypeOK)
	}
	var literalBranch cfg.Point
	for _, point := range child.Graph().RPO() {
		fact, ok := child.BranchCondition(point)
		if !ok || fact.Check.Kind != branchcond.CheckLiteralEqual {
			continue
		}
		lit, hasLit := fact.Check.LiteralValue()
		if hasLit && typ.TypeEquals(lit, typ.LiteralString("auto")) {
			literalBranch = point
			break
		}
	}
	if literalBranch == 0 {
		t.Fatal("select_shape branch for kind == \"auto\" missing")
	}
	sawFalseEdge := false
	for _, succ := range child.Graph().Successors(literalBranch) {
		cond, ok := child.Graph().EdgeCond(literalBranch, succ)
		if !ok || cond {
			continue
		}
		sawFalseEdge = true
		falseState, ok := child.StateAt(succ)
		if !ok || !state.IsBottom(reg, falseState) {
			t.Fatalf("select_shape false edge state = %#v/%v, want bottom", falseState, ok)
		}
	}
	if !sawFalseEdge {
		t.Fatal("select_shape false edge missing")
	}
	exit, ok := child.ExitState()
	if !ok {
		t.Fatal("select_shape lexical result missing exit state")
	}
	ret := exit.ReadReturnSlot(reg, 0)
	gotType, ok := typevalue.TypeOf(reg, ret)
	if !ok || !typ.TypeEquals(gotType, typetable.NewRecord().Field("mode", typ.LiteralString("AUTO")).Build()) {
		t.Fatalf("select_shape return type = %v/%v, want {mode=\"AUTO\"}", gotType, ok)
	}
	diags := diagnostics.Produce(root)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want lexical return shape to prove config.mode", diags)
	}
}

func TestRunChunkScalarLiteralClosesCompoundOrFallback(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function select_shape(kind)
	if not kind or kind == "auto" or kind == "any" or kind == "" then
		return { mode = "AUTO" }
	elseif kind == "none" then
		return { mode = "NONE" }
	end
	return nil
end

local config = select_shape("auto")
local mode: string = config.mode
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	child := requireLexicalResultByName(t, root, "select_shape")
	exit, ok := child.ExitState()
	if !ok {
		t.Fatal("compound select_shape lexical result missing exit state")
	}
	ret := exit.ReadReturnSlot(reg, 0)
	gotType, ok := typevalue.TypeOf(reg, ret)
	wantType := typetable.NewRecord().Field("mode", typ.LiteralString("AUTO")).Build()
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("compound select_shape return type = %v/%v, want %v", gotType, ok, wantType)
	}
	diags := diagnostics.Produce(root)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want compound lexical return shape to prove config.mode", diags)
	}
}

func TestRunChunkReturnedKeyListMembershipProvesIndexedRead(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function consume(tests: any[]): ()
end

local function sorted_keys(t)
	local keys: string[] = {}
	for k in pairs(t) do
		table.insert(keys, k)
	end
	table.sort(keys)
	return keys
end

local suites: {[string]: any[]} = {}
local suite_names = sorted_keys(suites)

for _, name in ipairs(suite_names) do
	consume(suites[name])
end
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	suitesPath := findRootLocalPath(t, root, "suites")
	suiteNamesPath := findRootLocalPath(t, root, "suite_names")
	callPoint := requireCallByCalleeName(t, root, "consume")
	var nameSymbol symbol.ID
	for _, point := range root.Graph().RPO() {
		fact, ok := root.GenericFor(point)
		if !ok || !fact.HasSymbols || fact.VariableIndex != 1 || len(fact.Names) <= fact.VariableIndex || fact.Names[fact.VariableIndex] != "name" {
			continue
		}
		nameSymbol = fact.Symbols[fact.VariableIndex]
		break
	}
	if nameSymbol == 0 {
		t.Fatalf("generic-for value variable name not found")
	}
	st, ok := root.StateAt(callPoint)
	if !ok {
		t.Fatalf("state at consume point %d missing", callPoint)
	}
	nameKey, nameKeyOK := root.StateKeyAtBoundary(callPoint, path.NewPath(nameSymbol, "name"))
	suitesKey, suitesKeyOK := root.StateKeyAtBoundary(callPoint, suitesPath)
	suiteNamesKey, suiteNamesKeyOK := root.StateKeyAtBoundary(callPoint, suiteNamesPath)
	var suiteNamesContainer keyspace.Key
	var suiteNamesContainerOK bool
	if suiteNamesKeyOK {
		suiteNamesContainer, suiteNamesContainerOK = root.KeySpace().InternStateKey(suiteNamesKey)
	}
	var suiteNamesDynamicSites []dynamicindex.Site
	var suiteNamesEquivalent []keyspace.Key
	if suiteNamesContainerOK {
		suiteNamesEquivalent = st.EquivalentKeyspaceKeys(root.KeySpace(), suiteNamesContainer)
	}
	for dynamicKey := range st.DynamicIndexFactsSnapshot().Facts {
		if dynamicKey.Table == suiteNamesContainer {
			suiteNamesDynamicSites = append(suiteNamesDynamicSites, dynamicKey.Site)
		}
	}
	var suiteNamesValueKeyTables []pathaddr.StateKey
	for _, site := range suiteNamesDynamicSites {
		suiteNamesValueKeyTables = append(suiteNamesValueKeyTables, st.DynamicIndexValueKeyMembershipTables(suiteNamesContainer, site)...)
	}
	if !nameKeyOK || !suitesKeyOK || !suiteNamesKeyOK || !suiteNamesContainerOK || !st.HasPathKeyMembership(nameKey, suitesKey) {
		t.Fatalf("returned key-list proof missing before consume: nameKey=%q/%v suitesKey=%q/%v suiteNamesKey=%q/%v container=%#v/%v equivalents=%#v dynamicSites=%#v valueKeyTables=%#v memberships=%#v dynamicFacts=%#v",
			nameKey, nameKeyOK, suitesKey, suitesKeyOK, suiteNamesKey, suiteNamesKeyOK, suiteNamesContainer, suiteNamesContainerOK,
			suiteNamesEquivalent, suiteNamesDynamicSites, suiteNamesValueKeyTables, st.KeyMembershipsSnapshot(), st.DynamicIndexFactsSnapshot())
	}
	site, ok := root.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("consume call site missing at %d", callPoint)
	}
	source, ok := site.ArgumentSourceAt(0)
	if !ok {
		t.Fatalf("consume argument source missing: %#v", site)
	}
	beforeValue, beforeOK := root.SourceValueBeforeBoundary(callPoint, source)
	value, ok := root.SourceValueAtBoundary(callPoint, source)
	if !ok {
		t.Fatalf("consume argument value missing at %d", callPoint)
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		beforePresence := presence.Bottom()
		if beforeOK {
			beforePresence = product.PresenceOf(beforeValue)
		}
		t.Fatalf("consume argument presence = %s, want present after key-list proof; beforeOK=%v beforePresence=%s source=%#v",
			got, beforeOK, beforePresence, source)
	}
}

func TestRunChunkReturnedKeyListMembershipSurvivesReturnedMap(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function consume(tests: any[]): ()
end

local function group_by_suite(entries)
	local suites: {[string]: any[]} = {}
	for _, entry in ipairs(entries) do
		local suite = entry.suite
		if suite then
			suites[suite] = suites[suite] or {}
			table.insert(suites[suite], entry)
		end
	end
	return suites
end

local function sorted_keys(t)
	local keys: string[] = {}
	for k in pairs(t) do
		table.insert(keys, k)
	end
	table.sort(keys)
	return keys
end

local suites = group_by_suite({})
local suite_names = sorted_keys(suites)

for _, name in ipairs(suite_names) do
	consume(suites[name])
end
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	sortedPoint := requireCallByCalleeName(t, root, "sorted_keys")
	outcome, ok := root.CallOutcomeAt(sortedPoint)
	if !ok {
		t.Fatalf("sorted_keys call outcome missing at %d", sortedPoint)
	}
	if len(outcome.NormalReturnFacts.DynamicValueKeys) != 1 ||
		!outcome.NormalReturnFacts.DynamicValueKeys[0].Container.Equal(path.Path{Root: "ret[0]"}) ||
		!outcome.NormalReturnFacts.DynamicValueKeys[0].Table.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("sorted_keys dynamic value key facts = %#v, want ret[0] values proven as keys of $0", outcome.NormalReturnFacts.DynamicValueKeys)
	}
}

func TestRunChunkReturnedKeyListMembershipRejectsConflictingWriter(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function sorted_keys(t)
	local keys: string[] = {}
	for k in pairs(t) do
		table.insert(keys, k)
	end
	table.insert(keys, "not-from-t")
	return keys
end

local suites: {[string]: any[]} = {}
local suite_names = sorted_keys(suites)
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{
		Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	sortedPoint := requireCallByCalleeName(t, root, "sorted_keys")
	outcome, ok := root.CallOutcomeAt(sortedPoint)
	if !ok {
		t.Fatalf("sorted_keys call outcome missing at %d", sortedPoint)
	}
	if got := outcome.NormalReturnFacts.DynamicValueKeys; len(got) != 0 {
		t.Fatalf("conflicting writer produced closed key-membership theorem: %#v", got)
	}
}

func TestRunChunkCapturedStaticMemberWriteSurvivesReturnedTable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function make()
    local obj = { x = 1 }
    local function init()
        obj.get_x = function(self): number
            return self.x
        end
    end
    init()
    return obj
end

local built = make()
local n: number = built:get_x()
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	makePoint := requireCallByCalleeName(t, root, "make")
	outcome, ok := root.CallOutcomeAt(makePoint)
	if !ok {
		t.Fatalf("make call outcome missing at %d", makePoint)
	}
	target := path.Path{Root: "ret[0]"}.Field("get_x")
	for _, fact := range outcome.NormalReturnFacts.PathStaticMembers {
		if fact.Path.Equal(target) {
			if product.Equal(reg, fact.Value, product.Bottom(reg)) {
				t.Fatalf("make path static member %s has bottom value", target)
			}
			memberType, hasMemberType := typevalue.TypeOf(reg, fact.Value)
			if !hasMemberType {
				t.Fatalf("make path static member %s value has no type witness: %#v", target, fact.Value)
			}
			memberFn, ok := memberType.(*typ.Function)
			if !ok || memberFn == nil || len(memberFn.Returns) != 1 || !typ.TypeEquals(memberFn.Returns[0], typ.Number) {
				t.Fatalf("make path static member %s type = %v, want function returning number", target, memberType)
			}
			nStmt := mustFindLocalAssign(t, stmts, "n")
			nPoint := requireLocalAssignmentPoint(t, root, nStmt, 0)
			got, ok := root.ExpressionValueAtBoundary(nPoint, nStmt.Exprs[0])
			if !ok {
				t.Fatalf("n expression boundary value missing at %d", nPoint)
			}
			gotType, gotTypeOK := typevalue.TypeOf(reg, got)
			if !gotTypeOK || !typ.TypeEquals(gotType, typ.Number) {
				t.Fatalf("built:get_x() type = %v/%v, want number (value %#v)", gotType, gotTypeOK, got)
			}
			return
		}
	}
	t.Fatalf("make path static members = %#v, want %s", outcome.NormalReturnFacts.PathStaticMembers, target)
}

func TestRunChunkReverseMapReadCarriesPrimaryMapMembership(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ChannelInfo = {
	chan: any,
	handler: (any, any, boolean, string) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any, any, boolean, string) -> any): ()
	local channel_id = tostring(chan)
	registered_channels[channel_id] = { chan = chan, handler = handler }
	channel_to_id[chan] = channel_id
end

local function dispatch(result: { channel: any, value: any, ok: boolean }, state: any): any
	register_channel(result.channel, function(inner_state, value, ok, id) return value end)
	local channel_id = channel_to_id[result.channel]
	if channel_id then
		local channel_info = registered_channels[channel_id]
		return channel_info.handler(state, result.value, result.ok, channel_id)
	end
	return nil
end

return dispatch
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	registeredPath := findRootLocalPath(t, root, "registered_channels")
	var dispatch *body.Result
	for _, child := range root.FunctionResults() {
		if child == nil || child.Graph() == nil {
			continue
		}
		for _, point := range child.Graph().RPO() {
			site, ok := child.CallSiteView(point)
			if ok && site.CalleeSymbol() != 0 && child.SymbolName(site.CalleeSymbol()) == "register_channel" {
				dispatch = child
				break
			}
		}
		if dispatch != nil {
			break
		}
	}
	if dispatch == nil {
		t.Fatal("dispatch lexical result with register_channel call not found")
	}
	var readPoint cfg.Point
	var channelID symbol.ID
	for _, point := range dispatch.Graph().RPO() {
		fact, ok := dispatch.LocalAssignment(point)
		if ok && fact.Name == "channel_id" {
			readPoint = point
			channelID = fact.Symbol
			break
		}
	}
	if readPoint == 0 {
		t.Fatalf("channel_id read point not found in dispatch")
	}
	readFact, ok := dispatch.LocalAssignment(readPoint)
	if !ok {
		t.Fatalf("channel_id local assignment missing at %d", readPoint)
	}
	readValue, ok := dispatch.LocalAssignmentSourceValueAtBoundary(readPoint, readFact.Source)
	if !ok || !dispatch.ValueProofAdmissible(readValue, typ.MaterializeOptional(typ.String)) {
		readType, _ := dispatch.ValueTypeWithPresence(readValue)
		t.Fatalf("reverse-map read value = %v/%v type=%v, want typed optional string proof before the nil guard", readValue, ok, readType)
	}
	var handlerPoint cfg.Point
	for _, point := range dispatch.Graph().RPO() {
		site, ok := dispatch.CallSiteView(point)
		if ok && site.CalleeMemberAccess() && site.ArgumentSourceCount() == 4 {
			handlerPoint = point
			break
		}
	}
	if handlerPoint == 0 {
		t.Fatal("handler call point not found")
	}
	rootValue, ok := dispatch.PathValueBeforeBoundary(handlerPoint, path.Path{Symbol: channelID})
	if !ok || !dispatch.ValueProofAdmissible(rootValue, typ.String) {
		rootType, _ := dispatch.ValueTypeWithPresence(rootValue)
		t.Fatalf("reverse-map root before handler = %v/%v type=%v, want admissible string proof", rootValue, ok, rootType)
	}
	var channelInfoPoint cfg.Point
	for _, point := range dispatch.Graph().RPO() {
		fact, ok := dispatch.LocalAssignment(point)
		if ok && fact.Name == "channel_info" {
			channelInfoPoint = point
			break
		}
	}
	if channelInfoPoint == 0 {
		t.Fatalf("channel_info read point not found in dispatch")
	}
	st, ok := dispatch.StateAt(channelInfoPoint)
	if !ok {
		t.Fatalf("state at channel_info read point %d missing", channelInfoPoint)
	}
	if !stateHasPathKeyMembership(dispatch.KeySpace(), st, channelID, registeredPath.Symbol) {
		t.Fatalf("key memberships = %#v, want channel_id proven as key of registered_channels", st.KeyMembershipsSnapshot())
	}
}

func TestRunChunkClosedReverseMapWriterInvariantSeedsDispatchEntry(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ChannelInfo = {
	chan: any,
	handler: (any, any, any, any) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
	local channel_id = tostring(chan)
	registered_channels[channel_id] = { chan = chan, handler = handler }
	channel_to_id[chan] = channel_id
	return true
end

local function unregister_channel(chan: any): boolean
	local channel_id = tostring(chan)
	if registered_channels[channel_id] then
		registered_channels[channel_id] = nil
		channel_to_id[chan] = nil
		return true
	end
	return false
end

local function dispatch(result: { channel: any?, value: any?, ok: boolean }, state: any): any
	local channel_id = channel_to_id[result.channel]
	if channel_id then
		local channel_info = registered_channels[channel_id]
		local reply = channel_info.handler(state, result.value, result.ok, channel_id)
		if not result.ok then
			registered_channels[channel_id] = nil
			channel_to_id[result.channel] = nil
		end
		return reply
	end
	return nil
end

return {
	register_channel = register_channel,
	unregister_channel = unregister_channel,
	dispatch = dispatch,
}
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	registeredPath := findRootLocalPath(t, root, "registered_channels")
	dispatch, channelInfoPoint, _ := findNestedLocalByName(t, root, "channel_info")
	var channelID symbol.ID
	for _, point := range dispatch.Graph().RPO() {
		fact, ok := dispatch.LocalAssignment(point)
		if ok && fact.Name == "channel_id" {
			channelID = fact.Symbol
			break
		}
	}
	if channelID == 0 {
		t.Fatalf("channel_id local not found")
	}
	st, ok := dispatch.StateAt(channelInfoPoint)
	if !ok {
		t.Fatalf("state at channel_info read point %d missing", channelInfoPoint)
	}
	if !stateHasPathKeyMembership(dispatch.KeySpace(), st, channelID, registeredPath.Symbol) {
		t.Fatalf("key memberships = %#v, want closed writer invariant to prove channel_id is key of registered_channels", st.KeyMembershipsSnapshot())
	}
}

func TestRunChunkPrimaryDeleteClearsReverseMapMembershipBeforeStaleRead(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ChannelInfo = {
	handler: (any) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any) -> any): ()
	local channel_id = tostring(chan)
	registered_channels[channel_id] = { handler = handler }
	channel_to_id[chan] = channel_id
end

local function dispatch(chan: any): any
	register_channel(chan, function(value) return value end)
	local channel_id = channel_to_id[chan]
	if channel_id then
		registered_channels[channel_id] = nil
		local stale_id = channel_to_id[chan]
		if stale_id then
			local channel_info = registered_channels[stale_id]
			return channel_info.handler(chan)
		end
	end
	return nil
end

return dispatch
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	registeredPath := findRootLocalPath(t, root, "registered_channels")
	dispatch, channelInfoPoint, _ := findNestedLocalByName(t, root, "channel_info")
	var staleID symbol.ID
	for _, point := range dispatch.Graph().RPO() {
		fact, ok := dispatch.LocalAssignment(point)
		if ok && fact.Name == "stale_id" {
			staleID = fact.Symbol
			break
		}
	}
	if staleID == 0 {
		t.Fatalf("stale_id local not found")
	}
	st, ok := dispatch.StateAt(channelInfoPoint)
	if !ok {
		t.Fatalf("state at channel_info read point %d missing", channelInfoPoint)
	}
	if stateHasPathKeyMembership(dispatch.KeySpace(), st, staleID, registeredPath.Symbol) {
		t.Fatalf("key memberships = %#v, want primary delete to clear stale reverse-map proof", st.KeyMembershipsSnapshot())
	}
}

func TestRunChunkCapturedDynamicIndexOverwriteClearsValueKeyProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ChannelInfo = {
	chan: any,
	handler: (any, any, boolean, string) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any, any, boolean, string) -> any): ()
	local channel_id = tostring(chan)
	registered_channels[channel_id] = { chan = chan, handler = handler }
	channel_to_id[chan] = channel_id
end

local function register_unpaired(chan: any, id: string): ()
	channel_to_id[chan] = id
end

local function dispatch(result: { channel: any, value: any, ok: boolean }, state: any): any
	register_channel(result.channel, function(inner_state, value, ok, id) return value end)
	register_unpaired(result.channel, "stale")
	local channel_id = channel_to_id[result.channel]
	if channel_id then
		local channel_info = registered_channels[channel_id]
		return channel_info.handler(state, result.value, result.ok, channel_id)
	end
	return nil
end

return dispatch
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	channelToIDPath := findRootLocalPath(t, root, "channel_to_id")
	registeredPath := findRootLocalPath(t, root, "registered_channels")
	var dispatch *body.Result
	var sawRegisterChannelValueKeyProof bool
	for _, child := range root.FunctionResults() {
		if child == nil || child.Graph() == nil {
			continue
		}
		for _, point := range child.Graph().RPO() {
			site, ok := child.CallSiteView(point)
			if ok && site.CalleeSymbol() != 0 && child.SymbolName(site.CalleeSymbol()) == "register_channel" {
				if outcome, ok := child.CallOutcomeAt(point); ok {
					for _, proof := range outcome.NormalReturnFacts.DynamicValueKeys {
						if proof.Container.Symbol == channelToIDPath.Symbol && proof.Table.Symbol == registeredPath.Symbol {
							sawRegisterChannelValueKeyProof = true
						}
					}
				}
			}
			if ok && site.CalleeSymbol() != 0 && child.SymbolName(site.CalleeSymbol()) == "register_unpaired" {
				dispatch = child
				break
			}
		}
		if dispatch != nil {
			break
		}
	}
	if dispatch == nil {
		t.Fatal("dispatch lexical result with register_unpaired call not found")
	}
	if !sawRegisterChannelValueKeyProof {
		var summaries []string
		for _, entry := range result.Snapshot().Entries() {
			facts := entry.Summary.NormalReturnFacts
			if len(facts.DynamicIndexFacts) == 0 && len(facts.DynamicValueKeys) == 0 {
				continue
			}
			summaries = append(summaries, fmt.Sprintf("%s dynamic=%#v valueKeys=%#v", entry.Key.Ref, facts.DynamicIndexFacts, facts.DynamicValueKeys))
		}
		t.Fatalf("register_channel outcome did not prove channel_to_id values are registered_channels keys; summaries: %s", strings.Join(summaries, " | "))
	}
	var readPoint cfg.Point
	var channelID symbol.ID
	for _, point := range dispatch.Graph().RPO() {
		fact, ok := dispatch.LocalAssignment(point)
		if ok && fact.Name == "channel_id" {
			readPoint = point
			channelID = fact.Symbol
			break
		}
	}
	if readPoint == 0 {
		t.Fatalf("channel_id read point not found in dispatch")
	}
	st, ok := dispatch.StateAt(readPoint)
	if !ok {
		t.Fatalf("state at channel_id read point %d missing", readPoint)
	}
	for _, membership := range st.KeyMembershipsSnapshot().Memberships {
		if membership.Kind == state.KeyMembershipDynamicIndexValue && membership.Container.Sym == channelToIDPath.Symbol {
			t.Fatalf("key memberships = %#v, want unpaired helper write to clear value-key proof for channel_to_id", st.KeyMembershipsSnapshot())
		}
	}
	var channelInfoPoint cfg.Point
	for _, point := range dispatch.Graph().RPO() {
		fact, ok := dispatch.LocalAssignment(point)
		if ok && fact.Name == "channel_info" {
			channelInfoPoint = point
			break
		}
	}
	if channelInfoPoint == 0 {
		t.Fatalf("channel_info read point not found in dispatch")
	}
	st, ok = dispatch.StateAt(channelInfoPoint)
	if !ok {
		t.Fatalf("state at channel_info read point %d missing", channelInfoPoint)
	}
	for _, membership := range st.KeyMembershipsSnapshot().Memberships {
		if membership.Kind != state.KeyMembershipPath {
			continue
		}
		key, keyOK := dispatch.KeySpace().FromStateKey(membership.Key.PathKey())
		table, tableOK := dispatch.KeySpace().FromStateKey(membership.Table.PathKey())
		if keyOK && tableOK && key.Sym == channelID && table.Sym == registeredPath.Symbol {
			t.Fatalf("key memberships = %#v, want unpaired helper write to prevent channel_id proof as registered_channels key", st.KeyMembershipsSnapshot())
		}
	}
}

func TestRunChunkImportedArrayReturnKeepsAnnotatedSiblingReturn(t *testing.T) {
	reg := standard.Registry()
	entryType := typetable.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewMap(typ.String, typ.Any)).
		Build()
	registryType := typetable.NewRecord().
		Field("find", typ.Func().
			Param("query", typ.Any).
			Returns(typ.NewArray(entryType), typ.Any).
			Build()).
		Build()
	stmts := parseChunk(t, `
local function run_suite(name: string, tests: any[]): ()
end

local function group_by_suite(entries)
	local suites: {[string]: any[]} = {}
	local no_suite: any[] = {}
	for _, entry in ipairs(entries) do
		table.insert(no_suite, entry)
	end
	return suites, no_suite
end

local entries, err = registry.find({["meta.type"] = "test"})
if err then
	return
end
local suites, no_suite = group_by_suite(entries)
run_suite("other", no_suite)
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:    reg,
			Globals:     []string{"registry", "ipairs", "table"},
			GlobalTypes: map[string]typ.Type{"registry": registryType},
			Signatures:  signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	groupPoint := requireCallByCalleeName(t, root, "group_by_suite")
	outcome, ok := root.CallOutcomeAt(groupPoint)
	if !ok {
		t.Fatalf("group_by_suite call outcome missing at %d", groupPoint)
	}
	if len(outcome.Results) < 2 {
		t.Fatalf("group_by_suite results = %#v, want two return slots", outcome.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, outcome.Results[1].Value)
	if !ok || !typ.TypeEquals(gotType, typ.NewArray(typ.Any)) {
		t.Fatalf("group_by_suite return 2 type = %v/%v (value %#v), want any[]; summaries %s", gotType, ok, outcome.Results[1].Value, debugSummaryReturnTypes(reg, result.Snapshot()))
	}
	runPoint, arg := requireCallArgumentByCalleeName(t, root, "run_suite", 1)
	value, ok := root.ExpressionValueAtBoundary(runPoint, arg)
	if !ok {
		t.Fatalf("run_suite argument value missing at %d", runPoint)
	}
	argType, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(argType, typ.NewArray(typ.Any)) {
		t.Fatalf("run_suite argument type = %v/%v (value %#v), want any[]", argType, ok, value)
	}
}

func TestRunChunkLocalFunctionInsertedArrayCallOutcomeInfersElementType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Entry = {id: string, meta: {type: string, suite: string?, order: number?}?}

local function group_by_suite(entries: {Entry})
	local no_suite = {}
	for _, entry in ipairs(entries) do
		table.insert(no_suite, entry)
	end
	return no_suite
end

local entries: {Entry} = {}
local no_suite = group_by_suite(entries)
local uncategorized: {Entry} = no_suite
`)

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Globals:    []string{"ipairs", "table"},
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("missing root result")
	}
	callPoint := requireCallByCalleeName(t, root, "group_by_suite")
	outcome, ok := root.CallOutcomeAt(callPoint)
	if !ok || len(outcome.Results) == 0 {
		t.Fatalf("CallOutcomeAt(%d) = %#v/%v, want inferred return", callPoint, outcome, ok)
	}
	want := typ.NewArray(typetable.NewRecord().
		Field("id", typ.String).
		Field("meta", typeexpr.Optional(typetable.NewRecord().
			Field("type", typ.String).
			Field("suite", typeexpr.Optional(typ.String)).
			Field("order", typeexpr.Optional(typ.Number)).
			Build())).
		Build())
	gotType, ok := typevalue.TypeOf(reg, outcome.Results[0].Value)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("group_by_suite result type = %v/%v (value %#v), want %v", gotType, ok, outcome.Results[0].Value, want)
	}
}

func TestRunChunkReexportsManifestSendEffectAsEscapeEvent(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local forward = function(payload)
	runtime.send(payload)
end
`)
	local := stmts[0].(*ast.LocalAssignStmt)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"runtime"}})
	forwardSym := mustBoundLocalAt(t, bindings, local, 0)
	m := manifest.New("actor_runtime")
	m.DefineFunctionSignature("runtime.send", signature.Function{
		Effect: effect.Empty.With(ownership.Send{FromParam: 0}),
	})

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"runtime"},
			Signatures: signaturelookup.Source{
				Manifests: []*manifest.Manifest{m},
			},
		},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	forwardKey, ok := result.TargetKey(forwardSym)
	if !ok {
		t.Fatalf("TargetKey(forward) missing")
	}
	assertSummaryEscapeEvent(
		t,
		result.Snapshot(),
		forwardKey,
		path.NewPlaceholder(0),
		callboundary.EscapeEventSend,
		true,
	)
}

func TestRunChunkMaterializedSummaryCarriesNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function sorted_keys(t)
	local keys: string[] = {}
	for k in pairs(t) do
		table.insert(keys, k)
	end
	table.sort(keys)
	return keys
end

local suites: {[string]: any[]} = {}
local suite_names = sorted_keys(suites)
return suite_names
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"pairs", "table"}})
	origin := functionOriginByName(t, bindings, "sorted_keys")
	if !origin.HasTargetSymbol {
		t.Fatalf("sorted_keys origin has no target symbol: %#v", origin)
	}

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	targetKey, ok := result.TargetKey(origin.TargetSymbol)
	if !ok {
		t.Fatalf("TargetKey(sorted_keys) missing")
	}
	assertSummaryReturnedKeyedArrayProvenance(t, result.Snapshot(), targetKey)
}

func TestRunChunkSpecializesGenericSummaryReturnAtCallSite(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

local function err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return ok(fn(result.value))
	end
	return err(result.error)
end

local mapped = map_result(ok("x"), function(item: string): number
	return 1
end)
return mapped
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	got, ok := result.Snapshot().Read(result.RootKey())
	if !ok || len(got.Returns) != 1 {
		t.Fatalf("root summary = %#v/%v, want one return", got, ok)
	}
	witness := product.Get(reg, got.Returns[0], typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || typ.IsAny(gotType) || typ.IsUnknown(gotType) {
		t.Fatalf("mapped return witness = %#v, want concrete Result<number>", witness)
	}
	if refinement.ContainsFreeTypeParam(gotType) {
		t.Fatalf("mapped return type = %v, want no free type params", gotType)
	}
}

func TestDeclaredFunctionReturnCanUseSummaryKeepsOwnedTypeParamAuthoritative(t *testing.T) {
	owned := typ.NewTypeParam("T", nil)
	fn := typ.Func().TypeParamRef(owned).Returns(owned).Build()

	if declaredFunctionReturnCanUseSummary(fn, owned, typ.String) {
		t.Fatal("owned function type parameter return used summary type; generic return contract must stay authoritative")
	}

	free := typ.NewTypeParam("U", nil)
	if !declaredFunctionReturnCanUseSummary(fn, free, typ.String) {
		t.Fatal("free non-owned return parameter did not accept concrete summary replacement")
	}
}

func TestRunChunkCarriesGenericResultValueThroughGuard(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

local function err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return ok(fn(result.value))
	end
	return err(result.error)
end

local mapped = map_result(ok(41), function(item: number): string
	return tostring(item)
end)
return mapped
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	child := requireLexicalResultByName(t, root, "map_result")
	var sawResultRoot bool
	for _, point := range child.Graph().RPO() {
		fact, ok := child.SourceCall(point)
		if !ok || fact.Call == nil || len(fact.Call.Args) != 1 {
			continue
		}
		argPath, ok := child.ExpressionPath(fact.Call.Args[0])
		if !ok || argPath.String() != "result.value" {
			continue
		}
		value, ok := child.ExpressionValueAtBoundary(point, fact.Call.Args[0])
		if !ok {
			t.Fatal("result.value boundary value missing in lexical result")
		}
		rootValue, rootOK := child.PathValueAtBoundary(point, argPath.RootOnly())
		if !rootOK {
			continue
		}
		sawResultRoot = true
		got, ok := typevalue.TypeOf(reg, value)
		if !ok || !subtype.IsSubtype(got, typ.Number) {
			structural, structuralOK := typevalue.StructuralTypeOf(reg, typevalue.NewCache(), value, typevalue.StructuralTypeOptions{})
			rootType, rootTypeOK := typevalue.TypeOf(reg, rootValue)
			rootStructural, rootStructuralOK := typevalue.StructuralTypeOf(reg, typevalue.NewCache(), rootValue, typevalue.StructuralTypeOptions{})
			t.Fatalf("result.value type = %v/%v structural=%v/%v presence=%v runtime=%v witness=%v origin=%v evidence=%v value=%#v root=%v/%v structural=%v/%v origin=%v rootOK=%v, want number",
				got, ok,
				structural, structuralOK,
				product.PresenceOf(value),
				product.Get(reg, value, runtimekind.Key),
				product.Get(reg, value, typewitness.Key),
				product.Get(reg, value, variantorigin.Key),
				product.Get(reg, value, evidence.Key),
				value,
				rootType, rootTypeOK,
				rootStructural, rootStructuralOK,
				product.Get(reg, rootValue, variantorigin.Key),
				rootOK)
		}
		return
	}
	if !sawResultRoot {
		return
	}
	t.Fatalf("callback call argument result.value not found")
}

func TestRunChunkParamUsesInstantiatedGenericContractForFalseEdge(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return { ok = true, value = fn(result.value) }
	end
	return err(result.error)
end

local mapped = map_result({ ok = false, error = "x" }, function(item: string): number
	return #item
end)
return mapped
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	child := requireLexicalResultByName(t, result.RootResult(), "map_result")
	for _, point := range child.Graph().RPO() {
		fact, ok := child.SourceCall(point)
		if !ok || fact.Call == nil || len(fact.Call.Args) != 1 {
			continue
		}
		argPath, ok := child.ExpressionPath(fact.Call.Args[0])
		if !ok || argPath.String() != "result.error" {
			continue
		}
		value, ok := child.ExpressionValueAtBoundary(point, fact.Call.Args[0])
		if !ok {
			t.Fatal("result.error boundary value missing in lexical result")
		}
		if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
			t.Fatalf("result.error presence = %s, want present", got)
		}
		got, ok := typevalue.TypeOf(reg, value)
		if !ok || !subtype.IsSubtype(got, typ.String) {
			t.Fatalf("result.error type = %v/%v, want subtype of string", got, ok)
		}
		return
	}
	t.Fatal("map_result result.error argument not found")
}

func TestRunChunkMaterializesGenericMapBindResultLocals(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }
type Profile = { id: string, count: number }

local function ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

local function err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

local function profile(id: string, count: number): Profile
	return { id = id, count = count }
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return ok(fn(result.value))
	end
	return err(result.error)
end

local function bind_result<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
	if result.ok then
		return fn(result.value)
	end
	return err(result.error)
end

local mapped = map_result(ok(profile("abc", 41)), function(item: Profile): string
	return item.id
end)

if mapped.ok then
	local x: string = mapped.value
end

local bound = bind_result(ok(profile("def", 41)), function(item: Profile): Result<number>
	return ok(item.count + 1)
end)

if bound.ok then
	local y: number = bound.value
end
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}

	mappedStmt := mustFindLocalAssign(t, stmts, "mapped")
	boundStmt := mustFindLocalAssign(t, stmts, "bound")
	xStmt := mustFindLocalAssign(t, stmts, "x")
	yStmt := mustFindLocalAssign(t, stmts, "y")

	mappedPoint := requireLocalAssignmentPoint(t, root, mappedStmt, 0)
	boundPoint := requireLocalAssignmentPoint(t, root, boundStmt, 0)
	xPoint := requireLocalAssignmentPoint(t, root, xStmt, 0)
	yPoint := requireLocalAssignmentPoint(t, root, yStmt, 0)

	assertBoundarySymbolWitnessClosed(t, reg, root, mappedPoint, mustResultLocalAt(t, root, mappedStmt, 0), "mapped")
	assertBoundarySymbolWitnessClosed(t, reg, root, boundPoint, mustResultLocalAt(t, root, boundStmt, 0), "bound")
	assertBoundaryExprRuntimeKind(t, reg, root, xPoint, xStmt.Exprs[0], runtimekind.Singleton(runtimekind.String), "mapped.value")
	assertBoundaryExprRuntimeKind(t, reg, root, yPoint, yStmt.Exprs[0], runtimekind.Singleton(runtimekind.Number), "bound.value")
}

func TestRunChunkMaterializesGenericPairMultipleReturns(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function pair<A, B>(a: A, b: B): (A, B)
	return a, b
end
local n, s = pair(42, "hello")
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	pairStmt := mustFindLocalAssign(t, stmts, "n")
	nPoint := requireLocalAssignmentPoint(t, root, pairStmt, 0)
	sPoint := requireLocalAssignmentPoint(t, root, pairStmt, 1)

	assertBoundarySymbolType(t, reg, root, nPoint, mustResultLocalAt(t, root, pairStmt, 0), typ.LiteralInt(42), "n")
	assertBoundarySymbolType(t, reg, root, sPoint, mustResultLocalAt(t, root, pairStmt, 1), typ.LiteralString("hello"), "s")
}

func TestRunChunkMaterializesAnnotatedArrayReturnAfterDynamicIPairsInsert(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function consume(xs: any[]): ()
end

local function group_by_suite(entries)
	local suites: {[string]: any[]} = {}
	local no_suite: any[] = {}
	for _, entry in ipairs(entries) do
		table.insert(no_suite, entry)
	end
	return suites, no_suite
end

local suites, no_suite = group_by_suite({})
consume(no_suite)
`)
	checkConfig := body.Config{
		Registry:   reg,
		Globals:    []string{"ipairs", "table"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(checkConfig)})
	origin := functionOriginByName(t, bindings, "group_by_suite")

	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: checkConfig,
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	targetKey, ok := result.TargetKey(origin.TargetSymbol)
	if !ok {
		t.Fatalf("TargetKey(group_by_suite) missing")
	}
	base, ok := result.Snapshot().Read(targetKey)
	if !ok || len(base.Returns) != 2 {
		t.Fatalf("base summary = %#v/%v, want two returns", base, ok)
	}
	baseType, baseTypeOK := typevalue.TypeOf(reg, base.Returns[1])
	if !baseTypeOK || !typ.TypeEquals(baseType, typ.NewArray(typ.Any)) {
		t.Fatalf("base summary return slot 2 type = %v/%v, want any[]; summaries %s", baseType, baseTypeOK, debugSummaryReturnTypes(reg, result.Snapshot()))
	}
	noSuiteStmt := mustFindLocalAssign(t, stmts, "no_suite")
	noSuitePoint := requireLocalAssignmentPoint(t, root, noSuiteStmt, 1)
	value, ok := root.SymbolValueAtBoundary(noSuitePoint, mustResultLocalAt(t, root, noSuiteStmt, 1))
	if !ok {
		t.Fatalf("no_suite boundary value missing at %v", noSuitePoint)
	}
	gotType, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(gotType, typ.NewArray(typ.Any)) {
		t.Fatalf("no_suite type = %v/%v, want any[] (value %#v, summaries %s)", gotType, ok, value, debugSummaryReturnTypes(reg, result.Snapshot()))
	}
	consumePoint, consumeArg := requireCallArgumentByCalleeName(t, root, "consume", 0)
	argValue, ok := root.ExpressionValueAtBoundary(consumePoint, consumeArg)
	if !ok {
		t.Fatalf("consume argument value missing at %v", consumePoint)
	}
	argType, ok := typevalue.TypeOf(reg, argValue)
	if !ok || !typ.TypeEquals(argType, typ.NewArray(typ.Any)) {
		t.Fatalf("consume argument type = %v/%v, want any[] (value %#v)", argType, ok, argValue)
	}
	for _, diag := range diagnostics.Produce(root) {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("direct-call diagnostics = %#v, want consume(no_suite) to be accepted", diag)
		}
	}
}

func TestRunChunkMaterializesNestedCallbackMethodReturnLocals(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Message = {
	from: fun(self: Message): string,
	payload: fun(self: Message): any,
}

type Channel = {
	receive: fun(self: Channel): (Message, boolean),
}

local process = {}
function process.listen(): Channel
	error("stub")
end
function process.send(pid: string, topic: string): boolean
	return true
end

local done = false
coroutine.spawn(function()
	local ch = process.listen()
	while not done do
		local msg, ok = ch:receive()
		if not ok then
			break
		end
		local reply_to = msg:from()
		process.send(reply_to, "ack")
	end
end)
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	listenFn := findFunctionForPath(t, bindings, stmts, "process.listen")
	listenType, ok := lowerFunctionExprType(listenFn, bindings, nil)
	if !ok || listenType == nil || len(listenType.Returns) != 1 {
		t.Fatalf("process.listen type = %#v/%v, want one return", listenType, ok)
	}
	if witness := typewitness.Of(listenType.Returns[0]); witness.IsTop() || witness.IsBottom() {
		t.Fatalf("process.listen declared return witness = %v for %v, want concrete", witness, listenType.Returns[0])
	}
	listenSym, ok := bindings.FunctionSymbol(listenFn)
	if !ok || listenSym == 0 {
		t.Fatalf("process.listen function symbol missing")
	}
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	processPath := findRootLocalPath(t, result.RootResult(), "process")
	listenKey, ok := result.PathKey(processPath.Field("listen").Key())
	if !ok {
		t.Fatalf("summary path key for process.listen missing")
	}
	functionKey, ok := result.FunctionKey(listenSym)
	if !ok || functionKey != listenKey {
		t.Fatalf("process.listen path key = %#v, function key = %#v/%v", listenKey, functionKey, ok)
	}
	child, chPoint, ch := findNestedLocalByName(t, result.RootResult(), "ch")
	assertBoundarySymbolConcreteType(t, reg, child, chPoint, ch, "nested ch")
	child, msgPoint, msg := findNestedLocalByName(t, result.RootResult(), "msg")
	assertBoundarySymbolConcreteType(t, reg, child, msgPoint, msg, "nested msg")
	child, point, reply := findNestedLocalByName(t, result.RootResult(), "reply_to")
	assertBoundarySymbolType(t, reg, child, point, reply, typ.String, "nested reply_to")
}

func TestRunChunkFieldDefinedWrapperReturnUsesCallerPathContext(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

function M.run()
	return M.dep.get()
end

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local res = M.run()
local answer: string = res.answer
return answer
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	answerStmt := mustFindLocalAssign(t, stmts, "answer")
	answerPoint := requireLocalAssignmentPoint(t, root, answerStmt, 0)
	assertBoundaryExprRuntimeKind(t, reg, root, answerPoint, answerStmt.Exprs[0], runtimekind.Singleton(runtimekind.String), "res.answer")
	if diags := diagnostics.Produce(root); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestRunChunkRecursiveMethodReturnDoesNotRematerializeForever(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Handler = { run: (self: Handler, n: number) -> Handler, value: number }

local function step(h: Handler): Handler
	return h:run(1)
end

local h: Handler = {
	value = 0,
	run = function(self: Handler, n: number): Handler return self end,
}
return step(h).value
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	if diags := diagnostics.Produce(result.RootResult()); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestRunChunkSeedsColonMethodCallbackParamFromTypedSignature(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel
type Router = {
	on: fun(self: Router, key: string, cb: fun(action: Action): string): (),
}

local function make_router(): Router
	error("stub")
end

local router: Router = make_router()
router:on("begin", function(action)
	local seen = action
	return action.kind
end)
`)
	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	child, point, seen := findNestedLocalByName(t, result.RootResult(), "seen")
	assertBoundarySymbolConcreteType(t, reg, child, point, seen, "typed colon callback param")
}

func TestRunChunkSeedsDirectTypedCallbackParamFromCalleeAnnotation(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Action = Begin | Commit

local visit_begin: fun(cb: fun(action: Begin): string): string = function(cb)
	return cb({ kind = "begin", id = "evt-1" })
end

visit_begin(function(action)
	local seen = action
	return action.id
end)
`)
	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	child, point, seen := findNestedLocalByName(t, result.RootResult(), "seen")
	want := typetable.NewRecord().
		Field("id", typ.String).
		Field("kind", typ.LiteralString("begin")).
		Build()
	assertBoundarySymbolType(t, reg, child, point, seen, want, "direct typed callback param")
}

func TestRunChunkSeedsDirectCallbackParamFromFunctionDeclarationAnnotation(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Begin = { kind: "begin", id: string }
type Envelope<T> = { payload: T }

local function visit_begin(cb: fun(env: Envelope<Begin>): string): string
	return cb({ payload = { kind = "begin", id = "evt-1" } })
end

visit_begin(function(env)
	local seen = env.payload
	return env.payload.id
end)
`)
	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	child, point, seen := findNestedLocalByName(t, result.RootResult(), "seen")
	want := typetable.NewRecord().
		Field("id", typ.String).
		Field("kind", typ.LiteralString("begin")).
		Build()
	assertBoundarySymbolType(t, reg, child, point, seen, want, "direct declaration callback param")
}

func TestRunChunkNestedContextKeepsFactoryCallbackLocalArgumentShape(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local captured_fn

coroutine = {
	spawn = function(fn: () -> ())
		captured_fn = fn
	end,
}

local actor = {}

function actor.new(initial_state: any, handlers: any): any
	local state = {}

	local function async(fn: () -> ())
		coroutine.spawn(function()
			fn()
		end)
	end

	state.async = async

	if handlers.__init then
		handlers.__init(state)
	end

	return {
		run = function() end,
	}
end

local a = actor.new({}, {
	__init = function(state)
		state.async(function() end)
	end,
})

captured_fn()
`)
	globals := []string{"_G", "coroutine"}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg, Globals: globals}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	if diags := diagnostics.Produce(result.RootResult()); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want factory callback context to preserve local state.async side effect", diags)
	}
}

func TestRunChunkServiceLocatorWrapperSingletonMaterializationConverges(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Logger = {
	info: (self: Logger, message: string) -> (),
}

local logger = {}

function logger.new(): Logger
	return {
		info = function(self: Logger, message: string): ()
		end,
	}
end

type Cache = {
	set: (self: Cache, key: string, value: any) -> (),
	get: (self: Cache, key: string) -> any,
}

local cache = {}

function cache.new(): Cache
	local store: {[string]: any} = {}
	return {
		set = function(self: Cache, key: string, value: any): ()
			store[key] = value
		end,
		get = function(self: Cache, key: string): any
			return store[key]
		end,
	}
end

type Services = {
	logger: Logger,
	cache: Cache,
}

local locator = {}
local _services: Services? = nil

function locator.init(): Services
	local services: Services = {
		logger = logger.new(),
		cache = cache.new(),
	}
	_services = services
	return services
end

function locator.get(): Services
	if not _services then
		return locator.init()
	end
	return _services
end

function locator.logger(): Logger
	return locator.get().logger
end

function locator.cache(): Cache
	return locator.get().cache
end

local services = locator.init()
services.logger:info("boot")
services.cache:set("boot", true)
locator.logger():info("ready")
locator.cache():set("ready", true)
`)
	result, err := RunChunk(stmts, Config{
		Check: body.Config{Registry: reg},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	if diags := diagnostics.Produce(result.RootResult()); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want service-locator wrapper singleton to type-check cleanly", diags)
	}
}

func TestRunChunkNonDominatingFieldDefinedWrapperReturnStaysMaybeNil(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function run(flag: boolean)
	local M = {
		dep = {
			get = function()
				return nil
			end,
		},
	}

	function M.run()
		return M.dep.get()
	end

	if flag then
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
	end

	local res = M.run()
	local answer: string = res.answer
	return answer
end

return run
`)
	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	if diags := diagnostics.Produce(root); len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one maybe-nil wrapper return diagnostic", diags)
	}
}

func TestRunChunkNonDominatingFieldWriteCallAssignmentStaysMaybeNil(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function run(flag: boolean)
	local M = {
		dep = {
			get = function()
				return nil
			end,
		},
	}

	if flag then
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
	end

	local res = M.dep.get()
	local answer: string = res.answer
	return answer
end

return run
`)
	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	if diags := diagnostics.Produce(root); len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one maybe-nil field call diagnostic", diags)
	}
}

func TestRunChunkFieldDefinedWrapperAliasFunctionValueUsesCallerPathContext(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Res = { answer: string }
local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

function M.run()
	return M.dep.get()
end

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local f: fun(): Res = M.run
local res = f()
local answer: string = res.answer
return answer
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	root := result.RootResult()
	fStmt := mustFindLocalAssign(t, stmts, "f")
	fPoint := requireLocalAssignmentPoint(t, root, fStmt, 0)
	fn, ok := root.FunctionValueTypeAtBoundary(fPoint, fStmt.Exprs[0])
	if !ok {
		t.Fatalf("function value type for M.run alias missing")
	}
	if len(fn.Returns) != 1 {
		t.Fatalf("function value returns = %v, want one Res return", fn.Returns)
	}
	want := typetable.NewRecord().Field("answer", typ.String).Build()
	if !subtype.IsSubtype(fn.Returns[0], want) {
		t.Fatalf("function value return = %v, want subtype of %v", fn.Returns[0], want)
	}
	if diags := diagnostics.Produce(root); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestRunChunkSeedsReachableHeapForNestedLiteralParamRead(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function read_name(payload)
	local value: string = payload.user.profile.name
	return value
end

local result = read_name({
	user = {
		profile = {
			name = "ok",
		},
	},
})

local answer: string = result
return answer
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	answerStmt := mustFindLocalAssign(t, stmts, "answer")
	answerPoint := requireLocalAssignmentPoint(t, root, answerStmt, 0)
	assertBoundaryExprRuntimeKind(t, reg, root, answerPoint, answerStmt.Exprs[0], runtimekind.Singleton(runtimekind.String), "answer")
	if diags := diagnostics.Produce(root); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestRunChunkSeedsReachableHeapThroughForwardingChain(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function read_name(payload)
	local value: string = payload.user.profile.name
	return value
end

local function forward(payload)
	return read_name(payload)
end

local result = forward({
	user = {
		profile = {
			name = "ok",
		},
	},
})

local answer: string = result
return answer
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatalf("RootResult missing")
	}
	answerStmt := mustFindLocalAssign(t, stmts, "answer")
	answerPoint := requireLocalAssignmentPoint(t, root, answerStmt, 0)
	assertBoundaryExprRuntimeKind(t, reg, root, answerPoint, answerStmt.Exprs[0], runtimekind.Singleton(runtimekind.String), "answer")
	if diags := diagnostics.Produce(root); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestRunChunkSeedsMethodSelfFromMetatableIndexFactory(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local methods = {}
local mt = { __index = methods }
local node = {}

type NodeInstance = {
	id: string,
}

local function sink(value: NodeInstance)
end

function node.new()
	local instance: NodeInstance = { id = "root" }
	return setmetatable(instance, mt)
end

function methods:touch()
	sink(self)
end

local instance: NodeInstance = { id = "root" }
methods.touch(instance)
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"setmetatable"}})
	context := collectMetatableMethodContext(bindings, nil, body.Config{}.ModuleExports, stmts)
	receivers := metatableMethodReceiverTypes(bindings, nil, body.Config{}.ModuleExports, stmts)
	methods := mustBoundLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	if got, ok := receivers[methods]; !ok || !subtype.IsSubtype(got, typetable.NewRecord().Field("id", typ.String).Build()) {
		t.Fatalf("metatable method receiver = %v/%v, want NodeInstance", got, ok)
	}
	gotRecord, ok := unwrap.Alias(receivers[methods]).(*typ.Record)
	if !ok || gotRecord.GetStaticStringIndex("touch") == nil {
		t.Fatalf("metatable method receiver = %v, want static fallback method touch", receivers[methods])
	}
	touch := gotRecord.GetStaticStringIndex("touch")
	touchFn, ok := unwrap.Alias(touch.Type).(*typ.Function)
	if !ok || len(touchFn.Params) == 0 || !typ.TypeEquals(touchFn.Params[0].Type, context.seedReceivers[methods]) {
		t.Fatalf("touch member type = %v, seed = %v, want explicit self to use original receiver", touch.Type, context.seedReceivers[methods])
	}
	seedRecord, ok := unwrap.Alias(context.seedReceivers[methods]).(*typ.Record)
	if !ok || seedRecord.GetStaticStringIndex("touch") != nil {
		t.Fatalf("seed receiver = %v, want original NodeInstance without prototype methods", context.seedReceivers[methods])
	}
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"setmetatable"},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	selfType, ok := methodImplicitSelfEntryType(t, root, "touch")
	if !ok || !subtype.IsSubtype(selfType, context.seedReceivers[methods]) {
		t.Fatalf("method self entry type = %v/%v, seed = %v", selfType, ok, context.seedReceivers[methods])
	}
	selfRecord, ok := unwrap.Alias(selfType).(*typ.Record)
	if !ok || selfRecord.GetStaticStringIndex("touch") == nil {
		t.Fatalf("method self entry type = %v, want metatable method surface", selfType)
	}
}

func methodImplicitSelfEntryType(t *testing.T, root *body.Result, method string) (typ.Type, bool) {
	t.Helper()
	for _, child := range root.FunctionResults() {
		fn := child.Function()
		origin, ok := root.FunctionOrigin(fn)
		if !ok || origin.Kind != bind.FunctionOriginMethod || origin.Method != method {
			continue
		}
		entry, ok := child.EntryState()
		if !ok {
			t.Fatalf("method %s entry state missing", method)
		}
		for _, slot := range child.FunctionParamSlots(fn) {
			if !slot.ImplicitSelf || slot.Symbol == 0 {
				continue
			}
			value := entry.ReadValue(child.Registry(), statekey.SymbolValue(slot.Symbol))
			if product.Equal(child.Registry(), value, product.Bottom(child.Registry())) {
				return nil, false
			}
			return typevalue.TypeOf(child.Registry(), value)
		}
	}
	return nil, false
}

func TestCollectPlainMethodReceiverPrefersSiblingDeclaredTypeOverLocalTableShape(t *testing.T) {
	stmts := parseChunk(t, `
type Store = {
	started_at: number,
	run: (self: Store) -> number,
}

local Store = {}

function Store:run(): number
	return self.started_at
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	context := collectMetatableMethodContext(bindings, nil, body.Config{}.ModuleExports, stmts)
	methods := mustBoundLocalAt(t, bindings, stmts[1].(*ast.LocalAssignStmt), 0)

	seed, ok := context.seedReceivers[methods]
	if !ok {
		t.Fatalf("seed receiver missing for Store")
	}
	field, ok := access.Field(seed, "started_at")
	if !ok || !typ.TypeEquals(field, typ.Number) {
		t.Fatalf("seed receiver started_at = %v/%v, want declared number field", field, ok)
	}
	receiver, ok := context.methodReceivers[methods]
	if !ok {
		t.Fatalf("method receiver missing for Store")
	}
	if field, ok := access.Field(receiver, "started_at"); !ok || !typ.TypeEquals(field, typ.Number) {
		t.Fatalf("method receiver started_at = %v/%v, want declared number field", field, ok)
	}
	if method, ok := access.Field(receiver, "run"); !ok || method == nil {
		t.Fatalf("method receiver run = %v/%v, want method surface retained", method, ok)
	}
}

func TestCollectPlainMethodReceiverUsesDeclaredReturnLocal(t *testing.T) {
	stmts := parseChunk(t, `
type Executor = {
	with_context: (self: Executor, ctx: table) -> Executor,
}

local function make_executor(): Executor
	local e = {}
	function e:with_context(ctx: table): Executor
		return self
	end
	return e
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	context := collectMetatableMethodContext(bindings, nil, body.Config{}.ModuleExports, stmts)
	makeAssign, ok := stmts[1].(*ast.LocalAssignStmt)
	if !ok || len(makeAssign.Exprs) == 0 {
		t.Fatalf("make_executor stmt = %T, want local function assignment", stmts[1])
	}
	makeFn, ok := makeAssign.Exprs[0].(*ast.FunctionExpr)
	if !ok || len(makeFn.Stmts) == 0 {
		t.Fatalf("make_executor expr = %#v, want function body", makeAssign.Exprs[0])
	}
	localE, ok := makeFn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("first make_executor stmt = %T, want local e assignment", makeFn.Stmts[0])
	}
	e := mustBoundLocalAt(t, bindings, localE, 0)
	receiver := context.methodReceivers[e]
	member, ok := access.Field(receiver, "with_context")
	if !ok {
		t.Fatalf("receiver = %v, want with_context method surface", receiver)
	}
	fn, ok := unwrap.Alias(member).(*typ.Function)
	if !ok || len(fn.Params) == 0 {
		t.Fatalf("with_context = %v, want function with self param", member)
	}
	if _, ok := access.Field(fn.Params[0].Type, "with_context"); !ok {
		t.Fatalf("with_context self = %v, want declared return receiver surface", fn.Params[0].Type)
	}
}

func TestCollectMetatableMethodReceiverForOptionObjectLiteralSelf(t *testing.T) {
	stmts := parseChunk(t, `
local methods = {}
local mt = { __index = methods }

type RouteTarget = {
    node_id: string?,
}

type NodeInstance = {
    node_id: string,
    targets: {RouteTarget},
}

local function new_node()
    local instance: NodeInstance = { node_id = "n1", targets = {} }
    return setmetatable(instance, mt), nil
end

function methods:data(data_type: string, content: unknown, options: {node_id: string?}?): (NodeInstance, string?)
    return self, nil
end

function methods:route(content: unknown): ()
    for _, target in ipairs(self.targets) do
        self:data("output", content, {
            node_id = target.node_id or self.node_id,
        })
    end
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"setmetatable"}})
	receivers := metatableMethodReceiverTypes(bindings, nil, body.Config{}.ModuleExports, stmts)
	methods := mustBoundLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	got, ok := receivers[methods]
	if !ok {
		t.Fatal("missing metatable method receiver")
	}
	if projected, ok := luatypeprojection.ApplySegments(got, []segment.Segment{{Kind: segment.SegmentField, Name: "node_id"}}); !ok || !subtype.IsSubtype(projected, typ.String) {
		t.Fatalf("receiver node_id = %v/%v from %v, want string", projected, ok, got)
	}
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry: standard.Registry(),
			Globals:  []string{"setmetatable"},
			Signatures: signaturelookup.Source{
				IncludeStdlib: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	selfType, ok := methodImplicitSelfEntryType(t, root, "route")
	if !ok {
		t.Fatal("route self entry type missing")
	}
	if projected, ok := luatypeprojection.ApplySegments(selfType, []segment.Segment{{Kind: segment.SegmentField, Name: "node_id"}}); !ok || !subtype.IsSubtype(projected, typ.String) {
		t.Fatalf("route self node_id = %v/%v from %v, want string", projected, ok, selfType)
	}
	targetType, ok := methodGenericForVariableType(t, root, "route", 1)
	if !ok {
		t.Fatal("route target loop variable type missing")
	}
	wantTarget := typetable.NewRecord().Field("node_id", typ.MaterializeOptional(typ.String)).Build()
	if !subtype.IsSubtype(targetType, wantTarget) {
		t.Fatalf("route target loop variable type = %v, want subtype of %v", targetType, wantTarget)
	}
	argType, ok := methodCallArgumentType(t, root, "route", "data", 2)
	if !ok {
		t.Fatal("route data argument 3 type missing")
	}
	wantArg := typetable.NewRecord().Field("node_id", typ.String).Build()
	if !subtype.IsSubtype(argType, wantArg) {
		t.Fatalf("route data argument 3 type = %v, want subtype of %v; node_id entry source: %s", argType, wantArg, methodCallObjectEntrySourceSummary(t, root, "route", "data", 2, "node_id"))
	}
}

func methodCallArgumentType(t *testing.T, root *body.Result, ownerMethod, calleeMethod string, index int) (typ.Type, bool) {
	t.Helper()
	for _, child := range root.FunctionResults() {
		fn := child.Function()
		origin, ok := root.FunctionOrigin(fn)
		if !ok || origin.Kind != bind.FunctionOriginMethod || origin.Method != ownerMethod {
			continue
		}
		graph := child.Graph()
		if graph == nil {
			t.Fatalf("method %s graph missing", ownerMethod)
		}
		for _, point := range graph.RPO() {
			site, ok := child.CallSiteView(point)
			if !ok || site.MethodName() != calleeMethod {
				continue
			}
			source, ok := site.ArgumentSourceAt(index)
			if !ok {
				t.Fatalf("method %s call %s argument %d missing", ownerMethod, calleeMethod, index)
			}
			value, ok := child.SourceValueAtBoundary(point, source)
			if !ok {
				return nil, false
			}
			return typevalue.TypeOf(child.Registry(), value)
		}
	}
	return nil, false
}

func methodCallObjectEntrySourceSummary(t *testing.T, root *body.Result, ownerMethod, calleeMethod string, index int, field string) string {
	t.Helper()
	for _, child := range root.FunctionResults() {
		fn := child.Function()
		origin, ok := root.FunctionOrigin(fn)
		if !ok || origin.Kind != bind.FunctionOriginMethod || origin.Method != ownerMethod {
			continue
		}
		for _, point := range child.Graph().RPO() {
			site, ok := child.CallSiteView(point)
			if !ok || site.MethodName() != calleeMethod {
				continue
			}
			source, ok := site.ArgumentSourceAt(index)
			if !ok || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
				return fmt.Sprintf("argument source %#v", source)
			}
			lit, ok := child.ObjectLiteralView(source.ExprRef)
			if !ok {
				return fmt.Sprintf("argument expr %d has no object literal", source.ExprRef)
			}
			var seen []string
			var found string
			lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
				seen = append(seen, entry.Suffix().String())
				if !entrySuffixField(entry.Suffix(), field) {
					return true
				}
				entrySource := entry.Source()
				op, opOK := child.ExpressionOperationRef(entrySource.ExprRef)
				path, pathOK := child.ValueSourcePath(entrySource)
				found = fmt.Sprintf("%#v op=%#v/%v path=%v/%v", entrySource, op, opOK, path, pathOK)
				return false
			})
			if found != "" {
				return found
			}
			return fmt.Sprintf("node_id entry not found; entries=%v", seen)
		}
	}
	return "call not found"
}

func entrySuffixField(p path.Path, field string) bool {
	if len(p.Segments) == 0 {
		return false
	}
	seg := p.Segments[len(p.Segments)-1]
	return seg.Kind == segment.SegmentField && seg.Name == field
}

func methodGenericForVariableType(t *testing.T, root *body.Result, method string, variableIndex int) (typ.Type, bool) {
	t.Helper()
	for _, child := range root.FunctionResults() {
		fn := child.Function()
		origin, ok := root.FunctionOrigin(fn)
		if !ok || origin.Kind != bind.FunctionOriginMethod || origin.Method != method {
			continue
		}
		for _, point := range child.Graph().RPO() {
			fact, ok := child.GenericFor(point)
			if !ok || !fact.HasSymbols || fact.VariableIndex != variableIndex || variableIndex < 0 || variableIndex >= len(fact.Symbols) {
				continue
			}
			value, ok := child.SymbolValueAtBoundary(point, fact.Symbols[variableIndex])
			if !ok {
				return nil, false
			}
			return typevalue.TypeOf(child.Registry(), value)
		}
	}
	return nil, false
}

func TestRunChunkBooleanModeLoopReceiverPresentAtMethodCall(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Executor = {
    with_context: (self: Executor, ctx: table) -> Executor,
}

local function make_executor(): Executor
    local e = {}
    function e:with_context(ctx: table): Executor
        return self
    end
    return e
end

local function run(use_template: boolean): ()
    local executor = nil
    if not use_template then
        executor = make_executor()
    end

    for i = 1, 2 do
        if use_template then
        else
            executor = executor:with_context({})
        end
    end
end
`)
	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	run, point, receiver := findNestedReceiverCall(t, root, "with_context")
	value, ok := run.PathValueAtBoundary(point, receiver)
	if !ok {
		t.Fatalf("PathValueAtBoundary(%s) returned false", receiver.Key())
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		t.Fatalf("receiver presence = %s in %#v, want present at loop method call", got, value)
	}
}

func TestCollectKeysMethodOriginTypeIncludesSelfFromStandaloneSetmetatable(t *testing.T) {
	stmts := parseChunk(t, `
local QueryBuilder = {}
QueryBuilder.__index = QueryBuilder

type QueryBuilderInstance = {
	with_filter: (self: QueryBuilderInstance, filter: unknown) -> QueryBuilderInstance,
}

function QueryBuilder:with_filter(filter: unknown): QueryBuilderInstance
	return self
end

local function new_builder(): QueryBuilderInstance
	local self: QueryBuilderInstance = {
		with_filter = QueryBuilder.with_filter,
	}
	setmetatable(self, QueryBuilder)
	return self
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"setmetatable"}})
	keys := collectKeys(bindings, summary.DefaultSummaryKey(ref.Root()), standard.Registry(), nil, body.Config{}.ModuleExports, stmts)
	var got *typ.Function
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Kind != bind.FunctionOriginMethod || origin.Method != "with_filter" {
			continue
		}
		key, ok := keys.functionKeys[origin.Symbol]
		if !ok {
			t.Fatal("method function key missing")
		}
		got = keys.functionTypes[key]
		break
	}
	if got == nil {
		t.Fatal("method function type missing")
	}
	if len(got.Params) == 0 || got.Params[0].Name != "self" {
		t.Fatalf("method function params = %#v, want explicit self first", got.Params)
	}
}

func TestRunChunkNestedGenericChannelHelperKeepsTypedChannelArgument(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Event = { id: string }
local function map_receive<T>(ch: Channel<T>, fn: (T) -> string): string
	local value, ok = ch:receive()
	if ok then
		return fn(value)
	end
	return "closed"
end
local M = {}
function M.read_event(ch: Channel<Event>): string
	return map_receive(ch, function(event: Event): string
		return event.id
	end)
end
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	found := false
	for _, child := range root.FunctionResults() {
		fn := child.Function()
		if fn == nil || len(fn.TypeParams) != 1 {
			continue
		}
		for _, slot := range child.FunctionParamSlots(fn) {
			if slot.Name != "ch" || slot.Symbol == 0 {
				continue
			}
			entry, ok := child.EntryState()
			if !ok {
				t.Fatal("map_receive entry state missing")
			}
			gotValue := entry.ReadValue(reg, statekey.SymbolValue(slot.Symbol))
			got, ok := typevalue.TypeOf(reg, gotValue)
			if !ok || got == nil {
				t.Fatalf("map_receive ch entry type = %v/%v, want typed Channel payload", got, ok)
			}
			payload, ok := typecall.AmbientChannelPayloadType(got)
			if !ok || payload == nil || typ.IsAny(payload) || typ.IsUnknown(payload) {
				t.Fatalf("map_receive ch entry type = %v, want typed Channel payload", got)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("map_receive ch entry slot not found")
	}
}

func TestRunChunkNestedGenericChannelReceiveKeepsPayloadPresenceCondition(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Event = { id: string }
local function map_receive<T>(ch: Channel<T>, fn: (T) -> string): string
	local value, ok = ch:receive()
	if ok then
		return fn(value)
	end
	return "closed"
end
local M = {}
function M.read_event(ch: Channel<Event>): string
	return map_receive(ch, function(event: Event): string
		return event.id
	end)
end
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	for _, child := range root.FunctionResults() {
		fn := child.Function()
		if fn == nil || len(fn.TypeParams) != 1 {
			continue
		}
		for _, point := range child.Graph().RPO() {
			site, ok := child.CallSiteView(point)
			if !ok || site.MethodName() != "receive" {
				continue
			}
			outcome, ok := child.CallOutcomeAt(point)
			if !ok {
				t.Fatalf("receive CallOutcomeAt(%d) missing", point)
			}
			var foundPayload bool
			for _, result := range outcome.Results {
				if result.Index != 0 {
					continue
				}
				got, ok := typevalue.TypeOf(reg, result.Value)
				if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) {
					t.Fatalf("receive payload result = %v/%v, want T witness", got, ok)
				}
				foundPayload = true
			}
			if !foundPayload {
				t.Fatalf("receive outcome results = %#v, want payload slot", outcome.Results)
			}
			for _, refinement := range outcome.ReturnConditionSlots {
				if refinement.ReturnIndex == 1 && refinement.ReturnValue && refinement.TargetIndex == 0 &&
					presence.Equal(product.PresenceOf(refinement.Value), presence.Present()) {
					return
				}
			}
			t.Fatalf("receive return condition slots = %#v, want ok=true => payload present", outcome.ReturnConditionSlots)
		}
	}
	t.Fatal("generic map_receive receive call not found")
}

func TestRunChunkNestedGenericChannelReceivePresenceReachesCallbackArgument(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Event = { id: string }
local function map_receive<T>(ch: Channel<T>, fn: (T) -> string): string
	local value, ok = ch:receive()
	if ok then
		return fn(value)
	end
	return "closed"
end
local M = {}
function M.read_event(ch: Channel<Event>): string
	return map_receive(ch, function(event: Event): string
		return event.id
	end)
end
`)

	result, err := RunChunk(stmts, Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	var sawCallback bool
	for _, mapReceive := range root.FunctionResults() {
		if mapReceive == nil || mapReceive.Graph() == nil {
			continue
		}
		var valuePath path.Path
		for _, point := range mapReceive.Graph().RPO() {
			fact, ok := mapReceive.LocalAssignment(point)
			if ok && fact.Name == "value" && fact.HasSymbol && fact.Symbol != 0 {
				valuePath = path.NewPath(fact.Symbol, "value")
				break
			}
		}
		if valuePath.IsEmpty() {
			continue
		}
		for _, point := range mapReceive.Graph().RPO() {
			site, ok := mapReceive.CallSiteView(point)
			if !ok || mapReceive.SymbolName(site.CalleeSymbol()) != "fn" {
				continue
			}
			sawCallback = true
			value, ok := mapReceive.PathValueAtBoundary(point, valuePath)
			if !ok {
				t.Fatalf("value boundary value missing at callback point %d", point)
			}
			if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
				t.Fatalf("value presence at callback point %d = %s (%#v), want present", point, got, value)
			}
		}
	}
	if !sawCallback {
		t.Fatal("fn(value) callback call not found")
	}
}

func TestMetatableMethodReceiverTypesUsesSelfIndexMethodSurfaceWithoutInstanceType(t *testing.T) {
	stmts := parseChunk(t, `
local Worker = {}
Worker.__index = Worker

function Worker:prepare(payload: any): any
	return payload
end

function Worker:run(payload: any): any
	return self:prepare(payload)
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	receivers := metatableMethodReceiverTypes(bindings, nil, body.Config{}.ModuleExports, stmts)
	worker := mustBoundLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	got := receivers[worker]
	record, ok := unwrap.Alias(got).(*typ.Record)
	if !ok || record.GetStaticStringIndex("prepare") == nil || record.GetStaticStringIndex("run") == nil {
		t.Fatalf("receiver surface = %v, want self-index prototype methods prepare/run", got)
	}
}

func TestMetatableMethodSurfaceKeepsImplicitSelfSeparateFromPayload(t *testing.T) {
	stmts := parseChunk(t, `
local Worker = {}
Worker.__index = Worker

function Worker:dispatch(payload: any): (boolean, string?)
	return payload ~= nil, nil
end

local function new_worker()
	return setmetatable({}, Worker)
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"setmetatable"}})
	receivers := metatableMethodReceiverTypes(bindings, nil, body.Config{}.ModuleExports, stmts)
	worker := mustBoundLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	record, ok := unwrap.Alias(receivers[worker]).(*typ.Record)
	if !ok {
		t.Fatalf("receiver = %v, want record", receivers[worker])
	}
	member := record.GetStaticStringIndex("dispatch")
	if member == nil {
		t.Fatalf("receiver = %v, want dispatch method", receivers[worker])
	}
	fn, ok := unwrap.Alias(member.Type).(*typ.Function)
	if !ok {
		t.Fatalf("dispatch member = %v, want function", member.Type)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("dispatch params = %#v, want self and payload", fn.Params)
	}
	if fn.Params[0].Name != "self" {
		t.Fatalf("dispatch first param = %#v, want self", fn.Params[0])
	}
	if fn.Params[1].Name != "payload" || !typ.IsAny(fn.Params[1].Type) {
		t.Fatalf("dispatch second param = %#v, want payload:any", fn.Params[1])
	}
}

func TestMetatableConstructorBranchOnlyFieldsStayOptional(t *testing.T) {
	stmts := parseChunk(t, `
type Resource = {
	close: (self: Resource) -> (),
}

local function open_resource(): Resource
	return {
		close = function(self: Resource) end,
	}
end

local Holder = {}
Holder.__index = Holder

function Holder.new(flag: boolean)
	local self = setmetatable({}, Holder)
	if flag then
		self.resource = open_resource()
	end
	return self
end

function Holder:close(): ()
	self.resource:close()
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"setmetatable"}})
	receivers := metatableMethodReceiverTypes(bindings, nil, body.Config{}.ModuleExports, stmts)
	holder := mustBoundLocalAt(t, bindings, stmts[2].(*ast.LocalAssignStmt), 0)
	record, ok := unwrap.Alias(receivers[holder]).(*typ.Record)
	if !ok {
		t.Fatalf("receiver = %v, want record", receivers[holder])
	}
	field := record.GetField("resource")
	if field == nil {
		t.Fatalf("receiver = %v, want optional resource field", receivers[holder])
	}
	if !field.Optional {
		t.Fatalf("resource field = %#v, want optional field", field)
	}
	if unwrap.IsOptionalLike(field.Type) {
		t.Fatalf("resource field type = %v, want non-nil payload carried by optional field shape", field.Type)
	}
}

func TestSeparateMetatableIndexMethodSurfaceDiagnosticsWithStdlib(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local methods = {}
local mt = { __index = methods }

type NodeInstance = {
	node_id: string,
}

local function new_node()
	local instance: NodeInstance = { node_id = "n1" }
	return setmetatable(instance, mt)
end

function methods:data(data_type: string, content: unknown): (NodeInstance, string?)
	return self, nil
end

function methods:_route_errors(error_content: unknown): ()
	self:data("error", error_content)
end
`)
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"setmetatable"},
			Signatures: signaturelookup.Source{
				IncludeStdlib: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	var routeResult *body.Result
	var routeShapes []string
	for _, child := range root.FunctionResults() {
		origin, ok := root.FunctionOrigin(child.Function())
		if ok && origin.Kind == bind.FunctionOriginMethod && origin.Method == "_route_errors" {
			staticCount := 0
			if entry, entryOK := child.EntryState(); entryOK {
				entry.ForEachPathStaticMember(func(key keyspace.Key, _ product.Value) bool {
					if strings.Contains(string(child.KeySpace().Format(key)), "data") {
						staticCount++
					}
					return true
				})
			}
			routeShapes = append(routeShapes, fmt.Sprintf("staticData=%d", staticCount))
			if routeResult == nil {
				routeResult = child
			}
		}
	}
	if routeResult == nil {
		t.Fatal("_route_errors result missing")
	}
	for _, point := range routeResult.Graph().RPO() {
		site, ok := routeResult.CallSiteView(point)
		methodPath, hasMethodPath := site.MethodPath()
		if !ok || site.MethodName() != "data" || !hasMethodPath {
			continue
		}
		value, ok := routeResult.PathValueBeforeBoundary(point, methodPath)
		if !ok {
			t.Fatalf("PathValueBeforeBoundary(%s) returned false", methodPath.Key())
		}
		memberType, ok := typevalue.TypeOf(reg, value)
		if !ok {
			var staticMembers []string
			if st, stOK := routeResult.StateAt(point); stOK {
				st.ForEachPathStaticMember(func(key keyspace.Key, member product.Value) bool {
					keyText := string(routeResult.KeySpace().Format(key))
					if strings.Contains(keyText, "data") {
						memberType, memberTypeOK := typevalue.TypeOf(reg, member)
						staticMembers = append(staticMembers, fmt.Sprintf("%s typeOK=%v type=%v value=%#v", keyText, memberTypeOK, memberType, member))
					}
					return true
				})
			}
			t.Fatalf("data value has no type before call: methodPath=%s value=%#v routeShapes=%v staticMembers=%v", methodPath.Key(), value, routeShapes, staticMembers)
		}
		if _, ok := unwrap.Alias(memberType).(*typ.Function); !ok {
			t.Fatalf("data member = %v, want function", memberType)
		}
		break
	}
	if diags := diagnostics.Produce(root); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want separate __index methods table to surface data on implicit self", diags)
	}
}

func TestDeadlockDataflowRouteErrorsBoundaryKeepsSelfDataSurface(t *testing.T) {
	reg := standard.Registry()
	src, err := os.ReadFile("../../../../testdata/fixtures/regression/deadlock-dataflow-node/main.lua")
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	stmts := parseChunk(t, string(src))
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"setmetatable"}})
	ctx := context.Background()
	stats := &Stats{}
	defer func() {
		t.Logf("freeze telemetry raw: input=%+v local=%+v scc=%+v region=%+v coordinate=%+v path=%+v fiber=%+v quotient=%+v template=%+v", stats.Freeze.InputValidation, stats.Freeze.LocalSyntax, stats.Freeze.SCCClosureLinking, stats.Freeze.RegionWTO, stats.Freeze.CoordinateClosure, stats.Freeze.PathDependencyPlanning, stats.Freeze.FiberLayout, stats.Freeze.ObservableQuotient, stats.Freeze.TemplateBinding)
	}()
	var profileWindow time.Duration
	if raw := os.Getenv("GO_LUA_TEST_PROFILE_WINDOW"); raw != "" {
		profileWindow, err = time.ParseDuration(raw)
		if err != nil || profileWindow <= 0 {
			t.Fatalf("GO_LUA_TEST_PROFILE_WINDOW=%q: %v", raw, err)
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, profileWindow)
		defer cancel()
	}
	result, err := RunBoundChunk(stmts, bindings, Config{
		Context: ctx, Stats: stats,
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"setmetatable"},
			Signatures: signaturelookup.Source{
				IncludeStdlib: true,
			},
		},
	})
	if profileWindow > 0 && errors.Is(err, context.DeadlineExceeded) {
		return
	}
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var routeResult *body.Result
	var dataKey summary.SummaryKey
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Kind == bind.FunctionOriginMethod && origin.Method == "data" {
			if origin.HasTargetSymbol {
				dataKey, _ = result.TargetKey(origin.TargetSymbol)
			}
			break
		}
	}
	var dataSummary summary.Summary
	if dataKey != (summary.SummaryKey{}) {
		dataSummary, _ = result.Snapshot().Read(dataKey)
	}
	for _, child := range result.RootResult().FunctionResults() {
		origin, ok := result.RootResult().FunctionOrigin(child.Function())
		if ok && origin.Kind == bind.FunctionOriginMethod && origin.Method == "_route_errors" {
			routeResult = child
			break
		}
	}
	if routeResult == nil {
		t.Fatal("_route_errors result missing")
	}
	for _, point := range routeResult.Graph().RPO() {
		site, ok := routeResult.CallSiteView(point)
		methodPath, hasMethodPath := site.MethodPath()
		if !ok || site.MethodName() != "data" || !hasMethodPath {
			continue
		}
		value, ok := routeResult.PathValueAtBoundary(point, methodPath)
		if !ok {
			t.Fatalf("PathValueAtBoundary(%s) returned false", methodPath.Key())
		}
		memberType, ok := typevalue.TypeOf(reg, value)
		if !ok {
			before, beforeOK := routeResult.PathValueBeforeBoundary(point, methodPath)
			beforeType, beforeTypeOK := typevalue.TypeOf(reg, before)
			var staticMembers []string
			if st, stOK := routeResult.StateAtBoundary(point); stOK {
				st.ForEachPathStaticMember(func(key keyspace.Key, member product.Value) bool {
					keyText := fmt.Sprintf("%v", key)
					if strings.Contains(keyText, "data") {
						memberType, memberTypeOK := typevalue.TypeOf(reg, member)
						staticMembers = append(staticMembers, fmt.Sprintf("%s typeOK=%v type=%v value=%#v", keyText, memberTypeOK, memberType, member))
					}
					return true
				})
			}
			t.Fatalf("data value has no type: methodPath=%s boundary=%#v beforeOK=%v beforeTypeOK=%v beforeType=%v before=%#v dataSummaryInvalidations=%#v staticMembers=%v", methodPath.Key(), value, beforeOK, beforeTypeOK, beforeType, before, dataSummary.NormalReturnFacts.PathInvalidations, staticMembers)
		}
		if _, ok := unwrap.Alias(memberType).(*typ.Function); !ok {
			t.Fatalf("data member = %v, want function", memberType)
		}
		return
	}
	t.Fatal("self:data call not found")
}

func TestMetatableMethodBoundaryReadKeepsPayloadAny(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local Worker = {}
Worker.__index = Worker

function Worker:prepare(payload: any): (any, string?)
	return { prepared = payload }, nil
end

function Worker:dispatch(payload: any): (boolean, string?)
	local prepared, err = self:prepare(payload)
	if err then
		return false, err
	end
	return prepared ~= nil, nil
end

function Worker:run(payload: any): boolean
	local ok, err = self:dispatch(payload)
	if err then
		return false
	end
	return ok
end

local function new_worker()
	return setmetatable({}, Worker)
end
`)
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"setmetatable"},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var runResult *body.Result
	for _, child := range result.RootResult().FunctionResults() {
		origin, ok := result.RootResult().FunctionOrigin(child.Function())
		if ok && origin.Kind == bind.FunctionOriginMethod && origin.Method == "run" {
			runResult = child
			break
		}
	}
	if runResult == nil {
		t.Fatal("run result missing")
	}
	for _, point := range runResult.Graph().RPO() {
		site, ok := runResult.CallSiteView(point)
		methodPath, hasMethodPath := site.MethodPath()
		if !ok || site.MethodName() != "dispatch" || !hasMethodPath {
			continue
		}
		value, ok := runResult.PathValueAtBoundary(point, methodPath)
		if !ok {
			t.Fatalf("PathValueAtBoundary(%s) returned false", methodPath.Key())
		}
		memberType, ok := typevalue.TypeOf(reg, value)
		if !ok {
			t.Fatalf("dispatch value has no type: %#v", value)
		}
		fn, ok := unwrap.Alias(memberType).(*typ.Function)
		if !ok {
			t.Fatalf("dispatch member = %v, want function", memberType)
		}
		if len(fn.Params) != 2 || fn.Params[0].Name != "self" || fn.Params[1].Name != "payload" || !typ.IsAny(fn.Params[1].Type) {
			t.Fatalf("dispatch boundary params = %#v, want self plus payload:any", fn.Params)
		}
		outcome, ok := runResult.CallOutcomeAt(point)
		if !ok {
			t.Fatalf("CallOutcomeAt(%d) returned false", point)
		}
		if len(outcome.ParamObligations) != 0 {
			t.Fatalf("dispatch call param obligations = %#v, want none for payload:any", outcome.ParamObligations)
		}
		return
	}
	t.Fatal("self:dispatch call missing")
}

func TestMethodCallInvalidationKeepsUnrelatedSelfMemberState(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local node = {}
local methods = {}
local mt = { __index = methods }

type Target = {
	id: string,
}

type NodeInstance = {
	targets: {Target},
	metadata: {[string]: unknown},
}

function node.new()
	local instance: NodeInstance = {
		targets = {},
		metadata = {},
	}
	return setmetatable(instance, mt)
end

function methods:update_metadata(updates)
	for k, v in pairs(updates) do
		self.metadata[k] = v
	end
	return self, nil
end

function methods:complete(extra_metadata)
	if extra_metadata then
		local _, err = self:update_metadata(extra_metadata)
		if err then
			return nil
		end
	end
	local out = table.create(#self.targets, 0)
	return out
end
`)
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"setmetatable"},
			Signatures: signaturelookup.Source{
				IncludeStdlib: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var complete *body.Result
	for _, child := range result.RootResult().FunctionResults() {
		origin, ok := result.RootResult().FunctionOrigin(child.Function())
		if ok && origin.Kind == bind.FunctionOriginMethod && origin.Method == "complete" {
			complete = child
			break
		}
	}
	if complete == nil {
		t.Fatal("complete result missing")
	}
	var calls []string
	targetType := typetable.NewRecord().Field("id", typ.String).Build()
	targetsType := typ.NewArray(targetType)
	for _, point := range complete.Graph().RPO() {
		site, ok := complete.CallSiteView(point)
		if !ok {
			continue
		}
		calls = append(calls, fmt.Sprintf("%d:%s:%s", point, site.MethodName(), site.CalleePathRef().String()))
		if !strings.Contains(site.CalleePathRef().String(), "table.create") {
			continue
		}
		var argValueText string
		if site, ok := complete.CallSiteView(point); ok {
			if source, ok := site.ArgumentSourceAt(0); ok {
				argValue, _ := complete.SourceValueBeforeBoundary(point, source)
				argType, argTypeOK := typevalue.TypeOf(reg, argValue)
				if !argTypeOK || !typ.TypeEquals(argType, typ.Integer) {
					argValueText = fmt.Sprintf("arg0=%v/%v value=%#v source=%#v", argType, argTypeOK, argValue, source)
				}
			}
		}
		selfPath := path.Path{Root: "self", Symbol: selfSymbol(t, complete), Version: 1}
		value, ok := complete.PathValueBeforeBoundary(point, selfPath.Field("targets"))
		if !ok {
			t.Fatal("self.targets value missing before table.create")
		}
		got, ok := typevalue.TypeOf(reg, value)
		if !ok || !typ.TypeEquals(got, targetsType) {
			t.Fatalf("self.targets type = %v/%v, want %v; %s", got, ok, targetsType, argValueText)
		}
		outcome, outcomeExact := complete.CallOutcomeAt(point)
		if !outcomeExact || len(outcome.Results) != 1 || outcome.Results[0].Index != 0 ||
			!outcome.PostReturnAuthority || outcome.SuspensionKnown || outcome.MaySuspend {
			t.Fatalf("table.create CallOutcome = %#v/%v, want exact authoritative allocation", outcome, outcomeExact)
		}
		allocationID, identityExact := identityvalue.ExactID(reg, outcome.Results[0].Value)
		object, objectExact := outcome.HeapTableObjects[allocationID]
		allocationPlacement, placementExact := outcome.Placements[allocationID]
		if !identityExact || !objectExact || !placementExact || len(outcome.HeapTableObjects) != 1 || len(outcome.Placements) != 1 ||
			!product.Equal(reg, object.Root(), outcome.Results[0].Value) || allocationPlacement == placement.Bottom {
			t.Fatalf("table.create allocation result/object/placement diverged: id=%#v/%v object=%#v/%v placement=%v/%v", allocationID, identityExact, object, objectExact, allocationPlacement, placementExact)
		}
		return
	}
	t.Fatalf("table.create call missing; calls=%v", calls)
}

func selfSymbol(t *testing.T, result *body.Result) symbol.ID {
	t.Helper()
	for _, slot := range result.FunctionParamSlots(result.Function()) {
		if slot.ImplicitSelf {
			return slot.Symbol
		}
	}
	t.Fatal("implicit self slot missing")
	return 0
}

func TestRunChunkDotCalledMethodStillRequiresExplicitSelf(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local methods = {}
local mt = { __index = methods }
local node = {}

type NodeInstance = {
	id: string,
}

function node.new()
	local instance: NodeInstance = { id = "root" }
	return setmetatable(instance, mt)
end

function methods:touch()
	return self.id
end

methods.touch()
`)
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"setmetatable"},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	diags := diagnostics.Produce(root)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly missing self diagnostic", diags)
	}
	if diags[0].Code != diagnostics.CodeDirectCallTooFewArgs {
		t.Fatalf("diagnostic code = %s, want %s: %#v", diags[0].Code, diagnostics.CodeDirectCallTooFewArgs, diags[0])
	}
	if !strings.Contains(diags[0].Message, "methods.touch expects 1 argument, got 0") {
		t.Fatalf("diagnostic message = %q, want explicit self arity evidence", diags[0].Message)
	}
}

func TestRunChunkDotCalledMethodChecksExplicitSelfType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local methods = {}
local mt = { __index = methods }
local node = {}

type NodeInstance = {
	id: string,
}

local function sink(value: NodeInstance)
end

function node.new()
	local instance: NodeInstance = { id = "root" }
	return setmetatable(instance, mt)
end

function methods:touch()
	sink(self)
end

methods.touch({ id = 42 })
`)
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry: reg,
			Globals:  []string{"setmetatable"},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	diags := diagnostics.Produce(root)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly explicit self type diagnostic", diags)
	}
	if diags[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s: %#v", diags[0].Code, diagnostics.CodeDirectCallArgType, diags[0])
	}
	if !strings.Contains(diags[0].Message, "argument 1.id is") || !strings.Contains(diags[0].Message, "not string") {
		t.Fatalf("diagnostic message = %q, want explicit self type mismatch", diags[0].Message)
	}
	evidence := diags[0].Explanation.Evidence()
	if len(evidence) < 2 || !strings.Contains(evidence[1].Message, "methods.touch parameter 1.id expects string") {
		t.Fatalf("diagnostic evidence = %#v, want declared explicit self parameter evidence", evidence)
	}
}

func TestRunChunkKeysAreScopedToOwningBody(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local methods = {}
local mt = { __index = methods }
local node = {}

type NodeInstance = {
	id: string,
	_queued_commands: unknown[],
}

local function accept_node(value: NodeInstance)
end

function methods:check_self()
	accept_node(self)
	return true
end

function methods:seed_context()
	return methods.check_self(self)
end

function methods:stdlib_calls(definitions)
	self._queued_commands = table.create(10, 0)
	for _, definition in ipairs(definitions) do
		self._queued_commands[#self._queued_commands + 1] = definition
	end
	return self._queued_commands
end

function node.new()
	local instance: NodeInstance = {
		id = "root",
		_queued_commands = {},
	}
	return setmetatable(instance, mt)
end

local instance: NodeInstance = node.new()
methods.seed_context(instance)
methods.stdlib_calls(instance, { "a", "b" })
`)
	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:   reg,
			Globals:    []string{"setmetatable"},
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := result.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	if diags := diagnostics.Produce(root); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want stdlib calls isolated from method context keys", diags)
	}
}

func TestRunChunkUsesExactConfiguredRootKey(t *testing.T) {
	reg := standard.Registry()
	want := product.Top()
	stmts := parseChunk(t, "return x + 1")
	rootKey := summary.SummaryKey{
		Ref:   ref.FuncRef{Kind: ref.KindRoot, ID: 42},
		Entry: summary.EntryKey{Values: 1, Facts: 2, References: 3},
	}

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:        reg,
			Globals:         []string{"x"},
			ExpressionValue: fixedExpressionValue(want),
		},
		RootKey: rootKey,
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}

	assertSummaryReturn(t, reg, result.Snapshot(), rootKey, want)
	if got, ok := result.Snapshot().Read(summary.DefaultSummaryKey(ref.Root())); ok {
		t.Fatalf("default root summary = %#v, want missing exact key", got)
	}
}

func fixedExpressionValue(value product.Value) func(cfg.Point, factflow.ExprRef, factflow.ValueSource, state.State) (product.Value, bool) {
	return func(cfg.Point, factflow.ExprRef, factflow.ValueSource, state.State) (product.Value, bool) {
		return value, true
	}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "fixpoint_program_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func onlyFunctionOrigin(t *testing.T, bindings *bind.Result) bind.FunctionOrigin {
	t.Helper()
	origins := bindings.FunctionOrigins()
	if len(origins) != 1 {
		t.Fatalf("FunctionOrigins length = %d, want 1: %#v", len(origins), origins)
	}
	return origins[0]
}

func functionOriginByName(t *testing.T, bindings *bind.Result, name string) bind.FunctionOrigin {
	t.Helper()
	for _, origin := range bindings.FunctionOrigins() {
		if origin.HasTargetSymbol && bindings.Name(origin.TargetSymbol) == name {
			return origin
		}
	}
	t.Fatalf("function origin %q not found: %#v", name, bindings.FunctionOrigins())
	return bind.FunctionOrigin{}
}

func mustBoundLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := bindings.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("bound local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("bound local symbol at %d is zero", index)
	}
	return locals[index]
}

func mustResultLocalAt(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := result.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("result local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("result local symbol at %d is zero", index)
	}
	return locals[index]
}

func mustFindLocalAssign(t *testing.T, stmts []ast.Stmt, name string) *ast.LocalAssignStmt {
	t.Helper()
	if stmt := findLocalAssign(stmts, name); stmt != nil {
		return stmt
	}
	t.Fatalf("local assignment for %q not found", name)
	return nil
}

func findLocalAssign(stmts []ast.Stmt, name string) *ast.LocalAssignStmt {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.LocalAssignStmt:
			for _, got := range s.Names {
				if got == name {
					return s
				}
			}
		case *ast.IfStmt:
			if found := findLocalAssign(s.Then, name); found != nil {
				return found
			}
			if found := findLocalAssign(s.Else, name); found != nil {
				return found
			}
		case *ast.DoBlockStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.WhileStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.RepeatStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.NumberForStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		case *ast.GenericForStmt:
			if found := findLocalAssign(s.Stmts, name); found != nil {
				return found
			}
		}
	}
	return nil
}

func requireCallArgumentByCalleeName(t *testing.T, result *body.Result, name string, index int) (cfg.Point, ast.Expr) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("result graph missing")
	}
	for _, point := range graph.RPO() {
		site, ok := result.CallSiteView(point)
		if !ok || site.CalleeSymbol() == 0 || result.SymbolName(site.CalleeSymbol()) != name {
			continue
		}
		fact, ok := result.SourceCall(point)
		if !ok || fact.Call == nil || index < 0 || index >= len(fact.Call.Args) {
			t.Fatalf("call %q arg index %d out of range", name, index)
		}
		return point, fact.Call.Args[index]
	}
	t.Fatalf("call %q not found", name)
	return 0, nil
}

func requireLocalAssignmentPoint(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) cfg.Point {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("result graph missing")
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point
		}
	}
	t.Fatalf("local assignment point for %v[%d] not found", stmt.Names, index)
	return 0
}

func assertBoundarySymbolWitnessClosed(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	id symbol.ID,
	name string,
) {
	t.Helper()
	value, ok := result.SymbolValueAtBoundary(point, id)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", name, point)
	}
	gotType, ok := structuralTypeFromBoundaryValue(reg, value)
	if !ok || typ.IsAny(gotType) || typ.IsUnknown(gotType) {
		t.Fatalf("%s boundary value = %#v, want concrete instantiated result evidence", name, value)
	}
	if refinement.ContainsFreeTypeParam(gotType) {
		t.Fatalf("%s structural type = %v, want no free type params", name, gotType)
	}
}

func structuralTypeFromBoundaryValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			return t, true
		}
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	return variant.TypeFromOrigin(origin.Family(), origin.Cases())
}

func assertBoundaryExprRuntimeKind(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	expr ast.Expr,
	want runtimekind.Value,
	label string,
) {
	t.Helper()
	value, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", label, point)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, want) {
		t.Fatalf("%s runtime kind = %s, want %s (value %#v)", label, got, want, value)
	}
}

func assertBoundarySymbolType(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	id symbol.ID,
	want typ.Type,
	label string,
) {
	t.Helper()
	value, ok := result.SymbolValueAtBoundary(point, id)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", label, point)
	}
	gotType, typeOK := structuralTypeFromBoundaryValue(reg, value)
	if !typeOK || !typ.TypeEquals(gotType, want) {
		t.Fatalf("%s structural type = %v, want %v (value %#v)", label, gotType, want, value)
	}
}

func assertBoundarySymbolConcreteType(
	t *testing.T,
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	id symbol.ID,
	label string,
) {
	t.Helper()
	value, ok := result.SymbolValueAtBoundary(point, id)
	if !ok {
		t.Fatalf("%s boundary value missing at %v", label, point)
	}
	gotType, typeOK := structuralTypeFromBoundaryValue(reg, value)
	if !typeOK || typ.IsAny(gotType) || typ.IsUnknown(gotType) || typ.IsNever(gotType) {
		t.Fatalf("%s structural type = %v/%v, want concrete (value %#v)", label, gotType, typeOK, value)
	}
}

func findNestedLocalByName(t *testing.T, root *body.Result, name string) (*body.Result, cfg.Point, symbol.ID) {
	t.Helper()
	if root == nil {
		t.Fatalf("root result missing")
	}
	for _, child := range root.FunctionResults() {
		if child == nil || child.Graph() == nil {
			continue
		}
		for _, point := range child.Graph().RPO() {
			fact, ok := child.LocalAssignment(point)
			if !ok || fact.Name != name || !fact.HasSymbol || fact.Symbol == 0 {
				continue
			}
			return child, point, fact.Symbol
		}
	}
	t.Fatalf("nested local assignment %q not found", name)
	return nil, 0, 0
}

func findNestedReceiverCall(t *testing.T, root *body.Result, method string) (*body.Result, cfg.Point, path.Path) {
	t.Helper()
	if root == nil {
		t.Fatalf("root result missing")
	}
	for _, child := range root.FunctionResults() {
		if child == nil || child.Graph() == nil {
			continue
		}
		for _, point := range child.Graph().RPO() {
			site, ok := child.CallSiteView(point)
			receiverPath, hasReceiverPath := site.ReceiverPath()
			if !ok || site.MethodName() != method || !hasReceiverPath || receiverPath.Symbol == 0 {
				continue
			}
			return child, point, receiverPath
		}
	}
	t.Fatalf("nested method call %q not found", method)
	return nil, 0, path.Path{}
}

func stateHasPathKeyMembership(ks *keyspace.KeySpace, st state.State, keySymbol, tableSymbol symbol.ID) bool {
	if ks == nil || keySymbol == 0 || tableSymbol == 0 {
		return false
	}
	for _, membership := range st.KeyMembershipsSnapshot().Memberships {
		if membership.Kind != state.KeyMembershipPath {
			continue
		}
		key, keyOK := ks.FromStateKey(membership.Key.PathKey())
		table, tableOK := ks.FromStateKey(membership.Table.PathKey())
		if keyOK && tableOK && key.Sym == keySymbol && table.Sym == tableSymbol {
			return true
		}
	}
	return false
}

func capturedSymbolByName(t *testing.T, bindings *bind.Result, fn *ast.FunctionExpr, name string) symbol.ID {
	t.Helper()
	for _, capture := range bindings.DirectCaptures(fn) {
		if capture.CapturedName == name && capture.Captured != 0 {
			return capture.Captured
		}
	}
	t.Fatalf("capture %q not found: %#v", name, bindings.DirectCaptures(fn))
	return 0
}

func requireCallByCalleeName(t *testing.T, result *body.Result, name string) cfg.Point {
	t.Helper()
	if result == nil || result.Graph() == nil {
		t.Fatalf("result graph missing")
	}
	for _, point := range result.Graph().RPO() {
		site, ok := result.CallSiteView(point)
		if !ok || site.CalleeSymbol() == 0 || result.SymbolName(site.CalleeSymbol()) != name {
			continue
		}
		return point
	}
	t.Fatalf("call %q not found", name)
	return 0
}

func findRootLocalPath(t *testing.T, result *body.Result, name string) path.Path {
	t.Helper()
	if result == nil || result.Graph() == nil {
		t.Fatalf("root result missing")
	}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || fact.Name != name || !fact.HasSymbol || fact.Symbol == 0 {
			continue
		}
		return path.NewPath(fact.Symbol, name)
	}
	t.Fatalf("root local assignment %q not found", name)
	return path.Path{}
}

func findFunctionForPath(t *testing.T, bindings *bind.Result, stmts []ast.Stmt, want string) *ast.FunctionExpr {
	t.Helper()
	targets := collectFunctionPathTargets(bindings, stmts)
	for fn, p := range targets {
		if p.String() == want {
			return fn
		}
	}
	t.Fatalf("function path %q not found in %v", want, targets)
	return nil
}

func assertSummaryReturn(t *testing.T, reg *axis.Registry, snapshot summary.Snapshot, key summary.SummaryKey, want product.Value) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	if len(got.Returns) != 1 {
		t.Fatalf("summary %s returns = %d, want 1: %#v", key.Ref, len(got.Returns), got)
	}
	if !product.Equal(reg, got.Returns[0], want) {
		t.Fatalf("summary %s return = %v, want %v", key.Ref, got.Returns[0], want)
	}
}

func debugSummaryReturnTypes(reg *axis.Registry, snapshot summary.Snapshot) string {
	var b strings.Builder
	for _, entry := range snapshot.Entries() {
		fmt.Fprintf(&b, "%v entry=%+v returns=[", entry.Key.Ref, entry.Key.Entry)
		for i, value := range entry.Summary.Returns {
			if i > 0 {
				b.WriteString(", ")
			}
			t, ok := typevalue.TypeOf(reg, value)
			fmt.Fprintf(&b, "%v/%v", t, ok)
		}
		b.WriteString("]; ")
	}
	return b.String()
}

func assertSummaryEscapeEvent(
	t *testing.T,
	snapshot summary.Snapshot,
	key summary.SummaryKey,
	target path.Path,
	kind callboundary.EscapeEventKind,
	recursive bool,
) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	for _, event := range got.NormalReturnFacts.EscapeEvents {
		if event.Target.Equal(target) && event.Kind == kind && event.Recursive == recursive {
			return
		}
	}
	t.Fatalf("summary %s escape events = %#v, want target %s kind %d recursive=%v", key.Ref, got.NormalReturnFacts.EscapeEvents, target, kind, recursive)
}

func assertSummaryReturnedKeyedArrayProvenance(t *testing.T, snapshot summary.Snapshot, key summary.SummaryKey) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	var dynamicFactFound bool
	for _, fact := range got.NormalReturnFacts.DynamicIndexFacts {
		if fact.Table.Equal(path.Path{Root: "ret[0]"}) && fact.Site != "" && fact.Value.Admission == dynamicindex.AdmissionAdmitted {
			dynamicFactFound = true
			break
		}
	}
	if !dynamicFactFound {
		t.Fatalf("summary %s dynamic-index facts = %#v, want admitted ret[0] array write", key.Ref, got.NormalReturnFacts.DynamicIndexFacts)
	}
	if len(got.NormalReturnFacts.DynamicValueKeys) != 1 ||
		!got.NormalReturnFacts.DynamicValueKeys[0].Container.Equal(path.Path{Root: "ret[0]"}) ||
		!got.NormalReturnFacts.DynamicValueKeys[0].Table.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("summary %s dynamic value key facts = %#v, want ret[0] values proven as keys of $0", key.Ref, got.NormalReturnFacts.DynamicValueKeys)
	}
}

func assertSummaryNormalReturnParam(
	t *testing.T,
	reg *axis.Registry,
	snapshot summary.Snapshot,
	key summary.SummaryKey,
	index int,
	wantPresence presence.Value,
	wantKind runtimekind.Value,
) {
	t.Helper()
	got, ok := snapshot.Read(key)
	if !ok {
		t.Fatalf("summary %s missing", key.Ref)
	}
	if len(got.NormalReturnParams) <= index {
		t.Fatalf("summary %s normal return params = %d, want index %d: %#v", key.Ref, len(got.NormalReturnParams), index, got)
	}
	value := got.NormalReturnParams[index]
	if gotPresence := product.PresenceOf(value); !presence.Equal(gotPresence, wantPresence) {
		t.Fatalf("summary %s param %d presence = %s, want %s", key.Ref, index, gotPresence, wantPresence)
	}
	if gotKind := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(gotKind, wantKind) {
		t.Fatalf("summary %s param %d runtime kind = %s, want %s", key.Ref, index, gotKind, wantKind)
	}
}
