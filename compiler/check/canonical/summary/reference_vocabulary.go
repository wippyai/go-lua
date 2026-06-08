package summary

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// ReferencePathProjectionForGraph derives the finite entry-path vocabulary a
// function can observe at entry. Value and callable reads keep exact paths;
// values that escape through calls, returns, or assignments keep subtrees so
// downstream callees/returns do not lose function-valued fields.
func ReferencePathProjectionForGraph(g *cfg.Graph) flow.ReferencePathProjection {
	if g == nil || g.Func() == nil || g.Bindings() == nil {
		return flow.ReferencePathProjection{}
	}
	c := referencePathCollector{
		graph:    g,
		captured: functionsymbols.CapturedFree(g, g.Func()),
	}
	for _, stmt := range g.Func().Stmts {
		c.stmt(stmt)
	}
	return flow.ReferencePathProjection{
		Exact:    c.paths(c.exact),
		Subtrees: c.paths(c.subtrees),
	}
}

type referencePathCollector struct {
	graph    *cfg.Graph
	captured functionsymbols.Set
	exact    map[constraint.PathKey]constraint.Path
	subtrees map[constraint.PathKey]constraint.Path
}

func (c *referencePathCollector) stmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, expr := range s.Lhs {
			c.writeTarget(expr)
		}
		for _, expr := range s.Rhs {
			c.escapeExpr(expr)
		}
	case *ast.LocalAssignStmt:
		for _, expr := range s.Exprs {
			c.escapeExpr(expr)
		}
	case *ast.FuncCallStmt:
		c.expr(s.Expr)
	case *ast.DoBlockStmt:
		c.stmts(s.Stmts)
	case *ast.WhileStmt:
		c.expr(s.Condition)
		c.stmts(s.Stmts)
	case *ast.RepeatStmt:
		c.stmts(s.Stmts)
		c.expr(s.Condition)
	case *ast.IfStmt:
		c.expr(s.Condition)
		c.stmts(s.Then)
		c.stmts(s.Else)
	case *ast.NumberForStmt:
		c.expr(s.Init)
		c.expr(s.Limit)
		c.expr(s.Step)
		c.stmts(s.Stmts)
	case *ast.GenericForStmt:
		for _, expr := range s.Exprs {
			c.expr(expr)
		}
		c.stmts(s.Stmts)
	case *ast.ReturnStmt:
		for _, expr := range s.Exprs {
			c.escapeExpr(expr)
		}
	case *ast.FuncDefStmt:
		// The nested body is represented by its own graph and vocabulary.
		return
	}
}

func (c *referencePathCollector) stmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		c.stmt(stmt)
	}
}

func (c *referencePathCollector) expr(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		c.addExact(c.pathOf(expr))
		c.call(e)
	case *ast.AttrGetExpr:
		if path := c.pathOf(e); !path.IsEmpty() {
			c.addExact(path)
			return
		}
		c.expr(e.Object)
		c.expr(e.Key)
	case *ast.TableExpr:
		c.addExact(c.pathOf(expr))
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			c.expr(field.Key)
			c.escapeExpr(field.Value)
		}
	case *ast.LogicalOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.RelationalOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.StringConcatOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Expr)
	case *ast.UnaryNotOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Expr)
	case *ast.UnaryLenOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		c.addExact(c.pathOf(expr))
		c.expr(e.Expr)
	case *ast.FunctionExpr:
		// Nested bodies are owned by their own graph.
		return
	default:
		c.addExact(c.pathOf(expr))
	}
}

func (c *referencePathCollector) call(call *ast.FuncCallExpr) {
	if call == nil {
		return
	}
	if call.Method != "" {
		if recv := c.pathOf(call.Receiver); !recv.IsEmpty() {
			c.addExact(recv.Field(call.Method))
			c.addExact(c.rootPath(recv))
		}
	} else {
		funcPath := c.pathOf(call.Func)
		c.addExact(funcPath)
		c.addExact(c.rootPath(funcPath))
	}
	c.expr(call.Func)
	c.expr(call.Receiver)
	firstArg := 0
	if call.Method != "" {
		firstArg = 1
	}
	for runtimeIdx := firstArg; runtimeIdx < callsite.RuntimeArgExprCount(call); runtimeIdx++ {
		c.escapeExpr(callsite.RuntimeArgExprAt(call, runtimeIdx))
	}
}

func (c *referencePathCollector) escapeExpr(expr ast.Expr) {
	c.addSubtree(c.pathOf(expr))
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		c.call(e)
	case *ast.AttrGetExpr:
		if path := c.pathOf(e); !path.IsEmpty() {
			c.addSubtree(path)
			return
		}
		c.expr(e.Object)
		c.expr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			c.expr(field.Key)
			c.escapeExpr(field.Value)
		}
	case *ast.LogicalOpExpr:
		c.escapeExpr(e.Lhs)
		c.escapeExpr(e.Rhs)
	case *ast.RelationalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.StringConcatOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryNotOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryLenOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		c.expr(e.Expr)
	case *ast.FunctionExpr:
		return
	}
}

func (c *referencePathCollector) writeTarget(expr ast.Expr) {
	path := c.pathOf(expr)
	if path.IsEmpty() {
		return
	}
	c.addExact(constraint.Path{Root: path.Root, Symbol: path.Symbol})
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		c.expr(e.Object)
		c.expr(e.Key)
	}
}

func (c *referencePathCollector) pathOf(expr ast.Expr) constraint.Path {
	if c == nil || c.graph == nil || c.graph.Bindings() == nil || expr == nil {
		return constraint.Path{}
	}
	path := flowpath.FromExprWithBindings(expr, nil, c.graph.Bindings())
	path.Version = 0
	return path
}

func (c *referencePathCollector) rootPath(path constraint.Path) constraint.Path {
	if path.IsEmpty() {
		return constraint.Path{}
	}
	return constraint.Path{Root: path.Root, Symbol: path.Symbol}
}

func (c *referencePathCollector) addExact(path constraint.Path) {
	c.add(&c.exact, path)
}

func (c *referencePathCollector) addSubtree(path constraint.Path) {
	c.add(&c.subtrees, path)
}

func (c *referencePathCollector) add(dst *map[constraint.PathKey]constraint.Path, path constraint.Path) {
	if path.IsEmpty() || path.Symbol == 0 {
		return
	}
	if c.graph == nil || c.graph.IsFreeSymbol(path.Symbol) {
		if !c.captured.Contains(path.Symbol) {
			return
		}
	}
	key := path.Key()
	if key == "" {
		return
	}
	if *dst == nil {
		*dst = make(map[constraint.PathKey]constraint.Path)
	}
	(*dst)[key] = path
}

func (c *referencePathCollector) paths(in map[constraint.PathKey]constraint.Path) []constraint.Path {
	if len(in) == 0 {
		return nil
	}
	keys := make([]constraint.PathKey, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]constraint.Path, 0, len(keys))
	for _, key := range keys {
		out = append(out, in[key])
	}
	return out
}
