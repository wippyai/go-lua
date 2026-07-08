package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NumericForRole identifies the structural numeric-for position at a CFG point.
type NumericForRole uint8

// Numeric for roles identify the init/check positions in a numeric-for loop.
const (
	NumericForRoleInit NumericForRole = iota + 1
	NumericForRoleCheck
)

// NumericForFact describes a numeric-for loop point.
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

// GenericForRole identifies the structural generic-for position at a CFG point.
type GenericForRole uint8

// Generic for roles identify the check/variable positions in a generic-for loop.
const (
	GenericForRoleCheck GenericForRole = iota + 1
	GenericForRoleVariable
)

// NoGenericForVariableIndex marks the check point in a generic-for loop.
const NoGenericForVariableIndex = -1

// GenericForFact describes a generic-for loop point.
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
