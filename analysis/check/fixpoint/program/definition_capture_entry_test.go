package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestDefinitionCaptureEntrySeedsCapturedStaticMembersAtFunctionEntryVersion(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local term = {}
term.spinner_frames = {"a", "b", "c"}
function term.spinner(index: integer): string
    return term.spinner_frames[1]
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatalf("prepareBoundChunkBodies: %v", err)
	}
	prepass, err := solvePrepared(prepared.root, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("solvePrepared(root): %v", err)
	}
	if _, err := collectCallContextKeys(&keys, stmts, bindings, body.Config{Registry: reg}, nil, prepared); err != nil {
		t.Fatalf("collectCallContextKeys: %v", err)
	}

	spinner, capture := capturedFunctionForTest(t, bindings)
	origin, ok := bindings.FunctionOrigin(spinner)
	if !ok {
		t.Fatal("spinner origin missing")
	}
	point, ok := definitionEntryPoint(prepass, origin)
	if !ok {
		t.Fatal("definition point missing")
	}
	if _, ok := prepass.StateAt(point); !ok {
		t.Fatal("caller state at definition point missing")
	}
	entry, entryKeys := keyedFunctionEntryForTest(t, keys.functions, spinner)
	rootValue := entry.ReadValue(reg, statekey.SymbolValue(capture.Captured))
	if _, ok := product.Get(reg, rootValue, identity.Key).ID(); !ok {
		t.Fatalf("captured root value = %#v, want heap identity", rootValue)
	}
	memberKey := pathdom.Path{
		Root:    capture.CapturedName,
		Symbol:  capture.Captured,
		Version: 1,
		Segments: []segment.Segment{{
			Kind: segment.SegmentField,
			Name: "spinner_frames",
		}},
	}.Key()
	if got, ok := entry.ReadPathStaticMember(entryKeys, memberKey); !ok || product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("entry static member %s missing: %#v/%v", memberKey, got, ok)
	}
}

func TestRunBoundChunkCapturedStaticArrayModuloIndexSourceIsString(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local term = {}
term.spinner_frames = {"a", "b", "c"}
function term.spinner(index: integer): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return frame
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	child, point, _ := findNestedLocalByName(t, result.RootResult(), "frame")
	source, ok := child.LoweredLocalAssignment(point)
	if !ok {
		t.Fatal("lowered frame assignment missing")
	}
	dyn, ok := child.DynamicIndexExpressionRef(source.Source().ExprRef)
	if !ok {
		t.Fatal("frame dynamic index missing")
	}
	if _, ok := child.PathValueAtBoundary(point, dyn.TablePath()); !ok {
		t.Fatal("frame dynamic table value missing")
	}
	if keyValue, ok := child.SourceValueAtBoundary(point, dyn.KeySource()); !ok {
		t.Fatal("frame dynamic key value missing")
	} else if keyType, typeOK := typevalue.TypeOf(reg, keyValue); !typeOK || !subtype.IsSubtype(keyType, typ.Integer) {
		t.Fatalf("frame dynamic key type = %v/%v, want integer", keyType, typeOK)
	}
	value, ok := child.SourceValueAtBoundary(point, source.Source())
	if !ok {
		t.Fatal("frame source value missing")
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typeexpr.Optional(typeexpr.Union(typ.LiteralString("a"), typ.LiteralString("b"), typ.LiteralString("c")))) {
		t.Fatalf("frame source type = %v/%v, want optional literal union", got, ok)
	}
	semantic, ok := child.LocalAssignment(point)
	if !ok || !semantic.HasSymbol {
		t.Fatal("semantic frame assignment symbol missing")
	}
	boundary, ok := child.StateAtBoundary(point)
	if !ok {
		t.Fatal("frame boundary state missing")
	}
	assigned := boundary.ReadValue(reg, statekey.SymbolValue(semantic.Symbol))
	if product.Equal(reg, assigned, product.Bottom(reg)) {
		t.Fatal("assigned frame value missing")
	}
	assignedType, ok := typevalue.TypeOf(reg, assigned)
	if !ok || !subtype.IsSubtype(assignedType, typ.String) {
		t.Fatalf("assigned frame type = %v/%v, want subtype of string", assignedType, ok)
	}
}

func TestRunBoundChunkCapturedTableInsertSurvivesCallContextRefresh(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local suites = {}

local function register(name: string)
    table.insert(suites, {name = name, count = 0})
end

local function run()
    for _, s in ipairs(suites) do
        local n: string = s.name
    end
end

register("a")
run()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	runResult, point, _ := findNestedLocalByName(t, result.RootResult(), "n")
	source, ok := runResult.LoweredLocalAssignment(point)
	if !ok {
		t.Fatal("lowered n assignment missing")
	}
	value, ok := runResult.SourceValueAtBoundary(point, source.Source())
	if !ok {
		t.Fatal("n source value missing")
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !subtype.IsSubtype(got, typ.String) {
		t.Fatalf("s.name source type = %v/%v, want string", got, ok)
	}
}

func TestRunBoundChunkExportedFieldFunctionUsesBodyParamObligationBeforeCapture(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type HTTP = {
    get: (url: string, options: table) -> ()
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local client = {}

function client.request(endpoint_path): ()
    local base_url = "https://api.example.test"
    local full_url = base_url .. endpoint_path
    local function send_once()
        return http.get(full_url, {})
    end
    send_once()
end

return client
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	request, point, fullURL := findNestedLocalByName(t, result.RootResult(), "full_url")
	value, ok := request.SymbolValueAtBoundary(point, fullURL)
	if !ok {
		t.Fatalf("full_url boundary value missing at %v", point)
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !subtype.IsSubtype(got, typ.String) {
		requestFn := findFunctionForPath(t, bindings, stmts, "client.request")
		slots := bindings.ParamSlots(requestFn)
		if len(slots) == 0 {
			t.Fatal("client.request param slots missing")
		}
		paramValue, _ := request.SymbolValueAtBoundary(point, slots[0].Symbol)
		paramType, paramOK := typevalue.TypeOf(reg, paramValue)
		fnSym, _ := bindings.FunctionSymbol(requestFn)
		fnKey := result.functionKeys[fnSym]
		sum, sumOK := result.snapshot.Read(fnKey)
		obligationType := typ.Type(nil)
		obligationOK := false
		if sumOK && len(sum.ParamObligations) != 0 {
			obligationType, obligationOK = typevalue.TypeOf(reg, sum.ParamObligations[0])
		}
		t.Fatalf("full_url type = %v/%v, want string from endpoint_path body obligation; endpoint_path type at same point = %v/%v; summary obligation = %v/%v (summary %v key %v)", got, ok, paramType, paramOK, obligationType, obligationOK, sumOK, fnKey)
	}
	sendOnce := findLocalFunctionByName(t, bindings, "send_once")
	var sendOnceResult *body.Result
	for _, child := range request.FunctionResults() {
		if child.Function() == sendOnce {
			sendOnceResult = child
			break
		}
	}
	if sendOnceResult == nil {
		t.Fatal("materialized send_once result missing below client.request")
	}
	entry, ok := sendOnceResult.EntryState()
	if !ok {
		t.Fatal("send_once entry state missing")
	}
	entryValue := entry.ReadValue(reg, statekey.SymbolValue(fullURL))
	entryType, ok := typevalue.TypeOf(reg, entryValue)
	if !ok || !subtype.IsSubtype(entryType, typ.String) {
		origin, originOK := bindings.FunctionOrigin(sendOnce)
		defPoint, defOK := definitionEntryPoint(request, origin)
		defType := typ.Type(nil)
		defTypeOK := false
		if originOK && defOK {
			if defState, stateOK := request.StateAtBoundary(defPoint); stateOK {
				defType, defTypeOK = typevalue.TypeOf(reg, defState.ReadValue(reg, statekey.SymbolValue(fullURL)))
			}
		}
		t.Fatalf("send_once captured full_url entry type = %v/%v, want string; definition point=%v/%v origin=%v def type=%v/%v", entryType, ok, defPoint, defOK, originOK, defType, defTypeOK)
	}
}

func TestCapturedFunctionEntryCarriesTransitiveClosureEnvironment(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local contract = {}
local open_binding = function()
    return contract
end
local callback = function()
    return open_binding()
end
	`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	var contractSym symbol.ID
	var openSym symbol.ID
	var callback *ast.FunctionExpr
	for _, stmt := range stmts {
		assign, ok := stmt.(*ast.LocalAssignStmt)
		if !ok {
			continue
		}
		symbols := bindings.LocalSymbols(assign)
		for i, name := range assign.Names {
			if name == "contract" && i < len(symbols) {
				contractSym = symbols[i]
			}
			if name == "open_binding" && i < len(symbols) {
				openSym = symbols[i]
			}
			if name == "callback" && i < len(symbols) && i < len(assign.Exprs) {
				if fn, ok := assign.Exprs[i].(*ast.FunctionExpr); ok {
					callback = fn
				}
			}
		}
	}
	if contractSym == 0 || openSym == 0 || callback == nil {
		t.Fatalf("symbols contract=%d open=%d callback=%v", contractSym, openSym, callback != nil)
	}
	captures := bindings.DirectCaptures(callback)
	if len(captures) != 1 || captures[0].Captured != openSym {
		t.Fatalf("callback direct captures = %#v, want only open_binding", captures)
	}
	contractValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	contractValue = product.Set(reg, contractValue, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	openValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	openValue = product.Set(reg, openValue, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	openValue = product.Set(reg, openValue, identity.Key, identity.Singleton(identity.LuaFunction(uint64(openSym))))
	caller := state.State{}.
		WriteValue(reg, statekey.SymbolValue(contractSym), contractValue).
		WriteValue(reg, statekey.SymbolValue(openSym), openValue)

	entry, changed := applyCapturedUpvalueEntryState(reg, bindings, callback, caller, state.State{})
	if !changed {
		t.Fatal("applyCapturedUpvalueEntryState changed=false, want transitive closure seed")
	}
	got := entry.ReadValue(reg, statekey.SymbolValue(contractSym))
	if !product.Equal(reg, got, contractValue) {
		t.Fatalf("transitive contract capture = %#v, want %#v", got, contractValue)
	}
}

func TestRunBoundChunkReturnedAnonymousClosureCapturesParamTypes(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Response = { status: integer, body: string }
type ResponseResult = { ok: true, value: Response } | { ok: false, error: string }
type Decorator = (string) -> string

local function build(handler: () -> ResponseResult, decorator: Decorator?): () -> ResponseResult
    return function(): ResponseResult
        local result = handler()
        if decorator then
            return { ok = true, value = { status = result.value.status, body = decorator(result.value.body) } }
        end
        return result
    end
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	var returned *ast.FunctionExpr
	var handlerSym, decoratorSym symbol.ID
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Kind != bind.FunctionOriginLiteral || origin.Parent == nil {
			continue
		}
		for _, capture := range bindings.DirectCaptures(origin.Func) {
			switch capture.CapturedName {
			case "handler":
				handlerSym = capture.Captured
			case "decorator":
				decoratorSym = capture.Captured
			}
		}
		if handlerSym != 0 && decoratorSym != 0 {
			returned = origin.Func
			break
		}
	}
	if returned == nil {
		t.Fatal("returned anonymous function with handler/decorator captures not found")
	}
	result, err := RunBoundChunk(stmts, bindings, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	returnedResult := findMaterializedFunctionResult(t, result.RootResult(), returned)
	entry, ok := returnedResult.EntryState()
	if !ok {
		t.Fatal("returned anonymous function entry state missing")
	}
	handlerValue := entry.ReadValue(reg, statekey.SymbolValue(handlerSym))
	handlerType, ok := typevalue.TypeOf(reg, handlerValue)
	wantHandler := typ.Func().Returns(typeexpr.Union(
		typetable.NewRecord().Field("ok", typ.LiteralBool(true)).Field("value", typetable.NewRecord().Field("status", typ.Integer).Field("body", typ.String).Build()).Build(),
		typetable.NewRecord().Field("ok", typ.LiteralBool(false)).Field("error", typ.String).Build(),
	)).Build()
	if !ok || !typ.TypeEquals(handlerType, wantHandler) {
		t.Fatalf("captured handler type = %v/%v, want %v", handlerType, ok, wantHandler)
	}
	decoratorValue := entry.ReadValue(reg, statekey.SymbolValue(decoratorSym))
	decoratorType, ok := typevalue.TypeOf(reg, decoratorValue)
	wantDecorator := typeexpr.Optional(typ.Func().Param("value", typ.String).Returns(typ.String).Build())
	if !ok || !typ.TypeEquals(decoratorType, wantDecorator) {
		t.Fatalf("captured decorator type = %v/%v, want %v", decoratorType, ok, wantDecorator)
	}
}

func capturedFunctionForTest(t *testing.T, bindings *bind.Result) (*ast.FunctionExpr, bind.Capture) {
	t.Helper()
	for _, origin := range bindings.FunctionOrigins() {
		captures := bindings.DirectCaptures(origin.Func)
		if len(captures) != 0 {
			return origin.Func, captures[0]
		}
	}
	t.Fatal("no captured function found")
	return nil, bind.Capture{}
}

func findMaterializedFunctionResult(t *testing.T, root *body.Result, fn *ast.FunctionExpr) *body.Result {
	t.Helper()
	if root == nil {
		t.Fatal("missing materialized root result")
	}
	if root.Function() == fn {
		return root
	}
	for _, child := range root.FunctionResults() {
		if found := findMaterializedFunctionResultOrNil(child, fn); found != nil {
			return found
		}
	}
	t.Fatal("materialized function result not found")
	return nil
}

func findMaterializedFunctionResultOrNil(root *body.Result, fn *ast.FunctionExpr) *body.Result {
	if root == nil {
		return nil
	}
	if root.Function() == fn {
		return root
	}
	for _, child := range root.FunctionResults() {
		if found := findMaterializedFunctionResultOrNil(child, fn); found != nil {
			return found
		}
	}
	return nil
}

func keyedFunctionEntryForTest(t *testing.T, functions []keyedFunction, fn *ast.FunctionExpr) (state.State, *keyspace.KeySpace) {
	t.Helper()
	for _, candidate := range functions {
		if candidate.funcExpr != fn {
			continue
		}
		if !candidate.hasEntryState {
			t.Fatal("captured function has no entry state")
		}
		return candidate.entryState, candidate.entryKeys
	}
	t.Fatal("captured function key missing")
	return state.State{}, nil
}

func findLocalFunctionByName(t *testing.T, bindings *bind.Result, name string) *ast.FunctionExpr {
	t.Helper()
	for _, origin := range bindings.FunctionOrigins() {
		assign, ok := origin.Stmt.(*ast.LocalAssignStmt)
		if !ok || origin.LocalIndex < 0 || origin.LocalIndex >= len(assign.Names) {
			continue
		}
		if assign.Names[origin.LocalIndex] == name {
			return origin.Func
		}
	}
	t.Fatalf("local function %q not found", name)
	return nil
}
