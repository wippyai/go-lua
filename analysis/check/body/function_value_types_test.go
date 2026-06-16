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

func heapTableValue(reg *axis.Registry, id identity.ID) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}
