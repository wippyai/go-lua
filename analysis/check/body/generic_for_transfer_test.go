package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestGenericForIPairsElementCarriesObjectLiteralAnyField(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function raw_value(): any
	return nil
end
local raw = raw_value()
local pages = {
	{ id = raw, route = "/ok" },
}
for _, page in ipairs(pages) do
	local id = page.id
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	var loop *ast.GenericForStmt
	for _, stmt := range stmts {
		if candidate, ok := stmt.(*ast.GenericForStmt); ok {
			loop = candidate
			break
		}
	}
	if loop == nil {
		t.Fatal("generic-for statement not found")
	}
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, local, 0)
	got, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0])
	if !ok {
		t.Fatal("ExpressionValueAtBoundary returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("page.id evidence = %s, want %s", gotEvidence, evidence.ExplicitTop())
	}
}

func TestGenericForIPairsElementCarriesObjectLiteralAnyFieldBeforeOrdinaryAssignment(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local raw: any = nil
local pages = {
	{ id = raw, route = "/ok" },
}
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	accessible[page.route] = page.id
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[3].(*ast.GenericForStmt)
	assign := loop.Stmts[0].(*ast.AssignStmt)
	point := requireOrdinaryAssignmentPoint(t, result, assign, 0)
	got, ok := result.ExpressionValueBeforeBoundary(point, assign.Rhs[0])
	if !ok {
		t.Fatal("ExpressionValueBeforeBoundary(page.id) returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("page.id evidence before assignment = %s, want %s", gotEvidence, evidence.ExplicitTop())
	}
}

func TestGenericForIPairsElementCarriesObjectLiteralGradualParameterField(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function f(raw)
	local pages = {
		{ id = raw, route = "/ok" },
	}
	for _, page in ipairs(pages) do
		local id = page.id
	end
end`)

	result, err := CheckFunction(fn, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	loop := fn.Stmts[1].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, local, 0)
	got, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0])
	if !ok {
		t.Fatal("ExpressionValueAtBoundary returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.GradualTop()) {
		t.Fatalf("page.id evidence = %s, want %s", gotEvidence, evidence.GradualTop())
	}
}

func TestGenericForPairsOverLocalDynamicMapCarriesInsertedRecordField(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local state = {
	active_sessions = {},
}
state.active_sessions["s1"] = {
	created_at = 1,
	last_activity = 2,
}
local function need_number(value: number): ()
end
for _, session_info in pairs(state.active_sessions) do
	need_number(session_info.created_at)
end`)

	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[3].(*ast.GenericForStmt)
	callStmt := loop.Stmts[0].(*ast.FuncCallStmt)
	call := callStmt.Expr.(*ast.FuncCallExpr)
	callPoint := cfg.Point(0)
	for _, candidate := range result.Graph().RPO() {
		view, ok := result.CallView(candidate)
		if !ok {
			continue
		}
		fact, _ := view.Borrowed()
		if fact.Call == call {
			callPoint = candidate
			break
		}
	}
	if callPoint == 0 {
		t.Fatal("need_number call point not found")
	}
	value, ok := result.ExpressionValueAtBoundary(callPoint, call.Args[0])
	if !ok {
		t.Fatal("ExpressionValueAtBoundary(session_info.created_at) returned false")
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.LiteralInt(1)) {
		t.Fatalf("session_info.created_at type = %v/%v, want 1 (value %#v)", got, ok, value)
	}
}

func TestGenericForIPairsTypeGuardClearsExplicitAnyElementEvidence(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function collect_strings(raw: any): {string}
	local out: {string} = {}
	if type(raw) ~= "table" then
		return out
	end
	for _, item in ipairs(raw) do
		if type(item) == "string" then
			table.insert(out, item)
		end
	end
	return out
end`)

	result, err := CheckFunction(fn, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
		Globals:    []string{"ipairs", "table", "type"},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	loop := fn.Stmts[2].(*ast.GenericForStmt)
	ifStmt := loop.Stmts[0].(*ast.IfStmt)
	callStmt := ifStmt.Then[0].(*ast.FuncCallStmt)
	call := callStmt.Expr.(*ast.FuncCallExpr)
	var point cfg.Point
	for _, candidate := range result.Graph().RPO() {
		view, ok := result.CallView(candidate)
		if !ok {
			continue
		}
		fact, _ := view.Borrowed()
		if fact.Call == call {
			point = candidate
			break
		}
	}
	if point == 0 {
		t.Fatal("table.insert call point not found")
	}
	itemArg := call.Args[1]
	itemPath, itemPathOK := result.ExpressionPath(itemArg)
	got, ok := result.ExpressionValueAtBoundary(point, itemArg)
	if !ok {
		if itemPathOK {
			if symValue, symOK := result.SymbolValueAtBoundary(point, itemPath.Symbol); symOK {
				t.Fatalf("ExpressionValueAtBoundary(item) returned false; path=%v/%v symbol=%v", itemPath, itemPathOK, symValue)
			}
		}
		t.Fatalf("ExpressionValueAtBoundary(item) returned false; path=%v/%v", itemPath, itemPathOK)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("item evidence = %s, want validated top", gotEvidence)
	}
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestGenericForIPairsUsesGenericArrayElementContract(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function map<T, U>(arr: {T}, fn: (T) -> U): {U}
	local out: {U} = {}
	for _, item in ipairs(arr) do
		local copy = item
	end
	return out
end`)

	result, err := CheckFunction(fn, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	loop := fn.Stmts[1].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	call := loop.Exprs[0].(*ast.FuncCallExpr)
	arrExpr := call.Args[0]
	point := requireLocalAssignmentPoint(t, result, local, 0)
	arrValue, arrOK := result.ExpressionValueAtBoundary(point, arrExpr)
	if !arrOK {
		t.Fatal("ExpressionValueAtBoundary for arr returned false")
	}
	arrType, arrTypeOK := typevalue.TypeOf(reg, arrValue)
	if !arrTypeOK || !typ.TypeEquals(arrType, typ.NewArray(typ.NewTypeParam("T", nil))) {
		t.Fatalf("arr type = %v/%v, want {T}", arrType, arrTypeOK)
	}
	assertExpressionTypeAtBoundary(t, reg, result, local, typ.NewTypeParam("T", nil))
}

func TestGenericForIPairsUsesEntryStateRootTypeForSegmentedSource(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function route(self)
	for _, target in ipairs(self.targets) do
		local id = target.node_id
	end
end`)
	bindings := bind.BindFunction(fn, bind.Options{Globals: []string{"ipairs"}})
	prepared, err := PrepareBoundFunction(fn, bindings, Config{
		Registry:   reg,
		Globals:    []string{"ipairs"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	self := mustParamSlot(t, bindings, fn, 0)
	targetType := typetable.NewRecord().Field("node_id", typ.String).Build()
	selfType := typetable.NewRecord().
		Field("targets", typ.NewArray(targetType)).
		Build()
	entry := state.State{}.WriteValue(reg, statekey.SymbolValue(self.Symbol), typevalue.WithWitness(reg, typevalue.FromType(reg, selfType), selfType))

	result, err := SolvePrepared(prepared, SolveConfig{EntryState: entry})
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}

	loop := fn.Stmts[0].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, local, typ.String)
}

func TestGenericForIPairsOptionalFieldFallbackIsString(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function collect(entries: {{ id: string, meta: { name: string? } }}): ()
	for i, entry in ipairs(entries) do
		local meta = entry.meta
		local display_name = meta.name or ("Unnamed test " .. i)
		local sink: string = display_name
	end
end`)
	bindings := bind.BindFunction(fn, bind.Options{Globals: []string{"ipairs"}})
	result, err := CheckBoundFunction(fn, bindings, Config{
		Registry:   reg,
		Globals:    []string{"ipairs"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	loop := fn.Stmts[0].(*ast.GenericForStmt)
	local := loop.Stmts[1].(*ast.LocalAssignStmt)
	sink := loop.Stmts[2].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, sink, 0)
	displayName := mustLocalAt(t, result, local, 0)
	got, ok := result.SymbolValueAtBoundary(point, displayName)
	if !ok {
		t.Fatal("display_name symbol value missing before sink assignment")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("display_name type = %v/%v, want string", gotType, ok)
	}
}

// TestGenericForLoopVarNegatedDiscriminantEdgeNarrowsRoot proves the else edge of
// a discriminant equality guard narrows an un-annotated generic-for loop variable
// to the complementary variant. The else-branch read item.payment_id must project
// the refund arm's required field as Present, not the union's optional string?.
func TestGenericForLoopVarNegatedDiscriminantEdgeNarrowsRoot(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Release = {kind: "release", token: string}
type Refund = {kind: "refund", payment_id: string}
type Compensation = Release | Refund

local items: {Compensation} = {}
for _, item in ipairs(items) do
	if item.kind == "release" then
		local token = item.token
	else
		local payment = item.payment_id
	end
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	var loop *ast.GenericForStmt
	for _, stmt := range stmts {
		if candidate, ok := stmt.(*ast.GenericForStmt); ok {
			loop = candidate
			break
		}
	}
	if loop == nil {
		t.Fatal("generic-for statement not found")
	}
	ifStmt := loop.Stmts[0].(*ast.IfStmt)
	elseLocal := ifStmt.Else[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, elseLocal, 0)
	got, ok := result.ExpressionValueAtBoundary(point, elseLocal.Exprs[0])
	if !ok {
		t.Fatal("ExpressionValueAtBoundary for item.payment_id returned false")
	}
	assertPresence(t, reg, got, presence.Present())
}

func TestGenericForPairsUsesAssertedIteratorSourceTypeForLoopVariables(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local transform_config: nil | string | {[string]: string} = {}
if type(transform_config) == "table" then
	for field_name, expression in pairs(transform_config :: {[string]: string}) do
		local field_copy = field_name
		local expression_copy = expression
	end
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	ifStmt := stmts[1].(*ast.IfStmt)
	loop := ifStmt.Then[0].(*ast.GenericForStmt)
	fieldLocal := loop.Stmts[0].(*ast.LocalAssignStmt)
	exprLocal := loop.Stmts[1].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, fieldLocal, typ.String)
	assertExpressionTypeAtBoundary(t, reg, result, exprLocal, typ.String)
}

func TestGenericForPairsUsesFlowRefinedIteratorSourceTypeForLoopVariables(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local transform_config: nil | string | {[string]: string} = {}
if type(transform_config) == "table" then
	for field_name, expression in pairs(transform_config) do
		local field_copy = field_name
		local expression_copy = expression
	end
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	ifStmt := stmts[1].(*ast.IfStmt)
	loop := ifStmt.Then[0].(*ast.GenericForStmt)
	fieldLocal := loop.Stmts[0].(*ast.LocalAssignStmt)
	exprLocal := loop.Stmts[1].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, fieldLocal, typ.String)
	assertExpressionTypeAtBoundary(t, reg, result, exprLocal, typ.String)
}

func TestGenericForIPairsUsesConfiguredBuiltinWithoutSignatureLookup(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local pages: {string} = {"alpha", "beta"}
for index, page in ipairs(pages) do
	local index_copy = index
	local page_copy = page
end`)

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
		Globals:  []string{"ipairs"},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[1].(*ast.GenericForStmt)
	indexLocal := loop.Stmts[0].(*ast.LocalAssignStmt)
	pageLocal := loop.Stmts[1].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, indexLocal, typ.Integer)
	assertExpressionTypeAtBoundary(t, reg, result, pageLocal, typ.String)
}

func TestGenericForPairsUsesConfiguredBuiltinWithoutSignatureLookup(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local transform_config: {[string]: string} = {}
for field_name, expression in pairs(transform_config) do
	local field_copy = field_name
	local expression_copy = expression
end`)

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
		Globals:  []string{"pairs"},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[1].(*ast.GenericForStmt)
	fieldLocal := loop.Stmts[0].(*ast.LocalAssignStmt)
	exprLocal := loop.Stmts[1].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, fieldLocal, typ.String)
	assertExpressionTypeAtBoundary(t, reg, result, exprLocal, typ.String)
}

func TestGenericForIPairsElementSupportsDiscriminantMemberProjection(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Usage = { input_tokens: number, output_tokens: number }
type DoneEvent = { type: "done", usage: Usage? }
type OtherEvent = { type: "other" }
type Event = DoneEvent | OtherEvent
local events: {Event} = {}
for _, event in ipairs(events) do
	if event.type == "done" then
		if event.usage then
			local usage = event.usage
		end
	end
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, expr := requireLocalAssignmentExprByName(t, result, "usage")
	got, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		t.Fatal("ExpressionValueAtBoundary returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	usage := typetable.NewRecord().
		Field("input_tokens", typ.Number).
		Field("output_tokens", typ.Number).
		Build()
	if !ok || !typ.TypeEquals(gotType, usage) {
		t.Fatalf("event.usage type = %v/%v, want Usage", gotType, ok)
	}
}

func TestGenericForLoopCarriesAnnotatedAccumulatorRecord(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Usage = { input_tokens: number, output_tokens: number }
type DoneEvent = { type: "done", usage: Usage? }
type OtherEvent = { type: "other" }
type Event = DoneEvent | OtherEvent
type Result = { usage: Usage }
local events: {Event} = {}
local usage: Usage = { input_tokens = 0, output_tokens = 0 }
for _, event in ipairs(events) do
	if event.type == "done" then
		if event.usage then
			usage = event.usage
		end
	end
end
local result: Result = { usage = usage }`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, expr := requireLocalAssignmentExprByName(t, result, "result")
	got, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		t.Fatal("ExpressionValueAtBoundary returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	usage := typetable.NewRecord().
		Field("input_tokens", typ.Number).
		Field("output_tokens", typ.Number).
		Build()
	want := typetable.NewRecord().Field("usage", usage).Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("result literal type = %v/%v, want Result", gotType, ok)
	}
}

func TestGenericForParamLoopCarriesAnnotatedAccumulatorRecord(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function process(events: {{ type: "done", usage: { input_tokens: number, output_tokens: number }? } | { type: "other" }}): ()
	local usage: { input_tokens: number, output_tokens: number } = { input_tokens = 0, output_tokens = 0 }
	for _, event in ipairs(events) do
		if event.type == "done" then
			if event.usage then
				usage = event.usage
			end
		end
	end
	local result: { usage: { input_tokens: number, output_tokens: number } } = { usage = usage }
end`)

	result, err := CheckFunction(fn, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	point, expr := requireLocalAssignmentExprByName(t, result, "result")
	got, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		t.Fatal("ExpressionValueAtBoundary returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	usage := typetable.NewRecord().
		Field("input_tokens", typ.Number).
		Field("output_tokens", typ.Number).
		Build()
	want := typetable.NewRecord().Field("usage", usage).Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("result literal type = %v/%v, want Result", gotType, ok)
	}
}

func TestGenericForPairsUsesNestedDeclaredMapTypeAfterDescendantWrites(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ActiveSession = {
	pid: any,
	created_at: number,
	terminating: boolean,
}

local state = {
	active_sessions = {} :: {[string]: ActiveSession},
}

local session_id = "s1"
state.active_sessions[session_id] = {
	pid = "pid",
	created_at = 1,
	terminating = false,
}
state.active_sessions[session_id] = nil

for id, session_info in pairs(state.active_sessions) do
	local id_copy = id
	local info_copy = session_info
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[5].(*ast.GenericForStmt)
	points := result.cfg.StmtPoints.PointsFor(loop)
	if len(points) == 0 {
		t.Fatalf("generic-for points = %v, want iterator call point", points)
	}
	site, ok := result.facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing iterator call site at point %d", points[0])
	}
	source, ok := site.ArgumentSourceAt(0)
	if !ok || !source.HasExpr {
		t.Fatalf("iterator source = %#v/%v, want expression source", source, ok)
	}
	sourcePath, ok := result.facts.ExpressionPath(source.ExprRef)
	if !ok {
		t.Fatalf("missing iterator source expression path for ref %d", source.ExprRef)
	}
	if _, ok := factquery.DominatingPathRootDeclarationSource(points[0], sourcePath, result.facts, result.Graph()); !ok {
		t.Fatalf("missing dominating declaration for iterator source path %v", sourcePath)
	}
	recovered, ok := genericForDominatingPathIteratorSourceType(transfer.NodeContext{
		Graph:    result.Graph(),
		Registry: reg,
		Point:    points[0],
		Node:     result.Graph().Node(points[0]),
		Read:     result.stateRead,
	}, result.typeValues, source, result.facts, result.visibility, result.boundarySources(sourceValueReadBoundary))
	if !ok {
		t.Fatalf("failed to recover declared iterator source type for %v", sourcePath)
	}
	wantSourceType := typetable.NewMap(typ.String, typetable.NewRecord().
		Field("pid", typ.Any).
		Field("created_at", typ.Number).
		Field("terminating", typ.Boolean).
		Build())
	if !typ.TypeEquals(recovered, wantSourceType) {
		t.Fatalf("recovered iterator source type = %v, want %v", recovered, wantSourceType)
	}
	idLocal := loop.Stmts[0].(*ast.LocalAssignStmt)
	infoLocal := loop.Stmts[1].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, idLocal, typ.String)
	assertExpressionTypeAtBoundary(t, reg, result, infoLocal, typetable.NewRecord().
		Field("pid", typ.Any).
		Field("created_at", typ.Number).
		Field("terminating", typ.Boolean).
		Build())
}

func TestGenericForIPairsUsesTableInsertElementTypeAfterCollectorLoop(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local source: {[string]: string} = {}
local out = {}

for id, value in pairs(source) do
	table.insert(out, id)
end

for _, id in ipairs(out) do
	local id_copy = id
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[3].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, local, typ.String)
}

func TestGenericForIPairsTableInsertObjectLiteralKeepsConstructedSiblingField(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local entries = {}
local failures = {}

for _, entry in ipairs(entries) do
	local label = "suite" .. "/" .. "name"
	table.insert(failures, { label = label, error = entry.error })
end

for _, f in ipairs(failures) do
	local label = f.label
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[3].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, local, 0)
	value, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0])
	if !ok {
		t.Fatal("ExpressionValueAtBoundary(f.label) returned false")
	}
	gotType, typeOK := result.ValueTypeWithPresence(value)
	if !typeOK || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("f.label type = %v/%v, want string; evidence=%s runtime=%s presence=%s",
			gotType, typeOK,
			product.Get(reg, value, evidence.Key),
			product.Get(reg, value, runtimekind.Key),
			product.PresenceOf(value))
	}
	if gotEvidence := product.Get(reg, value, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("f.label evidence = %s, want no untrusted top-origin evidence", gotEvidence)
	}
}

func TestGenericForTableInsertSeesAnnotatedAccumulatorClaim(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type TestEntry = { id: string, name: string }
local entries: any = nil
local tests: {TestEntry} = {}

for i, entry in ipairs(entries) do
	local display_name = entry.meta.name or ("Unnamed test " .. i)
	table.insert(tests, {
		id = entry.id :: string,
		name = display_name,
	})
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	testsSymbol := mustLocalAt(t, result, stmts[2].(*ast.LocalAssignStmt), 0)
	loop := stmts[3].(*ast.GenericForStmt)
	callStmt := loop.Stmts[1].(*ast.FuncCallStmt)
	var callPoint cfg.Point
	for _, point := range result.cfg.StmtPoints.PointsFor(callStmt) {
		if _, ok := result.facts.CallSite(point); ok {
			callPoint = point
			break
		}
	}
	if callPoint == 0 {
		t.Fatalf("missing table.insert call point for %v", result.cfg.StmtPoints.PointsFor(callStmt))
	}
	value, ok := result.PathValueBeforeBoundary(callPoint, pathdom.NewPath(testsSymbol, "tests"))
	if !ok {
		t.Fatalf("PathValueBeforeBoundary(tests) returned false")
	}
	if got := product.Get(reg, value, assertion.Key); !got.Has(assertion.TypeClaim) {
		t.Fatalf("tests assertion at table.insert = %s, want declared type claim; value=%v", got, value)
	}
}

func TestGenericForIPairsCarriesTableInsertKeyMembershipAfterCollectorLoop(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local source: {[string]: string} = {}
local out = {}

for id, value in pairs(source) do
	table.insert(out, id)
end

for _, id in ipairs(out) do
	local id_copy = id
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	source := mustLocalAt(t, result, stmts[0].(*ast.LocalAssignStmt), 0)
	loop := stmts[3].(*ast.GenericForStmt)
	id := result.bindings.GenericForSymbols(loop)[1]
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, local, 0)
	idKey, idOK := result.visibility.StateKeyAt(point, pathdom.NewPath(id, "id"))
	sourceKey, sourceOK := result.visibility.StateKeyAt(point, pathdom.NewPath(source, "source"))
	if !idOK || !sourceOK {
		t.Fatalf("state keys for id/source = %v/%v", idOK, sourceOK)
	}
	if !result.stateRead(point).HasPathKeyMembership(idKey, sourceKey) {
		t.Fatalf("id is not known as a key of source; memberships = %#v", result.stateRead(point).KeyMembershipsSnapshot())
	}
}

func TestGenericForPairsValueCarriesClosedStaticMemberUnionBeforeSelfWrite(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = value
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[1].(*ast.GenericForStmt)
	assign := loop.Stmts[0].(*ast.AssignStmt)
	point := requireOrdinaryAssignmentPoint(t, result, assign, 0)
	value, ok := result.ExpressionValueBeforeBoundary(point, assign.Rhs[0])
	if !ok {
		t.Fatal("ExpressionValueBeforeBoundary returned false for loop value")
	}
	got, ok := typevalue.TypeOf(reg, value)
	want := typ.MaterializeUnion([]typ.Type{typ.LiteralInt(1), typ.LiteralString("ready")})
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("loop value type = %v/%v, want %v", got, ok, want)
	}
	fact, ok := result.OrdinaryAssignment(point)
	if !ok {
		t.Fatal("missing ordinary assignment fact")
	}
	sourcePath, ok := result.ExpressionPath(fact.Value)
	if !ok {
		t.Fatal("missing assignment source path")
	}
	if !result.AssignmentSourcePathMatchesDynamicTargetRead(point, sourcePath) {
		t.Fatal("loop value is not recorded as read from the same dynamic target slot")
	}
}

func TestGenericForUnknownIteratorDoesNotSynthesizeLoopVariable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local transform_config: {[string]: string} = {}
for field_name, expression in iter(transform_config) do
	local field_copy = field_name
end`)

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[1].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, local, 0)
	if got, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0]); ok {
		t.Fatalf("ExpressionValueAtBoundary synthesized loop variable value = %v, want false", got)
	}
}

// TestGenericForStatelessFunctionIteratorNarrowsFirstVariable proves the loop
// variable of a stateless function iterator (for w in f do, where f returns the
// iterator fun(): string?) is typed from the iterator function's result, narrowed
// to its non-nil form for the first variable. gmatch returns fun(): string?, so w
// is string inside the body. This is the type that makes `local ok: string = w`
// check clean and `local n: number = w` report a type error.
func TestGenericForStatelessFunctionIteratorNarrowsFirstVariable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local s: string = "hello world"
for w in s:gmatch("%a+") do
	local copy = w
end`)

	result, err := CheckChunk(stmts, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	loop := stmts[1].(*ast.GenericForStmt)
	local := loop.Stmts[0].(*ast.LocalAssignStmt)
	assertExpressionTypeAtBoundary(t, reg, result, local, typ.String)
}

func assertExpressionTypeAtBoundary(t *testing.T, reg *axis.Registry, result *Result, local *ast.LocalAssignStmt, want typ.Type) {
	t.Helper()
	point := requireLocalAssignmentPoint(t, result, local, 0)
	got, ok := result.ExpressionValueAtBoundary(point, local.Exprs[0])
	if !ok {
		t.Fatalf("ExpressionValueAtBoundary returned false for %#v", local.Exprs[0])
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("expression type = %v/%v, want %v", gotType, ok, want)
	}
}
