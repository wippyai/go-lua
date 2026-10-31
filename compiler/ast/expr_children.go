package ast

// WalkExprChildren calls visit for each direct expression child in source
// order. Child indexes identify syntactic slots: table fields use alternating
// key/value indexes, and calls use function, receiver, then arguments.
func WalkExprChildren(expr Expr, visit func(Expr, int)) {
	if expr == nil || visit == nil {
		return
	}
	switch e := expr.(type) {
	case *AttrGetExpr:
		visit(e.Object, 0)
		visit(e.Key, 1)
	case *TableExpr:
		for i, field := range e.Fields {
			if field != nil {
				visit(field.Key, 2*i)
				visit(field.Value, 2*i+1)
			} else {
				visit(nil, 2*i)
				visit(nil, 2*i+1)
			}
		}
	case *FuncCallExpr:
		visit(e.Func, 0)
		visit(e.Receiver, 1)
		for i, arg := range e.Args {
			visit(arg, i+2)
		}
	case *LogicalOpExpr:
		visit(e.Lhs, 0)
		visit(e.Rhs, 1)
	case *RelationalOpExpr:
		visit(e.Lhs, 0)
		visit(e.Rhs, 1)
	case *StringConcatOpExpr:
		visit(e.Lhs, 0)
		visit(e.Rhs, 1)
	case *ArithmeticOpExpr:
		visit(e.Lhs, 0)
		visit(e.Rhs, 1)
	case *UnaryMinusOpExpr:
		visit(e.Expr, 0)
	case *UnaryNotOpExpr:
		visit(e.Expr, 0)
	case *UnaryLenOpExpr:
		visit(e.Expr, 0)
	case *UnaryBNotOpExpr:
		visit(e.Expr, 0)
	case *CastExpr:
		visit(e.Expr, 0)
	case *NonNilAssertExpr:
		visit(e.Expr, 0)
	}
}
