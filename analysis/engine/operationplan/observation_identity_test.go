package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestObservationOccurrencesUseCanonicalLoweringDebugCoordinates(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	body := wir.NewBody("test")
	body.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 1
	plan := New(graph, factflow.FactsInput{}).WithObservationIdentity(owner, body)
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
