package operationplan

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type observationDebugOrderGraph struct {
	cfg.Graph
	order []cfg.Point
}

func (g observationDebugOrderGraph) RPO() []cfg.Point {
	return append([]cfg.Point(nil), g.order...)
}

func TestObservationOccurrencesUseCanonicalLoweringDebugCoordinates(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	body := wir.NewBody("test")
	body.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 1
	plan := New(graph, factflow.FactsInput{}).WithObservationIdentity(owner, body, graph)
	assignment, ok := plan.AssignmentObservationAnchor(call)
	if !ok {
		t.Fatal("assignment occurrence missing")
	}
	wantAfter, _ := body.DebugPointID(call, wir.DebugPhaseAfter)
	if assignment.Point != wantAfter {
		t.Fatalf("assignment debug point = %v, want %v", assignment.Point, wantAfter)
	}
	if _, ok := plan.CallInvocationObservationAnchor(call); ok {
		t.Fatal("non-emitted call phase became an observation occurrence")
	}
	if plan.ObservationBody() != owner {
		t.Fatal("stable owner lost")
	}
}

func TestObservationIdentityAllowsUnreachableDenseCFGSlots(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	unreachable := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	body := wir.NewBody("unreachable-dense-slot")
	start := body.Len()
	body.Emit(wir.Instruction{Op: wir.OpCall, Point: call})
	body.SetPointRange(call, start, body.Len())
	body.AssignDebugPointOrdinals(graph)

	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 1
	surface, err := SealCallSurface(owner, graph.Size(), []cfg.Point{call}, []CallSurfaceSite{{
		Point: call, Target: RejectedCallSurfaceTarget(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	plan := New(graph, factflow.FactsInput{}).
		WithObservationIdentity(owner, body, graph).
		WithCallSurface(surface)
	if plan.ObservationBody() != owner {
		t.Fatal("unreachable dense slot cleared the reachable observation identity")
	}
	if _, ok := plan.AssignmentObservationAnchor(call); !ok {
		t.Fatal("reachable assignment observation anchor missing")
	}
	if _, ok := plan.CallInvocationObservationAnchor(call); !ok {
		t.Fatal("reachable call observation anchor missing")
	}
	if _, ok := plan.AssignmentObservationAnchor(unreachable); ok {
		t.Fatal("unreachable dense slot acquired an observation anchor")
	}
	if _, ok := plan.CallInvocationObservationAnchor(unreachable); ok {
		t.Fatal("unreachable dense slot acquired a call observation anchor")
	}
	got, ok := plan.CallSurface()
	if !ok || !got.Complete() || got.Digest() != surface.Digest() {
		t.Fatalf("reachable call surface = %#v/%v, want complete", got, ok)
	}
}

func TestObservationIdentityRejectsMalformedLoweringDebugTraversal(t *testing.T) {
	graph := cfg.New()
	middle := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), middle, false)
	graph.AddEdge(middle, graph.Exit(), false)
	want := graph.RPO()

	tests := []struct {
		name               string
		order              []cfg.Point
		bindMalformedGraph bool
	}{
		{name: "missing reachable point", order: []cfg.Point{graph.Entry(), graph.Exit()}},
		{name: "duplicate point", order: append(append([]cfg.Point(nil), want...), middle), bindMalformedGraph: true},
		{name: "out of range point", order: append(append([]cfg.Point(nil), want...), cfg.Point(graph.Size())), bindMalformedGraph: true},
		{name: "noncanonical order", order: func() []cfg.Point {
			out := append([]cfg.Point(nil), want...)
			slices.Reverse(out)
			return out
		}()},
	}
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 1
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loweringGraph := observationDebugOrderGraph{Graph: graph, order: test.order}
			body := wir.NewBody(test.name)
			body.AssignDebugPointOrdinals(loweringGraph)
			var identityGraph cfg.Graph = graph
			if test.bindMalformedGraph {
				identityGraph = loweringGraph
			}
			plan := New(identityGraph, factflow.FactsInput{}).WithObservationIdentity(owner, body, identityGraph)
			if plan.ObservationBody() != (lexicalidentity.StableLexicalBodyID{}) {
				t.Fatal("malformed lowering debug traversal published an observation identity")
			}
			if _, ok := plan.AssignmentObservationAnchor(middle); ok {
				t.Fatal("malformed lowering debug traversal published an observation anchor")
			}
		})
	}
}
