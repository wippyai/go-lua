package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// LocalAssignmentFact is the body-owned source projection for a local
// assignment. factflow owns assignment semantics; this view preserves source
// identity, declared annotation syntax, and target binding for readmodel and
// diagnostics.
type LocalAssignmentFact struct {
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

// OrdinaryAssignmentFact is the body-owned source projection for an ordinary
// assignment or function-definition assignment target.
type OrdinaryAssignmentFact struct {
	Stmt  *ast.AssignStmt
	Index int

	Target ast.Expr
	Value  ast.Expr
	Source sourceprovenance.ASTSource

	Symbol    symbol.ID
	HasSymbol bool
	Path      pathdom.Path
	HasPath   bool

	ContainerPath    pathdom.Path
	HasContainerPath bool

	Lhs []ast.Expr
	Rhs []ast.Expr
}

func (f LocalAssignmentFact) copy() LocalAssignmentFact {
	f.Exprs = copyAssignmentExprs(f.Exprs)
	f.Types = copyAssignmentTypes(f.Types)
	return f
}

func (f OrdinaryAssignmentFact) copy() OrdinaryAssignmentFact {
	f.Path = f.Path.Clone()
	f.ContainerPath = f.ContainerPath.Clone()
	f.Lhs = copyAssignmentExprs(f.Lhs)
	f.Rhs = copyAssignmentExprs(f.Rhs)
	return f
}

func assignmentFactsFromSource(bindings *bind.Result, built *cfgbuild.Result, sourceStmts []ast.Stmt) assignmentFactSet {
	out := assignmentFactSet{
		local:    map[cfg.Point]LocalAssignmentFact{},
		ordinary: map[cfg.Point]OrdinaryAssignmentFact{},
	}
	if bindings == nil || built == nil || built.Graph == nil {
		return out
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.AssignStmt:
				addOrdinaryAssignmentFacts(out.ordinary, bindings, built, stmt)
			case *ast.LocalAssignStmt:
				addLocalAssignmentFacts(out.local, bindings, built, stmt)
			case *ast.FuncDefStmt:
				addFunctionDefinitionAssignmentFact(out.ordinary, bindings, built, stmt)
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

type assignmentFactSet struct {
	local    map[cfg.Point]LocalAssignmentFact
	ordinary map[cfg.Point]OrdinaryAssignmentFact
}

func (s assignmentFactSet) localAt(point cfg.Point) (LocalAssignmentFact, bool) {
	fact, ok := s.local[point]
	if !ok {
		return LocalAssignmentFact{}, false
	}
	return fact.copy(), true
}

func (s assignmentFactSet) ordinaryAt(point cfg.Point) (OrdinaryAssignmentFact, bool) {
	fact, ok := s.ordinary[point]
	if !ok {
		return OrdinaryAssignmentFact{}, false
	}
	return fact.copy(), true
}

func addLocalAssignmentFacts(out map[cfg.Point]LocalAssignmentFact, bindings *bind.Result, built *cfgbuild.Result, stmt *ast.LocalAssignStmt) {
	if stmt == nil || len(stmt.Names) == 0 {
		return
	}
	points := assignmentPointsForStmt(built, stmt)
	if len(points) == 0 {
		return
	}
	sources := assignmentSourcesForStmt(bindings, built, stmt, stmt.Exprs, len(stmt.Names))
	exprs := copyAssignmentExprs(stmt.Exprs)
	types := copyAssignmentTypes(stmt.Types)
	for i, point := range points {
		if i >= len(stmt.Names) {
			break
		}
		id, hasSymbol := bindings.LocalSymbolAt(stmt, i)
		out[point] = LocalAssignmentFact{
			Stmt:      stmt,
			Index:     i,
			Name:      stmt.Names[i],
			Type:      assignmentTypeAt(stmt.Types, i),
			Expr:      assignmentExprAt(stmt.Exprs, i),
			Source:    assignmentSourceAt(sources, i),
			Symbol:    id,
			HasSymbol: hasSymbol && id != 0,
			Exprs:     exprs,
			Types:     types,
		}
	}
}

func addOrdinaryAssignmentFacts(out map[cfg.Point]OrdinaryAssignmentFact, bindings *bind.Result, built *cfgbuild.Result, stmt *ast.AssignStmt) {
	if stmt == nil || len(stmt.Lhs) == 0 {
		return
	}
	points := assignmentPointsForStmt(built, stmt)
	if len(points) == 0 {
		return
	}
	sources := assignmentSourcesForStmt(bindings, built, stmt, stmt.Rhs, len(stmt.Lhs))
	lhs := copyAssignmentExprs(stmt.Lhs)
	rhs := copyAssignmentExprs(stmt.Rhs)
	for i, point := range points {
		if i >= len(stmt.Lhs) {
			break
		}
		target := stmt.Lhs[i]
		id, hasSymbol := symbol.ID(0), false
		if ident, ok := target.(*ast.IdentExpr); ok {
			id, hasSymbol = bindings.SymbolOf(ident)
		}
		targetPath, hasPath := pathexpr.Resolve(target, bindings)
		containerPath, hasContainerPath := pathexpr.ResolveMutationContainer(target, bindings)
		out[point] = OrdinaryAssignmentFact{
			Stmt:             stmt,
			Index:            i,
			Target:           target,
			Value:            assignmentExprAt(stmt.Rhs, i),
			Source:           assignmentSourceAt(sources, i),
			Symbol:           id,
			HasSymbol:        hasSymbol && id != 0,
			Path:             targetPath,
			HasPath:          hasPath,
			ContainerPath:    containerPath,
			HasContainerPath: hasContainerPath,
			Lhs:              lhs,
			Rhs:              rhs,
		}
	}
}

func addFunctionDefinitionAssignmentFact(out map[cfg.Point]OrdinaryAssignmentFact, bindings *bind.Result, built *cfgbuild.Result, stmt *ast.FuncDefStmt) {
	if stmt == nil || stmt.Func == nil {
		return
	}
	targetPath, ok := pathexpr.ResolveFuncName(stmt.Name, bindings)
	if !ok || targetPath.IsEmpty() {
		return
	}
	point := firstAssignPointForStmt(built, stmt)
	if point == 0 {
		return
	}
	id, hasSymbol := bindings.FuncDefTargetSymbol(stmt)
	container := targetPath.Parent()
	out[point] = OrdinaryAssignmentFact{
		Index:            0,
		Target:           functionDefinitionTargetExpr(stmt),
		Value:            stmt.Func,
		Source:           assignmentSourceAt(sourceprovenance.AssignmentSources([]ast.Expr{stmt.Func}, 1, nil), 0),
		Symbol:           id,
		HasSymbol:        hasSymbol && id != 0,
		Path:             targetPath,
		HasPath:          true,
		ContainerPath:    container,
		HasContainerPath: !container.IsEmpty(),
		Rhs:              []ast.Expr{stmt.Func},
	}
}

func assignmentSourcesForStmt(bindings *bind.Result, built *cfgbuild.Result, stmt ast.Stmt, exprs []ast.Expr, targetCount int) []sourceprovenance.ASTSource {
	calls, ok := callorder.ValueList(exprs, sourceCallOrderOptions(bindings))
	if !ok {
		return nil
	}
	callPoints := sourceCallPointsForStmt(built, stmt)
	if len(callPoints) != len(calls) {
		return nil
	}
	resolver := sourceCallPointResolver(calls, callPoints)
	return sourceprovenance.AssignmentSources(exprs, targetCount, resolver)
}

func assignmentPointsForStmt(built *cfgbuild.Result, stmt ast.Stmt) []cfg.Point {
	if built == nil || built.Graph == nil || stmt == nil {
		return nil
	}
	var out []cfg.Point
	for _, point := range built.StmtPoints.PointsFor(stmt) {
		node := built.Graph.Node(point)
		if node != nil && node.Kind == cfg.NodeAssign {
			out = append(out, point)
		}
	}
	return out
}

func firstAssignPointForStmt(built *cfgbuild.Result, stmt ast.Stmt) cfg.Point {
	points := assignmentPointsForStmt(built, stmt)
	if len(points) == 0 {
		return 0
	}
	return points[0]
}

func assignmentSourceAt(sources []sourceprovenance.ASTSource, index int) sourceprovenance.ASTSource {
	if index < 0 || index >= len(sources) {
		return sourceprovenance.ASTSource{}
	}
	return sources[index]
}

func functionDefinitionTargetExpr(stmt *ast.FuncDefStmt) ast.Expr {
	if stmt == nil || stmt.Name == nil || stmt.Name.Method != "" {
		return nil
	}
	return stmt.Name.Func
}

func copyAssignmentExprs(in []ast.Expr) []ast.Expr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.Expr, len(in))
	copy(out, in)
	return out
}

func copyAssignmentTypes(in []ast.TypeExpr) []ast.TypeExpr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.TypeExpr, len(in))
	copy(out, in)
	return out
}

func assignmentExprAt(exprs []ast.Expr, index int) ast.Expr {
	if index < 0 || index >= len(exprs) {
		return nil
	}
	return exprs[index]
}

func assignmentTypeAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}
