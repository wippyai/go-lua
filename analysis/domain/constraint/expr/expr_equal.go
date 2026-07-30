package expr

import "sort"

// ExprEquals compares two Expr values for structural equality.
func ExprEquals(a, b Expr) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return VisitExpr(a, ExprVisitor[bool]{
		Var: func(av Var) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Var: func(bv Var) bool {
					return av.Name == bv.Name
				},
				Default: func(Expr) bool { return false },
			})
		},
		Const: func(av Const) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Const: func(bv Const) bool {
					return av.Value == bv.Value
				},
				Default: func(Expr) bool { return false },
			})
		},
		Len: func(av Len) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Len: func(bv Len) bool {
					return av.Of == bv.Of
				},
				Default: func(Expr) bool { return false },
			})
		},
		Param: func(av Param) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Param: func(bv Param) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		Ret: func(av Ret) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Ret: func(bv Ret) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		ParamLen: func(av ParamLen) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				ParamLen: func(bv ParamLen) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		RetLen: func(av RetLen) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				RetLen: func(bv RetLen) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		BinOp: func(av BinOp) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				BinOp: func(bv BinOp) bool {
					return av.Op == bv.Op && ExprEquals(av.Left, bv.Left) && ExprEquals(av.Right, bv.Right)
				},
				Default: func(Expr) bool { return false },
			})
		},
		Min: func(av Min) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Min: func(bv Min) bool {
					return ExprEquals(av.Left, bv.Left) && ExprEquals(av.Right, bv.Right)
				},
				Default: func(Expr) bool { return false },
			})
		},
		Max: func(av Max) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Max: func(bv Max) bool {
					return ExprEquals(av.Left, bv.Left) && ExprEquals(av.Right, bv.Right)
				},
				Default: func(Expr) bool { return false },
			})
		},
		Default: func(Expr) bool {
			return false
		},
	})
}

// collectVars collects unique variable names from expressions.
func collectVars(exprs ...Expr) []string {
	seen := make(map[string]bool)

	for _, e := range exprs {
		for _, v := range e.Variables() {
			seen[v] = true
		}
	}

	result := make([]string, 0, len(seen))

	for v := range seen {
		result = append(result, v)
	}

	sort.Strings(result)

	return result
}
