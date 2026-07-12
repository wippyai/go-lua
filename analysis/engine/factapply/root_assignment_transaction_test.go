package factapply

import (
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestConcreteRootAssignmentPointMatchesLegacyCompositionAcrossStateLanes(t *testing.T) {
	const userAxis userlattice.AxisID = "test.root-transaction"
	reg := concreteRootTransactionRegistry(t, userAxis)
	ks := keyspace.New()
	point := cfg.Point(401)
	target := symbol.ID(401)
	sourceRef := factflow.ExprRef(401)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceRef, HasExpr: true}
	assignment := factflow.NewRootAssignment(
		factflow.RootAssignmentOrdinaryRootWrite,
		target,
		pathdom.NewPath(target, "transaction-target"),
		source,
	)
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{point: assignment},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			sourceRef: presentValue(reg),
		},
	})
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	read := func(cfg.Point) state.State { return state.State{} }
	laneSeeds := concreteRootTransactionLaneSeeds(t, reg, ks, userAxis)

	// Run every registered lane in isolation, then deterministic random products
	// of the same representatives. This catches an extraction that accidentally
	// reconstructs State from only the lanes touched by root assignment.
	if got, want := len(state.DefaultLanes()), 17; got != want {
		t.Fatalf("default state lane count = %d, want %d", got, want)
	}
	for _, lane := range state.DefaultLanes() {
		domain := state.DomainWithLanes(reg, []state.LaneID{lane})
		seed := state.NormalizeForDomain(domain, laneSeeds[lane])
		assertConcreteRootTransactionDifferential(t, domain.Equal, ctx, facts, sources, read, assignment, seed, seed)
	}

	rng := rand.New(rand.NewSource(0x5eed))
	fullDomain := state.Domain(reg)
	for iteration := 0; iteration < 512; iteration++ {
		input := state.Reachable(state.State{})
		output := state.Reachable(state.State{})
		for _, lane := range state.DefaultLanes() {
			seed := laneSeeds[lane]
			if rng.Intn(2) == 0 {
				input = fullDomain.Join(input, seed)
			}
			if rng.Intn(2) == 0 {
				output = fullDomain.Join(output, seed)
			}
		}
		assertConcreteRootTransactionDifferential(t, fullDomain.Equal, ctx, facts, sources, read, assignment, input, output)
	}
}

func assertConcreteRootTransactionDifferential(
	t *testing.T,
	equal func(state.State, state.State) bool,
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	assignment factflow.RootAssignment,
	input state.State,
	output state.State,
) {
	t.Helper()
	want, applied := applyRootAssignmentFact(ctx, nil, facts, sources, read, input, output, assignment, nil, nil)
	if applied {
		want = applyCallOutcomeReturnSlotFactsAfterRootAssignment(ctx, facts, nil, nil, nil, nil, nil, read, input, want, assignment.TargetPathRef(), assignment.Source())
		want = applyCallOutcomePresenceRelationPublishes(ctx, facts, nil, nil, nil, read, want)
	}
	got := ApplyConcreteRootAssignmentPoint(ConcreteRootAssignmentPointRequest{
		Context:    ctx,
		Facts:      facts,
		Sources:    sources,
		Read:       read,
		Input:      input,
		Output:     output,
		Assignment: assignment,
	})
	if got.Applied != applied {
		t.Fatalf("Applied = %v, want %v", got.Applied, applied)
	}
	if !equal(got.Output, want) {
		t.Fatal("extracted root transaction differs from legacy composition")
	}
}

func concreteRootTransactionLaneSeeds(t *testing.T, axisReg *axis.Registry, ks *keyspace.KeySpace, userAxis userlattice.AxisID) map[state.LaneID]state.State {
	t.Helper()
	present := presentValue(axisReg)
	rootKey := concreteTransactionStateKey(t, "sym501@1.root")
	otherKey := concreteTransactionStateKey(t, "sym502@1.other")
	localRoot, ok := ks.FromStateKey(rootKey.PathKey())
	if !ok {
		t.Fatal("root key not interned")
	}
	localOther, ok := ks.FromStateKey(otherKey.PathKey())
	if !ok {
		t.Fatal("other key not interned")
	}
	tableID := identity.ID{Kind: "table", Site: "root-transaction", Index: 1}
	dynKey := dynamicindex.Key{Table: localRoot, Site: "root-transaction"}
	dynFact := dynamicindex.Fact{KeyPresence: presence.Present(), KeyValue: present, Value: present, Admission: dynamicindex.AdmissionAdmitted}
	effectKey := effectdelta.Key{Target: localRoot, Site: "root-transaction", Kind: effectdelta.Mutation}
	effectValue := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}
	escape := state.EscapeEvent{Target: rootKey, Kind: escapeevent.KindStore}
	channel := channelselectfact.Fact{Select: "root-transaction", Kind: channelselectfact.FactSelect, Result: rootKey}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: localRoot, Other: localOther}

	return map[state.LaneID]state.State{
		state.LaneValues:            state.State{}.WriteValue(axisReg, key.SymbolValue(501), present),
		state.LanePathEvidence:      state.State{}.WriteLocalPathKey(axisReg, localRoot, present).AddBranchProof(proof),
		state.LaneDynamicIndex:      state.State{}.WriteDynamicIndexFact(axisReg, dynKey, dynFact),
		state.LaneHeapTableIdentity: state.State{}.WriteHeapTableObject(axisReg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: present})),
		state.LaneFrozenTables:      state.State{}.FreezeTable(tableID),
		state.LaneEffectDeltas:      state.State{}.WriteEffectDelta(effectKey, effectValue),
		state.LaneEscapeEvents:      state.State{}.AddEscapeEvent(escape),
		state.LaneChannelSelect:     state.State{}.AddChannelSelectFact(channel),
		state.LaneStoreRelations:    state.State{}.AddStoreRelation(state.StoreRelation{Source: rootKey, Into: otherKey}),
		state.LaneKeyMemberships:    state.State{}.AddPathKeyMembership(rootKey, otherKey),
		state.LaneTypestates: state.State{}.AcquireTypestate(
			state.TypestateResourceFromCanonicalKey(rootKey, typestate.Protocol("root-transaction")),
			typestate.State("open"),
			typestate.Obligation{Final: typestate.State("closed")},
		),
		state.LanePlacement:     state.State{}.WritePlacement(tableID, placement.Stack),
		state.LaneLenFloors:     state.State{}.WriteLenFloor(ks, rootKey, 2),
		state.LaneNumFloors:     state.State{}.WriteNumFloor(ks, rootKey, 3),
		state.LaneNumCeils:      state.State{}.WriteNumCeil(ks, rootKey, 7),
		state.LaneDiffRelations: state.State{}.WriteDiffConstraint(state.RelValueOperand(rootKey), state.RelValueOperand(otherKey), -1),
		state.LaneUserLattices:  state.State{}.WriteUserElement(axisReg, ks, userAxis, rootKey, "Tainted"),
	}
}

func concreteRootTransactionRegistry(t *testing.T, userAxis userlattice.AxisID) *axis.Registry {
	t.Helper()
	reg := axis.NewRegistry()
	axis.Register(reg, variantorigin.Spec())
	axis.Register(reg, identity.Spec())
	axis.Register(reg, runtimekind.Spec())
	axis.Register(reg, typewitness.Spec())
	axis.Register(reg, escape.Spec())
	axis.Register(reg, evidence.Spec())
	axis.Register(reg, assertion.Spec())
	if _, err := userlattice.Register(reg, testTaintSpec(userAxis)); err != nil {
		t.Fatalf("register user state lane: %v", err)
	}
	return reg.Freeze()
}

func concreteTransactionStateKey(t *testing.T, raw string) pathaddr.StateKey {
	t.Helper()
	got, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(raw))
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", raw)
	}
	return got
}

func TestConcreteRootAssignmentPointAppliedFalsePreservesCoreDeltaAndGatesSidecars(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(402)
	target := symbol.ID(402)
	assignment := factflow.NewRootAssignment(
		factflow.RootAssignmentOrdinaryRootWrite,
		target,
		pathdom.NewPath(target, "missing"),
		factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 402, HasExpr: true},
	)
	base := state.State{}.WriteValue(reg, key.SymbolValue(999), presentValue(reg))
	result := ApplyConcreteRootAssignmentPoint(ConcreteRootAssignmentPointRequest{
		Context:    transfer.NodeContext{Registry: reg, Point: point},
		Facts:      factflow.NewFacts(factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{point: assignment}}),
		Sources:    &recordingSourceValues{},
		Read:       func(cfg.Point) state.State { return state.State{} },
		Input:      state.State{},
		Output:     base,
		Assignment: assignment,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			t.Fatal("applied=false invoked call-outcome sidecar")
			return callpayload.CallOutcome{}
		},
	})
	if result.Applied {
		t.Fatal("Applied = true, want false")
	}
	if !state.Domain(reg).Equal(result.Output, base) {
		t.Fatal("applied=false changed unrelated output state")
	}
}

func TestConcreteRootAssignmentPointReturnSlotProviderUsesImmutableInput(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	assignPoint := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), callPoint, false)
	graph.AddEdge(callPoint, assignPoint, false)
	graph.AddEdge(assignPoint, graph.Exit(), false)

	target := symbol.ID(403)
	targetPath := pathdom.NewPath(target, "call-result")
	source := factflow.ValueSource{
		Kind:         factflow.ValueSourceCall,
		CallPoint:    callPoint,
		HasCallPoint: true,
		ResultIndex:  0,
	}
	assignment := factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			callPoint: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, targetPath),
				},
			}),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{assignPoint: assignment},
	})
	inputMarker := symbol.ID(9001)
	outputMarker := symbol.ID(9002)
	input := state.State{}.WriteValue(reg, key.SymbolValue(inputMarker), presentValue(reg))
	output := state.State{}.WriteValue(reg, key.SymbolValue(outputMarker), presentValue(reg))
	callState := state.State{}.WriteReturnSlot(reg, 0, presentValue(reg))
	providerCalls := 0

	result := ApplyConcreteRootAssignmentPoint(ConcreteRootAssignmentPointRequest{
		Context: transfer.NodeContext{Graph: graph, Registry: reg, Point: assignPoint, Node: graph.Node(assignPoint)},
		Facts:   facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		Read: func(point cfg.Point) state.State {
			if point == callPoint {
				return callState
			}
			return state.State{}
		},
		Input:      input,
		Output:     output,
		Assignment: assignment,
		CallOutcome: func(_ transfer.NodeContext, _ factflow.CallSiteView, providerBase state.State, _ func(cfg.Point) state.State) callpayload.CallOutcome {
			providerCalls++
			if !state.Domain(reg).Equal(providerBase, input) {
				t.Fatal("return-slot provider did not receive immutable transaction Input")
			}
			return callpayload.CallOutcome{}
		},
	})
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls)
	}
	if got := result.Output.ReadValue(reg, key.SymbolValue(outputMarker)); !product.Equal(reg, got, presentValue(reg)) {
		t.Fatal("transaction discarded evolving pre-root Output")
	}
}
