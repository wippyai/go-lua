package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestInitialStatePlanOwnsCanonicalSparsePointSeeds(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	middle := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), middle, false)
	graph.AddEdge(middle, graph.Exit(), false)
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("initial-state-plan")))
	ordered := initialTestCoordinates(graph)
	entryValue := product.Top()
	middleValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	entryState := State{}.WriteValue(reg, key.Value(801), entryValue)
	middleState := State{}.WriteValue(reg, key.Value(802), middleValue)

	// Deliberately reverse construction order. Publication is graph-RPO stable.
	plan, err := NewInitialStatePlan(owner, graph.ID(), graph.Size(), ordered, []InitialStateSeed{
		NewInitialStateSeed(InitialCoordinate(middle), middleState),
		NewInitialStateSeed(InitialCoordinate(graph.Entry()), entryState),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ValidFor(owner, graph.ID(), graph.Size()) || plan.Empty() || plan.Len() != 2 {
		t.Fatalf("plan = valid:%t empty:%t len:%d", plan.ValidFor(owner, graph.ID(), graph.Size()), plan.Empty(), plan.Len())
	}
	firstPoint, first, ok := plan.Seed(0)
	if !ok || firstPoint != InitialCoordinate(graph.Entry()) || !product.Equal(reg, first.ReadValue(reg, key.Value(801)), entryValue) {
		t.Fatalf("first seed = %d/%t", firstPoint, ok)
	}
	gotMiddle, ok := plan.At(InitialCoordinate(middle))
	if !ok || !product.Equal(reg, gotMiddle.ReadValue(reg, key.Value(802)), middleValue) {
		t.Fatal("non-entry sparse seed was not retained")
	}
	if _, ok := plan.At(InitialCoordinate(graph.Exit())); ok {
		t.Fatal("unseeded point acquired an initial state")
	}
	clone := plan.Clone()
	if !clone.ValidFor(owner, graph.ID(), graph.Size()) || clone.Len() != plan.Len() {
		t.Fatal("cloned plan lost its exact owner")
	}

	foreign := cfg.New()
	foreignMiddle := foreign.AddNode(cfg.NodeAssign)
	foreign.AddEdge(foreign.Entry(), foreignMiddle, false)
	foreign.AddEdge(foreignMiddle, foreign.Exit(), false)
	if plan.ValidFor(owner, foreign.ID(), foreign.Size()) {
		t.Fatal("plan matched a different CFG with the same shape")
	}
	if plan.ValidFor(lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("initial-state-plan")), 1), graph.ID(), graph.Size()) {
		t.Fatal("plan matched a different lexical body")
	}
}

func TestInitialStatePlanRejectsDuplicateAndForeignPoints(t *testing.T) {
	graph := cfg.New()
	middle := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), middle, false)
	graph.AddEdge(middle, graph.Exit(), false)
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("initial-state-plan-invalid")))
	ordered := initialTestCoordinates(graph)
	seed := NewInitialStateSeed(InitialCoordinate(middle), State{})
	if _, err := NewInitialStatePlan(owner, graph.ID(), graph.Size(), ordered, []InitialStateSeed{seed, seed}); err == nil {
		t.Fatal("duplicate point seed was accepted")
	}
	if _, err := NewInitialStatePlan(owner, graph.ID(), graph.Size(), ordered, []InitialStateSeed{NewInitialStateSeed(InitialCoordinate(graph.Size()+10), State{})}); err == nil {
		t.Fatal("foreign point seed was accepted")
	}
	if (InitialStatePlan{}).Valid() {
		t.Fatal("zero plan is valid")
	}
	empty, err := NewInitialStatePlan(owner, graph.ID(), graph.Size(), ordered, nil)
	if err != nil || !empty.ValidFor(owner, graph.ID(), graph.Size()) || !empty.Empty() {
		t.Fatalf("prepared empty plan = %#v/%v", empty, err)
	}
}

func initialTestCoordinates(graph cfg.Graph) []InitialCoordinate {
	points := cfg.RPOReadOnly(graph)
	out := make([]InitialCoordinate, len(points))
	for index, point := range points {
		out[index] = InitialCoordinate(point)
	}
	return out
}
