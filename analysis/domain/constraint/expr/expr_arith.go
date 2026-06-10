package expr

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/internal/intarith"
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
		return intarith.SafeAdd(left, right)
	case OpSub:
		return intarith.SafeSub(left, right)
	case OpMul:
		return intarith.SafeMul(left, right)
	case OpDiv:
		if right == 0 {
			return 0, false
		}

		if left == intarith.MinInt64 && right == -1 {
			return 0, false
		}

		return left / right, true
	case OpMod:
		if right == 0 {
			return 0, false
		}

		if left == intarith.MinInt64 && right == -1 {
			return 0, true
		}

		return left % right, true
	default:
		return 0, false
	}
}
