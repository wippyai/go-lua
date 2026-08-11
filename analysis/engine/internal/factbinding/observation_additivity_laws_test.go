package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestObserveUnderDisjointSupportAdditivity is the component law required by
// prefix quotienting. For either kind of declared observation unit, observing
// a disjoint union is exactly the per-semantic-observation BDD union of
// observations below each component. The test intentionally uses only the
// public SlotWork observation route: opaque roots, units, rows, and the typed
// ResolveObservation boundary.
func TestObserveUnderDisjointSupportAdditivity(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	_, onOne, offOne, onTwo, offTwo := observationMasks(t, manager)
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	lowLow, ok := regions.And(offOne, offTwo)
	if !ok {
		t.Fatal("low-low")
	}
	lowHigh, ok := regions.And(offOne, onTwo)
	if !ok {
		t.Fatal("low-high")
	}
	highLow, ok := regions.And(onOne, offTwo)
	if !ok {
		t.Fatal("high-low")
	}
	highHigh, ok := regions.And(onOne, onTwo)
	if !ok {
		t.Fatal("high-high")
	}
	withinOne, ok := regions.Or(lowLow, highHigh)
	if !ok {
		t.Fatal("first disjoint region")
	}
	withinTwo, ok := regions.Or(lowHigh, highLow)
	if !ok || !regions.Seal() {
		t.Fatal("second disjoint region")
	}
	within, ok := support.Union(withinOne, withinTwo)
	if !ok {
		t.Fatal("disjoint union")
	}
	if overlap, ok := support.Intersect(withinOne, withinTwo); !ok || !support.Empty(overlap) {
		t.Fatal("support regions overlap")
	}

	binding, state, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil ||
		!patch.Write(declared.target[0], lowLow, 1) ||
		!patch.Write(declared.target[0], lowHigh, 2) ||
		!patch.Write(declared.target[0], highLow, 2) ||
		!patch.Write(declared.target[0], highHigh, 1) ||
		!patch.Write(declared.target[1], lowLow, 8) ||
		!patch.Write(declared.target[1], lowHigh, 4) ||
		!patch.Write(declared.target[1], highLow, 4) ||
		!patch.Write(declared.target[1], highHigh, 8) {
		t.Fatal("stage observation values")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	state = commit(t, work, state, candidate)
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}

	for _, test := range []struct {
		name string
		unit carrier.Unit
	}{
		{name: "exact", unit: declared.exact[0]},
		{name: "summary", unit: declared.summary},
	} {
		t.Run(test.name, func(t *testing.T) {
			combined := observe(t, binding, slotWork, root, test.unit, within)
			first := observe(t, binding, slotWork, root, test.unit, withinOne)
			second := observe(t, binding, slotWork, root, test.unit, withinTwo)

			assertObservationPartition(t, combined, within)
			assertObservationPartition(t, first, withinOne)
			assertObservationPartition(t, second, withinTwo)
			assertObservationAdditivity(t, combined, first, second)

			// Emission order is semantic discovery order, not map iteration. A
			// repeat over the same root/unit/region must reproduce it exactly.
			again := observe(t, binding, slotWork, root, test.unit, within)
			assertSameObservationEmission(t, combined, again)
		})
	}
}

type observationClassRegion struct {
	entries []ObservationEntry[uint64]
	region  support.Mask
}

func assertObservationPartition(t *testing.T, rows []observedRow, within support.Mask) {
	t.Helper()
	work := support.New(within.Manager())
	if work == nil {
		t.Fatal("partition work")
	}
	covered := work.False()
	for index, row := range rows {
		if !row.region.Valid() || support.Empty(row.region) || !row.region.Entails(within) {
			work.Discard()
			t.Fatal("observation escaped source support")
		}
		for previous := 0; previous < index; previous++ {
			overlap, ok := support.Intersect(rows[previous].region, row.region)
			if !ok || !support.Empty(overlap) {
				work.Discard()
				t.Fatal("observation rows overlap")
			}
		}
		var ok bool
		covered, ok = work.Or(covered, row.region)
		if !ok {
			work.Discard()
			t.Fatal("partition union")
		}
	}
	if !work.Seal() || !covered.Equal(within) {
		t.Fatal("observation did not exactly cover source support")
	}
}

func assertObservationAdditivity(t *testing.T, combined, first, second []observedRow) {
	t.Helper()
	got := observationClassRegions(t, combined)
	wantRows := make([]observedRow, 0, len(first)+len(second))
	wantRows = append(wantRows, first...)
	wantRows = append(wantRows, second...)
	want := observationClassRegions(t, wantRows)
	if len(got) != len(combined) {
		t.Fatal("combined observation emitted a semantic class more than once")
	}
	if len(got) != len(want) {
		t.Fatalf("semantic classes = %d, want %d", len(got), len(want))
	}
	for _, expected := range want {
		matched := false
		for _, actual := range got {
			if sameEntries(actual.entries, expected.entries) {
				if !actual.region.Equal(expected.region) {
					t.Fatal("semantic class region was not the BDD union of disjoint observations")
				}
				matched = true
				break
			}
		}
		if !matched {
			t.Fatal("combined observation omitted a semantic class")
		}
	}
}

func observationClassRegions(t *testing.T, rows []observedRow) []observationClassRegion {
	t.Helper()
	if len(rows) == 0 {
		return nil
	}
	work := support.New(rows[0].region.Manager())
	if work == nil {
		t.Fatal("class union work")
	}
	classes := make([]observationClassRegion, 0, len(rows))
	for _, row := range rows {
		index := -1
		for candidate := range classes {
			if sameEntries(classes[candidate].entries, row.entries) {
				index = candidate
				break
			}
		}
		if index < 0 {
			classes = append(classes, observationClassRegion{entries: append([]ObservationEntry[uint64](nil), row.entries...), region: work.False()})
			index = len(classes) - 1
		}
		region, ok := work.Or(classes[index].region, row.region)
		if !ok {
			work.Discard()
			t.Fatal("class BDD union")
		}
		classes[index].region = region
	}
	if !work.Seal() {
		t.Fatal("seal class BDD unions")
	}
	return classes
}

func assertSameObservationEmission(t *testing.T, first, second []observedRow) {
	t.Helper()
	if len(first) != len(second) {
		t.Fatalf("canonical rows = %d, want %d", len(second), len(first))
	}
	for index := range first {
		if !first[index].region.Equal(second[index].region) || !sameEntries(first[index].entries, second[index].entries) {
			t.Fatal("observation emission was not canonical")
		}
	}
}
