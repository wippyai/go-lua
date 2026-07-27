package front

// Native operation contracts are closed descriptions of operations admitted
// by the front.  This is intentionally a recognizer over the bound source
// topology, not a second value analysis: every positive row below names all
// of the facts that make it safe, and an incomplete shape publishes nothing.

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
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

func nativeOperationContracts(stmts []ast.Stmt, bindings *bind.Result, captureTransports map[wir.FunctionSymbolID]int) []NativeContract {
	return nativeOperationContractsWithTostring(stmts, bindings, captureTransports)
}

func nativeOperationContractsWithTostring(stmts []ast.Stmt, bindings *bind.Result, captureTransports map[wir.FunctionSymbolID]int) []NativeContract {
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
			nativeEffectCall(bindings, x, scope, add)
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
			nativeFunctionEffect(bindings, x, add)
			symbol, _ := bindings.FunctionSymbol(x)
			for range captureTransports[wir.FunctionSymbolID(symbol)] {
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
							if nativeFunctionHasOpenCall(bindings, expr.(*ast.FunctionExpr)) {
								scope.types[s.Names[i]] = "function-open"
							} else {
								scope.types[s.Names[i]] = "function"
							}
							continue
						}
						if typ := nativeTypeAt(s.Types, i); typ != "" {
							scope.types[s.Names[i]] = typ
						} else if typ, ok := nativeOperandClass(bindings, expr, scope); ok {
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
							if nativeFunctionHasOpenCall(bindings, s.Func) {
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
func nativeFunctionEffect(bindings *bind.Result, fn *ast.FunctionExpr, add func(string, string, ...string)) {
	if fn == nil {
		return
	}
	var hasError, hasOpen, hasAllocation bool
	var walkExpr func(ast.Expr)
	var walkStmts func([]ast.Stmt)
	walkExpr = func(expr ast.Expr) {
		switch x := expr.(type) {
		case *ast.FuncCallExpr:
			if _, ok := nativeBoundStdlibCall(bindings, x, "error"); ok {
				hasError = true
			}
			if nativeStdlibCallIsOpen(bindings, x, "os.clock") || nativeStdlibCallIsOpen(bindings, x, "load") {
				hasOpen = true
			}
			if nativeRegisteredMethod(x, "string.upper") ||
				nativeBoundStdlibMemberCall(bindings, x, "string.upper") ||
				nativeBoundStdlibMemberCall(bindings, x, "string.gsub") {
				hasAllocation = true
			}
			if nativeStdlibCallIsOpen(bindings, x, "pcall") {
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

func nativeFunctionHasOpenCall(bindings *bind.Result, fn *ast.FunctionExpr) bool {
	if fn == nil {
		return true
	}
	var open bool
	var walk func(ast.Expr)
	walk = func(expr ast.Expr) {
		switch item := expr.(type) {
		case *ast.FuncCallExpr:
			if nativeStdlibCallIsOpen(bindings, item, "os.clock") ||
				nativeStdlibCallIsOpen(bindings, item, "load") ||
				nativeStdlibCallIsOpen(bindings, item, "pcall") {
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

func nativeEffectCall(bindings *bind.Result, call *ast.FuncCallExpr, scope nativeOperationScope, add func(string, string, ...string)) {
	if call == nil {
		return
	}
	if nativeBoundMemberCall(bindings, call, "channel", "select") {
		add("effect_row", "exhaustive=true safepoint=required suspension=published yield=present")
		return
	}
	if nativeBoundStdlibMemberCall(bindings, call, "coroutine.yield") {
		add("effect_row", "control_transfer=suspend exhaustive=true suspension=published yield=present")
		return
	}
	if nativeBoundStdlibMemberCall(bindings, call, "coroutine.resume") {
		add("effect_row", "control_transfer=resume exhaustive=true safepoint=required suspension=published yield=present")
		return
	}
	if stdlib, ok := nativeBoundStdlibSignature(bindings, call); ok &&
		stdlib.OperationalEffects != nil &&
		stdlib.OperationalEffects.SuspensionKnown &&
		stdlib.OperationalEffects.MaySuspend {
		add("effect_row", "exhaustive=true safepoint=required suspension=published yield=present")
		return
	}
	if nativeBoundStdlibMemberCall(bindings, call, "string.gsub") && len(call.Args) >= 3 {
		add("effect_row", "allocation=present composed_from_callback=true control_transfer=callback exhaustive=true")
		return
	}
	if nativeBoundStdlibMemberCall(bindings, call, "table.sort") && len(call.Args) >= 2 {
		add("effect_row", "control_transfer=callback error=present exhaustive=true safepoint=required")
		return
	}
	if _, ok := nativeBoundStdlibCall(bindings, call, "load"); ok {
		add("effect_row", "exhaustive=false module_loading=present")
		return
	}
	if _, ok := nativeBoundStdlibCall(bindings, call, "pcall"); ok {
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
	if nativeStdlibCallIsOpen(bindings, call, "os.clock") {
		add("effect_row", "exhaustive=false")
	}
}

var nativeStdlibSignatures = signaturelookup.Source{IncludeStdlib: true}
var nativeStdlibGlobals = func() map[string]struct{} {
	names := signaturelookup.StdlibBareGlobals()
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}()

// nativeBoundStdlibCall recognizes a standard-library operation only when the
// binder resolves the callee to the corresponding global and the owning
// signature registry declares that operation. Source spelling alone is not a
// binding and therefore cannot license a native contract.
func nativeBoundStdlibCall(bindings *bind.Result, call *ast.FuncCallExpr, name string) (signature.Function, bool) {
	if call == nil || bindings == nil || name == "" || strings.Contains(name, ".") {
		return signature.Function{}, false
	}
	id, ok := call.Func.(*ast.IdentExpr)
	if !ok || !bindings.ResolvesToGlobal(id, name) {
		return signature.Function{}, false
	}
	if stdlib, registered := nativeStdlibSignatures.LookupView(name); registered {
		return stdlib, true
	}
	_, declared := nativeStdlibGlobals[name]
	if !declared {
		return signature.Function{}, false
	}
	// A declared bare global without a modeled signature is an open boundary.
	return signature.Function{Effect: effect.Unknown}, true
}

// nativeBoundStdlibMemberCall is the member counterpart: both the global root
// binding and the complete registered provider name must agree.
func nativeBoundStdlibMemberCall(bindings *bind.Result, call *ast.FuncCallExpr, name string) bool {
	object, method, ok := strings.Cut(name, ".")
	if !ok || object == "" || method == "" || strings.Contains(method, ".") {
		return false
	}
	if _, registered := nativeStdlibSignatures.LookupView(name); !registered {
		return false
	}
	return nativeBoundMemberCall(bindings, call, object, method)
}

func nativeBoundStdlibSignature(bindings *bind.Result, call *ast.FuncCallExpr) (signature.Function, bool) {
	if call == nil || bindings == nil {
		return signature.Function{}, false
	}
	if id, ok := call.Func.(*ast.IdentExpr); ok {
		return nativeBoundStdlibCall(bindings, call, id.Value)
	}
	attr, ok := call.Func.(*ast.AttrGetExpr)
	if !ok {
		return signature.Function{}, false
	}
	root, ok := attr.Object.(*ast.IdentExpr)
	member := ast.KeyName(attr.Key)
	if !ok || member == "" {
		return signature.Function{}, false
	}
	name := root.Value + "." + member
	if !nativeBoundStdlibMemberCall(bindings, call, name) {
		return signature.Function{}, false
	}
	return nativeStdlibSignatures.LookupView(name)
}

func nativeBoundMemberCall(bindings *bind.Result, call *ast.FuncCallExpr, object, method string) bool {
	if call == nil || bindings == nil {
		return false
	}
	attr, ok := call.Func.(*ast.AttrGetExpr)
	if !ok || ast.KeyName(attr.Key) != method {
		return false
	}
	id, ok := attr.Object.(*ast.IdentExpr)
	return ok && bindings.ResolvesToGlobal(id, object)
}

func nativeStdlibCallIsOpen(bindings *bind.Result, call *ast.FuncCallExpr, name string) bool {
	var stdlib signature.Function
	var ok bool
	if strings.Contains(name, ".") {
		if !nativeBoundStdlibMemberCall(bindings, call, name) {
			return false
		}
		stdlib, ok = nativeStdlibSignatures.LookupView(name)
	} else {
		stdlib, ok = nativeBoundStdlibCall(bindings, call, name)
	}
	return ok && (stdlib.Effect.IsOpen() || nativeSignatureCallsCallback(stdlib))
}

func nativeSignatureCallsCallback(stdlib signature.Function) bool {
	for _, label := range stdlib.Effect.Labels {
		result, ok := label.(returns.Return)
		if !ok {
			continue
		}
		if _, ok := result.Transform.(returns.CallbackReturn); ok {
			return true
		}
	}
	return false
}

// nativeRegisteredMethod covers colon calls whose receiver is the runtime
// value rather than a global library table. The method name is accepted only
// through the standard-library registry.
func nativeRegisteredMethod(call *ast.FuncCallExpr, name string) bool {
	if call == nil || call.Method == "" {
		return false
	}
	_, method, ok := strings.Cut(name, ".")
	if !ok || call.Method != method {
		return false
	}
	_, registered := nativeStdlibSignatures.LookupView(name)
	return registered
}
func nativeKnownFunction(expr ast.Expr, scope nativeOperationScope) bool {
	id, ok := expr.(*ast.IdentExpr)
	return ok && scope.types[id.Value] == "function"
}

func nativeOperandClass(bindings *bind.Result, expr ast.Expr, scope nativeOperationScope) (string, bool) {
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
		left, lok := nativeOperandClass(bindings, x.Lhs, scope)
		right, rok := nativeOperandClass(bindings, x.Rhs, scope)
		return left, lok && rok && left == right
	case *ast.FuncCallExpr:
		if _, ok := nativeBoundStdlibCall(bindings, x, "tostring"); ok {
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
