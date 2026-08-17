package expr

import (
	"fmt"
	"math"
)

// BinOp represents a binary arithmetic operation.
type BinOp struct {
	Op    Op
	Left  Expr
	Right Expr
}

// Op represents an arithmetic operator.
type Op int

const (
	OpAdd Op = iota // +
	OpSub           // -
	OpMul           // *
	OpDiv           // /
	OpMod           // %
)

func (op Op) String() string {
	switch op {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	default:
		return "?"
	}
}

func (BinOp) exprNode() {}

func (b BinOp) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left, b.Op, b.Right)
}

func (b BinOp) Substitute(subst map[string]Expr) Expr {
	return BinOp{
		Op:    b.Op,
		Left:  b.Left.Substitute(subst),
		Right: b.Right.Substitute(subst),
	}
}

func (b BinOp) Variables() []string {
	return collectVars(b.Left, b.Right)
}

func (b BinOp) Eval(env map[string]int64) (int64, bool) {
	left, ok := b.Left.Eval(env)
	if !ok {
		return 0, false
	}

	right, ok := b.Right.Eval(env)
	if !ok {
		return 0, false
	}

	switch b.Op {
	case OpAdd:
		return safeAdd(left, right)
	case OpSub:
		return safeSub(left, right)
	case OpMul:
		return safeMul(left, right)
	case OpDiv:
		if right == 0 {
			return 0, false
		}

		if left == math.MinInt64 && right == -1 {
			return 0, false
		}

		return left / right, true
	case OpMod:
		if right == 0 {
			return 0, false
		}

		if left == math.MinInt64 && right == -1 {
			return 0, true
		}

		return left % right, true
	default:
		return 0, false
	}
}

func safeAdd(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func safeSub(a, b int64) (int64, bool) {
	if b < 0 && a > math.MaxInt64+b {
		return 0, false
	}
	if b > 0 && a < math.MinInt64+b {
		return 0, false
	}
	return a - b, true
}

func safeMul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if a == -1 && b == math.MinInt64 {
		return 0, false
	}
	if b == -1 && a == math.MinInt64 {
		return 0, false
	}
	result := a * b
	if result/a != b {
		return 0, false
	}
	return result, true
}
