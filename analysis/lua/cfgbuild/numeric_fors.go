package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NumericForRole identifies the structural numeric-for position at a CFG point.
type NumericForRole uint8

// Numeric-for roles identify the init/check positions in a numeric-for loop.
const (
	NumericForRoleInit NumericForRole = iota + 1
	NumericForRoleCheck
)

// NumericFor records the structural points and operands for a numeric-for loop.
type NumericFor struct {
	Stmt *ast.NumberForStmt
	Role NumericForRole

	Name  string
	Init  ast.Expr
	Limit ast.Expr
	Step  ast.Expr

	Symbol    symbol.ID
	HasSymbol bool
}

// NumericFors records numeric-for structural metadata produced by cfgbuild.
type NumericFors struct {
	facts map[cfg.Point]NumericFor
}

func (n NumericFors) Get(point cfg.Point) (NumericFor, bool) {
	fact, ok := n.facts[point]
	return fact, ok
}

func (n *NumericFors) Set(point cfg.Point, fact NumericFor) {
	if n.facts == nil {
		n.facts = make(map[cfg.Point]NumericFor)
	}
	n.facts[point] = fact
}
