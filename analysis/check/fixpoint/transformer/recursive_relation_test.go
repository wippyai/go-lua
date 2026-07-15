package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRelationIsBottomRequiresOwnedIdentity(t *testing.T) {
	reg := standard.Registry()
	owner := NewArena(reg)
	if (Relation{}).IsBottom() {
		t.Fatal("ownerless zero relation reported Bottom")
	}
	if !(Relation{shape: Shape{Params: 1}, arena: owner}).IsBottom() {
		t.Fatal("owner-shaped empty relation did not report Bottom")
	}
	if (Relation{shape: Shape{Params: 1}, arena: owner, widened: true}).IsBottom() {
		t.Fatal("widened empty relation reported Bottom")
	}
	if (Relation{shape: Shape{Params: 1}, arena: owner, contextual: "top"}).IsBottom() {
		t.Fatal("contextual empty relation reported Bottom")
	}
	if (Relation{shape: Shape{Params: 1}, arena: owner, rows: []Row{{Guard: owner.True()}}}).IsBottom() {
		t.Fatal("non-empty relation reported Bottom")
	}
}

func TestComposeDirectCallRowsBottomValidatesBeforeZeroSuccessors(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	caller := NewBuilder(reg, shape, DefaultOutputCapabilityRegistry(), operationplan.New(cfg.New(), factflow.FactsInput{}))
	root := Root{Kind: RootParam}
	value := caller.Arena().Root(root)
	bindings := DirectCallBindings{
		Values: []ValueTerm{value},
		Paths:  []PathTerm{caller.Arena().Path(root)},
	}
	result := symbol.ID(81)
	validSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: 3, HasPoint: true, Final: true, Expanded: true,
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, result, path.NewPath(result, "result"),
		)},
	}).View()
	bottom := Relation{shape: shape, arena: NewArena(reg)}
	rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{
		Guard: caller.Arena().True(), Values: map[symbol.ID]ValueTerm{1: value},
	}, bottom, bindings, validSite, 4)
	if err != nil || len(rows) != 0 {
		t.Fatalf("Bottom composition = %d rows, %v; want zero successors", len(rows), err)
	}

	invalidSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: 3, HasPoint: true, Final: true, Expanded: true, Adjusted: true,
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, result, path.NewPath(result, "result"),
		)},
	}).View()
	if rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{Guard: caller.Arena().True()}, bottom, bindings, invalidSite, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom bypassed malformed-site validation: rows=%d err=%v", len(rows), err)
	}
	if rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{Guard: caller.Arena().True()}, bottom, DirectCallBindings{}, validSite, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom bypassed binding validation: rows=%d err=%v", len(rows), err)
	}
	malformed := bindings
	malformed.Values = []ValueTerm{ValueTerm(1 << 30)}
	if rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{Guard: caller.Arena().True()}, bottom, malformed, validSite, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom bypassed caller binding DAG validation: rows=%d err=%v", len(rows), err)
	}
	malformed = bindings
	malformed.Paths = []PathTerm{PathTerm(1 << 30)}
	if rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{Guard: caller.Arena().True()}, bottom, malformed, validSite, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom bypassed caller path binding validation: rows=%d err=%v", len(rows), err)
	}
	missingPoint := factflow.NewCallSite(factflow.CallSiteConfig{
		Final: true, Expanded: true,
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, result, path.NewPath(result, "result"),
		)},
	}).View()
	if rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{Guard: caller.Arena().True()}, bottom, bindings, missingPoint, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom bypassed exact source-point validation: rows=%d err=%v", len(rows), err)
	}
	if rows, err := ComposeDirectCallRows(caller, Shape{}, SymbolicCFGRow{Guard: caller.Arena().True()}, bottom, bindings, validSite, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom bypassed caller shape ownership: rows=%d err=%v", len(rows), err)
	}
	contractedBottom := bottom
	contractedBottom.paramContracts = []product.Value{product.Top()}
	if rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{Guard: caller.Arena().True()}, contractedBottom, bindings, validSite, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom silently discarded parameter contracts: rows=%d err=%v", len(rows), err)
	}
	foreignReg, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	foreign := Relation{shape: shape, arena: NewArena(foreignReg)}
	if rows, err := ComposeDirectCallRows(caller, shape, SymbolicCFGRow{Guard: caller.Arena().True()}, foreign, bindings, validSite, 4); err == nil || len(rows) != 0 {
		t.Fatalf("Bottom bypassed registry identity validation: rows=%d err=%v", len(rows), err)
	}
}

func TestPreparedDirectEquationSelfRecursiveBaseCaseLeastFixpoint(t *testing.T) {
	reg := standard.Registry()
	fixture := prepareRecursiveBaseCase(t, reg, "self")
	ref := CellRef{Function: 901}
	catalog, err := NewDirectCallCatalog(fixture.prepared.graph.Size(), map[cfg.Point]DirectCallTarget{
		fixture.call: {Cell: ref, Shape: fixture.prepared.Shape()},
	})
	if err != nil {
		t.Fatal(err)
	}
	equation, err := fixture.prepared.DirectEquation(ref, catalog)
	if err != nil {
		t.Fatalf("self-recursive equation rejected: %v", err)
	}
	cell, err := equation.Cell()
	if err != nil {
		t.Fatal(err)
	}
	evaluations := 0
	evaluate := cell.Equation
	cell.Equation = func(ctx context.Context, view RelationView) (Relation, error) {
		evaluations++
		return evaluate(ctx, view)
	}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{cell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	relation, ok := snapshot.Lookup(ref)
	if !ok || relation.IsBottom() || relation.ContextualReason() != "" || relation.Widened() || relation.Rows() != 2 {
		t.Fatalf("self-recursive base relation = ok:%v bottom:%v reason:%q widened:%v rows:%d keys:%v", ok, relation.IsBottom(), relation.ContextualReason(), relation.Widened(), relation.Rows(), recursiveRowKeys(relation))
	}
	if evaluations != 3 {
		t.Fatalf("self-recursive equation evaluations = %d, want base/growth/stable = 3", evaluations)
	}
	assertRecursiveBaseCaseResult(t, reg, relation, true, "self")
	assertRecursiveBaseCaseResult(t, reg, relation, false, "self")
}

func recursiveRowKeys(relation Relation) []string {
	keys := make([]string, len(relation.rows))
	for index, row := range relation.rows {
		keys[index] = rowKey(relation.arena, relation.effects, row)
	}
	return keys
}

func TestPreparedDirectEquationMutualRecursiveBaseCasesArePermutationDeterministic(t *testing.T) {
	reg := standard.Registry()
	leftFixture := prepareRecursiveBaseCase(t, reg, "left")
	rightFixture := prepareRecursiveBaseCase(t, reg, "right")
	leftRef, rightRef := CellRef{Function: 911}, CellRef{Function: 912}
	leftCatalog, err := NewDirectCallCatalog(leftFixture.prepared.graph.Size(), map[cfg.Point]DirectCallTarget{
		leftFixture.call: {Cell: rightRef, Shape: rightFixture.prepared.Shape()},
	})
	if err != nil {
		t.Fatal(err)
	}
	rightCatalog, err := NewDirectCallCatalog(rightFixture.prepared.graph.Size(), map[cfg.Point]DirectCallTarget{
		rightFixture.call: {Cell: leftRef, Shape: leftFixture.prepared.Shape()},
	})
	if err != nil {
		t.Fatal(err)
	}
	leftEquation, err := leftFixture.prepared.DirectEquation(leftRef, leftCatalog)
	if err != nil {
		t.Fatal(err)
	}
	rightEquation, err := rightFixture.prepared.DirectEquation(rightRef, rightCatalog)
	if err != nil {
		t.Fatal(err)
	}
	leftCell, err := leftEquation.Cell()
	if err != nil {
		t.Fatal(err)
	}
	rightCell, err := rightEquation.Cell()
	if err != nil {
		t.Fatal(err)
	}
	leftEvaluations, rightEvaluations := 0, 0
	leftEvaluate, rightEvaluate := leftCell.Equation, rightCell.Equation
	leftCell.Equation = func(ctx context.Context, view RelationView) (Relation, error) {
		leftEvaluations++
		return leftEvaluate(ctx, view)
	}
	rightCell.Equation = func(ctx context.Context, view RelationView) (Relation, error) {
		rightEvaluations++
		return rightEvaluate(ctx, view)
	}

	forward, err := SolveRelationCells(context.Background(), []RelationCell{leftCell, rightCell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if leftEvaluations != 3 || rightEvaluations != 3 {
		t.Fatalf("forward mutual equation evaluations = %d/%d, want 3/3", leftEvaluations, rightEvaluations)
	}
	reverse, err := SolveRelationCells(context.Background(), []RelationCell{rightCell, leftCell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if leftEvaluations != 6 || rightEvaluations != 6 {
		t.Fatalf("forward+reverse mutual equation evaluations = %d/%d, want 6/6", leftEvaluations, rightEvaluations)
	}
	for _, ref := range []CellRef{leftRef, rightRef} {
		want, wantOK := forward.Lookup(ref)
		got, gotOK := reverse.Lookup(ref)
		if !wantOK || !gotOK || !EqualRelation(want, got) || want.IsBottom() || want.ContextualReason() != "" || want.Widened() || want.Rows() != 2 {
			t.Fatalf("permutation relation %v = forward:%#v/%v reverse:%#v/%v", ref, want, wantOK, got, gotOK)
		}
	}
	left, _ := forward.Lookup(leftRef)
	right, _ := forward.Lookup(rightRef)
	assertRecursiveBaseCaseResult(t, reg, left, true, "left")
	assertRecursiveBaseCaseResult(t, reg, left, false, "right")
	assertRecursiveBaseCaseResult(t, reg, right, true, "right")
	assertRecursiveBaseCaseResult(t, reg, right, false, "left")
}

func TestContractedBottomPublishesCallEntrySidecarAndSparseProjectionAtomically(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	param, result := symbol.ID(1201), symbol.ID(1202)
	scalar, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, _ := factflow.NewPathValueSource(path.NewPath(param, "argument").Key(), 0, 0, 0, scalar)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: call, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{argument},
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, result, path.NewPath(result, "result"),
		)},
	})
	lowered := wir.NewBody("contracted-bottom")
	start := lowered.Len()
	lowered.Emit(wir.Instruction{Op: wir.OpCall, Point: call})
	lowered.SetPointRange(call, start, lowered.Len())
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 91
	contract := typevalue.String(reg)
	callSurface, err := operationplan.SealCallSurface(owner, graph.Size(), []cfg.Point{call}, []operationplan.CallSurfaceSite{{
		Point: call, Target: operationplan.RejectedCallSurfaceTarget(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{call: site},
	}).WithObservationIdentity(owner, lowered, graph).
		WithCallSurface(callSurface).
		WithBoundaryParams([]symbol.ID{param}).
		WithBoundaryParamContracts([]product.Value{contract})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	calleeRef := CellRef{Function: 1203}
	catalog, err := NewDirectCallCatalog(graph.Size(), map[cfg.Point]DirectCallTarget{
		call: {Cell: calleeRef, Shape: Shape{Params: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bottom := Relation{
		shape: Shape{Params: 1}, arena: NewArena(reg), observationComplete: true,
		paramContracts: []product.Value{contract},
	}
	view := RelationView{
		values:  map[CellRef]Relation{calleeRef: bottom},
		allowed: map[CellRef]struct{}{calleeRef: {}},
	}
	relation := prepared.EvaluateDirect(view, catalog)
	if !relation.IsBottom() || relation.ContextualReason() != "" || !relation.ObservationCoverageComplete() ||
		len(relation.annotations.observations) != 1 || len(relation.annotations.obligations) != 1 ||
		relation.projectionTrace == nil || relation.projectionTraceReason != "" {
		t.Fatalf("contracted Bottom = bottom:%v reason:%q complete:%v annotations:%d/%d trace:%v/%q",
			relation.IsBottom(), relation.ContextualReason(), relation.ObservationCoverageComplete(),
			len(relation.annotations.observations), len(relation.annotations.obligations), relation.projectionTrace != nil, relation.projectionTraceReason)
	}
	argumentGuard, resultGuard := Guard(0), Guard(0)
	for _, slot := range relation.projectionTrace.slots {
		kind, observed := slot.requirement.ObservationKind()
		if !observed {
			continue
		}
		switch ObservationKind(kind) {
		case ObservationCallArgument:
			argumentGuard = slot.guard
		case ObservationCallResult:
			resultGuard = slot.guard
		}
	}
	if argumentGuard != prepared.builder.Arena().True() || resultGuard != prepared.builder.Arena().False() {
		t.Fatalf("nonreturning call reach guards = argument:%d result:%d, want pre-call true/post-call false", argumentGuard, resultGuard)
	}
	actual := typevalue.LiteralBool(reg, false)
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{actual}, []path.Path{path.NewPlaceholder(0)})
	if err != nil {
		t.Fatal(err)
	}
	detailed, exact := relation.SpecializeDetailed(cursor, nil, SpecializationContext{})
	items := detailed.Observations.Items()
	if !exact || len(items) != 1 || items[0].Kind != ObservationCallArgument ||
		!product.Equal(reg, items[0].Actual, actual) || !items[0].HasExpected || !product.Equal(reg, items[0].Expected, contract) {
		t.Fatalf("contracted Bottom sidecar specialization = %#v/%v", items, exact)
	}
	requirements, sealed := plan.ObservationRequirements()
	surface, surfaceOK := plan.CallSurface()
	if !sealed || !surfaceOK {
		t.Fatal("contracted Bottom observation authority is not sealed")
	}
	identity := evaluated.Identity{
		Body:        owner,
		Relation:    evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		Entry:       evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		Lineage:     evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		Registry:    evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		CallSurface: surface.Digest(), Schema: requirements.SchemaID(), Inventory: requirements.ConsumerInventoryID(),
		PointCount: uint32(graph.Size()),
	}
	identity.View, err = evaluated.SealProjectionView(requirements, false)
	if err != nil {
		t.Fatal(err)
	}
	root, err := relation.EvaluateSparseRoot(context.Background(), EvaluatedRootRequest{
		Identity: identity, ExpectedIdentity: identity, Requirements: requirements, CallSurface: surface,
	}, cursor, SpecializationContext{})
	if err != nil || root.Identity() != identity {
		t.Fatalf("contracted Bottom sparse projection = identity:%v err:%v", root.Identity() == identity, err)
	}
	authority, err := relation.DeriveEvaluatedRootAuthority(context.Background(), cursor, nil)
	if err != nil {
		t.Fatal(err)
	}
	variant := relation
	variant.annotations.observations = append([]ObservationTerm(nil), relation.annotations.observations...)
	variant.annotations.observations[0].Expected = 0
	variantAuthority, err := variant.DeriveEvaluatedRootAuthority(context.Background(), cursor, nil)
	if err != nil {
		t.Fatal(err)
	}
	if EqualRelation(relation, variant) || authority.RelationIdentity() == variantAuthority.RelationIdentity() {
		t.Fatal("publication equality or canonical identity ignored relation-level annotations")
	}
	incomplete := relation
	incomplete.annotations.observations = nil
	incomplete.observationComplete = false
	incomplete.projectionTraceReason = "projection trace: required observation evidence is incomplete"
	incomplete.projectionTrace = cloneSparseProjectionTrace(relation.projectionTrace)
	for index := range incomplete.projectionTrace.slots {
		incomplete.projectionTrace.slots[index].observed = nil
	}
	recovered := JoinRelation(incomplete, relation)
	if !recovered.ObservationCoverageComplete() || recovered.projectionTraceReason != "" || !EqualRelation(recovered, relation) {
		t.Fatalf("joined evidence did not repair prior incomplete coverage: complete=%v reason=%q", recovered.ObservationCoverageComplete(), recovered.projectionTraceReason)
	}

	equation, err := prepared.DirectEquation(CellRef{Function: 1204}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	cell, err := equation.Cell()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	snapshot, err := SolveRelationCells(ctx, []RelationCell{cell, {
		Ref: calleeRef, Arena: bottom.arena, Shape: bottom.shape,
		Equation: func(context.Context, RelationView) (Relation, error) {
			cancel()
			return bottom, nil
		},
	}}, RelationSolveOptions{})
	if err == nil || len(snapshot.Entries()) != 0 {
		t.Fatalf("canceled contracted-Bottom transaction published %d entries, err=%v", len(snapshot.Entries()), err)
	}
}

type observedRecursiveLoopFixture struct {
	prepared *PreparedPlanCompiler
	call     cfg.Point
	owner    lexicalidentity.StableLexicalBodyID
	contract product.Value
}

func prepareObservedRecursiveLoop(t testing.TB, reg *axis.Registry, name string) observedRecursiveLoopFixture {
	t.Helper()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	baseReturn := graph.AddNode(cfg.NodeReturn)
	call := graph.AddNode(cfg.NodeCall)
	recursiveReturn := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, baseReturn, true)
	graph.AddEdge(branch, call, false)
	graph.AddEdge(baseReturn, graph.Exit(), false)
	graph.AddEdge(call, recursiveReturn, false)
	graph.AddEdge(recursiveReturn, graph.Exit(), false)

	param, result := symbol.ID(1301), symbol.ID(1302)
	scalar, _ := factflow.NewValueSourceShape(true, false, false, false)
	paramSource, _ := factflow.NewPathValueSource(path.NewPath(param, "param").Key(), 0, factflow.NoValueSourceIndex, 0, scalar)
	condition, _ := factflow.NewBranchCondition(paramSource, true)
	baseSource, _ := factflow.NewStringLiteralValueSource("base", 0, 0, 0, scalar)
	recursiveSource, _ := factflow.NewPathValueSource(path.NewPath(result, "result").Key(), 0, factflow.NoValueSourceIndex, 0, scalar)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: call, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{paramSource},
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, result, path.NewPath(result, "result"),
		)},
	})
	var owner lexicalidentity.StableLexicalBodyID
	copy(owner[:], []byte(name))
	lowered := wir.NewBody(name)
	start := lowered.Len()
	lowered.Emit(wir.Instruction{Op: wir.OpCall, Point: call})
	lowered.SetPointRange(call, start, lowered.Len())
	lowered.AssignDebugPointOrdinals(graph)
	callSurface, err := operationplan.SealCallSurface(owner, graph.Size(), []cfg.Point{call}, []operationplan.CallSurfaceSite{{
		Point: call, Target: operationplan.RejectedCallSurfaceTarget(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	contract := typevalue.String(reg)
	plan := operationplan.New(graph, factflow.FactsInput{
		BranchConditionSources: map[cfg.Point]factflow.BranchCondition{branch: condition},
		CallSites:              map[cfg.Point]factflow.CallSite{call: site},
		Returns: map[cfg.Point]factflow.Return{
			baseReturn:      factflow.NewReturn([]factflow.ValueSource{baseSource}),
			recursiveReturn: factflow.NewReturn([]factflow.ValueSource{recursiveSource}),
		},
	}).WithObservationIdentity(owner, lowered, graph).
		WithCallSurface(callSurface).
		WithBoundaryParams([]symbol.ID{param}).
		WithBoundaryParamContracts([]product.Value{contract})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	return observedRecursiveLoopFixture{prepared: prepared, call: call, owner: owner, contract: contract}
}

func TestSelfRecursiveObservedLoopKeepsOwnerLocalCallEntry(t *testing.T) {
	reg := standard.Registry()
	fixture := prepareObservedRecursiveLoop(t, reg, "self-observed-loop")
	prepared, call, contract := fixture.prepared, fixture.call, fixture.contract
	ref := CellRef{Function: 1303}
	catalog, err := NewDirectCallCatalog(prepared.graph.Size(), map[cfg.Point]DirectCallTarget{call: {Cell: ref, Shape: Shape{Params: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	equation, err := prepared.DirectEquation(ref, catalog)
	if err != nil {
		t.Fatal(err)
	}
	cell, err := equation.Cell()
	if err != nil {
		t.Fatal(err)
	}
	if !cell.Bottom.IsBottom() || cell.Bottom.effects == nil || cell.Bottom.descriptors == nil || cell.Bottom.authority == nil ||
		len(cell.Bottom.paramContracts) != 1 || !product.Equal(reg, cell.Bottom.paramContracts[0], contract) {
		t.Fatalf("owner-complete recursive Bottom = %#v", cell.Bottom)
	}
	evaluations := 0
	evaluate := cell.Equation
	cell.Equation = func(ctx context.Context, view RelationView) (Relation, error) {
		evaluations++
		return evaluate(ctx, view)
	}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{cell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	relation, ok := snapshot.Lookup(ref)
	if !ok || relation.IsBottom() || relation.ContextualReason() != "" || relation.Widened() || relation.Rows() != 1 ||
		!relation.ObservationCoverageComplete() || relation.projectionTrace == nil || relation.projectionTraceReason != "" || evaluations != 2 {
		t.Fatalf("observed recursive relation = ok:%v bottom:%v reason:%q widened:%v rows:%d complete:%v trace:%v/%q evals:%d",
			ok, relation.IsBottom(), relation.ContextualReason(), relation.Widened(), relation.Rows(), relation.ObservationCoverageComplete(), relation.projectionTrace != nil, relation.projectionTraceReason, evaluations)
	}
	if len(relation.annotations.observations) != 1 || relation.annotations.observations[0].Route != ([32]byte{}) {
		t.Fatalf("recursive semantic relation did not remain owner-local: %#v", relation.annotations.observations)
	}
	trueCursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralBool(reg, true)}, nil)
	trueResult, trueExact := relation.SpecializeDetailed(trueCursor, nil, SpecializationContext{})
	if !trueExact || len(trueResult.Summary.Returns) != 1 || len(trueResult.Observations.Items()) != 0 {
		t.Fatalf("base exit specialization = %#v observations=%#v exact=%v", trueResult.Summary, trueResult.Observations.Items(), trueExact)
	}
	falseCursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralBool(reg, false)}, nil)
	falseResult, falseExact := relation.SpecializeDetailed(falseCursor, nil, SpecializationContext{})
	items := falseResult.Observations.Items()
	if !falseExact || len(falseResult.Summary.Returns) != 0 || len(items) != 1 {
		t.Fatalf("recursive nonreturn specialization = %#v observations=%#v exact=%v", falseResult.Summary, items, falseExact)
	}
	for _, item := range items {
		if item.Kind != ObservationCallArgument || !item.HasExpected || !product.Equal(reg, item.Actual, typevalue.LiteralBool(reg, false)) || !product.Equal(reg, item.Expected, contract) {
			t.Fatalf("recursive invalid argument observation = %#v", item)
		}
	}
}

type recursiveBaseCaseFixture struct {
	prepared *PreparedPlanCompiler
	call     cfg.Point
}

func prepareRecursiveBaseCase(t testing.TB, reg *axis.Registry, literal string) recursiveBaseCaseFixture {
	t.Helper()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	baseReturn := graph.AddNode(cfg.NodeReturn)
	call := graph.AddNode(cfg.NodeCall)
	recursiveReturn := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, baseReturn, true)
	graph.AddEdge(branch, call, false)
	graph.AddEdge(baseReturn, graph.Exit(), false)
	graph.AddEdge(call, recursiveReturn, false)
	graph.AddEdge(recursiveReturn, graph.Exit(), false)

	param, result := symbol.ID(1001), symbol.ID(1002)
	scalar, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("scalar value-source shape rejected")
	}
	conditionSource, ok := factflow.NewPathValueSource(path.NewPath(param, "condition").Key(), 0, factflow.NoValueSourceIndex, 0, scalar)
	if !ok {
		t.Fatal("condition source rejected")
	}
	condition, ok := factflow.NewBranchCondition(conditionSource, true)
	if !ok {
		t.Fatal("branch condition rejected")
	}
	baseSource, _ := factflow.NewStringLiteralValueSource(literal, 0, 0, 0, scalar)
	recursiveArg, _ := factflow.NewBoolLiteralValueSource(true, 0, 0, 0, scalar)
	recursiveSource, _ := factflow.NewPathValueSource(path.NewPath(result, "recursive-result").Key(), 0, 0, 0, scalar)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: call, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{recursiveArg},
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, result, path.NewPath(result, "recursive-result"),
		)},
	})
	plan := operationplan.New(graph, factflow.FactsInput{
		BranchConditionSources: map[cfg.Point]factflow.BranchCondition{branch: condition},
		CallSites:              map[cfg.Point]factflow.CallSite{call: site},
		Returns: map[cfg.Point]factflow.Return{
			baseReturn:      factflow.NewReturn([]factflow.ValueSource{baseSource}),
			recursiveReturn: factflow.NewReturn([]factflow.ValueSource{recursiveSource}),
		},
	}).WithBoundaryParams([]symbol.ID{param})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{Params: 1})
	if err != nil {
		t.Fatalf("prepare recursive base case: %v", err)
	}
	return recursiveBaseCaseFixture{prepared: prepared, call: call}
}

func assertRecursiveBaseCaseResult(t testing.TB, reg *axis.Registry, relation Relation, input bool, want string) {
	t.Helper()
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralBool(reg, input)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, exact := relation.Specialize(cursor, nil, nil)
	wantValue := typevalue.LiteralString(reg, want)
	if !exact || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], wantValue) {
		t.Fatalf("specialize(%v) = %#v/%v, want %q", input, got, exact, want)
	}
}
