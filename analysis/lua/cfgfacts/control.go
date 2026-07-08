package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
