package front

// Native operation contracts are closed descriptions of operations admitted
// by the front.  This is intentionally a recognizer over the bound source
// topology, not a second value analysis: every positive row below names all
// of the facts that make it safe, and an incomplete shape publishes nothing.

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
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

var nativeStdlibSignatures = signaturelookup.Source{IncludeStdlib: true}

func nativeStdlibIdentity(name string) signaturelookup.Identity {
	identity, _ := nativeStdlibSignatures.RegistryIdentity(name)
	return identity
}

// nativeBoundStdlibCall recognizes a standard-library operation only when the
// binder resolves the callee to the corresponding global and the owning
// signature registry declares that operation. Source spelling alone is not a
// binding and therefore cannot license a native contract.
func nativeBoundStdlibCall(bindings *bind.Result, call *ast.FuncCallExpr, identity signaturelookup.Identity) (signature.Function, bool) {
	name := identity.Name()
	if call == nil || bindings == nil || name == "" || strings.Contains(name, ".") {
		return signature.Function{}, false
	}
	id, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		return signature.Function{}, false
	}
	global, bound := bindings.GlobalIdentity(id)
	if !bound || !global.Matches(name) {
		return signature.Function{}, false
	}
	if stdlib, registered := identity.Signature(); registered {
		return stdlib, true
	}
	// A declared bare global without a modeled signature is an open boundary.
	return signature.Function{Effect: effect.Unknown}, true
}

// nativeBoundStdlibMemberCall is the member counterpart: both the global root
// binding and the complete registered provider name must agree.
func nativeBoundStdlibMemberCall(bindings *bind.Result, call *ast.FuncCallExpr, identity signaturelookup.Identity) bool {
	name := identity.Name()
	object, method, ok := strings.Cut(name, ".")
	if !ok || object == "" || method == "" || strings.Contains(method, ".") {
		return false
	}
	if _, registered := identity.Signature(); !registered {
		return false
	}
	return nativeBoundMemberCall(bindings, call, object, method)
}

func nativeBoundStdlibSignature(bindings *bind.Result, call *ast.FuncCallExpr) (signature.Function, bool) {
	if call == nil || bindings == nil {
		return signature.Function{}, false
	}
	if id, ok := call.Func.(*ast.IdentExpr); ok {
		identity, registered := nativeStdlibSignatures.RegistryIdentity(id.Value)
		if !registered {
			return signature.Function{}, false
		}
		return nativeBoundStdlibCall(bindings, call, identity)
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
	identity, registered := nativeStdlibSignatures.RegistryIdentity(name)
	if !registered || !nativeBoundStdlibMemberCall(bindings, call, identity) {
		return signature.Function{}, false
	}
	return identity.Signature()
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
	identity, registered := nativeStdlibSignatures.RegistryIdentity(name)
	if !registered {
		return false
	}
	var stdlib signature.Function
	var ok bool
	if strings.Contains(name, ".") {
		if !nativeBoundStdlibMemberCall(bindings, call, identity) {
			return false
		}
		stdlib, ok = identity.Signature()
	} else {
		stdlib, ok = nativeBoundStdlibCall(bindings, call, identity)
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
func nativeRegisteredMethod(call *ast.FuncCallExpr, identity signaturelookup.Identity) bool {
	name := identity.Name()
	if call == nil || call.Method == "" {
		return false
	}
	_, method, ok := strings.Cut(name, ".")
	if !ok || call.Method != method {
		return false
	}
	_, registered := identity.Signature()
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
		if _, ok := nativeBoundStdlibCall(bindings, x, nativeStdlibIdentity("tostring")); ok {
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

func nativeOperationTopologyDrafts(body NativeBodyReference, stmts []ast.Stmt, bindings *bind.Result) []NativeTopologyDraft {
	var out []NativeTopologyDraft
	var site uint32
	add := func(draft *NativeEffectTopologyDraft) {
		if draft == nil {
			return
		}
		draft.Body = body
		draft.Site = site
		site++
		out = append(out, NativeTopologyDraft{Kind: NativeTopologyEffect, Effect: draft})
	}
	var walkStmts func([]ast.Stmt, nativeOperationScope, bool)
	var walkExpr func(ast.Expr, nativeOperationScope, bool)
	walkExpr = func(expr ast.Expr, scope nativeOperationScope, inLoop bool) {
		switch item := expr.(type) {
		case *ast.StringConcatOpExpr:
			for _, operand := range nativeFlattenConcat(item) {
				walkExpr(operand, scope, inLoop)
			}
		case *ast.FuncCallExpr:
			add(nativeEffectCallTopology(bindings, item, scope))
			walkExpr(item.Func, scope, inLoop)
			walkExpr(item.Receiver, scope, inLoop)
			for _, argument := range item.Args {
				walkExpr(argument, scope, inLoop)
			}
		case *ast.LogicalOpExpr:
			walkExpr(item.Lhs, scope, inLoop)
			walkExpr(item.Rhs, scope, inLoop)
		case *ast.AttrGetExpr:
			walkExpr(item.Object, scope, inLoop)
			walkExpr(item.Key, scope, inLoop)
		case *ast.TableExpr:
			for _, field := range item.Fields {
				if field != nil {
					walkExpr(field.Key, scope, inLoop)
					walkExpr(field.Value, scope, inLoop)
				}
			}
		case *ast.FunctionExpr:
			add(nativeFunctionEffectTopology(bindings, item))
			walkStmts(item.Stmts, nativeFunctionScope(item), inLoop)
		}
	}
	walkStmts = func(body []ast.Stmt, scope nativeOperationScope, inLoop bool) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.LocalAssignStmt:
				for index, expr := range item.Exprs {
					walkExpr(expr, scope, inLoop)
					if index >= len(item.Names) {
						continue
					}
					if function, ok := expr.(*ast.FunctionExpr); ok {
						if nativeFunctionHasOpenCall(bindings, function) {
							scope.types[item.Names[index]] = "function-open"
						} else {
							scope.types[item.Names[index]] = "function"
						}
					} else if class := nativeTypeAt(item.Types, index); class != "" {
						scope.types[item.Names[index]] = class
					} else if class, ok := nativeOperandClass(bindings, expr, scope); ok {
						scope.types[item.Names[index]] = class
					}
				}
			case *ast.AssignStmt:
				for _, expr := range item.Rhs {
					walkExpr(expr, scope, inLoop)
				}
				for _, expr := range item.Lhs {
					walkExpr(expr, scope, inLoop)
				}
			case *ast.FuncDefStmt:
				if item.Func != nil {
					if item.Name != nil {
						if id, ok := item.Name.Func.(*ast.IdentExpr); ok {
							if nativeFunctionHasOpenCall(bindings, item.Func) {
								scope.types[id.Value] = "function-open"
							} else {
								scope.types[id.Value] = "function"
							}
						}
					}
					walkExpr(item.Func, scope, inLoop)
				}
			case *ast.FuncCallStmt:
				walkExpr(item.Expr, scope, inLoop)
			case *ast.ReturnStmt:
				for _, expr := range item.Exprs {
					walkExpr(expr, scope, inLoop)
				}
			case *ast.IfStmt:
				walkExpr(item.Condition, scope, inLoop)
				walkStmts(item.Then, nativeScopeCopy(scope), inLoop)
				walkStmts(item.Else, nativeScopeCopy(scope), inLoop)
			case *ast.DoBlockStmt:
				walkStmts(item.Stmts, nativeScopeCopy(scope), inLoop)
			case *ast.WhileStmt:
				walkExpr(item.Condition, scope, inLoop)
				walkStmts(item.Stmts, nativeScopeCopy(scope), true)
			case *ast.RepeatStmt:
				walkStmts(item.Stmts, nativeScopeCopy(scope), true)
				walkExpr(item.Condition, scope, inLoop)
			case *ast.NumberForStmt:
				walkExpr(item.Init, scope, inLoop)
				walkExpr(item.Limit, scope, inLoop)
				walkExpr(item.Step, scope, inLoop)
				loop := nativeScopeCopy(scope)
				loop.types[item.Name] = nativeInteger
				walkStmts(item.Stmts, loop, true)
			case *ast.GenericForStmt:
				for _, expr := range item.Exprs {
					walkExpr(expr, scope, inLoop)
				}
				walkStmts(item.Stmts, nativeScopeCopy(scope), true)
			}
		}
	}
	walkStmts(stmts, nativeOperationScope{types: make(map[string]string)}, false)
	return out
}

func nativeFunctionEffectTopology(bindings *bind.Result, fn *ast.FunctionExpr) *NativeEffectTopologyDraft {
	if fn == nil {
		return nil
	}
	draft := &NativeEffectTopologyDraft{Operation: NativeEffectFunction}
	var position uint32
	var walkExpr func(ast.Expr)
	var walkStmts func([]ast.Stmt)
	walkExpr = func(expr ast.Expr) {
		switch item := expr.(type) {
		case *ast.FuncCallExpr:
			if _, ok := nativeBoundStdlibCall(bindings, item, nativeStdlibIdentity("error")); ok {
				draft.ErrorCallSites = append(draft.ErrorCallSites, position)
			}
			if nativeStdlibCallIsOpen(bindings, item, "os.clock") ||
				nativeStdlibCallIsOpen(bindings, item, "load") ||
				nativeStdlibCallIsOpen(bindings, item, "pcall") {
				draft.OpenCallSites = append(draft.OpenCallSites, position)
			}
			if nativeRegisteredMethod(item, nativeStdlibIdentity("string.upper")) ||
				nativeBoundStdlibMemberCall(bindings, item, nativeStdlibIdentity("string.upper")) ||
				nativeBoundStdlibMemberCall(bindings, item, nativeStdlibIdentity("string.gsub")) {
				draft.AllocationSites = append(draft.AllocationSites, position)
			}
			position++
			walkExpr(item.Func)
			walkExpr(item.Receiver)
			for _, argument := range item.Args {
				walkExpr(argument)
			}
		case *ast.StringConcatOpExpr:
			walkExpr(item.Lhs)
			walkExpr(item.Rhs)
		case *ast.LogicalOpExpr:
			walkExpr(item.Lhs)
			walkExpr(item.Rhs)
		case *ast.AttrGetExpr:
			walkExpr(item.Object)
			walkExpr(item.Key)
		case *ast.TableExpr:
			for _, field := range item.Fields {
				if field != nil {
					walkExpr(field.Key)
					walkExpr(field.Value)
				}
			}
		}
	}
	walkStmts = func(body []ast.Stmt) {
		for _, stmt := range body {
			switch item := stmt.(type) {
			case *ast.LocalAssignStmt:
				for _, expr := range item.Exprs {
					walkExpr(expr)
				}
			case *ast.AssignStmt:
				for _, expr := range item.Rhs {
					walkExpr(expr)
				}
			case *ast.FuncCallStmt:
				walkExpr(item.Expr)
			case *ast.ReturnStmt:
				for _, expr := range item.Exprs {
					walkExpr(expr)
				}
			case *ast.IfStmt:
				walkExpr(item.Condition)
				walkStmts(item.Then)
				walkStmts(item.Else)
			case *ast.DoBlockStmt:
				walkStmts(item.Stmts)
			case *ast.WhileStmt:
				walkExpr(item.Condition)
				walkStmts(item.Stmts)
			case *ast.RepeatStmt:
				walkStmts(item.Stmts)
				walkExpr(item.Condition)
			case *ast.NumberForStmt:
				walkExpr(item.Init)
				walkExpr(item.Limit)
				walkExpr(item.Step)
				walkStmts(item.Stmts)
			case *ast.GenericForStmt:
				for _, expr := range item.Exprs {
					walkExpr(expr)
				}
				walkStmts(item.Stmts)
			}
		}
	}
	walkStmts(fn.Stmts)
	return draft
}

func nativeEffectCallTopology(bindings *bind.Result, call *ast.FuncCallExpr, scope nativeOperationScope) *NativeEffectTopologyDraft {
	if call == nil {
		return nil
	}
	draft := &NativeEffectTopologyDraft{}
	switch {
	case nativeBoundMemberCall(bindings, call, "channel", "select"):
		draft.Operation = NativeEffectChannelSelect
	case nativeBoundStdlibMemberCall(bindings, call, nativeStdlibIdentity("coroutine.yield")):
		draft.Operation = NativeEffectCoroutineYield
	case nativeBoundStdlibMemberCall(bindings, call, nativeStdlibIdentity("coroutine.resume")):
		draft.Operation = NativeEffectCoroutineResume
	case nativeRegisteredSuspendingCall(bindings, call):
		draft.Operation = NativeEffectRegisteredSuspend
	case nativeBoundStdlibMemberCall(bindings, call, nativeStdlibIdentity("string.gsub")) && len(call.Args) >= 3:
		draft.Operation = NativeEffectStringGsub
	case nativeBoundStdlibMemberCall(bindings, call, nativeStdlibIdentity("table.sort")) && len(call.Args) >= 2:
		draft.Operation = NativeEffectTableSort
	default:
		if _, ok := nativeBoundStdlibCall(bindings, call, nativeStdlibIdentity("load")); ok {
			draft.Operation = NativeEffectModuleLoad
		} else if _, ok := nativeBoundStdlibCall(bindings, call, nativeStdlibIdentity("pcall")); ok {
			draft.Operation = NativeEffectProtectedCall
			if len(call.Args) != 0 {
				if _, ok := call.Args[0].(*ast.IdentExpr); ok && nativeKnownFunction(call.Args[0], scope) {
					draft.ArgumentShapes = []NativeOperandShape{NativeOperandSymbol}
				}
			}
		} else if id, ok := call.Func.(*ast.IdentExpr); ok && scope.types[id.Value] == "function" {
			draft.Operation = NativeEffectDirectLexicalCall
		} else if nativeStdlibCallIsOpen(bindings, call, "os.clock") {
			draft.Operation = NativeEffectOpenCall
		}
	}
	if draft.Operation == 0 {
		return nil
	}
	return draft
}

func nativeRegisteredSuspendingCall(bindings *bind.Result, call *ast.FuncCallExpr) bool {
	stdlib, ok := nativeBoundStdlibSignature(bindings, call)
	return ok && stdlib.OperationalEffects != nil &&
		stdlib.OperationalEffects.SuspensionKnown && stdlib.OperationalEffects.MaySuspend
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
