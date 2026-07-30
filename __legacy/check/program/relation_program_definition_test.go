package program

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestRelationDefinitionPointOwnsFunctionBehindExpressionRefinement(t *testing.T) {
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	function := symbol.ID(7201)
	const functionRef, refinedRef = factflow.ExprRef(7201), factflow.ExprRef(7202)
	inner := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: functionRef, HasExpr: true}
	outer := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: refinedRef, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, symbol.ID(7202), pathdom.NewPath(symbol.ID(7202), "callback"), outer),
		},
		ExpressionFunctions: map[factflow.ExprRef]symbol.ID{functionRef: function},
		ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{
			refinedRef: factflow.NewExpressionRefinement(inner, product.Top()),
		},
	})
	point, err := relationDefinitionPoint(facts, graph, function)
	if err != nil {
		t.Fatal(err)
	}
	if point != assign {
		t.Fatalf("definition point = %d, want assignment %d", point, assign)
	}
	second := graph.AddNode(cfg.NodeAssign)
	duplicate := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, symbol.ID(7202), pathdom.NewPath(symbol.ID(7202), "first"), outer),
			second: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, symbol.ID(7203), pathdom.NewPath(symbol.ID(7203), "second"), outer),
		},
		ExpressionFunctions: map[factflow.ExprRef]symbol.ID{functionRef: function},
		ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{
			refinedRef: factflow.NewExpressionRefinement(inner, product.Top()),
		},
	})
	if _, err := relationDefinitionPoint(duplicate, graph, function); err == nil {
		t.Fatal("duplicate lexical function occurrence gained an ambiguous definition coordinate")
	}
}
