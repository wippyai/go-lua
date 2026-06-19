package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFunctionContextEntryHoldsAllowsExtraCurrentHeapFacts(t *testing.T) {
	reg := standard.Registry()
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
		StaticMembers: map[path.PathKey]product.Value{
			path.PathKey(".name"): memberValue,
		},
	}))
	current := entry.WriteHeapTableObject(reg, extraID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: extraRoot,
	}))

	if !functionContextEntryHolds(reg, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned false, want extra current heap facts tolerated")
	}
}

func TestFunctionContextEntryHoldsRejectsMissingRequiredHeapFacts(t *testing.T) {
	reg := standard.Registry()
	requiredID := identity.LuaTableLiteral(7100, 3)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: heapTableValue(reg, requiredID),
	}))

	if functionContextEntryHolds(reg, entry, state.State{}, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want missing required heap facts rejected")
	}
}

func TestFunctionContextEntryHoldsRejectsWidenedPathRefinement(t *testing.T) {
	reg := standard.Registry()
	pathKey := path.PathKey("sym71@1.value")
	entry := state.State{}.WritePathKey(reg, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))
	current := state.State{}.WritePathKey(reg, pathKey, runtimeValue(reg, presence.Maybe(), runtimekind.String))

	if functionContextEntryHolds(reg, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want widened current path refinement rejected")
	}

	morePrecise := state.State{}.WritePathKey(reg, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))
	if !functionContextEntryHolds(reg, entry, morePrecise, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds rejected matching current path refinement")
	}
}

func TestFunctionContextEntryHoldsAllowsMissingSelfIdentityRefinement(t *testing.T) {
	reg := standard.Registry()
	sourceID := identity.LuaFunction(71)
	entry := state.State{}.WritePathKey(reg, path.PathKey("sym71@1"), identityValue(reg, sourceID))

	if !functionContextEntryHolds(reg, entry, state.State{}, sourceID) {
		t.Fatalf("functionContextEntryHolds rejected missing refinement for source function identity")
	}
}

func TestFunctionContextEntryHoldsRejectsMissingNonSelfIdentityRefinement(t *testing.T) {
	reg := standard.Registry()
	entry := state.State{}.WritePathKey(reg, path.PathKey("sym72@1"), identityValue(reg, identity.LuaFunction(72)))

	if functionContextEntryHolds(reg, entry, state.State{}, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds accepted missing refinement for non-source identity")
	}
}

func TestFunctionContextEntryHoldsRejectsTopIdentityForSpecificPathRefinement(t *testing.T) {
	reg := standard.Registry()
	pathKey := path.PathKey("sym73@1.service")
	requiredID := identity.LuaTableLiteral(7300, 1)
	entry := state.State{}.WritePathKey(reg, pathKey, heapTableValue(reg, requiredID))
	current := state.State{}.WritePathKey(reg, pathKey, runtimeValue(reg, presence.Present(), runtimekind.Table))

	if functionContextEntryHolds(reg, entry, current, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds accepted identity-top current value for specific required identity")
	}

	matching := state.State{}.WritePathKey(reg, pathKey, heapTableValue(reg, requiredID))
	if !functionContextEntryHolds(reg, entry, matching, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds rejected matching specific identity")
	}
}

func TestFunctionContextEntryHoldsRejectsTopRuntimeKindForSpecificPathRefinement(t *testing.T) {
	reg := standard.Registry()
	pathKey := path.PathKey("sym74@1.value")
	entry := state.State{}.WritePathKey(reg, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))
	current := state.State{}.WritePathKey(reg, pathKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	if functionContextEntryHolds(reg, entry, current, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds accepted runtime-kind top current value for specific required kind")
	}
}

func TestFunctionContextEntryHoldsAllowsMorePreciseCurrentPathRefinement(t *testing.T) {
	reg := standard.Registry()
	pathKey := path.PathKey("sym75@1.value")
	entry := state.State{}.WritePathKey(reg, pathKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	current := state.State{}.WritePathKey(reg, pathKey, runtimeValue(reg, presence.Present(), runtimekind.String))

	if !functionContextEntryHolds(reg, entry, current, identity.LuaFunction(71)) {
		t.Fatalf("functionContextEntryHolds rejected more precise current path refinement")
	}
}

func TestFunctionContextEntryHoldsRejectsMissingRequiredHeapStaticMember(t *testing.T) {
	reg := standard.Registry()
	requiredID := identity.LuaTableLiteral(7100, 4)
	requiredRoot := heapTableValue(reg, requiredID)
	memberValue := runtimeValue(reg, presence.Present(), runtimekind.String)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
		StaticMembers: map[path.PathKey]product.Value{
			path.PathKey(".name"): memberValue,
		},
	}))
	current := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
	}))

	if functionContextEntryHolds(reg, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want missing required heap static member rejected")
	}
}

func TestFunctionContextEntryHoldsRejectsWidenedHeapStaticMember(t *testing.T) {
	reg := standard.Registry()
	requiredID := identity.LuaTableLiteral(7100, 5)
	requiredRoot := heapTableValue(reg, requiredID)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
		StaticMembers: map[path.PathKey]product.Value{
			path.PathKey(".name"): runtimeValue(reg, presence.Present(), runtimekind.String),
		},
	}))
	current := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: requiredRoot,
		StaticMembers: map[path.PathKey]product.Value{
			path.PathKey(".name"): runtimeValue(reg, presence.Maybe(), runtimekind.String),
		},
	}))

	if functionContextEntryHolds(reg, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want widened heap static member rejected")
	}
}

func TestFunctionContextEntryHoldsRejectsTopCurrentHeapWhenHeapFactsRequired(t *testing.T) {
	reg := standard.Registry()
	requiredID := identity.LuaTableLiteral(7100, 6)
	entry := state.State{}.WriteHeapTableObject(reg, requiredID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: heapTableValue(reg, requiredID),
	}))
	current := state.Domain(reg).Top()

	if functionContextEntryHolds(reg, entry, current, identity.LuaFunction(1)) {
		t.Fatalf("functionContextEntryHolds returned true, want top current heap rejected when entry requires finite heap facts")
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
