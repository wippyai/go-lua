package factquery

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

func TestDominatingRootDeclarationSourceCarriesDeclaredValue(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	reg := standard.Registry()
	target := symbol.ID(18)
	declared := typevalue.FromType(reg, typ.String)

	got, ok := DominatingRootDeclarationSource(use, target, factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl: factflow.NewRootAssignmentWithDeclaredContractValue(
				factflow.RootAssignmentLocalDeclaration,
				target,
				pathdom.NewPath(target, "value"),
				factflow.ValueSource{},
				declared,
			),
		},
	}), graph)

	if !ok {
		t.Fatal("DominatingRootDeclarationSource returned !ok")
	}
	if !got.HasDeclaredValue || !product.Equal(reg, got.DeclaredValue, declared) {
		t.Fatalf("declared value = %#v/%v, want string declared value", got.DeclaredValue, got.HasDeclaredValue)
	}
}

func TestRootDeclarationQueryAnswersMultipleQuestions(t *testing.T) {
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	write := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, write, false)
	graph.AddEdge(write, use, false)
	graph.AddEdge(use, graph.Exit(), false)
	declTarget := symbol.ID(31)
	writeTarget := symbol.ID(32)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(41), HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			decl:  factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, declTarget, pathdom.NewPath(declTarget, "decl"), source),
			write: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, writeTarget, pathdom.NewPath(writeTarget, "write"), factflow.ValueSource{}),
		},
	})
	query := NewRootDeclarationQuery(facts, graph)

	declaration, ok := query.DominatingRootDeclarationSource(use, declTarget)
	if !ok || declaration.Point != decl || declaration.Source.ExprRef != source.ExprRef {
		t.Fatalf("declaration = %#v/%v, want point %d expr %d", declaration, ok, decl, source.ExprRef)
	}
	replacement, ok := query.DominatingOrdinaryRootWrite(use, writeTarget)
	if !ok || replacement != write {
		t.Fatalf("replacement = %d/%v, want %d/true", replacement, ok, write)
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
