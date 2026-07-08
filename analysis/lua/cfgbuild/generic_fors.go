package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// GenericForRole identifies the structural generic-for position at a CFG point.
type GenericForRole uint8

// Generic-for roles identify the check/variable positions in a generic-for loop.
const (
	GenericForRoleCheck GenericForRole = iota + 1
	GenericForRoleVariable
)

// NoGenericForVariableIndex marks the check point in a generic-for loop.
const NoGenericForVariableIndex = -1

// GenericFor records the structural points and operands for a generic-for loop.
type GenericFor struct {
	Stmt *ast.GenericForStmt
	Role GenericForRole

	Names   []string
	Exprs   []ast.Expr
	Sources []sourceprovenance.ASTSource

	Symbols    []symbol.ID
	HasSymbols bool

	VariableIndex int
}

// GenericFors records generic-for structural metadata produced by cfgbuild.
type GenericFors struct {
	facts map[cfg.Point]GenericFor
}

func (g GenericFors) Get(point cfg.Point) (GenericFor, bool) {
	fact, ok := g.facts[point]
	if !ok {
		return GenericFor{}, false
	}
	return copyGenericFor(fact), true
}

func (g *GenericFors) Set(point cfg.Point, fact GenericFor) {
	if g.facts == nil {
		g.facts = make(map[cfg.Point]GenericFor)
	}
	g.facts[point] = copyGenericFor(fact)
}

func copyGenericFor(fact GenericFor) GenericFor {
	fact.Names = append([]string(nil), fact.Names...)
	fact.Exprs = append([]ast.Expr(nil), fact.Exprs...)
	fact.Sources = append([]sourceprovenance.ASTSource(nil), fact.Sources...)
	fact.Symbols = append([]symbol.ID(nil), fact.Symbols...)
	return fact
}
