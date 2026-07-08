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
