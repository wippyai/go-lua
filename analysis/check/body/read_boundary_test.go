package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type sourceValueFunc func(cfg.Point, factflow.ValueSource, state.State, func(cfg.Point) state.State) (product.Value, bool)

func (f sourceValueFunc) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	return f(point, source, in, read)
}

func TestSourceValueAtBoundaryDoesNotUseExplanationRecovery(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	decl := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), decl, false)
	graph.AddEdge(decl, use, false)
	graph.AddEdge(use, graph.Exit(), false)

	target := symbol.ID(17)
	declExpr := factflow.ExprRef(31)
	useExpr := factflow.ExprRef(32)
	declSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: declExpr, HasExpr: true}
	useSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: useExpr, HasExpr: true}

	weakUse := typevalue.FromType(reg, typ.Any)
	concreteDeclaration := typevalue.FromType(reg, typ.String)
	result := &Result{
		registry: reg,
		cfg:      &cfgbuild.Result{Graph: graph},
		facts: factflow.NewFacts(factflow.FactsInput{
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				useExpr: pathdom.NewPath(target, "x"),
			},
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				decl: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "x"), declSource),
			},
		}),
		flow: transfer.Result{
			decl: state.State{},
			use:  state.State{},
		},
		sources: sourceValueFunc(func(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
			switch source.ExprRef {
			case declExpr:
				return concreteDeclaration, true
			case useExpr:
				return weakUse, true
			default:
				return product.Value{}, false
			}
		}),
	}

	got, ok := result.SourceValueAtBoundary(use, useSource)
	if !ok {
		t.Fatal("SourceValueAtBoundary returned !ok")
	}
	if product.Equal(reg, got, concreteDeclaration) {
		t.Fatal("SourceValueAtBoundary recovered declaration value; semantic projection must stay solved-state only")
	}
	if !product.Equal(reg, got, weakUse) {
		t.Fatalf("SourceValueAtBoundary = %v, want weak solved source %v", got, weakUse)
	}

	recovered, ok := result.SourceValueForExplanationAtBoundary(use, useSource)
	if !ok {
		t.Fatal("SourceValueForExplanationAtBoundary returned !ok")
	}
	if !product.Equal(reg, recovered, concreteDeclaration) {
		t.Fatalf("declaration recovery = %v, want declaration value %v", recovered, concreteDeclaration)
	}
}
