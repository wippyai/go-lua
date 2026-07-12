package genericlift

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

type testLifter struct {
	lane      state.LaneID
	lateFail  bool
	allowKind string
}

func (l testLifter) Lane() state.LaneID { return l.lane }
func (l testLifter) Build(ctx BuildContext) (LaneProgram, Support) {
	for _, operation := range ctx.Operations {
		usesLane := false
		for _, lane := range operation.Lanes {
			usesLane = usesLane || lane == l.lane
		}
		if usesLane && operation.Kind != l.allowKind {
			return nil, Unsupported
		}
	}
	return testProgram(l), Exact
}

type testProgram testLifter

func (p testProgram) Lane() state.LaneID { return p.lane }
func (p testProgram) Instantiate(bindings Bindings, patch *Patch) Support {
	patch.Set(p.lane, bindings.CallID)
	if p.lateFail {
		return Unsupported
	}
	return Exact
}

func TestDefaultRegistryDerivesEveryStateLaneAndFailsClosed(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := state.DefaultLanes()
	if got := registry.Lanes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("registry lanes=%v want State catalog=%v", got, want)
	}
	transformer := registry.Build(BuildContext{})
	if !transformer.Contextual() || !reflect.DeepEqual(transformer.FallbackLanes(), want) {
		t.Fatalf("missing adapters did not fail closed: contextual=%v fallback=%v", transformer.Contextual(), transformer.FallbackLanes())
	}
}

func TestOneAdapterEnablesOneUsedLaneWithoutEngineSwitch(t *testing.T) {
	registry, err := DefaultRegistry(testLifter{lane: state.LaneValues, allowKind: "value-op"})
	if err != nil {
		t.Fatal(err)
	}
	transformer := registry.Build(BuildContext{Operations: []Operation{{Kind: "value-op", Lanes: []state.LaneID{state.LaneValues}}}}, state.LaneValues)
	if transformer.Contextual() {
		t.Fatalf("supported lane fell back: %v", transformer.FallbackLanes())
	}
	got, support := transformer.Instantiate(Bindings{CallID: "call:1"}, Patch{})
	if support != Exact {
		t.Fatal("supported transformer did not instantiate")
	}
	if value, ok := got.Value(state.LaneValues); !ok || value != "call:1" {
		t.Fatalf("instantiated patch=%v/%v", value, ok)
	}
}

func TestUnknownOperationAndNewLaneFailClosed(t *testing.T) {
	const future state.LaneID = "future-lane"
	registry, err := NewRegistry([]state.LaneID{state.LaneValues, future}, testLifter{lane: state.LaneValues, allowKind: "value-op"})
	if err != nil {
		t.Fatal(err)
	}
	unknownOp := registry.Build(BuildContext{Operations: []Operation{{Kind: "new-op", Lanes: []state.LaneID{state.LaneValues}}}}, state.LaneValues)
	if !unknownOp.Contextual() || !reflect.DeepEqual(unknownOp.FallbackLanes(), []state.LaneID{state.LaneValues}) {
		t.Fatalf("unknown operation did not fail closed: %v", unknownOp.FallbackLanes())
	}
	newLane := registry.Build(BuildContext{}, future)
	if !newLane.Contextual() || !reflect.DeepEqual(newLane.FallbackLanes(), []state.LaneID{future}) {
		t.Fatalf("new lane did not fail closed: %v", newLane.FallbackLanes())
	}
	hidden := registry.Build(BuildContext{Operations: []Operation{{Kind: "value-op", Lanes: []state.LaneID{state.LaneValues, future}}}}, state.LaneValues)
	if !hidden.Contextual() || !reflect.DeepEqual(hidden.FallbackLanes(), []state.LaneID{future}) {
		t.Fatalf("caller used mask hid operation lane: %v", hidden.FallbackLanes())
	}
}

func TestLateFailurePublishesNoPartialPatch(t *testing.T) {
	registry, err := NewRegistry(
		[]state.LaneID{"a", "b"},
		testLifter{lane: "a", allowKind: "op"},
		testLifter{lane: "b", allowKind: "op", lateFail: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	transformer := registry.Build(BuildContext{Operations: []Operation{{Kind: "op", Lanes: []state.LaneID{"a", "b"}}}})
	base := Patch{}
	base.Set("existing", "kept")
	got, support := transformer.Instantiate(Bindings{CallID: "must-not-publish"}, base)
	if support != Unsupported {
		t.Fatal("late failure reported exact")
	}
	if _, ok := got.Value("a"); ok {
		t.Fatal("partial lane output published")
	}
	if value, ok := got.Value("existing"); !ok || value != "kept" {
		t.Fatal("base patch changed on aborted instantiation")
	}
}

func TestRegistryRejectsDuplicateAndOrphanAdapters(t *testing.T) {
	if _, err := NewRegistry([]state.LaneID{"a"}, testLifter{lane: "a"}, testLifter{lane: "a"}); err == nil || !strings.Contains(err.Error(), "duplicate adapter") {
		t.Fatalf("duplicate adapter error=%v", err)
	}
	if _, err := NewRegistry([]state.LaneID{"a"}, testLifter{lane: "orphan"}); err == nil || !strings.Contains(err.Error(), "orphan adapter") {
		t.Fatalf("orphan adapter error=%v", err)
	}
}
