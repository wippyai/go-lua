package transformer

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

var errDecisionMalformed = errors.New("transformer: malformed decision diagram")

// rankStructuralGuardAtoms is the sole variable-order authority for Boolean
// coverage and guarded semantic worlds.  Arena IDs are never an ordering
// tiebreaker: an unequal canonical collision is rejected instead of making
// relation identity depend on construction order.
func rankStructuralGuardAtoms(arena *Arena, atoms []ValueTerm, names map[ValueTerm]string, ranks map[ValueTerm]uint32) error {
	if arena == nil || names == nil || ranks == nil {
		return errDecisionMalformed
	}
	for _, atom := range atoms {
		if _, ok := names[atom]; !ok {
			names[atom] = arena.canonicalValue(atom)
		}
	}
	sort.Slice(atoms, func(i, j int) bool { return names[atoms[i]] < names[atoms[j]] })
	for index, atom := range atoms {
		if index != 0 && names[atoms[index-1]] == names[atom] && atoms[index-1] != atom {
			return fmt.Errorf("transformer: guard atom canonical identity collision")
		}
		ranks[atom] = uint32(index)
	}
	return nil
}

// decisionRef identifies either an immutable terminal or a reduced ordered
// decision node.  The same kernel owns Boolean guard proofs and multi-terminal
// semantic worlds; clients supply only the terminal algebra.
type decisionRef uint32
type decisionLeaf uint32

const (
	decisionFalse decisionRef = iota
	decisionTrue
)

type decisionBooleanOp uint8

const (
	decisionAnd decisionBooleanOp = iota + 1
	decisionOr
	decisionNot
)

type decisionNode struct {
	terminal    bool
	hasZeroLeaf bool
	hasOneLeaf  bool
	leaf        decisionLeaf
	variable    uint32
	low         decisionRef
	high        decisionRef
}

type decisionUniqueKey struct {
	variable uint32
	low      decisionRef
	high     decisionRef
}

type decisionApplyKey struct {
	op          uint8
	left, right decisionRef
}

type decisionMapKey struct {
	op    uint8
	value decisionRef
}

type guardedConditionKey struct {
	condition           decisionRef
	whenTrue, whenFalse decisionRef
}

// decisionCareKey identifies the canonical representative of value on the
// valuations admitted by care. Values outside care are not observations of
// the reduced product and therefore must not influence the representative.
type decisionCareKey struct {
	care  decisionRef
	value decisionRef
}

type decisionCareApplyKey struct {
	op               uint8
	leftCare, left   decisionRef
	rightCare, right decisionRef
}

type decisionKernel struct {
	nodes         []decisionNode
	terminals     map[decisionLeaf]decisionRef
	unique        map[decisionUniqueKey]decisionRef
	applyMemo     map[decisionApplyKey]decisionRef
	mapMemo       map[decisionMapKey]decisionRef
	iteMemo       map[guardedConditionKey]decisionRef
	careMemo      map[decisionCareKey]decisionRef
	careApplyMemo map[decisionCareApplyKey]decisionRef
	applyOps      uint64
}

type decisionCheckpoint struct {
	nodes    int
	applyOps uint64
}

func (k *decisionKernel) checkpoint() decisionCheckpoint {
	return decisionCheckpoint{nodes: len(k.nodes), applyOps: k.applyOps}
}

// rollback is used only on canceled/malformed unpublished transactions.  The
// success path pays no copy tax; rollback removes every memo entry that could
// name an appended node before truncating the arena.
func (k *decisionKernel) rollback(mark decisionCheckpoint) {
	for leaf, ref := range k.terminals {
		if int(ref) >= mark.nodes {
			delete(k.terminals, leaf)
		}
	}
	for key, ref := range k.unique {
		if int(ref) >= mark.nodes || int(key.low) >= mark.nodes || int(key.high) >= mark.nodes {
			delete(k.unique, key)
		}
	}
	for key, ref := range k.applyMemo {
		if int(ref) >= mark.nodes || int(key.left) >= mark.nodes || int(key.right) >= mark.nodes {
			delete(k.applyMemo, key)
		}
	}
	for key, ref := range k.mapMemo {
		if int(ref) >= mark.nodes || int(key.value) >= mark.nodes {
			delete(k.mapMemo, key)
		}
	}
	for key, ref := range k.iteMemo {
		if int(ref) >= mark.nodes || int(key.condition) >= mark.nodes || int(key.whenTrue) >= mark.nodes || int(key.whenFalse) >= mark.nodes {
			delete(k.iteMemo, key)
		}
	}
	for key, ref := range k.careMemo {
		if int(ref) >= mark.nodes || int(key.care) >= mark.nodes || int(key.value) >= mark.nodes {
			delete(k.careMemo, key)
		}
	}
	for key, ref := range k.careApplyMemo {
		if int(ref) >= mark.nodes || int(key.leftCare) >= mark.nodes || int(key.left) >= mark.nodes || int(key.rightCare) >= mark.nodes || int(key.right) >= mark.nodes {
			delete(k.careApplyMemo, key)
		}
	}
	k.nodes = k.nodes[:mark.nodes]
	k.applyOps = mark.applyOps
}

func newDecisionKernel() decisionKernel {
	return decisionKernel{
		terminals:     make(map[decisionLeaf]decisionRef),
		unique:        make(map[decisionUniqueKey]decisionRef),
		applyMemo:     make(map[decisionApplyKey]decisionRef),
		mapMemo:       make(map[decisionMapKey]decisionRef),
		iteMemo:       make(map[guardedConditionKey]decisionRef),
		careMemo:      make(map[decisionCareKey]decisionRef),
		careApplyMemo: make(map[decisionCareApplyKey]decisionRef),
	}
}

func (k *decisionKernel) resetBoolean() {
	k.nodes = k.nodes[:0]
	for key := range k.terminals {
		delete(k.terminals, key)
	}
	for key := range k.unique {
		delete(k.unique, key)
	}
	for key := range k.applyMemo {
		delete(k.applyMemo, key)
	}
	for key := range k.mapMemo {
		delete(k.mapMemo, key)
	}
	for key := range k.iteMemo {
		delete(k.iteMemo, key)
	}
	for key := range k.careMemo {
		delete(k.careMemo, key)
	}
	for key := range k.careApplyMemo {
		delete(k.careApplyMemo, key)
	}
	k.applyOps = 0
	// Boolean terminal references deliberately retain the established 0/1
	// representation consumed by evaluated.WorldProof.
	k.nodes = append(k.nodes,
		decisionNode{terminal: true, hasZeroLeaf: true, leaf: 0},
		decisionNode{terminal: true, hasOneLeaf: true, leaf: 1},
	)
	k.terminals[0], k.terminals[1] = decisionFalse, decisionTrue
}

// condition constructs the canonical multi-terminal ITE without recursion.
// condition is a Boolean DD; the two branches may be any terminal algebra
// owned by this kernel. The shared unique table supplies reduction.
func (k *decisionKernel) condition(ctx context.Context, condition, whenTrue, whenFalse decisionRef) (decisionRef, error) {
	if ctx == nil {
		return 0, errDecisionMalformed
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	root := guardedConditionKey{condition: condition, whenTrue: whenTrue, whenFalse: whenFalse}
	if result, present := k.iteMemo[root]; present {
		return result, nil
	}
	type frame struct {
		key       guardedConditionKey
		expanded  bool
		variable  uint32
		low, high guardedConditionKey
	}
	stack := []frame{{key: root}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := k.iteMemo[current.key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if int(current.key.condition) >= len(k.nodes) || int(current.key.whenTrue) >= len(k.nodes) || int(current.key.whenFalse) >= len(k.nodes) {
			return 0, errDecisionMalformed
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		if current.key.whenTrue == current.key.whenFalse {
			k.iteMemo[current.key] = current.key.whenTrue
			stack = stack[:len(stack)-1]
			continue
		}
		conditionNode := k.nodes[current.key.condition]
		if conditionNode.terminal {
			switch conditionNode.leaf {
			case 0:
				k.iteMemo[current.key] = current.key.whenFalse
			case 1:
				k.iteMemo[current.key] = current.key.whenTrue
			default:
				return 0, errDecisionMalformed
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if !current.expanded {
			current.expanded = true
			current.variable = conditionNode.variable
			trueNode, falseNode := k.nodes[current.key.whenTrue], k.nodes[current.key.whenFalse]
			if !trueNode.terminal && trueNode.variable < current.variable {
				current.variable = trueNode.variable
			}
			if !falseNode.terminal && falseNode.variable < current.variable {
				current.variable = falseNode.variable
			}
			conditionLow, conditionHigh := current.key.condition, current.key.condition
			if conditionNode.variable == current.variable {
				conditionLow, conditionHigh = conditionNode.low, conditionNode.high
			}
			trueLow, trueHigh := current.key.whenTrue, current.key.whenTrue
			if !trueNode.terminal && trueNode.variable == current.variable {
				trueLow, trueHigh = trueNode.low, trueNode.high
			}
			falseLow, falseHigh := current.key.whenFalse, current.key.whenFalse
			if !falseNode.terminal && falseNode.variable == current.variable {
				falseLow, falseHigh = falseNode.low, falseNode.high
			}
			current.low = guardedConditionKey{condition: conditionLow, whenTrue: trueLow, whenFalse: falseLow}
			current.high = guardedConditionKey{condition: conditionHigh, whenTrue: trueHigh, whenFalse: falseHigh}
		}
		if _, done := k.iteMemo[current.low]; !done {
			stack = append(stack, frame{key: current.low})
			continue
		}
		if _, done := k.iteMemo[current.high]; !done {
			stack = append(stack, frame{key: current.high})
			continue
		}
		k.iteMemo[current.key] = k.branch(current.variable, k.iteMemo[current.low], k.iteMemo[current.high])
		stack = stack[:len(stack)-1]
	}
	return k.iteMemo[root], nil
}

// restrict returns the canonical generalized cofactor of value under care.
// It agrees with value on every valuation where care is true. A branch on
// which care is false is replaced by its live sibling, so neither the shape
// nor the leaves of value outside care survive in the result. This is the
// decision-diagram form of quotienting a reduced product by reachability; it
// does not manufacture ITE(care, value, bottom) nodes.
//
// An empty care set has no typed representative at this layer (value may be a
// Boolean, lane, coordinate, Values, or diagnostic DD), so callers must map a
// globally-false care set to their registered product Bottom.
func (k *decisionKernel) restrict(ctx context.Context, care, value decisionRef) (decisionRef, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int(care) >= len(k.nodes) || int(value) >= len(k.nodes) || care == decisionFalse {
		return 0, errDecisionMalformed
	}
	type frame struct {
		key               decisionCareKey
		expanded          bool
		variable          uint32
		low, high         decisionCareKey
		lowLive, highLive bool
	}
	root := decisionCareKey{care: care, value: value}
	if result, ok := k.careMemo[root]; ok {
		return result, nil
	}
	stack := []frame{{key: root}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := k.careMemo[current.key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if int(current.key.care) >= len(k.nodes) || int(current.key.value) >= len(k.nodes) {
			return 0, errDecisionMalformed
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		careNode, valueNode := k.nodes[current.key.care], k.nodes[current.key.value]
		if careNode.terminal {
			if careNode.leaf != 1 {
				return 0, errDecisionMalformed
			}
			k.careMemo[current.key] = current.key.value
			stack = stack[:len(stack)-1]
			continue
		}
		if !current.expanded {
			current.expanded = true
			current.variable = careNode.variable
			if !valueNode.terminal && valueNode.variable < current.variable {
				current.variable = valueNode.variable
			}
			careLow, careHigh := current.key.care, current.key.care
			if careNode.variable == current.variable {
				careLow, careHigh = careNode.low, careNode.high
			}
			valueLow, valueHigh := current.key.value, current.key.value
			if !valueNode.terminal && valueNode.variable == current.variable {
				valueLow, valueHigh = valueNode.low, valueNode.high
			}
			current.lowLive, current.highLive = careLow != decisionFalse, careHigh != decisionFalse
			if !current.lowLive && !current.highLive {
				return 0, errDecisionMalformed
			}
			current.low = decisionCareKey{care: careLow, value: valueLow}
			current.high = decisionCareKey{care: careHigh, value: valueHigh}
		}
		if current.lowLive {
			if _, done := k.careMemo[current.low]; !done {
				stack = append(stack, frame{key: current.low})
				continue
			}
		}
		if current.highLive {
			if _, done := k.careMemo[current.high]; !done {
				stack = append(stack, frame{key: current.high})
				continue
			}
		}
		switch {
		case !current.lowLive:
			k.careMemo[current.key] = k.careMemo[current.high]
		case !current.highLive:
			k.careMemo[current.key] = k.careMemo[current.low]
		default:
			k.careMemo[current.key] = k.branch(current.variable, k.careMemo[current.low], k.careMemo[current.high])
		}
		stack = stack[:len(stack)-1]
	}
	return k.careMemo[root], nil
}

// applyUnderCare is the relational product of two component DDs with their
// reachability certificates. Where only one operand is reachable it returns
// that operand without invoking the lattice operator; where both are
// reachable it applies the operator; where neither is reachable it quotients
// the payload away. It is therefore the identity-correct join/widen primitive
// for a reachability reduced product without first constructing per-component
// Bottom masks.
func (k *decisionKernel) applyUnderCare(
	ctx context.Context,
	op uint8,
	commutative bool,
	leftCare, left, rightCare, right decisionRef,
	leaves decisionLeafBinary,
) (decisionRef, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int(leftCare) >= len(k.nodes) || int(left) >= len(k.nodes) || int(rightCare) >= len(k.nodes) || int(right) >= len(k.nodes) || leaves == nil {
		return 0, errDecisionMalformed
	}
	normalize := func(leftCare, left, rightCare, right decisionRef) decisionCareApplyKey {
		if commutative && (rightCare < leftCare || rightCare == leftCare && right < left) {
			leftCare, left, rightCare, right = rightCare, right, leftCare, left
		}
		return decisionCareApplyKey{op: op, leftCare: leftCare, left: left, rightCare: rightCare, right: right}
	}
	type frame struct {
		key               decisionCareApplyKey
		expanded          bool
		variable          uint32
		low, high         decisionCareApplyKey
		lowLive, highLive bool
	}
	root := normalize(leftCare, left, rightCare, right)
	if root.leftCare == decisionFalse && root.rightCare == decisionFalse {
		return 0, errDecisionMalformed
	}
	if result, ok := k.careApplyMemo[root]; ok {
		return result, nil
	}
	stack := []frame{{key: root}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := k.careApplyMemo[current.key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		key := current.key
		if int(key.leftCare) >= len(k.nodes) || int(key.left) >= len(k.nodes) || int(key.rightCare) >= len(k.nodes) || int(key.right) >= len(k.nodes) {
			return 0, errDecisionMalformed
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		leftCareNode, rightCareNode := k.nodes[key.leftCare], k.nodes[key.rightCare]
		if leftCareNode.terminal && leftCareNode.leaf > 1 || rightCareNode.terminal && rightCareNode.leaf > 1 {
			return 0, errDecisionMalformed
		}
		if key.leftCare == decisionTrue && key.rightCare == decisionFalse {
			k.careApplyMemo[key] = key.left
			stack = stack[:len(stack)-1]
			continue
		}
		if key.leftCare == decisionFalse && key.rightCare == decisionTrue {
			k.careApplyMemo[key] = key.right
			stack = stack[:len(stack)-1]
			continue
		}
		if key.leftCare == decisionTrue && key.rightCare == decisionTrue {
			result, err := k.apply(ctx, op, commutative, key.left, key.right, leaves)
			if err != nil {
				return 0, err
			}
			k.careApplyMemo[key] = result
			stack = stack[:len(stack)-1]
			continue
		}
		if !current.expanded {
			current.expanded = true
			current.variable = ^uint32(0)
			for _, node := range []decisionNode{leftCareNode, k.nodes[key.left], rightCareNode, k.nodes[key.right]} {
				if !node.terminal && node.variable < current.variable {
					current.variable = node.variable
				}
			}
			if current.variable == ^uint32(0) {
				return 0, errDecisionMalformed
			}
			split := func(ref decisionRef, node decisionNode) (decisionRef, decisionRef) {
				if !node.terminal && node.variable == current.variable {
					return node.low, node.high
				}
				return ref, ref
			}
			lcl, lch := split(key.leftCare, leftCareNode)
			ll, lh := split(key.left, k.nodes[key.left])
			rcl, rch := split(key.rightCare, rightCareNode)
			rl, rh := split(key.right, k.nodes[key.right])
			current.lowLive, current.highLive = lcl != decisionFalse || rcl != decisionFalse, lch != decisionFalse || rch != decisionFalse
			if !current.lowLive && !current.highLive {
				return 0, errDecisionMalformed
			}
			current.low = normalize(lcl, ll, rcl, rl)
			current.high = normalize(lch, lh, rch, rh)
		}
		if current.lowLive {
			if _, done := k.careApplyMemo[current.low]; !done {
				stack = append(stack, frame{key: current.low})
				continue
			}
		}
		if current.highLive {
			if _, done := k.careApplyMemo[current.high]; !done {
				stack = append(stack, frame{key: current.high})
				continue
			}
		}
		switch {
		case !current.lowLive:
			k.careApplyMemo[key] = k.careApplyMemo[current.high]
		case !current.highLive:
			k.careApplyMemo[key] = k.careApplyMemo[current.low]
		default:
			k.careApplyMemo[key] = k.branch(current.variable, k.careApplyMemo[current.low], k.careApplyMemo[current.high])
		}
		stack = stack[:len(stack)-1]
	}
	return k.careApplyMemo[root], nil
}

func (k *decisionKernel) terminal(leaf decisionLeaf) decisionRef {
	if prior, ok := k.terminals[leaf]; ok {
		return prior
	}
	ref := decisionRef(len(k.nodes))
	k.nodes = append(k.nodes, decisionNode{terminal: true, hasZeroLeaf: leaf == 0, hasOneLeaf: leaf == 1, leaf: leaf})
	k.terminals[leaf] = ref
	return ref
}

func (k *decisionKernel) branch(variable uint32, low, high decisionRef) decisionRef {
	if low == high {
		return low
	}
	key := decisionUniqueKey{variable: variable, low: low, high: high}
	if prior, ok := k.unique[key]; ok {
		return prior
	}
	ref := decisionRef(len(k.nodes))
	k.nodes = append(k.nodes, decisionNode{
		hasZeroLeaf: k.nodes[low].hasZeroLeaf || k.nodes[high].hasZeroLeaf,
		hasOneLeaf:  k.nodes[low].hasOneLeaf || k.nodes[high].hasOneLeaf,
		variable:    variable, low: low, high: high,
	})
	k.unique[key] = ref
	return ref
}

type decisionLeafBinary func(decisionLeaf, decisionLeaf) (decisionLeaf, error)

func (k *decisionKernel) reduce(ctx context.Context, op uint8, identity decisionRef, commutative bool, values []decisionRef, leaves decisionLeafBinary) (decisionRef, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return identity, nil
	}
	work := append([]decisionRef(nil), values...)
	for width := len(work); width > 1; {
		write := 0
		for read := 0; read < width; read += 2 {
			if read+1 == width {
				work[write] = work[read]
			} else {
				joined, err := k.apply(ctx, op, commutative, work[read], work[read+1], leaves)
				if err != nil {
					return 0, err
				}
				work[write] = joined
			}
			write++
		}
		width = write
	}
	return work[0], nil
}

// apply aligns ordered atoms and applies the client's exact terminal algebra.
// There is no semantic budget: finiteness follows from the two finite input
// DAGs and memoization over their Cartesian product.
func (k *decisionKernel) apply(ctx context.Context, op uint8, commutative bool, left, right decisionRef, leaves decisionLeafBinary) (decisionRef, error) {
	return k.applyWithMemo(ctx, op, commutative, false, left, right, leaves, k.applyMemo)
}

// applyScoped aligns two DDs for a terminal algebra whose meaning is captured
// by the caller (for example a particular registered coordinate slot).  Its
// structural memo is transaction-local, so two different captured algebras
// can never alias through the kernel-wide opcode memo.
func (k *decisionKernel) applyScoped(ctx context.Context, commutative bool, left, right decisionRef, leaves decisionLeafBinary) (decisionRef, error) {
	return k.applyWithMemo(ctx, 0, commutative, false, left, right, leaves, make(map[decisionApplyKey]decisionRef))
}

// applyScopedLeftZero is the exact masked zipper. A left terminal 0 is an
// absorbing result, so the right subtree is semantically unobserved and is
// never traversed. Coordinate normalization uses it to preserve an implicit
// default without walking the family skeleton hidden behind that tag.
func (k *decisionKernel) applyScopedLeftZero(ctx context.Context, left, right decisionRef, leaves decisionLeafBinary) (decisionRef, error) {
	return k.applyWithMemo(ctx, 0, false, true, left, right, leaves, make(map[decisionApplyKey]decisionRef))
}

func (k *decisionKernel) applyWithMemo(ctx context.Context, op uint8, commutative, leftZeroAbsorbing bool, left, right decisionRef, leaves decisionLeafBinary, memo map[decisionApplyKey]decisionRef) (decisionRef, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int(left) >= len(k.nodes) || int(right) >= len(k.nodes) || leaves == nil {
		return 0, errDecisionMalformed
	}
	normalize := func(left, right decisionRef) decisionApplyKey {
		if commutative && right < left {
			left, right = right, left
		}
		return decisionApplyKey{op: op, left: left, right: right}
	}
	type frame struct {
		key       decisionApplyKey
		expanded  bool
		variable  uint32
		low, high decisionApplyKey
	}
	root := normalize(left, right)
	stack := []frame{{key: root}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := memo[current.key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if int(current.key.left) >= len(k.nodes) || int(current.key.right) >= len(k.nodes) {
			return 0, errDecisionMalformed
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		ln, rn := k.nodes[current.key.left], k.nodes[current.key.right]
		if leftZeroAbsorbing && ln.terminal && ln.leaf == 0 {
			memo[current.key] = decisionFalse
			stack = stack[:len(stack)-1]
			continue
		}
		if ln.terminal && rn.terminal {
			leaf, err := leaves(ln.leaf, rn.leaf)
			if err != nil {
				return 0, err
			}
			memo[current.key] = k.terminal(leaf)
			stack = stack[:len(stack)-1]
			continue
		}
		if !current.expanded {
			variable := ^uint32(0)
			if !ln.terminal {
				variable = ln.variable
			}
			if !rn.terminal && rn.variable < variable {
				variable = rn.variable
			}
			ll, lh := current.key.left, current.key.left
			if !ln.terminal && ln.variable == variable {
				ll, lh = ln.low, ln.high
			}
			rl, rh := current.key.right, current.key.right
			if !rn.terminal && rn.variable == variable {
				rl, rh = rn.low, rn.high
			}
			current.expanded, current.variable = true, variable
			current.low, current.high = normalize(ll, rl), normalize(lh, rh)
		}
		if _, done := memo[current.low]; !done {
			stack = append(stack, frame{key: current.low})
			continue
		}
		if _, done := memo[current.high]; !done {
			stack = append(stack, frame{key: current.high})
			continue
		}
		memo[current.key] = k.branch(current.variable, memo[current.low], memo[current.high])
		stack = stack[:len(stack)-1]
	}
	return memo[root], nil
}

type decisionLeafUnary func(decisionLeaf) (decisionLeaf, error)

func (k *decisionKernel) mapLeaves(ctx context.Context, op uint8, value decisionRef, leafOp decisionLeafUnary) (decisionRef, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int(value) >= len(k.nodes) || leafOp == nil {
		return 0, errDecisionMalformed
	}
	type frame struct {
		ref      decisionRef
		expanded bool
	}
	root := decisionMapKey{op: op, value: value}
	stack := []frame{{ref: value}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		key := decisionMapKey{op: op, value: current.ref}
		if _, done := k.mapMemo[key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if int(current.ref) >= len(k.nodes) {
			return 0, errDecisionMalformed
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		node := k.nodes[current.ref]
		if node.terminal {
			leaf, err := leafOp(node.leaf)
			if err != nil {
				return 0, err
			}
			k.mapMemo[key] = k.terminal(leaf)
			stack = stack[:len(stack)-1]
			continue
		}
		if !current.expanded {
			current.expanded = true
		}
		lowKey, highKey := decisionMapKey{op: op, value: node.low}, decisionMapKey{op: op, value: node.high}
		if _, done := k.mapMemo[lowKey]; !done {
			stack = append(stack, frame{ref: node.low})
			continue
		}
		if _, done := k.mapMemo[highKey]; !done {
			stack = append(stack, frame{ref: node.high})
			continue
		}
		k.mapMemo[key] = k.branch(node.variable, k.mapMemo[lowKey], k.mapMemo[highKey])
		stack = stack[:len(stack)-1]
	}
	return k.mapMemo[root], nil
}

// mapLeavesTransient applies one query-specific semantic quotient without
// retaining an operation-keyed cache. The unique table still canonicalizes
// the output, so branches whose projected leaves become equal disappear
// before any dependent tuple alignment.
func (k *decisionKernel) mapLeavesTransient(ctx context.Context, value decisionRef, leafOp decisionLeafUnary) (decisionRef, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if int(value) >= len(k.nodes) || leafOp == nil {
		return 0, errDecisionMalformed
	}
	type frame struct {
		ref      decisionRef
		expanded bool
	}
	mapped := make(map[decisionRef]decisionRef)
	stack := []frame{{ref: value}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := mapped[current.ref]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if int(current.ref) >= len(k.nodes) {
			return 0, errDecisionMalformed
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		node := k.nodes[current.ref]
		if node.terminal {
			leaf, err := leafOp(node.leaf)
			if err != nil {
				return 0, err
			}
			mapped[current.ref] = k.terminal(leaf)
			stack = stack[:len(stack)-1]
			continue
		}
		if !current.expanded {
			current.expanded = true
		}
		if _, done := mapped[node.low]; !done {
			stack = append(stack, frame{ref: node.low})
			continue
		}
		if _, done := mapped[node.high]; !done {
			stack = append(stack, frame{ref: node.high})
			continue
		}
		mapped[current.ref] = k.branch(node.variable, mapped[node.low], mapped[node.high])
		stack = stack[:len(stack)-1]
	}
	return mapped[value], nil
}

func (k *decisionKernel) node(ref decisionRef) (decisionNode, bool) {
	if int(ref) >= len(k.nodes) {
		return decisionNode{}, false
	}
	return k.nodes[ref], true
}
