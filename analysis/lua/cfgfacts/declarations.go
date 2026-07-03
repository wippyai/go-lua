package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// TypeDefinitionKind identifies the declaration form associated with a CFG point.
type TypeDefinitionKind uint8

// Type definition kinds describe the concrete declaration form.
const (
	TypeDefinitionUnknown TypeDefinitionKind = iota
	TypeDefinitionAlias
	TypeDefinitionInterface
)

// TypeDefinitionFact describes a type declaration associated with a CFG point.
type TypeDefinitionFact struct {
	Kind TypeDefinitionKind

	Stmt      ast.Stmt
	Type      *ast.TypeDefStmt
	Interface *ast.InterfaceDefStmt
}

// ShortCircuitGuardFact records the guard operand tested by a synthetic branch
// emitted for a short-circuit logical operand (the left operand of an and/or
// whose right operand carries projected calls). It lets the semantics layer
// rebuild a branch-condition fact at the branch point so the right-operand edge
// inherits the guard's flow narrowing, exactly as an explicit if would.
type ShortCircuitGuardFact struct {
	Stmt      ast.Stmt
	Condition ast.Expr
}

// ExpressionEvaluationFact records a value expression evaluated at a CFG point
// that exists only to carry path-sensitive facts. The node is intentionally a
// structural no-op: it changes no runtime state, but consumers can inspect the
// expression under the CFG edge environment active at that point.
type ExpressionEvaluationFact struct {
	Stmt ast.Stmt
	Expr ast.Expr
}

// FunctionDefinitionFact describes a function declaration associated with a CFG point.
type FunctionDefinitionFact struct {
	Stmt *ast.FuncDefStmt
	Name *ast.FuncName
	Func *ast.FunctionExpr

	TargetSymbol    symbol.ID
	HasTargetSymbol bool
	TargetPath      path.Path
	HasTargetPath   bool
}
