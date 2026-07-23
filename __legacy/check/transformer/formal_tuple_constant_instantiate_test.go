package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalTupleConstantInstantiatesEveryCompleteGroup(t *testing.T) {
	program, ref, constant := formalTupleConstantInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	tuple, err := algebra.instantiateConstant(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := algebra.validateTuple(tuple); err != nil {
		t.Fatal(err)
	}
	care, err := algebra.care(tuple)
	if err != nil || care != decisionTrue {
		t.Fatalf("instantiated care = %d, %v; want true", care, err)
	}

	span, _, authority, ok := algebra.span(constant.variable)
	if !ok {
		t.Fatal("constant variable has no run-local span")
	}
	var values, ordinary, coordinate int
	for _, want := range constant.groups {
		roots, err := algebra.groupRoots(tuple, want.group)
		if err != nil {
			t.Fatal(err)
		}
		leaves := make([]decisionLeaf, len(roots))
		for index, root := range roots {
			if int(root) >= len(algebra.decisions.nodes) || !algebra.decisions.nodes[root].terminal {
				t.Fatalf("constant group %d root %d is not terminal", want.group.global, root)
			}
			leaves[index] = algebra.decisions.nodes[root].leaf
		}
		switch want.group.kind {
		case formalFiberGroupValues:
			values++
			got, err := algebra.materializeValuesGroup(authority, want.group, leaves)
			if err != nil || !want.group.valueDomain.Equal(got, want.values) || !got.Top {
				t.Fatalf("Values constant = %#v, %v; want %#v", got, err, want.values)
			}
		case formalFiberGroupOrdinaryLane:
			ordinary++
			got, err := algebra.materializeOrdinaryGroup(authority, want.group, leaves)
			equal, equalErr := authority.product.LaneEqual(got, want.factor)
			if err != nil || equalErr != nil || !equal {
				t.Fatalf("ordinary constant %q was not preserved: materialize=%v equal=%v", want.group.lane.ID(), err, equalErr)
			}
		case formalFiberGroupCoordinateLane:
			coordinate++
			got, err := algebra.materializeCoordinateGroup(authority, span, want.group, leaves)
			equal, equalErr := authority.product.LaneEqual(got, want.factor)
			if err != nil || equalErr != nil || !equal {
				t.Fatalf("coordinate constant %q was not preserved: materialize=%v equal=%v", want.group.lane.ID(), err, equalErr)
			}
		default:
			t.Fatalf("unexpected constant group kind %d", want.group.kind)
		}
	}
	if values != 1 || ordinary == 0 || coordinate == 0 {
		t.Fatalf("instantiated group census = Values:%d ordinary:%d coordinate:%d", values, ordinary, coordinate)
	}
}

func TestFormalTupleConstantInstantiationRejectsForeignRef(t *testing.T) {
	program, _, _ := formalTupleConstantInstantiationFixture(t)
	_, foreign, _ := formalTupleConstantInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if tuple, err := algebra.instantiateConstant(foreign); err == nil || !tuple.bottom() {
		t.Fatalf("foreign constant instantiated as %#v, %v", tuple, err)
	}
}

func TestFormalTupleConstantInstantiationIsRunLocalAndIdempotent(t *testing.T) {
	program, ref, _ := formalTupleConstantInstantiationFixture(t)
	first, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	left, err := first.instantiateConstant(ref)
	if err != nil {
		t.Fatal(err)
	}
	right, err := first.instantiateConstant(ref)
	if err != nil {
		t.Fatal(err)
	}
	isolated, err := second.instantiateConstant(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !first.same(left, right) || first.err() != nil {
		t.Fatalf("same constant did not intern to one run-local tuple: %#v %#v, %v", left, right, first.err())
	}
	if left.root.owner == isolated.root.owner {
		t.Fatal("separate algebra runs shared a tuple directory")
	}
	if err := first.validateTuple(isolated); err == nil {
		t.Fatal("first algebra accepted the second run's tuple")
	}
	if err := second.validateTuple(left); err == nil {
		t.Fatal("second algebra accepted the first run's tuple")
	}
}

func formalTupleConstantInstantiationFixture(t *testing.T) (*RelationProgram, formalRelationTupleConstantRef, formalRelationTupleConstant) {
	t.Helper()
	program := formalRootInputTestProgram(t, standard.Registry())
	body := &program.bodies[0]
	raw := state.RecomposeValueLane(program.registry, body.domain, state.State{}, state.ValueLaneFactor{Top: true})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), raw),
	)
	program.formalSlots = program.formalFibers.slots
	components, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents = components
	template, err := freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	equation, ok := template.equation(program.formalRegion.roots[0])
	if !ok || len(equation.Seeds) != 1 {
		t.Fatalf("entry constant equation = %#v/%t", equation.Seeds, ok)
	}
	ref := equation.Seeds[0]
	constant, ok := ref.constant(equation.Cell)
	if !ok {
		t.Fatal("entry constant reference is invalid")
	}
	return program, ref, constant
}
