package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NumericForRole identifies the structural numeric-for position at a CFG point.
type NumericForRole uint8

const (
	NumericForRoleInit NumericForRole = iota + 1
	NumericForRoleCheck
)

// NumericForFact is the body-owned source projection for numeric-for structure.
// WIR/factflow own numeric-for semantics; this view preserves source operands
// and the loop-symbol binding for diagnostics and readmodel queries.
type NumericForFact struct {
	Stmt *ast.NumberForStmt
	Role NumericForRole

	Name  string
	Init  ast.Expr
	Limit ast.Expr
	Step  ast.Expr

	Symbol    symbol.ID
	HasSymbol bool
}

func (r *Result) numericForFacts() map[cfg.Point]NumericForFact {
	if r == nil {
		return nil
	}
	if r.queries.numericForFactsOK {
		return r.queries.numericForFacts
	}
	out := r.computeNumericForFacts()
	r.queries.numericForFacts = out
	r.queries.numericForFactsOK = true
	return out
}

func (r *Result) computeNumericForFacts() map[cfg.Point]NumericForFact {
	out := map[cfg.Point]NumericForFact{}
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return out
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.NumberForStmt:
				r.addNumericForFact(out, stmt)
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
			case *ast.GenericForStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(r.sourceStmts)
	return out
}

func (r *Result) addNumericForFact(out map[cfg.Point]NumericForFact, stmt *ast.NumberForStmt) {
	if stmt == nil {
		return
	}
	id, hasSymbol := r.bindings.NumForSymbol(stmt)
	base := NumericForFact{
		Stmt:      stmt,
		Name:      stmt.Name,
		Init:      stmt.Init,
		Limit:     stmt.Limit,
		Step:      stmt.Step,
		Symbol:    id,
		HasSymbol: hasSymbol && id != 0,
	}
	var initPoint, checkPoint cfg.Point
	for _, point := range r.cfg.StmtPoints.PointsFor(stmt) {
		node := r.cfg.Graph.Node(point)
		if node == nil {
			continue
		}
		switch node.Kind {
		case cfg.NodeAssign:
			if initPoint == 0 {
				initPoint = point
			}
		case cfg.NodeBranch:
			if checkPoint == 0 {
				checkPoint = point
			}
		}
	}
	if initPoint != 0 {
		fact := base
		fact.Role = NumericForRoleInit
		out[initPoint] = fact
	}
	if checkPoint != 0 {
		fact := base
		fact.Role = NumericForRoleCheck
		out[checkPoint] = fact
	}
}
