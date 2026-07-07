package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFunctionContextEntryHoldsAllowsExtraCurrentHeapFacts(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	requiredID := identity.LuaTableLiteral(7100, 1)
	extraID := identity.LuaTableLiteral(7100, 2)
	requiredRoot := heapTableValue(reg, requiredID)
	extraRoot := heapTableValue(reg, extraID)
	memberValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
		StaticMembers: map[keyspace.Key]product.Value{
			nameStaticKey(t, ks, "name"): memberValue,
		},
	}))
	current := entry.WriteHeapTableObject(reg, extraID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: extraRoot,
	}))

	if !functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned false, want extra current heap facts tolerated")
	}
}

func TestFunctionContextEntryHoldsRejectsMissingRequiredHeapFacts(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	requiredID := identity.LuaTableLiteral(7100, 3)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: heapTableValue(reg, requiredID),
	}))

	if functionContextEntryHolds(reg, ks, ks, entry, state.State{}, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want missing required heap facts rejected")
	}
}

func TestFunctionContextEntryHoldsRejectsWidenedPathRefinement(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	pathKey := path.PathKey("sym71@1.value")
	entry := state.State{}.WritePathKey(reg, ks, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))
	current := state.State{}.WritePathKey(reg, ks, pathKey, runtimeValue(reg, presence.Maybe(), runtimekind.String))

	if functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want widened current path refinement rejected")
	}

	morePrecise := state.State{}.WritePathKey(reg, ks, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))
	if !functionContextEntryHolds(reg, ks, ks, entry, morePrecise, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds rejected matching current path refinement")
	}
}

func TestFunctionContextEntryHoldsAllowsMissingSelfIdentityRefinement(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	sourceID := identity.LuaFunction(71)
	entry := state.State{}.WritePathKey(reg, ks, path.PathKey("sym71@1"), identityValue(reg, sourceID))

	if !functionContextEntryHolds(reg, ks, ks, entry, state.State{}, sourceID) {
		t.Fatalf("functionContextEntryHolds rejected missing refinement for source function identity")
	}
}

func TestFunctionContextEntryHoldsRejectsMissingSelfIdentityWithExtraRequirement(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	sourceID := identity.LuaFunction(71)
	required := product.Set(reg, identityValue(reg, sourceID), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	entry := state.State{}.WritePathKey(reg, ks, path.PathKey("sym71@1"), required)

	if functionContextEntryHolds(reg, ks, ks, entry, state.State{}, sourceID) {
		t.Fatalf("functionContextEntryHolds accepted missing self path with extra required runtime kind")
	}
}

func TestFunctionContextEntryHoldsRejectsMissingNonSelfIdentityRefinement(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	entry := state.State{}.WritePathKey(reg, ks, path.PathKey("sym72@1"), identityValue(reg, identity.LuaFunction(72)))

	if functionContextEntryHolds(reg, ks, ks, entry, state.State{}, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds accepted missing refinement for non-source identity")
	}
}

func TestFunctionContextEntryHoldsRejectsTopIdentityForSpecificPathRefinement(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	pathKey := path.PathKey("sym73@1.service")
	requiredID := identity.LuaTableLiteral(7300, 1)
	entry := state.State{}.WritePathKey(reg, ks, pathKey, heapTableValue(reg, requiredID))
	current := state.State{}.WritePathKey(reg, ks, pathKey, runtimeValue(reg, presence.Present(), runtimekind.Table))

	if functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds accepted identity-top current value for specific required identity")
	}

	matching := state.State{}.WritePathKey(reg, ks, pathKey, heapTableValue(reg, requiredID))
	if !functionContextEntryHolds(reg, ks, ks, entry, matching, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds rejected matching specific identity")
	}
}

func TestFunctionContextEntryHoldsRejectsTopRuntimeKindForSpecificPathRefinement(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	pathKey := path.PathKey("sym74@1.value")
	entry := state.State{}.WritePathKey(reg, ks, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))
	current := state.State{}.WritePathKey(reg, ks, pathKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	if functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds accepted runtime-kind top current value for specific required kind")
	}
}

func TestFunctionContextEntryHoldsAllowsMorePreciseCurrentPathRefinement(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	pathKey := path.PathKey("sym75@1.value")
	entry := state.State{}.WritePathKey(reg, ks, pathKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	current := state.State{}.WritePathKey(reg, ks, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))

	if !functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds rejected more precise current path refinement")
	}
}

func TestFunctionContextEntryHoldsRejectsMissingRequiredHeapStaticMember(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	requiredID := identity.LuaTableLiteral(7100, 4)
	requiredRoot := heapTableValue(reg, requiredID)
	memberValue := runtimeValue(reg, presence.Present(), runtimekind.String)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
		StaticMembers: map[keyspace.Key]product.Value{
			nameStaticKey(t, ks, "name"): memberValue,
		},
	}))
	current := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
	}))

	if functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want missing required heap static member rejected")
	}
}

func TestFunctionContextEntryHoldsRejectsWidenedHeapStaticMember(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	requiredID := identity.LuaTableLiteral(7100, 5)
	requiredRoot := heapTableValue(reg, requiredID)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
		StaticMembers: map[keyspace.Key]product.Value{
			nameStaticKey(t, ks, "name"): runtimeValue(reg, presence.Present(), runtimekind.String),
		},
	}))
	current := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
		StaticMembers: map[keyspace.Key]product.Value{
			nameStaticKey(t, ks, "name"): runtimeValue(reg, presence.Maybe(), runtimekind.String),
		},
	}))

	if functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want widened heap static member rejected")
	}
}

func TestFunctionContextEntryHoldsRejectsTopCurrentHeapWhenHeapFactsRequired(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	requiredID := identity.LuaTableLiteral(7100, 6)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: heapTableValue(reg, requiredID),
	}))
	current := state.Domain(reg).Top()

	if functionContextEntryHolds(reg, ks, ks, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want top current heap rejected when entry requires finite heap facts")
	}
}

func TestCloneFunctionValueTypesDecouplesMutableIndexes(t *testing.T) {
	id := identity.LuaFunction(710)
	key, ok := factflow.CalleePathKeyFromPath(path.NewPath(71, "handler"))
	if !ok {
		t.Fatal("CalleePathKeyFromPath failed")
	}
	originalFn := typ.Func().Returns(typ.String).Build()
	mutatedFn := typ.Func().Returns(typ.Number).Build()
	types := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			id: originalFn,
		},
		ByPath: map[factflow.CalleePathKey]*typ.Function{
			key: originalFn,
		},
		ContextsByIdentity: map[identity.ID][]FunctionValueContext{
			id: {{EntryKeys: keyspace.New(), Type: originalFn}},
		},
	}

	clone := cloneFunctionValueTypes(types)
	types.ByIdentity[id] = mutatedFn
	types.ByPath[key] = mutatedFn
	types.ContextsByIdentity[id][0].Type = mutatedFn
	types.ContextsByIdentity[id] = append(types.ContextsByIdentity[id], FunctionValueContext{Type: mutatedFn})

	if clone.ByIdentity[id] != originalFn {
		t.Fatalf("clone ByIdentity changed after source map mutation")
	}
	if clone.ByPath[key] != originalFn {
		t.Fatalf("clone ByPath changed after source map mutation")
	}
	if got := clone.ContextsByIdentity[id]; len(got) != 1 || got[0].Type != originalFn {
		t.Fatalf("clone contexts = %#v, want one original context", got)
	}
}

func TestWithOwnedFunctionValueTypesReusesCallerOwnedProjection(t *testing.T) {
	id := identity.LuaFunction(711)
	originalFn := typ.Func().Returns(typ.String).Build()
	mutatedFn := typ.Func().Returns(typ.Number).Build()
	types := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			id: originalFn,
		},
		ContextsByIdentity: map[identity.ID][]FunctionValueContext{
			id: {{EntryKeys: keyspace.New(), Type: originalFn}},
		},
	}

	var defensive Result
	WithFunctionValueTypes(&defensive, types)
	types.ByIdentity[id] = mutatedFn
	types.ContextsByIdentity[id][0].Type = mutatedFn
	if defensive.funcTypes.ByIdentity[id] != originalFn {
		t.Fatalf("WithFunctionValueTypes reused caller map")
	}
	if defensive.funcTypes.ContextsByIdentity[id][0].Type != originalFn {
		t.Fatalf("WithFunctionValueTypes reused caller context slice")
	}

	types.ByIdentity[id] = originalFn
	types.ContextsByIdentity[id][0].Type = originalFn
	var owned Result
	WithOwnedFunctionValueTypes(&owned, types)
	types.ByIdentity[id] = mutatedFn
	types.ContextsByIdentity[id][0].Type = mutatedFn
	if owned.funcTypes.ByIdentity[id] != mutatedFn {
		t.Fatalf("WithOwnedFunctionValueTypes cloned caller-owned map")
	}
	if owned.funcTypes.ContextsByIdentity[id][0].Type != mutatedFn {
		t.Fatalf("WithOwnedFunctionValueTypes cloned caller-owned context slice")
	}
}

func TestFunctionValueTypesEqualComparesProjectionStructurally(t *testing.T) {
	reg := standard.Registry()
	id := identity.LuaFunction(981)
	pathKey := factflow.CalleePathKey("sym981.fn")
	fn := typ.Func().Param("name", typ.String).Returns(typ.Number).Build()
	otherFn := typ.Func().Param("name", typ.String).Returns(typ.Number).Build()
	leftKeys := keyspace.New()
	rightKeys := keyspace.New()
	if _, ok := leftKeys.FromPathKey(path.PathKey("sym981@1.decoy")); !ok {
		t.Fatal("left decoy path key did not parse")
	}
	contextPath := path.PathKey("sym981@1.ctx")
	contextValue := typevalue.LiteralString(reg, "ready")
	leftEntry := state.State{}.
		WriteValue(reg, statekey.ReturnSlot(0), product.Top()).
		WritePathKey(reg, leftKeys, contextPath, contextValue)
	rightEntry := state.State{}.
		WriteValue(reg, statekey.ReturnSlot(0), product.Top()).
		WritePathKey(reg, rightKeys, contextPath, contextValue)
	left := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{id: fn},
		ByPath:     map[factflow.CalleePathKey]*typ.Function{pathKey: fn},
		ParamSpansByPath: map[factflow.CalleePathKey][]factflow.SourceSpan{
			pathKey: {{StartLine: 1, StartCol: 2, EndLine: 1, EndCol: 6}},
		},
		ContextsByIdentity: map[identity.ID][]FunctionValueContext{
			id: {{Entry: leftEntry, EntryKeys: leftKeys, Type: fn}},
		},
	}
	right := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{id: otherFn},
		ByPath:     map[factflow.CalleePathKey]*typ.Function{pathKey: otherFn},
		ParamSpansByPath: map[factflow.CalleePathKey][]factflow.SourceSpan{
			pathKey: {{StartLine: 1, StartCol: 2, EndLine: 1, EndCol: 6}},
		},
		ContextsByIdentity: map[identity.ID][]FunctionValueContext{
			id: {{Entry: rightEntry, EntryKeys: rightKeys, Type: otherFn}},
		},
	}

	if !FunctionValueTypesEqual(reg, left, right) {
		t.Fatalf("FunctionValueTypesEqual rejected structurally equal projection")
	}

	right.ParamSpansByPath[pathKey] = []factflow.SourceSpan{{StartLine: 9}}
	if FunctionValueTypesEqual(reg, left, right) {
		t.Fatalf("FunctionValueTypesEqual accepted changed span projection")
	}
}

func TestResultHasFunctionValueTypesUsesStructuralEquality(t *testing.T) {
	result := &Result{registry: standard.Registry()}
	installed := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			identity.LuaFunction(982): typ.Func().Returns(typ.String).Build(),
		},
	}
	equivalent := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			identity.LuaFunction(982): typ.Func().Returns(typ.String).Build(),
		},
	}
	WithOwnedFunctionValueTypes(result, installed)

	if !result.HasFunctionValueTypes(equivalent) {
		t.Fatalf("HasFunctionValueTypes rejected structurally equal projection")
	}
}

func TestFunctionValueTypeForCallSiteAtBoundaryUsesCurrentPathValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(810)
	calleeSym := symbol.ID(8100)
	currentID := identity.LuaFunction(8101)
	staleID := identity.LuaFunction(8102)
	currentFn := typ.Func().Returns(typ.String).Build()
	staleFn := typ.Func().Returns(typ.Number).Build()
	calleePath := path.NewPath(calleeSym, "handler")
	calleeKey, ok := factflow.CalleePathKeyFromPath(calleePath)
	if !ok {
		t.Fatal("CalleePathKeyFromPath failed")
	}

	result := functionValueCallSiteResult(reg, point, calleeSym, identityValue(reg, currentID))
	WithOwnedFunctionValueTypes(result, FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			currentID: currentFn,
			staleID:   staleFn,
		},
		ByPath: map[factflow.CalleePathKey]*typ.Function{
			calleeKey: staleFn,
		},
	})

	got, ok := result.FunctionValueTypeForCallSiteAtBoundary(point, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: calleeSym,
		CalleePath:   calleePath,
	}))
	if !ok || got != currentFn {
		t.Fatalf("FunctionValueTypeForCallSiteAtBoundary = %v/%v, want current identity function", got, ok)
	}
}

func TestFunctionValueTypeForCallSiteAtBoundaryUsesGlobalTableOverridePathValue(t *testing.T) {
	reg := standard.Registry()
	result, err := CheckChunk(parseChunk(t, `
local captured_fn

_G.coroutine = {
    spawn = function(fn: () -> ())
        captured_fn = fn
        return true
    end,
}

coroutine.spawn(function() end)
`), Config{Registry: reg, Globals: []string{"coroutine"}})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	for _, point := range result.Graph().RPO() {
		site, ok := result.CallSite(point)
		if !ok || site.CalleePathRef().String() != "coroutine.spawn" {
			continue
		}
		fn, ok := result.FunctionValueTypeForCallSiteAtBoundary(point, site)
		if !ok || fn == nil || len(fn.Params) != 1 {
			t.Fatalf("FunctionValueTypeForCallSiteAtBoundary = %v/%v, want one-parameter override function", fn, ok)
		}
		return
	}
	t.Fatal("missing coroutine.spawn call site")
}

func TestFunctionValueTypeForCallSiteAtBoundaryRejectsStalePathWhenCurrentValueIsNotCallable(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(811)
	calleeSym := symbol.ID(8110)
	staleFn := typ.Func().Returns(typ.Number).Build()
	calleePath := path.NewPath(calleeSym, "handler")
	calleeKey, ok := factflow.CalleePathKeyFromPath(calleePath)
	if !ok {
		t.Fatal("CalleePathKeyFromPath failed")
	}

	result := functionValueCallSiteResult(reg, point, calleeSym, runtimeValue(reg, presence.Present(), runtimekind.String))
	WithOwnedFunctionValueTypes(result, FunctionValueTypes{
		ByPath: map[factflow.CalleePathKey]*typ.Function{
			calleeKey: staleFn,
		},
	})

	if got, ok := result.FunctionValueTypeForCallSiteAtBoundary(point, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: calleeSym,
		CalleePath:   calleePath,
	})); ok || got != nil {
		t.Fatalf("FunctionValueTypeForCallSiteAtBoundary = %v/%v, want stale path summary rejected", got, ok)
	}
}

func TestFunctionValueTypeForCallSiteAtBoundaryUsesCurrentMemberCallableWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(813)
	calleeSym := symbol.ID(8130)
	currentFn := typ.Func().Param("value", typ.String).Build()
	staleFn := typ.Func().Param("value", typ.Number).Build()
	calleePath := path.NewPath(calleeSym, "api").Field("send")
	calleeKey, ok := factflow.CalleePathKeyFromPath(calleePath)
	if !ok {
		t.Fatal("CalleePathKeyFromPath failed")
	}

	value := typevalue.WithWitness(reg, typevalue.FromType(reg, currentFn), currentFn)
	result := functionValueCallSitePathResult(t, reg, point, calleeSym, calleePath, value)
	WithOwnedFunctionValueTypes(result, FunctionValueTypes{
		ByPath: map[factflow.CalleePathKey]*typ.Function{
			calleeKey: staleFn,
		},
	})

	got, ok := result.FunctionValueTypeForCallSiteAtBoundary(point, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol:       calleeSym,
		CalleePath:         calleePath,
		CalleeMemberAccess: true,
	}))
	if !ok || got != currentFn {
		t.Fatalf("FunctionValueTypeForCallSiteAtBoundary = %v/%v, want current member callable witness", got, ok)
	}
}

func TestFunctionValueTypeForCallSiteAtBoundaryDoesNotUseDirectCallableWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(814)
	calleeSym := symbol.ID(8140)
	currentFn := typ.Func().Returns(typ.String).Build()
	calleePath := path.NewPath(calleeSym, "handler")
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, currentFn), currentFn)
	result := functionValueCallSiteResult(reg, point, calleeSym, value)

	got, ok := result.FunctionValueTypeForCallSiteAtBoundary(point, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: calleeSym,
		CalleePath:   calleePath,
	}))
	if ok || got != nil {
		t.Fatalf("FunctionValueTypeForCallSiteAtBoundary = %v/%v, want direct callable witness left unknown", got, ok)
	}
}

func TestFunctionValueTypeForCallSiteAtBoundaryFallsBackToCalleePathSummaryWhenCurrentValueMissing(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(812)
	calleeSym := symbol.ID(8120)
	fn := typ.Func().Returns(typ.Boolean).Build()
	calleePath := path.NewPath(calleeSym, "handler")
	calleeKey, ok := factflow.CalleePathKeyFromPath(calleePath)
	if !ok {
		t.Fatal("CalleePathKeyFromPath failed")
	}

	result := functionValueCallSiteResult(reg, point, calleeSym, product.Bottom(reg))
	WithOwnedFunctionValueTypes(result, FunctionValueTypes{
		ByPath: map[factflow.CalleePathKey]*typ.Function{
			calleeKey: fn,
		},
	})

	got, ok := result.FunctionValueTypeForCallSiteAtBoundary(point, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: calleeSym,
		CalleePath:   calleePath,
	}))
	if !ok || got != fn {
		t.Fatalf("FunctionValueTypeForCallSiteAtBoundary = %v/%v, want callee path summary", got, ok)
	}
}

func functionValueCallSiteResult(reg *axis.Registry, point cfg.Point, calleeSym symbol.ID, value product.Value) *Result {
	builder := visibility.NewBuilder()
	builder.Define(point, calleeSym, "handler")
	st := state.State{}
	if !product.Equal(reg, value, product.Bottom(reg)) {
		st = st.WriteValue(reg, statekey.SymbolValue(calleeSym), value)
	}
	return &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		flow: transfer.Result{
			point: st,
		},
	}
}

func functionValueCallSitePathResult(t *testing.T, reg *axis.Registry, point cfg.Point, calleeSym symbol.ID, calleePath path.Path, value product.Value) *Result {
	t.Helper()
	builder := visibility.NewBuilder()
	builder.Define(point, calleeSym, calleePath.Root)
	resolver := visibility.NewResolver(builder.Build())
	stateKey, ok := resolver.StateKeyAt(point, calleePath)
	if !ok {
		t.Fatalf("StateKeyAt(%s) failed", calleePath.String())
	}
	return &Result{
		registry:   reg,
		visibility: resolver,
		flow: transfer.Result{
			point: state.State{}.WritePathKey(reg, resolver.KeySpace(), stateKey.PathKey(), value),
		},
	}
}

func heapTableValue(reg *axis.Registry, id identity.ID) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}

func runtimeValue(reg *axis.Registry, p presence.Value, tag runtimekind.Tag) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, p)
	return product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(tag))
}

func identityValue(reg *axis.Registry, id identity.ID) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}

func nameStaticKey(t *testing.T, ks *keyspace.KeySpace, name string) keyspace.Key {
	t.Helper()
	k, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: name}})
	if !ok {
		t.Fatalf("FromRootlessSuffix(%q) failed", name)
	}
	return k
}
