package projectsummary

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type resolvedEffectProjectionStub struct {
	normalReturnFactProjectAssignmentStub
	writes    map[cfg.Point]factflow.DynamicIndexWrite
	values    map[cfg.Point]map[factflow.ValueSource]product.Value
	bodyReads *int
}

func (s resolvedEffectProjectionStub) ExitState() (state.State, bool) {
	if s.bodyReads != nil {
		(*s.bodyReads)++
	}
	return s.normalReturnFactProjectResultStub.exit, true
}

func (s resolvedEffectProjectionStub) DynamicIndexWrite(point cfg.Point) (factflow.DynamicIndexWrite, bool) {
	write, ok := s.writes[point]
	return write, ok
}

func (s resolvedEffectProjectionStub) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	values := s.values[point]
	value, ok := values[source]
	return value, ok
}

func TestResolvedEffectSummaryMatchesConcreteProjectionOutcomeAndAllStateLanes(t *testing.T) {
	reg := standard.Registry()
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	valueValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	if !presence.Equal(product.PresenceOf(keyValue), presence.Present()) ||
		!presence.Equal(product.PresenceOf(valueValue), presence.Present()) {
		t.Fatal("test values must be definitely present")
	}

	// Build the frozen relation once. Every subsequent specialization below has
	// no body-solver callback or result reader in its dependency surface.
	shape := transformer.Shape{Params: 1}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := transformer.CertifyPlan(plan, transformer.DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	builder := transformer.NewBuilder(reg, shape, nil, plan)
	root := transformer.Root{Kind: transformer.RootParam, Index: 0}
	tableTerm := builder.Arena().Path(root)
	keyTerm := builder.Arena().Constant(keyValue)
	valueTerm := builder.Arena().Constant(valueValue)

	concreteGraph := cfg.New()
	writePoint := concreteGraph.AddNode(cfg.NodeAssign)
	concreteGraph.AddEdge(concreteGraph.Entry(), writePoint, false)
	concreteGraph.AddEdge(writePoint, concreteGraph.Exit(), false)
	effect, err := builder.EffectArena().IndexMutation(transformer.IndexMutationConfig{
		Invalidation: transformer.InvalidatePathConfig{
			Target: tableTerm, Scope: transformer.InvalidationScopeDescendants,
			PreserveStructuralWitness: true, PreserveDynamicValueMemberships: true,
		},
		Table: tableTerm, Key: keyTerm, Value: valueTerm,
		Admission: dynamicindex.AdmissionAdmitted,
		Readback:  factflow.DynamicIndexReadbackKeyAndValue,
		Site:      transformer.EffectSite{Owner: 81, Ordinal: uint32(writePoint)},
	})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := builder.Build(certificate, []transformer.Row{{Guard: builder.Arena().True(), Effects: []transformer.EffectTerm{effect}}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := transformer.NewBindingCursor(shape, []product.Value{product.Top()}, []pathdom.Path{pathdom.NewPlaceholder(0)})
	if err != nil {
		t.Fatal(err)
	}
	lowerings := 0
	canonicalResolver := ResolvedEffectSummaryResolver(reg)
	resolverFn := func(effects []transformer.ResolvedEffect) (summary.Summary, bool) {
		lowerings++
		return canonicalResolver(effects)
	}
	gotSummary, ok := relation.SpecializeWithEffects(cursor, nil, transformer.SpecializationContext{}, resolverFn)
	if !ok {
		t.Fatal("safe relation failed specialization")
	}
	if lowerings != 1 {
		t.Fatalf("post-build lowerings = %d, want 1", lowerings)
	}

	param := symbol.ID(8101)
	paramPath := pathdom.NewPath(param, "table")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8102, HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8103, HasExpr: true}
	write := factflow.NewDynamicIndexWrite(paramPath, keySource, valueSource,
		dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue)
	invalidation := factflow.NewPathDescendantInvalidation(paramPath)
	bodyReads := 0
	concreteSummary := FromResult(resolvedEffectProjectionStub{
		normalReturnFactProjectAssignmentStub: normalReturnFactProjectAssignmentStub{
			normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
				reg: reg, graph: concreteGraph, exit: state.State{}, keys: keyspace.New(),
				slots: []statekey.Value{statekey.SymbolValue(param)},
			},
			pathInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{writePoint: invalidation},
		},
		writes: map[cfg.Point]factflow.DynamicIndexWrite{writePoint: write},
		values: map[cfg.Point]map[factflow.ValueSource]product.Value{
			writePoint: {keySource: keyValue, valueSource: valueValue},
		},
		bodyReads: &bodyReads,
	})
	readsAfterOracle := bodyReads
	if _, ok := relation.SpecializeWithEffects(cursor, nil, transformer.SpecializationContext{}, resolverFn); !ok {
		t.Fatal("second frozen relation specialization failed")
	}
	if bodyReads != readsAfterOracle || lowerings != 2 {
		t.Fatalf("frozen specialization performed body work: body reads %d -> %d, lowerings %d", readsAfterOracle, bodyReads, lowerings)
	}
	wantSummary := summary.Normalize(reg, summary.Summary{NormalReturnFacts: concreteSummary.NormalReturnFacts})
	if !summary.Equal(reg, gotSummary, wantSummary) {
		t.Fatalf("resolved effect Summary differs from concrete projection:\n got %#v\nwant %#v", gotSummary.NormalReturnFacts, wantSummary.NormalReturnFacts)
	}

	callerPoint := cfg.Point(8104)
	callerTable := symbol.ID(8104)
	callerPath := pathdom.NewPath(callerTable, "items")
	argument := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8105, HasExpr: true}
	callerFacts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{callerPoint: factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextStatement, ArgumentSources: []factflow.ValueSource{argument},
		})},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{argument.ExprRef: callerPath},
	})
	site, ok := callerFacts.CallSiteView(callerPoint)
	if !ok {
		t.Fatal("missing caller site")
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(callerPoint, callerTable, "items")
	visibilityResolver := visibility.NewResolver(visibilityBuilder.Build())
	callCtx := transfer.NodeContext{Registry: reg, Point: callerPoint}
	in := state.State{}.
		WriteValue(reg, statekey.SymbolValue(callerTable), product.Top()).
		WritePathKey(reg, visibilityResolver.KeySpace(), visibilityResolver.KeyAt(callerPoint, callerPath.Field("stale")), valueValue)
	transaction := callresult.NewPreparedSummaryTransaction(callresult.ProviderConfig{
		Facts: callerFacts, KeySpace: visibilityResolver.KeySpace(), TypeValues: typevalue.NewCache(),
	})
	read := func(cfg.Point) state.State { return in }
	toOutcome := func(sum summary.Summary) callpayload.CallOutcome {
		return transaction.Apply(callCtx, site, in, read, sum, nil, false)
	}
	gotOutcome, wantOutcome := toOutcome(gotSummary), toOutcome(wantSummary)
	if !reflect.DeepEqual(gotOutcome.NormalReturnFacts, wantOutcome.NormalReturnFacts) ||
		gotOutcome.PostReturnAuthority != wantOutcome.PostReturnAuthority {
		t.Fatalf("Summary adapter mismatch:\n got %#v\nwant %#v", gotOutcome, wantOutcome)
	}
	apply := func(outcome callpayload.CallOutcome) state.State {
		result := factapply.ApplyResolvedCallOutcomeOrdinaryEffects(factapply.ResolvedCallOutcomeOrdinaryEffectsRequest{
			Context: callCtx, Facts: callerFacts, Resolver: visibilityResolver,
			TypeValues: typevalue.NewCache(), Output: in, Site: site, Outcome: outcome,
		})
		if !result.Applied {
			t.Fatal("ordinary call outcome rejected")
		}
		return result.Output
	}
	gotState, wantState := apply(gotOutcome), apply(wantOutcome)
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("lane catalog width = %d, want 17", len(lanes))
	}
	for _, lane := range lanes {
		domain, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			t.Fatal(err)
		}
		if !domain.Equal(gotState, wantState) {
			t.Fatalf("resolved effect CallOutcome differs on state lane %q", lane)
		}
	}
}

func TestResolvedEffectSummarySafeSliceRejectsWholeUnsafeSequence(t *testing.T) {
	reg := standard.Registry()
	scalar := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	base := transformer.ResolvedIndexMutation{
		Invalidation: transformer.ResolvedPathInvalidation{
			Target: pathdom.NewPlaceholder(0), Scope: transformer.InvalidationScopeDescendants,
			PreserveStructuralWitness: true, PreserveDynamicValueMemberships: true,
		},
		Table: pathdom.NewPlaceholder(0), Key: scalar, Value: scalar,
		Admission: dynamicindex.AdmissionAdmitted,
		Readback:  factflow.DynamicIndexReadbackKeyAndValue,
		Site:      transformer.EffectSite{Owner: 1, Ordinal: 2},
	}
	safe := transformer.ResolvedEffect{Kind: transformer.EffectIndexMutation, Mutation: base}
	if _, ok := LowerResolvedEffects(reg, []transformer.ResolvedEffect{safe}); !ok {
		t.Fatal("test baseline is not in safe slice")
	}
	identityValue := product.Set(reg, scalar, identity.Key, identity.Singleton(identity.ID{Kind: "table", Site: "unsafe", Index: 1}))
	appendMutation := base
	appendMutation.AppendCandidate = true
	identityMutation := base
	identityMutation.Value = identityValue
	precise := base
	precise.Invalidation.Precise = &transformer.ResolvedPreciseDynamicTarget{Table: base.Table, Key: scalar}
	structuralAmbiguity := base
	structuralAmbiguity.Invalidation.PreserveStructuralWitness = false
	membershipAmbiguity := base
	membershipAmbiguity.Invalidation.PreserveDynamicValueMemberships = false
	overlap := base
	overlap.Site.Ordinal++
	unsafe := []transformer.ResolvedEffect{
		{Kind: transformer.EffectInvalidatePath, Invalidation: base.Invalidation},
		{Kind: transformer.EffectIndexMutation, Mutation: appendMutation},
		{Kind: transformer.EffectIndexMutation, Mutation: identityMutation},
		{Kind: transformer.EffectIndexMutation, Mutation: precise},
		{Kind: transformer.EffectIndexMutation, Mutation: structuralAmbiguity},
		{Kind: transformer.EffectIndexMutation, Mutation: membershipAmbiguity},
	}
	for i, effect := range unsafe {
		if got, ok := LowerResolvedEffects(reg, []transformer.ResolvedEffect{safe, effect}); ok || !reflect.DeepEqual(got, summary.Summary{}) {
			t.Fatalf("unsafe case %d partially published: %#v", i, got)
		}
	}
	if got, ok := LowerResolvedEffects(reg, []transformer.ResolvedEffect{safe, {Kind: transformer.EffectIndexMutation, Mutation: overlap}}); ok || !reflect.DeepEqual(got, summary.Summary{}) {
		t.Fatalf("overlapping transaction sequence partially published: %#v", got)
	}
}
