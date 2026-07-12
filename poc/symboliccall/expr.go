// Package symboliccall is an isolated proof that function value transformers
// can be composed once and instantiated many times over the production product
// domain. It is not imported by the checker.
package symboliccall

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type exprOp uint8

const (
	opBottom exprOp = iota
	opParam
	opConst
	opJoin
	opCall
)

// FunctionID is a stable lexical function identity in the experiment.
type FunctionID string

// Expr is an immutable symbolic product-value expression.
type Expr struct{ n *exprNode }

type exprNode struct {
	op     exprOp
	param  int
	value  product.Value
	args   []Expr
	callee FunctionID
	slot   int
}

// Param reads one callee parameter at instantiation.
func Param(index int) Expr {
	if index < 0 {
		panic("symboliccall: negative parameter")
	}
	return Expr{n: &exprNode{op: opParam, param: index}}
}

// Const returns a caller-independent production product value.
func Const(value product.Value) Expr { return Expr{n: &exprNode{op: opConst, value: value}} }

// Call is one stable direct call result. Arguments are defensively copied.
func Call(callee FunctionID, slot int, args ...Expr) Expr {
	if callee == "" || slot < 0 {
		panic("symboliccall: invalid call")
	}
	return Expr{n: &exprNode{op: opCall, callee: callee, slot: slot, args: cloneExprs(args)}}
}

// Join constructs a flattened, idempotent symbolic product join. Bottom
// operands disappear. Equality is collision-safe and does not rely on hashes.
func Join(args ...Expr) Expr {
	flat := make([]Expr, 0, len(args))
	for _, arg := range args {
		if arg.n == nil || arg.n.op == opBottom {
			continue
		}
		if arg.n.op == opJoin {
			flat = append(flat, arg.n.args...)
		} else {
			flat = append(flat, arg)
		}
	}
	dedup := flat[:0]
	for _, candidate := range flat {
		seen := false
		for _, kept := range dedup {
			if exprEqual(candidate, kept) {
				seen = true
				break
			}
		}
		if !seen {
			dedup = append(dedup, candidate)
		}
	}
	switch len(dedup) {
	case 0:
		return Expr{}
	case 1:
		return dedup[0]
	default:
		return Expr{n: &exprNode{op: opJoin, args: cloneExprs(dedup)}}
	}
}

func cloneExprs(in []Expr) []Expr { return append([]Expr(nil), in...) }

func exprEqual(a, b Expr) bool {
	if a.n == b.n {
		return true
	}
	if a.n == nil || b.n == nil || a.n.op != b.n.op {
		return false
	}
	switch a.n.op {
	case opBottom:
		return true
	case opParam:
		return a.n.param == b.n.param
	case opConst:
		// Registry ownership is checked when the expression is evaluated. The
		// production interner gives equal values shared representation, while
		// unequal values never compare by a digest alone.
		return a.n.value == b.n.value
	case opCall:
		return a.n.callee == b.n.callee && a.n.slot == b.n.slot && exprSliceEqual(a.n.args, b.n.args)
	case opJoin:
		if len(a.n.args) != len(b.n.args) {
			return false
		}
		used := make([]bool, len(b.n.args))
		for _, left := range a.n.args {
			found := false
			for j, right := range b.n.args {
				if !used[j] && exprEqual(left, right) {
					used[j], found = true, true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func exprSliceEqual(a, b []Expr) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !exprEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func eval(reg *axis.Registry, expr Expr, params []product.Value) (product.Value, error) {
	if expr.n == nil || expr.n.op == opBottom {
		return product.Bottom(reg), nil
	}
	switch expr.n.op {
	case opParam:
		if expr.n.param >= len(params) {
			return product.Value{}, fmt.Errorf("symboliccall: parameter %d out of range", expr.n.param)
		}
		return params[expr.n.param], nil
	case opConst:
		// This validates that the constant belongs to reg.
		_ = product.Equal(reg, expr.n.value, expr.n.value)
		return expr.n.value, nil
	case opJoin:
		out := product.Bottom(reg)
		for _, arg := range expr.n.args {
			value, err := eval(reg, arg, params)
			if err != nil {
				return product.Value{}, err
			}
			out = product.Join(reg, out, value)
		}
		return out, nil
	case opCall:
		return product.Value{}, fmt.Errorf("symboliccall: unresolved call to %q", expr.n.callee)
	default:
		return product.Value{}, fmt.Errorf("symboliccall: invalid expression")
	}
}

func substitute(expr Expr, params []Expr) (Expr, bool) {
	if expr.n == nil {
		return Expr{}, true
	}
	switch expr.n.op {
	case opBottom, opConst:
		return expr, true
	case opParam:
		if expr.n.param >= len(params) {
			return Expr{}, false
		}
		return params[expr.n.param], true
	case opJoin:
		args := make([]Expr, len(expr.n.args))
		for i, arg := range expr.n.args {
			var ok bool
			args[i], ok = substitute(arg, params)
			if !ok {
				return Expr{}, false
			}
		}
		return Join(args...), true
	case opCall:
		return Expr{}, false
	default:
		return Expr{}, false
	}
}
