package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
)

func TestAgeMovesRecentToSummaryAndMergesCapabilities(t *testing.T) {
	schema, linked := correlatedFixtureWithCapabilityCount(t, "local a = {}; return a", 2)
	root := allocationKeyAt(t, schema, 0)
	first, firstOK := linked.Host().Capabilities().At(0)
	second, secondOK := linked.Host().Capabilities().At(1)
	if !firstOK || !secondOK {
		t.Fatal("age fixture omitted root or capabilities")
	}
	recent := mustAgeAllocation(t, schema, root, materialization.Recent)
	summary := mustAgeAllocation(t, schema, root, materialization.Summary)
	recentValue, recentOK := schema.WithCapability(mustCorrelatedSingleton(t, schema, recent), recent, first)
	summaryValue, summaryOK := schema.WithCapability(mustCorrelatedSingleton(t, schema, summary), summary, second)
	if !recentOK || !summaryOK {
		t.Fatal("attach age capabilities")
	}
	input, joinOK := schema.Join(recentValue, summaryValue)
	if !joinOK {
		t.Fatal("join age inputs")
	}
	aged, ageOK := schema.Age(input, root)
	if !ageOK {
		t.Fatal("age rooted relation")
	}
	want, wantOK := schema.WithCapability(summaryValue, summary, first)
	if !wantOK || !schema.Equal(aged, want) {
		t.Fatalf("Age = %x, want merged summary %x", schema.Fingerprint(aged), schema.Fingerprint(want))
	}
	if !schema.HasCapability(aged, summary, first) || !schema.HasCapability(aged, summary, second) || schema.HasCapability(aged, recent, first) {
		t.Fatal("Age lost, smeared, or retained a capability")
	}
	atoms, atomsOK := schema.Atoms(aged)
	if !atomsOK || len(atoms) != 1 || atoms[0] != summary {
		t.Fatalf("Age alternatives = %v/%v, want sole summary", atoms, atomsOK)
	}

	again, againOK := schema.Age(aged, root)
	if !againOK || !schema.Equal(again, aged) {
		t.Fatal("Age is not idempotent")
	}
}

func TestAgePreservesUnrelatedRootsAndLatticeLaws(t *testing.T) {
	schema, _ := freshAllocationFixture(t)
	firstRoot := allocationKeyAt(t, schema, 0)
	secondRoot := allocationKeyAt(t, schema, 1)
	if firstRoot == secondRoot {
		t.Fatal("fresh fixture omitted distinct allocation roots")
	}
	firstRecent := mustAgeAllocation(t, schema, firstRoot, materialization.Recent)
	firstSummary := mustAgeAllocation(t, schema, firstRoot, materialization.Summary)
	secondRecent := mustAgeAllocation(t, schema, secondRoot, materialization.Recent)
	firstRecentValue := mustCorrelatedSingleton(t, schema, firstRecent)
	firstSummaryValue := mustCorrelatedSingleton(t, schema, firstSummary)
	secondRecentValue := mustCorrelatedSingleton(t, schema, secondRecent)
	mixed, mixedOK := schema.Join(firstRecentValue, secondRecentValue)
	if !mixedOK {
		t.Fatal("join distinct allocation roots")
	}
	agedMixed, ageOK := schema.Age(mixed, firstRoot)
	if !ageOK {
		t.Fatal("age first root")
	}
	wantMixed, wantOK := schema.Join(firstSummaryValue, secondRecentValue)
	if !wantOK || !schema.Equal(agedMixed, wantMixed) {
		t.Fatal("Age changed an unrelated identity")
	}

	joined, joinedOK := schema.Join(firstRecentValue, firstSummaryValue)
	if !joinedOK {
		t.Fatal("join representative values")
	}
	samples := []Value{
		schema.Default(),
		firstRecentValue,
		firstSummaryValue,
		secondRecentValue,
		joined,
		mixed,
		schema.Top(),
	}
	for leftIndex, left := range samples {
		agedLeft, leftOK := schema.Age(left, firstRoot)
		if !leftOK {
			t.Fatalf("Age representative %d", leftIndex)
		}
		for rightIndex, right := range samples {
			agedRight, rightOK := schema.Age(right, firstRoot)
			if !rightOK {
				t.Fatalf("Age representative %d", rightIndex)
			}
			if schema.LessOrEq(left, right) && !schema.LessOrEq(agedLeft, agedRight) {
				t.Fatalf("Age not monotone for %d <= %d", leftIndex, rightIndex)
			}
			join, joinOK := schema.Join(left, right)
			if !joinOK {
				t.Fatalf("join representatives %d/%d", leftIndex, rightIndex)
			}
			agedJoin, ageJoinOK := schema.Age(join, firstRoot)
			if !ageJoinOK {
				t.Fatalf("Age joined representatives %d/%d", leftIndex, rightIndex)
			}
			joinedAges, joinedAgesOK := schema.Join(agedLeft, agedRight)
			if !joinedAgesOK || !schema.Equal(agedJoin, joinedAges) {
				t.Fatalf("Age did not preserve join for %d/%d", leftIndex, rightIndex)
			}
		}
	}
}

func TestAgeFencesForeignOwnersAndPreservesExtremes(t *testing.T) {
	schema, _ := freshAllocationFixture(t)
	root := allocationKeyAt(t, schema, 0)
	for _, input := range []Value{schema.Default(), schema.Top()} {
		aged, ageOK := schema.Age(input, root)
		if !ageOK || !schema.Same(aged, input) {
			t.Fatal("Age changed a lattice extreme")
		}
	}
	other, _ := freshAllocationFixture(t)
	foreignRoot := allocationKeyAt(t, other, 0)
	if _, ok := schema.Age(other.Top(), root); ok {
		t.Fatal("Age admitted a foreign Value")
	}
	if _, ok := schema.Age(schema.Top(), foreignRoot); ok {
		t.Fatal("Age admitted a foreign allocation root")
	}
	if _, ok := schema.Age(Value{}, root); ok {
		t.Fatal("Age admitted an invalid Value")
	}
	var zeroRoot heap.Key
	if _, ok := schema.Age(schema.Top(), zeroRoot); ok {
		t.Fatal("Age admitted a zero allocation root")
	}
}

func TestAgeWarmAllocationShape(t *testing.T) {
	schema, linked := correlatedFixtureWithCapabilityCount(t, "local a = {}; return a", 2)
	root := allocationKeyAt(t, schema, 0)
	first, firstOK := linked.Host().Capabilities().At(0)
	second, secondOK := linked.Host().Capabilities().At(1)
	if !firstOK || !secondOK {
		t.Fatal("warm age fixture")
	}
	recent := mustAgeAllocation(t, schema, root, materialization.Recent)
	summary := mustAgeAllocation(t, schema, root, materialization.Summary)
	recentValue, recentOK := schema.WithCapability(mustCorrelatedSingleton(t, schema, recent), recent, first)
	summaryValue, summaryOK := schema.WithCapability(mustCorrelatedSingleton(t, schema, summary), summary, second)
	if !recentOK || !summaryOK {
		t.Fatal("warm age capabilities")
	}
	input, inputOK := schema.Join(recentValue, summaryValue)
	if !inputOK {
		t.Fatal("warm age join")
	}
	var sink Value
	if allocations := testing.AllocsPerRun(1_000, func() {
		var ok bool
		sink, ok = schema.Age(schema.Top(), root)
		if !ok {
			t.Fatal("Age Top")
		}
		sink, ok = schema.Age(schema.Default(), root)
		if !ok {
			t.Fatal("Age Default")
		}
		sink, ok = schema.Age(summaryValue, root)
		if !ok {
			t.Fatal("Age unchanged summary")
		}
	}); allocations != 0 {
		t.Fatalf("unchanged Age allocations = %v, want 0", allocations)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		var ok bool
		sink, ok = schema.Age(input, root)
		if !ok {
			t.Fatal("Age transformed input")
		}
	}); allocations != 1 {
		t.Fatalf("Age transformed allocations = %v, want 1 immutable image", allocations)
	}
	if sink.IsBottom() {
		t.Fatal("Age allocation sink")
	}
}

func mustAgeAllocation(t testing.TB, schema *Schema, root heap.Key, role materialization.Role) Atom {
	t.Helper()
	atom, ok := schema.Allocation(root, role)
	if !ok {
		t.Fatalf("Allocation(%v, %d)", root, role)
	}
	return atom
}
