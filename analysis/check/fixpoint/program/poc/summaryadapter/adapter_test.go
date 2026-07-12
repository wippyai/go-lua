package summaryadapter

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestGuardedRowsMatchConcreteBodySummaryBoundary(t *testing.T) {
	reg := standard.Registry()
	stmts, err := parse.ParseString(`
local function choose(flag, amount)
    local incremented = amount + 1
    if flag then
        return "yes"
    end
    return "no"
end
`, "summaryadapter_concrete.lua")
	if err != nil {
		t.Fatal(err)
	}
	concrete, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var bodySummary summary.Summary
	for _, entry := range concrete.Snapshot().Entries() {
		if entry.Key != concrete.RootKey() {
			bodySummary = entry.Summary
			break
		}
	}
	if len(bodySummary.Returns) != 1 || len(bodySummary.ParamObligations) != 2 {
		t.Fatalf("concrete body boundary = returns:%d obligations:%d", len(bodySummary.Returns), len(bodySummary.ParamObligations))
	}

	unknown := product.Top()
	number := bodySummary.ParamObligations[1]
	plan, err := NewPlan(reg, Spec{
		Params:                 2,
		CommonParamObligations: []Value{Constant(bodySummary.ParamObligations[0]), Constant(number)},
		Rows: []Row{
			{Guards: []Guard{{Param: 0, Kind: GuardTruthy}}, Returns: []Value{Constant(typevalue.LiteralString(reg, "yes"))}},
			{Guards: []Guard{{Param: 0, Kind: GuardFalsy}}, Returns: []Value{Constant(typevalue.LiteralString(reg, "no"))}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, feasible, err := plan.Specialize([]product.Value{unknown, number})
	if err != nil || !feasible {
		t.Fatalf("Specialize = feasible:%v err:%v", feasible, err)
	}
	if !product.LessOrEq(reg, got.Returns[0], bodySummary.Returns[0]) ||
		!product.LessOrEq(reg, bodySummary.Returns[0], got.Returns[0]) {
		t.Fatal("guarded rows and concrete body solve disagree on return boundary")
	}
	for i := range got.ParamObligations {
		if !product.LessOrEq(reg, got.ParamObligations[i], bodySummary.ParamObligations[i]) ||
			!product.LessOrEq(reg, bodySummary.ParamObligations[i], got.ParamObligations[i]) {
			gotType, _ := typevalue.TypeOf(reg, got.ParamObligations[i])
			wantType, _ := typevalue.TypeOf(reg, bodySummary.ParamObligations[i])
			t.Fatalf("guarded rows and concrete body solve disagree on parameter obligation %d: got=%v concrete=%v", i, gotType, wantType)
		}
	}
}

func TestSpecializeUsesExistingSummaryJoinForFeasibleRows(t *testing.T) {
	reg := standard.Registry()
	boolean := typevalue.FromType(reg, typ.Boolean)
	placeholder := pathdom.NewPlaceholder(0)
	status := placeholder.Field("status")
	stale := placeholder.Field("stale")
	yes := typevalue.LiteralString(reg, "yes")
	no := typevalue.LiteralString(reg, "no")
	plan, err := NewPlan(reg, Spec{Params: 1, Rows: []Row{
		{
			Guards:            []Guard{{Param: 0, Kind: GuardTruthy}},
			Returns:           []Value{Constant(yes)},
			PathRefinements:   []PathRefinement{{Path: status, Value: Constant(yes)}},
			PathInvalidations: []PathInvalidation{{Path: stale, PreserveStructuralWitness: true}},
			EffectDeltas: []EffectDelta{{
				Target: placeholder, Site: effectdelta.Site("summaryadapter"), Kind: effectdelta.Mutation,
				Before: Parameter(0), After: Constant(yes), Change: effectdelta.ChangeChanged,
			}},
		},
		{
			Guards:            []Guard{{Param: 0, Kind: GuardFalsy}},
			Returns:           []Value{Constant(no)},
			PathRefinements:   []PathRefinement{{Path: status, Value: Constant(no)}},
			PathInvalidations: []PathInvalidation{{Path: stale, PreserveStructuralWitness: true}},
			EffectDeltas: []EffectDelta{{
				Target: placeholder, Site: effectdelta.Site("summaryadapter"), Kind: effectdelta.Mutation,
				Before: Parameter(0), After: Constant(no), Change: effectdelta.ChangeChanged,
			}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, feasible, err := plan.Specialize([]product.Value{boolean})
	if err != nil || !feasible {
		t.Fatalf("Specialize = feasible:%v err:%v", feasible, err)
	}
	left := rowSummary(reg, placeholder, status, stale, boolean, yes)
	right := rowSummary(reg, placeholder, status, stale, boolean, no)
	left.ParamObligations = nil
	right.ParamObligations = nil
	want := summary.Join(reg, summary.NormalizeOwned(reg, left), summary.NormalizeOwned(reg, right))
	if !summary.Equal(reg, got, want) {
		t.Fatal("specialization did not preserve Summary.Join semantics")
	}
}

func TestConditionalObligationFailsWholeSpecialization(t *testing.T) {
	reg := standard.Registry()
	boolean := typevalue.FromType(reg, typ.Boolean)
	plan, err := NewPlan(reg, Spec{Params: 1, Rows: []Row{
		{
			Guards:           []Guard{{Param: 0, Kind: GuardTruthy}},
			Returns:          []Value{Constant(typevalue.LiteralString(reg, "partial"))},
			ParamObligations: []Value{Constant(boolean)},
		},
		{Returns: []Value{Constant(typevalue.LiteralString(reg, "would-leak"))}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, feasible, err := plan.Specialize([]product.Value{boolean})
	if !errors.Is(err, ErrConditionalObligation) || feasible || !summary.Equal(reg, got, summary.Summary{}) {
		t.Fatalf("Specialize = %#v/%v/%v, want empty atomic fallback", got, feasible, err)
	}
}

func TestSummaryOutcomeAndPostCallStateDifferentialEveryStateLane(t *testing.T) {
	reg := standard.Registry()
	if len(state.DefaultLanes()) != 17 {
		t.Fatalf("default state lanes = %d, want 17", len(state.DefaultLanes()))
	}
	boolean := typevalue.FromType(reg, typ.Boolean)
	placeholder := pathdom.NewPlaceholder(0)
	status := placeholder.Field("status")
	stale := placeholder.Field("stale")
	result := typevalue.LiteralString(reg, "done")
	direct := rowSummary(reg, placeholder, status, stale, boolean, result)
	plan, err := NewPlan(reg, Spec{
		Params:                 1,
		CommonParamObligations: []Value{Constant(boolean)},
		Rows: []Row{{
			Returns:           []Value{Constant(result)},
			PathRefinements:   []PathRefinement{{Path: status, Value: Constant(result)}},
			PathInvalidations: []PathInvalidation{{Path: stale, PreserveStructuralWitness: true}},
			EffectDeltas: []EffectDelta{{
				Target: placeholder, Site: effectdelta.Site("summaryadapter"), Kind: effectdelta.Mutation,
				Before: Parameter(0), After: Constant(result), Change: effectdelta.ChangeChanged,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	adapted, feasible, err := plan.Specialize([]product.Value{boolean})
	if err != nil || !feasible || !summary.Equal(reg, adapted, summary.Normalize(reg, direct)) {
		t.Fatalf("summary differential failed: feasible:%v err:%v", feasible, err)
	}

	callee := symbol.ID(9101)
	key := summary.DefaultSummaryKey(ref.FromSymbol(9102))
	directProvider := summaryProvider(reg, callee, key, direct)
	adaptedProvider := summaryProvider(reg, callee, key, adapted)
	fixture := newCallFixture(reg, callee)
	directOutcome := directProvider(transfer.NodeContext{Registry: reg, Point: fixture.point}, fixture.site, fixture.entry, nil)
	adaptedOutcome := adaptedProvider(transfer.NodeContext{Registry: reg, Point: fixture.point}, fixture.site, fixture.entry, nil)
	assertOutcomeEqual(t, reg, directOutcome, adaptedOutcome)

	for _, lane := range state.DefaultLanes() {
		lanes := []state.LaneID{lane}
		domain := state.DomainWithLanes(reg, lanes)
		entry := state.NormalizeForDomain(domain, fixture.entry)
		want := fixture.apply(reg, directProvider, lanes, entry)
		got := fixture.apply(reg, adaptedProvider, lanes, entry)
		if !domain.Equal(want, got) {
			t.Fatalf("post-call state differs in lane %s", lane)
		}
	}
}

func rowSummary(_ *axis.Registry, placeholder, status, stale pathdom.Path, before, result product.Value) summary.Summary {
	return summary.Summary{
		Returns:          []product.Value{result},
		ParamObligations: []product.Value{before},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathRefinements:   []callboundary.PathValueFact{{Path: status, Value: result}},
			PathInvalidations: []callboundary.PathInvalidationFact{{Path: stale, PreserveStructuralWitness: true}},
			EffectDeltas: []callboundary.EffectDelta{{
				Target: placeholder, Site: effectdelta.Site("summaryadapter"), Kind: effectdelta.Mutation,
				Value: effectdelta.Value{Before: before, After: result, Change: effectdelta.ChangeChanged},
			}},
		},
	}
}

func summaryProvider(reg *axis.Registry, callee symbol.ID, key summary.SummaryKey, sum summary.Summary) callpayload.CallOutcomeProvider {
	return callresult.OutcomeProvider(callresult.ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{Key: key, Summary: sum}),
		KeyFor:    callresult.ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})
}

type callFixture struct {
	point    cfg.Point
	site     factflow.CallSiteView
	facts    factflow.Facts
	resolver *visibility.Resolver
	entry    state.State
}

func newCallFixture(reg *axis.Registry, callee symbol.ID) callFixture {
	point := cfg.Point(1)
	arg := symbol.ID(9201)
	argPath := pathdom.NewPath(arg, "arg")
	expr := factflow.ExprRef(9201)
	builder := visibility.NewBuilder()
	builder.Define(point, arg, "arg")
	resolver := visibility.NewResolver(builder.Build())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{point: factflow.NewCallSite(factflow.CallSiteConfig{
			Context:         factflow.CallSiteContextAssignmentSource,
			CalleeSymbol:    callee,
			ArgumentSources: []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: expr, HasExpr: true}},
		})},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{expr: argPath},
	})
	ks := resolver.KeySpace()
	entry := state.Reachable(state.State{}).
		WritePathKey(reg, ks, resolver.KeyAt(point, argPath.Field("status")), typevalue.LiteralString(reg, "before")).
		WritePathKey(reg, ks, resolver.KeyAt(point, argPath.Field("stale").Field("child")), typevalue.LiteralString(reg, "stale"))
	return callFixture{point: point, site: factsCallSite(facts, point), facts: facts, resolver: resolver, entry: entry}
}

func factsCallSite(facts factflow.Facts, point cfg.Point) factflow.CallSiteView {
	site, _ := facts.CallSiteView(point)
	return site
}

func (f callFixture) apply(reg *axis.Registry, provider callpayload.CallOutcomeProvider, lanes []state.LaneID, entry state.State) state.State {
	transferFn := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: f.facts, CallOutcome: provider, Visibility: f.resolver,
	})
	got := transferFn(transfer.NodeContext{Registry: reg, Point: f.point}, entry)
	return state.NormalizeForDomain(state.DomainWithLanes(reg, lanes), got)
}

func assertOutcomeEqual(t *testing.T, reg *axis.Registry, left, right callpayload.CallOutcome) {
	t.Helper()
	if left.PostReturnAuthority != right.PostReturnAuthority || left.SuspensionKnown != right.SuspensionKnown ||
		len(left.Results) != len(right.Results) || len(left.ParamObligations) != len(right.ParamObligations) ||
		len(left.NormalReturnFacts.PathRefinements) != len(right.NormalReturnFacts.PathRefinements) ||
		len(left.NormalReturnFacts.PathInvalidations) != len(right.NormalReturnFacts.PathInvalidations) ||
		len(left.NormalReturnFacts.EffectDeltas) != len(right.NormalReturnFacts.EffectDeltas) {
		t.Fatalf("outcome shapes differ: %#v vs %#v", left, right)
	}
	for i := range left.Results {
		if left.Results[i].Index != right.Results[i].Index || !product.Equal(reg, left.Results[i].Value, right.Results[i].Value) {
			t.Fatalf("result %d differs", i)
		}
	}
	for i := range left.ParamObligations {
		if left.ParamObligations[i].ParamIndex != right.ParamObligations[i].ParamIndex ||
			!product.Equal(reg, left.ParamObligations[i].Value, right.ParamObligations[i].Value) {
			t.Fatalf("obligation %d differs", i)
		}
	}
	for i := range left.NormalReturnFacts.PathRefinements {
		l, r := left.NormalReturnFacts.PathRefinements[i], right.NormalReturnFacts.PathRefinements[i]
		if !l.Path.Equal(r.Path) || !product.Equal(reg, l.Value, r.Value) {
			t.Fatalf("path refinement %d differs", i)
		}
	}
	for i := range left.NormalReturnFacts.PathInvalidations {
		l, r := left.NormalReturnFacts.PathInvalidations[i], right.NormalReturnFacts.PathInvalidations[i]
		if !l.Path.Equal(r.Path) || l.PreserveStructuralWitness != r.PreserveStructuralWitness || l.ClearTarget != r.ClearTarget {
			t.Fatalf("path invalidation %d differs", i)
		}
	}
	for i := range left.NormalReturnFacts.EffectDeltas {
		l, r := left.NormalReturnFacts.EffectDeltas[i], right.NormalReturnFacts.EffectDeltas[i]
		if !l.Target.Equal(r.Target) || l.Site != r.Site || l.Kind != r.Kind || !effectdelta.Domain(reg).Equal(l.Value, r.Value) {
			t.Fatalf("effect delta %d differs", i)
		}
	}
}
