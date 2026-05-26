package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDependencyMap_Register(t *testing.T) {
	dm := make(dependencyMap)

	dm.register("key1", 1)
	dm.register("key1", 2)
	dm.register("key2", 3)
	dm.register("", 4) // empty key should be ignored

	if len(dm["key1"]) != 2 {
		t.Errorf("key1 should have 2 points, got %d", len(dm["key1"]))
	}
	if len(dm["key2"]) != 1 {
		t.Errorf("key2 should have 1 point, got %d", len(dm["key2"]))
	}
	if _, exists := dm[""]; exists {
		t.Error("empty key should not be registered")
	}
}

func TestAddDependentPoints(t *testing.T) {
	deps := make(dependencyMap)
	deps["key1"] = []cfg.Point{1, 2}
	deps["key2"] = []cfg.Point{3}

	worklist := []cfg.Point{10}
	inQueue := map[cfg.Point]bool{10: true}

	changedKeys := []string{"key1", "key2"}
	worklist = addDependentPoints(deps, changedKeys, worklist, inQueue)

	if len(worklist) != 4 {
		t.Errorf("expected 4 points in worklist, got %d", len(worklist))
	}

	// All points should be in queue
	for _, p := range []cfg.Point{1, 2, 3, 10} {
		if !inQueue[p] {
			t.Errorf("point %d should be in queue", p)
		}
	}
}

func TestAddDependentPoints_NoDuplicates(t *testing.T) {
	deps := make(dependencyMap)
	deps["key1"] = []cfg.Point{1, 2}

	worklist := []cfg.Point{1} // 1 is already in worklist
	inQueue := map[cfg.Point]bool{1: true}

	changedKeys := []string{"key1"}
	worklist = addDependentPoints(deps, changedKeys, worklist, inQueue)

	// Should only add point 2, not duplicate 1
	if len(worklist) != 2 {
		t.Errorf("expected 2 points (no duplicate), got %d", len(worklist))
	}
}

func TestAddDependentPoints_UnknownKey(t *testing.T) {
	deps := make(dependencyMap)
	deps["key1"] = []cfg.Point{1}

	var worklist []cfg.Point
	inQueue := map[cfg.Point]bool{}

	changedKeys := []string{"unknown_key"}
	worklist = addDependentPoints(deps, changedKeys, worklist, inQueue)

	if len(worklist) != 0 {
		t.Errorf("expected 0 points for unknown key, got %d", len(worklist))
	}
}

func TestAddDependentPoints_DeterministicOrder(t *testing.T) {
	deps := make(dependencyMap)
	deps["key2"] = []cfg.Point{4, 1}
	deps["key1"] = []cfg.Point{3, 2}

	var worklist []cfg.Point
	inQueue := map[cfg.Point]bool{}

	changedKeys := []string{"key2", "key1"}
	worklist = addDependentPoints(deps, changedKeys, worklist, inQueue)

	expected := []cfg.Point{1, 2, 3, 4}
	if len(worklist) != len(expected) {
		t.Fatalf("expected %d points, got %d", len(expected), len(worklist))
	}
	for i, got := range worklist {
		if got != expected[i] {
			t.Fatalf("worklist[%d] = %d, want %d", i, got, expected[i])
		}
	}
}

func TestSymbolTypeSource(t *testing.T) {
	// Test that symbolTypeSource struct has correct field defaults
	src := symbolTypeSource{}

	// Default should be false for boolean fields
	if src.tryDeclPoint {
		t.Error("tryDeclPoint should default to false")
	}
	if src.skipIfExists {
		t.Error("skipIfExists should default to false")
	}
	if src.types != nil {
		t.Error("types should default to nil")
	}
}

func TestSetValueInvalidatesNarrowedTypeCache(t *testing.T) {
	s := &Solution{
		values: liftFlowValues(nil),
		narrowedTypeCache: map[narrowedTypeCacheKey]narrowedTypeCacheValue{
			{point: 1, path: constraint.PathKey("sym1@1")}: {t: typ.String, ok: true},
		},
	}

	s.setValue("sym1@1", typ.Number)

	if len(s.narrowedTypeCache) != 0 {
		t.Fatalf("setValue left %d stale narrowed-type cache entries", len(s.narrowedTypeCache))
	}
}

func TestBuildPointValueMapUsesCurrentStateIndependentOfQueryCache(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	entry := c.Entry()

	symX := setupSymbol(g, "x", []cfg.Point{entry})
	symY := setupSymbol(g, "y", []cfg.Point{entry})
	symZ := setupSymbol(g, "z", []cfg.Point{entry})
	setVersion(g, entry, symX, cfg.Version{Root: "x", Symbol: symX, ID: 1})
	setVersion(g, entry, symY, cfg.Version{Root: "y", Symbol: symY, ID: 1})
	setVersion(g, entry, symZ, cfg.Version{Root: "z", Symbol: symZ, ID: 1})

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Boolean
	inputs.DeclaredTypes[symY] = typ.Any
	inputs.DeclaredTypes[symZ] = typ.Number

	s := &Solution{
		inputs:     inputs,
		resolver:   testResolver(),
		pkResolver: pathkey.NewResolver(g),
		values: liftFlowValues(nil),
	}
	pathX := constraint.Path{Root: "x", Symbol: symX}
	pathY := constraint.Path{Root: "y", Symbol: symY}
	yKey := s.pkResolver.KeyAt(entry, pathY)
	zKey := s.pkResolver.KeyAt(entry, constraint.Path{Root: "z", Symbol: symZ})
	s.setValue(string(yKey), typ.String)

	for _, cacheEnabled := range []bool{false, true} {
		s.queryCacheEnabled = cacheEnabled
		values := s.buildPointValueMap(entry, pathX, typ.Boolean, []constraint.Constraint{
			constraint.NotNil{Path: pathY},
		})
		if got := values[yKey]; got != typ.String {
			t.Fatalf("queryCacheEnabled=%v: y value = %v, want current state string", cacheEnabled, got)
		}
		if _, ok := values[zKey]; ok {
			t.Fatalf("queryCacheEnabled=%v: unreferenced z was materialized in point value map", cacheEnabled)
		}
	}
}

func TestDependencyMap_Empty(t *testing.T) {
	dm := make(dependencyMap)

	// Register with empty key should be no-op
	dm.register("", 1)

	if len(dm) != 0 {
		t.Error("empty key registration should not add entries")
	}
}
