package program

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestRelationActivationBypassesPrewarmedSharedAndRetainedState(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(): string return "ok" end
return leaf()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	cache := NewSummarySolveCache(reg)
	legacyConfig := Config{
		Check:        body.Config{Registry: reg, Schedule: transfer.ScheduleWTO},
		SummaryCache: cache, CacheProfile: "relation-activation-test",
	}
	legacy, err := RunBoundChunk(stmts, bindings, legacyConfig)
	if err != nil {
		t.Fatal(err)
	}
	prewarmed := len(cache.entries)
	if prewarmed == 0 {
		t.Fatal("legacy run did not prewarm shared cache")
	}
	// Compare against the same prewarmed legacy state, not the cold population
	// pass whose retained handoff lifecycle is intentionally different.
	legacy, err = RunBoundChunk(stmts, bindings, legacyConfig)
	if err != nil {
		t.Fatal(err)
	}
	stats := &Stats{}
	activeConfig := legacyConfig
	activeConfig.Stats = stats
	activeConfig.enableRelationActivation = true
	active, err := RunBoundChunk(stmts, bindings, activeConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(cache.entries) != prewarmed {
		t.Fatalf("active run published shared cache entries: before=%d after=%d", prewarmed, len(cache.entries))
	}
	if stats.RelationCallsHandled == 0 {
		t.Fatal("prewarmed active run did not handle a relation call")
	}
	t.Logf("legacy bodies=%#v active bodies=%#v", materializedBodyVersions(legacy.RootResult()), materializedBodyVersions(active.RootResult()))
	compareResultTrees(t, reg, legacy.RootResult(), active.RootResult(), "root")
}

func TestRelationActivationCancellationPublishesNoSharedState(t *testing.T) {
	stmts := parseChunk(t, `local function leaf(): string return "ok" end return leaf()`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	cache := NewSummarySolveCache(reg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunBoundChunk(stmts, bindings, Config{
		Context: ctx, Check: body.Config{Registry: reg}, SummaryCache: cache,
		enableRelationActivation: true,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled activation error = %v, want context.Canceled", err)
	}
	if len(cache.entries) != 0 {
		t.Fatalf("canceled activation published %d shared entries", len(cache.entries))
	}
}

func TestRelationActivationFreezeRejectionFallsBackToLegacy(t *testing.T) {
	stats := &Stats{}
	activation, err := freezeRelationActivation(context.Background(), stats, relationRunCatalog{})
	if err != nil {
		t.Fatalf("freeze rejection escaped optimization boundary: %v", err)
	}
	if activation != nil {
		t.Fatal("rejected catalog produced an activation")
	}
	if stats.RelationActivationFallbacks != 1 {
		t.Fatalf("activation fallbacks = %d, want 1", stats.RelationActivationFallbacks)
	}
}

func TestRelationActivationFreezeCancellationDoesNotFallBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	activation, err := freezeRelationActivation(ctx, &Stats{}, relationRunCatalog{})
	if activation != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled freeze = activation:%v error:%v, want nil/context.Canceled", activation, err)
	}
}

func TestRelationActivationRunBoundChunkExactDifferential(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(): string
	return "ok"
end
local function wrapper(): string
	local value = leaf()
	return value
end
local result = wrapper()
return result
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	assertRelationActivationDifferential(t,
		func(config Config) (Result, error) { return RunBoundChunk(stmts, bindings, config) },
		false,
	)
}

func TestRelationActivationRunBoundFunctionExactDifferential(t *testing.T) {
	stmts := parseChunk(t, `
return function()
	local function leaf()
		return "ok"
	end
	local function wrapper()
		local value = leaf()
		return value
	end
	local result = wrapper()
	return result
end
`)
	ret := stmts[0].(*ast.ReturnStmt)
	fn := ret.Exprs[0].(*ast.FunctionExpr)
	bindings := bind.BindFunction(fn, bind.Options{})
	assertRelationActivationDifferential(t,
		func(config Config) (Result, error) { return RunBoundFunction(fn, bindings, config) },
		true,
	)
}

func TestRelationActivationCertifiedContextIdentityDifferential(t *testing.T) {
	stmts := parseChunk(t, `
local function identity(value: string): string
	return value
end
return identity("caller-value")
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
	legacyStats := &Stats{}
	legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: legacyStats})
	if err != nil {
		t.Fatal(err)
	}
	activeStats := &Stats{}
	active, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: activeStats, enableRelationActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	compareRelationCorpusResult(t, reg, legacy, active)
	if activeStats.RelationCallsHandled == 0 {
		t.Fatalf("certified contextual identity call was not handled: producers=%d owners=%d fallbacks=%d contexts=%d",
			activeStats.RelationProducersEligible, activeStats.RelationOwnersActive, activeStats.RelationActivationFallbacks, activeStats.MaxContextCount)
	}
	if activeStats.SummaryBodySolves >= legacyStats.SummaryBodySolves || activeStats.SummaryPointTransfers >= legacyStats.SummaryPointTransfers {
		t.Fatalf("certified context did not reduce summary work: legacy=%d/%d active=%d/%d",
			legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers,
			activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers)
	}
	t.Logf("certified context legacy=%d/%d active=%d/%d handled=%d", legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers, activeStats.RelationCallsHandled)
}

func TestRelationActivationCertifiedContextDirectChainDifferential(t *testing.T) {
	stmts := parseChunk(t, `
local function identity(value: string)
	return value
end
local function wrapper(value: string)
	local forwarded = identity(value)
	return forwarded
end
local result = wrapper("caller-value")
return result
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
	legacyStats, activeStats := &Stats{}, &Stats{}
	legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: legacyStats})
	if err != nil {
		t.Fatal(err)
	}
	active, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: activeStats, enableRelationActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	compareRelationCorpusResult(t, reg, legacy, active)
	if activeStats.RelationProducersEligible != 2 || activeStats.RelationOwnersActive != 1 || activeStats.RelationCallsHandled == 0 {
		t.Fatalf("context direct chain was not admitted: producers=%d owners=%d handled=%d fallbacks=%d",
			activeStats.RelationProducersEligible, activeStats.RelationOwnersActive, activeStats.RelationCallsHandled, activeStats.RelationActivationFallbacks)
	}
	if activeStats.SummaryBodySolves >= legacyStats.SummaryBodySolves || activeStats.SummaryPointTransfers >= legacyStats.SummaryPointTransfers {
		t.Fatalf("context direct chain did not reduce work: legacy=%d/%d active=%d/%d",
			legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers)
	}
	t.Logf("context chain legacy=%d/%d active=%d/%d producers=%d owners=%d handled=%d",
		legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers,
		activeStats.RelationProducersEligible, activeStats.RelationOwnersActive, activeStats.RelationCallsHandled)
}

func TestRelationActivationGuardPostStateExact(t *testing.T) {
	stmts := parseChunk(t, `
local function choose(value: boolean)
	if value then return "yes" end
	return "no"
end
local truthy = choose(true)
local falsy = choose(false)
return truthy, falsy
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
	legacyStats, activeStats := &Stats{}, &Stats{}
	legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: legacyStats})
	if err != nil {
		t.Fatal(err)
	}
	active, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: activeStats, enableRelationActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	compareRelationCorpusResult(t, reg, legacy, active)
	if activeStats.RelationProducersEligible != 1 || activeStats.RelationOwnersActive != 1 || activeStats.RelationContextsSpecialized != 1 || activeStats.RelationCallsHandled == 0 ||
		activeStats.SummaryBodySolves >= legacyStats.SummaryBodySolves || activeStats.SummaryPointTransfers >= legacyStats.SummaryPointTransfers {
		t.Fatalf("branch post-state activation = legacy:%d/%d active:%d/%d producers:%d owners:%d contexts:%d handled:%d fallbacks:%d",
			legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers,
			activeStats.RelationProducersEligible, activeStats.RelationOwnersActive, activeStats.RelationContextsSpecialized, activeStats.RelationCallsHandled, activeStats.RelationActivationFallbacks)
	}
}

func TestRelationActivationCertifiedContextUnsafeFamiliesStayLegacy(t *testing.T) {
	cases := []struct{ name, source string }{
		{"capture", `local suffix = "!"; local function f(v: string): string return v .. suffix end; return f("x")`},
		{"heap-mutation", `local function f(v: {x: string}): string v.x = "changed"; return v.x end; return f({x="x"})`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmts := parseChunk(t, tc.source)
			bindings := bind.BindChunk(stmts, bind.Options{})
			reg := standard.Registry()
			check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO}
			legacyStats, activeStats := &Stats{}, &Stats{}
			legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: legacyStats})
			if err != nil {
				t.Fatal(err)
			}
			active, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: activeStats, enableRelationActivation: true})
			if err != nil {
				t.Fatal(err)
			}
			if activeStats.RelationCallsHandled != 0 || activeStats.SummaryBodySolves != legacyStats.SummaryBodySolves || activeStats.SummaryPointTransfers != legacyStats.SummaryPointTransfers {
				t.Fatalf("unsafe family left legacy path: handled=%d legacy=%d/%d active=%d/%d", activeStats.RelationCallsHandled, legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers)
			}
			compareRelationObservableTrees(t, legacy.RootResult(), active.RootResult(), "root")
		})
	}
}

func assertRelationActivationDifferential(t *testing.T, run func(Config) (Result, error), expectReduction bool) {
	t.Helper()
	reg := standard.Registry()
	legacyStats := &Stats{}
	legacy, err := run(Config{Check: body.Config{Registry: reg}, Stats: legacyStats})
	if err != nil {
		t.Fatal(err)
	}
	activeStats := &Stats{}
	activeConfig := Config{Check: body.Config{Registry: reg}, Stats: activeStats, enableRelationActivation: true}
	active, err := run(activeConfig)
	if err != nil {
		t.Fatal(err)
	}

	wantEntries, gotEntries := legacy.Snapshot().Entries(), active.Snapshot().Entries()
	if len(wantEntries) != len(gotEntries) {
		t.Fatalf("summary entries = %d, want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if wantEntries[i].Key != gotEntries[i].Key || !summary.Equal(reg, wantEntries[i].Summary, gotEntries[i].Summary) {
			t.Fatalf("normalized summary entry %d differs", i)
		}
	}
	compareResultTrees(t, reg, legacy.RootResult(), active.RootResult(), "root")
	if activeStats.RelationCallsHandled == 0 {
		t.Fatalf("relation activation was a no-op: producers=%d owners=%d fallbacks=%d legacy=%d/%d active=%d/%d",
			activeStats.RelationProducersEligible, activeStats.RelationOwnersActive, activeStats.RelationActivationFallbacks,
			legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers,
			activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers)
	}
	if activeStats.SummaryBodySolves > legacyStats.SummaryBodySolves || activeStats.SummaryPointTransfers > legacyStats.SummaryPointTransfers {
		t.Fatalf("activation regressed summary work: legacy=%d/%d active=%d/%d", legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers)
	}
	if expectReduction && (activeStats.SummaryBodySolves >= legacyStats.SummaryBodySolves || activeStats.SummaryPointTransfers >= legacyStats.SummaryPointTransfers) {
		t.Fatalf("activation did not reduce both summary solves and transfers: legacy=%d/%d active=%d/%d handled=%d producers=%d owners=%d fallbacks=%d",
			legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers,
			activeStats.RelationCallsHandled, activeStats.RelationProducersEligible, activeStats.RelationOwnersActive, activeStats.RelationActivationFallbacks)
	}
	t.Logf("legacy summary solves/transfers=%d/%d materialize=%d active=%d/%d materialize=%d handled=%d", legacyStats.SummaryBodySolves, legacyStats.SummaryPointTransfers, legacyStats.MaterializeBodySolves, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers, activeStats.MaterializeBodySolves, activeStats.RelationCallsHandled)
}

func compareResultTrees(t *testing.T, reg *axis.Registry, want, got *body.Result, name string) {
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
		t.Fatalf("%s signature manifest projection differs", name)
	}
	wantChildren, gotChildren := want.FunctionResults(), got.FunctionResults()
	if len(wantChildren) != len(gotChildren) {
		t.Fatalf("%s function result count = %d, want %d", name, len(gotChildren), len(wantChildren))
	}
	for i := range wantChildren {
		compareResultTrees(t, reg, wantChildren[i], gotChildren[i], name+"/child")
	}
}
