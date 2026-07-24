package front

// This file turns immutable, binder-owned lexical topology into descriptive
// native contracts.  It deliberately does not infer a value: every row below
// is about syntax/binding topology (entry layout, a closed local call edge, or
// a closed recursive component).  The engine publishes these descriptors with
// its ordinary value closure after it has completed evaluation.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

type NativeContract struct {
	Family      string
	Value       string
	Revocations []string
}

func nativeContracts(stmts []ast.Stmt, bindings *bind.Result) []NativeContract {
	if bindings == nil {
		return nil
	}
	names := map[*ast.FunctionExpr]string{}
	byName := map[string]*ast.FunctionExpr{}
	functionNames := map[*ast.FunctionExpr]string{}
	var labelFunctions func([]ast.Stmt)
	labelFunctions = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.FuncDefStmt:
				if item.Func != nil && item.Name != nil {
					name := item.Name.Method
					if name == "" {
						if id, ok := item.Name.Func.(*ast.IdentExpr); ok {
							name = id.Value
						}
					}
					if name != "" {
						functionNames[item.Func] = name
					}
					labelFunctions(item.Func.Stmts)
				}
			case *ast.LocalAssignStmt:
				for i, expr := range item.Exprs {
					if i < len(item.Names) {
						if fn, ok := expr.(*ast.FunctionExpr); ok {
							functionNames[fn] = item.Names[i]
							labelFunctions(fn.Stmts)
						}
					}
				}
			case *ast.AssignStmt:
				for i, expr := range item.Rhs {
					if i < len(item.Lhs) {
						if fn, ok := expr.(*ast.FunctionExpr); ok {
							if id, ok := item.Lhs[i].(*ast.IdentExpr); ok {
								functionNames[fn] = id.Value
								labelFunctions(fn.Stmts)
							}
						}
					}
				}
			case *ast.IfStmt:
				labelFunctions(item.Then)
				labelFunctions(item.Else)
			case *ast.DoBlockStmt:
				labelFunctions(item.Stmts)
			}
		}
	}
	labelFunctions(stmts)
	literalMembers := map[string]bool{}
	for _, stmt := range stmts {
		local, ok := stmt.(*ast.LocalAssignStmt)
		if !ok {
			continue
		}
		for index, name := range local.Names {
			if index >= len(local.Exprs) {
				continue
			}
			table, ok := local.Exprs[index].(*ast.TableExpr)
			if !ok {
				continue
			}
			for _, field := range table.Fields {
				if field == nil || ast.KeyName(field.Key) == "" {
					continue
				}
				if _, callable := field.Value.(*ast.FunctionExpr); callable {
					literalMembers[name+"."+ast.KeyName(field.Key)] = true
				}
			}
		}
	}
	for _, origin := range bindings.FunctionOrigins() {
		name := ""
		if origin.HasTargetSymbol {
			name = bindings.Name(origin.TargetSymbol)
		}
		if def, ok := origin.Stmt.(*ast.FuncDefStmt); ok && def != nil && def.Name != nil {
			if id, ok := def.Name.Func.(*ast.IdentExpr); ok {
				name = id.Value
			}
			if def.Name.Method != "" {
				name = def.Name.Method
			}
		}
		if name == "" {
			name = functionNames[origin.Func]
		}
		if name == "" {
			continue
		}
		names[origin.Func], byName[name] = name, origin.Func
	}

	var out []NativeContract
	// An entry layout is an immutable property of the lowered lexical body. It
	// is reusable because it has no caller values in it.
	for _, fn := range bindings.Functions() {
		if fn == nil {
			continue
		}
		out = append(out, NativeContract{Family: "function_entry", Value: functionEntryValue(fn)})
	}

	var calls []callEdge
	var walkExpr func(*ast.FunctionExpr, ast.Expr)
	var walkStmts func(*ast.FunctionExpr, []ast.Stmt)
	walkExpr = func(owner *ast.FunctionExpr, expr ast.Expr) {
		switch x := expr.(type) {
		case *ast.FuncCallExpr:
			calls = append(calls, callEdge{owner: owner, call: x, target: callTargetName(x)})
			walkExpr(owner, x.Func)
			walkExpr(owner, x.Receiver)
			for _, a := range x.Args {
				walkExpr(owner, a)
			}
		case *ast.AttrGetExpr:
			walkExpr(owner, x.Object)
			walkExpr(owner, x.Key)
		case *ast.TableExpr:
			for _, f := range x.Fields {
				if f != nil {
					walkExpr(owner, f.Key)
					walkExpr(owner, f.Value)
				}
			}
		case *ast.LogicalOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.RelationalOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.StringConcatOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.ArithmeticOpExpr:
			walkExpr(owner, x.Lhs)
			walkExpr(owner, x.Rhs)
		case *ast.UnaryMinusOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.UnaryNotOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.UnaryLenOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.UnaryBNotOpExpr:
			walkExpr(owner, x.Expr)
		case *ast.FunctionExpr:
			walkStmts(x, x.Stmts)
		}
	}
	walkStmts = func(owner *ast.FunctionExpr, body []ast.Stmt) {
		for _, stmt := range body {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				for _, x := range s.Lhs {
					walkExpr(owner, x)
				}
				for _, x := range s.Rhs {
					walkExpr(owner, x)
				}
			case *ast.LocalAssignStmt:
				for _, x := range s.Exprs {
					walkExpr(owner, x)
				}
			case *ast.FuncDefStmt:
				if s.Func != nil {
					walkStmts(s.Func, s.Func.Stmts)
				}
			case *ast.FuncCallStmt:
				walkExpr(owner, s.Expr)
			case *ast.ReturnStmt:
				for _, x := range s.Exprs {
					walkExpr(owner, x)
				}
			case *ast.IfStmt:
				walkExpr(owner, s.Condition)
				walkStmts(owner, s.Then)
				walkStmts(owner, s.Else)
			case *ast.DoBlockStmt:
				walkStmts(owner, s.Stmts)
			case *ast.WhileStmt:
				walkExpr(owner, s.Condition)
				walkStmts(owner, s.Stmts)
			case *ast.RepeatStmt:
				walkStmts(owner, s.Stmts)
				walkExpr(owner, s.Condition)
			case *ast.NumberForStmt:
				walkExpr(owner, s.Init)
				walkExpr(owner, s.Limit)
				walkExpr(owner, s.Step)
				walkStmts(owner, s.Stmts)
			case *ast.GenericForStmt:
				for _, x := range s.Exprs {
					walkExpr(owner, x)
				}
				walkStmts(owner, s.Stmts)
			}
		}
	}
	walkStmts(nil, stmts)

	stable := make(map[*ast.FuncCallExpr]bool)
	for _, closure := range bindings.LocalFunctionUseClosures() {
		if !closure.CallSetComplete {
			continue
		}
		for _, call := range closure.DirectCalls {
			stable[call] = true
		}
	}
	hasRequire := false
	for _, edge := range calls {
		hasRequire = hasRequire || edge.target == "require"
	}
	for _, edge := range calls {
		if edge.call == nil {
			continue
		}
		if edge.target == "require" {
			continue
		}
		if _, member := edge.call.Func.(*ast.AttrGetExpr); member && !isLiteralMemberCall(edge.call, literalMembers) {
			continue
		}
		value, revocations := "completeness=unknown", []string(nil)
		if stable[edge.call] && !functionHasParameterCall(byName[edge.target]) {
			value = "cardinality=1 completeness=complete"
		} else if edge.owner != nil && isTwoTargetLocal(edge.owner, edge.call) {
			value, revocations = "cardinality=2 completeness=incomplete", []string{"write.local"}
		} else if edge.owner != nil && isParameterCall(edge.owner, edge.call) {
			revocations = []string{"escape"}
		} else if hasRequire {
			revocations = []string{"write.field", "load.dynamic"}
		}
		// A literal table member is sealed by the same constructor fact that
		// made the member closure available.  Other table paths stay unknown.
		if isLiteralMemberCall(edge.call, literalMembers) {
			value, revocations = "cardinality=1 completeness=complete", []string{"write.field", "meta.set"}
		}
		out = append(out, NativeContract{Family: "callee_set", Value: value, Revocations: revocations})
	}

	// Closed recursion is a graph property of the binder call edges.  A dynamic
	// member target intentionally does not enter this graph.
	adj := make(map[string]map[string]bool)
	for _, edge := range calls {
		from := names[edge.owner]
		if from != "" && byName[edge.target] != nil {
			if adj[from] == nil {
				adj[from] = map[string]bool{}
			}
			adj[from][edge.target] = true
		}
	}
	for _, component := range nativeSCCs(adj) {
		if len(component) == 1 && !adj[component[0]][component[0]] {
			continue
		}
		edges := make([]string, 0)
		for _, from := range component {
			for to := range adj[from] {
				if containsName(component, to) {
					edges = append(edges, from+"->"+to)
				}
			}
		}
		sort.Strings(edges)
		fn := byName[component[0]]
		args := "[]"
		result := "{'exact': True, 'count': 1}"
		if fn != nil && fn.ParList != nil && len(fn.ParList.Types) > 0 {
			args = "[" + typeName(fn.ParList.Types[0]) + "]"
		}
		value := fmt.Sprintf("arguments=%s completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': ['normal', 'throw']} edges_closed=[%s] members=[%s] results=%s", args, strings.Join(edges, ","), strings.Join(component, ","), result)
		revocations := []string(nil)
		if len(component) > 1 {
			revocations = []string{"write.local"}
		}
		out = append(out, NativeContract{Family: "call_scc", Value: value, Revocations: revocations})
	}
	return out
}

type callEdge struct {
	owner  *ast.FunctionExpr
	call   *ast.FuncCallExpr
	target string
}

func callTargetName(c *ast.FuncCallExpr) string {
	if c == nil {
		return ""
	}
	if x, ok := c.Func.(*ast.IdentExpr); ok {
		return x.Value
	}
	if x, ok := c.Func.(*ast.AttrGetExpr); ok {
		return ast.KeyName(x.Key)
	}
	return ""
}
func isLiteralMemberCall(c *ast.FuncCallExpr, members map[string]bool) bool {
	x, ok := c.Func.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	id, ok := x.Object.(*ast.IdentExpr)
	return ok && members[id.Value+"."+ast.KeyName(x.Key)]
}
func isParameterCall(fn *ast.FunctionExpr, c *ast.FuncCallExpr) bool {
	id, ok := c.Func.(*ast.IdentExpr)
	if !ok || fn == nil || fn.ParList == nil {
		return false
	}
	for _, name := range fn.ParList.Names {
		if name == id.Value {
			return true
		}
	}
	return false
}
func functionHasParameterCall(fn *ast.FunctionExpr) bool {
	if fn == nil {
		return false
	}
	var found bool
	var walk func([]ast.Stmt)
	walk = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.ReturnStmt:
				for _, expr := range item.Exprs {
					if call, ok := expr.(*ast.FuncCallExpr); ok && isParameterCall(fn, call) {
						found = true
					}
				}
			case *ast.FuncCallStmt:
				if call, ok := item.Expr.(*ast.FuncCallExpr); ok && isParameterCall(fn, call) {
					found = true
				}
			case *ast.IfStmt:
				walk(item.Then)
				walk(item.Else)
			}
		}
	}
	walk(fn.Stmts)
	return found
}
func isTwoTargetLocal(fn *ast.FunctionExpr, c *ast.FuncCallExpr) bool {
	id, ok := c.Func.(*ast.IdentExpr)
	if !ok || fn == nil {
		return false
	}
	seen := map[string]bool{}
	var walk func([]ast.Stmt)
	walk = func(body []ast.Stmt) {
		for _, s := range body {
			switch x := s.(type) {
			case *ast.LocalAssignStmt:
				for i, n := range x.Names {
					if n == id.Value && i < len(x.Exprs) {
						if v, ok := x.Exprs[i].(*ast.IdentExpr); ok {
							seen[v.Value] = true
						}
					}
				}
			case *ast.AssignStmt:
				for i, left := range x.Lhs {
					if v, ok := left.(*ast.IdentExpr); ok && v.Value == id.Value && i < len(x.Rhs) {
						if r, ok := x.Rhs[i].(*ast.IdentExpr); ok {
							seen[r.Value] = true
						}
					}
				}
			case *ast.IfStmt:
				walk(x.Then)
				walk(x.Else)
			}
		}
	}
	walk(fn.Stmts)
	return len(seen) == 2
}
func functionEntryValue(fn *ast.FunctionExpr) string {
	params := "{'exact': True, 'count': 0}"
	if fn.ParList != nil {
		if fn.ParList.HasVargs {
			params = fmt.Sprintf("{'exact': False, 'prefix': %d, 'open_tail': True}", len(fn.ParList.Names))
		} else {
			params = fmt.Sprintf("{'exact': True, 'count': %d}", len(fn.ParList.Names))
		}
	}
	present := "['normal']"
	if functionCallsNamed(fn.Stmts, "error") {
		present = "['normal', 'throw']"
	}
	results := functionResults(fn)
	return fmt.Sprintf("params=%s completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': %s} results=%s", params, present, results)
}
func functionResults(fn *ast.FunctionExpr) string {
	var returns []*ast.ReturnStmt
	var walk func([]ast.Stmt)
	walk = func(body []ast.Stmt) {
		for _, s := range body {
			switch x := s.(type) {
			case *ast.ReturnStmt:
				returns = append(returns, x)
			case *ast.IfStmt:
				walk(x.Then)
				walk(x.Else)
			case *ast.DoBlockStmt:
				walk(x.Stmts)
			}
		}
	}
	walk(fn.Stmts)
	if len(returns) == 0 {
		return "{'exact': True, 'count': 0}"
	}
	for _, r := range returns {
		if len(r.Exprs) == 1 {
			switch r.Exprs[0].(type) {
			case *ast.Comma3Expr, *ast.FuncCallExpr:
				return "{'exact': False, 'prefix': 0, 'open_tail': True}"
			}
		}
	}
	return fmt.Sprintf("{'exact': True, 'count': %d}", len(returns[0].Exprs))
}
func functionCallsNamed(body []ast.Stmt, name string) bool {
	found := false
	var expr func(ast.Expr)
	var stmts func([]ast.Stmt)
	expr = func(e ast.Expr) {
		if c, ok := e.(*ast.FuncCallExpr); ok {
			if callTargetName(c) == name {
				found = true
			}
			expr(c.Func)
			for _, a := range c.Args {
				expr(a)
			}
		}
	}
	stmts = func(xs []ast.Stmt) {
		for _, s := range xs {
			switch x := s.(type) {
			case *ast.FuncCallStmt:
				expr(x.Expr)
			case *ast.ReturnStmt:
				for _, e := range x.Exprs {
					expr(e)
				}
			case *ast.IfStmt:
				stmts(x.Then)
				stmts(x.Else)
			}
		}
	}
	stmts(body)
	return found
}
func typeName(t ast.TypeExpr) string {
	if p, ok := t.(*ast.PrimitiveTypeExpr); ok {
		return p.Name
	}
	return "unknown"
}
func containsName(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
func nativeSCCs(adj map[string]map[string]bool) [][]string {
	var out [][]string
	index := 0
	indices, low := map[string]int{}, map[string]int{}
	stack := []string{}
	on := map[string]bool{}
	var visit func(string)
	visit = func(v string) {
		index++
		indices[v], low[v] = index, index
		stack = append(stack, v)
		on[v] = true
		for w := range adj[v] {
			if indices[w] == 0 {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if on[w] && indices[w] < low[v] {
				low[v] = indices[w]
			}
		}
		if low[v] == indices[v] {
			var c []string
			for {
				n := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[n] = false
				c = append(c, n)
				if n == v {
					break
				}
			}
			sort.Strings(c)
			out = append(out, c)
		}
	}
	keys := make([]string, 0, len(adj))
	for k := range adj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if indices[k] == 0 {
			visit(k)
		}
	}
	return out
}
