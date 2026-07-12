package factapply

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestResolvedCallOutcomeSeamClassifiesEveryDescriptor(t *testing.T) {
	const (
		ordinary            = "ordinary-node"
		ordinarySidecar     = "ordinary-node+state-sensitive-return-slot-sidecar"
		trustedOrdinaryHeap = "legacy-ordinary-node+trusted-provenance-required"
		edgeSidecar         = "edge+state-sensitive-assignment-sidecar"
		edge                = "edge"
		outside             = "outside"
	)
	want := map[string]string{
		"Results": outside, "PostReturnAuthority": outside,
		"SuspensionKnown": outside, "MaySuspend": outside,
		"NormalReturnFacts": ordinarySidecar, "ProtectedCallTypestate": ordinary,
		"HeapTableObjects": trustedOrdinaryHeap, "Placements": trustedOrdinaryHeap,
		"ParamObligations": outside, "PathObligations": outside,
		"TypestateRequirements": outside,
		"ParamPathRefinements":  ordinary, "ParamPathWrites": ordinary,
		"ParamLengthFloors": ordinary, "ParamPathInvalidations": ordinary,
		"ParamConditions": ordinary, "ParamPathRelations": ordinary,
		"ReturnConditionRefinements": edge, "ReturnConditionSlots": edge,
		// Also consumed by the later assignment sidecar against a distinct,
		// evolving assignment state; it is deliberately not ordinary/edge-only.
		"ReturnPresenceRelations": edgeSidecar, "ParamExposures": ordinary,
	}
	roles := callpayload.CallOutcomeFieldRoles()
	if len(roles) != len(want) {
		t.Fatalf("call-outcome roles = %d, want classified descriptors = %d", len(roles), len(want))
	}
	for _, role := range roles {
		classification, ok := want[role.FieldName]
		if !ok {
			t.Fatalf("call-outcome descriptor %q has no resolved-seam classification", role.FieldName)
		}
		if classification == "" {
			t.Fatalf("call-outcome descriptor %q has empty classification", role.FieldName)
		}
		delete(want, role.FieldName)
	}
	if len(want) != 0 {
		t.Fatalf("resolved-seam classifications without descriptors: %v", want)
	}
}

func TestResolvedCallOutcomeRequestsAreProviderIndependent(t *testing.T) {
	providerType := reflect.TypeOf((callpayload.CallOutcomeProvider)(nil))
	for _, request := range []any{ResolvedCallOutcomeOrdinaryEffectsRequest{}, ResolvedCallOutcomeEdgeRequest{}} {
		typ := reflect.TypeOf(request)
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).Type == providerType {
				t.Fatalf("%s unexpectedly accepts provider field %s", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}

func TestResolvedCallOutcomeEdgeDoesNotReenterPoisonedProvider(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	arg := symbol.ID(1910)
	expr := factflow.ExprRef(1910)
	argPath := pathdom.NewPath(arg, "arg")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{call: factflow.NewCallSite(factflow.CallSiteConfig{
			Context:         factflow.CallSiteContextCondition,
			ArgumentSources: []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: expr, HasExpr: true}},
		})},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{expr: argPath},
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatal("missing condition call site")
	}
	builder := visibility.NewBuilder()
	builder.Define(branch, arg, "arg")
	resolver := visibility.NewResolver(builder.Build())
	present := product.NewWithPresence(reg, product.ShapeTop, product.PresenceOf(presentValue(reg)))
	outcome := callpayload.CallOutcome{ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{{
		ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: present,
	}}}
	ctx := transfer.EdgeContext{
		Graph: graph, Registry: reg, Edge: cfg.Edge{From: branch, To: thenPoint, Cond: true}, HasCond: true,
	}
	input := state.State{}.WriteValue(reg, key.SymbolValue(arg), product.Top())
	providerCalls := 0
	provider := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		providerCalls++
		if providerCalls != 1 {
			panic("resolved edge helper re-entered poisoned provider")
		}
		return outcome
	}
	throughTraversal := applyCallOutcomeEdgeFacts(ctx, facts, nil, provider, resolver, nil, nil, input)
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls)
	}
	direct := ApplyResolvedCallOutcomeEdge(ResolvedCallOutcomeEdgeRequest{
		Context: ctx, Facts: facts, Resolver: resolver, Output: input,
		CallPoint: call, Site: site, Outcome: outcome,
	})
	if !state.Domain(reg).Equal(direct, throughTraversal) {
		t.Fatal("provider-independent edge application differs from provider traversal")
	}
}

func TestResolvedCallOutcomeOrdinaryEffectsPreserveAllStateLanes(t *testing.T) {
	const userAxis userlattice.AxisID = "test.resolved-call-outcome"
	reg := concreteRootTransactionRegistry(t, userAxis)
	seeds := concreteRootTransactionLaneSeeds(t, reg, keyspace.New(), userAxis)
	if got, want := len(state.DefaultLanes()), 17; got != want {
		t.Fatalf("default state lane count = %d, want %d", got, want)
	}
	for _, lane := range state.DefaultLanes() {
		input := state.Domain(reg).Join(state.Reachable(state.State{}), seeds[lane])
		result := ApplyResolvedCallOutcomeOrdinaryEffects(ResolvedCallOutcomeOrdinaryEffectsRequest{
			Context: transfer.NodeContext{Registry: reg}, Output: input,
		})
		if !result.Applied {
			t.Fatalf("empty outcome rejected for lane %q", lane)
		}
		if !state.Domain(reg).Equal(result.Output, input) {
			t.Fatalf("empty outcome changed state lane %q", lane)
		}
	}
}

func TestResolvedCallOutcomeOrdinaryEffectsFailClosedForUncertifiedHeap(t *testing.T) {
	reg := standard.Registry()
	id := identity.ID{Kind: "test", Site: "caller", Index: 1}
	input := state.Reachable(state.State{}).WriteValue(reg, key.ReturnSlot(0), typevalue.FromType(reg, typ.String))
	outcome := callpayload.CallOutcome{
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: typevalue.FromType(reg, typ.Number)}),
		},
		Placements: map[identity.ID]placement.Value{id: placement.OwnedHeap},
		ParamPathWrites: []callpayload.CallParamPathWrite{{
			Path: pathdom.NewPlaceholder(0), Value: typevalue.FromType(reg, typ.Number),
		}},
	}
	result := ApplyResolvedCallOutcomeOrdinaryEffects(ResolvedCallOutcomeOrdinaryEffectsRequest{
		Context: transfer.NodeContext{Registry: reg}, Output: input, Outcome: outcome,
	})
	if result.Applied {
		t.Fatal("uncertified identity-keyed heap outcome was applied")
	}
	if !state.Domain(reg).Equal(result.Output, input) {
		t.Fatal("uncertified heap outcome partially changed caller state")
	}
}

func TestResolvedCallOutcomeOrdinaryEffectsPreservePhaseOrder(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1901)
	syms := []symbol.ID{1901, 1902, 1903}
	exprs := []factflow.ExprRef{1901, 1902, 1903}
	paths := []pathdom.Path{
		pathdom.NewPath(syms[0], "first").Field("x"),
		pathdom.NewPath(syms[1], "second").Field("x"),
		pathdom.NewPath(syms[2], "third").Field("x"),
	}
	builder := visibility.NewBuilder()
	expressionPaths := make(map[factflow.ExprRef]pathdom.Path, len(paths))
	argumentSources := make([]factflow.ValueSource, len(paths))
	for i := range paths {
		builder.Define(point, syms[i], paths[i].Root)
		expressionPaths[exprs[i]] = paths[i].Parent()
		argumentSources[i] = factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: exprs[i], HasExpr: true}
	}
	resolver := visibility.NewResolver(builder.Build())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{point: factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextStatement, ArgumentSources: argumentSources,
		})},
		ExpressionPaths: expressionPaths,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatal("missing call site")
	}
	stringValue := typevalue.FromType(reg, typ.String)
	numberValue := typevalue.FromType(reg, typ.Number)
	boolValue := typevalue.FromType(reg, typ.Boolean)
	input := state.State{}
	for _, p := range paths {
		input = input.WritePathKey(reg, resolver.KeySpace(), resolver.KeyAt(point, p), stringValue)
	}
	outcome := callpayload.CallOutcome{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			// The first refinement runs before parameter writes.
			PathRefinements: []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(0).Field("x"), Value: numberValue}},
			// The second invalidation runs after parameter writes.
			PathInvalidations: []callboundary.PathInvalidationFact{{Path: pathdom.NewPlaceholder(1).Field("x"), ClearTarget: true}},
			// Final persistent writes run after parameter invalidation.
			PersistentPathWrites: []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(2).Field("x"), Value: numberValue}},
		},
		ParamPathWrites: []callpayload.CallParamPathWrite{
			{Path: pathdom.NewPlaceholder(0).Field("x"), Value: boolValue},
			{Path: pathdom.NewPlaceholder(1).Field("x"), Value: boolValue},
		},
		ParamPathInvalidations: []callpayload.CallParamPathInvalidation{{Path: pathdom.NewPlaceholder(2).Field("x")}},
	}
	got := ApplyResolvedCallOutcomeOrdinaryEffects(ResolvedCallOutcomeOrdinaryEffectsRequest{
		Context: transfer.NodeContext{Registry: reg, Point: point}, Facts: facts,
		Resolver: resolver, TypeValues: typevalue.NewCache(), Output: input,
		Site: site, Outcome: outcome,
	}).Output
	assertPathValue(t, reg, resolver.KeySpace(), got, resolver.KeyAt(point, paths[0]), boolValue)
	assertPathValue(t, reg, resolver.KeySpace(), got, resolver.KeyAt(point, paths[1]), product.Bottom(reg))
	assertPathValue(t, reg, resolver.KeySpace(), got, resolver.KeyAt(point, paths[2]), numberValue)
}
