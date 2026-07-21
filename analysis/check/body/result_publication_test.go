package body

import (
	"context"
	"errors"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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

func TestPublishN5NormalExitJoinsCompleteNormalTerminals(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	returned := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), returned, false)
	graph.AddEdge(graph.Entry(), graph.Exit(), false)

	returnID := identity.LuaTableLiteralAtSite("normal-exit-test", 1)
	fallthroughID := identity.LuaTableLiteralAtSite("normal-exit-test", 2)
	returnValue := identityvalue.Present(reg, returnID)
	fallthroughValue := identityvalue.Present(reg, fallthroughID)
	returnObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: returnValue})
	fallthroughObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: fallthroughValue})
	base := state.NormalizeForDomain(state.Domain(reg), state.Reachable(state.State{}))
	fallthroughState := base.WriteReturnSlot(reg, 7, typevalue.LiteralString(reg, "transient-call-result")).WriteHeapTableObject(reg, fallthroughID, fallthroughObject)
	terminal := base.WriteReturnSlot(reg, 0, returnValue).WriteReturnSlot(reg, 7, typevalue.LiteralString(reg, "transient-call-result")).WriteHeapTableObject(reg, returnID, returnObject)
	result := &Result{
		registry: reg,
		cfg:      &cfgbuild.Result{Graph: graph},
		facts: factflow.NewFacts(factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{
			returned: factflow.NewReturn([]factflow.ValueSource{{TargetIndex: 0}}),
		}}),
	}
	flow := transfer.Result{graph.Exit(): fallthroughState}
	coordinates := StabilizedResultCoordinates{
		PointReachable:      map[cfg.Point]bool{graph.Exit(): true},
		PlannedNodeOutputs:  map[cfg.Point]state.State{returned: terminal},
		NodeOutputReachable: map[cfg.Point]bool{returned: true},
	}
	if err := result.publishN5NormalExit(flow, coordinates); err != nil {
		t.Fatal(err)
	}

	wantFallthrough := fallthroughState.WriteReturnSlot(reg, 7, product.Bottom(reg))
	wantTerminal := terminal.WriteReturnSlot(reg, 7, product.Bottom(reg))
	want := state.RegisteredProductDomain(reg).Lattice().Join(wantFallthrough, wantTerminal)
	got := flow[graph.Exit()]
	if !state.RegisteredProductDomain(reg).Lattice().Equal(got, want) {
		t.Fatalf("public normal exit differs from complete terminal join\ngot:  %#v\nwant: %#v", got, want)
	}
	if transient := got.ReadReturnSlot(reg, 7); !product.Equal(reg, transient, product.Bottom(reg)) {
		t.Fatalf("public normal exit retained transient ReturnSlot(7): %#v", transient)
	}
	if returnedValue := got.ReadReturnSlot(reg, 0); !product.Equal(reg, returnedValue, returnValue) {
		t.Fatalf("public normal exit ReturnSlot(0) = %#v, want %#v", returnedValue, returnValue)
	}
	if object := got.ReadHeapTableObject(reg, returnID); heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) || !heapidentity.ObjectDomain(reg).Equal(object, returnObject) {
		t.Fatalf("public normal exit heap object = %#v, want returned non-bottom object %#v", object, returnObject)
	}
}

func TestPublishN5NormalExitJoinsDifferentReturnedTables(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeReturn)
	second := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), first, false)
	graph.AddEdge(graph.Entry(), second, false)

	firstID := identity.LuaTableLiteralAtSite("normal-exit-test", 11)
	secondID := identity.LuaTableLiteralAtSite("normal-exit-test", 12)
	firstValue := identityvalue.Present(reg, firstID)
	secondValue := identityvalue.Present(reg, secondID)
	firstObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: firstValue})
	secondObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: secondValue})
	base := state.NormalizeForDomain(state.Domain(reg), state.Reachable(state.State{}))
	firstTerminal := base.WriteReturnSlot(reg, 0, firstValue).WriteHeapTableObject(reg, firstID, firstObject)
	secondTerminal := base.WriteReturnSlot(reg, 0, secondValue).WriteHeapTableObject(reg, secondID, secondObject)
	result := &Result{
		registry: reg,
		cfg:      &cfgbuild.Result{Graph: graph},
		facts: factflow.NewFacts(factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{
			first:  factflow.NewReturn([]factflow.ValueSource{{TargetIndex: 0}}),
			second: factflow.NewReturn([]factflow.ValueSource{{TargetIndex: 0}}),
		}}),
	}
	flow := transfer.Result{}
	coordinates := StabilizedResultCoordinates{
		PointReachable: map[cfg.Point]bool{graph.Exit(): false},
		PlannedNodeOutputs: map[cfg.Point]state.State{
			first: firstTerminal, second: secondTerminal,
		},
		NodeOutputReachable: map[cfg.Point]bool{first: true, second: true},
	}
	if err := result.publishN5NormalExit(flow, coordinates); err != nil {
		t.Fatal(err)
	}

	want := state.RegisteredProductDomain(reg).Lattice().Join(firstTerminal, secondTerminal)
	got := flow[graph.Exit()]
	if !state.RegisteredProductDomain(reg).Lattice().Equal(got, want) {
		t.Fatalf("public normal exit differs from two-return join\ngot:  %#v\nwant: %#v", got, want)
	}
	for _, item := range []struct {
		id     identity.ID
		object heapidentity.TableObject
	}{{firstID, firstObject}, {secondID, secondObject}} {
		if gotObject := got.ReadHeapTableObject(reg, item.id); !heapidentity.ObjectDomain(reg).Equal(gotObject, item.object) {
			t.Fatalf("public normal exit heap object %s = %#v, want %#v", item.id, gotObject, item.object)
		}
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
