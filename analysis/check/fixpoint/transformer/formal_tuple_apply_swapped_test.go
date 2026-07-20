package transformer

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalTupleApplyBindsSwappedArgumentsSimultaneously(t *testing.T) {
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-apply-swapped"))
	callerID, targetID := lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1)
	callerTerms, targetTerms := NewArena(reg), NewArena(reg)
	if !callerTerms.bindLexicalOwner(callerID) || !targetTerms.bindLexicalOwner(targetID) {
		t.Fatal("bind lexical owners")
	}
	shape := Shape{Params: 2, Results: 1}
	a := callerTerms.Root(Root{Kind: RootParam, Index: 0})
	b := callerTerms.Root(Root{Kind: RootParam, Index: 1})
	aPath := callerTerms.Path(Root{Kind: RootParam, Index: 0})
	bPath := callerTerms.Path(Root{Kind: RootParam, Index: 1})
	point := cfg.Point(17)
	if callerTerms.bindCallResult(point, 0) == 0 {
		t.Fatal("bind call result")
	}
	leftFrame := callerTerms.relationFrame(2, point, 1, shape, []ValueTerm{a, b}, []PathTerm{aPath, bPath}, 1)
	rightFrame := callerTerms.relationFrame(2, point, 2, shape, []ValueTerm{b, a}, []PathTerm{bPath, aPath}, 1)
	if leftFrame == 0 || rightFrame == 0 || leftFrame == rightFrame {
		t.Fatal("freeze distinct swapped call frames")
	}
	bindFormalApplyTestEnvironment(t, callerTerms, shape, 100)
	bindFormalApplyTestEnvironment(t, targetTerms, shape, 110)
	if err := callerTerms.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	if err := targetTerms.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	bindFormalApplyTestInputs(t, callerTerms, shape, 100)
	bindFormalApplyTestInputs(t, targetTerms, shape, 110)

	callerEffects, targetEffects := NewEffectArena(callerTerms), NewEffectArena(targetTerms)
	callerCode := &relationCode{
		terms: callerTerms, effects: callerEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes: []relationNode{
			{},
			{kind: relationNodeSequence, steps: []boundaryStep{
				{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: leftFrame}},
				{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: rightFrame}},
			}, next: 2},
			{kind: relationNodeOutcome, outcome: 1},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}}, contributions: []semanticContribution{{}},
	}
	targetP0 := targetTerms.Root(Root{Kind: RootParam, Index: 0})
	targetCode := &relationCode{
		terms: targetTerms, effects: targetEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes:         []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes:      []boundaryOutcomeTuple{{}, {returnTransaction: testReturnTransactionTerm(t, 1, targetP0)}},
		contributions: []semanticContribution{{}},
	}
	closeAndFreezeRelationGuardTestForest(t, []*relationCode{callerCode, targetCode})
	callerTerms.Seal()
	targetTerms.Seal()
	callerEffects.Seal()
	targetEffects.Seal()
	callerCode.sealed, targetCode.sealed = true, true

	productDomain := state.RegisteredProductDomain(reg)
	program := &RelationProgram{
		registry: reg,
		bodies: []relationProgramBody{
			{body: callerID, variable: 1, keys: keyspace.New(), relation: Relation{shape: shape, arena: callerTerms, effects: callerEffects, descriptors: callerCode.descriptors, code: callerCode, root: 1}, productDomain: productDomain},
			{body: targetID, variable: 2, keys: keyspace.New(), relation: Relation{shape: shape, arena: targetTerms, effects: targetEffects, descriptors: targetCode.descriptors, code: targetCode, root: 1}, productDomain: productDomain},
		},
		byBody: map[lexicalidentity.StableLexicalBodyID]relationVar{callerID: 1, targetID: 2},
	}
	prepareFormalApplyTestProgram(t, program, leftFrame, rightFrame)
	slots, err := freezeSlotSpace(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalSlots = slots
	program.formalFibers, err = freezeFormalFiberInventoryWithSlots(program, slots)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents, err = freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	rootInputs, err := freezeFormalRootInputTemplates(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalTemplate = &formalRelationTemplate{rootInputs: rootInputs}
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}

	targetTuple := formalTupleTestLive(t, algebra, 2)
	targetTuple = formalApplyTestExposeOutcome(t, algebra, targetTuple, 1)
	tableID := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	tableRoot := identityvalue.Present(reg, tableID)
	targetDiagnostics := callpayload.DiagnosticOutput{SuspensionKnown: true, MaySuspend: true}
	targetSpan, _, targetAuthority, ok := algebra.span(targetTuple.variable)
	if !ok {
		t.Fatal("target diagnostic span")
	}
	for _, descriptor := range targetSpan.descriptors() {
		if descriptor.role != formalFiberDiagnostics {
			continue
		}
		leaf, internErr := targetAuthority.internDiagnostics(targetDiagnostics)
		if internErr != nil {
			t.Fatal(internErr)
		}
		targetTuple, err = algebra.writeScalar(targetTuple, descriptor, algebra.decisions.terminal(leaf))
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	targetMember, ok := targetSpan.keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "data"}})
	if !ok {
		t.Fatal("target member suffix")
	}
	heapState := productDomain.Lattice().Bottom().WriteHeapTableObject(reg, tableID,
		heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: tableRoot, StaticMembers: map[keyspace.Key]product.Value{
			targetMember: typevalue.LiteralString(reg, "payload"),
		}}))
	heapLane, ok := productDomain.ProductLane(state.LaneHeapTableIdentity)
	if !ok {
		t.Fatal("heap-table lane")
	}
	heapFactors, err := productDomain.DecomposeLanes(heapState, []state.ProductLane{heapLane})
	if err != nil || len(heapFactors) != 1 {
		t.Fatalf("heap factor = %#v, %v", heapFactors, err)
	}
	callerTuple := formalTupleTestLive(t, algebra, 1)
	callerSpan, _, _, ok := algebra.span(1)
	callerValues, ok := callerSpan.valuesGroup()
	if !ok {
		t.Fatal("caller Values group")
	}
	callerP0, p0OK := program.formalSlots.Slot(callerID, Root{Kind: RootParam, Index: 0})
	callerP1, p1OK := program.formalSlots.Slot(callerID, Root{Kind: RootParam, Index: 1})
	if !p0OK || !p1OK {
		t.Fatal("caller parameter slots")
	}
	valueA, valueB := tableRoot, typevalue.LiteralString(reg, "actual-b")
	callerTuple, err = algebra.writeValuesFactor(callerTuple, callerValues, state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{
		callerP0: valueA, callerP1: valueB,
	}})
	if err != nil {
		t.Fatal(err)
	}
	pointRegions, err := algebra.formalApplyCorrelatedTargetRegions(
		operatorForFormalApplyTest(t, program, callerCode, 1), callerTuple,
		[]formalRelationTuple{formalTupleTestLive(t, algebra, 2)},
	)
	if err != nil || len(pointRegions) != 1 || len(pointRegions[0].occurrences) != 0 {
		t.Fatalf("nonterminal Apply point correlation = %#v, %v", pointRegions, err)
	}
	targetSnapshot := targetTuple
	type arenaSize struct{ values, paths, guards, frames, environments int }
	size := func(arena *Arena) arenaSize {
		return arenaSize{len(arena.values), len(arena.paths), len(arena.guards), len(arena.callFrames), len(arena.environment)}
	}
	beforeCaller, beforeTargetArena := size(callerTerms), size(targetTerms)

	left, leftObservation, err := algebra.applyOutcome(operatorForFormalApplyTest(t, program, callerCode, 1), callerTuple, []formalRelationTuple{targetTuple})
	if err != nil {
		t.Fatal(err)
	}
	right, rightObservation, err := algebra.applyOutcome(operatorForFormalApplyTest(t, program, callerCode, 2), callerTuple, []formalRelationTuple{targetTuple})
	if err != nil {
		t.Fatal(err)
	}
	installHeapProjection := func(observation *formalApplyObservation) {
		t.Helper()
		if len(observation.regions) != 1 || observation.regions[0].publication.normalReturn == nil {
			t.Fatal("formal Apply observation has no normal-return source")
		}
		source := observation.regions[0].publication.normalReturn
		for index, factor := range source.factors {
			if factor.Lane() == heapLane {
				source.factors[index] = heapFactors[0]
				return
			}
		}
		t.Fatal("formal Apply normal-return source has no heap factor")
	}
	installHeapProjection(&leftObservation)
	installHeapProjection(&rightObservation)
	// Both caller occurrences project from the same already-stabilized lexical
	// callee tuple.  The post-WTO DTO fence may specialize that tuple under each
	// caller binding, but it may neither execute the callee nor fieldwise-join
	// the two correlated outcomes.
	formalExecution := &formalRelationExecution{algebra: algebra}
	factorOrdinals := make(map[relationVar][]int)
	projectOutcome := func(step uint32, observation formalApplyObservation) callpayload.CallOutcome {
		t.Helper()
		if len(observation.regions) != 1 {
			t.Fatalf("formal Apply observation regions at step %d = %#v", step, observation.regions)
		}
		observed := observation.regions[0]
		alternatives, projectionErr := formalExecution.projectFormalApplyCallOutcome(context.Background(), formalApplyCallOutcomeProjection{
			step: observation.step, region: observed.region, publication: observed.publication,
		}, factorOrdinals)
		if projectionErr != nil {
			t.Fatalf("formal Apply CallOutcome at step %d: %v", step, projectionErr)
		}
		outcomes := alternatives.Outcomes()
		if len(outcomes) != 1 || len(outcomes[0].Results) != 1 {
			t.Fatalf("formal Apply CallOutcome at step %d = %#v, want one exact alternative", step, outcomes)
		}
		return outcomes[0]
	}
	leftOutcome, rightOutcome := projectOutcome(1, leftObservation), projectOutcome(2, rightObservation)
	if got := leftOutcome.Results[0].Value; !product.Equal(reg, got, valueA) {
		t.Fatalf("left caller outcome = %#v, want %#v", got, valueA)
	}
	if got := rightOutcome.Results[0].Value; !product.Equal(reg, got, valueB) {
		t.Fatalf("right caller outcome = %#v, want %#v", got, valueB)
	}
	object, present := leftOutcome.HeapTableObjects[tableID]
	if !present {
		t.Fatal("left caller outcome omitted reachable callee heap object")
	}
	object.VisitStaticMembers(func(member keyspace.Key, _ product.Value) bool {
		if caller := program.bodies[0].keys.FormatReadOnly(member); caller == "" {
			t.Fatalf("caller outcome retained non-caller member key %#v", member)
		}
		if target := targetSpan.keys.FormatReadOnly(member); target != "" {
			t.Fatalf("caller outcome retained target-owned member key %q", target)
		}
		return true
	})
	if len(factorOrdinals) != 1 || factorOrdinals[2] == nil {
		t.Fatalf("two caller occurrences prepared %d callee factor plans, want one shared lexical plan", len(factorOrdinals))
	}
	// The observation fence accepts only the exact predecessor/target tuples
	// used by the final Apply evaluation. This models a recursive component's
	// intermediate candidate being superseded before stabilization: the stale
	// witness must fail closed rather than leaking its earlier correlation.
	applyCell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	predecessorCell := formalRelationCell{Variable: 1, Root: 1, Kind: formalRelationCellNode}
	targetOutcomeCell := formalRelationCell{Variable: 2, Kind: formalRelationCellOutcome, Outcome: 1}
	program.formalTemplate.applyCells = []formalRelationCellRef{{cell: applyCell}}
	algebra.applyObservations[applyCell] = formalApplyObservationWitness{
		predecessorCell: predecessorCell, predecessorValue: callerTuple,
		outcomeCells: []formalRelationCell{targetOutcomeCell}, outcomeValues: []formalRelationTuple{targetTuple},
		observation: leftObservation,
	}
	values := map[formalRelationCell]formalRelationTuple{
		applyCell: left, predecessorCell: callerTuple, targetOutcomeCell: targetTuple,
	}
	formalExecution.values = values
	beforeValues := make(map[formalRelationCell]formalRelationTuple, len(values))
	for cell, value := range values {
		beforeValues[cell] = value
	}
	detached, detachErr := formalExecution.detachFormalApplyCallOutcomes(context.Background())
	if detachErr != nil || detached[applyCell].Empty() {
		t.Fatalf("stable Apply observation detachment = %#v, %v", detached, detachErr)
	}
	for cell, before := range beforeValues {
		if after := values[cell]; after != before {
			t.Fatalf("CallOutcome detachment mutated stabilized cell %+v: %#v -> %#v", cell, before, after)
		}
	}
	changedCaller, writeErr := algebra.writeValuesFactor(callerTuple, callerValues, state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{
		callerP0: typevalue.LiteralString(reg, "later-a"), callerP1: valueB,
	}})
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	values[predecessorCell] = changedCaller
	if detached, staleErr := formalExecution.detachFormalApplyCallOutcomes(context.Background()); staleErr == nil || detached != nil {
		t.Fatalf("stale recursive Apply observation escaped: %#v, %v", detached, staleErr)
	}
	values[predecessorCell] = callerTuple
	nonreturning, err := algebra.applyNonreturning(operatorForFormalApplyTest(t, program, callerCode, 1), callerTuple, targetTuple)
	if err != nil {
		t.Fatal(err)
	}
	if nonreturning.bottom() || nonreturning.variable != 1 {
		t.Fatalf("nonreturning Apply = %#v, want reachable caller-owned complete tuple", nonreturning)
	}
	targetCell := formalRelationCell{Variable: 2, Kind: formalRelationCellNonreturning}
	executed := evaluateFormalRelationEquation(algebra, formalRelationEquation{
		Operator: formalRelationOperatorRef{kind: formalRelationCellNonreturning, code: callerCode},
		Inputs: []formalRelationTemplateInput{
			{Source: formalRelationCellRef{cell: predecessorCell}, Influence: formalRelationInfluenceApplyNonreturningPredecessor},
			{Source: formalRelationCellRef{cell: targetCell}, Influence: formalRelationInfluenceCalleeNonreturning},
		},
		ApplyNonreturning: []formalApplyNonreturningTransaction{{
			Operator:    operatorForFormalApplyTest(t, program, callerCode, 1),
			Predecessor: formalRelationCellRef{cell: predecessorCell},
			Target:      formalRelationCellRef{cell: targetCell},
		}},
	}, func(cell formalRelationCell) formalRelationTuple {
		switch cell {
		case predecessorCell:
			return callerTuple
		case targetCell:
			return targetTuple
		default:
			return formalRelationTuple{}
		}
	})
	if err := algebra.err(); err != nil || !algebra.same(executed, nonreturning) {
		t.Fatalf("typed nonreturning executor = %#v, %v; want direct %#v", executed, err, nonreturning)
	}
	resultDescriptor, err := algebra.applyResultDescriptor(1, point, 0)
	if err != nil {
		t.Fatal(err)
	}
	resultRoot := func(tuple formalRelationTuple) formalFiberValue {
		span, directory, _, ok := algebra.span(tuple.variable)
		if !ok {
			t.Fatal("Apply result span")
		}
		ordinal, ok := span.ordinal(resultDescriptor)
		if !ok {
			t.Fatal("Apply result descriptor")
		}
		value, readErr := directory.valueAt(tuple.root, ordinal)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return value
	}
	if resultRoot(nonreturning) != resultRoot(callerTuple) {
		t.Fatal("nonreturning Apply published a result binding")
	}
	if resultRoot(left) == resultRoot(callerTuple) {
		t.Fatal("normal Apply did not publish its declared result binding")
	}
	carried, reachable, err := algebra.formalDiagnosticOutput(context.Background(), nonreturning)
	if err != nil || !reachable || !carried.RepresentationEqual(program.registry, targetDiagnostics) {
		t.Fatalf("nonreturning Apply diagnostics = %#v/%t, %v; want exact callee carry %#v", carried, reachable, err, targetDiagnostics)
	}
	leftBinding := formalApplyTestResultBinding(t, algebra, left, callerID, point)
	rightBinding := formalApplyTestResultBinding(t, algebra, right, callerID, point)
	if leftBinding.value.term != a || !leftBinding.pathPresent || leftBinding.path.term != aPath {
		t.Fatalf("Apply [a,b] result = %#v, want atomic a/aPath", leftBinding)
	}
	if rightBinding.value.term != b || !rightBinding.pathPresent || rightBinding.path.term != bPath {
		t.Fatalf("Apply [b,a] result = %#v, want atomic b/bPath", rightBinding)
	}
	if leftBinding.value.arena != callerTerms || rightBinding.value.arena != callerTerms || left.variable != 1 || right.variable != 1 {
		t.Fatal("Apply result is not caller-owned")
	}
	if targetTuple != targetSnapshot || size(callerTerms) != beforeCaller || size(targetTerms) != beforeTargetArena {
		t.Fatal("Apply mutated its target tuple or a sealed term arena")
	}
}

func bindFormalApplyTestEnvironment(t *testing.T, arena *Arena, shape Shape, first symbol.ID) {
	t.Helper()
	for index := uint32(0); index < shape.Params; index++ {
		id := first + symbol.ID(index)
		if arena.bindEnvironmentSymbol(id) == 0 {
			t.Fatalf("bind formal Apply parameter %d", id)
		}
	}
}

func bindFormalApplyTestInputs(t *testing.T, arena *Arena, shape Shape, first symbol.ID) {
	t.Helper()
	entries := make([]relationMiddleEntry, 0, shape.Params)
	for index := uint32(0); index < shape.Params; index++ {
		id := first + symbol.ID(index)
		middle, exact := arena.middleRoot(statekey.SymbolValue(id))
		input := Root{Kind: RootParam, Index: index}
		if !exact || arena.Root(middle) == 0 || arena.Path(middle) == 0 || arena.Root(input) == 0 || arena.Path(input) == 0 {
			t.Fatalf("bind formal Apply input %d", index)
		}
		entries = append(entries, relationMiddleEntry{middle: middle, input: input})
	}
	if err := arena.middle.bindInputs(shape, entries); err != nil {
		t.Fatal(err)
	}
}

func operatorForFormalApplyTest(t *testing.T, program *RelationProgram, code *relationCode, step uint32) formalRelationOperatorRef {
	t.Helper()
	cell := formalRelationCell{Variable: 1, Root: 1, Step: step, Kind: formalRelationCellStep}
	footprint, err := program.formalFibers.operatorFootprints.bind(program, cell)
	if err != nil {
		t.Fatal(err)
	}
	operator := formalRelationOperatorRef{kind: formalRelationCellStep, code: code, root: 1, step: step, footprint: footprint}
	apply, err := freezeFormalApplyStep(program, 1, operator)
	if err != nil {
		t.Fatal(err)
	}
	operator.apply = apply
	return operator
}

func prepareFormalApplyTestProgram(t *testing.T, program *RelationProgram, frames ...callFrameTerm) {
	t.Helper()
	if len(program.bodies) != 2 || len(frames) == 0 {
		t.Fatal("formal Apply test program shape")
	}
	for bodyIndex := range program.bodies {
		body := &program.bodies[bodyIndex]
		roots := relationRootCarrier{shape: body.relation.Shape()}
		params := make([]symbol.ID, 0, body.relation.Shape().Params)
		contracts := make([]product.Value, 0, body.relation.Shape().Params)
		for index := uint32(0); index < body.relation.Shape().Params; index++ {
			id := symbol.ID(100 + bodyIndex*10 + int(index))
			params = append(params, id)
			contracts = append(contracts, product.Top())
			roots.roots = append(roots.roots, relationStateRoot{
				root: Root{Kind: RootParam, Index: index}, slot: statekey.SymbolValue(id),
				path: body.keys.FromPath(pathdom.NewPath(id, "")),
			})
		}
		body.roots = roots
		graph := cfg.New()
		graph.AddEdge(graph.Entry(), graph.Exit(), false)
		body.graph = graph
		body.plan = operationplan.New(body.graph, factflow.FactsInput{}).
			WithBoundaryParams(params).WithBoundaryParamContracts(contracts).
			WithBoundaryCaptures(nil).WithBoundaryGlobals(nil)
		body.entrySeedPlan = state.NewEntrySeedPlan(nil)
	}
	caller, target := &program.bodies[0], &program.bodies[1]
	maxFrame := callFrameTerm(0)
	for _, frame := range frames {
		if frame > maxFrame {
			maxFrame = frame
		}
	}
	caller.frames = make([]linkedRelationFrame, int(maxFrame)+1)
	for occurrence, term := range frames {
		node := caller.relation.arena.callFrames[term]
		rootCircuit, rootErr := freezeFrameRootCircuit(node, caller)
		if rootErr != nil {
			t.Fatal(rootErr)
		}
		outbound := make(state.BoundaryRoots, len(rootCircuit))
		for index := range target.roots.roots {
			outbound[index] = state.BoundaryRoot{Slot: caller.roots.roots[index].slot, Path: caller.roots.roots[index].path, Value: product.Bottom(program.registry)}
		}
		allocation, err := state.NewBoundaryAllocationAuthority(
			state.ApplyBoundaryAllocationRoute(target.body, caller.body, uint32(node.point), uint32(occurrence)), nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		_, resultPath, resultErr := frameCallResultCarrier(caller.keys, caller.body, node.point, 0)
		if resultErr != nil {
			t.Fatal(resultErr)
		}
		caller.frames[term] = linkedRelationFrame{
			owner: 1, target: 2, term: term, callerBody: caller.body, targetBody: target.body,
			callerKeys: caller.keys, targetKeys: target.keys, targetRoots: target.roots,
			point: node.point, occurrence: uint32(occurrence), shape: node.shape,
			rootCircuit: rootCircuit, outboundRoots: outbound,
			resultSelectors: []linkedFrameResult{{slot: 0, targets: []linkedFrameResultTarget{{slot: caller.relation.arena.middle.registers[0].slot, path: resultPath, stateTarget: true}}}},
			route:           linkedFrameRouteAuthority{owner: 1, target: 2, frame: term},
			existentials:    state.BoundaryExistentialNamespace{OwnerHi: 1, Point: uint32(node.point), Partition: uint32(occurrence)},
			allocations:     allocation, control: linkedCallControlOrdinary,
		}
		linked := &caller.frames[term]
		linked.boundary, err = freezeLinkedFrameBoundaryTopology(caller, linked)
		if err != nil {
			t.Fatal(err)
		}
	}
	vocabulary := &formalGuardVocabulary{
		ranks: make(map[formalGuardRankKey]uint32), apply: make(map[formalRelationCell]formalGuardBoundary),
		definitions: make(map[formalRelationDefinitionRef]formalGuardBoundary), loops: make(map[formalGuardLoopLifetime]formalGuardRankSet), sealed: true,
	}
	for step := range frames {
		boundary := formalGuardBoundary{
			owner: vocabulary, rename: formalGuardRankMap{owner: vocabulary},
			domain: formalGuardRankSet{owner: vocabulary}, close: formalGuardRankSet{owner: vocabulary},
		}
		vocabulary.apply[formalRelationCell{Variable: 1, Root: 1, Step: uint32(step + 1), Kind: formalRelationCellStep}] = boundary
	}
	program.formalGuards = vocabulary
}

func formalApplyTestExposeOutcome(t *testing.T, algebra *formalTupleAlgebra, tuple formalRelationTuple, outcome boundaryOutcomeRef) formalRelationTuple {
	t.Helper()
	if err := algebra.cacheFormalOutcomeFactorSpellings(tuple); err != nil {
		t.Fatal(err)
	}
	span, _, authority, ok := algebra.span(tuple.variable)
	if !ok {
		t.Fatal("target span")
	}
	for _, descriptor := range span.descriptors() {
		if descriptor.role != formalFiberOutcome || descriptor.outcome != outcome {
			continue
		}
		leaf, err := authority.internOutcomeOccurrence(formalQualifiedOutcomeOccurrence{code: authority.code, ref: outcome, root: 1})
		if err != nil {
			t.Fatal(err)
		}
		result, err := algebra.writeScalar(tuple, descriptor, algebra.decisions.terminal(leaf))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	t.Fatal("target outcome descriptor")
	return formalRelationTuple{}
}

func formalApplyTestResultBinding(t *testing.T, algebra *formalTupleAlgebra, tuple formalRelationTuple, owner lexicalidentity.StableLexicalBodyID, point cfg.Point) formalQualifiedBinding {
	t.Helper()
	descriptor, err := algebra.applyResultDescriptor(tuple.variable, point, 0)
	if err != nil {
		t.Fatal(err)
	}
	formalRoot, ok := descriptor.slot.Root()
	if !ok || formalRoot.Owner() != owner {
		t.Fatal("result FormalSlot has foreign owner")
	}
	span, directory, authority, _ := algebra.span(tuple.variable)
	ordinal, ok := span.ordinal(descriptor)
	if !ok {
		t.Fatal("result ordinal")
	}
	value, err := directory.valueAt(tuple.root, ordinal)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := algebra.decisions.node(decisionRef(value))
	if !ok || !node.terminal || node.leaf < 2 {
		t.Fatalf("result is not an unconditional binding terminal: root=%d node=%#v present=%t", value, node, ok)
	}
	terminal, err := authority.terminal(node.leaf)
	if err != nil || terminal.kind != formalComponentBindings || len(terminal.bindings) != 1 {
		t.Fatalf("result terminal = %#v, %v", terminal, err)
	}
	return terminal.bindings[0]
}
