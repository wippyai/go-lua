package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalRootEntrySeedsMatchConcreteLawInRecursiveBody(t *testing.T) {
	program := formalRootInputTestProgram(t, standard.Registry())
	body := &program.bodies[0]
	reg := program.registry

	values := []product.Value{
		typevalue.LiteralString(reg, "param-default"),
		typevalue.LiteralString(reg, "capture-default"),
		typevalue.LiteralString(reg, "global-default"),
		typevalue.LiteralString(reg, "ambient-default"),
	}
	seeds := make([]state.ValueSeed, len(body.relation.arena.middle.entries))
	for index, entry := range body.relation.arena.middle.entries {
		register, ok := body.relation.arena.middle.register(entry.middle)
		if !ok {
			t.Fatalf("Middle %d has no register", index)
		}
		seeds[index] = state.ValueSeed{Slot: register.slot, Value: values[index]}
	}
	body.entrySeedPlan = state.NewEntrySeedPlan(seeds)
	actualCapture := typevalue.LiteralString(reg, "capture-actual")
	captureRegister, _ := body.relation.arena.middle.register(body.relation.arena.middle.entries[1].middle)
	raw := state.State{}.WriteValue(reg, captureRegister.slot, actualCapture)
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), raw),
	)
	// Use the typed loop binder owned by relationCode rather than manufacturing
	// an untyped graph cycle.  WTO may revisit the root's recursive component,
	// while root instantiation remains one idempotent equation law.
	guard := body.relation.arena.Truthy(body.relation.arena.Root(Root{Kind: RootParam, Index: 0}))
	const binder loopMuTerm = 1
	program = formalRelationExecutorTestProgramFromBase(t, program, []relationNode{
		{},
		{kind: relationNodeLoopMu, binder: binder, body: 2, exits: []relationRootRef{3}},
		{kind: relationNodeChoice, guard: guard, whenTrue: 4, whenFalse: 5},
		{kind: relationNodeOutcome, outcome: 1},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: binder}}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, binder: binder, route: 0}}},
	})
	body = &program.bodies[0]
	template := program.formalTemplate
	equation, ok := template.equation(program.formalRegion.roots[0])
	if !ok {
		t.Fatal("recursive root has no equation")
	}
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	tuple, err := algebra.instantiateRootEquation(equation)
	if err != nil {
		t.Fatal(err)
	}
	regions, err := algebra.tupleLeafRegions(tuple)
	if err != nil || len(regions) != 1 {
		t.Fatalf("entry leaf regions = %d/%v", len(regions), err)
	}
	concrete, err := body.entrySeedPlan.Apply(reg, state.Reachable(raw))
	if err != nil {
		t.Fatal(err)
	}
	for index, entry := range body.relation.arena.middle.entries {
		register, _ := body.relation.arena.middle.register(entry.middle)
		want := concrete.ReadValue(reg, register.slot)
		got, exact := regions[0].evaluator.valueAtRoot(entry.middle)
		if !exact || !product.Equal(reg, got, want) {
			t.Fatalf("Middle %d formal/concrete = %#v/%#v exact=%t", index, got, want, exact)
		}
	}
	if got := concrete.ReadValue(reg, captureRegister.slot); !product.Equal(reg, got, actualCapture) {
		t.Fatal("concrete reference did not preserve capture actual")
	}
}

func TestFormalRootInputInstantiationBindsEveryRootAndCompleteGroup(t *testing.T) {
	program, equation := formalRootInputInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	tuple, err := algebra.instantiateRootEquation(equation)
	if err != nil {
		t.Fatal(err)
	}
	if err := algebra.validateTuple(tuple); err != nil {
		t.Fatal(err)
	}
	root := equation.Operator.rootInput
	span, directory, authority, ok := algebra.span(root.variable)
	if !ok {
		t.Fatal("root input has no run-local span")
	}
	for _, binding := range root.bindings {
		slot, ok := program.formalSlots.Slot(program.bodies[root.variable-1].body, binding.middle)
		if !ok {
			t.Fatalf("Middle root %#v has no formal slot", binding.middle)
		}
		var descriptor formalFiberDescriptor
		for _, candidate := range span.descriptors() {
			if candidate.role == formalFiberMiddleValue && candidate.slot == slot {
				if descriptor.role != formalFiberInvalid {
					t.Fatalf("Middle root %#v has ambiguous descriptors", binding.middle)
				}
				descriptor = candidate
			}
		}
		ordinal, present := span.ordinal(descriptor)
		if descriptor.role == formalFiberInvalid || !present {
			t.Fatalf("Middle root %#v has no value descriptor", binding.middle)
		}
		value, err := directory.valueAt(tuple.root, ordinal)
		if err != nil {
			t.Fatal(err)
		}
		ref := decisionRef(value)
		if int(ref) >= len(algebra.decisions.nodes) || !algebra.decisions.nodes[ref].terminal {
			t.Fatalf("Middle root %#v is not one terminal", binding.middle)
		}
		terminal, err := authority.terminal(algebra.decisions.nodes[ref].leaf)
		if err != nil || terminal.kind != formalComponentBindings || len(terminal.bindings) != 1 {
			t.Fatalf("Middle root %#v terminal = %#v, %v", binding.middle, terminal, err)
		}
		got := terminal.bindings[0]
		if got.value != (relationArenaValueRef{owner: root.variable, arena: binding.arena, term: binding.inputValue}) ||
			!got.pathPresent || got.path != (relationArenaPathRef{owner: root.variable, arena: binding.arena, term: binding.inputPath}) || got.apply.present() {
			t.Fatalf("Middle root %#v binding = %#v", binding.middle, got)
		}
	}

	if len(equation.Seeds) != 1 {
		t.Fatalf("root equation seeds = %d, want 1", len(equation.Seeds))
	}
	seed, err := algebra.instantiateConstant(equation.Seeds[0])
	if err != nil {
		t.Fatal(err)
	}
	var values, ordinary, coordinate int
	for _, groupRef := range root.groups {
		group := program.formalFibers.groups[groupRef.groupGlobal]
		equal, err := algebra.compareGroupRoots(span, authority, group, tuple, seed, decisionTrue, false)
		if err != nil || !equal {
			t.Fatalf("root group %d differs from its entry constant: equal=%t err=%v", group.global, equal, err)
		}
		switch group.kind {
		case formalFiberGroupValues:
			values++
		case formalFiberGroupOrdinaryLane:
			ordinary++
		case formalFiberGroupCoordinateLane:
			coordinate++
		}
	}
	if values != 1 || ordinary == 0 || coordinate == 0 || len(root.groups) != span.groupCount {
		t.Fatalf("root group census = Values:%d ordinary:%d coordinate:%d total:%d/%d", values, ordinary, coordinate, len(root.groups), span.groupCount)
	}
}

func TestFormalRootInputInstantiationRejectsForeignAndIncompleteTemplates(t *testing.T) {
	program, equation := formalRootInputInstantiationFixture(t)
	_, foreign := formalRootInputInstantiationFixture(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if tuple, err := algebra.instantiateRootEquation(foreign); err == nil || !tuple.bottom() {
		t.Fatalf("foreign root equation instantiated as %#v, %v", tuple, err)
	}
	broken := equation
	incomplete := *equation.Operator.rootInput
	incomplete.groups = incomplete.groups[:len(incomplete.groups)-1]
	broken.Operator.rootInput = &incomplete
	if tuple, err := algebra.instantiateRootEquation(broken); err == nil || !tuple.bottom() {
		t.Fatalf("incomplete root equation instantiated as %#v, %v", tuple, err)
	}
}

func TestFormalRootInputInstantiationIsRunLocal(t *testing.T) {
	program, equation := formalRootInputInstantiationFixture(t)
	first, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	left, err := first.instantiateRootEquation(equation)
	if err != nil {
		t.Fatal(err)
	}
	right, err := second.instantiateRootEquation(equation)
	if err != nil {
		t.Fatal(err)
	}
	if left.root.owner == right.root.owner {
		t.Fatal("separate root-input runs shared a directory")
	}
	if err := first.validateTuple(right); err == nil {
		t.Fatal("first algebra accepted the second root-input tuple")
	}
	if err := second.validateTuple(left); err == nil {
		t.Fatal("second algebra accepted the first root-input tuple")
	}
}

func formalRootInputInstantiationFixture(t *testing.T) (*RelationProgram, formalRelationEquation) {
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
	if !ok || equation.Operator.rootInput == nil {
		t.Fatal("root equation has no formal input operator")
	}
	return program, equation
}
