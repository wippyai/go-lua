package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Return records the structural return statement and its value-list sources.
type Return struct {
	Stmt    *ast.ReturnStmt
	Exprs   []ast.Expr
	Sources []sourceprovenance.ASTSource
}

// Returns records structural return metadata produced by cfgbuild.
type Returns struct {
	facts map[cfg.Point]Return
}

func (r Returns) Get(point cfg.Point) (Return, bool) {
	fact, ok := r.facts[point]
	if !ok {
		return Return{}, false
	}
	return copyReturn(fact), true
}

func (r *Returns) Set(point cfg.Point, fact Return) {
	if r.facts == nil {
		r.facts = make(map[cfg.Point]Return)
	}
	r.facts[point] = copyReturn(fact)
}

func copyReturn(fact Return) Return {
	fact.Exprs = append([]ast.Expr(nil), fact.Exprs...)
	fact.Sources = append([]sourceprovenance.ASTSource(nil), fact.Sources...)
	return fact
}
