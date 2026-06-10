package expr

// Simplify attempts to simplify an expression by constant folding.
func Simplify(e Expr) Expr {
	return VisitExpr(e, ExprVisitor[Expr]{
		Var: func(Var) Expr {
			return e
		},
		Const: func(Const) Expr {
			return e
		},
		Len: func(Len) Expr {
			return e
		},
		Param: func(Param) Expr {
			return e
		},
		Ret: func(Ret) Expr {
			return e
		},
		ParamLen: func(ParamLen) Expr {
			return e
		},
		RetLen: func(RetLen) Expr {
			return e
		},
		Min: func(x Min) Expr {
			left := Simplify(x.Left)
			right := Simplify(x.Right)
			lc, lok := left.(Const)
			rc, rok := right.(Const)

			if lok && rok {
				if lc.Value < rc.Value {
					return lc
				}

				return rc
			}

			return Min{Left: left, Right: right}
		},
		Max: func(x Max) Expr {
			left := Simplify(x.Left)
			right := Simplify(x.Right)
			lc, lok := left.(Const)
			rc, rok := right.(Const)

			if lok && rok {
				if lc.Value > rc.Value {
					return lc
				}

				return rc
			}

			return Max{Left: left, Right: right}
		},
		BinOp: func(x BinOp) Expr {
			left := Simplify(x.Left)
			right := Simplify(x.Right)

			lc, lok := left.(Const)
			rc, rok := right.(Const)

			if lok && rok {
				result, ok := BinOp{Op: x.Op, Left: lc, Right: rc}.Eval(nil)
				if ok {
					return C(result)
				}
			}

			// x + 0 = x, 0 + x = x
			if x.Op == OpAdd {
				if lok && lc.Value == 0 {
					return right
				}

				if rok && rc.Value == 0 {
					return left
				}
			}

			// x - 0 = x
			if x.Op == OpSub && rok && rc.Value == 0 {
				return left
			}

			// x * 1 = x, 1 * x = x, x * 0 = 0, 0 * x = 0
			if x.Op == OpMul {
				if lok && lc.Value == 1 {
					return right
				}

				if rok && rc.Value == 1 {
					return left
				}

				if (lok && lc.Value == 0) || (rok && rc.Value == 0) {
					return C(0)
				}
			}

			return BinOp{Op: x.Op, Left: left, Right: right}
		},
		Default: func(Expr) Expr {
			return e
		},
	})
}
