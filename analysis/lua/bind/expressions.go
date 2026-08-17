package bind

import "github.com/wippyai/go-lua/compiler/ast"

// The expression lane owns runtime reads, value traversal, and the narrow
// runtime-type call recognition that changes how a call's base is bound. It
// stays in package bind because these operations share the binder's private
// scope/control state; splitting them into a second binder would create a
// competing lexical authority.

func (b *binder) visitExprList(step bindStep) {
	exprs := expressionList(step.node)
	if step.index >= len(exprs) {
		return
	}
	expr := exprs[step.index]
	step.index++
	b.push(step)
	b.scheduleExpr(expr, step.mode)
}

func expressionList(node ast.PositionHolder) []ast.Expr {
	switch n := node.(type) {
	case *ast.AssignStmt:
		return n.Rhs
	case *ast.ReturnStmt:
		return n.Exprs
	case *ast.GenericForStmt:
		return n.Exprs
	case *ast.FuncCallExpr:
		return n.Args
	default:
		return nil
	}
}

func (b *binder) visitLValue(expr ast.Expr, mode exprBindMode) {
	switch e := expr.(type) {
	case nil:
	case *ast.IdentExpr:
		if mode == exprBindTypeQuery {
			b.bindTypeQueryWriteIdent(e)
		} else {
			b.bindWriteIdent(e)
		}
	case *ast.AttrGetExpr:
		if e.KeySyntax != ast.AttrKeyDot {
			b.scheduleExpr(e.Key, mode)
		}
		b.scheduleExpr(e.Object, mode)
	default:
		b.scheduleExpr(expr, mode)
	}
}

func (b *binder) visitExpr(expr ast.Expr, mode exprBindMode) {
	switch e := expr.(type) {
	case nil:
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr:
	case *ast.Comma3Expr:
		if mode == exprBindRuntime {
			b.bindVararg(e)
		}
	case *ast.IdentExpr:
		if mode == exprBindTypeQuery {
			b.bindTypeQueryIdent(e)
		} else {
			b.bindReadIdent(e)
		}
	case *ast.AttrGetExpr:
		if e.KeySyntax != ast.AttrKeyDot {
			b.scheduleExpr(e.Key, mode)
		}
		b.scheduleExpr(e.Object, mode)
	case *ast.TableExpr:
		b.push(bindStep{kind: stepTableFields, node: e, mode: mode})
	case *ast.FuncCallExpr:
		if value, ok := b.runtimeTypeCallBase(e); ok {
			b.bindRuntimeTypeValue(value)
			// The marked base is the only call component omitted from ordinary
			// binding. Arguments and explicit type arguments retain the current
			// traversal mode, so static queries keep authority without creating
			// executable read evidence.
			b.push(bindStep{kind: stepTypeList, node: e, phase: phaseCallTypes})
			b.push(bindStep{kind: stepExprList, node: e, mode: mode})
			return
		}
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseCallTypes})
		b.push(bindStep{kind: stepExprList, node: e, mode: mode})
		b.scheduleExpr(e.Receiver, mode)
		b.push(bindStep{kind: stepDirectGlobalCall, node: e})
		b.scheduleExpr(e.Func, mode)
	case *ast.LogicalOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.RelationalOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.StringConcatOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.ArithmeticOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.UnaryMinusOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.UnaryNotOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.UnaryLenOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.UnaryBNotOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.FunctionExpr:
		b.enterFunction(e, false, functionOriginDetails{
			kind:       FunctionOriginLiteral,
			localIndex: -1,
		}, mode)
	case *ast.CastExpr:
		b.scheduleType(e.Type)
		b.scheduleExpr(e.Expr, mode)
	case *ast.NonNilAssertExpr:
		b.scheduleExpr(e.Expr, mode)
	}
}

// recordDirectGlobalCall runs immediately after a normal call's function
// expression has been bound and before its receiver/arguments. It records
// generic syntactic/binding evidence for both runtime and static-query calls;
// containment later decides whether a literal direct require is executable.
func (b *binder) recordDirectGlobalCall(call *ast.FuncCallExpr) {
	if b == nil || b.result == nil || call == nil || call.Method != "" || call.Receiver != nil {
		return
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident == nil {
		return
	}
	identity, ok := b.result.GlobalIdentity(ident)
	if !ok {
		return
	}
	b.result.directGlobalCalls = append(b.result.directGlobalCalls, DirectGlobalCall{
		Call: call, Global: identity,
	})
}

func runtimeTypeMethodName(name string) bool {
	switch name {
	case "is", "kind", "name", "elem", "key", "val", "inner", "ret",
		"fields", "variants", "params", "tparams":
		return true
	default:
		return false
	}
}

// runtimeTypeCallBase recognizes the exact call shapes whose base compiles to
// OP_LOADTYPE. It intentionally does not classify a plain value, a dynamic
// member key, or an unrecognized method spelling.
func (b *binder) runtimeTypeCallBase(call *ast.FuncCallExpr) (RuntimeTypeValue, bool) {
	if b == nil || b.result == nil || call == nil {
		return RuntimeTypeValue{}, false
	}
	var base *ast.IdentExpr
	switch {
	case call.Method == "" && call.Receiver == nil:
		if ident, ok := call.Func.(*ast.IdentExpr); ok {
			base = ident
		} else if member, ok := call.Func.(*ast.AttrGetExpr); ok {
			key, keyOK := member.Key.(*ast.StringExpr)
			ident, identOK := member.Object.(*ast.IdentExpr)
			if !keyOK || !identOK ||
				(member.KeySyntax != ast.AttrKeyDot && member.KeySyntax != ast.AttrKeyIndex) {
				return RuntimeTypeValue{}, false
			}
			if !runtimeTypeMethodName(key.Value) {
				return RuntimeTypeValue{}, false
			}
			base = ident
		} else {
			return RuntimeTypeValue{}, false
		}
	case call.Func == nil:
		if !runtimeTypeMethodName(call.Method) {
			return RuntimeTypeValue{}, false
		}
		ident, ok := call.Receiver.(*ast.IdentExpr)
		if !ok {
			return RuntimeTypeValue{}, false
		}
		base = ident
	default:
		return RuntimeTypeValue{}, false
	}
	return b.runtimeTypeValueAuthority(base)
}

func (b *binder) visitTableFields(step bindStep) {
	table := step.node.(*ast.TableExpr)
	if step.index >= len(table.Fields) {
		return
	}
	field := table.Fields[step.index]
	step.index++
	b.push(step)
	if field == nil {
		return
	}
	b.scheduleExpr(field.Value, step.mode)
	if field.KeySyntax != ast.AttrKeyDot {
		b.scheduleExpr(field.Key, step.mode)
	}
}
