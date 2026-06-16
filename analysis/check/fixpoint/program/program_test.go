package program

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestProgramProductionUsesPreparedBodyAPI(t *testing.T) {
	src, err := os.ReadFile("program.go")
	if err != nil {
		t.Fatalf("ReadFile(program.go): %v", err)
	}
	if strings.Contains(string(src), "body.CheckBound") {
		t.Fatalf("program.go uses body.CheckBound*, want prepared statics with SolvePrepared")
	}
}

func TestRunBoundChunkStatsObservePreparedStaticReuse(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local f = function()
	return 1
end
return f()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	stats := Stats{}

	if _, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg},
		Stats: &stats,
	}); err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}

	prepares := stats.Body.StaticChunkPrepares + stats.Body.StaticFunctionPrepares
	if stats.Body.StaticChunkPrepares != 1 || stats.Body.StaticFunctionPrepares != 1 {
		t.Fatalf("body prepares = chunk:%d function:%d, want 1/1", stats.Body.StaticChunkPrepares, stats.Body.StaticFunctionPrepares)
	}
	if stats.Body.BodySolves <= prepares {
		t.Fatalf("BodySolves = %d, prepares = %d, want static reuse visible", stats.Body.BodySolves, prepares)
	}
	phaseSolves := stats.PrepassBodySolves + stats.SummaryBodySolves + stats.MaterializeBodySolves
	if phaseSolves != stats.Body.BodySolves {
		t.Fatalf("phase solves = %d, BodySolves = %d", phaseSolves, stats.Body.BodySolves)
	}
	if stats.PrepassBodySolves == 0 || stats.SummaryBodySolves == 0 || stats.MaterializeBodySolves == 0 {
		t.Fatalf("phase stats = prepass:%d summary:%d materialize:%d, want all populated", stats.PrepassBodySolves, stats.SummaryBodySolves, stats.MaterializeBodySolves)
	}
	if stats.Query.BodyInvocations == 0 || stats.Query.Solver.TransferCalls != stats.Query.BodyInvocations {
		t.Fatalf("query stats = %#v, want body invocations matching solver transfer calls", stats.Query)
	}
}

func TestRunBoundChunkUsesSuppliedBindIdentityForLocalCallee(t *testing.T) {
	reg := standard.Registry()
	want := product.Top()
	stmts := parseChunk(t, `
local f = function()
	return 1
end
return f()
`)
	local := stmts[0].(*ast.LocalAssignStmt)
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
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, stmts)
	if _, err := collectCallContextKeys(&keys, stmts, bindings, body.Config{Registry: reg}, nil); err != nil {
		t.Fatalf("collectCallContextKeys: %v", err)
	}
	if len(keys.contexts) == 0 {
		t.Fatalf("call contexts missing")
	}
	if !contextEntryHasFunctionIdentity(reg, keys.contexts) {
		t.Fatalf("call contexts lack captured path function identity")
	}

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
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, stmts)
	if _, err := collectCallContextKeys(&keys, stmts, bindings, body.Config{Registry: reg}, nil); err != nil {
		t.Fatalf("collectCallContextKeys: %v", err)
	}
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
	receivers := metatableMethodReceiverTypes(bindings, nil, stmts)
	methods := mustBoundLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	if got, ok := receivers[methods]; !ok || !subtype.IsSubtype(got, typetable.NewRecord().Field("id", typ.String).Build()) {
		t.Fatalf("metatable method receiver = %v/%v, want NodeInstance", got, ok)
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
	if diags := diagnostics.Produce(root); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want method self seeded from metatable factory", diags)
	}
}

func TestRunChunkCallContextKeysAreScopedToOwningBody(t *testing.T) {
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
	stmts := parseChunk(t, "return 1")
	rootKey := summary.SummaryKey{
		Ref:   ref.FuncRef{Kind: ref.KindRoot, ID: 42},
		Entry: summary.EntryKey{Values: 1, Facts: 2, References: 3},
	}

	result, err := RunChunk(stmts, Config{
		Check: body.Config{
			Registry:        reg,
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

func contextEntryHasFunctionIdentity(reg *axis.Registry, contexts []keyedFunction) bool {
	for _, context := range contexts {
		snapshot := context.entryState.PathRefinementsSnapshot()
		if snapshot.Top {
			continue
		}
		for _, value := range snapshot.Refinements {
			if _, ok := product.Get(reg, value, identity.Key).ID(); ok {
				return true
			}
		}
	}
	return false
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
