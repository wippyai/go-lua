package program

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCanonicalCellCensusClassifiesReuseAndEveryResetReason(t *testing.T) {
	reg := standard.Registry()
	domain := state.Domain(reg)
	stringEntry := canonicalTestEntry(reg, typevalue.LiteralString(reg, "s"))
	numberEntry := canonicalTestEntry(reg, typevalue.LiteralInt(reg, 1))
	joinedEntry := domain.Join(stringEntry, numberEntry)
	function := summary.DefaultSummaryKey(ref.FromSymbol(1))
	stats := &Stats{}
	base := solveAttribution{
		stats: stats,
		key: bodySolveAttributionKey{
			bodyID:   41,
			function: function,
			phase:    SolvePhaseSummary,
		},
	}
	record := func(entry state.State, input, routing uint64, dependencyChange bool) {
		observation := base
		observation.dependencyChange = dependencyChange
		observation.canonical = canonicalSolveObservation{
			valid:       true,
			registry:    reg,
			entry:       entry,
			lanes:       state.DefaultLanes(),
			inputDigest: input,
			resolution:  routing,
		}
		stats.recordBodySolve(&observation, 1)
	}
	record(stringEntry, 1, 10, false) // initial build
	record(stringEntry, 1, 10, false) // exact reuse
	record(joinedEntry, 2, 10, false) // monotone extension
	record(joinedEntry, 2, 10, true)  // callee revision reset
	record(stringEntry, 1, 10, false) // entry shrink reset
	record(numberEntry, 3, 10, false) // incomparable context reset
	record(numberEntry, 3, 11, false) // routing reset

	contextAttribution := base
	contextAttribution.key.function.Entry.Values = 7
	contextAttribution.key.context = true
	contextAttribution.canonical = canonicalSolveObservation{
		valid: true, registry: reg, entry: stringEntry, lanes: state.DefaultLanes(), inputDigest: 4, resolution: 10,
	}
	stats.recordBodySolve(&contextAttribution, 1)

	report := stats.CanonicalCellCensus()
	if report.LexicalFunctions != 1 || report.SemanticContextCells != 1 || len(report.Cells) != 2 {
		t.Fatalf("shape report=%#v", report)
	}
	baseCell := report.Cells[0]
	if baseCell.Function != function {
		baseCell = report.Cells[1]
	}
	if baseCell.ActualBodySolves != 7 || baseCell.WorkspaceBuilds != 5 || baseCell.EligibleMonotoneExtensions != 2 || baseCell.TheoreticalBodySolvesAvoided != 2 {
		t.Fatalf("base cell accounting=%#v", baseCell)
	}
	if baseCell.Resets != (CanonicalCellResets{CalleeRevision: 1, EntryShrink: 1, Context: 1, Routing: 1}) {
		t.Fatalf("base resets=%#v", baseCell.Resets)
	}
	if report.Resets.Context != 2 { // incomparable entry + semantic context partition
		t.Fatalf("aggregate context resets=%d, want 2", report.Resets.Context)
	}
}

func TestCanonicalCellCensusCountsCatalogAndContextDimensionsAutomatically(t *testing.T) {
	report := (*Stats)(nil).CanonicalCellCensus()
	if !reflect.DeepEqual(report.RegisteredStateLanes, state.DefaultLanes()) {
		t.Fatalf("registered lanes=%#v want catalog=%#v", report.RegisteredStateLanes, state.DefaultLanes())
	}
	typeOfEntry := reflect.TypeFor[summary.EntryKey]()
	if len(report.ContextKeyDimensions) != typeOfEntry.NumField() {
		t.Fatalf("context dimensions=%#v want %d fields", report.ContextKeyDimensions, typeOfEntry.NumField())
	}
	for i := 0; i < typeOfEntry.NumField(); i++ {
		if report.ContextKeyDimensions[i] != typeOfEntry.Field(i).Name {
			t.Fatalf("context dimension %d=%q want %q", i, report.ContextKeyDimensions[i], typeOfEntry.Field(i).Name)
		}
	}
}

func TestCanonicalCellCensusIsDeterministicAcrossCellInsertionOrder(t *testing.T) {
	reg := standard.Registry()
	entry := canonicalTestEntry(reg, typevalue.LiteralString(reg, "s"))
	keys := []summary.SummaryKey{
		{Ref: ref.FromSymbol(2), Entry: summary.EntryKey{Facts: 2}},
		{Ref: ref.FromSymbol(1), Entry: summary.EntryKey{Values: 1}},
	}
	build := func(order []summary.SummaryKey) CanonicalCellReport {
		stats := &Stats{}
		for _, function := range order {
			bodyID := uint64(100)
			if function == keys[1] {
				bodyID = 101
			}
			attribution := &solveAttribution{
				stats:     stats,
				key:       bodySolveAttributionKey{bodyID: bodyID, function: function, phase: SolvePhaseSummary, context: true},
				canonical: canonicalSolveObservation{valid: true, registry: reg, entry: entry, lanes: state.DefaultLanes(), resolution: 1},
			}
			stats.recordBodySolve(attribution, 1)
		}
		return stats.CanonicalCellCensus()
	}
	forward := build(keys)
	reverse := build([]summary.SummaryKey{keys[1], keys[0]})
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatalf("forward=%#v reverse=%#v", forward, reverse)
	}
}

func TestProgramCanonicalCellInstrumentationIsBehaviorNeutralAndDeterministic(t *testing.T) {
	stmts := parseChunk(t, `
		local function leaf(x) return x end
		local function wrapper(x) return leaf(x) end
		return wrapper("ok")
	`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	run := func(stats *Stats) Result {
		result, err := RunBoundChunk(stmts, bindings, Config{
			Check: body.Config{Registry: standard.Registry()},
			Stats: stats,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	firstStats := &Stats{}
	first := run(firstStats)
	secondStats := &Stats{}
	second := run(secondStats)
	if first.rootResult == nil || second.rootResult == nil || len(first.snapshot.Entries()) != len(second.snapshot.Entries()) {
		t.Fatal("instrumentation changed program publication")
	}
	firstReport, secondReport := firstStats.CanonicalCellCensus(), secondStats.CanonicalCellCensus()
	if len(firstReport.Cells) == 0 || !reflect.DeepEqual(firstReport, secondReport) {
		t.Fatalf("first=%#v second=%#v", firstReport, secondReport)
	}
}

func canonicalTestEntry(reg *axis.Registry, value product.Value) state.State {
	return state.Domain(reg).Bottom().WriteValue(reg, key.SymbolValue(symbol.ID(1)), value)
}
