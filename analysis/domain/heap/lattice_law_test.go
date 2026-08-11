package heap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestHeapWidenKeepsJoinExactAndNamesTheMuLoss exercises the one permitted
// Heap precision boundary. Ordinary Join retains two incompatible complete
// Worlds. Widen coalesces them only at Mu, remains an upper bound, and its
// fixed rank strictly descends. Consumers may derive universal conclusions
// from the coalesced state, but an exact witness needs independent evidence
// that Widen equalled Join.
func TestHeapWidenKeepsJoinExactAndNamesTheMuLoss(t *testing.T) {
	schema, key, meta := heapLatticeFixture(t)
	leftObject := heapLatticeObject(t, schema, ShapeEligible, FrozenMutable, noneContainment(t, schema))
	rightObject := heapLatticeObject(t, schema, ShapeIneligible, FrozenFrozen, exactContainment(t, schema, meta))
	leftWorld, ok := schema.One(key, leftObject)
	if !ok {
		t.Fatal("left one world")
	}
	rightWorld, ok := schema.One(key, rightObject)
	if !ok {
		t.Fatal("right one world")
	}
	left, ok := schema.Relation(key, leftWorld)
	if !ok {
		t.Fatal("left relation")
	}
	right, ok := schema.Relation(key, rightWorld)
	if !ok {
		t.Fatal("right relation")
	}

	joined, ok := Join(left, right)
	if !ok || joined.WorldCount() != 2 {
		t.Fatal("ordinary Join erased a same-family correlated alternative")
	}
	first, firstOK := joined.WorldAt(0)
	second, secondOK := joined.WorldAt(1)
	if !firstOK || !secondOK || first.Kind() != WorldOne || second.Kind() != WorldOne {
		t.Fatal("Join did not retain two complete One worlds")
	}
	for _, world := range []World{first, second} {
		object, objectOK := world.Recent()
		shape, frozen, headerOK := object.Header()
		if !objectOK || !headerOK || shape != ShapeEligible && shape != ShapeIneligible || frozen != FrozenMutable && frozen != FrozenFrozen {
			t.Fatal("Join split a complete object header")
		}
	}

	widened, ok := Widen(left, right)
	if !ok || widened.WorldCount() != 1 || Equal(joined, widened) {
		t.Fatal("Mu widening did not make its loss of same-family correlation observable")
	}
	if !LessOrEq(left, widened) || !LessOrEq(right, widened) {
		t.Fatal("Mu widening failed to cover its exact Join")
	}
	world, worldOK := widened.WorldAt(0)
	recent, recentOK := world.Recent()
	shape, frozen, headerOK := recent.Header()
	if !worldOK || !recentOK || !headerOK || shape != ShapeEligible|ShapeIneligible || frozen != FrozenMutable|FrozenFrozen ||
		!recent.MayHaveNoMetatable() || recent.MetatableCount() != 1 {
		t.Fatal("Mu widening did not retain the complete joined may state")
	}

	rank, rankOK := NewWidenRank(schema)
	if !rankOK || rank.Width() != 3 {
		t.Fatal("sealed complete-world rank")
	}
	assertStrictRankDescent(t, rank, key, left, widened)
	idempotent, idempotentOK := Widen(joined, joined)
	if !idempotentOK || !Equal(idempotent, joined) {
		t.Fatal("Widen(v,v) rewrote an ordinary same-family antichain")
	}
}

// TestHeapAdmittedOrderFastPathsPreserveSemanticParity keeps the immutable
// representation shortcut separate from semantic equality. A copied Value
// shares its sealed world slice and must take the O(1) Same path; a separately
// issued, same-content relation must not, while Equal/LessOrEq/Join remain
// identical. The two metatable alternatives also ensure the order path does
// not discard the unknown-meta bit while bypassing nested validation.
func TestHeapAdmittedOrderFastPathsPreserveSemanticParity(t *testing.T) {
	schema, key, _ := heapLatticeFixture(t)
	none := noneContainment(t, schema)
	unknown, unknownOK := schema.ContainmentUnknown()
	if !unknownOK {
		t.Fatal("unknown metatable containment")
	}
	noneObject := heapLatticeObject(t, schema, ShapeEligible, FrozenMutable, none)
	unknownObject := heapLatticeObject(t, schema, ShapeEligible, FrozenMutable, unknown)
	noneWorld, noneWorldOK := schema.One(key, noneObject)
	unknownWorld, unknownWorldOK := schema.One(key, unknownObject)
	if !noneWorldOK || !unknownWorldOK {
		t.Fatal("metatable worlds")
	}
	noneValue, noneValueOK := schema.Relation(key, noneWorld)
	unknownValue, unknownValueOK := schema.Relation(key, unknownWorld)
	if !noneValueOK || !unknownValueOK {
		t.Fatal("metatable relations")
	}

	copied := noneValue
	if !Same(noneValue, copied) || !Equal(noneValue, copied) || !LessOrEq(noneValue, copied) || !LessOrEq(copied, noneValue) {
		t.Fatal("copied admitted Value did not use the identity fast path")
	}
	identityJoin, identityJoinOK := Join(noneValue, copied)
	if !identityJoinOK || !Same(identityJoin, noneValue) {
		t.Fatal("Join rewrote an identical admitted Value")
	}

	reissued, reissuedOK := schema.Relation(key, noneWorld)
	if !reissuedOK || Same(noneValue, reissued) || !Equal(noneValue, reissued) || !LessOrEq(noneValue, reissued) || !LessOrEq(reissued, noneValue) {
		t.Fatal("independently issued equal relation did not preserve semantic parity")
	}

	joined, joinedOK := Join(noneValue, unknownValue)
	if !joinedOK || joined.WorldCount() != 2 || !LessOrEq(noneValue, joined) || !LessOrEq(unknownValue, joined) {
		t.Fatal("Join discarded a distinct metatable alternative")
	}
	if LessOrEq(unknownValue, noneValue) {
		t.Fatal("unknown metatable was incorrectly ordered below no metatable")
	}

	if !Same(schema.Bottom(), schema.Bottom()) || !Same(schema.Top(), schema.Top()) {
		t.Fatal("bottom/top did not retain O(1) identity semantics")
	}
}

// TestHeapManyWidenDoesNotSplitRecentSummaryBeforeMu proves that Many is a
// single paired control world. Join retains both paired histories; only Mu
// coalesces their role-local states, never turns Recent and Summary into two
// independent factor values.
func TestHeapManyWidenPreservesPairedWorldsUntilMu(t *testing.T) {
	schema, key, meta := heapLatticeFixture(t)
	mutable := heapLatticeObject(t, schema, ShapeEligible, FrozenMutable, noneContainment(t, schema))
	frozen := heapLatticeObject(t, schema, ShapeIneligible, FrozenFrozen, exactContainment(t, schema, meta))
	firstWorld, firstOK := schema.Many(key, mutable, frozen)
	secondWorld, secondOK := schema.Many(key, frozen, mutable)
	if !firstOK || !secondOK {
		t.Fatal("paired Many worlds")
	}
	first, firstOK := schema.Relation(key, firstWorld)
	second, secondOK := schema.Relation(key, secondWorld)
	if !firstOK || !secondOK {
		t.Fatal("paired Many relations")
	}
	joined, joinOK := Join(first, second)
	if !joinOK || joined.WorldCount() != 2 {
		t.Fatal("Join erased Recent/Summary history pairing")
	}
	widened, widenOK := Widen(first, second)
	if !widenOK || widened.WorldCount() != 1 || !LessOrEq(first, widened) || !LessOrEq(second, widened) {
		t.Fatal("Mu widening did not cover complete paired histories")
	}
	world, worldOK := widened.WorldAt(0)
	recent, recentOK := world.Recent()
	summary, summaryOK := world.Summary()
	if !worldOK || !recentOK || !summaryOK {
		t.Fatal("Many lost one of its simultaneous role objects")
	}
	for _, object := range []Object{recent, summary} {
		shape, frozenState, headerOK := object.Header()
		if !headerOK || shape != ShapeEligible|ShapeIneligible || frozenState != FrozenMutable|FrozenFrozen {
			t.Fatal("Mu coalescence failed to preserve a complete role-local may state")
		}
	}
	rank, rankOK := NewWidenRank(schema)
	if !rankOK {
		t.Fatal("Many rank")
	}
	assertStrictRankDescent(t, rank, key, first, widened)
}

func assertStrictRankDescent(t testing.TB, rank WidenRank, key Key, before, after Value) {
	t.Helper()
	for component := 0; component < rank.Width(); component++ {
		beforeRank := rank.At(key, before, component)
		afterRank := rank.At(key, after, component)
		switch {
		case afterRank < beforeRank:
			return
		case afterRank > beforeRank:
			t.Fatalf("rank ascended at component %d: %d -> %d", component, beforeRank, afterRank)
		}
	}
	t.Fatal("changed Mu widening did not descend")
}

func heapLatticeFixture(t testing.TB) (Schema, Key, Reference) {
	t.Helper()
	program, err := programlower.Lower(programlower.Source{Name: "heap_lattice.lua", Text: []byte(`
local first = {}
local second = {}
return first
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: "heap_lattice", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := Seal(linked)
	if !ok {
		t.Fatal("sealed lattice fixture")
	}
	var keys []Key
	for index := 0; index < schema.KeyCount(); index++ {
		candidate, candidateOK := schema.KeyAt(index)
		if candidateOK && candidate.Kind() == RootAllocation {
			keys = append(keys, candidate)
		}
	}
	if len(keys) < 2 {
		t.Fatal("fixture omitted two Heap allocation keys")
	}
	key, other := keys[0], keys[1]
	meta, metaOK := schema.Reference(other, materialization.Recent)
	if !metaOK {
		t.Fatal("fixture Heap roots")
	}
	return schema, key, meta
}

func heapLatticeObject(t testing.TB, schema Schema, shape Shape, frozen Frozen, metatable Containment) Object {
	t.Helper()
	object, ok := schema.Object(shape, frozen, metatable)
	if !ok {
		t.Fatal("Heap object")
	}
	return object
}
