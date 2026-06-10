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

// PointNumericIsUnsat reports whether the point's numeric axis is unreachable.
func PointNumericIsUnsat(state *PointState) bool {
	return state != nil && state.Num != nil && state.Num.IsUnsat()
}

// PointNumericCheckUnsat runs the numeric domain's consistency check and
// reports whether the point's numeric axis is unreachable.
func PointNumericCheckUnsat(state *PointState) bool {
	return state != nil && state.Num != nil && !state.Num.CheckSatisfiability()
}

// PointNumericIsReachable reports whether the point's numeric axis admits at
// least one numeric state.
func PointNumericIsReachable(state *PointState) bool {
	return !PointNumericIsUnsat(state)
}

// PointNumericHasState reports whether the point carries a materialized numeric
// axis, including the unreachable numeric bottom.
func PointNumericHasState(state *PointState) bool {
	return state != nil && state.Num != nil
}

func pointNumericState(state *PointState) *numeric.State {
	if state == nil {
		return nil
	}
	return state.Num
}

// PointNumericHasReachableState reports whether the point carries a materialized
// numeric axis that is not the unreachable numeric bottom.
func PointNumericHasReachableState(state *PointState) bool {
	return PointNumericHasState(state) && !state.Num.IsUnsat()
}

// EnsurePointNumericReachableState materializes the reachable empty numeric
// state for entry seeding when the axis is absent or unreachable.
func EnsurePointNumericReachableState(state *PointState) bool {
	if state == nil {
		return false
	}
	if state.Num != nil && !state.Num.IsUnsat() {
		return false
	}
	state.Num = numeric.NewState()
	return true
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

// NumericLengthBoundContainerOps translates a proven `#container OP c`
// comparison into primitive numeric atoms for the container identity.
func NumericLengthBoundContainerOps(container ContainerRef, op string, c int64) []NumericOp {
	if !container.IsValid() {
		return nil
	}
	return NumericLengthBoundOps(container.pathKey(), op, c)
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

// NumericDropLenBoundSymbolOp materializes a length-bound reset for a bare
// symbol slot.
func NumericDropLenBoundSymbolOp(sym cfg.SymbolID) (NumericOp, bool) {
	ref, ok := ContainerRefOfSymbol(sym)
	if !ok {
		return NumericOp{}, false
	}
	return NumericDropLenBoundContainerOp(ref)
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

// NumericVarEqConstSymbolOp materializes `sym == c`.
func NumericVarEqConstSymbolOp(sym cfg.SymbolID, c int64) (NumericOp, bool) {
	key, ok := NumericVarKeyOfSymbol(sym)
	if !ok {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericVarEqConst, Key: key, Const: c}, true
}

// NumericVarLeLenOffsetContainerOp materializes `sym <= len(container) + offset`.
func NumericVarLeLenOffsetContainerOp(sym cfg.SymbolID, container ContainerRef, offset int64) (NumericOp, bool) {
	key, ok := NumericVarKeyOfSymbol(sym)
	if !ok || !container.IsValid() {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericVarLeLenOffset, Key: key, Other: container.pathKey(), Offset: offset}, true
}

// NumericLenGeConstSymbolOp materializes `len(sym) >= lower` for a bare symbol
// container.
func NumericLenGeConstSymbolOp(sym cfg.SymbolID, lower int64) (NumericOp, bool) {
	ref, ok := ContainerRefOfSymbol(sym)
	if !ok {
		return NumericOp{}, false
	}
	return NumericLenGeConstContainerOp(ref, lower)
}

// NumericIncrementLenLowerSymbolOp materializes `len(sym) += delta` on the
// current lower bound for a bare symbol slot.
func NumericIncrementLenLowerSymbolOp(sym cfg.SymbolID, delta int64) (NumericOp, bool) {
	ref, ok := ContainerRefOfSymbol(sym)
	if !ok {
		return NumericOp{}, false
	}
	return NumericIncrementLenLowerContainerOp(ref, delta)
}

// NumericDropLenBoundContainerOp materializes a length-bound reset for container.
func NumericDropLenBoundContainerOp(container ContainerRef) (NumericOp, bool) {
	if !container.IsValid() {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericDropLenBound, Key: container.pathKey()}, true
}

// NumericLenGeConstContainerOp materializes `len(container) >= lower`.
func NumericLenGeConstContainerOp(container ContainerRef, lower int64) (NumericOp, bool) {
	if !container.IsValid() {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericLenGeConst, Key: container.pathKey(), Const: lower}, true
}

// NumericLenGeConstLocalOp materializes `len(target) >= lower` for a point-local
// SSA identity. Use this for proofs that must not survive a reassignment.
func NumericLenGeConstLocalOp(target LocalAddress, lower int64) (NumericOp, bool) {
	key := target.Key()
	if key == "" {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericLenGeConst, Key: key, Const: lower}, true
}

// NumericIncrementLenLowerContainerOp materializes `len(container) += delta`.
func NumericIncrementLenLowerContainerOp(container ContainerRef, delta int64) (NumericOp, bool) {
	if !container.IsValid() || delta <= 0 {
		return NumericOp{}, false
	}
	return NumericOp{Kind: NumericIncrementLenLower, Key: container.pathKey(), Delta: delta}, true
}

// NumericLenGeConstPathOp materializes a resolved path length floor as the
// primitive numeric atom stored in PointState.Num.
func NumericLenGeConstPathOp(path constraint.Path, lower int64) (NumericOp, bool) {
	ref, ok := ContainerRefOfPath(path)
	if !ok {
		return NumericOp{}, false
	}
	return NumericLenGeConstContainerOp(ref, lower)
}

// NumericLengthBoundAddress is a point-local numeric length interval keyed by
// the shared stable-address vocabulary.
type NumericLengthBoundAddress struct {
	Target StableAddress
	Lower  int64
	Upper  int64
}

// PointNumericLenBoundsForContainer reads the numeric length interval for
// container from a point state.
func PointNumericLenBoundsForContainer(state *PointState, container ContainerRef) (lower, upper int64, ok bool) {
	if state == nil || PointNumericIsUnsat(state) {
		return 0, 0, false
	}
	return numericLenBoundsForContainer(state.Num, container)
}

func numericLenBoundsForContainer(num *numeric.State, container ContainerRef) (lower, upper int64, ok bool) {
	if num == nil || !container.IsValid() {
		return 0, 0, false
	}
	return num.LenBoundsFor(container.pathKey())
}

// ForEachPointNumericLenBoundAddress visits numeric length facts through the
// shared stable-address vocabulary. Non-container numeric keys remain private to
// the numeric domain and are not exported as boundary container proofs.
func ForEachPointNumericLenBoundAddress(state *PointState, fn func(target StableAddress, lower, upper int64) bool) {
	if state == nil || PointNumericIsUnsat(state) {
		return
	}
	forEachNumericLenBoundAddress(state.Num, fn)
}

// PointNumericLengthBoundAddresses returns all point-local numeric length facts
// in stable-address form.
func PointNumericLengthBoundAddresses(state *PointState) []NumericLengthBoundAddress {
	var out []NumericLengthBoundAddress
	ForEachPointNumericLenBoundAddress(state, func(target StableAddress, lower, upper int64) bool {
		out = append(out, NumericLengthBoundAddress{Target: target, Lower: lower, Upper: upper})
		return true
	})
	return out
}

func forEachNumericLenBoundAddress(num *numeric.State, fn func(target StableAddress, lower, upper int64) bool) {
	if num == nil || num.IsUnsat() || fn == nil {
		return
	}
	num.ForEachLenBound(func(key constraint.PathKey, lower, upper int64) bool {
		container, ok := containerRefOfKey(key)
		if !ok {
			return true
		}
		target, ok := container.StableAddress()
		if !ok {
			return true
		}
		return fn(target, lower, upper)
	})
}

// PointNumericBoundsForWithTheory reads scalar bounds for key using numeric
// theory inference.
func PointNumericBoundsForWithTheory(state *PointState, key constraint.PathKey) (lower, upper int64, ok bool) {
	if state == nil || PointNumericIsUnsat(state) || key == "" {
		return 0, 0, false
	}
	return numeric.BoundsForWithTheory(state.Num, key)
}

// PointNumericBoundsFor reads direct scalar bounds for key without deriving
// additional theory consequences.
func PointNumericBoundsFor(state *PointState, key constraint.PathKey) (lower, upper int64, ok bool) {
	if state == nil || PointNumericIsUnsat(state) || key == "" {
		return 0, 0, false
	}
	return state.Num.BoundsFor(key)
}

// PointNumericLenRefWithOffsetForVar reports the container length reference
// currently bounding sym, if the numeric domain carries one.
func PointNumericLenRefWithOffsetForVar(state *PointState, sym cfg.SymbolID) (ContainerRef, int64, bool) {
	if state == nil || PointNumericIsUnsat(state) {
		return ContainerRef{}, 0, false
	}
	return numericLenRefWithOffsetForVar(state.Num, sym)
}

// PointNumericLenRefRootSymbolWithOffsetForVar reports a bare-symbol length
// reference currently bounding sym. Nested container refs are intentionally not
// collapsed to their root symbol.
func PointNumericLenRefRootSymbolWithOffsetForVar(state *PointState, sym cfg.SymbolID) (cfg.SymbolID, int64, bool) {
	ref, offset, ok := PointNumericLenRefWithOffsetForVar(state, sym)
	if !ok {
		return 0, 0, false
	}
	root, segments, ok := ParseSymbolPathKey(ref.pathKey())
	if !ok || len(segments) != 0 {
		return 0, 0, false
	}
	return root, offset, true
}

func numericLenRefWithOffsetForVar(num *numeric.State, sym cfg.SymbolID) (ContainerRef, int64, bool) {
	if num == nil {
		return ContainerRef{}, 0, false
	}
	idxKey, ok := NumericVarKeyOfSymbol(sym)
	if !ok {
		return ContainerRef{}, 0, false
	}
	refKey, offset, ok := num.LenRefWithOffsetFor(idxKey)
	if !ok {
		return ContainerRef{}, 0, false
	}
	ref, ok := containerRefOfKey(refKey)
	return ref, offset, ok
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
