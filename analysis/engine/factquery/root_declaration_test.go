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

func TestDominatingOrdinaryRootWriteIgnoresDescendantWrites(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	descendantWrite := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, descendantWrite, false)
	graph.AddEdge(descendantWrite, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	target := symbol.ID(16)

	_, ok := DominatingOrdinaryRootWrite(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl:            factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "value"), factflow.ValueSource{}),
			descendantWrite: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, pathdom.NewPath(target, "value").Field("child"), factflow.ValueSource{}),
		},
	}), graph)
	if ok {
		t.Fatal("DominatingOrdinaryRootWrite treated descendant write as root replacement")
	}
}

func TestDominatingOrdinaryRootWriteFindsRootReplacement(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	rootWrite := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, rootWrite, false)
	graph.AddEdge(rootWrite, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	target := symbol.ID(17)

	got, ok := DominatingOrdinaryRootWrite(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl:      factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "value"), factflow.ValueSource{}),
			rootWrite: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, pathdom.NewPath(target, "value"), factflow.ValueSource{}),
		},
	}), graph)
	if !ok || got != rootWrite {
		t.Fatalf("DominatingOrdinaryRootWrite = %d/%v, want %d/true", got, ok, rootWrite)
	}
}

func TestDominatingPathRootDeclarationSourceStopsAtQueriedPathWrite(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	write := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, write, false)
	graph.AddEdge(write, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	target := pathdom.NewPath(symbol.ID(14), "state").Field("active_sessions")
	declSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(26), HasExpr: true}
	writeSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(27), HasExpr: true}

	_, ok := DominatingPathRootDeclarationSource(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl:  factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target.Symbol, target.RootOnly(), declSource),
			write: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target.Symbol, target, writeSource),
		},
	}), graph)

	if ok {
		t.Fatal("DominatingPathRootDeclarationSource crossed a write to the queried path")
	}
}

func TestDominatingPathRootDeclarationSourceIgnoresDescendantEntryWrite(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	write := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, write, false)
	graph.AddEdge(write, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	target := pathdom.NewPath(symbol.ID(15), "state").Field("active_sessions")
	declSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(28), HasExpr: true}
	writeSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(29), HasExpr: true}

	got, ok := DominatingPathRootDeclarationSource(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl:  factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target.Symbol, target.RootOnly(), declSource),
			write: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target.Symbol, target.IndexStr("s1"), writeSource),
		},
	}), graph)

	if !ok {
		t.Fatal("DominatingPathRootDeclarationSource treated descendant entry write as container overwrite")
	}
	if got.Point != decl || got.Source.ExprRef != declSource.ExprRef {
		t.Fatalf("source = %#v, want declaration source", got)
	}
}
