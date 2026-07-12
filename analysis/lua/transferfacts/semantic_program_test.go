package transferfacts

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	semanticprogram "github.com/wippyai/go-lua/analysis/semantic/program"
)

func TestAssembleSemanticProgramMapsRPOAndDerivesLoop(t *testing.T) {
	graph, body, header := loopProgramFixture()
	var digest semanticprogram.TransactionDigest
	digest[0] = 1
	var proof [32]byte
	proof[0] = 2
	got, topology, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{{
		ID: "worker", Graph: graph, WIR: body,
		Transactions: []PointTransactionRef{{Point: header, Ref: semanticprogram.TransactionRef{Digest: digest}}},
		Observations: []PointObservationSpec{{Point: header, Kind: semanticprogram.ObserveNode, Schema: "node.v1"}},
		Routes:       []PointRouteSpec{{Point: header, Known: []semanticprogram.KnownTarget{{Guard: "self", Member: "worker"}}, Residue: semanticprogram.TargetResidue{Native: true}, Completeness: semanticprogram.TargetsComplete, Proof: proof}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	rpo := graph.RPO()
	points := topology.Points()
	if len(points) != len(rpo) {
		t.Fatalf("topology points=%d, want %d", len(points), len(rpo))
	}
	for i, point := range points {
		if point.Point != rpo[i] || point.Block != semanticprogram.BlockID(i+1) || point.Member != "worker" {
			t.Fatalf("point[%d]=%#v, want RPO point %d/block %d", i, point, rpo[i], i+1)
		}
	}
	if got.Entry() != 1 || len(got.CallSCC().Members) != 1 || got.CallSCC().Members[0] != "worker" {
		t.Fatalf("entry/SCC = %d/%#v", got.Entry(), got.CallSCC())
	}
	loops := got.Loops()
	if len(loops) != 1 || loops[0].Owner != "worker" || len(loops[0].Blocks) != 2 {
		t.Fatalf("loops = %#v", loops)
	}
	var sawTrue, sawFalse, sawFlow bool
	for _, edge := range got.Edges() {
		switch edge.Guard {
		case "branch.true":
			sawTrue = true
		case "branch.false":
			sawFalse = true
		case "flow":
			sawFlow = true
		}
	}
	if !sawTrue || !sawFalse || !sawFlow {
		t.Fatalf("edge guards true=%v false=%v flow=%v", sawTrue, sawFalse, sawFlow)
	}
	if len(got.Transactions()) != 1 || len(got.Observations()) != 1 || len(got.Routes()) != 1 {
		t.Fatalf("attachments tx=%d obs=%d routes=%d", len(got.Transactions()), len(got.Observations()), len(got.Routes()))
	}
}

func TestAssembleSemanticProgramIsMemberOrderDeterministic(t *testing.T) {
	member := func(id semanticprogram.MemberID) ProgramMember {
		graph, body := linearProgramFixture(string(id))
		return ProgramMember{ID: id, Graph: graph, WIR: body}
	}
	left, leftMap, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{member("alpha"), member("beta")}})
	if err != nil {
		t.Fatal(err)
	}
	right, rightMap, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{member("beta"), member("alpha")}})
	if err != nil {
		t.Fatal(err)
	}
	if left.Digest() != right.Digest() || !slices.Equal(leftMap.Points(), rightMap.Points()) {
		t.Fatal("member input order changed neutral program")
	}
}

func TestAssembleSemanticProgramPreservesNestedWTOOwnership(t *testing.T) {
	graph := cfg.New()
	for graph.Size() < 6 {
		graph.AddNode(cfg.NodeNoop)
	}
	edges := map[cfg.Point][]cfg.Point{0: {1}, 1: {2, 5}, 2: {3, 4}, 3: {1}, 4: {2}}
	for from, successors := range edges {
		for _, to := range successors {
			graph.AddEdge(from, to, false)
		}
	}
	body := wirBodyForGraph("nested", graph)
	got, _, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{{ID: "nested", Graph: graph, WIR: body}}})
	if err != nil {
		t.Fatal(err)
	}
	loops := got.Loops()
	if len(loops) != 2 {
		t.Fatalf("loops=%#v, want nested pair", loops)
	}
	var child *semanticprogram.LoopMu
	for index := range loops {
		if loops[index].Parent != 0 {
			child = &loops[index]
		}
	}
	if child == nil {
		t.Fatalf("loops=%#v, want explicit Parent", loops)
	}
	var parent *semanticprogram.LoopMu
	for index := range loops {
		if loops[index].ID == child.Parent {
			parent = &loops[index]
		}
	}
	if parent == nil || child.Entry == parent.Entry || len(child.Blocks) >= len(parent.Blocks) {
		t.Fatalf("parent=%#v child=%#v", parent, child)
	}
}

func TestAssembleSemanticProgramRejectsIncompleteWIRAndPointAttachments(t *testing.T) {
	t.Run("missing WIR point window", func(t *testing.T) {
		graph, body := linearProgramFixture("missing")
		body = wir.NewBody("missing")
		body.AssignDebugPointOrdinals(graph)
		_, _, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{{ID: "missing", Graph: graph, WIR: body}}})
		assertTopologyError(t, err, "no point window")
	})
	t.Run("stale WIR debug order", func(t *testing.T) {
		graph, body := linearProgramFixture("stale")
		other := cfg.New()
		body.AssignDebugPointOrdinals(other)
		_, _, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{{ID: "stale", Graph: graph, WIR: body}}})
		assertTopologyError(t, err, "debug")
	})
	t.Run("uncovered transaction point", func(t *testing.T) {
		graph, body := linearProgramFixture("point")
		var digest semanticprogram.TransactionDigest
		digest[0] = 1
		_, _, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{{ID: "point", Graph: graph, WIR: body, Transactions: []PointTransactionRef{{Point: 99, Ref: semanticprogram.TransactionRef{Digest: digest}}}}}})
		assertTopologyError(t, err, "uncovered point")
	})
	t.Run("duplicate member", func(t *testing.T) {
		graph, body := linearProgramFixture("dup")
		member := ProgramMember{ID: "dup", Graph: graph, WIR: body}
		_, _, err := AssembleSemanticProgram(ProgramInput{Members: []ProgramMember{member, member}})
		assertTopologyError(t, err, "duplicate member")
	})
}

func linearProgramFixture(name string) (*cfg.CFG, *wir.Body) {
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	return graph, wirBodyForGraph(name, graph)
}

func loopProgramFixture() (*cfg.CFG, *wir.Body, cfg.Point) {
	graph := cfg.New()
	header := graph.AddNode(cfg.NodeBranch)
	bodyPoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), header, false)
	graph.AddEdge(header, bodyPoint, true)
	graph.AddEdge(header, graph.Exit(), false)
	graph.AddEdge(bodyPoint, header, false)
	return graph, wirBodyForGraph("loop", graph), header
}

func wirBodyForGraph(name string, graph cfg.Graph) *wir.Body {
	body := wir.NewBody(name)
	for _, point := range graph.RPO() {
		body.SetPointRange(point, body.Len(), body.Len())
	}
	body.AssignDebugPointOrdinals(graph)
	return body
}

func assertTopologyError(t *testing.T, err error, want string) {
	t.Helper()
	if !errors.Is(err, ErrSemanticProgramTopology) || !strings.Contains(err.Error(), want) {
		t.Fatalf("error=%v, want %q", err, want)
	}
}
