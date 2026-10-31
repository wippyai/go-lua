package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// An assignment at the target of a guarded edge reads the guarded value
// before it writes the new one: `if type(x) == "string" then x = f(x) end`.
func TestNarrowTypeBeforeAssuming_EdgeFactReachesWritingPoint(t *testing.T) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	before := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	after := cfg.Version{Root: "x", Symbol: symX, ID: 2}
	setVersion(g, c.Entry(), symX, before)
	setVersion(g, branch, symX, before)
	setVersion(g, thenNode, symX, after)
	g.params = []cfg.SymbolID{symX}

	pathX := constraint.Path{Root: "x", Symbol: symX}
	declared := typ.NewUnion(typ.String, typ.Number)
	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = declared
	inputs.EdgeConditions = []EdgeCondition{{
		From:      branch,
		To:        thenNode,
		Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
	}}
	inputs.Assignments = []UnifiedAssignment{{Point: thenNode, TargetPath: pathX}}

	s := Solve(inputs, testResolver())
	if got := s.NarrowTypeBeforeAssuming(thenNode, pathX, declared, constraint.TrueCondition()); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("read before the write at the guarded point = %v, want string", got)
	}
}
