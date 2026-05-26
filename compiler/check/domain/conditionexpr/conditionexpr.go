// Package conditionexpr extracts expression-local truth conditions.
//
// This package owns the AST-to-condition algebra used by short-circuit
// expression scopes. Flow extraction, synthesis, and observation consume this
// surface instead of rebuilding local override maps or parallel logical DNF
// helpers.
package conditionexpr

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

type versionedGraph interface {
	VisibleVersion(cfg.Point, cfg.SymbolID) cfg.Version
}

// Branches holds the conditions for the truthy and falsy outcomes of an
// expression.
type Branches struct {
	OnTrue  constraint.Condition
	OnFalse constraint.Condition
}

// Extractor projects Lua condition expressions into flow-domain constraints.
type Extractor struct {
	P             cfg.Point
	SC            *scope.State
	Inputs        *flow.Inputs
	Bindings      *bind.BindingTable
	Graph         versionedGraph
	ConstResolver func(string) *flow.ConstValue
}

// ConditionForTruth returns the condition that holds when expr evaluates to the
// requested truthiness.
func (e Extractor) ConditionForTruth(expr ast.Expr, truthy bool) constraint.Condition {
	branches := e.Conditions(expr)
	if truthy {
		return branches.OnTrue
	}
	return branches.OnFalse
}

// Conditions returns both truthy and falsy conditions for expr.
func (e Extractor) Conditions(expr ast.Expr) Branches {
	switch v := expr.(type) {
	case *ast.TrueExpr:
		return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.FalseCondition()}
	case *ast.FalseExpr:
		return Branches{OnTrue: constraint.FalseCondition(), OnFalse: constraint.TrueCondition()}
	case *ast.UnaryNotOpExpr:
		inner := e.Conditions(v.Expr)
		return Branches{OnTrue: inner.OnFalse, OnFalse: inner.OnTrue}
	case *ast.LogicalOpExpr:
		return e.logicalConditions(v)
	case *ast.RelationalOpExpr:
		return e.relationalConditions(v)
	case *ast.AttrGetExpr:
		return e.pathConditions(v, true)
	case *ast.IdentExpr:
		if v.Value == "true" {
			return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.FalseCondition()}
		}
		if v.Value == "false" {
			return Branches{OnTrue: constraint.FalseCondition(), OnFalse: constraint.TrueCondition()}
		}
		return e.pathConditions(v, false)
	default:
		return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.TrueCondition()}
	}
}

func (e Extractor) logicalConditions(expr *ast.LogicalOpExpr) Branches {
	if expr == nil {
		return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.TrueCondition()}
	}
	left := e.Conditions(expr.Lhs)
	right := e.Conditions(expr.Rhs)
	switch expr.Operator {
	case "and":
		return Branches{
			OnTrue:  constraint.And(left.OnTrue, right.OnTrue),
			OnFalse: constraint.Or(left.OnFalse, constraint.And(left.OnTrue, right.OnFalse)),
		}
	case "or":
		return Branches{
			OnTrue:  constraint.Or(left.OnTrue, constraint.And(left.OnFalse, right.OnTrue)),
			OnFalse: constraint.And(left.OnFalse, right.OnFalse),
		}
	default:
		return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.TrueCondition()}
	}
}

func (e Extractor) relationalConditions(expr *ast.RelationalOpExpr) Branches {
	if expr == nil {
		return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.TrueCondition()}
	}
	if cmp, ok := guard.ExtractTypeProbeComparison(expr); ok {
		path := e.pathFromExpr(cmp.Probe.Expr)
		if path.IsEmpty() {
			return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.TrueCondition()}
		}
		hasType := constraint.FromConstraints(constraint.HasType{Path: path, Type: cmp.Probe.Key})
		notHasType := constraint.FromConstraints(constraint.NotHasType{Path: path, Type: cmp.Probe.Key})
		if cmp.Equal {
			return Branches{OnTrue: hasType, OnFalse: notHasType}
		}
		return Branches{OnTrue: notHasType, OnFalse: hasType}
	}
	return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.TrueCondition()}
}

func (e Extractor) pathConditions(expr ast.Expr, includeFieldPresence bool) Branches {
	path := e.pathFromExpr(expr)
	if path.IsEmpty() {
		return Branches{OnTrue: constraint.TrueCondition(), OnFalse: constraint.TrueCondition()}
	}
	onTrue := []constraint.Constraint{constraint.Truthy{Path: path}}
	if includeFieldPresence {
		if attr, ok := expr.(*ast.AttrGetExpr); ok {
			if basePath := e.pathFromExpr(attr.Object); !basePath.IsEmpty() {
				if seg, ok := flowpath.StaticAttrKeySegmentWithConst(attr.Key, e.ConstResolver); ok && seg.Kind == constraint.SegmentField {
					onTrue = append(onTrue, constraint.HasField{Path: basePath, Field: seg.Name})
				}
			}
		}
	}
	return Branches{
		OnTrue:  constraint.FromConstraints(onTrue...),
		OnFalse: constraint.FromConstraints(constraint.Falsy{Path: path}),
	}
}

func (e Extractor) pathFromExpr(expr ast.Expr) constraint.Path {
	return flowpath.FromExprWithBindingsAt(expr, e.ConstResolver, e.bindings(), e.graph(), e.P)
}

func (e Extractor) bindings() *bind.BindingTable {
	if e.Bindings != nil {
		return e.Bindings
	}
	return resolve.GetBindings(e.Inputs)
}

func (e Extractor) graph() versionedGraph {
	if e.Graph != nil {
		return e.Graph
	}
	if e.Inputs != nil {
		return e.Inputs.Graph
	}
	return nil
}
