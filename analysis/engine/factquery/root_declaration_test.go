package factquery

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestDominatingRootDeclarationSourceFindsDeclarationOnIDomChain(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	target := symbol.ID(11)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(21), HasExpr: true}

	got, ok := DominatingRootDeclarationSource(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "value"), source),
		},
	}), graph)

	if !ok {
		t.Fatal("DominatingRootDeclarationSource returned !ok")
	}
	if got.Point != decl || got.Symbol != target || got.Source.ExprRef != source.ExprRef {
		t.Fatalf("source = %#v, want point %d symbol %d expr %d", got, decl, target, source.ExprRef)
	}
}

func TestDominatingRootDeclarationSourceStopsAtOrdinaryRootWrite(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	write := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, write, false)
	graph.AddEdge(write, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	target := symbol.ID(12)
	declSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(22), HasExpr: true}
	writeSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(23), HasExpr: true}

	_, ok := DominatingRootDeclarationSource(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl:  factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "value"), declSource),
			write: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, pathdom.NewPath(target, "value"), writeSource),
		},
	}), graph)

	if ok {
		t.Fatal("DominatingRootDeclarationSource crossed an ordinary root write")
	}
}

func TestDominatingRootDeclarationSourceIgnoresDescendantWrites(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	write := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, write, false)
	graph.AddEdge(write, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	target := symbol.ID(13)
	declSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(24), HasExpr: true}
	writeSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(25), HasExpr: true}

	got, ok := DominatingRootDeclarationSource(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl:  factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "value"), declSource),
			write: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, pathdom.NewPath(target, "value").Field("child"), writeSource),
		},
	}), graph)

	if !ok {
		t.Fatal("DominatingRootDeclarationSource treated descendant write as root overwrite")
	}
	if got.Point != decl || got.Source.ExprRef != declSource.ExprRef {
		t.Fatalf("source = %#v, want declaration source", got)
	}
}
