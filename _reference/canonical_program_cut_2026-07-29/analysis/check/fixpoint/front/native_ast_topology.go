package front

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/compiler/ast"
)

// nativeASTTopologyDrafts retains only binder-owned lexical coordinates that
// WIR does not yet spell: authored return arities and bound callee alternatives.
// Semantic entry/callee/effect rows are produced by publication kernels.
func nativeASTTopologyDrafts(root Compilation, stmts []ast.Stmt, bindings *bind.Result) []NativeTopologyDraft {
	if bindings == nil {
		return nil
	}
	body := NativeBodyReference{Body: [32]byte(root.Body)}
	byName := nativeASTFunctionNames(stmts, bindings)
	literalMembers := nativeASTLiteralMembers(stmts)
	var drafts []NativeTopologyDraft
	for _, fn := range bindings.Functions() {
		if fn == nil {
			continue
		}
		entry := nativeFunctionEntryTopology(body, fn)
		drafts = append(drafts, NativeTopologyDraft{
			Kind: NativeTopologyFunctionEntry, FunctionEntry: &entry,
		})
	}

	calls := nativeASTCallEdges(stmts)
	stable := make(map[*ast.FuncCallExpr]bool)
	for _, closure := range bindings.LocalFunctionUseClosures() {
		if !closure.CallSetComplete {
			continue
		}
		for _, call := range closure.DirectCalls {
			stable[call] = true
		}
	}
	var moduleLoadSites []uint32
	for index, edge := range calls {
		if signaturelookup.HasStdlibCapability(edge.target, capability.DispatchModuleLoad) {
			moduleLoadSites = append(moduleLoadSites, uint32(index))
		}
	}
	for index, edge := range calls {
		if edge.call == nil ||
			signaturelookup.HasStdlibCapability(edge.target, capability.DispatchModuleLoad) {
			continue
		}
		literalMember := nativeASTLiteralMemberCall(edge.call, literalMembers)
		if _, member := edge.call.Func.(*ast.AttrGetExpr); member && !literalMember {
			continue
		}
		topology := NativeCalleeOpen
		var targets []string
		switch {
		case literalMember:
			topology = NativeCalleeLiteralMember
			targets = []string{edge.target}
		case stable[edge.call] && !nativeASTFunctionHasParameterCall(byName[edge.target]):
			topology = NativeCalleeDirectLexical
			targets = []string{edge.target}
		case edge.owner != nil && nativeASTTwoTargetLocal(edge.owner, edge.call):
			topology = NativeCalleeLocalAlternatives
			targets = nativeASTLocalTargets(edge.owner, edge.call)
		case edge.owner != nil && nativeASTParameterCall(edge.owner, edge.call):
			topology = NativeCalleeParameter
		}
		draft := NativeCalleeTopologyDraft{
			Body: body, Site: uint32(index), Topology: topology,
			TargetSymbols: targets, ModuleLoadSites: append([]uint32(nil), moduleLoadSites...),
		}
		drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyCallee, Callee: &draft})
	}
	drafts = append(drafts, nativeOperationTopologyDrafts(body, stmts, bindings)...)
	return drafts
}

func nativeFunctionEntryTopology(body NativeBodyReference, fn *ast.FunctionExpr) NativeFunctionEntryDraft {
	draft := NativeFunctionEntryDraft{Body: body}
	if fn == nil {
		return draft
	}
	if fn.ParList != nil {
		draft.Parameters = uint32(len(fn.ParList.Names))
		if fn.ParList.HasVargs {
			draft.Varargs = 1
		}
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch item := stmt.(type) {
			case *ast.ReturnStmt:
				row := NativeReturnShapeDraft{Slots: uint32(len(item.Exprs))}
				if len(item.Exprs) == 1 {
					switch item.Exprs[0].(type) {
					case *ast.Comma3Expr, *ast.FuncCallExpr:
						row.OpenTail = 1
					}
				}
				draft.Returns = append(draft.Returns, row)
			case *ast.IfStmt:
				walk(item.Then)
				walk(item.Else)
			case *ast.DoBlockStmt:
				walk(item.Stmts)
			}
		}
	}
	walk(fn.Stmts)
	if nativeASTFunctionCallsNamed(fn.Stmts, "error") {
		draft.ErrorCalls = []uint32{0}
	}
	return draft
}

type nativeASTCallEdge struct {
	owner  *ast.FunctionExpr
	call   *ast.FuncCallExpr
	target string
}

func nativeASTCallEdges(stmts []ast.Stmt) []nativeASTCallEdge {
	var calls []nativeASTCallEdge
	var walkExpr func(*ast.FunctionExpr, ast.Expr)
	var walkStmts func(*ast.FunctionExpr, []ast.Stmt)
	walkExpr = func(owner *ast.FunctionExpr, expr ast.Expr) {
		switch item := expr.(type) {
		case *ast.FuncCallExpr:
			calls = append(calls, nativeASTCallEdge{owner: owner, call: item, target: nativeASTCallTarget(item)})
			walkExpr(owner, item.Func)
			walkExpr(owner, item.Receiver)
			for _, argument := range item.Args {
				walkExpr(owner, argument)
			}
		case *ast.AttrGetExpr:
			walkExpr(owner, item.Object)
			walkExpr(owner, item.Key)
		case *ast.TableExpr:
			for _, field := range item.Fields {
				if field != nil {
					walkExpr(owner, field.Key)
					walkExpr(owner, field.Value)
				}
			}
		case *ast.LogicalOpExpr:
			walkExpr(owner, item.Lhs)
			walkExpr(owner, item.Rhs)
		case *ast.RelationalOpExpr:
			walkExpr(owner, item.Lhs)
			walkExpr(owner, item.Rhs)
		case *ast.StringConcatOpExpr:
			walkExpr(owner, item.Lhs)
			walkExpr(owner, item.Rhs)
		case *ast.ArithmeticOpExpr:
			walkExpr(owner, item.Lhs)
			walkExpr(owner, item.Rhs)
		case *ast.UnaryMinusOpExpr:
			walkExpr(owner, item.Expr)
		case *ast.UnaryNotOpExpr:
			walkExpr(owner, item.Expr)
		case *ast.UnaryLenOpExpr:
			walkExpr(owner, item.Expr)
		case *ast.UnaryBNotOpExpr:
			walkExpr(owner, item.Expr)
		case *ast.FunctionExpr:
			walkStmts(item, item.Stmts)
		}
	}
	walkStmts = func(owner *ast.FunctionExpr, body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.AssignStmt:
				for _, expr := range item.Lhs {
					walkExpr(owner, expr)
				}
				for _, expr := range item.Rhs {
					walkExpr(owner, expr)
				}
			case *ast.LocalAssignStmt:
				for _, expr := range item.Exprs {
					walkExpr(owner, expr)
				}
			case *ast.FuncDefStmt:
				if item.Func != nil {
					walkStmts(item.Func, item.Func.Stmts)
				}
			case *ast.FuncCallStmt:
				walkExpr(owner, item.Expr)
			case *ast.ReturnStmt:
				for _, expr := range item.Exprs {
					walkExpr(owner, expr)
				}
			case *ast.IfStmt:
				walkExpr(owner, item.Condition)
				walkStmts(owner, item.Then)
				walkStmts(owner, item.Else)
			case *ast.DoBlockStmt:
				walkStmts(owner, item.Stmts)
			case *ast.WhileStmt:
				walkExpr(owner, item.Condition)
				walkStmts(owner, item.Stmts)
			case *ast.RepeatStmt:
				walkStmts(owner, item.Stmts)
				walkExpr(owner, item.Condition)
			case *ast.NumberForStmt:
				walkExpr(owner, item.Init)
				walkExpr(owner, item.Limit)
				walkExpr(owner, item.Step)
				walkStmts(owner, item.Stmts)
			case *ast.GenericForStmt:
				for _, expr := range item.Exprs {
					walkExpr(owner, expr)
				}
				walkStmts(owner, item.Stmts)
			}
		}
	}
	walkStmts(nil, stmts)
	return calls
}

func nativeASTFunctionNames(stmts []ast.Stmt, bindings *bind.Result) map[string]*ast.FunctionExpr {
	functionNames := make(map[*ast.FunctionExpr]string)
	var label func([]ast.Stmt)
	label = func(body []ast.Stmt) {
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
					label(item.Func.Stmts)
				}
			case *ast.LocalAssignStmt:
				for index, expr := range item.Exprs {
					if index < len(item.Names) {
						if fn, ok := expr.(*ast.FunctionExpr); ok {
							functionNames[fn] = item.Names[index]
							label(fn.Stmts)
						}
					}
				}
			case *ast.AssignStmt:
				for index, expr := range item.Rhs {
					if index < len(item.Lhs) {
						if fn, ok := expr.(*ast.FunctionExpr); ok {
							if id, ok := item.Lhs[index].(*ast.IdentExpr); ok {
								functionNames[fn] = id.Value
								label(fn.Stmts)
							}
						}
					}
				}
			case *ast.IfStmt:
				label(item.Then)
				label(item.Else)
			case *ast.DoBlockStmt:
				label(item.Stmts)
			}
		}
	}
	label(stmts)
	byName := make(map[string]*ast.FunctionExpr)
	for _, origin := range bindings.FunctionOrigins() {
		name := ""
		if origin.HasTargetSymbol {
			name = bindings.Name(origin.TargetSymbol)
		}
		if definition, ok := origin.Stmt.(*ast.FuncDefStmt); ok && definition != nil && definition.Name != nil {
			if id, ok := definition.Name.Func.(*ast.IdentExpr); ok {
				name = id.Value
			}
			if definition.Name.Method != "" {
				name = definition.Name.Method
			}
		}
		if name == "" {
			name = functionNames[origin.Func]
		}
		if name != "" {
			byName[name] = origin.Func
		}
	}
	return byName
}

func nativeASTLiteralMembers(stmts []ast.Stmt) map[string]bool {
	out := make(map[string]bool)
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
				if field != nil && ast.KeyName(field.Key) != "" {
					if _, callable := field.Value.(*ast.FunctionExpr); callable {
						out[name+"."+ast.KeyName(field.Key)] = true
					}
				}
			}
		}
	}
	return out
}

func nativeASTCallTarget(call *ast.FuncCallExpr) string {
	if call == nil {
		return ""
	}
	if item, ok := call.Func.(*ast.IdentExpr); ok {
		return item.Value
	}
	if item, ok := call.Func.(*ast.AttrGetExpr); ok {
		return ast.KeyName(item.Key)
	}
	return ""
}

func nativeASTLiteralMemberCall(call *ast.FuncCallExpr, members map[string]bool) bool {
	item, ok := call.Func.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	id, ok := item.Object.(*ast.IdentExpr)
	return ok && members[id.Value+"."+ast.KeyName(item.Key)]
}

func nativeASTParameterCall(fn *ast.FunctionExpr, call *ast.FuncCallExpr) bool {
	id, ok := call.Func.(*ast.IdentExpr)
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

func nativeASTFunctionHasParameterCall(fn *ast.FunctionExpr) bool {
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
					if call, ok := expr.(*ast.FuncCallExpr); ok && nativeASTParameterCall(fn, call) {
						found = true
					}
				}
			case *ast.FuncCallStmt:
				if call, ok := item.Expr.(*ast.FuncCallExpr); ok && nativeASTParameterCall(fn, call) {
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

func nativeASTTwoTargetLocal(fn *ast.FunctionExpr, call *ast.FuncCallExpr) bool {
	return len(nativeASTLocalTargets(fn, call)) == 2
}

func nativeASTLocalTargets(fn *ast.FunctionExpr, call *ast.FuncCallExpr) []string {
	id, ok := call.Func.(*ast.IdentExpr)
	if !ok || fn == nil {
		return nil
	}
	seen := make(map[string]bool)
	var walk func([]ast.Stmt)
	walk = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.LocalAssignStmt:
				for index, name := range item.Names {
					if name == id.Value && index < len(item.Exprs) {
						if value, ok := item.Exprs[index].(*ast.IdentExpr); ok {
							seen[value.Value] = true
						}
					}
				}
			case *ast.AssignStmt:
				for index, left := range item.Lhs {
					if value, ok := left.(*ast.IdentExpr); ok && value.Value == id.Value && index < len(item.Rhs) {
						if right, ok := item.Rhs[index].(*ast.IdentExpr); ok {
							seen[right.Value] = true
						}
					}
				}
			case *ast.IfStmt:
				walk(item.Then)
				walk(item.Else)
			}
		}
	}
	walk(fn.Stmts)
	out := make([]string, 0, len(seen))
	for target := range seen {
		out = append(out, target)
	}
	return out
}

func nativeASTFunctionCallsNamed(body []ast.Stmt, name string) bool {
	found := false
	var expr func(ast.Expr)
	var stmts func([]ast.Stmt)
	expr = func(value ast.Expr) {
		if call, ok := value.(*ast.FuncCallExpr); ok {
			if nativeASTCallTarget(call) == name {
				found = true
			}
			expr(call.Func)
			for _, argument := range call.Args {
				expr(argument)
			}
		}
	}
	stmts = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.FuncCallStmt:
				expr(item.Expr)
			case *ast.ReturnStmt:
				for _, value := range item.Exprs {
					expr(value)
				}
			case *ast.IfStmt:
				stmts(item.Then)
				stmts(item.Else)
			}
		}
	}
	stmts(body)
	return found
}
