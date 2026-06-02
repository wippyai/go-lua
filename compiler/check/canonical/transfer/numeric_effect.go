package transfer

import (
	"math"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
)

type NumericOpKind uint8

const (
	NumericDropLenBound NumericOpKind = iota + 1
	NumericLenGeConst
	NumericLenLeConst
	NumericVarEqConst
	NumericVarGeConst
	NumericVarLeConst
	NumericVarLeLenOffset
	NumericIncrementLenLower
)

// NumericEffect is the canonical product-state effect for facts in the numeric
// component. Callers lower guards, assignments, and table-length mutations into
// primitive numeric atoms; this reducer owns cloning, top initialization, and
// canonical storage of PointState.Num.
type NumericEffect struct {
	Ops             []NumericOp
	RequireExisting bool
}

type NumericOp struct {
	Kind   NumericOpKind
	Key    constraint.PathKey
	Other  constraint.PathKey
	Const  int64
	Offset int64
	Delta  int64
}

func (t *Transfer) applyNumericEffect(out *flow.PointState, effect NumericEffect) bool {
	if out == nil || len(effect.Ops) == 0 {
		return false
	}
	if effect.RequireExisting && out.Num == nil {
		return false
	}
	before := out.Num
	num := before.Clone()
	if num == nil {
		num = numeric.NewState()
	}
	applied := false
	for _, op := range effect.Ops {
		applied = applyNumericOp(num, op) || applied
	}
	if !applied {
		return false
	}
	next := normalizeNumericEffectState(num)
	if numeric.StateDomain.Equal(before, next) {
		return false
	}
	out.Num = next
	return true
}

func applyNumericOp(num *numeric.State, op NumericOp) bool {
	if num == nil {
		return false
	}
	switch op.Kind {
	case NumericDropLenBound:
		if op.Key == "" {
			return false
		}
		num.DropLenBound(op.Key)
		return true
	case NumericLenGeConst:
		if op.Key == "" {
			return false
		}
		num.ApplyLenGeConst(op.Key, op.Const)
		return true
	case NumericLenLeConst:
		if op.Key == "" {
			return false
		}
		num.ApplyLenLeConst(op.Key, op.Const)
		return true
	case NumericVarEqConst:
		if op.Key == "" {
			return false
		}
		num.ApplyEqConst(op.Key, op.Const)
		return true
	case NumericVarGeConst:
		if op.Key == "" {
			return false
		}
		num.ApplyGeConst(op.Key, op.Const)
		return true
	case NumericVarLeConst:
		if op.Key == "" {
			return false
		}
		num.ApplyLeConst(op.Key, op.Const)
		return true
	case NumericVarLeLenOffset:
		if op.Key == "" || op.Other == "" {
			return false
		}
		num.ApplyLeLenOfWithOffset(op.Key, op.Other, op.Offset)
		return true
	case NumericIncrementLenLower:
		if op.Key == "" || op.Delta <= 0 {
			return false
		}
		if lower, _, ok := num.LenBoundsFor(op.Key); ok {
			num.ApplyLenGeConst(op.Key, lower+op.Delta)
		} else {
			num.ApplyLenGeConst(op.Key, op.Delta)
		}
		return true
	default:
		return false
	}
}

func normalizeNumericEffectState(num *numeric.State) *numeric.State {
	if num == nil || num.IsTop() {
		return numeric.Top()
	}
	return num
}

func numericConstComparisonOps(key constraint.PathKey, op string, c int64) []NumericOp {
	switch op {
	case "<":
		if c == math.MinInt64 {
			return nil
		}
		return []NumericOp{{Kind: NumericVarLeConst, Key: key, Const: c - 1}}
	case "<=":
		return []NumericOp{{Kind: NumericVarLeConst, Key: key, Const: c}}
	case ">":
		if c == math.MaxInt64 {
			return nil
		}
		return []NumericOp{{Kind: NumericVarGeConst, Key: key, Const: c + 1}}
	case ">=":
		return []NumericOp{{Kind: NumericVarGeConst, Key: key, Const: c}}
	default:
		return nil
	}
}

func numericLengthBoundOps(key constraint.PathKey, op string, c int64) []NumericOp {
	switch op {
	case "<":
		if c == math.MinInt64 {
			return nil
		}
	case ">":
		if c == math.MaxInt64 {
			return nil
		}
	}
	floor, ceil, hasFloor, hasCeil := lengthBoundFromOp(op, c)
	if !hasFloor && !hasCeil {
		return nil
	}
	ops := make([]NumericOp, 0, 2)
	if hasFloor {
		ops = append(ops, NumericOp{Kind: NumericLenGeConst, Key: key, Const: floor})
	}
	if hasCeil {
		ops = append(ops, NumericOp{Kind: NumericLenLeConst, Key: key, Const: ceil})
	}
	return ops
}
