package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// GenericForRole identifies the structural generic-for position at a CFG point.
type GenericForRole uint8

const (
	GenericForRoleCheck GenericForRole = iota + 1
	GenericForRoleVariable
)

// NoGenericForVariableIndex marks the check point in a generic-for loop.
const NoGenericForVariableIndex = -1

// GenericForFact is the body-owned source projection for generic-for structure.
// Transfer owns the semantic iterator value; this projection supplies the
// source iterator expressions and variable binding points.
type GenericForFact struct {
	Stmt *ast.GenericForStmt
	Role GenericForRole

	Names   []string
	Exprs   []ast.Expr
	Sources []sourceprovenance.ASTSource

	Symbols    []symbol.ID
	HasSymbols bool

	VariableIndex int
}

func (f GenericForFact) copy() GenericForFact {
	f.Names = append([]string(nil), f.Names...)
	f.Exprs = append([]ast.Expr(nil), f.Exprs...)
	f.Sources = append([]sourceprovenance.ASTSource(nil), f.Sources...)
	f.Symbols = append([]symbol.ID(nil), f.Symbols...)
	return f
}

func genericForFactsFromSource(bindings *bind.Result, built *cfgbuild.Result, sourceStmts []ast.Stmt) map[cfg.Point]GenericForFact {
	out := map[cfg.Point]GenericForFact{}
	if bindings == nil || built == nil || built.Graph == nil {
		return out
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.GenericForStmt:
				addGenericForFact(out, bindings, built, stmt)
				walk(stmt.Stmts)
			case *ast.IfStmt:
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			case *ast.NumberForStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(sourceStmts)
	return out
}

func addGenericForFact(out map[cfg.Point]GenericForFact, bindings *bind.Result, built *cfgbuild.Result, stmt *ast.GenericForStmt) {
	if stmt == nil {
		return
	}
	symbols := bindings.GenericForSymbols(stmt)
	base := GenericForFact{
		Stmt:          stmt,
		Names:         append([]string(nil), stmt.Names...),
		Exprs:         append([]ast.Expr(nil), stmt.Exprs...),
		Symbols:       append([]symbol.ID(nil), symbols...),
		HasSymbols:    completeGenericForSymbols(symbols, len(stmt.Names)),
		VariableIndex: NoGenericForVariableIndex,
	}
	calls, callsOK := callorder.ValueList(stmt.Exprs, sourceCallOrderOptions(bindings))
	callPoints := sourceCallPointsForStmt(built, stmt)
	if callsOK && len(callPoints) >= len(calls) {
		resolver := sourceCallPointResolver(calls, callPoints[:len(calls)])
		base.Sources = sourceprovenance.ValueListSources(stmt.Exprs, false, resolver)
	}
	var branch cfg.Point
	var variables []cfg.Point
	for _, point := range built.StmtPoints.PointsFor(stmt) {
		node := built.Graph.Node(point)
		if node == nil {
			continue
		}
		switch node.Kind {
		case cfg.NodeBranch:
			if branch == 0 {
				branch = point
			}
		case cfg.NodeAssign:
			if branch != 0 {
				variables = append(variables, point)
			}
		}
	}
	if branch != 0 {
		fact := base.copy()
		fact.Role = GenericForRoleCheck
		out[branch] = fact
	}
	for i, point := range variables {
		if i >= len(stmt.Names) {
			break
		}
		fact := base.copy()
		fact.Role = GenericForRoleVariable
		fact.VariableIndex = i
		out[point] = fact
	}
}

func completeGenericForSymbols(symbols []symbol.ID, want int) bool {
	if len(symbols) != want {
		return false
	}
	for _, id := range symbols {
		if id == 0 {
			return false
		}
	}
	return true
}

func sourceCallPointResolver(calls []callorder.Occurrence, points []cfg.Point) sourceprovenance.CallPointResolver {
	if len(calls) == 0 || len(points) == 0 {
		return nil
	}
	callPoints := make(map[*ast.FuncCallExpr]cfg.Point, len(calls))
	exprPoints := make(map[int]cfg.Point, len(calls))
	for i, call := range calls {
		if i >= len(points) {
			break
		}
		callPoints[call.Call] = points[i]
		exprPoints[call.ExprIndex] = points[i]
	}
	return func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		if point, ok := callPoints[call]; ok {
			return point, true
		}
		point, ok := exprPoints[exprIndex]
		return point, ok
	}
}
