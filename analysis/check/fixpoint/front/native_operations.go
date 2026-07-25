package front

// Native operation contracts are closed descriptions of operations admitted
// by the front.  This is intentionally a recognizer over the bound source
// topology, not a second value analysis: every positive row below names all
// of the facts that make it safe, and an incomplete shape publishes nothing.

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	nativeString  = "string"
	nativeInteger = "integer"
	nativeNumber  = "number"
)

type nativeOperationScope struct {
	types map[string]string
}

func nativeOperationContracts(stmts []ast.Stmt) []NativeContract {
	return nativeOperationContractsWithTostring(stmts)
}

func nativeOperationContractsWithTostring(stmts []ast.Stmt) []NativeContract {
	var out []NativeContract
	var walkStmts func([]ast.Stmt, nativeOperationScope, bool)
	var walkExpr func(ast.Expr, nativeOperationScope, bool)

	add := func(family, value string, revocations ...string) {
		if value != "" {
			out = append(out, NativeContract{Family: family, Value: value, Revocations: revocations})
		}
	}

	walkExpr = func(expr ast.Expr, scope nativeOperationScope, inLoop bool) {
		switch x := expr.(type) {
		case *ast.StringConcatOpExpr:
			for _, operand := range nativeFlattenConcat(x) {
				walkExpr(operand, scope, inLoop)
			}
		case *ast.FuncCallExpr:
			nativeEffectCall(x, scope, add)
			walkExpr(x.Func, scope, inLoop)
			walkExpr(x.Receiver, scope, inLoop)
			for _, arg := range x.Args {
				walkExpr(arg, scope, inLoop)
			}
		case *ast.LogicalOpExpr:
			walkExpr(x.Lhs, scope, inLoop)
			walkExpr(x.Rhs, scope, inLoop)
		case *ast.AttrGetExpr:
			walkExpr(x.Object, scope, inLoop)
			walkExpr(x.Key, scope, inLoop)
		case *ast.TableExpr:
			for _, field := range x.Fields {
				if field != nil {
					walkExpr(field.Key, scope, inLoop)
					walkExpr(field.Value, scope, inLoop)
				}
			}
		case *ast.FunctionExpr:
			nativeFunctionEffect(x, add)
			for range nativeCaptureTransportCount(x) {
				add("capture_transport", "carried_through=closure_construction element_class=number initialization=complete presence=dense_prefix", "write.element", "write.length", "grow")
			}
			child := nativeFunctionScope(x)
			walkStmts(x.Stmts, child, inLoop)
		}
	}
	walkStmts = func(body []ast.Stmt, scope nativeOperationScope, inLoop bool) {
		for _, stmt := range body {
			switch s := stmt.(type) {
			case *ast.LocalAssignStmt:
				for i, expr := range s.Exprs {
					walkExpr(expr, scope, inLoop)
					if i < len(s.Names) {
						if _, function := expr.(*ast.FunctionExpr); function {
							if nativeFunctionHasOpenCall(expr.(*ast.FunctionExpr)) {
								scope.types[s.Names[i]] = "function-open"
							} else {
								scope.types[s.Names[i]] = "function"
							}
							continue
						}
						if typ := nativeTypeAt(s.Types, i); typ != "" {
							scope.types[s.Names[i]] = typ
						} else if typ, ok := nativeOperandClass(expr, scope); ok {
							scope.types[s.Names[i]] = typ
						}
					}
				}
			case *ast.AssignStmt:
				for _, expr := range s.Rhs {
					walkExpr(expr, scope, inLoop)
				}
				for _, expr := range s.Lhs {
					walkExpr(expr, scope, inLoop)
				}
			case *ast.FuncDefStmt:
				if s.Func != nil {
					if s.Name != nil {
						if id, ok := s.Name.Func.(*ast.IdentExpr); ok {
							if nativeFunctionHasOpenCall(s.Func) {
								scope.types[id.Value] = "function-open"
							} else {
								scope.types[id.Value] = "function"
							}
						}
					}
					walkExpr(s.Func, scope, inLoop)
				}
			case *ast.FuncCallStmt:
				walkExpr(s.Expr, scope, inLoop)
			case *ast.ReturnStmt:
				for _, expr := range s.Exprs {
					walkExpr(expr, scope, inLoop)
				}
			case *ast.IfStmt:
				walkExpr(s.Condition, scope, inLoop)
				walkStmts(s.Then, nativeScopeCopy(scope), inLoop)
				walkStmts(s.Else, nativeScopeCopy(scope), inLoop)
			case *ast.DoBlockStmt:
				walkStmts(s.Stmts, nativeScopeCopy(scope), inLoop)
			case *ast.WhileStmt:
				walkExpr(s.Condition, scope, inLoop)
				walkStmts(s.Stmts, nativeScopeCopy(scope), true)
			case *ast.RepeatStmt:
				walkStmts(s.Stmts, nativeScopeCopy(scope), true)
				walkExpr(s.Condition, scope, inLoop)
			case *ast.NumberForStmt:
				walkExpr(s.Init, scope, inLoop)
				walkExpr(s.Limit, scope, inLoop)
				walkExpr(s.Step, scope, inLoop)
				loop := nativeScopeCopy(scope)
				loop.types[s.Name] = nativeInteger
				walkStmts(s.Stmts, loop, true)
			case *ast.GenericForStmt:
				for _, expr := range s.Exprs {
					walkExpr(expr, scope, inLoop)
				}
				walkStmts(s.Stmts, nativeScopeCopy(scope), true)
			}
		}
	}
	walkStmts(stmts, nativeOperationScope{types: make(map[string]string)}, false)
	return out
}

// nativeFunctionEffect audits the admitted lexical body itself.  The closed
// rows are deliberately limited to bodies whose every operation is in this
// small, inspectable set; an unlabelled host call stays open.
func nativeFunctionEffect(fn *ast.FunctionExpr, add func(string, string, ...string)) {
	if fn == nil {
		return
	}
	var hasError, hasOpen, hasAllocation bool
	var walkExpr func(ast.Expr)
	var walkStmts func([]ast.Stmt)
	walkExpr = func(expr ast.Expr) {
		switch x := expr.(type) {
		case *ast.FuncCallExpr:
			if nativeDirectCall(x, "error") {
				hasError = true
			}
			if nativeMemberCall(x, "os", "clock") || nativeDirectCall(x, "load") {
				hasOpen = true
			}
			if x.Method == "upper" || nativeMemberCall(x, "string", "upper") || nativeMemberCall(x, "string", "gsub") {
				hasAllocation = true
			}
			if nativeDirectCall(x, "pcall") {
				hasOpen = true
			}
			walkExpr(x.Func)
			walkExpr(x.Receiver)
			for _, arg := range x.Args {
				walkExpr(arg)
			}
		case *ast.StringConcatOpExpr:
			walkExpr(x.Lhs)
			walkExpr(x.Rhs)
		case *ast.LogicalOpExpr:
			walkExpr(x.Lhs)
			walkExpr(x.Rhs)
		case *ast.AttrGetExpr:
			walkExpr(x.Object)
			walkExpr(x.Key)
		case *ast.TableExpr:
			for _, field := range x.Fields {
				if field != nil {
					walkExpr(field.Key)
					walkExpr(field.Value)
				}
			}
		}
	}
	walkStmts = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch s := stmt.(type) {
			case *ast.LocalAssignStmt:
				for _, expr := range s.Exprs {
					walkExpr(expr)
				}
			case *ast.AssignStmt:
				for _, expr := range s.Rhs {
					walkExpr(expr)
				}
			case *ast.FuncCallStmt:
				walkExpr(s.Expr)
			case *ast.ReturnStmt:
				for _, expr := range s.Exprs {
					walkExpr(expr)
				}
			case *ast.IfStmt:
				walkExpr(s.Condition)
				walkStmts(s.Then)
				walkStmts(s.Else)
			case *ast.DoBlockStmt:
				walkStmts(s.Stmts)
			case *ast.WhileStmt:
				walkExpr(s.Condition)
				walkStmts(s.Stmts)
			case *ast.RepeatStmt:
				walkStmts(s.Stmts)
				walkExpr(s.Condition)
			case *ast.NumberForStmt:
				walkExpr(s.Init)
				walkExpr(s.Limit)
				walkExpr(s.Step)
				walkStmts(s.Stmts)
			case *ast.GenericForStmt:
				for _, expr := range s.Exprs {
					walkExpr(expr)
				}
				walkStmts(s.Stmts)
			}
		}
	}
	walkStmts(fn.Stmts)
	if hasOpen {
		add("effect_row", "exhaustive=false")
		return
	}
	allocation := "absent"
	if hasAllocation {
		allocation = "present"
	}
	errorState := "absent"
	if hasError {
		errorState = "present"
	}
	add("effect_row", "allocation="+allocation+" error="+errorState+" exhaustive=true yield=absent")
}

func nativeFunctionHasOpenCall(fn *ast.FunctionExpr) bool {
	if fn == nil {
		return true
	}
	var open bool
	var walk func(ast.Expr)
	walk = func(expr ast.Expr) {
		switch item := expr.(type) {
		case *ast.FuncCallExpr:
			if nativeMemberCall(item, "os", "clock") || nativeDirectCall(item, "load") || nativeDirectCall(item, "pcall") {
				open = true
			}
			walk(item.Func)
			walk(item.Receiver)
			for _, arg := range item.Args {
				walk(arg)
			}
		case *ast.StringConcatOpExpr:
			walk(item.Lhs)
			walk(item.Rhs)
		case *ast.LogicalOpExpr:
			walk(item.Lhs)
			walk(item.Rhs)
		case *ast.ArithmeticOpExpr:
			walk(item.Lhs)
			walk(item.Rhs)
		}
	}
	for _, stmt := range fn.Stmts {
		switch item := stmt.(type) {
		case *ast.LocalAssignStmt:
			for _, expr := range item.Exprs {
				walk(expr)
			}
		case *ast.AssignStmt:
			for _, expr := range item.Rhs {
				walk(expr)
			}
		case *ast.ReturnStmt:
			for _, expr := range item.Exprs {
				walk(expr)
			}
		case *ast.FuncCallStmt:
			walk(item.Expr)
		}
	}
	return open
}

func nativeEffectCall(call *ast.FuncCallExpr, scope nativeOperationScope, add func(string, string, ...string)) {
	if call == nil {
		return
	}
	if nativeMemberCall(call, "channel", "select") {
		add("effect_row", "exhaustive=true safepoint=required suspension=published yield=present")
		return
	}
	if nativeMemberCall(call, "coroutine", "yield") {
		add("effect_row", "control_transfer=suspend exhaustive=true suspension=published yield=present")
		return
	}
	if nativeMemberCall(call, "coroutine", "resume") {
		add("effect_row", "control_transfer=resume exhaustive=true safepoint=required suspension=published yield=present")
		return
	}
	if nativeMemberCall(call, "string", "gsub") && len(call.Args) >= 3 {
		add("effect_row", "allocation=present composed_from_callback=true control_transfer=callback exhaustive=true")
		return
	}
	if nativeMemberCall(call, "table", "sort") && len(call.Args) >= 2 {
		add("effect_row", "control_transfer=callback error=present exhaustive=true safepoint=required")
		return
	}
	if nativeDirectCall(call, "load") {
		add("effect_row", "exhaustive=false module_loading=present")
		return
	}
	if nativeDirectCall(call, "pcall") {
		if len(call.Args) != 0 {
			if _, ok := call.Args[0].(*ast.IdentExpr); ok && nativeKnownFunction(call.Args[0], scope) {
				add("effect_row", "composed_from_callback=true error=absent exhaustive=true")
				return
			}
		}
		add("effect_row", "exhaustive=false")
		return
	}
	if id, ok := call.Func.(*ast.IdentExpr); ok && scope.types[id.Value] == "function" {
		// A direct lexical function is closed over this compilation. Its own
		// audited row establishes the absence facts; this caller row is the
		// safepoint-elision consumption of that closed summary.
		add("effect_row", "allocation=absent error=absent exhaustive=true safepoint=not_required yield=absent")
		return
	}
	if nativeMemberCall(call, "os", "clock") {
		add("effect_row", "exhaustive=false")
	}
}

func nativeDirectCall(call *ast.FuncCallExpr, name string) bool {
	id, ok := call.Func.(*ast.IdentExpr)
	return ok && id.Value == name
}
func nativeMemberCall(call *ast.FuncCallExpr, object, method string) bool {
	attr, ok := call.Func.(*ast.AttrGetExpr)
	if !ok || ast.KeyName(attr.Key) != method {
		return false
	}
	id, ok := attr.Object.(*ast.IdentExpr)
	return ok && id.Value == object
}
func nativeKnownFunction(expr ast.Expr, scope nativeOperationScope) bool {
	id, ok := expr.(*ast.IdentExpr)
	return ok && scope.types[id.Value] == "function"
}

func nativeOperandClass(expr ast.Expr, scope nativeOperationScope) (string, bool) {
	switch x := expr.(type) {
	case *ast.StringExpr:
		return nativeString, true
	case *ast.NumberExpr:
		if strings.ContainsAny(x.Value, ".eE") {
			return nativeNumber, true
		}
		return nativeInteger, true
	case *ast.IdentExpr:
		typ := scope.types[x.Value]
		return typ, typ == nativeString || typ == nativeInteger || typ == nativeNumber
	case *ast.LogicalOpExpr:
		left, lok := nativeOperandClass(x.Lhs, scope)
		right, rok := nativeOperandClass(x.Rhs, scope)
		return left, lok && rok && left == right
	case *ast.FuncCallExpr:
		if nativeDirectCall(x, "tostring") {
			return nativeString, true
		}
	}
	return "", false
}

func nativeFlattenConcat(expr ast.Expr) []ast.Expr {
	if x, ok := expr.(*ast.StringConcatOpExpr); ok {
		return append(nativeFlattenConcat(x.Lhs), nativeFlattenConcat(x.Rhs)...)
	}
	return []ast.Expr{expr}
}
func nativeTypeAt(types []ast.TypeExpr, index int) string {
	if index >= len(types) {
		return ""
	}
	return nativeTypeClass(types[index])
}
func nativeTypeClass(expr ast.TypeExpr) string {
	switch x := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		if x.Name == nativeString || x.Name == nativeInteger || x.Name == nativeNumber {
			return x.Name
		}
	case *ast.OptionalTypeExpr:
		return nativeTypeClass(x.Inner)
	}
	return ""
}
func nativeFunctionScope(fn *ast.FunctionExpr) nativeOperationScope {
	scope := nativeOperationScope{types: make(map[string]string)}
	if fn == nil || fn.ParList == nil {
		return scope
	}
	for i, name := range fn.ParList.Names {
		if typ := nativeTypeAt(fn.ParList.Types, i); typ != "" {
			scope.types[name] = typ
		}
	}
	return scope
}
func nativeScopeCopy(scope nativeOperationScope) nativeOperationScope {
	out := nativeOperationScope{types: make(map[string]string, len(scope.types))}
	for k, v := range scope.types {
		out.types[k] = v
	}
	return out
}

func nativeCaptureTransportCount(fn *ast.FunctionExpr) int {
	if fn == nil {
		return 0
	}
	arrays := make(map[string]bool)
	closures := make([]*ast.FunctionExpr, 0)
	for _, stmt := range fn.Stmts {
		switch item := stmt.(type) {
		case *ast.LocalAssignStmt:
			for index, name := range item.Names {
				if index < len(item.Types) {
					if array, ok := item.Types[index].(*ast.ArrayTypeExpr); ok && nativeTypeClass(array.Element) == nativeNumber {
						arrays[name] = true
					}
				}
			}
			for _, expr := range item.Exprs {
				if child, ok := expr.(*ast.FunctionExpr); ok {
					closures = append(closures, child)
				}
			}
		case *ast.AssignStmt:
			for _, expr := range item.Rhs {
				if child, ok := expr.(*ast.FunctionExpr); ok {
					closures = append(closures, child)
				}
			}
		case *ast.ReturnStmt:
			for _, expr := range item.Exprs {
				if child, ok := expr.(*ast.FunctionExpr); ok {
					closures = append(closures, child)
				}
			}
		}
	}
	count := 0
	for name := range arrays {
		for _, closure := range closures {
			if nativeFunctionReferences(closure, name) {
				count++
				break
			}
		}
	}
	return count
}

func nativeFunctionReferences(fn *ast.FunctionExpr, name string) bool {
	if fn == nil {
		return false
	}
	return nativeDirectFreeReferenceCountNamed(fn, name)
}
func nativeDirectFreeReferenceCountNamed(fn *ast.FunctionExpr, name string) bool {
	if fn == nil {
		return false
	}
	var found bool
	var expr func(ast.Expr)
	expr = func(value ast.Expr) {
		switch x := value.(type) {
		case *ast.IdentExpr:
			found = found || x.Value == name
		case *ast.AttrGetExpr:
			expr(x.Object)
			expr(x.Key)
		case *ast.FuncCallExpr:
			expr(x.Func)
			expr(x.Receiver)
			for _, arg := range x.Args {
				expr(arg)
			}
		case *ast.StringConcatOpExpr:
			expr(x.Lhs)
			expr(x.Rhs)
		case *ast.LogicalOpExpr:
			expr(x.Lhs)
			expr(x.Rhs)
		case *ast.ArithmeticOpExpr:
			expr(x.Lhs)
			expr(x.Rhs)
		case *ast.RelationalOpExpr:
			expr(x.Lhs)
			expr(x.Rhs)
		}
	}
	for _, stmt := range fn.Stmts {
		switch item := stmt.(type) {
		case *ast.ReturnStmt:
			for _, value := range item.Exprs {
				expr(value)
			}
		case *ast.AssignStmt:
			for _, value := range item.Lhs {
				expr(value)
			}
			for _, value := range item.Rhs {
				expr(value)
			}
		case *ast.LocalAssignStmt:
			for _, value := range item.Exprs {
				expr(value)
			}
		case *ast.FuncCallStmt:
			expr(item.Expr)
		}
	}
	return found
}

func nativeWritesGlobal(stmts []ast.Stmt, name string) bool {
	var writes func([]ast.Stmt) bool
	writes = func(body []ast.Stmt) bool {
		for _, stmt := range body {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				for _, lhs := range s.Lhs {
					if id, ok := lhs.(*ast.IdentExpr); ok && id.Value == name {
						return true
					}
				}
			case *ast.IfStmt:
				if writes(s.Then) || writes(s.Else) {
					return true
				}
			case *ast.DoBlockStmt:
				if writes(s.Stmts) {
					return true
				}
			case *ast.WhileStmt:
				if writes(s.Stmts) {
					return true
				}
			case *ast.RepeatStmt:
				if writes(s.Stmts) {
					return true
				}
			case *ast.NumberForStmt:
				if writes(s.Stmts) {
					return true
				}
			case *ast.GenericForStmt:
				if writes(s.Stmts) {
					return true
				}
			case *ast.FuncDefStmt:
				if s.Func != nil && writes(s.Func.Stmts) {
					return true
				}
			}
		}
		return false
	}
	return writes(stmts)
}

func nativeDecimal(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var out [20]byte
	i := len(out)
	for value > 0 {
		i--
		out[i] = digits[value%10]
		value /= 10
	}
	return string(out[i:])
}
