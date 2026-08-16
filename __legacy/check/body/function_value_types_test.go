package body

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

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
	}

	clone := cloneFunctionValueTypes(types)
	types.ByIdentity[id] = mutatedFn
	types.ByPath[key] = mutatedFn

	if clone.ByIdentity[id] != originalFn {
		t.Fatalf("clone ByIdentity changed after source map mutation")
	}
	if clone.ByPath[key] != originalFn {
		t.Fatalf("clone ByPath changed after source map mutation")
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
	}

	var defensive Result
	WithFunctionValueTypes(&defensive, types)
	types.ByIdentity[id] = mutatedFn
	if defensive.funcTypes.ByIdentity[id] != originalFn {
		t.Fatalf("WithFunctionValueTypes reused caller map")
	}

	types.ByIdentity[id] = originalFn
	var owned Result
	WithOwnedFunctionValueTypes(&owned, types)
	types.ByIdentity[id] = mutatedFn
	if owned.funcTypes.ByIdentity[id] != mutatedFn {
		t.Fatalf("WithOwnedFunctionValueTypes cloned caller-owned map")
	}
}

func TestFunctionValueTypesEqualComparesProjectionStructurally(t *testing.T) {
	id := identity.LuaFunction(981)
	pathKey := factflow.CalleePathKey("sym981.fn")
	fn := typ.Func().Param("name", typ.String).Returns(typ.Number).Build()
	otherFn := typ.Func().Param("name", typ.String).Returns(typ.Number).Build()
	left := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{id: fn},
		ByPath:     map[factflow.CalleePathKey]*typ.Function{pathKey: fn},
		ParamSpansByPath: map[factflow.CalleePathKey][]factflow.SourceSpan{
			pathKey: {{StartLine: 1, StartCol: 2, EndLine: 1, EndCol: 6}},
		},
	}
	right := FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{id: otherFn},
		ByPath:     map[factflow.CalleePathKey]*typ.Function{pathKey: otherFn},
		ParamSpansByPath: map[factflow.CalleePathKey][]factflow.SourceSpan{
			pathKey: {{StartLine: 1, StartCol: 2, EndLine: 1, EndCol: 6}},
		},
	}

	if !FunctionValueTypesEqual(left, right) {
		t.Fatalf("FunctionValueTypesEqual rejected structurally equal projection")
	}

	right.ParamSpansByPath[pathKey] = []factflow.SourceSpan{{StartLine: 9}}
	if FunctionValueTypesEqual(left, right) {
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

func TestSealedFunctionValueTypesUseIdentityWithoutWeakeningStructuralFallback(t *testing.T) {
	id := identity.LuaFunction(983)
	stringFn := typ.Func().Returns(typ.String).Build()
	numberFn := typ.Func().Returns(typ.Number).Build()

	shared := SealFunctionValueTypes(FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{id: stringFn},
	})
	if !FunctionValueTypesEqual(shared, shared) {
		t.Fatal("copies of one sealed immutable projection are not equal")
	}

	// A distinct seal must retain the structural fallback: sealing is not a
	// content hash and cannot make different projections compare equal.
	different := SealFunctionValueTypes(FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{id: numberFn},
	})
	if FunctionValueTypesEqual(shared, different) {
		t.Fatal("distinct sealed projections bypassed structural comparison")
	}

	equivalent := SealFunctionValueTypes(FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			id: typ.Func().Returns(typ.String).Build(),
		},
	})
	if !FunctionValueTypesEqual(shared, equivalent) {
		t.Fatal("distinct structurally equal sealed projections were rejected")
	}
}

func TestCanonicalFunctionValueTypesReusesInstalledImmutableProjection(t *testing.T) {
	reg := standard.Registry()
	id := identity.LuaFunction(984)
	installed := SealFunctionValueTypes(FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			id: typ.Func().Returns(typ.String).Build(),
		},
	})
	result := &Result{registry: reg}
	WithOwnedFunctionValueTypes(result, installed)

	equivalent := SealFunctionValueTypes(FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			id: typ.Func().Returns(typ.String).Build(),
		},
	})
	canonical, ok := result.CanonicalFunctionValueTypes(equivalent)
	if !ok || canonical.identity != installed.identity {
		t.Fatal("equivalent projection did not reuse installed immutable identity")
	}

	changed := SealFunctionValueTypes(FunctionValueTypes{
		ByIdentity: map[identity.ID]*typ.Function{
			id: typ.Func().Returns(typ.Number).Build(),
		},
	})
	if _, ok := result.CanonicalFunctionValueTypes(changed); ok {
		t.Fatal("changed projection reused installed immutable identity")
	}
}

func BenchmarkFunctionValueTypesEqualSharedMaterializationProjection(b *testing.B) {
	const functionCount = 2048
	types := FunctionValueTypes{ByIdentity: make(map[identity.ID]*typ.Function, functionCount)}
	fn := typ.Func().Param("value", typ.String).Returns(typ.Number).Build()
	for i := 0; i < functionCount; i++ {
		types.ByIdentity[identity.LuaFunction(uint64(i+1))] = fn
	}
	types = SealFunctionValueTypes(types)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !FunctionValueTypesEqual(types, types) {
			b.Fatal("shared projection changed")
		}
	}
}

func BenchmarkFunctionValueTypesEqualIndependentProjection(b *testing.B) {
	const functionCount = 2048
	types := FunctionValueTypes{ByIdentity: make(map[identity.ID]*typ.Function, functionCount)}
	fn := typ.Func().Param("value", typ.String).Returns(typ.Number).Build()
	for i := 0; i < functionCount; i++ {
		types.ByIdentity[identity.LuaFunction(uint64(i+1))] = fn
	}
	left := SealFunctionValueTypes(types)
	right := types
	right.identity = &functionValueTypesIdentity{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !FunctionValueTypesEqual(left, right) {
			b.Fatal("equivalent independent projection changed")
		}
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
	}).View())
	if !ok || got != currentFn {
		t.Fatalf("FunctionValueTypeForCallSiteAtBoundary = %v/%v, want current identity function", got, ok)
	}
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
	}).View()); ok || got != nil {
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
	}).View())
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
	}).View())
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
	}).View())
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

func runtimeValue(reg *axis.Registry, p presence.Value, tag runtimekind.Tag) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, p)
	return product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(tag))
}

func identityValue(reg *axis.Registry, id identity.ID) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}
