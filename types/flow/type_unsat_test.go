package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func litretInputs(g *mockSSAGraph, sym cfg.SymbolID, base typ.Type, cond constraint.Condition, branch, thenNode cfg.Point) *Inputs {
	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = base
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: cond},
	}
	return inputs
}

func TestFlow_TypeTheoryLiteralContradiction(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}
	g.params = []cfg.SymbolID{symX}

	httpLit := typ.LiteralString("http")
	jsonLit := typ.LiteralString("json")
	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs := litretInputs(g, symX, httpLit,
		constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.HashTypeKey(jsonLit.Hash())}),
		branch, thenNode)
	inputs.TypeKeys[jsonLit.Hash()] = jsonLit

	s := Solve(inputs, testResolver())
	if !s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("expected edge requiring \"json\" of \"http\" to be unreachable")
	}
}

func TestFlow_TypeTheoryLiteralSatisfiable(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}
	g.params = []cfg.SymbolID{symX}

	httpLit := typ.LiteralString("http")
	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs := litretInputs(g, symX, httpLit,
		constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.HashTypeKey(httpLit.Hash())}),
		branch, thenNode)
	inputs.TypeKeys[httpLit.Hash()] = httpLit

	s := Solve(inputs, testResolver())
	if s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("edge requiring \"http\" of \"http\" should stay reachable")
	}
}

func TestFlow_TypeTheoryNotHasTypeDeadJoin(t *testing.T) {
	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}
	g.params = []cfg.SymbolID{symX}

	httpLit := typ.LiteralString("http")
	jsonLit := typ.LiteralString("json")
	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = httpLit
	inputs.TypeKeys[httpLit.Hash()] = httpLit
	inputs.TypeKeys[jsonLit.Hash()] = jsonLit
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.HashTypeKey(jsonLit.Hash())})},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(constraint.NotHasType{Path: pathX, Type: narrow.HashTypeKey(httpLit.Hash())})},
	}

	s := Solve(inputs, testResolver())
	if !s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("expected then edge to be unreachable")
	}
	if !s.IsEdgeUnreachable(branch, elseNode) {
		t.Error("expected else edge to be unreachable")
	}
	if !s.IsPointDead(join) {
		t.Error("expected join of two dead branches to be dead")
	}
}

func TestFlow_TypeTheoryCompanionRefinesPath(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}
	g.params = []cfg.SymbolID{symX}

	pathX := constraint.Path{Root: "x", Symbol: symX}
	fieldPath := constraint.Path{Root: "x", Symbol: symX, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "f"}}}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.Nil
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(
			constraint.Truthy{Path: fieldPath},
			constraint.NotNil{Path: pathX},
		)},
	}

	s := Solve(inputs, testResolver())
	if s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("companion field constraint can refine the path, edge should stay reachable")
	}
}

func TestFlow_TypeTheoryNonParamStaysReachable(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	jsonLit := typ.LiteralString("json")
	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs := litretInputs(g, symX, typ.Nil,
		constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.HashTypeKey(jsonLit.Hash())}),
		branch, thenNode)
	inputs.TypeKeys[jsonLit.Hash()] = jsonLit

	s := Solve(inputs, testResolver())
	if s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("non-parameter locals can be assigned before the edge, edge should stay reachable")
	}
}

func TestFlow_TypeTheoryReassignedParamStaysReachable(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}
	g.params = []cfg.SymbolID{symX}

	httpLit := typ.LiteralString("http")
	jsonLit := typ.LiteralString("json")
	pathX := constraint.Path{Root: "x", Symbol: symX}

	inputs := litretInputs(g, symX, httpLit,
		constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.HashTypeKey(jsonLit.Hash())}),
		branch, thenNode)
	inputs.TypeKeys[jsonLit.Hash()] = jsonLit
	inputs.Assignments = []UnifiedAssignment{{Point: branch, TargetPath: pathX}}

	s := Solve(inputs, testResolver())
	if s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("reassigned parameter can hold another value at the edge, edge should stay reachable")
	}
}

func TestFlow_TypeTheoryNilBase(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symY := setupSymbol(g, "y", allPoints)
	verY := cfg.Version{Root: "y", Symbol: symY, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symY, verY)
	}
	g.params = []cfg.SymbolID{symY}

	pathY := constraint.Path{Root: "y", Symbol: symY}

	inputs := litretInputs(g, symY, typ.Nil,
		constraint.FromConstraints(constraint.NotNil{Path: pathY}),
		branch, thenNode)

	s := Solve(inputs, testResolver())
	if !s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("expected NotNil edge of nil to be unreachable")
	}

	inputs = litretInputs(g, symY, typ.Nil,
		constraint.FromConstraints(constraint.IsNil{Path: pathY}),
		branch, thenNode)

	s = Solve(inputs, testResolver())
	if s.IsEdgeUnreachable(branch, thenNode) {
		t.Error("IsNil edge of nil should stay reachable")
	}
}
