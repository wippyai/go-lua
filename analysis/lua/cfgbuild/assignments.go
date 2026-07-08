package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type LocalAssignment struct {
	Stmt  *ast.LocalAssignStmt
	Index int

	Name   string
	Type   ast.TypeExpr
	Expr   ast.Expr
	Source sourceprovenance.ASTSource

	Symbol    symbol.ID
	HasSymbol bool

	Exprs []ast.Expr
	Types []ast.TypeExpr
}

type OrdinaryAssignment struct {
	Stmt  *ast.AssignStmt
	Index int

	Target ast.Expr
	Value  ast.Expr
	Source sourceprovenance.ASTSource

	Symbol    symbol.ID
	HasSymbol bool
	Path      path.Path
	HasPath   bool

	ContainerPath    path.Path
	HasContainerPath bool

	Lhs []ast.Expr
	Rhs []ast.Expr
}

type Assignments struct {
	local    map[cfg.Point]LocalAssignment
	ordinary map[cfg.Point]OrdinaryAssignment
}

func (a Assignments) Local(point cfg.Point) (LocalAssignment, bool) {
	fact, ok := a.local[point]
	if !ok {
		return LocalAssignment{}, false
	}
	return copyLocalAssignment(fact), true
}

func (a Assignments) Ordinary(point cfg.Point) (OrdinaryAssignment, bool) {
	fact, ok := a.ordinary[point]
	if !ok {
		return OrdinaryAssignment{}, false
	}
	return copyOrdinaryAssignment(fact), true
}

func (a *Assignments) SetLocal(point cfg.Point, fact LocalAssignment) {
	if point == 0 {
		return
	}
	if a.local == nil {
		a.local = make(map[cfg.Point]LocalAssignment)
	}
	a.local[point] = copyLocalAssignment(fact)
}

func (a *Assignments) SetOrdinary(point cfg.Point, fact OrdinaryAssignment) {
	if point == 0 {
		return
	}
	if a.ordinary == nil {
		a.ordinary = make(map[cfg.Point]OrdinaryAssignment)
	}
	a.ordinary[point] = copyOrdinaryAssignment(fact)
}

func copyLocalAssignment(fact LocalAssignment) LocalAssignment {
	fact.Exprs = copyExprs(fact.Exprs)
	fact.Types = copyTypeExprs(fact.Types)
	return fact
}

func copyOrdinaryAssignment(fact OrdinaryAssignment) OrdinaryAssignment {
	fact.Path = fact.Path.Clone()
	fact.ContainerPath = fact.ContainerPath.Clone()
	fact.Lhs = copyExprs(fact.Lhs)
	fact.Rhs = copyExprs(fact.Rhs)
	return fact
}

func copyExprs(in []ast.Expr) []ast.Expr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.Expr, len(in))
	copy(out, in)
	return out
}

func copyTypeExprs(in []ast.TypeExpr) []ast.TypeExpr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.TypeExpr, len(in))
	copy(out, in)
	return out
}

func exprAt(exprs []ast.Expr, index int) ast.Expr {
	if index < 0 || index >= len(exprs) {
		return nil
	}
	return exprs[index]
}

func typeAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}
