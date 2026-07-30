package body

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestExecutionFactoryPublishesWithoutConstructingConcreteRoute(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f() return "done" end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	entry, initial := factory.SeedEntry(state.State{}, nil)
	coordinates := reachableFactoryPublicationCoordinates(factory)

	result, err := factory.PublishResult(ResultPublicationConfig{
		Coordinates: coordinates,
		Solve:       SolveConfig{Context: ctx},
		SeededEntry: entry,
		Initial:     initial,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("PublishResult returned a nil exact readmodel")
	}
	// Result has no transfer/provider field that a readmodel query could replay.
}

func TestPublishResultOwnsDetachedDiagnosticOutput(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareFunction(parseFunction(t, `function f(x) return x end`), Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	entry, initial := factory.SeedEntry(state.State{}, nil)
	coordinates := reachableFactoryPublicationCoordinates(factory)
	coordinates.DiagnosticOutput = callpayload.DiagnosticOutput{
		SuspensionKnown: true,
		PathObligations: []callpayload.CallPathObligation{{
			Path:  pathdom.NewPlaceholder(0).Field("before"),
			Value: typevalue.String(reg),
		}},
	}

	result, err := factory.PublishResult(ResultPublicationConfig{
		Coordinates: coordinates,
		Solve:       SolveConfig{Context: ctx},
		SeededEntry: entry,
		Initial:     initial,
	})
	if err != nil {
		t.Fatal(err)
	}
	coordinates.DiagnosticOutput.PathObligations[0].Path.Segments[0].Name = "source-mutated"
	got := result.DiagnosticOutput()
	if len(got.PathObligations) != 1 || got.PathObligations[0].Path.Segments[0].Name != "before" {
		t.Fatalf("published diagnostic output = %#v", got)
	}
	got.PathObligations[0].Path.Segments[0].Name = "consumer-mutated"
	again := result.DiagnosticOutput()
	if again.PathObligations[0].Path.Segments[0].Name != "before" {
		t.Fatalf("second diagnostic output = %#v", again)
	}
}

func TestPublishResultOwnsExactDetachedCallOutcomesWithoutProviderReplay(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f(g) return g() end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	entry, initial := factory.SeedEntry(state.State{}, nil)
	coordinates := reachableFactoryPublicationCoordinates(factory)
	callPoint := publicationCallPoint(t, prepared)
	coordinates.CallOutcomes = map[cfg.Point]callpayload.CallOutcome{
		callPoint: {
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0}},
			PathObligations: []callpayload.CallPathObligation{{
				Path: pathdom.NewPlaceholder(0).Field("before"),
			}},
		},
	}

	result, err := factory.PublishResult(ResultPublicationConfig{
		Coordinates: coordinates,
		Solve:       SolveConfig{Context: ctx},
		SeededEntry: entry,
		Initial:     initial,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := coordinates.CallOutcomes[callPoint]
	source.Results[0].Index = 9
	source.PathObligations[0].Path.Segments[0].Name = "source-mutated"

	got, ok := result.CallOutcomeAt(callPoint)
	if !ok || !got.PostReturnAuthority || got.Results[0].Index != 0 || got.PathObligations[0].Path.Segments[0].Name != "before" {
		t.Fatalf("published outcome = %#v/%v", got, ok)
	}
	got.Results[0].Index = 7
	got.PathObligations[0].Path.Segments[0].Name = "consumer-mutated"
	again, ok := result.CallOutcomeAt(callPoint)
	if !ok || again.Results[0].Index != 0 || again.PathObligations[0].Path.Segments[0].Name != "before" {
		t.Fatalf("second published outcome = %#v/%v", again, ok)
	}
	if _, ok := result.CallOutcomeAt(factory.Graph().Entry()); ok {
		t.Fatal("CallOutcomeAt invented an outcome for a non-call point")
	}
}

func TestPublishResultDistinguishesExecutedEmptyCallFromAbsentPoint(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f(g) g() end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	entry, initial := factory.SeedEntry(state.State{}, nil)
	coordinates := reachableFactoryPublicationCoordinates(factory)
	callPoint := publicationCallPoint(t, prepared)
	coordinates.CallOutcomes = map[cfg.Point]callpayload.CallOutcome{callPoint: {}}

	result, err := factory.PublishResult(ResultPublicationConfig{
		Coordinates: coordinates,
		Solve:       SolveConfig{Context: ctx},
		SeededEntry: entry,
		Initial:     initial,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome, ok := result.CallOutcomeAt(callPoint); !ok || !outcome.Empty() {
		t.Fatalf("executed-empty call outcome = %#v/%v, want explicit empty entry", outcome, ok)
	}
	if _, ok := result.CallOutcomeAt(factory.Graph().Entry()); ok {
		t.Fatal("non-call point acquired a CallOutcome entry")
	}
}

func TestPublishResultRejectsMissingExactCallOutcome(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f(g) return g() end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	entry, initial := factory.SeedEntry(state.State{}, nil)
	result, err := factory.PublishResult(ResultPublicationConfig{
		Coordinates: reachableFactoryPublicationCoordinates(factory),
		Solve:       SolveConfig{Context: ctx},
		SeededEntry: entry,
		Initial:     initial,
	})
	if err == nil || !strings.Contains(err.Error(), "no exact call outcome") {
		t.Fatalf("PublishResult error = %v, want missing exact call outcome", err)
	}
	if result != nil {
		t.Fatal("failed exact publication exposed a Result")
	}
}

func TestPublishResultCancellationExposesNoCallOutcome(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f(g) return g() end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	factoryContext, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: factoryContext, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	entry, initial := factory.SeedEntry(state.State{}, nil)
	coordinates := reachableFactoryPublicationCoordinates(factory)
	coordinates.CallOutcomes = map[cfg.Point]callpayload.CallOutcome{
		publicationCallPoint(t, prepared): {PostReturnAuthority: true},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := factory.PublishResult(ResultPublicationConfig{
		Coordinates: coordinates,
		Solve:       SolveConfig{Context: canceled},
		SeededEntry: entry,
		Initial:     initial,
	})
	if !errors.Is(err, solve.ErrCanceled) && !errors.Is(err, context.Canceled) {
		t.Fatalf("PublishResult error = %v, want cancellation", err)
	}
	if result != nil {
		t.Fatal("canceled exact publication exposed a Result")
	}
}

func reachableFactoryPublicationCoordinates(factory *ExecutionFactory) StabilizedResultCoordinates {
	return reachablePublicationCoordinatesForPlan(factory, compileObservationPlan(factory.Graph(), factory.prepared.facts))
}

func reachablePublicationCoordinatesForPlan(factory *ExecutionFactory, plan ObservationPlan) StabilizedResultCoordinates {
	domain, graph := factory.Domain(), factory.Graph()
	reachable := state.NormalizeForDomain(domain, state.Reachable(state.State{}))
	coordinates := StabilizedResultCoordinates{
		PointInputs:         make(transfer.Result, graph.Size()),
		PlannedNodeOutputs:  make(map[cfg.Point]state.State),
		PointReachable:      make(map[cfg.Point]bool, graph.Size()),
		NodeOutputReachable: make(map[cfg.Point]bool),
		EdgeNormal:          make(map[ResultEdge]bool),
	}
	for _, point := range cfg.RPOReadOnly(graph) {
		coordinates.PointInputs[point] = reachable
		coordinates.PointReachable[point] = true
	}
	for _, point := range plan.NodePoints() {
		coordinates.PlannedNodeOutputs[point] = reachable
		coordinates.NodeOutputReachable[point] = true
	}
	for _, edge := range plan.Edges() {
		coordinates.EdgeNormal[edge] = true
	}
	return coordinates
}

func publicationCallPoint(t *testing.T, prepared *Static) cfg.Point {
	t.Helper()
	for _, point := range cfg.RPOReadOnly(prepared.Graph()) {
		if _, ok := prepared.facts.CallSiteView(point); ok {
			return point
		}
	}
	t.Fatal("prepared function has no call point")
	return 0
}
