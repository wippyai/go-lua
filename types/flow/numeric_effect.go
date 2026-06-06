package flow

import (
	"math"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
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

// NumericEffect is the canonical PointState effect for facts in the numeric
// component. Transfer lowers guards, assignments, and table-length mutations
// into primitive numeric atoms; flow owns cloning, top initialization, and
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

func ApplyNumericEffect(out *PointState, effect NumericEffect) bool {
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

func NumericConstComparisonOps(key constraint.PathKey, op string, c int64) []NumericOp {
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

func NumericLengthBoundOps(key constraint.PathKey, op string, c int64) []NumericOp {
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
	floor, ceil, hasFloor, hasCeil := LengthBoundFromOp(op, c)
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

// NumericKeyOfValueKey lifts a value slot key into the numeric domain's key
// carrier. The numeric domain stores scalar value bounds and container length
// bounds in one PathKey-indexed state, so this conversion is a flow concern.
func NumericKeyOfValueKey(key ValueKey) (constraint.PathKey, bool) {
	if key == "" {
		return "", false
	}
	return constraint.PathKey(key), true
}

// NumericDropLenBoundValueKeyOp materializes a length-bound reset for key.
func NumericDropLenBoundValueKeyOp(key ValueKey) (NumericOp, bool) {
	numericKey, ok := NumericKeyOfValueKey(key)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericDropLenBound, Key: numericKey}, true
}

// NumericLenGeConstValueKeyOp materializes `len(key) >= lower`.
func NumericLenGeConstValueKeyOp(key ValueKey, lower int64) (NumericOp, bool) {
	numericKey, ok := NumericKeyOfValueKey(key)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericLenGeConst, Key: numericKey, Const: lower}, true
}

// NumericIncrementLenLowerValueKeyOp materializes `len(key) += delta` on the
// current lower bound.
func NumericIncrementLenLowerValueKeyOp(key ValueKey, delta int64) (NumericOp, bool) {
	numericKey, ok := NumericKeyOfValueKey(key)
	if !ok || delta <= 0 {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericIncrementLenLower, Key: numericKey, Delta: delta}, true
}

// NumericVarEqConstValueKeyOp materializes `key == c`.
func NumericVarEqConstValueKeyOp(key ValueKey, c int64) (NumericOp, bool) {
	numericKey, ok := NumericKeyOfValueKey(key)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericVarEqConst, Key: numericKey, Const: c}, true
}

// NumericVarKeyOfSymbol returns the numeric-component key for a scalar symbol.
// Numeric variables are value cells, so their key is the symbol value key lifted
// into the numeric domain's PathKey carrier.
func NumericVarKeyOfSymbol(sym cfg.SymbolID) (constraint.PathKey, bool) {
	if sym == 0 {
		return "", false
	}
	return NumericKeyOfValueKey(SymbolValueKey(sym))
}

// SymbolOfNumericVarKey returns the bare symbol identified by a numeric variable
// key. Nested container-path length keys are not scalar symbol variables.
func SymbolOfNumericVarKey(key constraint.PathKey) (cfg.SymbolID, bool) {
	sym, segments, ok := ParseSymbolPathKey(key)
	return sym, ok && len(segments) == 0
}

// NumericVarGeConstSymbolOp materializes `sym >= c`.
func NumericVarGeConstSymbolOp(sym cfg.SymbolID, c int64) (NumericOp, bool) {
	key, ok := NumericVarKeyOfSymbol(sym)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericVarGeConst, Key: key, Const: c}, true
}

// NumericVarLeConstSymbolOp materializes `sym <= c`.
func NumericVarLeConstSymbolOp(sym cfg.SymbolID, c int64) (NumericOp, bool) {
	key, ok := NumericVarKeyOfSymbol(sym)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericVarLeConst, Key: key, Const: c}, true
}

// NumericVarLeLenOffsetSymbolOp materializes `sym <= len(other) + offset`.
func NumericVarLeLenOffsetSymbolOp(sym cfg.SymbolID, other constraint.PathKey, offset int64) (NumericOp, bool) {
	key, ok := NumericVarKeyOfSymbol(sym)
	if !ok || other == "" {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericVarLeLenOffset, Key: key, Other: other, Offset: offset}, true
}

// NumericLenGeConstSymbolOp materializes `len(sym) >= lower` for a bare symbol
// container.
func NumericLenGeConstSymbolOp(sym cfg.SymbolID, lower int64) (NumericOp, bool) {
	key, ok := NumericVarKeyOfSymbol(sym)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericLenGeConst, Key: key, Const: lower}, true
}

// NumericLenGeConstPathOp materializes a resolved path length floor as the
// primitive numeric atom stored in PointState.Num.
func NumericLenGeConstPathOp(path constraint.Path, lower int64) (NumericOp, bool) {
	key, ok := SymbolPathKeyOf(path)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericLenGeConst, Key: key, Const: lower}, true
}

// NumericLenGeConstIndexedPrefixOps translates an indexed path read/write into
// length floors for each container prefix it proves present.
func NumericLenGeConstIndexedPrefixOps(path constraint.Path) []NumericOp {
	if path.Symbol == 0 || len(path.Segments) == 0 {
		return nil
	}
	ops := make([]NumericOp, 0, len(path.Segments))
	for i, seg := range path.Segments {
		if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 {
			continue
		}
		prefix := constraint.Path{Symbol: path.Symbol, Root: path.Root}
		if i > 0 {
			prefix.Segments = path.Segments[:i]
		}
		op, ok := NumericLenGeConstPathOp(prefix, int64(seg.Index))
		if ok {
			ops = append(ops, op)
		}
	}
	return ops
}

// LengthBoundFromOp translates a proven `#x OP c` comparison into the inclusive
// integer length floor and/or ceiling it establishes. A strict bound is tightened
// to its integer neighbor. Equality bounds both ends; inequality bounds the
// length only when c is 0 because lengths are non-negative.
func LengthBoundFromOp(op string, c int64) (floor, ceil int64, hasFloor, hasCeil bool) {
	switch op {
	case ">":
		return c + 1, 0, true, false
	case ">=":
		return c, 0, true, false
	case "<":
		return 0, c - 1, false, true
	case "<=":
		return 0, c, false, true
	case "==":
		return c, c, true, true
	case "~=":
		if c == 0 {
			return 1, 0, true, false
		}
		return 0, 0, false, false
	default:
		return 0, 0, false, false
	}
}
