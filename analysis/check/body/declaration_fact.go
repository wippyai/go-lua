package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// TypeDefinitionKind identifies the source declaration form associated with a
// CFG point.
type TypeDefinitionKind uint8

const (
	TypeDefinitionAlias TypeDefinitionKind = iota + 1
	TypeDefinitionInterface
)

// TypeDefinitionFact is the body-owned source projection for a type
// declaration. The checker owns this metadata because it is consumed by
// readmodels and diagnostics; cfgbuild owns only CFG topology.
type TypeDefinitionFact struct {
	Kind TypeDefinitionKind

	Stmt      ast.Stmt
	Type      *ast.TypeDefStmt
	Interface *ast.InterfaceDefStmt
}

// FunctionDefinitionFact is the body-owned source projection for a function
// declaration assignment.
type FunctionDefinitionFact struct {
	Stmt *ast.FuncDefStmt
	Name *ast.FuncName
	Func *ast.FunctionExpr

	TargetSymbol    symbol.ID
	HasTargetSymbol bool
	TargetPath      pathdom.Path
	HasTargetPath   bool
}

func (f FunctionDefinitionFact) copy() FunctionDefinitionFact {
	f.TargetPath = f.TargetPath.Clone()
	return f
}

type declarationFactSet struct {
	typeDefs map[cfg.Point]TypeDefinitionFact
	funcDefs map[cfg.Point]FunctionDefinitionFact
}

func (s declarationFactSet) typeDefinition(point cfg.Point) (TypeDefinitionFact, bool) {
	fact, ok := s.typeDefs[point]
	return fact, ok
}

func (s declarationFactSet) functionDefinition(point cfg.Point) (FunctionDefinitionFact, bool) {
	fact, ok := s.funcDefs[point]
	if !ok {
		return FunctionDefinitionFact{}, false
	}
	return fact.copy(), true
}

func declarationFactsFromSource(bindings *bind.Result, built *cfgbuild.Result, sourceStmts []ast.Stmt) declarationFactSet {
	out := declarationFactSet{
		typeDefs: map[cfg.Point]TypeDefinitionFact{},
		funcDefs: map[cfg.Point]FunctionDefinitionFact{},
	}
	if bindings == nil || built == nil || built.Graph == nil {
		return out
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.TypeDefStmt:
				addTypeDefinitionFact(out.typeDefs, built, stmt)
			case *ast.InterfaceDefStmt:
				addInterfaceDefinitionFact(out.typeDefs, built, stmt)
			case *ast.FuncDefStmt:
				addFunctionDefinitionFact(out.funcDefs, bindings, built, stmt)
			case *ast.IfStmt:
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			case *ast.NumberForStmt:
				walk(stmt.Stmts)
			case *ast.GenericForStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(sourceStmts)
	return out
}

func addTypeDefinitionFact(out map[cfg.Point]TypeDefinitionFact, built *cfgbuild.Result, stmt *ast.TypeDefStmt) {
	point := firstPointOfKindForStmt(built, stmt, cfg.NodeNoop)
	if point == 0 {
		return
	}
	out[point] = TypeDefinitionFact{
		Kind: TypeDefinitionAlias,
		Stmt: stmt,
		Type: stmt,
	}
}

func addInterfaceDefinitionFact(out map[cfg.Point]TypeDefinitionFact, built *cfgbuild.Result, stmt *ast.InterfaceDefStmt) {
	point := firstPointOfKindForStmt(built, stmt, cfg.NodeNoop)
	if point == 0 {
		return
	}
	out[point] = TypeDefinitionFact{
		Kind:      TypeDefinitionInterface,
		Stmt:      stmt,
		Interface: stmt,
	}
}

func addFunctionDefinitionFact(out map[cfg.Point]FunctionDefinitionFact, bindings *bind.Result, built *cfgbuild.Result, stmt *ast.FuncDefStmt) {
	if stmt == nil || stmt.Func == nil {
		return
	}
	point := firstAssignPointForStmt(built, stmt)
	if point == 0 {
		return
	}
	target, _ := pathexpr.ResolveFuncName(stmt.Name, bindings)
	id, hasSymbol := bindings.FuncDefTargetSymbol(stmt)
	out[point] = FunctionDefinitionFact{
		Stmt:            stmt,
		Name:            stmt.Name,
		Func:            stmt.Func,
		TargetSymbol:    id,
		HasTargetSymbol: hasSymbol && id != 0,
		TargetPath:      target,
		HasTargetPath:   !target.IsEmpty(),
	}
}

func firstPointOfKindForStmt(built *cfgbuild.Result, stmt ast.Stmt, kind cfg.NodeKind) cfg.Point {
	if built == nil || built.Graph == nil || stmt == nil {
		return 0
	}
	for _, point := range built.StmtPoints.PointsFor(stmt) {
		node := built.Graph.Node(point)
		if node != nil && node.Kind == kind {
			return point
		}
	}
	return 0
}
