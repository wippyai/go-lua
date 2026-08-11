package index

import (
	"strconv"
	"strings"
	"testing"
	"time"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// TestRawGetSelectionIndexConsumesEachSelectedRouteOnce is the hot-path
// growth law for RawGet's solve-local ordinal index.  The callback represents
// the sole authenticated SelectionAt pass made while a transfer, evidence
// check, or selector locator materializes its ordinal table.  The table then
// serves every route lookup without reopening that Selection.  The census
// proves that materialization is one pass over the authenticated routes; the
// retained table is bounded linearly in that route count, rather than carrying
// a candidate cross-product.
//
// The two sizes are deliberately far apart.  This is not a semantic bound:
// both complete arbitrary finite selections and no production limit is
// introduced by the test.
func TestRawGetSelectionIndexConsumesEachSelectedRouteOnce(t *testing.T) {
	for _, count := range []int{64, 4096} {
		t.Run(rawGetSelectionScaleName(count), func(t *testing.T) {
			tags := rawGetSelectionTags(count)
			var index rawSelectionIndex
			observations := 0
			if !index.build(len(tags), func(ordinal int) (uint64, bool) {
				observations++
				return tags[ordinal], true
			}) {
				t.Fatal("raw-get selection index build")
			}
			if observations != count {
				t.Fatalf("raw-get selection observations=%d, want exactly one per route=%d", observations, count)
			}
			if slots := len(index.entries); slots < count || slots > count*4 {
				t.Fatalf("raw-get selection index slots=%d for routes=%d, want linear table", slots, count)
			}
			for ordinal := len(tags) - 1; ordinal >= 0; ordinal-- {
				got, found := index.ordinal(tags[ordinal])
				if !found || got != ordinal {
					t.Fatalf("raw-get selection tag %d = ordinal:%d found:%t, want %d", ordinal, got, found, ordinal)
				}
			}
			if _, found := index.ordinal(rawGetMissingSelectionTag(tags)); found {
				t.Fatal("raw-get selection index accepted an absent route")
			}
		})
	}
}

// TestRawGetSelectionIndexWarmBuildAndProbeAllocatesNothing establishes the
// allocation side of the same contract.  rawSelectionIndex belongs to
// RawGet's pooled solve-local scratch; once a maximum live selection has been
// observed, rebuilding its ordinal table and resolving every selected route
// must reuse that storage.  This exercises the exact production build/lookup
// code used by transfer, evidence, and the Heap locator--not a test model.
func TestRawGetSelectionIndexWarmBuildAndProbeAllocatesNothing(t *testing.T) {
	tags := rawGetSelectionTags(4096)
	missing := rawGetMissingSelectionTag(tags)
	var index rawSelectionIndex
	if !rawGetSelectionIndexWork(&index, tags, missing) {
		t.Fatal("cold raw-get selection index work")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if !rawGetSelectionIndexWork(&index, tags, missing) {
			panic("warm raw-get selection index work")
		}
	}); allocations != 0 {
		t.Fatalf("warm raw-get selection index allocated %v times", allocations)
	}
}

func rawGetSelectionIndexWork(index *rawSelectionIndex, tags []uint64, missing uint64) bool {
	if index == nil || !index.build(len(tags), func(ordinal int) (uint64, bool) {
		return tags[ordinal], true
	}) {
		return false
	}
	for ordinal := len(tags) - 1; ordinal >= 0; ordinal-- {
		got, found := index.ordinal(tags[ordinal])
		if !found || got != ordinal {
			return false
		}
	}
	_, found := index.ordinal(missing)
	return !found
}

func rawGetSelectionTags(count int) []uint64 {
	if count < 1 {
		return nil
	}
	tags := make([]uint64, count)
	for ordinal := range tags {
		// Mix ordinal into a nonzero sparse tag universe.  Route tags are opaque
		// canonical identities; the index must preserve their exact ordinal,
		// rather than relying on input density or declaration order.
		tags[ordinal] = rawSelectionHash(uint64(ordinal) + 1)
	}
	return tags
}

func rawGetMissingSelectionTag(tags []uint64) uint64 {
	for candidate := uint64(1); ; candidate++ {
		found := false
		for _, tag := range tags {
			if tag == candidate {
				found = true
				break
			}
		}
		if !found {
			return candidate
		}
	}
}

func rawGetSelectionScaleName(count int) string {
	switch count {
	case 64:
		return "small"
	case 4096:
		return "large"
	default:
		return "unexpected"
	}
}

// TestRawGetReducerSourceLookupStaysLinearAcrossOnePayloadSourceFrontier
// exercises the source-resolution portion of the one production RawGet
// reducer.  The frontier is built by appendRawSource from distinct existing
// Boundary Values and their sealed Value coordinates: it creates no Factor fact
// and does not bypass the descriptor constructor.  Every row is then selected
// by the same sourceValue helper called by fixed and tail scalar reduction.
//
// A source descriptor is an immutable finite upper bound.  Its cardinality is
// therefore not a solver budget, and this law intentionally makes it large:
// the former scan-per-source implementation performed quadratic work over this
// single selected frontier.  The current byValue relation performs one map
// lookup and one authenticated source-row read per selected value.
func TestRawGetReducerSourceLookupStaysLinearAcrossOnePayloadSourceFrontier(t *testing.T) {
	const (
		rows   = 8192
		rounds = 16
	)
	fixture := rawGetReducerSourceFixtureFor(t, rows)
	if calls := rawGetReducerSourceLookupWork(fixture); calls != rows {
		t.Fatalf("cold raw-get reducer source reads=%d, want exactly one per selected row=%d", calls, rows)
	}

	started := time.Now()
	for round := 0; round < rounds; round++ {
		if calls := rawGetReducerSourceLookupWork(fixture); calls != rows {
			t.Fatalf("round %d raw-get reducer source reads=%d, want %d", round, calls, rows)
		}
	}
	// This deliberately generous wall-clock guard is not a semantic timeout:
	// it distinguishes O(rows) owner lookup from the prior O(rows^2) scan
	// while allowing normal shared-worker scheduling variance.  It is reached
	// only after every selected source has been validated by the reducer.
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("raw-get reducer source frontier took %s for %d selected rows x %d rounds; expected indexed lookup", elapsed, rows, rounds)
	}
}

// TestRawGetReducerSourceLookupWarmsWithoutAllocation records the allocation
// half of the same hot contract.  Source descriptors and sealed facts are
// cold; sourceValue and its authenticated selection callback must only read
// them during an already-assembled reduction.
func TestRawGetReducerSourceLookupWarmsWithoutAllocation(t *testing.T) {
	fixture := rawGetReducerSourceFixtureFor(t, 8192)
	if calls := rawGetReducerSourceLookupWork(fixture); calls != len(fixture.values) {
		t.Fatalf("cold raw-get reducer source reads=%d, want %d", calls, len(fixture.values))
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if calls := rawGetReducerSourceLookupWork(fixture); calls != len(fixture.values) {
			panic("warm raw-get reducer source lookup")
		}
	}); allocations != 0 {
		t.Fatalf("warm raw-get reducer source lookup allocated %v times", allocations)
	}
}

type rawGetReducerSourceFixture struct {
	rule    *RawGetRule
	payload rawPayload
	values  []linkboundary.Value
	facts   []valuedomain.Value
}

func rawGetReducerSourceFixtureFor(t testing.TB, count int) rawGetReducerSourceFixture {
	t.Helper()
	if count < 1 {
		t.Fatal("raw-get source frontier cardinality")
	}
	program, err := lower.Lower(lower.Source{
		Name: "raw_get_source_frontier.lua",
		Text: []byte(rawGetSourceFrontierSource(count)),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapSealed := heapdomain.Seal(linked)
	schema, sealed := valuedomain.Seal(linked, heaps)
	if !heapSealed || !sealed {
		t.Fatal("raw-get source frontier Value schema")
	}

	fixture := rawGetReducerSourceFixture{rule: &RawGetRule{}, values: make([]linkboundary.Value, 0, count), facts: make([]valuedomain.Value, 0, count)}
	sourceTags := make(map[linkboundary.Value]rawSourceTag, count)
	boundaryValues := linked.Boundary().Values()
	for index := 0; index < boundaryValues.Count() && len(fixture.values) < count; index++ {
		value, valueOK := boundaryValues.At(index)
		coordinate, coordinateOK := schema.CoordinateFor(value)
		fact, factOK := schema.SourceValue(value)
		if !valueOK || !coordinateOK || !factOK || schema.Equal(fact, schema.Bottom()) {
			continue
		}
		if !appendRawSource(&fixture.rule.sources, sourceTags, &fixture.payload, value, coordinate) {
			t.Fatal("raw-get source frontier descriptor")
		}
		fixture.values = append(fixture.values, value)
		fixture.facts = append(fixture.facts, fact)
	}
	if len(fixture.values) != count || len(fixture.payload.sources) != count || len(fixture.payload.byValue) != count || len(fixture.rule.sources) != count {
		t.Fatalf("raw-get source frontier values=%d payload-sources=%d by-value=%d source-rows=%d, want %d", len(fixture.values), len(fixture.payload.sources), len(fixture.payload.byValue), len(fixture.rule.sources), count)
	}
	return fixture
}

func rawGetReducerSourceLookupWork(fixture rawGetReducerSourceFixture) int {
	if fixture.rule == nil || len(fixture.values) == 0 || len(fixture.values) != len(fixture.facts) {
		return -1
	}
	reads := 0
	view := rawGetView{source: func(tag rawSourceTag) rawSelected[valuedomain.Value] {
		index := int(tag) - 1
		if index < 0 || index >= len(fixture.facts) {
			return rawSelected[valuedomain.Value]{}
		}
		reads++
		return rawSelected[valuedomain.Value]{value: fixture.facts[index], present: true, found: true, valid: true}
	}}
	for _, value := range fixture.values {
		selected := fixture.rule.sourceValue(view, fixture.payload, value)
		if !selected.valid || !selected.found || !selected.present {
			return -1
		}
	}
	return reads
}

func rawGetSourceFrontierSource(count int) string {
	var text strings.Builder
	text.Grow(count * 3)
	text.WriteString("return ")
	for value := 0; value < count; value++ {
		if value != 0 {
			text.WriteString(", ")
		}
		text.WriteString(strconv.Itoa(value + 1))
	}
	return text.String()
}
