package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func collectDiagnosticReachability(result *body.Result, graph cfg.Graph) map[cfg.Point]bool {
	reachable := make(map[cfg.Point]bool)
	if result == nil || graph == nil {
		return reachable
	}
	envs := cachedGuardEnvironments(result)
	for _, point := range graph.RPO() {
		env, ok := envs[point]
		reachable[point] = !ok || !env.unreachable
	}
	return reachable
}

func diagnosticPointReachable(reachable map[cfg.Point]bool, point cfg.Point) bool {
	if reachable == nil {
		return true
	}
	ok, known := reachable[point]
	return !known || ok
}

func collectFunctionCaptureReads(result *body.Result) map[*ast.FunctionExpr][]symbol.ID {
	sets := make(map[*ast.FunctionExpr]map[symbol.ID]struct{})
	var walk func(*body.Result) map[symbol.ID]struct{}
	walk = func(parent *body.Result) map[symbol.ID]struct{} {
		out := make(map[symbol.ID]struct{})
		if parent == nil {
			return out
		}
		for _, child := range parent.FunctionResults() {
			childSet := make(map[symbol.ID]struct{})
			for _, capture := range parent.DirectCaptures(child.Function()) {
				if capture.Captured != 0 {
					childSet[capture.Captured] = struct{}{}
				}
			}
			for id := range walk(child) {
				childSet[id] = struct{}{}
			}
			if len(childSet) > 0 && child.Function() != nil {
				sets[child.Function()] = childSet
			}
			for id := range childSet {
				out[id] = struct{}{}
			}
		}
		return out
	}
	walk(result)
	out := make(map[*ast.FunctionExpr][]symbol.ID, len(sets))
	for fn, set := range sets {
		ids := make([]symbol.ID, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		out[fn] = ids
	}
	return out
}

func collectReachableSymbolReads(result *body.Result, graph cfg.Graph, reachable map[cfg.Point]bool) map[cfg.Point]map[symbol.ID]struct{} {
	reads := make(map[cfg.Point]map[symbol.ID]struct{})
	functionCaptures := collectFunctionCaptureReads(result)
	add := func(point cfg.Point, id symbol.ID) {
		if id == 0 {
			return
		}
		if reads[point] == nil {
			reads[point] = make(map[symbol.ID]struct{})
		}
		reads[point][id] = struct{}{}
	}
	for _, point := range graph.RPO() {
		if !diagnosticPointReachable(reachable, point) {
			continue
		}
		collector := symbolReadCollector{result: result, functionCaptures: functionCaptures, add: func(id symbol.ID) { add(point, id) }}
		if fact, ok := result.LocalAssignment(point); ok {
			collector.exprs(fact.Exprs)
			collector.typeExprs(fact.Types)
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			collector.exprs(fact.Rhs)
			collector.lvalues(fact.Lhs)
		}
		if fact, ok := result.Call(point); ok {
			collector.expr(fact.Func)
			collector.expr(fact.Receiver)
			collector.exprs(fact.Args)
		}
		if fact, ok := result.ReturnFact(point); ok {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := result.BranchCondition(point); ok {
			collector.expr(fact.Condition)
		}
		if fact, ok := result.NumericFor(point); ok {
			collector.expr(fact.Init)
			collector.expr(fact.Limit)
			collector.expr(fact.Step)
		}
		if fact, ok := result.GenericFor(point); ok && fact.Role == cfgfacts.GenericForRoleCheck {
			collector.exprs(fact.Exprs)
		}
		if fact, ok := result.TypeDefinition(point); ok {
			collector.typeDefinition(fact)
		}
		if fact, ok := result.FunctionDefinition(point); ok {
			collector.functionNameReads(fact.Name)
			collector.expr(fact.Func)
		}
	}
	return reads
}

func symbolHasReachableRead(readsByPoint map[cfg.Point]map[symbol.ID]struct{}, id symbol.ID) bool {
	for _, reads := range readsByPoint {
		if _, ok := reads[id]; ok {
			return true
		}
	}
	return false
}

type symbolReadCollector struct {
	result           *body.Result
	functionCaptures map[*ast.FunctionExpr][]symbol.ID
	add              func(symbol.ID)
}

func (c symbolReadCollector) exprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.expr(expr)
	}
}

func (c symbolReadCollector) typeExprs(exprs []ast.TypeExpr) {
	for _, expr := range exprs {
		c.typeExpr(expr)
	}
}

func (c symbolReadCollector) typeParams(params []ast.TypeParamExpr) {
	for _, param := range params {
		c.typeExpr(param.Constraint)
	}
}

func (c symbolReadCollector) functionParams(params []ast.FunctionParamExpr) {
	for _, param := range params {
		c.typeExpr(param.Type)
	}
}

func (c symbolReadCollector) typeDefinition(fact cfgfacts.TypeDefinitionFact) {
	if fact.Type != nil {
		c.typeParams(fact.Type.TypeParams)
		c.typeExpr(fact.Type.Type)
	}
	if fact.Interface != nil {
		for _, field := range fact.Interface.Fields {
			c.typeExpr(field.Type)
		}
		for _, method := range fact.Interface.Methods {
			if method.Type != nil {
				c.typeExpr(method.Type)
			}
		}
	}
}

func (c symbolReadCollector) functionTypeExprs(fn *ast.FunctionExpr) {
	if fn == nil {
		return
	}
	c.typeParams(fn.TypeParams)
	if fn.ParList != nil {
		c.typeExprs(fn.ParList.Types)
		c.typeExpr(fn.ParList.VarargType)
	}
	c.typeExprs(fn.ReturnTypes)
}

func (c symbolReadCollector) typeExpr(expr ast.TypeExpr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.PrimitiveTypeExpr, *ast.SelfTypeExpr, *ast.LiteralTypeExpr, *ast.TypeRefExpr:
		return
	case *ast.OptionalTypeExpr:
		c.typeExpr(e.Inner)
	case *ast.UnionTypeExpr:
		c.typeExprs(e.Types)
	case *ast.IntersectionTypeExpr:
		c.typeExprs(e.Types)
	case *ast.ArrayTypeExpr:
		c.typeExpr(e.Element)
	case *ast.MapTypeExpr:
		c.typeExpr(e.Key)
		c.typeExpr(e.Value)
	case *ast.RecordTypeExpr:
		for _, field := range e.Fields {
			c.typeExpr(field.Type)
		}
	case *ast.FunctionTypeExpr:
		c.typeParams(e.TypeParams)
		c.functionParams(e.Params)
		c.typeExpr(e.Variadic)
		c.typeExprs(e.Returns)
	case *ast.AssertsTypeExpr:
		c.typeExpr(e.NarrowTo)
	case *ast.GenericTypeExpr:
		c.typeExprs(e.Args)
	case *ast.MetaTypeExpr:
		c.typeExpr(e.Inner)
	case *ast.TupleTypeExpr:
		c.typeExprs(e.Elements)
	case *ast.TypeOfExpr:
		c.expr(e.Expr)
	case *ast.KeyOfExpr:
		c.typeExpr(e.Inner)
	case *ast.IndexAccessExpr:
		c.typeExpr(e.Object)
		c.typeExpr(e.Index)
	case *ast.ConditionalTypeExpr:
		c.typeExpr(e.Check)
		c.typeExpr(e.Extends)
		c.typeExpr(e.Then)
		c.typeExpr(e.Else)
	}
}

func (c symbolReadCollector) lvalues(exprs []ast.Expr) {
	for _, expr := range exprs {
		c.lvalue(expr)
	}
}

func (c symbolReadCollector) lvalue(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		return
	case *ast.AttrGetExpr:
		c.expr(e.Object)
		c.expr(e.Key)
	default:
		c.expr(expr)
	}
}

func (c symbolReadCollector) functionNameReads(name *ast.FuncName) {
	if name == nil {
		return
	}
	c.lvalue(name.Func)
	c.expr(name.Receiver)
}

func (c symbolReadCollector) expr(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		if id, ok := c.result.SymbolOfIdent(e); ok {
			c.add(id)
		}
	case *ast.AttrGetExpr:
		c.expr(e.Object)
		c.expr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			c.expr(field.Key)
			c.expr(field.Value)
		}
	case *ast.FuncCallExpr:
		c.expr(e.Func)
		c.expr(e.Receiver)
		c.exprs(e.Args)
	case *ast.LogicalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
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
	case *ast.CastExpr:
		c.expr(e.Expr)
	case *ast.NonNilAssertExpr:
		c.expr(e.Expr)
	case *ast.FunctionExpr:
		c.functionTypeExprs(e)
		for _, id := range c.functionCaptures[e] {
			c.add(id)
		}
		return
	}
}
