// Package symboliccall is an isolated proof that function value transformers
// can be composed once and instantiated many times over the production product
// domain. It is not imported by the checker.
package symboliccall

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

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
	opCapture
	opVararg
	opGlobal
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
	global GlobalRoot
}

// Global reads one stable module root supplied at instantiation. The module
// and exported name are structural identity, not a process-global string.
func Global(module, name string) Expr {
	root := GlobalRoot{Module: module, Name: name}
	if !root.valid() {
		panic("symboliccall: invalid stable global root")
	}
	return Expr{n: &exprNode{op: opGlobal, global: root}}
}

// Capture reads one namespace-distinct lexical closure cell. It is never
// interchangeable with Param(index), even when the numeric index is equal.
func Capture(index int) Expr {
	if index < 0 {
		panic("symboliccall: negative capture")
	}
	return Expr{n: &exprNode{op: opCapture, param: index}}
}

// Vararg reads one element of the incoming Lua vararg pack. At concrete
// instantiation an index beyond the exact pack length evaluates to Absent;
// an explicitly supplied nil remains a present nil value.
func Vararg(index int) Expr {
	if index < 0 {
		panic("symboliccall: negative vararg index")
	}
	return Expr{n: &exprNode{op: opVararg, param: index}}
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
	case opParam, opCapture, opVararg:
		return a.n.param == b.n.param
	case opGlobal:
		return a.n.global == b.n.global
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

// exprCanonicalKey is a stable representation for deterministic row order.
// Product constants use their registry-stable canonical hash. Constructors
// reject unequal constants with the same hash before relying on this key.
func exprCanonicalKey(reg *axis.Registry, expr Expr) string {
	if expr.n == nil {
		return "0"
	}
	var b strings.Builder
	b.WriteString(strconv.Itoa(int(expr.n.op)))
	b.WriteByte(':')
	switch expr.n.op {
	case opParam, opCapture, opVararg:
		b.WriteString(strconv.Itoa(expr.n.param))
	case opGlobal:
		b.WriteString(expr.n.global.key())
	case opConst:
		b.WriteString(strconv.FormatUint(product.Hash(reg, expr.n.value), 16))
	case opJoin:
		keys := make([]string, len(expr.n.args))
		for i, arg := range expr.n.args {
			keys[i] = exprCanonicalKey(reg, arg)
		}
		sort.Strings(keys)
		b.WriteString(strings.Join(keys, ","))
	case opCall:
		b.WriteString(string(expr.n.callee))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(expr.n.slot))
		for _, arg := range expr.n.args {
			b.WriteByte(':')
			b.WriteString(exprCanonicalKey(reg, arg))
		}
	}
	return b.String()
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
	return evalBoundary(reg, expr, params, nil, nil)
}

func evalBoundary(reg *axis.Registry, expr Expr, params, captures, varargs []product.Value) (product.Value, error) {
	return evalEnvironment(reg, expr, params, captures, varargs, nil)
}

func evalEnvironment(reg *axis.Registry, expr Expr, params, captures, varargs []product.Value, globals map[GlobalRoot]product.Value) (product.Value, error) {
	if expr.n == nil || expr.n.op == opBottom {
		return product.Bottom(reg), nil
	}
	switch expr.n.op {
	case opParam:
		if expr.n.param >= len(params) {
			return product.Value{}, fmt.Errorf("symboliccall: parameter %d out of range", expr.n.param)
		}
		return params[expr.n.param], nil
	case opCapture:
		if expr.n.param >= len(captures) {
			return product.Value{}, fmt.Errorf("symboliccall: capture %d out of range", expr.n.param)
		}
		return captures[expr.n.param], nil
	case opVararg:
		if expr.n.param >= len(varargs) {
			return product.Absent(reg), nil
		}
		return varargs[expr.n.param], nil
	case opGlobal:
		value, ok := globals[expr.n.global]
		if !ok {
			return product.Value{}, fmt.Errorf("symboliccall: stable global %s is unbound", expr.n.global.key())
		}
		return value, nil
	case opConst:
		// This validates that the constant belongs to reg.
		_ = product.Equal(reg, expr.n.value, expr.n.value)
		return expr.n.value, nil
	case opJoin:
		out := product.Bottom(reg)
		for _, arg := range expr.n.args {
			value, err := evalEnvironment(reg, arg, params, captures, varargs, globals)
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
	case opBottom, opConst, opCapture, opVararg, opGlobal:
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
