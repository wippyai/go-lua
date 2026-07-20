package transformer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalRootEntrySeedMatchesCanonicalCoordinateSeedAcrossFullProduct(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	param := statekey.SymbolValue(101)
	capture := statekey.SymbolValue(102)
	global := statekey.SymbolValue(103)
	allocation := identity.ID{Kind: "test", Site: "formal-root-entry", Index: 1}

	raw := state.Reachable(body.productDomain.Lattice().Bottom()).
		WriteValue(reg, param, typevalue.LiteralString(reg, "raw-entry")).
		WriteValue(reg, global, typevalue.LiteralString(reg, "raw-global")).
		WritePlacement(allocation, placement.SharedHeap)
	initial := state.Reachable(body.productDomain.Lattice().Bottom()).
		WriteValue(reg, param, typevalue.LiteralString(reg, "initial-override")).
		WriteValue(reg, global, typevalue.LiteralString(reg, "initial-global")).
		WritePlacement(allocation, placement.Stack)
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: param, Value: typevalue.LiteralString(reg, "must-not-overwrite")},
		{Slot: capture, Value: typevalue.LiteralString(reg, "missing-only-fill")},
	})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), initial),
	)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
	})
	body = &program.bodies[0]

	// This is the pre-existing coordinate root law, written out independently:
	// entry override, missing-only EntrySeed, Reachable normalization.
	want := initial
	var err error
	want, err = body.entrySeedPlan.Apply(reg, want)
	if err != nil {
		t.Fatal(err)
	}
	want = state.NormalizeForDomain(body.domain, state.Reachable(want))
	prepared, err := prepareRelationRootEntry(program, body, raw)
	if err != nil || !body.domain.Equal(want, prepared) {
		t.Fatalf("canonical root entry = equal:%t err:%v", body.domain.Equal(want, prepared), err)
	}
	if got := prepared.ReadValue(reg, param); !product.Equal(reg, got, typevalue.LiteralString(reg, "initial-override")) {
		t.Fatalf("InitialStatePlan did not override raw entry: %#v", got)
	}
	if got := prepared.ReadValue(reg, capture); !product.Equal(reg, got, typevalue.LiteralString(reg, "missing-only-fill")) {
		t.Fatalf("EntrySeedPlan did not fill missing capture: %#v", got)
	}
	if got := prepared.ReadPlacement(allocation); got != placement.Stack {
		t.Fatalf("full-product initial override placement = %v, want stack", got)
	}

	execution, err := executeFormalRootRelation(context.Background(), program, body.body, raw)
	if err != nil || execution == nil {
		t.Fatalf("formal root invocation = %#v, %v", execution, err)
	}
	rootCell := program.formalRegion.roots[body.variable-1]
	rootTuple := execution.values[rootCell]
	if rootTuple.bottom() {
		t.Fatal("formal root invocation published Bottom")
	}
	wantConstant, err := freezeFormalRelationTupleConstant(program, body.variable, want)
	if err != nil {
		t.Fatal(err)
	}
	assertFormalRootTupleMatchesConstant(t, execution.algebra, rootTuple, wantConstant)
	publication, err := execution.Publication(body.body)
	if err != nil {
		t.Fatal(err)
	}
	published, present, err := publication.PointInput(context.Background(), 1, 0)
	if err != nil || !present || !body.domain.Equal(published, want) {
		t.Fatalf("formal root publication = equal:%t err:%v", body.domain.Equal(published, want), err)
	}
	rootCoordinate, present := publication.node(body.relation.code.root)
	if !present {
		t.Fatal("formal publication omitted selected root diagnostics")
	}
	wantDiagnostics, wantReachable, err := execution.algebra.formalDiagnosticOutput(context.Background(), rootTuple)
	if err != nil {
		t.Fatal(err)
	}
	gotDiagnostics, gotReachable, err := publication.joinDiagnosticOutput(context.Background(), rootCoordinate)
	if err != nil || gotReachable != wantReachable || !callpayload.DiagnosticOutputLattice(reg).Equal(gotDiagnostics, wantDiagnostics) {
		t.Fatalf("formal root diagnostics = reachable:%t/%t equal:%t err:%v", gotReachable, wantReachable, callpayload.DiagnosticOutputLattice(reg).Equal(gotDiagnostics, wantDiagnostics), err)
	}

	// The selected production root replaces its template root binding and entry
	// constant; joining either would introduce non-ground symbolic leaves.
	for _, group := range wantConstant.groups {
		roots, rootErr := execution.algebra.groupRoots(rootTuple, group.group)
		if rootErr != nil {
			t.Fatal(rootErr)
		}
		for _, root := range roots {
			if int(root) >= len(execution.algebra.decisions.nodes) || !execution.algebra.decisions.nodes[root].terminal {
				t.Fatalf("production root retained symbolic/template input in group %d", group.group.global)
			}
		}
	}
}

func TestFormalPublicationPartitionsOneCorrelatedLeafVectorWithoutCrossProduct(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	guardTerm := base.bodies[0].relation.arena.Truthy(base.bodies[0].relation.arena.Root(Root{Kind: RootParam, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{}, {kind: relationNodeChoice, guard: guardTerm, whenTrue: 2, whenFalse: 3},
		{kind: relationNodeBottom}, {kind: relationNodeBottom},
	})
	body := &program.bodies[0]
	execution, err := executeFormalRootRelation(context.Background(), program, body.body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := execution.Publication(body.body)
	if err != nil {
		t.Fatal(err)
	}
	coordinate, present := publication.node(body.relation.code.root)
	if !present {
		t.Fatal("missing formal root coordinate")
	}
	tuple := execution.values[coordinate.cell]
	span, _, authority, ok := execution.algebra.span(body.variable)
	if !ok {
		t.Fatal("missing formal publication span")
	}
	values, ok := span.valuesGroup()
	if !ok {
		t.Fatal("missing formal Values group")
	}
	guard, err := execution.algebra.decisionForGuard(body.variable, 0, body.relation.arena, guardTerm)
	if err != nil {
		t.Fatal(err)
	}
	writeCorrelatedValue := func(root Root, whenTrue, whenFalse product.Value) {
		t.Helper()
		slot, exact := program.formalSlots.Slot(body.body, root)
		member, memberOK := values.slot(slot)
		_, ordinalOK := member.address(values.descriptor)
		if !exact || !memberOK || !ordinalOK {
			t.Fatalf("formal correlated root %#v is outside Values", root)
		}
		trueLeaf, leafErr := authority.internGroundValue(whenTrue)
		if leafErr != nil {
			t.Fatal(leafErr)
		}
		falseLeaf, leafErr := authority.internGroundValue(whenFalse)
		if leafErr != nil {
			t.Fatal(leafErr)
		}
		decision, decisionErr := execution.algebra.decisions.condition(context.Background(), guard,
			execution.algebra.decisions.terminal(trueLeaf), execution.algebra.decisions.terminal(falseLeaf))
		if decisionErr != nil {
			t.Fatal(decisionErr)
		}
		roots, rootsErr := execution.algebra.groupRoots(tuple, values.descriptor)
		if rootsErr != nil {
			t.Fatal(rootsErr)
		}
		roots[member.position] = decision
		next, applyErr := execution.algebra.applyGroupRoots(span, tuple.root.owner, tuple.root, authority, values.descriptor, roots)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		tuple = execution.algebra.normalize(formalRelationTuple{variable: tuple.variable, root: next})
	}
	leftTrue, leftFalse := typevalue.LiteralString(reg, "left-true"), typevalue.LiteralString(reg, "left-false")
	rightTrue, rightFalse := typevalue.LiteralString(reg, "right-true"), typevalue.LiteralString(reg, "right-false")
	writeCorrelatedValue(Root{Kind: RootMiddle, Index: 0}, leftTrue, leftFalse)
	writeCorrelatedValue(Root{Kind: RootMiddle, Index: 1}, rightTrue, rightFalse)
	quiet := callpayload.DiagnosticOutput{SuspensionKnown: true}
	suspending := callpayload.DiagnosticOutput{SuspensionKnown: true, MaySuspend: true}
	quietLeaf, err := authority.internDiagnostics(quiet)
	if err != nil {
		t.Fatal(err)
	}
	suspendingLeaf, err := authority.internDiagnostics(suspending)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticDecision, err := execution.algebra.decisions.condition(context.Background(), guard,
		execution.algebra.decisions.terminal(suspendingLeaf), execution.algebra.decisions.terminal(quietLeaf))
	if err != nil {
		t.Fatal(err)
	}
	diagnosticDescriptor := span.forest.descriptors[span.first+int(publication.diagnostic)]
	tuple, err = execution.algebra.writeScalar(tuple, diagnosticDescriptor, diagnosticDecision)
	if err != nil {
		t.Fatal(err)
	}
	execution.values[coordinate.cell] = tuple

	partitions, err := execution.algebra.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{tuple: tuple, ordinals: publication.ordinals}}, nil)
	if err != nil || len(partitions) != 2 {
		t.Fatalf("correlated publication partitions = %d, %v; want 2, not Cartesian 4", len(partitions), err)
	}
	published, present, err := publication.PointInput(context.Background(), 1, 0)
	if err != nil || !present {
		t.Fatal(err)
	}
	if got, want := published.ReadValue(reg, statekey.SymbolValue(101)), product.Join(reg, leftTrue, leftFalse); !product.Equal(reg, got, want) {
		t.Fatalf("left correlated publication = %#v, want %#v", got, want)
	}
	if got, want := published.ReadValue(reg, statekey.SymbolValue(102)), product.Join(reg, rightTrue, rightFalse); !product.Equal(reg, got, want) {
		t.Fatalf("right correlated publication = %#v, want %#v", got, want)
	}
	diagnostics, reachable, err := publication.joinDiagnosticOutput(context.Background(), coordinate)
	wantDiagnostics := callpayload.DiagnosticOutputLattice(reg).Join(suspending, quiet)
	if err != nil || !reachable || !callpayload.DiagnosticOutputLattice(reg).Equal(diagnostics, wantDiagnostics) {
		t.Fatalf("correlated diagnostic publication = %#v/%t, %v; want %#v", diagnostics, reachable, err, wantDiagnostics)
	}
}

func TestFormalRootEntrySeedCancellationPublishesNothingAndDoesNotMutateTemplate(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
	})
	wantEquations := cloneFormalRelationEquations(program.formalTemplate.equations)
	wantConstants := append([]formalRelationTupleConstant(nil), program.formalTemplate.constants...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := executeFormalRootRelation(ctx, program, program.bodies[0].body, state.State{})
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled formal root invocation = %#v, %v", result, err)
	}
	if !reflect.DeepEqual(program.formalTemplate.equations, wantEquations) ||
		!reflect.DeepEqual(program.formalTemplate.constants, wantConstants) {
		t.Fatal("canceled root invocation mutated sealed formal template")
	}
}

func TestFormalPublicationCancellationReturnsNoPartialState(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
	})
	body := &program.bodies[0]
	execution, err := executeFormalRootRelation(context.Background(), program, body.body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := execution.Publication(body.body)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, present, err := publication.PointInput(ctx, 1, 0)
	if !errors.Is(err, context.Canceled) || !present || !reflect.DeepEqual(got, state.State{}) {
		t.Fatalf("canceled formal publication = %#v, %v", got, err)
	}
}

func TestFormalPublicationRejectsFreeSymbolicRootWithoutConcreteEdge(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
	})
	body := &program.bodies[0]
	// Syntax-only execution intentionally retains free IN bindings in the MID
	// product. Publication may name the body cell, but it must not erase those
	// roots into a concrete State without a selected invocation edge.
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := execution.Publication(body.body)
	if err != nil {
		t.Fatal(err)
	}
	coordinate, present := publication.node(body.relation.code.root)
	if !present {
		t.Fatal("missing symbolic body root coordinate")
	}
	if got, pointPresent, err := publication.PointInput(context.Background(), 1, 0); err == nil || !pointPresent || !reflect.DeepEqual(got, state.State{}) {
		t.Fatalf("free symbolic body root published concrete State %#v, %v", got, err)
	}
	diagnostics, reachable, err := publication.joinDiagnosticOutput(context.Background(), coordinate)
	if err != nil || !reachable || !callpayload.DiagnosticOutputLattice(program.registry).Equal(
		diagnostics, callpayload.DiagnosticOutputLattice(program.registry).Bottom(),
	) {
		t.Fatalf("symbolic body diagnostics = %#v/%t, %v", diagnostics, reachable, err)
	}
}

func assertFormalRootTupleMatchesConstant(t *testing.T, algebra *formalTupleAlgebra, tuple formalRelationTuple, want formalRelationTupleConstant) {
	t.Helper()
	span, _, authority, ok := algebra.span(want.variable)
	if !ok {
		t.Fatal("formal root tuple has no span")
	}
	for _, group := range want.groups {
		roots, err := algebra.groupRoots(tuple, group.group)
		if err != nil {
			t.Fatal(err)
		}
		leaves := make([]decisionLeaf, len(roots))
		for index, root := range roots {
			if int(root) >= len(algebra.decisions.nodes) || !algebra.decisions.nodes[root].terminal {
				t.Fatalf("formal root group %d is not ground", group.group.global)
			}
			leaves[index] = algebra.decisions.nodes[root].leaf
		}
		switch group.group.kind {
		case formalFiberGroupValues:
			got, materializeErr := algebra.materializeValuesGroup(authority, group.group, leaves)
			if materializeErr != nil || !group.group.valueDomain.Equal(got, group.values) {
				t.Fatalf("Values root factor differs: %v", materializeErr)
			}
		case formalFiberGroupOrdinaryLane:
			got, materializeErr := algebra.materializeOrdinaryGroup(authority, group.group, leaves)
			equal, equalErr := authority.product.LaneEqual(got, group.factor)
			if materializeErr != nil || equalErr != nil || !equal {
				t.Fatalf("ordinary root factor %q differs: %v/%v", group.group.lane.ID(), materializeErr, equalErr)
			}
		case formalFiberGroupCoordinateLane:
			got, materializeErr := algebra.materializeCoordinateGroup(authority, span, group.group, leaves)
			equal, equalErr := authority.product.LaneEqual(got, group.factor)
			if materializeErr != nil || equalErr != nil || !equal {
				t.Fatalf("coordinate root factor %q differs: %v/%v", group.group.lane.ID(), materializeErr, equalErr)
			}
		default:
			t.Fatalf("invalid formal root group kind %d", group.group.kind)
		}
	}
}
