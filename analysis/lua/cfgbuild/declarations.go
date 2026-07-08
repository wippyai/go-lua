package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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

// TypeDefinition describes a type declaration associated with a CFG point.
type TypeDefinition struct {
	Kind TypeDefinitionKind

	Stmt      ast.Stmt
	Type      *ast.TypeDefStmt
	Interface *ast.InterfaceDefStmt
}

// FunctionDefinition describes a function declaration associated with a CFG point.
type FunctionDefinition struct {
	Stmt *ast.FuncDefStmt
	Name *ast.FuncName
	Func *ast.FunctionExpr

	TargetSymbol    symbol.ID
	HasTargetSymbol bool
	TargetPath      path.Path
	HasTargetPath   bool
}

// Declarations records source declaration metadata produced while building CFG
// topology. It is structural source metadata, not a semantic fact lane.
type Declarations struct {
	typeDefs map[cfg.Point]TypeDefinition
	funcDefs map[cfg.Point]FunctionDefinition
}

func (d Declarations) TypeDefinition(point cfg.Point) (TypeDefinition, bool) {
	fact, ok := d.typeDefs[point]
	return fact, ok
}

func (d *Declarations) SetTypeDefinition(point cfg.Point, fact TypeDefinition) {
	if d.typeDefs == nil {
		d.typeDefs = make(map[cfg.Point]TypeDefinition)
	}
	d.typeDefs[point] = fact
}

func (d Declarations) FunctionDefinition(point cfg.Point) (FunctionDefinition, bool) {
	fact, ok := d.funcDefs[point]
	if !ok {
		return FunctionDefinition{}, false
	}
	return copyFunctionDefinition(fact), true
}

func (d *Declarations) SetFunctionDefinition(point cfg.Point, fact FunctionDefinition) {
	if d.funcDefs == nil {
		d.funcDefs = make(map[cfg.Point]FunctionDefinition)
	}
	d.funcDefs[point] = copyFunctionDefinition(fact)
}

func copyFunctionDefinition(fact FunctionDefinition) FunctionDefinition {
	fact.TargetPath = fact.TargetPath.Clone()
	return fact
}
