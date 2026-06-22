package body

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCheckChunkRequiresRegistry(t *testing.T) {
	_, err := CheckChunk(nil, Config{})
	if !errors.Is(err, ErrRegistryRequired) {
		t.Fatalf("CheckChunk error = %v, want ErrRegistryRequired", err)
	}
}

func TestCheckChunkRejectsInvalidLifecycleManifest(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("lifecycle")
	m.DefineFunctionSignature("begin", signature.Function{
		Type: typ.Func().Param("tx", typ.Any).Build(),
		Effect: effect.Empty.With(lifecycle.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: "transaction",
			State:    "active",
		}),
	})
	_, err := CheckChunk(parseChunk(t, "begin({})"), Config{
		Registry: reg,
		Globals:  []string{"begin"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `lifecycle protocol "transaction" is not declared as a typestate FSM`) {
		t.Fatalf("CheckChunk error = %v, want invalid lifecycle manifest", err)
	}
}

func TestCheckChunkAcceptsDeclaredLifecycleManifest(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("lifecycle")
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    "transaction",
		States:      []typestate.State{"active", "finished"},
		FinalStates: []typestate.State{"finished"},
		Transitions: []typestate.TransitionDecl{{From: "active", To: "finished"}},
	}); err != nil {
		t.Fatalf("DefineTypestateProtocol: %v", err)
	}
	m.DefineFunctionSignature("finish", signature.Function{
		Type: typ.Func().Param("tx", typ.Any).Build(),
		Effect: effect.Empty.With(lifecycle.Transition{
			Target:   effect.ParamRef{Index: 0},
			Protocol: "transaction",
			From:     "active",
			To:       "finished",
		}),
	})
	if _, err := CheckChunk(parseChunk(t, "finish({})"), Config{
		Registry: reg,
		Globals:  []string{"finish"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	}); err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
}

func TestCheckChunkAssignsLocalFromExpressionValue(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, "local x = 1")
	want := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), markKey, markLow)

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return want, true
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	x := mustLocalAt(t, result, stmts[0].(*ast.LocalAssignStmt), 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got := exit.ReadValue(reg, key.SymbolValue(x))
	assertProductEqual(t, reg, got, want)
	if gotMark := product.Get(reg, got, markKey); gotMark != markLow {
		t.Fatalf("custom axis = %v, want %v", gotMark, markLow)
	}
}

func TestCheckChunkMissingUnionMapEntryUnderInferredReturnStaysOptional(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Task = {kind: "task", id: string}
type Timer = {kind: "timer", id: string}
type Envelope = Task | Timer
type State = {processed: {[string]: Envelope}, counters: {[string]: number}}
type Actor = {state: State}
local function new_actor(): Actor
	return {state = {processed = {}, counters = {}}}
end
local actor = new_actor()
actor.state.processed["m1"] = {kind = "task", id = "m1"}
actor.state.counters["task"] = 1
local missing_processed: Envelope = actor.state.processed["missing"]
local missing_counter: number = actor.state.counters["missing"]
`)
	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	for _, name := range []string{"missing_processed", "missing_counter"} {
		point, expr := requireLocalAssignmentExprByName(t, result, name)
		got, ok := result.ExpressionValueAtBoundary(point, expr)
		if !ok {
			t.Fatalf("%s ExpressionValueAtBoundary returned false", name)
		}
		assertPresence(t, reg, got, presence.Maybe())
	}
}

func TestAliasVariantWriteInvalidatesEquivalentGuardedProjection(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type FileSlot = {
	kind: "file",
	path: string,
}
type TimerSlot = {
	kind: "timer",
	seconds: number,
}
type Slot = {
	value: FileSlot | TimerSlot,
}
type Slots = {[string]: Slot}

local slots: Slots = {
	active = {
		value = {kind = "file", path = "/tmp/active"},
	},
}

local alias = slots.active

if alias.value.kind == "file" then
	local before: string = alias.value.path
	alias.value = {kind = "timer", seconds = 5}
	local stale_path: string = slots.active.value.path
	local stale_seconds: number = before
end
`)
	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, expr := requireLocalAssignmentExprByName(t, result, "stale_path")
	read, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		t.Fatalf("stale_path expr = %T, want AttrGetExpr", expr)
	}
	receiver, ok := result.ExpressionValueAtBoundary(point, read.Object)
	if !ok {
		t.Fatal("receiver ExpressionValueAtBoundary returned false")
	}
	receiverType, ok := typevalue.StructuralTypeOf(reg, result.typeValues, receiver, typevalue.StructuralTypeOptions{
		ApplyPresence: true,
	})
	if !ok {
		t.Fatalf("receiver type unavailable from value %v", receiver)
	}
	if _, ok := access.Field(receiverType, "seconds"); !ok {
		t.Fatalf("receiver type = %s, want timer field seconds", receiverType)
	}
	if _, ok := access.Field(receiverType, "path"); ok {
		t.Fatalf("receiver type = %s, still admits stale file field path", receiverType)
	}
	if stale, ok := result.ExpressionValueAtBoundary(point, expr); ok {
		staleType, typeOK := typevalue.StructuralTypeOf(reg, result.typeValues, stale, typevalue.StructuralTypeOptions{
			ApplyPresence: true,
		})
		if typeOK && subtype.IsSubtype(staleType, typ.String) {
			t.Fatalf("stale path read type = %s, want not assignable to string after alias variant write", staleType)
		}
	}
}

func TestNestedDynamicVariantWriteInvalidatesGuardedProjection(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type FileSlot = {
	kind: "file",
	path: string,
}
type TimerSlot = {
	kind: "timer",
	seconds: number,
}
type Slot = {
	value: FileSlot | TimerSlot,
}
type Slots = {[string]: Slot}

local slots: Slots = {
	active = {
		value = {kind = "file", path = "/tmp/active"},
	},
}
local key = "active"

if slots.active.value.kind == "file" then
	slots[key].value = {kind = "timer", seconds = 20}
	local stale_path: string = slots.active.value.path
end
`)
	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, expr := requireLocalAssignmentExprByName(t, result, "stale_path")
	stalePath, ok := result.ExpressionPath(expr)
	if !ok {
		t.Fatalf("stale_path expression path unavailable")
	}
	foundInvalidation := false
	for _, candidate := range result.Graph().RPO() {
		invalidation, ok := result.facts.PathDescendantInvalidation(candidate)
		if !ok {
			continue
		}
		if invalidation.ContainerPath().Root == "slots" {
			foundInvalidation = true
			break
		}
	}
	if !foundInvalidation {
		t.Fatalf("nested dynamic write did not publish descendant invalidation for slots")
	}
	if boundary, ok := result.boundaryStateAt(point); ok {
		for _, p := range []path.Path{stalePath.Parent().Parent(), stalePath.Parent(), stalePath} {
			if key := result.visibility.KeyAt(point, p); key != "" {
				if exact := boundary.ReadPathKey(reg, result.KeySpace(), key); !product.Equal(reg, exact, product.Bottom(reg)) {
					t.Fatalf("stale dynamic path ancestor key %s survived invalidation as %v", key, exact)
				}
			}
		}
		root := stalePath.RootOnly()
		rootValue := boundary.ReadValue(reg, key.SymbolValue(root.Symbol))
		if id, ok := product.Get(reg, rootValue, identity.Key).ID(); ok {
			if members := boundary.ReadHeapTableObject(reg, id).StaticMembers(); len(members) != 0 {
				t.Fatalf("stale dynamic root heap members survived invalidation: %#v", members)
			}
		}
	}
	if stale, ok := result.ExpressionValueAtBoundary(point, expr); ok {
		staleType, typeOK := typevalue.StructuralTypeOf(reg, result.typeValues, stale, typevalue.StructuralTypeOptions{
			ApplyPresence: true,
		})
		if typeOK && subtype.IsSubtype(staleType, typ.String) {
			t.Fatalf("stale dynamic path read type = %s, want not assignable to string after nested dynamic variant write", staleType)
		}
	}
}

func TestNestedDynamicVariantWriteInvalidatesAliasedProjection(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type FileSlot = {
	kind: "file",
	path: string,
}
type TimerSlot = {
	kind: "timer",
	seconds: number,
}
type Slot = {
	value: FileSlot | TimerSlot,
}
type Slots = {[string]: Slot}

local slots: Slots = {
	active = {
		value = {kind = "file", path = "/tmp/active"},
	},
}
local active = slots.active
local key = "active"

if active.value.kind == "file" then
	slots[key].value = {kind = "timer", seconds = 20}
	local stale_path: string = active.value.path
end
`)
	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, expr := requireLocalAssignmentExprByName(t, result, "stale_path")
	if stale, ok := result.ExpressionValueAtBoundary(point, expr); ok {
		staleType, typeOK := typevalue.StructuralTypeOf(reg, result.typeValues, stale, typevalue.StructuralTypeOptions{
			ApplyPresence: true,
		})
		if typeOK && subtype.IsSubtype(staleType, typ.String) {
			t.Fatalf("aliased stale dynamic path read type = %s, want not assignable to string after nested dynamic variant write", staleType)
		}
	}
}

func TestDynamicIndexWriteInvalidatesGuardedFieldProjection(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Box = {
	value: string?,
}

local box: Box = {value = "ready"}
local alias = box
local key = "value"

if box.value then
	alias[key] = nil
	local after: string = box.value
end
`)
	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, expr := requireLocalAssignmentExprByName(t, result, "after")
	if value, ok := result.ExpressionValueAtBoundary(point, expr); ok {
		got, typeOK := typevalue.StructuralTypeOf(reg, result.typeValues, value, typevalue.StructuralTypeOptions{
			ApplyPresence: true,
		})
		if typeOK && subtype.IsSubtype(got, typ.String) {
			t.Fatalf("guarded dynamic-index read type = %s, want not assignable to string after alias dynamic write", got)
		}
	}
}

func requireLocalAssignmentExprByName(t *testing.T, result *Result, name string) (cfg.Point, ast.Expr) {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || fact.Name != name || fact.Expr == nil {
			continue
		}
		return point, fact.Expr
	}
	t.Fatalf("local assignment %q not found", name)
	return 0, nil
}

func TestBoundaryReadsUseMaterializedNodeOutputs(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local x = f()`)
	callOutcomeCalls := 0

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
		Globals:  []string{"f"},
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			callOutcomeCalls++
			return callpayload.CallOutcome{Results: []callpayload.CallResult{{
				Index: 0,
				Value: typevalue.FromType(ctx.Registry, typ.String),
			}}}
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	if callOutcomeCalls == 0 {
		t.Fatal("call outcome provider was not exercised during analysis")
	}

	var point cfg.Point
	var found bool
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(candidate)
		if !ok || fact.Name != "x" {
			continue
		}
		point = candidate
		found = true
		if _, ok := result.LocalAssignmentSourceValueAtBoundary(candidate, fact.Source); !ok {
			t.Fatal("first boundary source read failed")
		}
		break
	}
	if !found {
		t.Fatal("local assignment for x not found")
	}
	callsAfterFirstRead := callOutcomeCalls
	fact, _ := result.LocalAssignment(point)
	if _, ok := result.LocalAssignmentSourceValueAtBoundary(point, fact.Source); !ok {
		t.Fatal("second boundary source read failed")
	}
	if callOutcomeCalls != callsAfterFirstRead {
		t.Fatalf("repeated boundary read called call outcome provider %d extra times", callOutcomeCalls-callsAfterFirstRead)
	}
}

func TestCheckFunctionWhileIndexReadCarriesRangeAndPositiveProofs(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function first(xs: {number}): number
	local i: number = 1
	while i <= #xs do
		local v: number = xs[i]
		if v > 0 then
			return v
		end
		i = i + 1
	end
	return 0
end`)

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	var branchPoint cfg.Point
	var branchFound bool
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.BranchCondition(candidate)
		if !ok || fact.While == nil {
			continue
		}
		branchPoint = candidate
		branchFound = true
		if fact.Check.Kind != branchcond.CheckIndexInRange {
			t.Fatalf("while check kind = %v, want CheckIndexInRange", fact.Check.Kind)
		}
		break
	}
	if !branchFound {
		t.Fatal("while branch condition not found")
	}
	var iAssign cfg.Point
	var iSymbol symbol.ID
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(candidate)
		if ok && fact.Name == "i" {
			iAssign = candidate
			iSymbol = fact.Symbol
			root, rootOK := result.facts.RootAssignment(candidate)
			if !rootOK {
				t.Fatalf("local i at point %d has no root assignment", candidate)
			}
			source := root.Source()
			if !source.HasExpr {
				t.Fatalf("local i root assignment source has no expr: %#v", source)
			}
			value, ok := result.facts.ExpressionValue(source.ExprRef)
			if !ok {
				t.Fatalf("local i source expr %d has no expression value", source.ExprRef)
			}
			if got, ok := typevalue.TypeOf(reg, value); !ok || !typ.TypeEquals(got, typ.LiteralInt(1)) {
				t.Fatalf("local i source expr type = %v/%v, want literal 1", got, ok)
			}
			break
		}
	}
	if iAssign == 0 {
		t.Fatal("local i assignment not found")
	}
	var incAssign cfg.Point
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != iSymbol {
			continue
		}
		incAssign = candidate
		root, rootOK := result.facts.RootAssignment(candidate)
		if !rootOK {
			t.Fatalf("increment i at point %d has no root assignment", candidate)
		}
		source := root.Source()
		if !source.HasExpr {
			t.Fatalf("increment source has no expr: %#v", source)
		}
		if _, ok := result.facts.ExpressionOperation(source.ExprRef); !ok {
			t.Fatalf("increment source expr %d has no expression operation", source.ExprRef)
		}
		break
	}
	if incAssign == 0 {
		t.Fatal("increment i assignment not found")
	}
	evidence := result.facts.BranchPathEvidence(branchPoint)
	if len(evidence) == 0 {
		t.Fatalf("branch %d lowered no branch path evidence", branchPoint)
	}
	branchState, _ := result.StateAt(branchPoint)
	if branchFloors := branchState.NumFloorsSnapshot(result.KeySpace()).Floors; len(branchFloors) == 0 {
		t.Fatalf("branch %d has no numeric floors before body", branchPoint)
	}
	if indexKey := result.visibility.KeyAt(branchPoint, evidence[0].Path()); indexKey == "" {
		t.Fatalf("branch %d index evidence path has no visibility key: %#v", branchPoint, evidence[0].Path())
	}
	otherPath, ok := evidence[0].OtherPath()
	if !ok {
		t.Fatalf("branch %d evidence has no array path: %#v", branchPoint, evidence[0])
	}
	if arrayKey := result.visibility.KeyAt(branchPoint, otherPath); arrayKey == "" {
		t.Fatalf("branch %d array proof path has no visibility key: %#v", branchPoint, otherPath)
	}
	var point cfg.Point
	var attr *ast.AttrGetExpr
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(candidate)
		if !ok || fact.Name != "v" {
			continue
		}
		got, ok := fact.Expr.(*ast.AttrGetExpr)
		if !ok {
			t.Fatalf("v source = %T, want indexed attr", fact.Expr)
		}
		point = candidate
		attr = got
		break
	}
	if attr == nil {
		t.Fatal("local v assignment not found")
	}
	arrayPath, ok := result.ExpressionPath(attr.Object)
	if !ok {
		t.Fatal("array expression path not found")
	}
	indexPath, ok := result.ExpressionPath(attr.Key)
	if !ok {
		t.Fatal("index expression path not found")
	}
	if !result.IndexInRangeAtBoundary(point, indexPath, arrayPath) {
		st, _ := result.StateAt(point)
		t.Fatalf("missing %s <= len(%s) proof at point %d from branch %d; proofs=%#v",
			indexPath, arrayPath, point, branchPoint, st.BranchProofsSnapshot(result.KeySpace()).Proofs)
	}
	floor, ok := result.NumericFloorAtBoundary(point, indexPath)
	if !ok || floor < 1 {
		st, _ := result.StateAt(point)
		t.Fatalf("index numeric floor = %d/%v, want >=1 at point %d; floors=%#v",
			floor, ok, point, st.NumFloorsSnapshot(result.KeySpace()).Floors)
	}
	if !result.IndexReadSafeAtBoundary(point, indexPath, 1, 0, arrayPath) {
		t.Fatalf("index read %s[%s] not marked safe at point %d despite range and positive proofs", arrayPath, indexPath, point)
	}
}

func TestCheckFunctionUpperBoundWithoutPositiveFloorDoesNotMarkIndexReadSafe(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function read(xs: {number}, i: number): ()
	if i <= #xs then
		local v: number? = xs[i]
	end
end`)

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	point, expr := requireLocalAssignmentExprByName(t, result, "v")
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		t.Fatalf("v source = %T, want indexed attr", expr)
	}
	arrayPath, ok := result.ExpressionPath(attr.Object)
	if !ok {
		t.Fatal("array expression path not found")
	}
	indexPath, ok := result.ExpressionPath(attr.Key)
	if !ok {
		t.Fatal("index expression path not found")
	}
	if !result.IndexInRangeAtBoundary(point, indexPath, arrayPath) {
		st, _ := result.StateAt(point)
		t.Fatalf("missing upper-bound proof for %s <= len(%s) at point %d; proofs=%#v",
			indexPath, arrayPath, point, st.BranchProofsSnapshot(result.KeySpace()).Proofs)
	}
	if floor, ok := result.NumericFloorAtBoundary(point, indexPath); ok && floor >= 1 {
		t.Fatalf("unexpected positive floor for %s at point %d: %d", indexPath, point, floor)
	}
	if result.IndexReadSafeAtBoundary(point, indexPath, 1, 0, arrayPath) {
		t.Fatalf("index read %s[%s] marked safe with upper-bound proof but no positive floor", arrayPath, indexPath)
	}
}

func TestCheckFunctionNumericForLengthLimitCarriesRangeAndPositiveProofs(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function labels(xs: {string})
	for i = 1, #xs do
		local v: string = xs[i]
	end
end`)

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	var point cfg.Point
	var attr *ast.AttrGetExpr
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(candidate)
		if !ok || fact.Name != "v" {
			continue
		}
		got, ok := fact.Expr.(*ast.AttrGetExpr)
		if !ok {
			t.Fatalf("v source = %T, want indexed attr", fact.Expr)
		}
		point = candidate
		attr = got
		break
	}
	if attr == nil {
		t.Fatal("local v assignment not found")
	}
	arrayPath, ok := result.ExpressionPath(attr.Object)
	if !ok {
		t.Fatal("array expression path not found")
	}
	indexPath, ok := result.ExpressionPath(attr.Key)
	if !ok {
		t.Fatal("index expression path not found")
	}
	if !result.IndexInRangeAtBoundary(point, indexPath, arrayPath) {
		st, _ := result.StateAt(point)
		t.Fatalf("missing %s <= len(%s) proof at point %d; proofs=%#v",
			indexPath, arrayPath, point, st.BranchProofsSnapshot(result.KeySpace()).Proofs)
	}
	floor, ok := result.NumericFloorAtBoundary(point, indexPath)
	if !ok || floor < 1 {
		st, _ := result.StateAt(point)
		t.Fatalf("index numeric floor = %d/%v, want >=1 at point %d; floors=%#v",
			floor, ok, point, st.NumFloorsSnapshot(result.KeySpace()).Floors)
	}
	if !result.IndexReadSafeAtBoundary(point, indexPath, 1, 0, arrayPath) {
		t.Fatalf("index read %s[%s] not marked safe at point %d despite range and positive proofs", arrayPath, indexPath, point)
	}
}

func TestCheckFunctionReverseNumericForLengthInitCarriesRangeAndPositiveProofs(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function labels(xs: {string})
	for i = #xs, 1, -1 do
		local v: string = xs[i]
	end
end`)

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	var point cfg.Point
	var attr *ast.AttrGetExpr
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(candidate)
		if !ok || fact.Name != "v" {
			continue
		}
		got, ok := fact.Expr.(*ast.AttrGetExpr)
		if !ok {
			t.Fatalf("v source = %T, want indexed attr", fact.Expr)
		}
		point = candidate
		attr = got
		break
	}
	if attr == nil {
		t.Fatal("local v assignment not found")
	}
	arrayPath, ok := result.ExpressionPath(attr.Object)
	if !ok {
		t.Fatal("array expression path not found")
	}
	indexPath, ok := result.ExpressionPath(attr.Key)
	if !ok {
		t.Fatal("index expression path not found")
	}
	if !result.IndexInRangeAtBoundary(point, indexPath, arrayPath) {
		st, _ := result.StateAt(point)
		t.Fatalf("missing %s <= len(%s) proof at point %d; proofs=%#v",
			indexPath, arrayPath, point, st.BranchProofsSnapshot(result.KeySpace()).Proofs)
	}
	floor, ok := result.NumericFloorAtBoundary(point, indexPath)
	if !ok || floor < 1 {
		st, _ := result.StateAt(point)
		t.Fatalf("index numeric floor = %d/%v, want >=1 at point %d; floors=%#v",
			floor, ok, point, st.NumFloorsSnapshot(result.KeySpace()).Floors)
	}
	if !result.IndexReadSafeAtBoundary(point, indexPath, 1, 0, arrayPath) {
		t.Fatalf("index read %s[%s] not marked safe at point %d despite range and positive proofs", arrayPath, indexPath, point)
	}
}

func TestCheckFunctionReturnSlotEvaluatesExpressionWithNestedCall(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function f(user: { id: string, retries: number })
	return user.id .. ":" .. tostring(user.retries)
end`)

	result, err := CheckFunction(fn, Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got, ok := typevalue.TypeOf(reg, exit.ReadReturnSlot(reg, 0))
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("return slot type = %v/%v, want string", got, ok)
	}
}

func TestCheckChunkSeedsDeclaredLocalValueWhenLiteralSourceUnresolved(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local x: string | number = 42`)

	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	stmt := stmts[0].(*ast.LocalAssignStmt)
	x := mustLocalAt(t, result, stmt, 0)
	assign := requireLocalAssignmentPoint(t, result, stmt, 0)
	succs := result.Graph().Successors(assign)
	if len(succs) != 1 {
		t.Fatalf("assignment successors = %v, want one successor", succs)
	}
	after, ok := result.StateAt(succs[0])
	if !ok {
		t.Fatalf("missing state after local assignment")
	}
	got := after.ReadValue(reg, key.SymbolValue(x))
	if product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("symbol value is bottom after annotated local assignment")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Number)))
}

func TestCheckFunctionRunsIntraprocedurally(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(a) local b = a return b end")

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	if result.Registry() != reg {
		t.Fatalf("result registry = %p, want %p", result.Registry(), reg)
	}
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	if len(graph.RPO()) == 0 {
		t.Fatalf("CFG RPO is empty")
	}
	if _, ok := result.StateAt(graph.Entry()); !ok {
		t.Fatalf("flow has no entry state")
	}
	if _, ok := result.ExitState(); !ok {
		t.Fatalf("flow has no exit state")
	}
	returnPoints := result.ReturnPoints()
	if len(returnPoints) != 1 {
		t.Fatalf("return points = %v, want one point", returnPoints)
	}
	returnFact, ok := result.ReturnFact(returnPoints[0])
	if !ok {
		t.Fatalf("missing return fact at %v", returnPoints[0])
	}
	if len(returnFact.Exprs) != 1 {
		t.Fatalf("return fact has %d exprs, want 1", len(returnFact.Exprs))
	}
}

func TestCheckFunctionReturnSourcesUseLoweredFacts(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(a) return a, nil end")

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	returnPoints := result.ReturnPoints()
	if len(returnPoints) != 1 {
		t.Fatalf("return points = %v, want one point", returnPoints)
	}
	sources, ok := result.ReturnValueSources(returnPoints[0])
	if !ok {
		t.Fatalf("missing lowered return sources at %v", returnPoints[0])
	}
	if len(sources) != 2 {
		t.Fatalf("return source count = %d, want 2", len(sources))
	}
}

func TestCheckFunctionSeedsDeclaredParameterEntryState(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(x: string?) local y = x end")

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	slot := mustParamSlot(t, result.bindings, fn, 0)
	entry, ok := result.StateAt(result.Graph().Entry())
	if !ok {
		t.Fatalf("missing entry state")
	}
	got := entry.ReadValue(reg, key.SymbolValue(slot.Symbol))
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
	if pathValue := entry.ReadPathKey(reg, result.KeySpace(), pathaddr.LocalKey(pathaddr.VersionedRootString(slot.Symbol, 1)).PathKey()); !product.Equal(reg, pathValue, product.Bottom(reg)) {
		t.Fatalf("entry path lane for parameter root = %v, want bottom", pathValue)
	}
}

func TestCheckFunctionProjectsFieldFromDeclaredParameterWitness(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type User = { id: string, retries: number }
function f(user: User)
	local id = user.id
	local retries = user.retries
	return id, retries
end`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	fn := result.Function()
	id := mustLocalAt(t, result, fn.Stmts[0].(*ast.LocalAssignStmt), 0)
	retries := mustLocalAt(t, result, fn.Stmts[1].(*ast.LocalAssignStmt), 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	assertPresence(t, reg, exit.ReadValue(reg, key.SymbolValue(id)), presence.Present())
	assertRuntimeKind(t, reg, exit.ReadValue(reg, key.SymbolValue(id)), runtimekind.Singleton(runtimekind.String))
	assertPresence(t, reg, exit.ReadValue(reg, key.SymbolValue(retries)), presence.Present())
	assertRuntimeKind(t, reg, exit.ReadValue(reg, key.SymbolValue(retries)), runtimekind.Singleton(runtimekind.Number))
}

func TestCheckFunctionParameterEntryStateKeepsExplicitEntryValueAndPath(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(x: string?) return x end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	explicitValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	pathKey := pathaddr.LocalKey(pathaddr.VersionedRootString(slot.Symbol, 1)).PathKey()
	explicitPath := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	ks := prepared.KeySpace()
	entryState := state.State{}.
		WriteValue(reg, key.SymbolValue(slot.Symbol), explicitValue).
		WritePathKey(reg, ks, pathKey, explicitPath)

	result, err := SolvePrepared(prepared, SolveConfig{EntryState: entryState})
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}

	entry, ok := result.StateAt(result.Graph().Entry())
	if !ok {
		t.Fatalf("missing entry state")
	}
	assertProductEqual(t, reg, entry.ReadValue(reg, key.SymbolValue(slot.Symbol)), explicitValue)
	assertProductEqual(t, reg, entry.ReadPathKey(reg, ks, pathKey), explicitPath)
}

func TestCheckFunctionParameterEntryStateMergesExplicitInitial(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(x: string?, y: number) return x end")
	bindings := bind.BindFunction(fn, bind.Options{})
	built := cfgbuild.BuildFunction(fn, bindings)
	xSlot := mustParamSlot(t, bindings, fn, 0)
	ySlot := mustParamSlot(t, bindings, fn, 1)
	explicitX := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	initialEntry := state.State{}.WriteValue(reg, key.SymbolValue(xSlot.Symbol), explicitX)

	result, err := CheckBoundFunction(fn, bindings, Config{
		Registry: reg,
		Initial: func(point cfg.Point) (state.State, bool) {
			if built != nil && built.Graph != nil && point == built.Graph.Entry() {
				return initialEntry, true
			}
			return state.State{}, false
		},
	})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	entry, ok := result.StateAt(result.Graph().Entry())
	if !ok {
		t.Fatalf("missing entry state")
	}
	assertProductEqual(t, reg, entry.ReadValue(reg, key.SymbolValue(xSlot.Symbol)), explicitX)
	yValue := entry.ReadValue(reg, key.SymbolValue(ySlot.Symbol))
	assertPresence(t, reg, yValue, presence.Present())
	assertRuntimeKind(t, reg, yValue, runtimekind.Singleton(runtimekind.Number))
}

func TestCheckChunkSeedsDeclaredParameterAliasEntryState(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type MaybeString = string?
function f(x: MaybeString)
	local y = x
end`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	child, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	slot := mustParamSlot(t, child.bindings, child.Function(), 0)
	entry, ok := child.StateAt(child.Graph().Entry())
	if !ok {
		t.Fatalf("missing child entry state")
	}
	got := entry.ReadValue(reg, key.SymbolValue(slot.Symbol))
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestCheckFunctionSeedsImportedDeclaredParameterEntryState(t *testing.T) {
	reg := standard.Registry()
	event := typetable.NewRecord().Field("id", typ.String).Build()
	source := typetable.NewRecord().
		Field("primary", typ.Instantiate(ambient.ChannelGeneric(), event)).
		Build()
	protocol := manifest.New("protocol")
	protocol.DefineType("Source", source)
	stmts := parseChunk(t, `
local protocol = require("protocol")
function consume(source: protocol.Source)
	local primary = source.primary
end`)

	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{
		Registry: reg,
		ModuleTypes: typelookup.Source{
			Manifests: []*manifest.Manifest{protocol},
		},
	})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	slot := mustParamSlot(t, result.bindings, result.Function(), 0)
	entry, ok := result.StateAt(result.Graph().Entry())
	if !ok {
		t.Fatalf("missing entry state")
	}
	got := entry.ReadValue(reg, key.SymbolValue(slot.Symbol))
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, source) {
		t.Fatalf("imported parameter type = %v/%v, want %v", gotType, ok, source)
	}
}

func TestCheckChunkManifestSameAsSignatureUsesArgumentSourceValue(t *testing.T) {
	reg := standard.Registry()
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	m := manifest.New("test")
	m.DefineFunctionSignature("id", signature.Function{
		Type:   typ.Func().Param("value", typ.Any).Returns(typ.Number).Build(),
		Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
	})
	stmts := parseChunk(t, `local x: string = id("s")`)

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
		Globals:  []string{"id"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return argValue, true
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	x := mustLocalAt(t, result, stmts[0].(*ast.LocalAssignStmt), 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got := exit.ReadValue(reg, key.SymbolValue(x))
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestCheckChunkDefaultExpressionValueProjectsStaticReadOptionality(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local out = t["name"]`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"t"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	local := stmts[0].(*ast.LocalAssignStmt)
	assignPoint := requireCheckStmtPoint(t, built, local)
	attr := local.Exprs[0].(*ast.AttrGetExpr)
	tSym := mustIdentSymbol(t, bindings, attr.Object.(*ast.IdentExpr))
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(assignPoint, tSym, "t")
	resolver := visibility.NewResolver(resolverBuilder.Build())
	entry := state.State{}.WriteValue(
		reg,
		key.SymbolValue(tSym),
		product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table)),
	)

	result, err := CheckBoundChunk(stmts, bindings, Config{
		Registry:   reg,
		Globals:    []string{"t"},
		Visibility: resolver,
		EntryState: entry,
	})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}

	out := mustLocalAt(t, result, local, 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	assertPresence(t, reg, exit.ReadValue(reg, key.SymbolValue(out)), presence.Top())
}

func TestCheckChunkDefaultExpressionValueUsesExactPathPresenceProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local out = t.name`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"t"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	local := stmts[0].(*ast.LocalAssignStmt)
	assignPoint := requireCheckStmtPoint(t, built, local)
	attr := local.Exprs[0].(*ast.AttrGetExpr)
	tSym := mustIdentSymbol(t, bindings, attr.Object.(*ast.IdentExpr))
	readPath := path.NewPath(tSym, "t").Field("name")
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(assignPoint, tSym, "t")
	resolver := visibility.NewResolver(resolverBuilder.Build())
	childValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	entry := state.State{}.WritePathKey(reg, resolver.KeySpace(), resolver.KeyAt(assignPoint, readPath), childValue)

	result, err := CheckBoundChunk(stmts, bindings, Config{
		Registry:   reg,
		Globals:    []string{"t"},
		Visibility: resolver,
		EntryState: entry,
	})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}

	out := mustLocalAt(t, result, local, 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got := exit.ReadValue(reg, key.SymbolValue(out))
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestCheckChunkBuildsDefaultVisibilityForExactPathWriteThenRead(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local t = {}
local a: string = "alpha"
local b: number = 1
t.a = a
t.b = b
local outA = t.a
local outB = t.b
`)

	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	outA := mustLocalAt(t, result, stmts[5].(*ast.LocalAssignStmt), 0)
	outB := mustLocalAt(t, result, stmts[6].(*ast.LocalAssignStmt), 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	gotA := exit.ReadValue(reg, key.SymbolValue(outA))
	assertPresence(t, reg, gotA, presence.Present())
	assertRuntimeKind(t, reg, gotA, runtimekind.Singleton(runtimekind.String))
	gotB := exit.ReadValue(reg, key.SymbolValue(outB))
	assertPresence(t, reg, gotB, presence.Present())
	assertRuntimeKind(t, reg, gotB, runtimekind.Singleton(runtimekind.Number))
}

func TestCheckChunkBuildsDefaultVisibilityForBranchPathEquality(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
if t.left == t.right then
	local out = t.right
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"t"}})
	ifStmt := stmts[0].(*ast.IfStmt)
	local := ifStmt.Then[0].(*ast.LocalAssignStmt)
	condition := ifStmt.Condition.(*ast.RelationalOpExpr)
	left := condition.Lhs.(*ast.AttrGetExpr)
	tSym := mustIdentSymbol(t, bindings, left.Object.(*ast.IdentExpr))
	stringValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{
		Registry: reg,
		Globals:  []string{"t"},
	})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	ks := prepared.KeySpace()
	entry := state.State{}.
		WritePathKey(reg, ks, path.PathKey(pathaddr.VersionedRootString(tSym, 1)+".left"), stringValue).
		WritePathKey(reg, ks, path.PathKey(pathaddr.VersionedRootString(tSym, 1)+".right"), product.Top())

	result, err := SolvePrepared(prepared, SolveConfig{EntryState: entry})
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}

	out := mustLocalAt(t, result, local, 0)
	assignPoint := requireLocalAssignmentPoint(t, result, local, 0)
	succs := result.Graph().Successors(assignPoint)
	if len(succs) != 1 {
		t.Fatalf("assignment successors = %v, want one successor", succs)
	}
	after, ok := result.StateAt(succs[0])
	if !ok {
		t.Fatalf("missing state after local assignment")
	}
	got := after.ReadValue(reg, key.SymbolValue(out))
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestReadBoundaryCallResultAssignmentSourceSeesTypeWitness(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local v = Point(data)
`)

	result, err := CheckChunk(stmts, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	assign := stmts[2].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, assign, 0)
	fact, ok := result.facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing lowered local assignment at %d", point)
	}
	source := fact.Source()
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint {
		t.Fatalf("assignment source = %#v, want call result source", source)
	}
	v := mustLocalAt(t, result, assign, 0)
	raw, ok := result.StateAt(point)
	if !ok {
		t.Fatalf("missing raw state at assignment point")
	}
	if got := raw.ReadValue(reg, key.SymbolValue(v)); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("raw assignment input target = %v, want bottom before assignment materialization", got)
	}

	got, ok := result.SourceValueAtBoundary(point, source)
	if !ok {
		t.Fatalf("SourceValueAtBoundary returned false")
	}
	assertConcreteTypeWitness(t, reg, got)
	target, ok := result.SymbolValueAtBoundary(point, v)
	if !ok {
		t.Fatalf("SymbolValueAtBoundary for assigned local returned false")
	}
	assertConcreteTypeWitness(t, reg, target)
}

func TestReadBoundaryLaterAssignmentSeesNormalPostconditionTypeWitness(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {x: number, y: number}
	local function validate(data: any)
	local v = Point(data)
	local p: {x: number, y: number} = data
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	fn := result.Function()
	assign := fn.Stmts[1].(*ast.LocalAssignStmt)
	point := requireLocalAssignmentPoint(t, result, assign, 0)
	got, ok := result.ExpressionValueAtBoundary(point, assign.Exprs[0])
	if !ok {
		t.Fatalf("ExpressionValueAtBoundary returned false")
	}
	assertConcreteTypeWitness(t, reg, got)
}

func TestReadBoundaryNestedFunctionsSeeCastAndNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {x: number, y: number}
local function validate(data: any)
	Point(data)
	local p: {x: number, y: number} = data
	return p
end
local function validate_assign(data: any)
	local v = Point(data)
	local p: {x: number, y: number} = data
	return p
end
local function expect_point(x)
	return Point(x)
end
local function validate_wrapped(data: any)
	expect_point(data)
	local p: {x: number, y: number} = data
	return p
end
return validate, validate_assign, validate_wrapped
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 4 {
		t.Fatalf("nested functions = %d, want 4", len(functions))
	}
	assertLocalAssignmentExprWitness := func(result *Result, stmtIndex int, exprIndex int) {
		t.Helper()
		fn := result.Function()
		assign := fn.Stmts[stmtIndex].(*ast.LocalAssignStmt)
		point := requireLocalAssignmentPoint(t, result, assign, 0)
		got, ok := result.ExpressionValueAtBoundary(point, assign.Exprs[exprIndex])
		if !ok {
			t.Fatalf("ExpressionValueAtBoundary for stmt %d returned false", stmtIndex)
		}
		assertConcreteTypeWitness(t, reg, got)
	}
	validateAssign, err := CheckBoundFunction(functions[1], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction(validate_assign): %v", err)
	}
	assertLocalAssignmentExprWitness(validateAssign, 1, 0)
}

func TestReadBoundaryBranchSuccessorExpressionSeesEdgeRefinement(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {x: number, y: number}
function validate(data: any)
	local _, err = Point:is(data)
	if err == nil then
		local narrowed = data
	end
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	fn := result.Function()

	data := mustParamSlot(t, result.bindings, fn, 0).Symbol
	typeIsStmt := fn.Stmts[0].(*ast.LocalAssignStmt)
	typeIsPoint := requireLocalAssignmentPoint(t, result, typeIsStmt, 1)
	if before, ok := result.SymbolValueAtBoundary(typeIsPoint, data); ok {
		if witness := product.Get(reg, before, typewitness.Key); !witness.IsTop() {
			t.Fatalf("pre-branch data witness = %v, want no concrete witness", witness)
		}
	}

	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	thenLocal := ifStmt.Then[0].(*ast.LocalAssignStmt)
	thenPoint := requireLocalAssignmentPoint(t, result, thenLocal, 0)
	got, ok := result.ExpressionValueAtBoundary(thenPoint, thenLocal.Exprs[0])
	if !ok {
		t.Fatalf("ExpressionValueAtBoundary returned false")
	}
	assertConcreteTypeWitness(t, reg, got)
}

func TestReadBoundaryPresentAliasDiscriminantRemovesDescendantNil(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type TextNode = { kind: "text", value: string }
type GroupNode = { kind: "group", children: {TreeNode} }
type TreeNode = TextNode | GroupNode

function validate(tree: TreeNode)
	if tree.kind == "group" then
		local first = tree.children[1]
		if first and first.kind == "text" then
			local value = first.value
		end
	end
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	fn := result.Function()
	outer := fn.Stmts[0].(*ast.IfStmt)
	firstLocal := outer.Then[0].(*ast.LocalAssignStmt)
	inner := outer.Then[1].(*ast.IfStmt)
	valueLocal := inner.Then[0].(*ast.LocalAssignStmt)
	valuePoint := requireLocalAssignmentPoint(t, result, valueLocal, 0)

	firstSym := mustLocalAt(t, result, firstLocal, 0)
	firstValue, ok := result.SymbolValueAtBoundary(valuePoint, firstSym)
	if !ok {
		t.Fatalf("first boundary value missing")
	}
	if !presence.Equal(product.PresenceOf(firstValue), presence.Present()) {
		firstType, _ := typevalue.TypeOf(reg, firstValue)
		t.Fatalf("first presence = %v type=%v, want present", product.PresenceOf(firstValue), firstType)
	}
	got, ok := result.ExpressionValueAtBoundary(valuePoint, valueLocal.Exprs[0])
	if !ok {
		t.Fatalf("ExpressionValueAtBoundary returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("first.value type = %v/%v presence=%v, want string present", gotType, ok, product.PresenceOf(got))
	}
}

func TestReadBoundaryChannelSelectBranchSeesPayloadWitness(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Event = { id: string }
type Retry = { attempt: number }
type Timer = { elapsed: number }
type Stop = { reason: string }
function consume(primary: Channel<Event>, retry: Channel<Retry>, timers: Channel<Timer>, stops: Channel<Stop>)
	local result = channel.select {
		primary:case_receive(),
		retry:case_receive(),
		timers:case_receive(),
		stops:case_receive(),
	}
	if result.channel == primary then
		local event = result.value
		local id = event.id
		return id
	end
	if result.channel == retry then
		local retry = result.value
		local attempt = retry.attempt
		return tostring(attempt)
	end
	if result.channel == timers then
		local timer = result.value
		local elapsed = timer.elapsed
		return tostring(elapsed)
	end
	local fallback = result.value
	return fallback.reason
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg, Globals: []string{"channel"}})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	fn := result.Function()
	selectLocal := fn.Stmts[0].(*ast.LocalAssignStmt)
	resultSym := mustLocalAt(t, result, selectLocal, 0)
	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	branchPoint := requireCheckStmtPoint(t, result.cfg, ifStmt)
	relations := result.facts.BranchPathRelations(branchPoint)
	if len(relations) == 0 {
		t.Fatalf("missing branch path relation at %d", branchPoint)
	}
	thenLocal := ifStmt.Then[0].(*ast.LocalAssignStmt)
	thenPoint := requireLocalAssignmentPoint(t, result, thenLocal, 0)
	root, ok := result.SymbolValueAtBoundary(thenPoint, resultSym)
	if !ok {
		t.Fatalf("selected result root missing at branch boundary")
	}
	thenState, ok := result.StateAt(thenPoint)
	if !ok {
		t.Fatalf("missing then state")
	}
	channelFacts := thenState.ChannelSelectFactsSnapshot()
	if channelFacts.Bottom || len(channelFacts.Facts) == 0 {
		t.Fatalf("then branch has no channel-select facts: %#v", channelFacts)
	}
	got, ok := result.ExpressionValueAtBoundary(thenPoint, thenLocal.Exprs[0])
	if !ok {
		t.Fatalf("ExpressionValueAtBoundary returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok {
		t.Fatalf("payload witness missing in %v", got)
	}
	want := typetable.NewRecord().Field("id", typ.String).Build()
	if !typ.TypeEquals(gotType, want) {
		rootType, _ := typevalue.TypeOf(reg, root)
		t.Fatalf("selected payload type = %v, want %v; root at branch = %v; relations=%#v channelFacts=%#v", gotType, want, rootType, relations, channelFacts)
	}
	idLocal := ifStmt.Then[1].(*ast.LocalAssignStmt)
	idPoint := requireLocalAssignmentPoint(t, result, idLocal, 0)
	idValue, ok := result.ExpressionValueAtBoundary(idPoint, idLocal.Exprs[0])
	if !ok {
		t.Fatalf("id ExpressionValueAtBoundary returned false")
	}
	idType, ok := typevalue.TypeOf(reg, idValue)
	if !ok || !typ.TypeEquals(idType, typ.String) {
		t.Fatalf("selected payload id type = %v/%v, want string", idType, ok)
	}

	secondIf := fn.Stmts[3].(*ast.IfStmt)
	secondLocal := secondIf.Then[0].(*ast.LocalAssignStmt)
	secondPoint := requireLocalAssignmentPoint(t, result, secondLocal, 0)
	secondValue, ok := result.ExpressionValueAtBoundary(secondPoint, secondLocal.Exprs[0])
	if !ok {
		t.Fatalf("second ExpressionValueAtBoundary returned false")
	}
	secondType, ok := typevalue.TypeOf(reg, secondValue)
	if !ok {
		t.Fatalf("second payload witness missing in %v", secondValue)
	}
	timerWant := typetable.NewRecord().Field("elapsed", typ.Number).Build()
	if !typ.TypeEquals(secondType, timerWant) {
		t.Fatalf("second selected payload type = %v, want %v", secondType, timerWant)
	}

	fallbackLocal := fn.Stmts[4].(*ast.LocalAssignStmt)
	fallbackPoint := requireLocalAssignmentPoint(t, result, fallbackLocal, 0)
	fallbackValue, ok := result.ExpressionValueAtBoundary(fallbackPoint, fallbackLocal.Exprs[0])
	if !ok {
		t.Fatalf("fallback ExpressionValueAtBoundary returned false")
	}
	fallbackType, ok := typevalue.TypeOf(reg, fallbackValue)
	if !ok {
		t.Fatalf("fallback payload witness missing in %v", fallbackValue)
	}
	stopWant := typetable.NewRecord().Field("reason", typ.String).Build()
	if !typ.TypeEquals(fallbackType, stopWant) {
		t.Fatalf("fallback selected payload type = %v, want %v", fallbackType, stopWant)
	}
}

func TestReadBoundaryChannelSelectPayloadPathAssignmentCarriesNestedWitness(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Leaf = { kind: "leaf", id: string }
type Control = { kind: "control", name: string }
type Timer = { kind: "timer", tick: number }
type RouteA = { kind: "route_a", ch: Channel<Leaf | Control> }
type RouteB = { kind: "route_b", ch: Channel<Control> }
type Stream = {
	kind: "stream",
	router: {
		selected: RouteA | RouteB,
		fallback: Channel<Control>,
	},
}
type Box = {
	kind: "box",
	next: Box | Stream | Other,
}
type Other = { kind: "other", reason: string }
type Event = Stream | Box | Other
function consume(events: Channel<Event>, controls: Channel<Control>, timers: Channel<Timer>)
	local selected = channel.select {
		events:case_receive(),
		controls:case_receive(),
		timers:case_receive(),
	}
	if selected.channel == events then
		local payload = selected.value
		if payload.kind == "stream" then
			local route = payload.router.selected
			if route.kind == "route_a" then
				local routed = channel.select {
					route.ch:case_receive(),
					payload.router.fallback:case_receive(),
				}
				if routed.channel == route.ch then
					local value = routed.value
					if value.kind == "control" then
						local name = value.name
						return name
					end
					local id = value.id
					return id
				end
				local fallback = routed.value
				return fallback.name
			end
		end
	end
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg, Globals: []string{"channel"}})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	fn := result.Function()
	outerIf := fn.Stmts[1].(*ast.IfStmt)
	payloadLocal := outerIf.Then[0].(*ast.LocalAssignStmt)
	streamIf := outerIf.Then[1].(*ast.IfStmt)
	routeLocal := streamIf.Then[0].(*ast.LocalAssignStmt)
	routeIf := streamIf.Then[1].(*ast.IfStmt)
	routedLocal := routeIf.Then[0].(*ast.LocalAssignStmt)
	routedIf := routeIf.Then[1].(*ast.IfStmt)
	valueLocal := routedIf.Then[0].(*ast.LocalAssignStmt)
	valueIf := routedIf.Then[1].(*ast.IfStmt)
	nameLocal := valueIf.Then[0].(*ast.LocalAssignStmt)
	idLocal := routedIf.Then[2].(*ast.LocalAssignStmt)
	fallbackLocal := routeIf.Then[2].(*ast.LocalAssignStmt)

	payloadPoint := requireLocalAssignmentPoint(t, result, payloadLocal, 0)
	payloadSym := mustLocalAt(t, result, payloadLocal, 0)
	payloadValue, ok := postAssignmentSymbolValue(t, result, payloadPoint, payloadSym)
	if !ok {
		t.Fatalf("payload local missing after assignment")
	}
	if payloadType, ok := typevalue.TypeOf(reg, payloadValue); !ok || typ.IsAny(payloadType) || typ.IsUnknown(payloadType) {
		t.Fatalf("payload type = %v/%v, want concrete selected Event payload", payloadType, ok)
	}

	routePoint := requireLocalAssignmentPoint(t, result, routeLocal, 0)
	routeSym := mustLocalAt(t, result, routeLocal, 0)
	routeValue, ok := postAssignmentSymbolValue(t, result, routePoint, routeSym)
	if !ok {
		t.Fatalf("route local missing after assignment")
	}
	if routeType, ok := typevalue.TypeOf(reg, routeValue); !ok || !typ.TypeEquals(routeType, typeexpr.Union(
		typetable.NewRecord().Field("kind", typ.LiteralString("route_a")).Field("ch", typ.Instantiate(ambient.ChannelGeneric(), typeexpr.Union(
			typetable.NewRecord().Field("kind", typ.LiteralString("leaf")).Field("id", typ.String).Build(),
			typetable.NewRecord().Field("kind", typ.LiteralString("control")).Field("name", typ.String).Build(),
		))).Build(),
		typetable.NewRecord().Field("kind", typ.LiteralString("route_b")).Field("ch", typ.Instantiate(ambient.ChannelGeneric(),
			typetable.NewRecord().Field("kind", typ.LiteralString("control")).Field("name", typ.String).Build(),
		)).Build(),
	)) {
		t.Fatalf("route type = %v/%v, want RouteA | RouteB", routeType, ok)
	}

	routedPoint := requireLocalAssignmentPoint(t, result, routedLocal, 0)
	routedSym := mustLocalAt(t, result, routedLocal, 0)
	routedValue, ok := postAssignmentSymbolValue(t, result, routedPoint, routedSym)
	if !ok {
		t.Fatalf("routed local missing after assignment")
	}
	routedType, ok := typevalue.TypeOf(reg, routedValue)
	if !ok {
		t.Fatalf("routed select witness missing in %v", routedValue)
	}
	if typ.IsAny(routedType) || typ.IsUnknown(routedType) {
		t.Fatalf("routed select type = %v, want finite case union", routedType)
	}

	valuePoint := requireLocalAssignmentPoint(t, result, valueLocal, 0)
	valueSym := mustLocalAt(t, result, valueLocal, 0)
	value, ok := postAssignmentSymbolValue(t, result, valuePoint, valueSym)
	if !ok {
		t.Fatalf("selected nested value missing after assignment")
	}
	valueType, ok := typevalue.TypeOf(reg, value)
	wantSelected := typeexpr.Union(
		typetable.NewRecord().Field("kind", typ.LiteralString("leaf")).Field("id", typ.String).Build(),
		typetable.NewRecord().Field("kind", typ.LiteralString("control")).Field("name", typ.String).Build(),
	)
	if !ok || !typ.TypeEquals(valueType, wantSelected) {
		t.Fatalf("selected nested value type = %v/%v, want %v", valueType, ok, wantSelected)
	}

	namePoint := requireLocalAssignmentPoint(t, result, nameLocal, 0)
	nameSym := mustLocalAt(t, result, nameLocal, 0)
	nameValue, ok := postAssignmentSymbolValue(t, result, namePoint, nameSym)
	if !ok {
		t.Fatalf("control name missing after assignment")
	}
	nameType, ok := typevalue.TypeOf(reg, nameValue)
	if !ok || !typ.TypeEquals(nameType, typ.String) {
		t.Fatalf("control name type = %v/%v, want string", nameType, ok)
	}

	idPoint := requireLocalAssignmentPoint(t, result, idLocal, 0)
	idSym := mustLocalAt(t, result, idLocal, 0)
	idValue, ok := postAssignmentSymbolValue(t, result, idPoint, idSym)
	if !ok {
		t.Fatalf("leaf id missing after assignment")
	}
	idType, ok := typevalue.TypeOf(reg, idValue)
	if !ok || !typ.TypeEquals(idType, typ.String) {
		t.Fatalf("leaf id type = %v/%v, want string", idType, ok)
	}

	fallbackPoint := requireLocalAssignmentPoint(t, result, fallbackLocal, 0)
	fallbackSym := mustLocalAt(t, result, fallbackLocal, 0)
	fallback, ok := postAssignmentSymbolValue(t, result, fallbackPoint, fallbackSym)
	if !ok {
		t.Fatalf("fallback nested value missing after assignment")
	}
	fallbackType, ok := typevalue.TypeOf(reg, fallback)
	wantFallback := typetable.NewRecord().Field("kind", typ.LiteralString("control")).Field("name", typ.String).Build()
	if !ok || !typ.TypeEquals(fallbackType, wantFallback) {
		t.Fatalf("fallback nested value type = %v/%v, want %v", fallbackType, ok, wantFallback)
	}
}

func TestReadBoundaryDiscriminantUnlocksBoxChannelFields(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Leaf = { kind: "leaf", id: string }
type Deadline = { kind: "deadline", tick: number }
type RouteA = { kind: "route_a", ch: Channel<Leaf> }
type RouteB = { kind: "route_b", ch: Channel<Deadline> }
type Box = {
	kind: "box",
	node: {
		left: Channel<RouteA | RouteB>,
		right: Channel<Leaf | Deadline>,
	},
}
type Other = { kind: "other", reason: string }
type Event = Box | Other
function consume(events: Channel<Event>)
	local selected = channel.select {
		events:case_receive(),
	}
	local payload = selected.value
	if payload.kind == "box" then
		local left = payload.node.left
		local boxed = channel.select {
			payload.node.left:case_receive(),
			payload.node.right:case_receive(),
		}
		return left
	end
	return nil
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg, Globals: []string{"channel"}})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	fn := result.Function()
	boxIf := fn.Stmts[2].(*ast.IfStmt)
	leftLocal := boxIf.Then[0].(*ast.LocalAssignStmt)
	leftPoint := requireLocalAssignmentPoint(t, result, leftLocal, 0)
	leftValue, ok := result.ExpressionValueAtBoundary(leftPoint, leftLocal.Exprs[0])
	if !ok {
		t.Fatalf("payload.node.left missing at box branch boundary")
	}
	leftType, ok := typevalue.TypeOf(reg, leftValue)
	want := typ.Instantiate(ambient.ChannelGeneric(), typeexpr.Union(
		typetable.NewRecord().Field("kind", typ.LiteralString("route_a")).Field("ch", typ.Instantiate(ambient.ChannelGeneric(),
			typetable.NewRecord().Field("kind", typ.LiteralString("leaf")).Field("id", typ.String).Build(),
		)).Build(),
		typetable.NewRecord().Field("kind", typ.LiteralString("route_b")).Field("ch", typ.Instantiate(ambient.ChannelGeneric(),
			typetable.NewRecord().Field("kind", typ.LiteralString("deadline")).Field("tick", typ.Number).Build(),
		)).Build(),
	))
	if !ok || !typ.TypeEquals(leftType, want) {
		t.Fatalf("payload.node.left type = %v/%v, want %v", leftType, ok, want)
	}
}

func postAssignmentSymbolValue(t *testing.T, result *Result, point cfg.Point, sym symbol.ID) (product.Value, bool) {
	t.Helper()
	succs := result.Graph().Successors(point)
	if len(succs) != 1 {
		t.Fatalf("assignment %d successors = %v, want one", point, succs)
	}
	post, ok := result.StateAt(succs[0])
	if !ok {
		t.Fatalf("missing state after assignment %d", point)
	}
	value := post.ReadValue(result.registry, key.SymbolValue(sym))
	if product.Equal(result.registry, value, product.Bottom(result.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func TestReadBoundaryDiscriminatedReturnSourceSeesBranchOriginRefinement(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }
type User = { id: string, email: string }
function get_email(id: string): (string?, string?)
	local r: Result<User> = { ok = true, value = { id = id, email = "a@test" } }
	if r.ok then
		return r.value.email, nil
	end
	return nil, r.error
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	for _, point := range result.ReturnPoints() {
		sources, ok := result.ReturnValueSources(point)
		if !ok || len(sources) != 2 {
			continue
		}
		second, ok := result.SourceValueAtBoundary(point, sources[1])
		if !ok || !presence.Equal(product.PresenceOf(second), presence.Absent()) {
			continue
		}
		first, ok := result.SourceValueAtBoundary(point, sources[0])
		if !ok {
			t.Fatalf("success return source at point %d was not readable", point)
		}
		if got := product.PresenceOf(first); !presence.Equal(got, presence.Present()) {
			t.Fatalf("success return source presence = %s, want present", got)
		}
		return
	}
	t.Fatal("success return point not found")
}

func TestReadBoundaryGenericResultFalseEdgeProjectsErrorPresent(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }
type User = { id: string, email: string }

local function err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return { ok = true, value = fn(result.value) }
	end
	return err(result.error)
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 2 {
		t.Fatalf("nested functions = %d, want 2", len(functions))
	}
	result, err := CheckBoundFunction(functions[1], bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	mapFn := result.Function()
	ret := mapFn.Stmts[1].(*ast.ReturnStmt)
	call := ret.Exprs[0].(*ast.FuncCallExpr)
	arg := call.Args[0]
	var point cfg.Point
	found := false
	for _, candidate := range result.ReturnPoints() {
		fact, ok := result.ReturnFact(candidate)
		if ok && fact.Stmt == ret {
			point = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("return point for err(result.error) not found")
	}
	value, ok := result.ExpressionValueAtBoundary(point, arg)
	if !ok {
		t.Fatalf("result.error boundary value missing")
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		t.Fatalf("result.error presence = %s, want present", got)
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("result.error type = %v/%v, want string", got, ok)
	}

	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.Call(candidate)
		if !ok || fact.Call != call {
			continue
		}
		callValue, ok := result.ExpressionValueAtBoundary(candidate, arg)
		if !ok {
			t.Fatalf("call-point result.error boundary value missing")
		}
		if got := product.PresenceOf(callValue); !presence.Equal(got, presence.Present()) {
			t.Fatalf("call-point result.error presence = %s, want present", got)
		}
		got, ok := typevalue.TypeOf(reg, callValue)
		if !ok || !typ.TypeEquals(got, typ.String) {
			t.Fatalf("call-point result.error type = %v/%v, want string", got, ok)
		}
		return
	}
	t.Fatalf("call point for err(result.error) not found")
}

func TestReadBoundarySignatureResultAssignmentSeesBranchOriginRefinement(t *testing.T) {
	reg := standard.Registry()
	userType := typetable.NewRecord().
		Field("id", typ.String).
		Field("email", typ.String).
		Build()
	resultUser := typeexpr.Union(
		typetable.NewRecord().Field("ok", typ.LiteralBool(true)).Field("value", userType).Build(),
		typetable.NewRecord().Field("ok", typ.LiteralBool(false)).Field("error", typ.String).Build(),
	)
	repoManifest := manifest.New("repo")
	repoManifest.DefineFunctionSignature("repo.find_by_id", signature.Function{
		Type: typ.Func().Param("id", typ.String).Returns(resultUser).Build(),
	})
	stmts := parseChunk(t, `
function get_email(id: string): (string?, string?)
	local r = repo.find_by_id(id)
	if r.ok then
		return r.value.email, nil
	end
	return nil, r.error
end
`)

	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"repo"}})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result, err := CheckBoundFunction(functions[0], bindings, Config{
		Registry: reg,
		Globals:  []string{"repo"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{repoManifest},
		},
	})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	seenSuccess := false
	seenFailure := false
	for _, point := range result.ReturnPoints() {
		sources, ok := result.ReturnValueSources(point)
		if !ok || len(sources) != 2 {
			continue
		}
		first, firstOK := result.SourceValueAtBoundary(point, sources[0])
		second, secondOK := result.SourceValueAtBoundary(point, sources[1])
		if !firstOK || !secondOK {
			continue
		}
		switch {
		case presence.Equal(product.PresenceOf(first), presence.Present()) &&
			presence.Equal(product.PresenceOf(second), presence.Absent()):
			seenSuccess = true
		case presence.Equal(product.PresenceOf(first), presence.Absent()) &&
			presence.Equal(product.PresenceOf(second), presence.Present()):
			seenFailure = true
		}
	}
	if !seenSuccess || !seenFailure {
		t.Fatalf("return source presence success=%v failure=%v, want both discriminated paths", seenSuccess, seenFailure)
	}
}

func TestCheckChunkUserExpressionValueOverridesDefaultStaticReadProjector(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local out = t.name`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"t"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	local := stmts[0].(*ast.LocalAssignStmt)
	assignPoint := requireCheckStmtPoint(t, built, local)
	attr := local.Exprs[0].(*ast.AttrGetExpr)
	tSym := mustIdentSymbol(t, bindings, attr.Object.(*ast.IdentExpr))
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(assignPoint, tSym, "t")
	override := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Absent()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.Nil),
	)

	result, err := CheckBoundChunk(stmts, bindings, Config{
		Registry:   reg,
		Globals:    []string{"t"},
		Visibility: visibility.NewResolver(resolverBuilder.Build()),
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return override, true
		},
	})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}

	out := mustLocalAt(t, result, local, 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got := exit.ReadValue(reg, key.SymbolValue(out))
	assertPresence(t, reg, got, presence.Absent())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.Nil))
}

func TestCheckBoundFunctionUsesSuppliedBindingIdentity(t *testing.T) {
	reg, markKey := testRegistry(t)
	want := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), markKey, markLow)
	stmts := parseChunk(t, `
local captured = 1
function f()
	local value = captured
	return value
end`)
	capturedDecl, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt 0 = %T, want *ast.LocalAssignStmt", stmts[0])
	}
	def, ok := stmts[1].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt 1 = %T, want function definition", stmts[1])
	}
	valueDecl, ok := def.Func.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("function stmt 0 = %T, want *ast.LocalAssignStmt", def.Func.Stmts[0])
	}

	bindings := bind.BindChunk(stmts, bind.Options{})
	captured := mustBoundLocalAt(t, bindings, capturedDecl, 0)
	suppliedLocal := mustBoundLocalAt(t, bindings, valueDecl, 0)
	captures := bindings.DirectCaptures(def.Func)
	if len(captures) != 1 || captures[0].Captured != captured {
		t.Fatalf("DirectCaptures = %+v, want captured symbol %d", captures, captured)
	}

	config := Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return want, true
		},
	}
	result, err := CheckBoundFunction(def.Func, bindings, config)
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	if got := mustLocalAt(t, result, valueDecl, 0); got != suppliedLocal {
		t.Fatalf("bound result local = %d, want supplied binding local %d", got, suppliedLocal)
	}
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	assertProductEqual(t, reg, exit.ReadValue(reg, key.SymbolValue(suppliedLocal)), want)

	independent, err := CheckFunction(def.Func, config)
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	if got := mustLocalAt(t, independent, valueDecl, 0); got == suppliedLocal {
		t.Fatalf("independent CheckFunction local = %d, want a fresh rebind", got)
	}
}

func TestCopyConfigCopiesMutableFields(t *testing.T) {
	reg := standard.Registry()
	expr := factflow.ExprRef(42)
	initial := map[factflow.ExprRef]product.Value{
		expr: product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
	}
	config := Config{
		Registry:         reg,
		Globals:          []string{"before"},
		ExpressionValues: initial,
	}

	copied := copyConfig(config)
	config.Globals[0] = "after"
	initial[expr] = product.Absent(reg)

	if got := copied.Globals; len(got) != 1 || got[0] != "before" {
		t.Fatalf("copied globals = %v, want [before]", got)
	}
	gotValue := copied.ExpressionValues[expr]
	if gotPresence := product.PresenceOf(gotValue); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("copied expression presence = %s, want present", gotPresence)
	}
}

func TestCheckChunkAcceptsSequencedLogicalCallCFG(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, "print(value() and true)")

	if _, err := CheckChunk(stmts, Config{Registry: reg}); err != nil {
		t.Fatalf("CheckChunk error = %v, want supported logical-call CFG", err)
	}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "check_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func parseFunction(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := parseChunk(t, src)
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	return def.Func
}

func mustLocalAt(t *testing.T, result *Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := result.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("local symbol at %d is zero", index)
	}
	return locals[index]
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

func mustIdentSymbol(t *testing.T, bindings *bind.Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(ident)
	if !ok || id == 0 {
		t.Fatalf("missing symbol for ident %q", ident.Value)
	}
	return id
}

func mustParamSlot(t *testing.T, bindings *bind.Result, fn *ast.FunctionExpr, index int) bind.ParamSlot {
	t.Helper()
	slots := bindings.ParamSlots(fn)
	if index < 0 || index >= len(slots) {
		t.Fatalf("param slot index %d out of range for %d slots", index, len(slots))
	}
	if slots[index].Symbol == 0 {
		t.Fatalf("param slot %d has zero symbol", index)
	}
	return slots[index]
}

func requireCheckStmtPoint(t *testing.T, built *cfgbuild.Result, stmt ast.Stmt) cfg.Point {
	t.Helper()
	if built == nil {
		t.Fatalf("missing cfg build result")
	}
	points := built.StmtPoints.PointsFor(stmt)
	if len(points) != 1 {
		t.Fatalf("stmt points = %v, want one point", points)
	}
	return points[0]
}

func requireLocalAssignmentPoint(t *testing.T, result *Result, stmt *ast.LocalAssignStmt, index int) cfg.Point {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point
		}
	}
	t.Fatalf("missing local assignment point for index %d", index)
	return 0
}

func assertProductEqual(t *testing.T, reg *axis.Registry, got, want product.Value) {
	t.Helper()
	if !product.Equal(reg, got, want) {
		t.Fatalf("value = %v, want %v", got, want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtimekind = %s, want %s", kind, want)
	}
}

func assertPresence(t *testing.T, _ *axis.Registry, got product.Value, want presence.Value) {
	t.Helper()
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("presence = %s, want %s", gotPresence, want)
	}
}

func assertConcreteTypeWitness(t *testing.T, reg *axis.Registry, got product.Value) {
	t.Helper()
	witness := product.Get(reg, got, typewitness.Key)
	if _, ok := witness.Type(); !ok {
		t.Fatalf("type witness = %v, want concrete witness", witness)
	}
}

type markValue uint8

const (
	markBottom markValue = iota
	markLow
	markHigh
)

func testRegistry(t *testing.T) (*axis.Registry, axis.Key[markValue]) {
	t.Helper()
	markKey := axis.NewKey[markValue]("check.test.mark." + strings.ReplaceAll(t.Name(), "/", "."))
	reg, err := standard.RegistryWithAxes(axis.Spec[markValue]{
		Key:    markKey,
		Bottom: func() markValue { return markBottom },
		Top:    func() markValue { return markHigh },
		Equal:  func(a, b markValue) bool { return a == b },
		LessOrEq: func(a, b markValue) bool {
			return a == b || a == markBottom || b == markHigh
		},
		Join: func(a, b markValue) markValue {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b markValue) markValue {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(_, next markValue) markValue { return next },
		Hash:  func(v markValue) uint64 { return uint64(v) },
	}.Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes: %v", err)
	}
	return reg, markKey
}
