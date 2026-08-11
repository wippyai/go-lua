package topology

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// TestIncrementalTopologyBoundedExhaustiveDifferential is deliberately a
// black-box law.  The model in topology_model_test.go recomputes full SCCs
// and the residual DAG from the edge relation; it does not share Graph's
// condensation or insertion algorithm.  The bounded sequence depths cover
// every directed endpoint and both edge provenances for graphs through four
// nodes, while keeping the normal test gate practical.
func TestIncrementalTopologyBoundedExhaustiveDifferential(t *testing.T) {
	for _, test := range []struct {
		nodes, depth int
	}{
		{1, 5}, // repeated/mixed self provenance and rollback-sized prefixes
		{2, 4}, // every two-provenance directed sequence through four inserts
		{3, 3},
		{4, 3},
	} {
		t.Run(fmt.Sprintf("nodes=%d/depth=%d", test.nodes, test.depth), func(t *testing.T) {
			graph := appendNodes(t, test.nodes)
			exhaustTopology(t, graph, newModel(test.nodes), topologyActions(test.nodes), test.depth, nil)
		})
	}
}

func exhaustTopology(t *testing.T, graph *Graph, model modelGraph, actions []modelEdge, depth int, path []modelEdge) {
	t.Helper()
	assertTopologyMatchesModel(t, graph, model)
	if depth == 0 {
		return
	}
	for _, action := range actions {
		mark := graph.Mark()
		before := graph.ResidualSweep()
		want, accepted, changed := model.insert(action)
		got, err := graph.Insert(toEdge(action))
		if accepted {
			if err != nil {
				t.Fatalf("Insert(%s) after %s: %v, want accepted", modelEdgeString(action), modelPath(path), err)
			}
			if got.Inserted != changed {
				t.Fatalf("Insert(%s) after %s Inserted=%t, want %t", modelEdgeString(action), modelPath(path), got.Inserted, changed)
			}
			if changed && before.Len() != 0 {
				t.Fatalf("successful mutation after %s left prior Sweep live", modelPath(path))
			}
			assertTopologyMatchesModel(t, graph, want)
			exhaustTopology(t, graph, want, actions, depth-1, append(path, action))
		} else {
			if !errors.Is(err, ErrResidualCycle) {
				t.Fatalf("Insert(%s) after %s: %v, want ErrResidualCycle", modelEdgeString(action), modelPath(path), err)
			}
			if got.Inserted || before.Len() != graph.NodeCount() {
				t.Fatalf("rejected Insert(%s) after %s mutated observable topology", modelEdgeString(action), modelPath(path))
			}
			assertTopologyMatchesModel(t, graph, model)
		}
		if !graph.Rewind(mark) {
			t.Fatalf("Rewind after %s", modelPath(append(path, action)))
		}
		assertTopologyMatchesModel(t, graph, model)
	}
}

func TestIncrementalTopologyRejectsMalformedEdgesAtomically(t *testing.T) {
	graph := appendNodes(t, 2)
	baseline := newModel(2)
	for _, edge := range []Edge{
		{From: -1, To: 0, Ordinary: true},
		{From: 0, To: -1, Boundary: true},
		{From: 2, To: 0, Ordinary: true},
		{From: 0, To: 2, Boundary: true},
		{From: 0, To: 1},
	} {
		change, err := graph.Insert(edge)
		if !errors.Is(err, ErrInvalidEdge) || change.Inserted {
			t.Fatalf("Insert(%#v) = %#v, %v; want invalid edge without mutation", edge, change, err)
		}
		assertTopologyMatchesModel(t, graph, baseline)
	}
}

func TestIncrementalTopologyRollbackRestoresMergedSCCAndEdgeProvenance(t *testing.T) {
	graph := appendNodes(t, 4)
	model := newModel(4)
	for _, edge := range []modelEdge{
		{from: 0, to: 1, boundary: true},
		{from: 1, to: 0, boundary: true},
		{from: 2, to: 3, ordinary: true},
	} {
		model = insertAccepted(t, graph, model, edge)
	}
	mark := graph.Mark()
	before := model
	for _, edge := range []modelEdge{
		{from: 1, to: 2, boundary: true},
		{from: 3, to: 0, boundary: true}, // coalesces every full SCC
		{from: 0, to: 2, ordinary: true}, // residual remains a DAG
	} {
		model = insertAccepted(t, graph, model, edge)
	}
	if members := graph.Members(0); !reflect.DeepEqual(members, []Node{0, 1, 2, 3}) {
		t.Fatalf("merged Members(0) = %v, want all nodes", members)
	}
	if !graph.Rewind(mark) {
		t.Fatal("Rewind merged suffix failed")
	}
	assertTopologyMatchesModel(t, graph, before)
	if _, ok := graph.Edge(3, 0); ok {
		t.Fatal("Rewind retained boundary edge from rolled-back merge")
	}
	if _, ok := graph.Edge(0, 2); ok {
		t.Fatal("Rewind retained ordinary edge from rolled-back suffix")
	}
}

func TestIncrementalTopologyIsDeterministicAcrossProvenanceInsertionPermutations(t *testing.T) {
	edges := []modelEdge{
		{from: 0, to: 1, ordinary: true},
		{from: 1, to: 2, ordinary: true},
		{from: 2, to: 3, ordinary: true},
		{from: 3, to: 0, boundary: true},
		{from: 0, to: 2, boundary: true},
		{from: 0, to: 1, boundary: true}, // mixed provenance on one relation
	}
	permutations := [][]modelEdge{
		edges,
		{edges[5], edges[4], edges[3], edges[2], edges[1], edges[0]},
		{edges[2], edges[0], edges[4], edges[1], edges[5], edges[3]},
	}
	var want topologyObservation
	for index, sequence := range permutations {
		graph := appendNodes(t, 4)
		model := newModel(4)
		for _, edge := range sequence {
			model = insertAccepted(t, graph, model, edge)
		}
		assertTopologyMatchesModel(t, graph, model)
		got := observeTopology(t, graph)
		if index == 0 {
			want = got
		} else if !reflect.DeepEqual(got, want) {
			t.Fatalf("insertion permutation %d changed topology:\n got %#v\nwant %#v", index, got, want)
		}
	}
}

func insertAccepted(t *testing.T, graph *Graph, model modelGraph, edge modelEdge) modelGraph {
	t.Helper()
	want, accepted, _ := model.insert(edge)
	if !accepted {
		t.Fatalf("model setup edge unexpectedly closes residual cycle: %s", modelEdgeString(edge))
	}
	if _, err := graph.Insert(toEdge(edge)); err != nil {
		t.Fatalf("Insert(%s): %v", modelEdgeString(edge), err)
	}
	assertTopologyMatchesModel(t, graph, want)
	return want
}

type topologyObservation struct {
	edges      []Edge
	components [][]Node
	cyclic     []bool
	sweep      []Node
}

func observeTopology(t *testing.T, graph *Graph) topologyObservation {
	t.Helper()
	result := topologyObservation{}
	for from := 0; from < graph.NodeCount(); from++ {
		for to := 0; to < graph.NodeCount(); to++ {
			edge, ok := graph.Edge(Node(from), Node(to))
			if ok {
				result.edges = append(result.edges, edge)
			}
		}
	}
	for node := 0; node < graph.NodeCount(); node++ {
		_, cyclic, ok := graph.Component(Node(node))
		if !ok {
			t.Fatalf("Component(%d) absent", node)
		}
		result.components = append(result.components, graph.Members(Node(node)))
		result.cyclic = append(result.cyclic, cyclic)
	}
	result.sweep = sweepNodes(t, graph.ResidualSweep())
	return result
}

func assertTopologyMatchesModel(t *testing.T, graph *Graph, model modelGraph) {
	t.Helper()
	if got := graph.NodeCount(); got != model.nodes {
		t.Fatalf("NodeCount = %d, want %d", got, model.nodes)
	}
	for from := 0; from < model.nodes; from++ {
		for to := 0; to < model.nodes; to++ {
			want, exists := model.edges[[2]int{from, to}]
			got, ok := graph.Edge(Node(from), Node(to))
			if ok != exists {
				t.Fatalf("Edge(%d,%d) present = %t, want %t", from, to, ok, exists)
			}
			if ok && (got.From != Node(from) || got.To != Node(to) || got.Ordinary != want.ordinary || got.Boundary != want.boundary) {
				t.Fatalf("Edge(%d,%d) = %#v, want %#v", from, to, got, toEdge(want))
			}
		}
	}

	components := model.components()
	for node := 0; node < model.nodes; node++ {
		wantMembers := componentFor(components, node)
		wantCyclic := len(wantMembers) > 1 || model.edges[[2]int{node, node}].ordinary || model.edges[[2]int{node, node}].boundary
		head, cyclic, ok := graph.Component(Node(node))
		if !ok || head != Node(wantMembers[0]) || cyclic != wantCyclic {
			t.Fatalf("Component(%d) = (%d,%t,%t), want (%d,%t,true)", node, head, cyclic, ok, wantMembers[0], wantCyclic)
		}
		gotMembers := graph.Members(Node(node))
		wantNodes := nodes(wantMembers)
		if !reflect.DeepEqual(gotMembers, wantNodes) {
			t.Fatalf("Members(%d) = %v, want %v", node, gotMembers, wantNodes)
		}
	}

	wantSweep, acyclic := model.residualSweep()
	if !acyclic {
		t.Fatal("test model admitted residual cycle")
	}
	gotSweep := sweepNodes(t, graph.ResidualSweep())
	if len(gotSweep) != model.nodes {
		t.Fatalf("ResidualSweep length = %d, want %d", len(gotSweep), model.nodes)
	}
	seen := make([]bool, model.nodes)
	position := make([]int, model.nodes)
	for index, node := range gotSweep {
		if node < 0 || int(node) >= model.nodes || seen[node] {
			t.Fatalf("ResidualSweep is not a node permutation: %v", gotSweep)
		}
		seen[node], position[node] = true, index
	}
	for _, edge := range model.edges {
		if edge.ordinary && position[edge.from] >= position[edge.to] {
			t.Fatalf("ResidualSweep %v violates ordinary edge %s", gotSweep, modelEdgeString(edge))
		}
	}
	// The model's order is used only to establish that a complete residual
	// order exists; Graph's public determinism law is tested across insertion
	// permutations above, not by baking in an arbitrary tie-breaker here.
	if len(wantSweep) != len(gotSweep) {
		t.Fatalf("model residual order length = %d, got %d", len(wantSweep), len(gotSweep))
	}
}

func appendNodes(t *testing.T, count int) *Graph {
	t.Helper()
	graph := New()
	for want := 0; want < count; want++ {
		got, ok := graph.Append()
		if !ok || got != Node(want) {
			t.Fatalf("Append %d = %d/%t", want, got, ok)
		}
	}
	return graph
}

func topologyActions(nodes int) []modelEdge {
	result := make([]modelEdge, 0, 2*nodes*nodes)
	for from := 0; from < nodes; from++ {
		for to := 0; to < nodes; to++ {
			result = append(result, modelEdge{from: from, to: to, ordinary: true})
			result = append(result, modelEdge{from: from, to: to, boundary: true})
		}
	}
	return result
}

func componentFor(components [][]int, node int) []int {
	for _, component := range components {
		for _, member := range component {
			if member == node {
				return component
			}
		}
	}
	panic("model component missing node")
}

func sweepNodes(t *testing.T, sweep Sweep) []Node {
	t.Helper()
	result := make([]Node, 0, sweep.Len())
	for index := 0; index < sweep.Len(); index++ {
		node, ok := sweep.At(index)
		if !ok {
			t.Fatalf("Sweep.At(%d) absent before mutation", index)
		}
		result = append(result, node)
	}
	if _, ok := sweep.At(sweep.Len()); ok {
		t.Fatal("Sweep.At(end) accepted out-of-range index")
	}
	return result
}

func toEdge(edge modelEdge) Edge {
	return Edge{From: Node(edge.from), To: Node(edge.to), Ordinary: edge.ordinary, Boundary: edge.boundary}
}

func nodes(source []int) []Node {
	result := make([]Node, len(source))
	for index, node := range source {
		result[index] = Node(node)
	}
	return result
}

func modelEdgeString(edge modelEdge) string {
	return fmt.Sprintf("%d→%d[o=%t,b=%t]", edge.from, edge.to, edge.ordinary, edge.boundary)
}

func modelPath(path []modelEdge) string {
	if len(path) == 0 {
		return "∅"
	}
	return fmt.Sprint(path)
}
