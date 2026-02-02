package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// This file provides utilities for finding table literals and their relationships
// to functions in a CFG. These utilities support self-type resolution for methods
// defined in table constructors or assigned to table fields.

// FindTableLiteralForSymbol finds the TableExpr assigned to a symbol.
//
// This is used to find the table literal that defines a class or object,
// enabling self-type resolution for methods defined on that table.
func FindTableLiteralForSymbol(graph *cfg.Graph, sym cfg.SymbolID) (*ast.TableExpr, cfg.Point) {
	if graph == nil || sym == 0 {
		return nil, 0
	}
	var result *ast.TableExpr
	var resultPoint cfg.Point
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if result != nil {
			return
		}
		for i, target := range info.Targets {
			if target.Symbol == sym && i < len(info.Sources) {
				if tbl, ok := info.Sources[i].(*ast.TableExpr); ok {
					result = tbl
					resultPoint = p
					return
				}
			}
		}
	})
	return result, resultPoint
}

// FindFieldAssignmentBase finds the base object symbol when a function is assigned via field assignment.
//
// For patterns like `obj.method = function(self) ... end`, this function finds
// the base object (obj) so that self can be typed as the object's type. Also
// returns the table literal assigned to that symbol, if any.
func FindFieldAssignmentBase(graph *cfg.Graph, fn *ast.FunctionExpr, point cfg.Point) (cfg.SymbolID, *ast.TableExpr, cfg.Point) {
	if graph == nil || fn == nil {
		return 0, nil, 0
	}
	var baseSym cfg.SymbolID
	var tblPoint cfg.Point
	matchFunc := func(expr ast.Expr) bool {
		if expr == nil {
			return false
		}
		if expr == fn {
			return true
		}
		other, ok := expr.(*ast.FunctionExpr)
		if !ok || other == nil {
			return false
		}
		return other.Line() == fn.Line() &&
			other.Column() == fn.Column() &&
			other.LastLine() == fn.LastLine() &&
			other.LastColumn() == fn.LastColumn()
	}

	// Prefer the assignment at the function's definition point.
	if point != 0 {
		if info := graph.Assign(point); info != nil {
			for i, target := range info.Targets {
				if i >= len(info.Sources) || !matchFunc(info.Sources[i]) {
					continue
				}
				if target.Kind == cfg.TargetField && target.BaseSymbol != 0 {
					baseSym = target.BaseSymbol
					break
				}
				if target.Kind == cfg.TargetIndex && target.BaseSymbol != 0 {
					baseSym = target.BaseSymbol
					break
				}
			}
		}
	}

	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if baseSym != 0 {
			return
		}
		for i, target := range info.Targets {
			if i >= len(info.Sources) || !matchFunc(info.Sources[i]) {
				continue
			}
			if target.Kind == cfg.TargetField && target.BaseSymbol != 0 {
				baseSym = target.BaseSymbol
				return
			}
			if target.Kind == cfg.TargetIndex && target.BaseSymbol != 0 {
				baseSym = target.BaseSymbol
				return
			}
		}
	})
	if baseSym == 0 {
		return 0, nil, 0
	}
	// Find the table literal assigned to the base symbol.
	tbl, p := FindTableLiteralForSymbol(graph, baseSym)
	if p != 0 {
		tblPoint = p
	}
	return baseSym, tbl, tblPoint
}

// FindTableLiteralOwner finds the table literal containing fn as a field value.
//
// For patterns like `local obj = { method = function(self) ... }`, this function
// finds the containing table so that self can be typed as the table's type.
// Returns both the TableExpr and its assigned symbol.
func FindTableLiteralOwner(graph *cfg.Graph, fn *ast.FunctionExpr) (*ast.TableExpr, cfg.SymbolID) {
	if graph == nil || fn == nil {
		return nil, 0
	}
	var resultTbl *ast.TableExpr
	var resultSym cfg.SymbolID
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if resultTbl != nil {
			return
		}
		for i, src := range info.Sources {
			tbl, ok := src.(*ast.TableExpr)
			if !ok {
				continue
			}
			for _, field := range tbl.Fields {
				if field.Value == fn {
					resultTbl = tbl
					if i < len(info.Targets) {
						resultSym = info.Targets[i].Symbol
					}
					return
				}
			}
		}
	})
	return resultTbl, resultSym
}
