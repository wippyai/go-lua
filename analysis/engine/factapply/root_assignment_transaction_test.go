package factapply

import (
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
		Context: ctx,
		Facts:   facts,
		Sources: sources,
		Read:    read,
		Input:   input,
		Output:  output,
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
		Context: transfer.NodeContext{Registry: reg, Point: point},
		Facts:   factflow.NewFacts(factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{point: assignment}}),
		Sources: &recordingSourceValues{},
		Read:    func(cfg.Point) state.State { return state.State{} },
		Input:   state.State{},
		Output:  base,
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

func TestConcreteRootAssignmentPointAppliedFalsePublishesOnlyDynamicMembershipLane(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4021)
	target := symbol.ID(4021)
	container := symbol.ID(4022)
	table := symbol.ID(4023)
	keySymbol := symbol.ID(4024)
	targetPath := pathdom.NewPath(target, "target")
	containerPath := pathdom.NewPath(container, "container")
	tablePath := pathdom.NewPath(table, "registered")
	keyPath := pathdom.NewPath(keySymbol, "key")
	sourceRef := factflow.ExprRef(4021)
	keyRef := factflow.ExprRef(4022)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceRef, HasExpr: true}
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyRef, HasExpr: true}

	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	builder.Define(point, container, "container")
	builder.Define(point, table, "registered")
	builder.Define(point, keySymbol, "key")
	resolver := visibility.NewResolver(builder.Build())
	containerStateKey, ok := resolver.StateKeyAt(point, containerPath)
	if !ok {
		t.Fatal("missing container state key")
	}
	containerKey, ok := resolver.KeySpace().InternStateKey(containerStateKey)
	if !ok {
		t.Fatal("missing container key")
	}
	tableStateKey, ok := resolver.StateKeyAt(point, tablePath)
	if !ok {
		t.Fatal("missing table state key")
	}
	targetStateKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		t.Fatal("missing target state key")
	}
	dyn, ok := factflow.NewDynamicIndexExpression(containerPath, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}
	site := dynamicindex.Site("root-transaction-applied-false")
	input := state.State{}.
		WriteValue(reg, key.SymbolValue(9900), presentValue(reg)).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: containerKey, Site: site}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			Value: presentValue(reg), HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
		})).
		AddDynamicIndexValueKeyMembership(containerKey, site, tableStateKey)
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source),
		},
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{sourceRef: dyn},
		ExpressionPaths:         map[factflow.ExprRef]pathdom.Path{keyRef: keyPath},
	})

	result := ApplyConcreteRootAssignmentPoint(ConcreteRootAssignmentPointRequest{
		Context:  transfer.NodeContext{Registry: reg, Point: point},
		Resolver: resolver,
		Facts:    facts,
		Sources:  &recordingSourceValues{},
		Read:     func(cfg.Point) state.State { return state.State{} },
		Input:    input,
		Output:   input,
	})
	if result.Applied {
		t.Fatal("Applied = true, want false for unresolved source")
	}
	if !result.Output.HasPathKeyMembership(targetStateKey, tableStateKey) {
		t.Fatal("applied=false lost dynamic-index membership evidence")
	}
	withoutMembership := make([]state.LaneID, 0, len(state.DefaultLanes())-1)
	for _, lane := range state.DefaultLanes() {
		if lane != state.LaneKeyMemberships {
			withoutMembership = append(withoutMembership, lane)
		}
	}
	domain := state.DomainWithLanes(reg, withoutMembership)
	if !domain.Equal(result.Output, input) {
		t.Fatal("applied=false changed a state lane other than key memberships")
	}
}

func TestConcreteRootAssignmentPointMissingFactFailsClosed(t *testing.T) {
	reg := standard.Registry()
	base := state.State{}.WriteValue(reg, key.SymbolValue(4029), presentValue(reg))
	result := new(ConcreteRootAssignmentPointExecutor).Apply(ConcreteRootAssignmentPointRequest{
		Context: transfer.NodeContext{Registry: reg, Point: 4029},
		Facts:   factflow.Facts{},
		Input:   state.State{},
		Output:  base,
	})
	if result.Applied || !state.Domain(reg).Equal(result.Output, base) {
		t.Fatal("missing Facts.RootAssignment did not fail closed")
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
		Input:  input,
		Output: output,
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

func TestConcreteRootAssignmentPointUsesDistinctProviderBasesForSlotsAndPresence(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), callPoint, false)
	graph.AddEdge(callPoint, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	graph.AddEdge(assignErr, graph.Exit(), false)

	value := symbol.ID(404)
	errValue := symbol.ID(405)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(errValue, "err")
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceCall, CallPoint: callPoint, HasCallPoint: true, TargetIndex: 0, ResultIndex: 0}
	errSource := factflow.ValueSource{Kind: factflow.ValueSourceCall, CallPoint: callPoint, HasCallPoint: true, TargetIndex: 1, ResultIndex: 1}
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			callPoint: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, value, valuePath),
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, errValue, errPath),
				},
			}),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignValue: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, value, valuePath, valueSource),
			assignErr:   factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, errValue, errPath, errSource),
		},
	})
	builder := visibility.NewBuilder()
	valueVersion := builder.Define(assignValue, value, "value")
	builder.SetVisible(assignErr, value, valueVersion)
	builder.Define(assignErr, errValue, "err")
	resolver := visibility.NewResolver(builder.Build())
	inputMarker := symbol.ID(9003)
	outputMarker := symbol.ID(9004)
	input := state.State{}.WriteValue(reg, key.SymbolValue(inputMarker), presentValue(reg))
	output := state.State{}.
		WriteValue(reg, key.SymbolValue(outputMarker), presentValue(reg)).
		WriteValue(reg, key.SymbolValue(value), presentValue(reg))
	callState := state.State{}.
		WriteReturnSlot(reg, 0, presentValue(reg)).
		WriteReturnSlot(reg, 1, absentValue(reg))
	bases := make([]state.State, 0, 2)
	delegate := callOutcomeReturnPresenceProvider()

	result := new(ConcreteRootAssignmentPointExecutor).Apply(ConcreteRootAssignmentPointRequest{
		Context:  transfer.NodeContext{Graph: graph, Registry: reg, Point: assignErr, Node: graph.Node(assignErr)},
		Resolver: resolver,
		Facts:    facts,
		Sources:  sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		Read: func(point cfg.Point) state.State {
			if point == callPoint {
				return callState
			}
			return state.State{}
		},
		Input:  input,
		Output: output,
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, base state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			bases = append(bases, base)
			return delegate(ctx, site, base, read)
		},
	})
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if len(bases) != 2 {
		firstIsInput := len(bases) != 0 && state.Domain(reg).Equal(bases[0], input)
		t.Fatalf("provider calls = %d (firstIsInput=%v), want return-slot and presence calls", len(bases), firstIsInput)
	}
	if !state.Domain(reg).Equal(bases[0], input) {
		t.Fatal("return-slot provider base is not immutable Input")
	}
	if got := bases[1].ReadValue(reg, key.SymbolValue(outputMarker)); !product.Equal(reg, got, presentValue(reg)) {
		t.Fatal("presence provider base does not contain evolving Output")
	}
	if got := bases[1].ReadValue(reg, key.SymbolValue(errValue)); !product.Equal(reg, got, absentValue(reg)) {
		t.Fatal("presence provider base does not contain completed root assignment")
	}
}

type observingSourceValues struct {
	values map[factflow.ValueSource]product.Value
	inputs map[factflow.ValueSource][]state.State
}

func (s *observingSourceValues) ValueOfSource(_ cfg.Point, source factflow.ValueSource, in state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	if s.inputs == nil {
		s.inputs = make(map[factflow.ValueSource][]state.State)
	}
	s.inputs[source] = append(s.inputs[source], in)
	value, ok := s.values[source]
	return value, ok
}

func TestConcreteRootAssignmentPointObjectSiblingSourcesUseFixedInputSnapshot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(406)
	target := symbol.ID(406)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 4060, HasExpr: true}
	firstSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 4061, HasExpr: true}
	secondSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 4062, HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
	sources := &observingSourceValues{values: map[factflow.ValueSource]product.Value{
		objectSource: rootValue,
		firstSource:  presentValue(reg),
		secondSource: absentValue(reg),
	}}
	builder := visibility.NewBuilder()
	builder.Define(point, target, "object")
	resolver := visibility.NewResolver(builder.Build())
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "object"), objectSource),
		},
		ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
			objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(fieldSuffix("first"), firstSource, factflow.SourceSpan{}, ""),
				factflow.NewObjectEntryWithMetadata(fieldSuffix("second"), secondSource, factflow.SourceSpan{}, ""),
			}),
		},
	})
	inputMarker := symbol.ID(9005)
	outputMarker := symbol.ID(9006)
	input := state.State{}.WriteValue(reg, key.SymbolValue(inputMarker), presentValue(reg))
	output := state.State{}.WriteValue(reg, key.SymbolValue(outputMarker), presentValue(reg))

	result := ApplyConcreteRootAssignmentPoint(ConcreteRootAssignmentPointRequest{
		Context:  transfer.NodeContext{Registry: reg, Point: point},
		Resolver: resolver,
		Facts:    facts,
		Sources:  sources,
		Read:     func(cfg.Point) state.State { return state.State{} },
		Input:    input,
		Output:   output,
	})
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	for _, source := range []factflow.ValueSource{firstSource, secondSource} {
		seen := sources.inputs[source]
		if len(seen) != 1 {
			t.Fatalf("entry source %v observations = %d, want one", source, len(seen))
		}
		if !state.Domain(reg).Equal(seen[0], input) {
			t.Fatalf("entry source %v did not observe fixed pre-entry Input", source)
		}
	}
}

type orderedSourceValues struct {
	values map[factflow.ValueSource]product.Value
	labels map[factflow.ValueSource]string
	events *[]string
}

func (s orderedSourceValues) ValueOfSource(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	if label := s.labels[source]; label != "" {
		*s.events = append(*s.events, label)
	}
	value, ok := s.values[source]
	return value, ok
}

func TestFactsNodeTransferFinalizesCovarianceAfterSamePointOperations(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(407)
	rootTarget := symbol.ID(407)
	pathTarget := symbol.ID(408)
	exposed := symbol.ID(409)
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 4070, HasExpr: true}
	pathSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 4071, HasExpr: true}
	staticSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 4072, HasExpr: true}
	returnSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 4073, HasExpr: true}
	pathTargetPath := pathdom.NewPath(pathTarget, "container").Field("field")
	narrowType := typetable.NewRecord().Field("x", typ.Number).Build()
	wideType := typetable.NewRecord().Field("x", typ.Any).Build()
	narrowValue := typevalue.WithWitness(reg, typevalue.FromType(reg, narrowType), narrowType)
	wideValue := typevalue.WithWitness(reg, typevalue.FromType(reg, wideType), wideType)
	events := make([]string, 0, 5)
	sources := orderedSourceValues{
		values: map[factflow.ValueSource]product.Value{
			rootSource:   presentValue(reg),
			pathSource:   presentValue(reg),
			staticSource: absentValue(reg),
			returnSource: presentValue(reg),
		},
		labels: map[factflow.ValueSource]string{
			rootSource: "root", pathSource: "path", staticSource: "static", returnSource: "return",
		},
		events: &events,
	}
	builder := visibility.NewBuilder()
	builder.Define(point, rootTarget, "root")
	builder.Define(point, pathTarget, "container")
	builder.Define(point, exposed, "exposed")
	resolver := visibility.NewResolver(builder.Build())
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, rootTarget, pathdom.NewPath(rootTarget, "root"), rootSource),
		},
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			point: factflow.NewPathAssignment(pathTargetPath, pathSource),
		},
		PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
			point: factflow.NewPathStaticMemberWrite(pathTargetPath, staticSource),
		},
		Returns: map[cfg.Point]factflow.Return{
			point: factflow.NewReturn([]factflow.ValueSource{returnSource}),
		},
		CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
			point: {factflow.NewCovariantExposure(pathdom.NewPath(exposed, "exposed"), wideValue, factflow.CovariantExposureRecord)},
		},
	})

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts:      facts,
		Sources:    sources,
		Visibility: resolver,
		CovariantWiden: func(sourceWitness, contract typ.Type, segments []segment.Segment) (typ.Type, [][]segment.Segment, bool) {
			events = append(events, "finalizer")
			return testCovariantRecordWiden(sourceWitness, contract, segments)
		},
	})(transfer.NodeContext{Registry: reg, Point: point}, state.State{}.
		WriteValue(reg, key.SymbolValue(exposed), narrowValue).
		WriteValue(reg, key.SymbolValue(pathTarget), presentValue(reg)))

	if len(events) < 5 || events[len(events)-1] != "finalizer" {
		t.Fatalf("operation order = %v, want covariant finalizer last", events)
	}
	for _, operation := range []string{"root", "path", "static", "return"} {
		seen := false
		for i, event := range events {
			if event == operation {
				seen = true
				if i == len(events)-1 {
					t.Fatalf("operation order = %v, %s ran after finalizer", events, operation)
				}
				break
			}
		}
		if !seen {
			t.Fatalf("operation order = %v, missing %s", events, operation)
		}
	}
	gotType, ok := typevalue.TypeOf(reg, got.ReadValue(reg, key.SymbolValue(exposed)))
	if !ok || !typ.TypeEquals(gotType, wideType) {
		t.Fatalf("finalized exposed type = %v/%v, want %v", gotType, ok, wideType)
	}
}
