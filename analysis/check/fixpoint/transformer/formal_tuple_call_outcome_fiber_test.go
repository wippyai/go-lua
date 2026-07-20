package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func formalCallOutcomeFiberFixture(t *testing.T) (*RelationProgram, *formalTupleAlgebra, formalRelationCell, formalCallOutcomeFiber) {
	t.Helper()
	return formalCallOutcomeFiberFixtureWithProvider(t, callpayload.CallOutcomeProgram{})
}

func writeFormalCallOutcomeFiberTestValue(
	t *testing.T,
	algebra *formalTupleAlgebra,
	tuple formalRelationTuple,
	fiber formalCallOutcomeFiber,
	value callpayload.CallOutcomeAlternativeSet,
) formalRelationTuple {
	t.Helper()
	_, _, authority, ok := algebra.span(tuple.variable)
	if !ok {
		t.Fatal("formal call-outcome test tuple has no authority")
	}
	leaf, err := authority.internCallOutcomes(value)
	if err != nil {
		t.Fatal(err)
	}
	tuple, err = algebra.writeScalar(tuple, fiber.descriptor, algebra.decisions.terminal(leaf))
	if err != nil {
		t.Fatal(err)
	}
	return tuple
}

func formalCallOutcomeFiberFixtureWithProvider(t *testing.T, provider callpayload.CallOutcomeProgram) (*RelationProgram, *formalTupleAlgebra, formalRelationCell, formalCallOutcomeFiber) {
	t.Helper()
	const point cfg.Point = 1
	const readPoint cfg.Point = 0
	base := formalRootInputTestProgram(t, standard.Registry())
	priorPlan := base.bodies[0].plan
	globals, contracts := priorPlan.BoundaryGlobals(), priorPlan.BoundaryGlobalContracts()
	boundaryGlobals := make([]operationplan.BoundaryGlobal, len(globals))
	for index := range globals {
		boundaryGlobals[index] = operationplan.BoundaryGlobal{Symbol: globals[index], Contract: contracts[index]}
	}
	base.bodies[0].plan = operationplan.New(base.bodies[0].graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{point: factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextStatement, Point: point, HasPoint: true,
		})},
	}).WithBoundaryParams(priorPlan.BoundaryParams()).
		WithBoundaryParamContracts(priorPlan.BoundaryParamContracts()).
		WithBoundaryCaptures(priorPlan.BoundaryCaptures()).
		WithBoundaryGlobals(boundaryGlobals)
	base.bodies[0].relation.code.publication.points = []relationPointPublication{{point: readPoint, ref: 2}}
	base.bodies[0].nodeReads = make([][]cfg.Point, int(point)+1)
	base.bodies[0].nodeReads[point] = []cfg.Point{readPoint}
	base.bodies[0].externalCalls = provider
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepExternalCall, point: point}}, next: 2},
		{kind: relationNodeBottom},
	})
	algebra := formalTupleTestAlgebra(t, program)
	span, present := program.formalFibers.span(1)
	if !present {
		t.Fatal("formal call-outcome fixture has no body span")
	}
	fiber, present := span.callOutcomeFiber(point)
	if !present {
		t.Fatal("formal call-outcome fixture has no point fiber")
	}
	for _, equation := range program.formalTemplate.equations {
		step, exact := formalRelationStepOperator(equation.Operator)
		if exact && step.kind == boundaryStepExternalCall && step.point == point {
			return program, algebra, equation.Cell.cell, fiber
		}
	}
	t.Fatal("formal call-outcome fixture has no producer equation")
	return nil, nil, formalRelationCell{}, formalCallOutcomeFiber{}
}

func TestFormalExternalCallPreparesOnceAndPublishesOneAtomicProviderOutcome(t *testing.T) {
	prepares, evaluations := 0, 0
	provider := callpayload.SealCallOutcomeProgram(
		"formal ExternalCall executor test",
		[]string{"SuspensionKnown", "MaySuspend"},
		state.NewLaneSet(), state.NewLaneSet(),
		func(transfer.NodeContext, factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
			prepares++
			return callpayload.CallOutcomeSiteShape{FieldNames: []string{"SuspensionKnown", "MaySuspend"}}, nil
		},
		nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			evaluations++
			return callpayload.CallOutcome{SuspensionKnown: true, MaySuspend: true}, nil
		},
	)
	program, _, site, fiber := formalCallOutcomeFiberFixtureWithProvider(t, provider)
	if prepares != 1 {
		t.Fatalf("provider preparations after freeze = %d, want 1", prepares)
	}
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if evaluations != 1 {
		t.Fatalf("provider evaluations = %d, want 1", evaluations)
	}
	tuple := execution.values[site]
	diagnostics, reachable, err := execution.algebra.formalDiagnosticOutput(context.Background(), tuple)
	if err != nil || !reachable || !diagnostics.SuspensionKnown || !diagnostics.MaySuspend {
		t.Fatalf("provider diagnostics = %#v, reachable=%v, err=%v", diagnostics, reachable, err)
	}
	alternatives, err := execution.algebra.callOutcomeAlternatives(context.Background(), tuple, fiber)
	want := callpayload.NewCallOutcomeAlternativeSet(program.registry, callpayload.CallOutcome{
		SuspensionKnown: true, MaySuspend: true,
	})
	if err != nil || !alternatives.Equal(program.registry, want) {
		t.Fatalf("provider alternatives = %#v, want %#v, err=%v", alternatives, want, err)
	}
}

func TestFormalExternalCallComposesRawProviderResultsBeforeNormalization(t *testing.T) {
	first := callpayload.SealCallOutcomeProgram(
		"raw composition first", []string{"ParamObligations"}, state.NewLaneSet(), state.NewLaneSet(), nil, nil,
		func(ctx transfer.NodeContext, _ factflow.CallSiteView, _ callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			value := product.Absent(ctx.Registry)
			obligation := callpayload.CallParamObligation{ParamIndex: 0, Value: value}
			return callpayload.CallOutcome{ParamObligations: []callpayload.CallParamObligation{obligation, obligation}}, nil
		},
	)
	second := callpayload.SealCallOutcomeProgram(
		"raw composition second", []string{"SuspensionKnown", "MaySuspend"}, state.NewLaneSet(), state.NewLaneSet(), nil, nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			return callpayload.CallOutcome{}, nil
		},
	)
	provider := callpayload.ComposeCallOutcomePrograms(
		[]callpayload.CallOutcomeProgram{first, second},
		func(_ transfer.NodeContext, left, _ callpayload.CallOutcome) callpayload.CallOutcome {
			if len(left.ParamObligations) == 2 {
				left.SuspensionKnown = true
				left.MaySuspend = true
			}
			return left
		},
	)
	program, _, site, _ := formalCallOutcomeFiberFixtureWithProvider(t, provider)
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, reachable, err := execution.algebra.formalDiagnosticOutput(context.Background(), execution.values[site])
	if err != nil || !reachable || !diagnostics.SuspensionKnown || !diagnostics.MaySuspend {
		t.Fatalf("raw composition diagnostics = %#v reachable=%v err=%v", diagnostics, reachable, err)
	}
}

func TestFormalCallOutcomePublicationRejectsReachableCallWithoutProducer(t *testing.T) {
	provider := callpayload.SealCallOutcomeProgram(
		"formal call producer totality test",
		[]string{"SuspensionKnown"}, state.NewLaneSet(), state.NewLaneSet(),
		func(transfer.NodeContext, factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
			return callpayload.CallOutcomeSiteShape{FieldNames: []string{"SuspensionKnown"}}, nil
		},
		nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			return callpayload.CallOutcome{SuspensionKnown: true}, nil
		},
	)
	program, _, _, _ := formalCallOutcomeFiberFixtureWithProvider(t, provider)
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	view, err := execution.Publication(program.bodies[0].body)
	if err != nil {
		t.Fatal(err)
	}
	point := cfg.Point(1)
	if _, exact, err := view.callOutcomeFromPublishedOutput(context.Background(), point, state.State{}, true, true, nil); err == nil || exact {
		t.Fatalf("reachable producerless call = exact:%v err:%v, want fail closed", exact, err)
	}
	got, exact, err := view.callOutcomeFromPublishedOutput(context.Background(), point, state.State{}, false, false, nil)
	if err != nil || !exact || !got.Empty() {
		t.Fatalf("unreachable producerless call = %#v exact:%v err:%v, want explicit zero DTO", got, exact, err)
	}
}

func TestFormalCallOutcomeFiberDistinguishesAbsenceFromExecutedEmpty(t *testing.T) {
	program, algebra, site, fiber := formalCallOutcomeFiberFixture(t)
	tuple := formalTupleTestLive(t, algebra, 1)

	absent, err := algebra.callOutcomeAlternatives(context.Background(), tuple, fiber)
	if err != nil || !absent.Empty() {
		t.Fatalf("absent alternatives = %#v, %v", absent, err)
	}
	absentExecution := &formalRelationExecution{algebra: algebra, values: map[formalRelationCell]formalRelationTuple{site: tuple}}
	absentView, err := absentExecution.Publication(program.bodies[0].body)
	if err != nil {
		t.Fatal(err)
	}
	absentOutcome, declared, err := absentView.CallOutcome(context.Background(), fiber.descriptor.point)
	if err != nil || !declared || !absentOutcome.Empty() {
		t.Fatalf("totalized absent outcome = %#v, declared=%v, err=%v", absentOutcome, declared, err)
	}
	executed := callpayload.NewCallOutcomeAlternativeSet(program.registry, callpayload.CallOutcome{})
	tuple = writeFormalCallOutcomeFiberTestValue(t, algebra, tuple, fiber, executed)
	got, err := algebra.callOutcomeAlternatives(context.Background(), tuple, fiber)
	if err != nil || got.Empty() || !got.Equal(program.registry, executed) {
		t.Fatalf("executed-empty alternatives = %#v, %v", got, err)
	}

	execution := &formalRelationExecution{algebra: algebra, values: map[formalRelationCell]formalRelationTuple{site: tuple}}
	view, err := execution.Publication(program.bodies[0].body)
	if err != nil {
		t.Fatal(err)
	}
	outcome, present, err := view.CallOutcome(context.Background(), fiber.descriptor.point)
	if err != nil || !present || !outcome.Empty() {
		t.Fatalf("published executed-empty outcome = %#v, present=%v, err=%v", outcome, present, err)
	}
}

func TestFormalCallOutcomeFiberJoinsExactAlternativesBeforePublication(t *testing.T) {
	program, algebra, site, fiber := formalCallOutcomeFiberFixture(t)
	tuple := formalTupleTestLive(t, algebra, 1)
	first := callpayload.CallOutcome{SuspensionKnown: true}
	second := callpayload.CallOutcome{SuspensionKnown: true, MaySuspend: true}
	want := callpayload.NewCallOutcomeAlternativeSet(program.registry, first, second)
	tuple = writeFormalCallOutcomeFiberTestValue(t, algebra, tuple, fiber, want)
	got, err := algebra.callOutcomeAlternatives(context.Background(), tuple, fiber)
	if err != nil || !got.Equal(program.registry, want) {
		t.Fatalf("joined alternatives = %#v, want %#v, err=%v", got, want, err)
	}

	execution := &formalRelationExecution{algebra: algebra, values: map[formalRelationCell]formalRelationTuple{site: tuple}}
	view, err := execution.Publication(program.bodies[0].body)
	if err != nil {
		t.Fatal(err)
	}
	outcome, present, err := view.CallOutcome(context.Background(), fiber.descriptor.point)
	if err != nil || !present || !outcome.SuspensionKnown || !outcome.MaySuspend {
		t.Fatalf("collapsed outcome = %#v, present=%v, err=%v", outcome, present, err)
	}
}
