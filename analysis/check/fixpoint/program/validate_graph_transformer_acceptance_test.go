package program

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// This is an inactive acceptance harness, not a production routing shortcut.
// It deliberately builds one lexical Relation from the converged base
// validate_graph equation and proves that the same immutable artifact covers
// every currently materialized exact entry. A future operation-plan compiler
// must keep this entire test green while replacing the concrete build input.
type validateGraphTransformerAcceptance struct {
	relation  transformer.Relation
	entries   []summary.EntrySummary
	stats     *Stats
	caller    findRootNodesApplication
	callerKey summary.SummaryKey
}

func TestValidateGraphInactiveRelationAcceptance(t *testing.T) {
	fixture := newValidateGraphTransformerAcceptance(t)
	if got := len(fixture.entries); got != 2 {
		t.Fatalf("validate_graph exact entries = %d, want base + one context", got)
	}
	if fixture.entries[0].Key.Entry != (summary.EntryKey{}) {
		t.Fatalf("first validate_graph entry = %#v, want base entry", fixture.entries[0].Key)
	}

	before := totalAttributedBodySolves(fixture.stats)
	cursor, err := transformer.NewBindingCursor(fixture.relation.Shape(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	instantiated := make([]summary.Summary, len(fixture.entries))
	for i, entry := range fixture.entries {
		got, ok := fixture.relation.Specialize(cursor, nil, nil)
		if !ok {
			t.Fatalf("validate_graph entry %v specialization fell back", entry.Key)
		}
		if !summary.Equal(fixture.caller.config.Registry, got, entry.Summary) {
			t.Fatalf("validate_graph entry %v specialized Summary differs", entry.Key)
		}
		instantiated[i] = got
	}
	if after := totalAttributedBodySolves(fixture.stats); after != before {
		t.Fatalf("relation applications ran body solves: before=%d after=%d", before, after)
	}

	// The one real caller is the end-to-end acceptance surface. Re-solving the
	// caller is test observation only; the validate_graph application itself is
	// Relation.Specialize + the production Summary-to-CallOutcome adapter and
	// contains no callee body-solver reference.
	for i, entry := range fixture.entries {
		adapted, err := solveCompileWithValidateGraphSummary(fixture.caller, fixture.callerKey, entry.Key, instantiated[i])
		if err != nil {
			t.Fatalf("entry %v adapted compiler.compile: %v", entry.Key, err)
		}
		point := validateGraphCallPoint(t, adapted)
		wantOutcome, wantOK := fixture.caller.oracle.CallOutcomeAt(point)
		gotOutcome, gotOK := adapted.CallOutcomeAt(point)
		if wantOK != gotOK || !reflect.DeepEqual(wantOutcome, gotOutcome) {
			t.Fatalf("entry %v line 1314 CallOutcome differs\nwant=%#v\n got=%#v", entry.Key, wantOutcome, gotOutcome)
		}
		wantState, wantOK := fixture.caller.oracle.StateAtBoundary(point)
		gotState, gotOK := adapted.StateAtBoundary(point)
		if wantOK != gotOK || (wantOK && !state.Domain(fixture.caller.config.Registry).Equal(wantState, gotState)) {
			t.Fatalf("entry %v line 1314 post-call State differs", entry.Key)
		}
		wantDiagnostics, err := json.Marshal(diagnostics.Produce(fixture.caller.oracle))
		if err != nil {
			t.Fatal(err)
		}
		gotDiagnostics, err := json.Marshal(diagnostics.Produce(adapted))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(wantDiagnostics, gotDiagnostics) {
			t.Fatalf("entry %v compiler.compile diagnostic bytes differ", entry.Key)
		}
	}

	var summarySolves, summaryTransfers int
	for _, attribution := range fixture.stats.BodySolveAttribution() {
		if attribution.Function.Ref == fixture.entries[0].Key.Ref && attribution.Phase == SolvePhaseSummary {
			summarySolves += attribution.BodySolves
			summaryTransfers += attribution.PointTransfers
		}
	}
	if summarySolves != 2 || summaryTransfers == 0 {
		t.Fatalf("concrete lifecycle = %d summary solves / %d transfers, want two non-empty solves", summarySolves, summaryTransfers)
	}
	t.Logf("inactive validate_graph acceptance: one Relation build, two exact specializations, zero callee body solves after build; concrete baseline=%d summary solves/%d point transfers", summarySolves, summaryTransfers)
}

func TestValidateGraphBranchGuardOnlyEligibilityCensus(t *testing.T) {
	fixture := newFindRootNodesPOCFixture(t)
	oracle := fixture.applications[len(fixture.applications)-1].oracle
	total, conditions, exact := 0, 0, 0
	blocked := make(map[string]int)
	for point := cfg.Point(0); int(point) < oracle.Graph().Size(); point++ {
		if !oracle.Graph().IsBranch(point) {
			continue
		}
		total++
		algebra := oracle.BranchAlgebra(point)
		if _, ok := algebra.ConditionSource(); ok {
			conditions++
		}
		if ok, _ := algebra.GuardOnly(); ok {
			exact++
		} else {
			for _, reason := range algebra.GuardOnlyBlockers() {
				blocked[reason]++
			}
		}
	}
	if total == 0 {
		t.Fatal("validate_graph branch census is empty")
	}
	t.Logf("validate_graph guard-only eligibility: %d/%d condition branches exact (%d structural branch points); blockers=%v", exact, conditions, total, blocked)
}

func newValidateGraphTransformerAcceptance(tb testing.TB) validateGraphTransformerAcceptance {
	tb.Helper()
	base := newFindRootNodesPOCFixture(tb)
	reg := base.applications[0].config.Registry
	entries := make([]summary.EntrySummary, 0, 2)
	for _, entry := range base.result.Snapshot().EntriesOwnedNormalized() {
		if entry.Key.Ref == base.validateKey.Ref {
			entries = append(entries, summary.EntrySummary{Key: entry.Key, Summary: entry.Summary})
		}
	}
	if len(entries) == 0 {
		tb.Fatal("validate_graph snapshot entries missing")
	}
	relation := buildFindRootNodesRelation(tb, reg, entries[0].Summary)
	return validateGraphTransformerAcceptance{
		relation:  relation,
		entries:   entries,
		stats:     base.stats,
		caller:    base.compileApplications[len(base.compileApplications)-1],
		callerKey: base.compileKey,
	}
}

func totalAttributedBodySolves(stats *Stats) int {
	total := 0
	for _, attribution := range stats.BodySolveAttribution() {
		total += attribution.BodySolves
	}
	return total
}

func solveCompileWithValidateGraphSummary(application findRootNodesApplication, owner, key summary.SummaryKey, sum summary.Summary) (*body.Result, error) {
	baseFactory := application.config.CallOutcomeFactory
	solveConfig := application.config.SolveConfig()
	solveConfig.CallOutcomeFactory = func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProvider {
		var original callpayload.CallOutcomeProvider
		if baseFactory != nil {
			original = baseFactory(ctx)
		}
		index := callresult.NewSummaryIndexBase(callresult.SummaryIndexBaseConfig{}).WithOwnerFunctionExpressionKeys(owner, nil)
		adapted := callresult.OutcomeProvider(callresult.ProviderConfig{
			Summaries:     summary.NewSnapshot(application.config.Registry, summary.EntrySummary{Key: key, Summary: sum}),
			ProtectedCall: ctx.ProtectedCall,
			KeyFor:        func(_ transfer.NodeContext, _ factflow.CallSiteView) (summary.SummaryKey, bool) { return key, true },
			CalleeValue:   callresult.CalleeValueFunc(ctx.CalleeValue), ReceiverCallable: callresult.ReceiverCallableFunc(ctx.ReceiverCallable),
			Facts: ctx.Facts, Index: index, Sources: ctx.Sources,
			ReturnPresenceRelations: callresult.ReturnPresenceRelationsForPathFunc(ctx.ReturnPresenceRelationsPath),
			KeySpace:                ctx.KeySpace, TypeValues: ctx.TypeValues,
		})
		return func(node transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			if site.CallSpan().StartLine == 1314 && site.CalleePathRef().String() == "compiler.validate_graph" {
				return adapted(node, site, in, read)
			}
			if original != nil {
				return original(node, site, in, read)
			}
			return callpayload.CallOutcome{}
		}
	}
	return body.SolvePrepared(application.prepared, solveConfig)
}

func validateGraphCallPoint(tb testing.TB, result *body.Result) cfg.Point {
	tb.Helper()
	for point := cfg.Point(0); int(point) < result.Graph().Size(); point++ {
		if site, ok := result.CallSiteView(point); ok && site.CallSpan().StartLine == 1314 && site.CalleePathRef().String() == "compiler.validate_graph" {
			return point
		}
	}
	tb.Fatal("compiler.validate_graph call at line 1314 missing")
	return 0
}

func BenchmarkValidateGraphInactiveRelationLifecycle(b *testing.B) {
	fixture := newValidateGraphTransformerAcceptance(b)
	reg := fixture.caller.config.Registry
	b.Run("build_once", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = buildFindRootNodesRelation(b, reg, fixture.entries[0].Summary)
		}
	})
	b.Run("specialize_two_exact_entries", func(b *testing.B) {
		cursor, err := transformer.NewBindingCursor(fixture.relation.Shape(), nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, entry := range fixture.entries {
				got, ok := fixture.relation.Specialize(cursor, nil, nil)
				if !ok || !summary.Equal(reg, got, entry.Summary) {
					b.Fatal("validate_graph specialization differs")
				}
			}
		}
	})
}
