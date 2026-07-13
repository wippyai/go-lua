package program

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/relationcall"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestStrictRelationPhaseCollapseOmitsEquationAndReusesMaterialization(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(): number return "diagnostic-parity" end
local function wrapper(): number local value = leaf(); return value end
return wrapper()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
	legacyStats, strictStats := &Stats{}, &Stats{}
	legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: legacyStats, forceLegacyRelations: true})
	if err != nil {
		t.Fatal(err)
	}
	strict, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: strictStats})
	if err != nil {
		t.Fatal(err)
	}
	compareStrictPhaseCollapseParity(t, reg, legacy, strict)
	if strictDiagnosticCount(strict.RootResult()) == 0 {
		t.Fatal("strict admitted-owner parity fixture produced no diagnostics")
	}
	if strictStats.RelationUnexpectedMisses != 0 || strictStats.RelationActivationFallbacks != 0 {
		t.Fatalf("strict relation misses/fallbacks = %d/%d", strictStats.RelationUnexpectedMisses, strictStats.RelationActivationFallbacks)
	}
	if strictStats.RelationSummaryEquationsOmitted != 1 || strictStats.RelationMaterializationsReused != 1 {
		t.Fatalf("strict collapse counters = omitted:%d reused:%d, want 1/1", strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused)
	}
	if strictStats.SummaryBodySolves+1 != legacyStats.SummaryBodySolves || strictStats.MaterializeBodySolves+1 != legacyStats.MaterializeBodySolves {
		t.Fatalf("strict phase work = legacy summary/materialize %d/%d strict %d/%d", legacyStats.SummaryBodySolves, legacyStats.MaterializeBodySolves, strictStats.SummaryBodySolves, strictStats.MaterializeBodySolves)
	}
}

func strictDiagnosticCount(result *body.Result) int {
	if result == nil {
		return 0
	}
	total := len(diagnostics.Produce(result))
	for _, child := range result.FunctionResults() {
		total += strictDiagnosticCount(child)
	}
	return total
}

func TestStrictRelationPhaseCollapseMixedOwnerIsWholeOwnerOnly(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(): string return "ok" end
local function mixed(): string
  local a = leaf()
  local b = tostring(1)
  return a .. b
end
return mixed()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
	legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, forceLegacyRelations: true})
	if err != nil {
		t.Fatal(err)
	}
	stats := &Stats{}
	strict, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	compareStrictPhaseCollapseParity(t, reg, legacy, strict)
	if stats.RelationSummaryEquationsOmitted != 1 || stats.RelationMaterializationsReused != 1 {
		t.Fatalf("mixed owner collapse = omitted:%d reused:%d, want only leaf 1/1", stats.RelationSummaryEquationsOmitted, stats.RelationMaterializationsReused)
	}
	if stats.RelationUnexpectedMisses != 0 {
		t.Fatalf("mixed unsupported owner attempted partial relation routing: misses=%d", stats.RelationUnexpectedMisses)
	}
}

func TestStrictRelationCatalogStatefulOwnerDoesNotPoisonSafeLeaf(t *testing.T) {
	stmts := parseChunk(t, `
local function first(): string return "first" end
local function second(): string return "second" end
return first(), second()
`)
	seen := false
	_, err := RunChunk(stmts, Config{
		Check:                body.Config{Registry: standard.Registry(), TypeValues: typevalue.NewCache()},
		forceLegacyRelations: true,
		relationCatalogAudit: func(c relationRunCatalog) error {
			if len(c.entries) < 2 {
				t.Fatalf("catalog entries = %d, want two call-free leaves", len(c.entries))
			}
			c.entries[0].hasEntryState = true
			strict := c.strictProductionActivationSlice()
			if len(strict.entries) != len(c.entries)-1 {
				t.Fatalf("strict entries = %d, want safe leaf retained from %d", len(strict.entries), len(c.entries))
			}
			seen = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("relation catalog audit did not run")
	}
}

func TestStrictRelationPhaseCollapseContextFanoutParity(t *testing.T) {
	var source strings.Builder
	source.WriteString(`
local function leaf(): string return "ok" end
local function worker(value)
  if value then return leaf() end
  return ""
end
local out = ""
`)
	for i := 0; i < 24; i++ {
		fmt.Fprintf(&source, "out = worker(\"value-%d\")\n", i)
	}
	source.WriteString("return out\n")
	stmts, err := parse.ParseString(source.String(), "strict-context-fanout.lua")
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
	legacy, err := RunChunk(stmts, Config{Check: check, forceLegacyRelations: true})
	if err != nil {
		t.Fatal(err)
	}
	stats := &Stats{}
	strict, err := RunChunk(stmts, Config{Check: check, Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	compareStrictPhaseCollapseParity(t, reg, legacy, strict)
	if stats.MaxContextCount != 25 {
		t.Fatalf("context fanout = %d, want 25 (24 worker entries plus the nested leaf route)", stats.MaxContextCount)
	}
	if stats.RelationSummaryEquationsOmitted != 1 || stats.RelationMaterializationsReused != 1 || stats.RelationUnexpectedMisses != 0 {
		t.Fatalf("fanout strict counters = omitted:%d reused:%d misses:%d, want 1/1/0", stats.RelationSummaryEquationsOmitted, stats.RelationMaterializationsReused, stats.RelationUnexpectedMisses)
	}
}

func TestStrictRelationPhaseCollapseInjectedRejectionRejectsTransaction(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(): string return "ok" end
local function wrapper(): string local value = leaf(); return value end
return wrapper()
`)
	reg := standard.Registry()
	stats := &Stats{}
	result, err := RunChunk(stmts, Config{
		Check: body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO},
		Stats: stats, strictRelationForceReject: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RootResult() == nil {
		t.Fatal("resolver-miss fallback did not complete legacy transaction")
	}
	if stats.RelationUnexpectedMisses != 1 || stats.RelationActivationFallbacks != 1 {
		t.Fatalf("resolver miss counters = misses:%d fallbacks:%d, want 1/1", stats.RelationUnexpectedMisses, stats.RelationActivationFallbacks)
	}
	if stats.RelationSummaryEquationsOmitted != 0 || stats.RelationMaterializationsReused != 0 {
		t.Fatalf("rejected activation collapsed work: omitted:%d reused:%d", stats.RelationSummaryEquationsOmitted, stats.RelationMaterializationsReused)
	}
}

func TestStrictRelationProviderMissLatchesWithoutStatsOrFallback(t *testing.T) {
	missed := false
	fellBack := false
	base := body.Config{CallOutcomeFactory: func(body.CallOutcomeContext) callpayload.CallOutcomeProvider {
		return func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			fellBack = true
			return callpayload.CallOutcome{PostReturnAuthority: true}
		}
	}}
	resolverFactory := relationResolverFactory(func(body.CallOutcomeContext, ...relationResolverInput) relationcall.Resolver {
		return func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (callpayload.CallOutcome, bool) {
			return callpayload.CallOutcome{}, false
		}
	})
	configured := checkConfigWithSummaries(base, nil, nil, nil, nil, metatableMethodProof{}, resolverFactory, nil, true, func() { missed = true }, nil)
	provider := configured.CallOutcomeFactory(body.CallOutcomeContext{})
	out := provider(transfer.NodeContext{}, factflow.CallSiteView{}, state.State{}, nil)
	if !missed || fellBack || out.PostReturnAuthority {
		t.Fatalf("strict miss = latched:%v fallback:%v outcome:%#v", missed, fellBack, out)
	}
}

func TestStrictRelationPhaseCollapseStatsNilRejectionFallsBackWholly(t *testing.T) {
	stmts := parseChunk(t, `local function leaf(): string return "ok" end return leaf()`)
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
	legacy, err := RunChunk(stmts, Config{Check: check, forceLegacyRelations: true})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := RunChunk(stmts, Config{Check: check, strictRelationForceReject: true})
	if err != nil {
		t.Fatal(err)
	}
	compareResultTrees(t, reg, legacy.RootResult(), rejected.RootResult(), "root")
}

func TestStrictRelationPhaseCollapseCancellationReleasesRetainedResults(t *testing.T) {
	stmts := parseChunk(t, `local function leaf(): string return "ok" end return leaf()`)
	ctx, cancel := context.WithCancel(context.Background())
	stats := &Stats{}
	_, err := RunChunk(stmts, Config{
		Context: ctx,
		Check:   body.Config{Registry: standard.Registry(), TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO},
		Stats:   stats,
		WidenAt: func(summary.SummaryKey) bool {
			cancel()
			return false
		},
	})
	if !errors.Is(err, context.Canceled) && !errors.Is(err, solve.ErrCanceled) {
		t.Fatalf("canceled phase collapse error = %v", err)
	}
	if stats.RelationRetainedResultsReleased == 0 {
		t.Fatal("canceled phase collapse did not release activation-owned Results")
	}
	if stats.RelationMaterializationsReused != 0 {
		t.Fatalf("canceled phase collapse reached materialization reuse: %d", stats.RelationMaterializationsReused)
	}
}

func compareStrictPhaseCollapseParity(t *testing.T, reg *axis.Registry, legacy, strict Result) {
	t.Helper()
	want, got := legacy.Snapshot().Entries(), strict.Snapshot().Entries()
	if len(want) != len(got) {
		t.Fatalf("summary entries = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if want[i].Key != got[i].Key || !summary.Equal(reg, want[i].Summary, got[i].Summary) {
			t.Fatalf("summary entry %d differs", i)
		}
	}
	compareStrictPhaseCollapseResultTrees(t, reg, legacy.RootResult(), strict.RootResult(), "root")
}

// compareStrictPhaseCollapseResultTrees compares the canonical body-state
// oracle, exact ResultVersion input lineage, and every user-visible projection.
func compareStrictPhaseCollapseResultTrees(t *testing.T, reg *axis.Registry, want, got *body.Result, name string) {
	t.Helper()
	if (want == nil) != (got == nil) {
		t.Fatalf("%s result presence differs", name)
	}
	if want == nil {
		return
	}
	comparePreparedResults(t, reg, want, got, 1)
	if want.ResultVersion() != got.ResultVersion() {
		t.Fatalf("%s ResultVersion = %d, want %d", name, got.ResultVersion(), want.ResultVersion())
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
		t.Fatalf("%s child count = %d, want %d", name, len(gotChildren), len(wantChildren))
	}
	for i := range wantChildren {
		compareStrictPhaseCollapseResultTrees(t, reg, wantChildren[i], gotChildren[i], name+"/child")
	}
}
