package transformer

import (
	"context"
	"fmt"
)

// formalTupleGuardJoinOp is private to tuple-boundary existential closure.
// decisionKernel memo keys include the operation tag, so product Join must not
// alias the Boolean And/Or tags used by Care.
const formalTupleGuardJoinOp uint8 = 0xf1

type formalDecisionVectorID uint32

// formalDecisionVectorInterner gives the synchronized group traversal a
// collision-safe dense key without byte/string materialization. Hashes select
// a bucket only; exact vector equality remains the identity proof.
type formalDecisionVectorInterner struct {
	vectors [][]decisionRef
	buckets map[uint64][]formalDecisionVectorID
}

func newFormalDecisionVectorInterner() *formalDecisionVectorInterner {
	return &formalDecisionVectorInterner{vectors: make([][]decisionRef, 1), buckets: make(map[uint64][]formalDecisionVectorID)}
}

func (i *formalDecisionVectorInterner) intern(values []decisionRef) (formalDecisionVectorID, error) {
	if i == nil || len(values) == 0 || uint64(len(i.vectors)) > uint64(^formalDecisionVectorID(0)) {
		return 0, errDecisionMalformed
	}
	hash := uint64(0x9e3779b97f4a7c15) ^ uint64(len(values))
	for _, value := range values {
		hash ^= uint64(value) + 0x9e3779b97f4a7c15 + hash<<6 + hash>>2
	}
	for _, id := range i.buckets[hash] {
		prior := i.vectors[id]
		if len(prior) != len(values) {
			continue
		}
		equal := true
		for index := range prior {
			if prior[index] != values[index] {
				equal = false
				break
			}
		}
		if equal {
			return id, nil
		}
	}
	id := formalDecisionVectorID(len(i.vectors))
	i.vectors = append(i.vectors, values)
	i.buckets[hash] = append(i.buckets[hash], id)
	return id, nil
}

func (i *formalDecisionVectorInterner) vector(id formalDecisionVectorID) ([]decisionRef, bool) {
	if i == nil || id == 0 || int(id) >= len(i.vectors) {
		return nil, false
	}
	return i.vectors[id], true
}

// composeGuardBoundary is the one tuple-level alpha-rename and invocation
// closure transaction. Care and every product component are transformed in
// the same forest-global ROBDD. Product components existentially close through
// their registered Join; Boolean OR is used only for Care.
func (a *formalTupleAlgebra) composeGuardBoundary(tuple formalRelationTuple, boundary formalGuardBoundary) (formalRelationTuple, error) {
	if a == nil || a.ctx == nil || a.program == nil || a.program.formalGuards == nil ||
		!boundary.valid() || boundary.owner != a.program.formalGuards {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple guard boundary is foreign")
	}
	if err := a.ctx.Err(); err != nil {
		return formalRelationTuple{}, err
	}
	if err := a.validateTuple(tuple); err != nil {
		return formalRelationTuple{}, err
	}
	if tuple.bottom() {
		return tuple, nil
	}
	var traceDetail *formalRelationEvalTraceDetail
	if a.evalTrace != nil {
		traceDetail = a.evalTrace.active
	}
	if len(boundary.rename.pairs) == 0 && len(boundary.domain.ranks) == 0 && len(boundary.close.ranks) == 0 {
		// The common guard-free Apply is exact physical identity: no DD,
		// directory, terminal, memo, or validation scratch is allocated.
		return tuple, nil
	}
	span, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple guard composition has foreign tuple ownership")
	}
	descriptors := span.forest.descriptors[span.first : span.first+span.count]
	if len(descriptors) != span.count || len(descriptors) == 0 || descriptors[0].role != formalFiberCare {
		return formalRelationTuple{}, errFormalComponentMalformed
	}

	for _, rank := range boundary.close.ranks {
		if rank >= boundary.owner.size {
			return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple guard boundary has foreign close rank")
		}
	}

	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}

	var readMark formalRelationEvalTracePhaseMark
	if traceDetail != nil {
		readMark = beginFormalRelationEvalTracePhase(a)
	}
	original := make([]decisionRef, span.count)
	for ordinal := range descriptors {
		value, err := directory.valueAt(tuple.root, formalFiberOrdinal(ordinal))
		if err != nil {
			return fail(err)
		}
		original[ordinal] = decisionRef(value)
	}
	if traceDetail != nil {
		finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeRead, readMark)
	}
	mapped := original
	var err error
	if len(boundary.rename.pairs) != 0 || len(boundary.domain.ranks) != 0 {
		var substituteMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			substituteMark = beginFormalRelationEvalTracePhase(a)
		}
		mapped, err = boundary.substituteDecisionVector(a.ctx, &a.decisions, original)
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeSubstitute, substituteMark)
		}
		if err != nil {
			return fail(err)
		}
	}
	closed := mapped
	if len(boundary.close.ranks) != 0 {
		var closeMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			closeMark = beginFormalRelationEvalTracePhase(a)
		}
		closedCare, closeErr := boundary.closeBoolean(a.ctx, &a.decisions, mapped[0])
		if closeErr != nil {
			return fail(closeErr)
		}
		closed = append([]decisionRef(nil), mapped...)
		closed[0] = closedCare

		var groupsMark formalRelationEvalTracePhaseMark
		if traceDetail != nil {
			groupsMark = beginFormalRelationEvalTracePhase(a)
		}
		grouped := make([]bool, span.count)
		groups := span.groupDescriptors()
		for _, group := range groups {
			if !group.valid() || group.variable != tuple.variable {
				return fail(errFormalComponentForeignOwner)
			}
			roots := make([]decisionRef, len(group.members))
			for index, ordinal := range group.members {
				if ordinal == 0 || int(ordinal) >= len(grouped) || grouped[ordinal] {
					return fail(errFormalComponentMalformed)
				}
				grouped[ordinal] = true
				roots[index] = mapped[ordinal]
			}
			localCare, localRoots, localErr := a.closeGuardedDecisionVector(mapped[0], roots, boundary.close,
				func(leftCare decisionRef, left []decisionRef, rightCare decisionRef, right []decisionRef) ([]decisionRef, error) {
					var joinMark formalRelationEvalTracePhaseMark
					if traceDetail != nil {
						joinMark = beginFormalRelationEvalTracePhase(a)
						traceDetail.guardComposeCloseJoins++
					}
					joined, joinErr := a.joinGuardedGroupRoots(span, authority, group, leftCare, left, rightCare, right)
					if traceDetail != nil {
						finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeJoin, joinMark)
					}
					return joined, joinErr
				})
			if localErr != nil {
				return fail(localErr)
			}
			if localCare != closedCare || len(localRoots) != len(group.members) {
				return fail(errDecisionMalformed)
			}
			for index, ordinal := range group.members {
				closed[ordinal] = localRoots[index]
			}
		}
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeGroups, groupsMark)
		}

		for ordinal, descriptor := range descriptors {
			if ordinal == 0 || grouped[ordinal] {
				continue
			}
			localCare, localRoots, localErr := a.closeGuardedDecisionVector(mapped[0], []decisionRef{mapped[ordinal]}, boundary.close,
				func(leftCare decisionRef, left []decisionRef, rightCare decisionRef, right []decisionRef) ([]decisionRef, error) {
					if len(left) != 1 || len(right) != 1 {
						return nil, errDecisionMalformed
					}
					var joinMark formalRelationEvalTracePhaseMark
					if traceDetail != nil {
						joinMark = beginFormalRelationEvalTracePhase(a)
						traceDetail.guardComposeCloseJoins++
					}
					joined, joinErr := a.decisions.applyUnderCare(
						a.ctx, formalTupleGuardJoinOp, true,
						leftCare, left[0], rightCare, right[0],
						func(leftLeaf, rightLeaf decisionLeaf) (decisionLeaf, error) {
							leftLeaf, leafErr := a.componentLeaf(authority, descriptor, leftLeaf)
							if leafErr != nil {
								return 0, leafErr
							}
							rightLeaf, leafErr = a.componentLeaf(authority, descriptor, rightLeaf)
							if leafErr != nil {
								return 0, leafErr
							}
							if leftLeaf < 2 || rightLeaf < 2 {
								return a.combineAbsent(formalComponentJoin, descriptor, leftLeaf, rightLeaf)
							}
							return authority.combine(a.ctx, formalComponentJoin, leftLeaf, rightLeaf)
						},
					)
					if traceDetail != nil {
						finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeJoin, joinMark)
						finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeScalarJoin, joinMark)
					}
					if joinErr != nil {
						return nil, joinErr
					}
					return []decisionRef{joined}, nil
				})
			if localErr != nil {
				return fail(localErr)
			}
			if localCare != closedCare || len(localRoots) != 1 {
				return fail(errDecisionMalformed)
			}
			closed[ordinal] = localRoots[0]
		}
		if traceDetail != nil {
			finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeClose, closeMark)
		}
	}

	var validateMark formalRelationEvalTracePhaseMark
	if traceDetail != nil {
		validateMark = beginFormalRelationEvalTracePhase(a)
	}
	writes := make([]formalFiberWrite, 0, len(descriptors))
	if err := boundary.owner.validateDecisionRootVector(&a.decisions, closed, boundary.close); err != nil {
		return fail(err)
	}
	for ordinal, descriptor := range descriptors {
		root := closed[ordinal]
		if err := a.validateDescriptorRoot(authority, descriptor, root); err != nil {
			return fail(err)
		}
		if root != original[ordinal] {
			writes = append(writes, formalFiberWrite{ordinal: formalFiberOrdinal(ordinal), value: formalFiberValue(root)})
		}
	}
	if traceDetail != nil {
		finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposeValidate, validateMark)
	}
	if len(writes) == 0 {
		a.decisions.rollback(mark)
		// Even an identity guard closure is a producer boundary for Apply. The
		// complete tuple survives unchanged, but its exact lane spellings still
		// need registration before the downstream correlation transaction reads
		// them.
		if err := a.cacheFormalTupleFactorSpellings(tuple); err != nil {
			return fail(err)
		}
		return tuple, nil
	}
	var publishMark formalRelationEvalTracePhaseMark
	if traceDetail != nil {
		publishMark = beginFormalRelationEvalTracePhase(a)
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		return fail(err)
	}
	root, _, err := directory.applyDelta(tuple.root, delta)
	if err != nil {
		return fail(err)
	}
	result := a.normalize(formalRelationTuple{variable: tuple.variable, root: root})
	if err := a.err(); err != nil {
		return fail(err)
	}
	if err := a.validateTuple(result); err != nil {
		return fail(err)
	}
	// Guard closure is a registered product producer. Coordinate families are
	// lifted independently above, so their resulting complete lane spelling
	// does not necessarily coincide with a spelling factored by either input.
	// Register the correlated per-lane rows here, before Apply can consume the
	// closed target; a consumer must never reconstruct a missing spelling.
	if err := a.cacheFormalTupleFactorSpellings(result); err != nil {
		return fail(err)
	}
	if traceDetail != nil {
		finishFormalRelationEvalTracePhase(a, &traceDetail.guardComposePublish, publishMark)
	}
	return result, nil
}

// substituteDecisionVector shares one rewrite memo across the complete tuple.
// A common guard sub-DAG is therefore renamed once even when Care and several
// product components reference it. ITE reconstruction is the same ordered
// operation used by substituteDecision; this is its tuple-width transaction.
func (b formalGuardBoundary) substituteDecisionVector(ctx context.Context, kernel *decisionKernel, roots []decisionRef) ([]decisionRef, error) {
	if ctx == nil || kernel == nil || !b.valid() || len(roots) == 0 {
		return nil, fmt.Errorf("transformer: formal guard vector substitution is unowned")
	}
	type frame struct {
		ref      decisionRef
		expanded bool
	}
	mark := kernel.checkpoint()
	memo := make(map[decisionRef]decisionRef)
	for _, root := range roots {
		if _, done := memo[root]; done {
			continue
		}
		stack := []frame{{ref: root}}
		for len(stack) != 0 {
			current := &stack[len(stack)-1]
			if _, done := memo[current.ref]; done {
				stack = stack[:len(stack)-1]
				continue
			}
			if len(memo)&255 == 0 {
				if err := ctx.Err(); err != nil {
					kernel.rollback(mark)
					return nil, err
				}
			}
			node, ok := kernel.node(current.ref)
			if !ok {
				kernel.rollback(mark)
				return nil, fmt.Errorf("transformer: formal guard vector substitution has foreign node")
			}
			if node.terminal {
				memo[current.ref] = current.ref
				stack = stack[:len(stack)-1]
				continue
			}
			if !current.expanded {
				current.expanded = true
			}
			if _, done := memo[node.low]; !done {
				stack = append(stack, frame{ref: node.low})
				continue
			}
			if _, done := memo[node.high]; !done {
				stack = append(stack, frame{ref: node.high})
				continue
			}
			target, mapped := b.rename.target(node.variable)
			if !mapped && b.domain.contains(node.variable) {
				kernel.rollback(mark)
				return nil, fmt.Errorf("transformer: formal guard vector substitution omitted callee rank %d", node.variable)
			}
			if !mapped {
				if node.variable >= b.owner.size {
					kernel.rollback(mark)
					return nil, fmt.Errorf("transformer: formal guard vector substitution encountered foreign rank")
				}
				target = node.variable
			}
			if target >= b.owner.size {
				kernel.rollback(mark)
				return nil, fmt.Errorf("transformer: formal guard vector substitution encountered foreign rank")
			}
			condition := kernel.branch(target, decisionFalse, decisionTrue)
			mappedRoot, err := kernel.condition(ctx, condition, memo[node.high], memo[node.low])
			if err != nil {
				kernel.rollback(mark)
				return nil, err
			}
			memo[current.ref] = mappedRoot
			stack = stack[:len(stack)-1]
		}
	}
	out := make([]decisionRef, len(roots))
	for index, root := range roots {
		mapped, ok := memo[root]
		if !ok {
			kernel.rollback(mark)
			return nil, errDecisionMalformed
		}
		out[index] = mapped
	}
	return out, nil
}

// validateDecisionRootVector performs the rank/ownership/no-leak proof once
// over the union of all tuple roots. Shared sub-DAGs are visited once.
func (v *formalGuardVocabulary) validateDecisionRootVector(kernel *decisionKernel, roots []decisionRef, forbidden formalGuardRankSet) error {
	if !v.valid() || kernel == nil || len(roots) == 0 || forbidden.owner != nil && forbidden.owner != v {
		return fmt.Errorf("transformer: formal guard vector validation is unowned")
	}
	seen := make(map[decisionRef]struct{})
	stack := append([]decisionRef(nil), roots...)
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		node, ok := kernel.node(ref)
		if !ok {
			return fmt.Errorf("transformer: formal guard vector has foreign ROBDD node")
		}
		if node.terminal {
			continue
		}
		if node.variable >= v.size {
			return fmt.Errorf("transformer: formal guard vector has unranked variable %d", node.variable)
		}
		if forbidden.owner != nil && forbidden.contains(node.variable) {
			return fmt.Errorf("transformer: formal guard vector leaked closed rank %d", node.variable)
		}
		stack = append(stack, node.low, node.high)
	}
	return nil
}

// closeGuardedDecisionVector existentially closes one complete semantic
// component under its reachability certificate. The vector is either one
// independent descriptor or one whole registered product group. A synchronized
// traversal is required: closing members independently would invent product
// combinations that the registered group algebra never admitted.
func (a *formalTupleAlgebra) closeGuardedDecisionVector(
	care decisionRef,
	roots []decisionRef,
	closed formalGuardRankSet,
	join func(decisionRef, []decisionRef, decisionRef, []decisionRef) ([]decisionRef, error),
) (decisionRef, []decisionRef, error) {
	if a == nil || a.ctx == nil || len(roots) == 0 || closed.owner == nil || join == nil {
		return 0, nil, errDecisionMalformed
	}
	type result struct {
		care  decisionRef
		roots []decisionRef
	}
	type frame struct {
		key      formalDecisionVectorID
		expanded bool
		variable uint32
		lowKey   formalDecisionVectorID
		highKey  formalDecisionVectorID
	}
	initial := make([]decisionRef, 1+len(roots))
	initial[0] = care
	copy(initial[1:], roots)
	interner := newFormalDecisionVectorInterner()
	rootKey, err := interner.intern(initial)
	if err != nil {
		return 0, nil, err
	}
	memo := make(map[formalDecisionVectorID]result)
	stack := []frame{{key: rootKey}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := memo[current.key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if len(memo)&255 == 0 {
			if err := a.ctx.Err(); err != nil {
				return 0, nil, err
			}
		}
		values, valuesOK := interner.vector(current.key)
		if !valuesOK {
			return 0, nil, errDecisionMalformed
		}
		if !current.expanded {
			if len(values) != len(initial) {
				return 0, nil, errDecisionMalformed
			}
			careNode, ok := a.decisions.node(values[0])
			if !ok || careNode.terminal && careNode.leaf > 1 {
				return 0, nil, errDecisionMalformed
			}
			if careNode.terminal && careNode.leaf == 0 {
				memo[current.key] = result{care: decisionFalse, roots: make([]decisionRef, len(roots))}
				continue
			}
			variable := ^uint32(0)
			allTerminal := true
			for _, ref := range values {
				node, nodeOK := a.decisions.node(ref)
				if !nodeOK {
					return 0, nil, errDecisionMalformed
				}
				if !node.terminal {
					allTerminal = false
					if node.variable < variable {
						variable = node.variable
					}
				}
			}
			if allTerminal {
				terminalRoots := append([]decisionRef(nil), values[1:]...)
				memo[current.key] = result{care: decisionTrue, roots: terminalRoots}
				continue
			}
			low := make([]decisionRef, len(values))
			high := make([]decisionRef, len(values))
			for index, ref := range values {
				node, _ := a.decisions.node(ref)
				low[index], high[index] = ref, ref
				if !node.terminal && node.variable == variable {
					low[index], high[index] = node.low, node.high
				}
			}
			current.expanded = true
			current.variable = variable
			current.lowKey, err = interner.intern(low)
			if err != nil {
				return 0, nil, err
			}
			current.highKey, err = interner.intern(high)
			if err != nil {
				return 0, nil, err
			}
			if _, done := memo[current.lowKey]; !done {
				stack = append(stack, frame{key: current.lowKey})
				continue
			}
			if _, done := memo[current.highKey]; !done {
				stack = append(stack, frame{key: current.highKey})
				continue
			}
		}
		low, lowOK := memo[current.lowKey]
		high, highOK := memo[current.highKey]
		if !lowOK {
			return 0, nil, errDecisionMalformed
		}
		if !highOK {
			// The low child may have completed after the parent was expanded.
			// Schedule the high child without rebuilding either cofactor.
			stack = append(stack, frame{key: current.highKey})
			continue
		}
		var out result
		if closed.contains(current.variable) {
			var err error
			out.care, err = a.decisions.apply(a.ctx, uint8(decisionOr), true, low.care, high.care, decisionLeafOr)
			if err != nil {
				return 0, nil, err
			}
			switch {
			case low.care == decisionFalse:
				out.roots = append([]decisionRef(nil), high.roots...)
			case high.care == decisionFalse:
				out.roots = append([]decisionRef(nil), low.roots...)
			default:
				var err error
				out.roots, err = join(low.care, low.roots, high.care, high.roots)
				if err != nil {
					return 0, nil, err
				}
			}
		} else {
			out.care = a.decisions.branch(current.variable, low.care, high.care)
			out.roots = make([]decisionRef, len(roots))
			for index := range out.roots {
				out.roots[index] = a.decisions.branch(current.variable, low.roots[index], high.roots[index])
			}
		}
		if len(out.roots) != len(roots) {
			return 0, nil, errDecisionMalformed
		}
		memo[current.key] = out
		if a.evalTrace != nil && a.evalTrace.active != nil {
			a.evalTrace.active.guardComposeCloseStates++
		}
	}
	out, ok := memo[rootKey]
	if !ok {
		return 0, nil, errDecisionMalformed
	}
	return out.care, out.roots, nil
}

func (a *formalTupleAlgebra) joinGuardedGroupRoots(
	span formalFiberDescriptorSpan,
	authority *formalComponentTerminalAuthority,
	group formalFiberGroupDescriptor,
	leftCare decisionRef,
	left []decisionRef,
	rightCare decisionRef,
	right []decisionRef,
) ([]decisionRef, error) {
	if len(left) != len(group.members) || len(right) != len(group.members) {
		return nil, errFormalComponentMalformed
	}
	physicalIdentity := true
	for index := range left {
		if left[index] != right[index] {
			physicalIdentity = false
			break
		}
	}
	if physicalIdentity {
		return append([]decisionRef(nil), left...), nil
	}
	resultCare, err := a.decisions.apply(a.ctx, uint8(decisionOr), true, leftCare, rightCare, decisionLeafOr)
	if err != nil || resultCare == decisionFalse {
		if err == nil {
			err = errDecisionMalformed
		}
		return nil, err
	}
	if group.kind == formalFiberGroupValues {
		return a.combineValuesGroupRoots(formalComponentJoin, authority, group, left, right, leftCare, rightCare, resultCare)
	}
	if group.kind == formalFiberGroupCoordinateLane {
		return a.combineCoordinateGroupRoots(formalComponentJoin, authority, span, group, left, right, leftCare, rightCare, resultCare)
	}
	demands := make([]decisionRef, 0, 2+len(left)+len(right))
	demands = append(demands, leftCare, rightCare)
	demands = append(demands, left...)
	demands = append(demands, right...)
	var partitionMark formalRelationEvalTracePhaseMark
	if a.evalTrace != nil && a.evalTrace.active != nil {
		partitionMark = beginFormalRelationEvalTracePhase(a)
	}
	regions, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, resultCare, demands)
	if a.evalTrace != nil && a.evalTrace.active != nil {
		finishFormalRelationEvalTracePhase(a, &a.evalTrace.active.guardComposeGroupPartition, partitionMark)
		a.evalTrace.active.guardComposeGroupRegions += len(regions)
	}
	if err != nil {
		return nil, err
	}
	var leavesMark formalRelationEvalTracePhaseMark
	if a.evalTrace != nil && a.evalTrace.active != nil {
		leavesMark = beginFormalRelationEvalTracePhase(a)
	}
	result := make([]decisionRef, len(group.members))
	for _, region := range regions {
		if len(region.leaves) != len(demands) || region.leaves[0] > 1 || region.leaves[1] > 1 {
			return nil, errDecisionMalformed
		}
		leftLive, rightLive := region.leaves[0] == 1, region.leaves[1] == 1
		leftLeaves := region.leaves[2 : 2+len(left)]
		rightLeaves := region.leaves[2+len(left):]
		var leaves []decisionLeaf
		switch {
		case leftLive && rightLive:
			leaves, err = a.combineGroupLeaves(formalComponentJoin, authority, span, group, leftLeaves, rightLeaves)
		case leftLive:
			leaves, err = a.canonicalGroupLeaves(authority, span, group, leftLeaves)
		case rightLive:
			leaves, err = a.canonicalGroupLeaves(authority, span, group, rightLeaves)
		default:
			return nil, errDecisionMalformed
		}
		if err != nil || len(leaves) != len(result) {
			if err == nil {
				err = errFormalComponentMalformed
			}
			return nil, err
		}
		for index, leaf := range leaves {
			result[index], err = a.decisions.condition(a.ctx, region.care, a.decisions.terminal(leaf), result[index])
			if err != nil {
				return nil, err
			}
		}
	}
	if a.evalTrace != nil && a.evalTrace.active != nil {
		finishFormalRelationEvalTracePhase(a, &a.evalTrace.active.guardComposeGroupLeaves, leavesMark)
	}
	return result, nil
}
