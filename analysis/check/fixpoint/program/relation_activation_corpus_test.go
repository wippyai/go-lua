package program

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

type relationCorpusMeasurement struct {
	result Result
	stats  Stats
	wall   time.Duration
	bytes  uint64
}

type relationCorpusTotals struct {
	legacyWall, activeWall   time.Duration
	legacyBytes, activeBytes uint64
	legacySummarySolves      int
	activeSummarySolves      int
	legacySummaryTransfers   int
	activeSummaryTransfers   int
	legacyMaterializeSolves  int
	activeMaterializeSolves  int
	producers                int
	owners                   int
	handled                  int
}

// TestRelationActivationRepresentativeCorpusDifferential is the promotion
// gate for the first exact-leaf activation slice. It includes the historical
// compiler pathology, ordinary wrapper chains, loops, recursive fallback and
// heap-effect fallback. Every observable result remains on the legacy oracle;
// the counters describe whether activation avoided real solver work.
func TestRelationActivationRepresentativeCorpusDifferential(t *testing.T) {
	cases := []struct {
		name   string
		source string
		file   string
		check  func(*axis.Registry) body.Config
		exact  bool
		// requireNoProducers pins the current validate_graph activation blocker:
		// its useful lexical leaves all require parameter/direct-call support.
		requireNoProducers bool
	}{
		{name: "wrapper-chain", exact: true, source: `
local function leaf(): string return "ok" end
local function left(): string return leaf() end
local function right(): string return leaf() end
return left() .. right()
`},
		{name: "loop-caller", exact: true, source: `
local function leaf(): number return 1 end
local total = 0
for i = 1, 8 do total = total + leaf() end
return total
`},
		{name: "certified-context-identity", exact: true, source: `
local function identity(value: string): string return value end
return identity("caller-value")
`},
		{name: "recursive-fallback", exact: true, source: `
local function count(n: number): number
  if n <= 0 then return 0 end
  return 1 + count(n - 1)
end
return count(3)
`},
		{name: "heap-effect-fallback", source: `
local function make(): { value: string } return { value = "ok" } end
return make().value
`},
		{name: "validate-graph", file: "../../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua", check: validateGraphRelationCorpusConfig, requireNoProducers: true},
		{name: "dataflow-node", file: "../../../../testdata/fixtures/regression/deadlock-dataflow-node/main.lua"},
	}

	var totals relationCorpusTotals
	executed := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executed++
			source := tc.source
			filename := tc.name + ".lua"
			if tc.file != "" {
				data, err := os.ReadFile(tc.file)
				if err != nil {
					t.Fatal(err)
				}
				source, filename = string(data), tc.file
			}
			stmts, err := parse.ParseString(source, filename)
			if err != nil {
				t.Fatal(err)
			}
			reg := standard.Registry()
			check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO, Signatures: signaturelookup.Source{IncludeStdlib: true}}
			if tc.check != nil {
				check = tc.check(reg)
			}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
			legacy := measureRelationCorpusRun(t, stmts, bindings, check, false)
			active := measureRelationCorpusRun(t, stmts, bindings, check, true)
			totals.add(legacy, active)
			t.Logf("legacy wall=%s alloc=%.2fMiB solves/transfers=%d/%d materialize=%d; active wall=%s alloc=%.2fMiB solves/transfers=%d/%d materialize=%d producers=%d owners=%d handled=%d",
				legacy.wall, float64(legacy.bytes)/(1<<20), legacy.stats.SummaryBodySolves, legacy.stats.SummaryPointTransfers, legacy.stats.MaterializeBodySolves,
				active.wall, float64(active.bytes)/(1<<20), active.stats.SummaryBodySolves, active.stats.SummaryPointTransfers, active.stats.MaterializeBodySolves,
				active.stats.RelationProducersEligible, active.stats.RelationOwnersActive, active.stats.RelationCallsHandled)
			if tc.exact {
				compareRelationCorpusResult(t, reg, legacy.result, active.result)
			} else {
				if tc.requireNoProducers && active.stats.RelationProducersEligible != 0 {
					t.Fatalf("validate_graph exact-leaf producer census = %d, want 0 until parameterized/direct-call admission lands", active.stats.RelationProducersEligible)
				}
				if active.stats.RelationOwnersActive != 0 || active.stats.RelationCallsHandled != 0 {
					t.Fatalf("allocation/context fixture unexpectedly activated: owners=%d handled=%d", active.stats.RelationOwnersActive, active.stats.RelationCallsHandled)
				}
				if active.stats.SummaryBodySolves != legacy.stats.SummaryBodySolves || active.stats.SummaryPointTransfers != legacy.stats.SummaryPointTransfers || active.stats.MaterializeBodySolves != legacy.stats.MaterializeBodySolves {
					t.Fatalf("inactive activation changed solve work: legacy=%d/%d/%d active=%d/%d/%d",
						legacy.stats.SummaryBodySolves, legacy.stats.SummaryPointTransfers, legacy.stats.MaterializeBodySolves,
						active.stats.SummaryBodySolves, active.stats.SummaryPointTransfers, active.stats.MaterializeBodySolves)
				}
				compareRelationObservableTrees(t, legacy.result.RootResult(), active.result.RootResult(), "root")
			}
		})
	}
	if executed == len(cases) && (totals.handled == 0 || totals.producers == 0 || totals.owners == 0) {
		t.Fatalf("activation corpus was a no-op: producers=%d owners=%d handled=%d", totals.producers, totals.owners, totals.handled)
	}
	if totals.activeSummarySolves > totals.legacySummarySolves || totals.activeSummaryTransfers > totals.legacySummaryTransfers || totals.activeMaterializeSolves > totals.legacyMaterializeSolves {
		t.Fatalf("activation regressed aggregate solve work: legacy summary=%d/%d materialize=%d active=%d/%d materialize=%d",
			totals.legacySummarySolves, totals.legacySummaryTransfers, totals.legacyMaterializeSolves,
			totals.activeSummarySolves, totals.activeSummaryTransfers, totals.activeMaterializeSolves)
	}
	t.Logf("TOTAL legacy wall=%s alloc=%.2fMiB summary=%d/%d materialize=%d; active wall=%s alloc=%.2fMiB summary=%d/%d materialize=%d producers=%d owners=%d handled=%d",
		totals.legacyWall, float64(totals.legacyBytes)/(1<<20), totals.legacySummarySolves, totals.legacySummaryTransfers, totals.legacyMaterializeSolves,
		totals.activeWall, float64(totals.activeBytes)/(1<<20), totals.activeSummarySolves, totals.activeSummaryTransfers, totals.activeMaterializeSolves,
		totals.producers, totals.owners, totals.handled)
}

func validateGraphRelationCorpusConfig(reg *axis.Registry) body.Config {
	uuid := manifest.New("uuid")
	uuid.SetExport(typetable.NewRecord().Field("v7", typ.Func().Returns(typ.String).Build()).Build())
	return body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Globals: []string{"uuid"}, Schedule: transfer.ScheduleWTO,
		Signatures:    signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{uuid}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{uuid}},
	}
}

func measureRelationCorpusRun(t *testing.T, stmts []ast.Stmt, bindings *bind.Result, check body.Config, active bool) relationCorpusMeasurement {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	stats := Stats{}
	start := time.Now()
	result, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: &stats, enableRelationActivation: active})
	wall := time.Since(start)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	return relationCorpusMeasurement{result: result, stats: stats, wall: wall, bytes: after.TotalAlloc - before.TotalAlloc}
}

func compareRelationCorpusResult(t *testing.T, reg *axis.Registry, legacy, active Result) {
	t.Helper()
	wantEntries, gotEntries := legacy.Snapshot().Entries(), active.Snapshot().Entries()
	if len(wantEntries) != len(gotEntries) {
		t.Fatalf("summary entries = %d, want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if wantEntries[i].Key != gotEntries[i].Key {
			t.Fatalf("summary key %d differs: legacy=%#v active=%#v", i, wantEntries[i].Key, gotEntries[i].Key)
		}
		if !summary.Equal(reg, wantEntries[i].Summary, gotEntries[i].Summary) {
			t.Fatalf("summary entry %d differs: key legacy=%#v active=%#v digest legacy=%d active=%d\nlegacy=%#v\nactive=%#v", i,
				wantEntries[i].Key, gotEntries[i].Key,
				summary.NormalizedPayloadDigest(reg, wantEntries[i].Summary), summary.NormalizedPayloadDigest(reg, gotEntries[i].Summary),
				wantEntries[i].Summary, gotEntries[i].Summary)
		}
	}
	compareResultTrees(t, reg, legacy.RootResult(), active.RootResult(), "root")
}

// compareRelationObservableTrees is used only after the census proves that no
// owner activated and no relation call was handled. Allocation-bearing bodies
// use process-local CFG IDs, so independently prepared raw states and context
// entry keys are not comparable. User-visible diagnostics and manifests remain
// exact while work counters prove routing stayed entirely legacy.
func compareRelationObservableTrees(t *testing.T, want, got *body.Result, name string) {
	t.Helper()
	if (want == nil) != (got == nil) {
		t.Fatalf("%s result presence differs", name)
	}
	if want == nil {
		return
	}
	wantDiagnostics, _ := json.Marshal(diagnostics.Produce(want))
	gotDiagnostics, _ := json.Marshal(diagnostics.Produce(got))
	if !reflect.DeepEqual(wantDiagnostics, gotDiagnostics) {
		t.Fatalf("%s diagnostic bytes differ", name)
	}
	if !reflect.DeepEqual(want.SignatureManifests(), got.SignatureManifests()) {
		t.Fatalf("%s signature manifests differ", name)
	}
	wantChildren, gotChildren := want.FunctionResults(), got.FunctionResults()
	if len(wantChildren) != len(gotChildren) {
		t.Fatalf("%s function result count = %d, want %d", name, len(gotChildren), len(wantChildren))
	}
	for i := range wantChildren {
		compareRelationObservableTrees(t, wantChildren[i], gotChildren[i], name+"/child")
	}
}

func (t *relationCorpusTotals) add(legacy, active relationCorpusMeasurement) {
	t.legacyWall += legacy.wall
	t.activeWall += active.wall
	t.legacyBytes += legacy.bytes
	t.activeBytes += active.bytes
	t.legacySummarySolves += legacy.stats.SummaryBodySolves
	t.activeSummarySolves += active.stats.SummaryBodySolves
	t.legacySummaryTransfers += legacy.stats.SummaryPointTransfers
	t.activeSummaryTransfers += active.stats.SummaryPointTransfers
	t.legacyMaterializeSolves += legacy.stats.MaterializeBodySolves
	t.activeMaterializeSolves += active.stats.MaterializeBodySolves
	t.producers += active.stats.RelationProducersEligible
	t.owners += active.stats.RelationOwnersActive
	t.handled += active.stats.RelationCallsHandled
}
