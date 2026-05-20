package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// This file provides utilities for finding table literals and their relationships
// to functions in a CFG. These utilities support self-type resolution for methods
// defined in table constructors or assigned to table fields.

// FindTableLiteralForSymbol finds the TableExpr assigned to a symbol.
//
// This is used to find the table literal that defines a class or object,
// enabling self-type resolution for methods defined on that table.
func FindTableLiteralForSymbol(assignments []api.AssignmentEvidence, sym cfg.SymbolID) (*ast.TableExpr, cfg.Point) {
	if len(assignments) == 0 || sym == 0 {
		return nil, 0
	}
	var result *ast.TableExpr
	var resultPoint cfg.Point
	for _, assign := range assignments {
		if result != nil {
			break
		}
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if target.Symbol != sym {
				return
			}
			if tbl, ok := src.(*ast.TableExpr); ok {
				result = tbl
				resultPoint = p
			}
		})
	}
	return result, resultPoint
}

// FindFieldAssignmentBase finds the base object symbol when a function is assigned via field assignment.
//
// For patterns like `obj.method = function(self) ... end`, this function finds
// the base object (obj) so that self can be typed as the object's type. Also
// returns the table literal assigned to that symbol, if any.
func FindFieldAssignmentBase(assignments []api.AssignmentEvidence, fn *ast.FunctionExpr, point cfg.Point) (cfg.SymbolID, *ast.TableExpr, cfg.Point) {
	if len(assignments) == 0 || fn == nil {
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
		if info := assignmentInfoAt(assignments, point); info != nil {
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
				if !matchFunc(src) {
					return
				}
				if target.Kind == cfg.TargetField && target.BaseSymbol != 0 {
					baseSym = target.BaseSymbol
					return
				}
				if target.Kind == cfg.TargetIndex && target.BaseSymbol != 0 {
					baseSym = target.BaseSymbol
					return
				}
			})
		}
	}

	for _, assign := range assignments {
		if baseSym != 0 {
			break
		}
		info := assign.Info
		if info == nil {
			continue
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if !matchFunc(src) {
				return
			}
			if target.Kind == cfg.TargetField && target.BaseSymbol != 0 {
				baseSym = target.BaseSymbol
				return
			}
			if target.Kind == cfg.TargetIndex && target.BaseSymbol != 0 {
				baseSym = target.BaseSymbol
				return
			}
		})
	}
	if baseSym == 0 {
		return 0, nil, 0
	}
	// Find the table literal assigned to the base symbol.
	tbl, p := FindTableLiteralForSymbol(assignments, baseSym)
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
func FindTableLiteralOwner(assignments []api.AssignmentEvidence, fn *ast.FunctionExpr) (*ast.TableExpr, cfg.SymbolID) {
	if len(assignments) == 0 || fn == nil {
		return nil, 0
	}
	var resultTbl *ast.TableExpr
	var resultSym cfg.SymbolID
	for _, assign := range assignments {
		if resultTbl != nil {
			break
		}
		info := assign.Info
		if info == nil {
			continue
		}
		info.EachSource(func(i int, src ast.Expr) {
			if resultTbl != nil {
				return
			}
			tbl, ok := src.(*ast.TableExpr)
			if !ok {
				return
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
		})
	}
	return resultTbl, resultSym
}

func assignmentInfoAt(assignments []api.AssignmentEvidence, point cfg.Point) *cfg.AssignInfo {
	for _, assign := range assignments {
		if assign.Point == point {
			return assign.Info
		}
	}
	return nil
}
