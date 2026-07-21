package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// formalRelationTuple is one guarded, persistent complete-product payload.
// A zero variable is global Bottom. Every non-Bottom root belongs to exactly
// one lexical body's frozen descriptor span; caller identity is absent.
type formalRelationTuple struct {
	variable relationVar
	root     formalFiberDirectoryRoot
}

func (t formalRelationTuple) bottom() bool {
	return t.variable == 0 && t.root.owner == nil && t.root.ref == 0
}

// formalTupleAlgebra owns one solve transaction over the forest-global DD and
// per-body persistent directories. Semantic errors are scratch-owned and are
// reported before any result is published; lattice callbacks cannot return an
// error directly.
type formalTupleAlgebra struct {
	ctx         context.Context
	program     *RelationProgram
	rootEntry   *formalRootEntrySeed
	components  *formalComponentTerminalArena
	decisions   decisionKernel
	directories []*formalFiberDirectoryArena
	constants   map[formalRelationTupleConstantRef]formalRelationTuple
	guards      map[formalScopedGuardKey]decisionRef
	// factorReachability is populated whenever a complete factor is factored
	// into terminal leaves. Apply retrieves the immutable program by that exact
	// leaf vector; it never scans a factor or seals a program in leaf execution.
	factorReachability map[formalFactorReachabilityKey][]formalFactorReachabilityEntry
	// factorExecutionCapabilities intern all immutable execution authority for
	// one exact complete product spelling. Apply performs one lookup and neither
	// scans factors nor seals a derived capability in leaf execution.
	factorExecutionCapabilities map[formalFactorExecutionCapabilityKey][]formalFactorExecutionCapabilityEntry
	// applyObservations retains the latest exact observation witness produced by
	// each Apply equation. Witnesses are outside the semantic tuple lattice: they
	// are accepted only after WTO completion proves that every input tuple used
	// by the witness is the stabilized solution tuple for that input cell.
	applyObservations map[formalRelationCell]formalApplyObservationWitness
	// externalCallLeafOutcomes is evaluation-local dirty scheduling for sealed
	// ExternalCall leaves.  A leaf is keyed solely by its own frozen sparse
	// input certificate, so a source outside that certificate cannot cause its
	// provider evaluator to run again.  Parent composition remains outside this
	// cache and therefore still observes an owning leaf's changed outcome.
	externalCallLeafOutcomes map[uint64][]formalExternalCallLeafOutcomeCacheEntry
	// evalTrace is nil outside explicit diagnostic runs. It carries no semantic
	// state and is never consulted by an evaluator or lattice law.
	evalTrace  *formalRelationEvalTrace
	firstError error
}

func newFormalTupleAlgebra(ctx context.Context, program *RelationProgram) (*formalTupleAlgebra, error) {
	if ctx == nil || program == nil || program.formalFibers == nil || program.formalComponents == nil {
		return nil, fmt.Errorf("transformer: formal tuple algebra is unowned")
	}
	components, err := newFormalComponentTerminalArena(program.formalComponents)
	if err != nil {
		return nil, err
	}
	decisions := newDecisionKernel()
	decisions.resetBoolean()
	directories := make([]*formalFiberDirectoryArena, len(program.formalFibers.spans))
	for index, span := range program.formalFibers.spans {
		if span.variable != relationVar(index+1) || span.count < 0 {
			return nil, fmt.Errorf("transformer: formal tuple directory schema is malformed")
		}
		directory, directoryErr := newFormalFiberDirectoryArena(span.count)
		if directoryErr != nil {
			return nil, directoryErr
		}
		directories[index] = directory
	}
	algebra := &formalTupleAlgebra{
		ctx: ctx, program: program, components: components, decisions: decisions, directories: directories,
		guards:                      make(map[formalScopedGuardKey]decisionRef),
		factorReachability:          make(map[formalFactorReachabilityKey][]formalFactorReachabilityEntry),
		factorExecutionCapabilities: make(map[formalFactorExecutionCapabilityKey][]formalFactorExecutionCapabilityEntry),
		applyObservations:           make(map[formalRelationCell]formalApplyObservationWitness),
		externalCallLeafOutcomes:    make(map[uint64][]formalExternalCallLeafOutcomeCacheEntry),
	}
	if err := algebra.prefreezeFormalBottomReachability(); err != nil {
		return nil, err
	}
	return algebra, nil
}

func (a *formalTupleAlgebra) fail(err error) {
	if err != nil && a.firstError == nil {
		a.firstError = err
	}
}

func (a *formalTupleAlgebra) err() error {
	if a == nil {
		return fmt.Errorf("transformer: formal tuple algebra is unowned")
	}
	return a.firstError
}

func (a *formalTupleAlgebra) lattice() lattice.Lattice[formalRelationTuple] {
	return lattice.Lattice[formalRelationTuple]{
		Bottom:   func() formalRelationTuple { return formalRelationTuple{} },
		Equal:    a.equal,
		Same:     a.same,
		LessOrEq: a.lessOrEq,
		Join: func(left, right formalRelationTuple) formalRelationTuple {
			return a.combine(formalComponentJoin, left, right)
		},
		Widen: func(left, right formalRelationTuple) formalRelationTuple {
			return a.combine(formalComponentWiden, left, right)
		},
	}
}

func (a *formalTupleAlgebra) equal(left, right formalRelationTuple) bool {
	return a.compare(left, right, false)
}

func (a *formalTupleAlgebra) lessOrEq(left, right formalRelationTuple) bool {
	return a.compare(left, right, true)
}

func (a *formalTupleAlgebra) same(left, right formalRelationTuple) bool {
	if err := a.validateTuple(left); err != nil {
		a.fail(err)
		return false
	}
	if err := a.validateTuple(right); err != nil {
		a.fail(err)
		return false
	}
	if left.bottom() || right.bottom() {
		return left.bottom() && right.bottom()
	}
	return left.variable == right.variable && left.root.owner == right.root.owner && left.root.ref == right.root.ref
}

func (a *formalTupleAlgebra) validateTuple(tuple formalRelationTuple) error {
	if a == nil {
		return fmt.Errorf("transformer: formal tuple algebra is unowned")
	}
	if tuple.variable == 0 {
		if tuple.bottom() {
			return nil
		}
		return fmt.Errorf("transformer: formal tuple has a non-canonical Bottom")
	}
	_, directory, _, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory {
		return fmt.Errorf("transformer: formal tuple has foreign directory ownership")
	}
	return directory.validateRoot(tuple.root)
}

func (a *formalTupleAlgebra) span(variable relationVar) (formalFiberDescriptorSpan, *formalFiberDirectoryArena, *formalComponentTerminalAuthority, bool) {
	if a == nil || a.program == nil {
		return formalFiberDescriptorSpan{}, nil, nil, false
	}
	span, ok := a.program.formalFibers.span(variable)
	if !ok {
		return formalFiberDescriptorSpan{}, nil, nil, false
	}
	if int(variable) > len(a.directories) || a.directories[variable-1] == nil {
		return formalFiberDescriptorSpan{}, nil, nil, false
	}
	authority, ok := a.components.authority(variable)
	return span, a.directories[variable-1], authority, ok
}

func (a *formalTupleAlgebra) care(tuple formalRelationTuple) (decisionRef, error) {
	if tuple.bottom() {
		return decisionFalse, nil
	}
	_, directory, _, ok := a.span(tuple.variable)
	if !ok {
		return 0, fmt.Errorf("transformer: formal tuple has no descriptor span")
	}
	value, err := directory.valueAt(tuple.root, 0)
	if err != nil {
		return 0, err
	}
	care := decisionRef(value)
	if int(care) >= len(a.decisions.nodes) {
		return 0, errDecisionMalformed
	}
	return care, nil
}

func (a *formalTupleAlgebra) componentRoot(authority *formalComponentTerminalAuthority, descriptor formalFiberDescriptor, root decisionRef) (decisionRef, error) {
	if root != decisionFalse {
		return root, nil
	}
	value, err := authority.defaultFor(a.ctx, descriptor)
	if err != nil {
		return 0, err
	}
	switch value.kind {
	case formalComponentDefaultAbsent, formalComponentDefaultBooleanFalse:
		return decisionFalse, nil
	case formalComponentDefaultTerminal:
		return a.decisions.terminal(value.leaf), nil
	case formalComponentDefaultCoordinateGroup:
		return 0, errFormalCoordinateGroupRequired
	default:
		return 0, errFormalComponentMalformed
	}
}

func (a *formalTupleAlgebra) combineAbsent(op formalComponentBinaryOp, descriptor formalFiberDescriptor, left, right decisionLeaf) (decisionLeaf, error) {
	if left == 1 || right == 1 {
		return 0, errFormalComponentMalformed
	}
	switch op {
	case formalComponentJoin, formalComponentWiden:
		if left == 0 {
			return right, nil
		}
		return left, nil
	case formalComponentMeet:
		return 0, nil
	case formalComponentNarrow:
		if descriptor.role == formalFiberMiddleValue || descriptor.role == formalFiberMiddlePath {
			// Empty is the one exact syntactic intersection: it denotes no
			// symbolic function. Distinct nonempty sets are rejected by the
			// component authority because their denotations may overlap.
			return 0, nil
		}
		return left, nil
	default:
		return 0, errFormalComponentMalformed
	}
}

// componentLeaf interprets structural leaf zero in the descriptor's own
// algebra. Symbolic occurrence fibers use zero as absence; registered product
// fibers use their registered Bottom. This conversion is required at every DD
// leaf, not merely when the entire component root is decisionFalse.
func (a *formalTupleAlgebra) componentLeaf(authority *formalComponentTerminalAuthority, descriptor formalFiberDescriptor, leaf decisionLeaf) (decisionLeaf, error) {
	if leaf != 0 {
		return leaf, nil
	}
	value, err := authority.defaultFor(a.ctx, descriptor)
	if err != nil {
		return 0, err
	}
	switch value.kind {
	case formalComponentDefaultAbsent, formalComponentDefaultBooleanFalse:
		return 0, nil
	case formalComponentDefaultTerminal:
		return value.leaf, nil
	case formalComponentDefaultCoordinateGroup:
		return 0, errFormalCoordinateGroupRequired
	default:
		return 0, errFormalComponentMalformed
	}
}

func (a *formalTupleAlgebra) combineBoolean(op formalComponentBinaryOp, left, right decisionRef) (decisionRef, error) {
	switch op {
	case formalComponentJoin, formalComponentWiden:
		return a.decisions.apply(a.ctx, uint8(decisionOr), true, left, right, decisionLeafOr)
	case formalComponentMeet:
		return a.decisions.apply(a.ctx, uint8(decisionAnd), true, left, right, decisionLeafAnd)
	case formalComponentNarrow:
		return left, nil
	default:
		return 0, errFormalComponentMalformed
	}
}

func (a *formalTupleAlgebra) normalize(tuple formalRelationTuple) formalRelationTuple {
	if tuple.bottom() {
		return tuple
	}
	care, err := a.care(tuple)
	if err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	if care == decisionFalse {
		return formalRelationTuple{}
	}
	return tuple
}

func (a *formalTupleAlgebra) compare(left, right formalRelationTuple, order bool) bool {
	if a == nil {
		return false
	}
	if err := a.validateTuple(left); err != nil {
		a.fail(err)
		return false
	}
	if err := a.validateTuple(right); err != nil {
		a.fail(err)
		return false
	}
	// A failed formal transaction is never published, but the enclosing generic
	// solver must still be able to observe that its scratch values stopped
	// changing and return the recorded error.  Keep physical equality reflexive
	// after failure; reporting every value unequal turns one semantic error into
	// an infinite WTO ascent over an otherwise unchanged tuple.
	if a.firstError != nil {
		if left.bottom() || right.bottom() {
			return left.bottom() && right.bottom()
		}
		return left.variable == right.variable && left.root.owner == right.root.owner && left.root.ref == right.root.ref
	}
	if left.bottom() || right.bottom() {
		if order {
			return left.bottom()
		}
		return left.bottom() && right.bottom()
	}
	if left.variable != right.variable || left.root.owner != right.root.owner {
		return false
	}
	if left.root.ref == right.root.ref {
		return true
	}
	span, directory, authority, ok := a.span(left.variable)
	if !ok || left.root.owner != directory {
		a.fail(fmt.Errorf("transformer: formal tuple comparison has foreign ownership"))
		return false
	}
	leftCare, err := a.care(left)
	if err != nil {
		a.fail(err)
		return false
	}
	rightCare, err := a.care(right)
	if err != nil {
		a.fail(err)
		return false
	}
	careOK, err := a.decisionRelation(leftCare, rightCare, func(l, r decisionLeaf) (bool, error) {
		if l > 1 || r > 1 {
			return false, errDecisionMalformed
		}
		if order {
			return l == 0 || r == 1, nil
		}
		return l == r, nil
	})
	if err != nil || !careOK {
		a.fail(err)
		return false
	}
	descriptors := span.forest.descriptors[span.first : span.first+span.count]
	groups := span.groupDescriptors()
	grouped := make([]bool, span.count)
	for _, group := range groups {
		for _, ordinal := range group.members {
			if int(ordinal) < 0 || int(ordinal) >= len(grouped) || grouped[ordinal] {
				a.fail(errFormalComponentMalformed)
				return false
			}
			grouped[ordinal] = true
		}
	}
	err = directory.visitDifferences(left.root, right.root, func(ordinal formalFiberOrdinal, leftValue, rightValue formalFiberValue) error {
		if ordinal == 0 || int(ordinal) >= len(descriptors) {
			return nil
		}
		if grouped[ordinal] {
			return nil
		}
		descriptor := descriptors[ordinal]
		leftRoot, rootErr := a.componentRoot(authority, descriptor, decisionRef(leftValue))
		if rootErr != nil {
			return rootErr
		}
		rightRoot, rootErr := a.componentRoot(authority, descriptor, decisionRef(rightValue))
		if rootErr != nil {
			return rootErr
		}
		care := leftCare
		if !order {
			care = rightCare
		}
		leftRoot, rootErr = a.decisions.restrict(a.ctx, care, leftRoot)
		if rootErr != nil {
			return rootErr
		}
		rightRoot, rootErr = a.decisions.restrict(a.ctx, care, rightRoot)
		if rootErr != nil {
			return rootErr
		}
		equal, relationErr := a.decisionRelation(leftRoot, rightRoot, func(l, r decisionLeaf) (bool, error) {
			if descriptor.role == formalFiberGroundValueTop {
				if l > 1 || r > 1 {
					return false, errDecisionMalformed
				}
				if order {
					return l == 0 || r == 1, nil
				}
				return l == r, nil
			}
			l, rootErr = a.componentLeaf(authority, descriptor, l)
			if rootErr != nil {
				return false, rootErr
			}
			r, rootErr = a.componentLeaf(authority, descriptor, r)
			if rootErr != nil {
				return false, rootErr
			}
			if l == 0 || r == 0 {
				if order {
					return l == 0, nil
				}
				return l == r, nil
			}
			if order {
				return authority.lessOrEq(l, r)
			}
			return authority.equal(l, r)
		})
		if relationErr != nil {
			return relationErr
		}
		if !equal {
			return errFormalTupleRelationFalse
		}
		return nil
	})
	if err == errFormalTupleRelationFalse {
		return false
	}
	if err != nil {
		a.fail(err)
		return false
	}
	groupCare := rightCare
	if order {
		groupCare = leftCare
	}
	for _, group := range groups {
		equal, groupErr := a.compareGroupRoots(span, authority, group, left, right, groupCare, order)
		if groupErr != nil {
			a.fail(groupErr)
			return false
		}
		if !equal {
			return false
		}
	}
	return true
}

var errFormalTupleRelationFalse = fmt.Errorf("transformer: formal tuple relation is false")

func (a *formalTupleAlgebra) decisionRelation(left, right decisionRef, leaves func(decisionLeaf, decisionLeaf) (bool, error)) (bool, error) {
	if int(left) >= len(a.decisions.nodes) || int(right) >= len(a.decisions.nodes) || leaves == nil {
		return false, errDecisionMalformed
	}
	type pair struct{ left, right decisionRef }
	memo := make(map[pair]bool)
	active := make(map[pair]bool)
	var visit func(decisionRef, decisionRef) (bool, error)
	visit = func(lref, rref decisionRef) (bool, error) {
		key := pair{lref, rref}
		if value, ok := memo[key]; ok {
			return value, nil
		}
		if active[key] {
			return false, errDecisionMalformed
		}
		active[key] = true
		defer delete(active, key)
		leftNode, rightNode := a.decisions.nodes[lref], a.decisions.nodes[rref]
		if leftNode.terminal && rightNode.terminal {
			value, err := leaves(leftNode.leaf, rightNode.leaf)
			if err == nil {
				memo[key] = value
			}
			return value, err
		}
		variable := ^uint32(0)
		if !leftNode.terminal {
			variable = leftNode.variable
		}
		if !rightNode.terminal && rightNode.variable < variable {
			variable = rightNode.variable
		}
		ll, lh, rl, rh := lref, lref, rref, rref
		if !leftNode.terminal && leftNode.variable == variable {
			ll, lh = leftNode.low, leftNode.high
		}
		if !rightNode.terminal && rightNode.variable == variable {
			rl, rh = rightNode.low, rightNode.high
		}
		low, err := visit(ll, rl)
		if err != nil || !low {
			return low, err
		}
		high, err := visit(lh, rh)
		if err == nil {
			memo[key] = high
		}
		return high, err
	}
	return visit(left, right)
}

func (a *formalFiberDirectoryArena) visitDifferences(left, right formalFiberDirectoryRoot, visit func(formalFiberOrdinal, formalFiberValue, formalFiberValue) error) error {
	if err := a.validateRoot(left); err != nil {
		return err
	}
	if err := a.validateRoot(right); err != nil {
		return err
	}
	if visit == nil {
		return fmt.Errorf("transformer: formal directory comparison has no leaf visitor")
	}
	var walk func(formalFiberNodeRef, formalFiberNodeRef, uint, int, int) error
	walk = func(lref, rref formalFiberNodeRef, height uint, start, span int) error {
		if lref == rref {
			return nil
		}
		if height == 0 {
			leftValue, err := a.leafValue(lref)
			if err != nil {
				return err
			}
			rightValue, err := a.leafValue(rref)
			if err != nil {
				return err
			}
			return visit(formalFiberOrdinal(start), leftValue, rightValue)
		}
		ll, lh, err := a.children(lref, height)
		if err != nil {
			return err
		}
		rl, rh, err := a.children(rref, height)
		if err != nil {
			return err
		}
		half := span / 2
		if err := walk(ll, rl, height-1, start, half); err != nil {
			return err
		}
		return walk(lh, rh, height-1, start+half, half)
	}
	return walk(left.ref, right.ref, a.height, 0, a.leafBase)
}

func (a *formalTupleAlgebra) combine(op formalComponentBinaryOp, left, right formalRelationTuple) formalRelationTuple {
	if a == nil {
		return formalRelationTuple{}
	}
	if err := a.validateTuple(left); err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	if err := a.validateTuple(right); err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	// Once a transaction has failed, preserve the solver's current left
	// iterate.  The result is discarded when executeFormalRelation reports
	// firstError; retaining the iterate makes the poisoned lattice absorbing
	// instead of repeatedly replacing it with Bottom forever.
	if a.firstError != nil {
		return left
	}
	if left.bottom() || right.bottom() {
		switch op {
		case formalComponentJoin, formalComponentWiden:
			if left.bottom() {
				return right
			}
			return left
		case formalComponentMeet:
			return formalRelationTuple{}
		case formalComponentNarrow:
			return left
		}
	}
	if a.same(left, right) {
		return left
	}
	if left.variable != right.variable {
		a.fail(fmt.Errorf("transformer: formal tuple operation crossed lexical bodies"))
		return formalRelationTuple{}
	}
	span, directory, authority, ok := a.span(left.variable)
	if !ok || left.root.owner != directory || right.root.owner != directory {
		a.fail(fmt.Errorf("transformer: formal tuple operation has foreign directory ownership"))
		return formalRelationTuple{}
	}
	leftCare, err := a.care(left)
	if err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	rightCare, err := a.care(right)
	if err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	careOp, careLeaves := decisionOr, decisionLeafOr
	if op == formalComponentMeet {
		careOp, careLeaves = decisionAnd, decisionLeafAnd
	}
	combinedCare := leftCare
	if op != formalComponentNarrow {
		combinedCare, err = a.decisions.apply(a.ctx, uint8(careOp), true, leftCare, rightCare, careLeaves)
	}
	if err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	if combinedCare == decisionFalse {
		return formalRelationTuple{}
	}
	descriptors := span.forest.descriptors[span.first : span.first+span.count]
	groups := span.groupDescriptors()
	grouped := make([]int, span.count)
	for index := range grouped {
		grouped[index] = -1
	}
	for groupIndex, group := range groups {
		for _, ordinal := range group.members {
			if int(ordinal) < 0 || int(ordinal) >= len(grouped) || grouped[ordinal] != -1 {
				a.fail(errFormalComponentMalformed)
				return formalRelationTuple{}
			}
			grouped[ordinal] = groupIndex
		}
	}
	dirtyGroups := make([]bool, len(groups))
	writes := make([]formalFiberWrite, 0, span.count)
	err = directory.visitDifferences(left.root, right.root, func(ordinal formalFiberOrdinal, leftValue, rightValue formalFiberValue) error {
		if int(ordinal) >= len(descriptors) {
			return fmt.Errorf("transformer: formal tuple ordinal escaped descriptor span")
		}
		descriptor := descriptors[ordinal]
		if descriptor.role == formalFiberCare {
			if formalFiberValue(combinedCare) != leftValue {
				writes = append(writes, formalFiberWrite{ordinal: ordinal, value: formalFiberValue(combinedCare)})
			}
			return nil
		}
		if groupIndex := grouped[ordinal]; groupIndex >= 0 {
			dirtyGroups[groupIndex] = true
			return nil
		}
		leftRoot, rightRoot := decisionRef(leftValue), decisionRef(rightValue)
		if int(leftRoot) >= len(a.decisions.nodes) || int(rightRoot) >= len(a.decisions.nodes) {
			return errDecisionMalformed
		}
		leftRoot, err = a.componentRoot(authority, descriptor, leftRoot)
		if err != nil {
			return err
		}
		rightRoot, err = a.componentRoot(authority, descriptor, rightRoot)
		if err != nil {
			return err
		}
		var joined decisionRef
		switch descriptor.role {
		case formalFiberGroundValueTop:
			joined, err = a.combineBoolean(op, leftRoot, rightRoot)
		default:
			joined, err = a.decisions.applyUnderCare(
				a.ctx, uint8(op), op == formalComponentJoin || op == formalComponentMeet,
				leftCare, leftRoot, rightCare, rightRoot,
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
						return a.combineAbsent(op, descriptor, leftLeaf, rightLeaf)
					}
					return authority.combine(a.ctx, op, leftLeaf, rightLeaf)
				},
			)
		}
		if err != nil {
			return err
		}
		if formalFiberValue(joined) != leftValue {
			writes = append(writes, formalFiberWrite{ordinal: ordinal, value: formalFiberValue(joined)})
		}
		return nil
	})
	if err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	for groupIndex, group := range groups {
		if !dirtyGroups[groupIndex] {
			continue
		}
		roots, groupErr := a.combineGroupRoots(op, span, authority, group, left, right, leftCare, rightCare, combinedCare)
		if groupErr != nil {
			a.fail(groupErr)
			return formalRelationTuple{}
		}
		if len(roots) != len(group.members) {
			a.fail(errFormalComponentMalformed)
			return formalRelationTuple{}
		}
		for index, ordinal := range group.members {
			member, ok := group.member(ordinal)
			address, addressOK := member.address(group)
			if !ok || !addressOK || address != ordinal || int(ordinal) < 0 || int(ordinal) >= span.count {
				a.fail(errFormalComponentForeignOwner)
				return formalRelationTuple{}
			}
			descriptor := descriptors[ordinal]
			if groupErr = a.validateDescriptorRoot(authority, descriptor, roots[index]); groupErr != nil {
				a.fail(groupErr)
				return formalRelationTuple{}
			}
			leftValue, readErr := directory.valueAt(left.root, ordinal)
			if readErr != nil {
				a.fail(readErr)
				return formalRelationTuple{}
			}
			if formalFiberValue(roots[index]) != leftValue {
				writes = append(writes, formalFiberWrite{ordinal: ordinal, value: formalFiberValue(roots[index])})
			}
		}
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	root, _, err := directory.applyDelta(left.root, delta)
	if err != nil {
		a.fail(err)
		return formalRelationTuple{}
	}
	return a.normalize(formalRelationTuple{variable: left.variable, root: root})
}

func (a *formalTupleAlgebra) validateDescriptorLeaf(authority *formalComponentTerminalAuthority, descriptor formalFiberDescriptor, leaf decisionLeaf) error {
	if descriptor.role == formalFiberCare || descriptor.role == formalFiberGroundValueTop {
		if leaf <= 1 {
			return nil
		}
		return errFormalComponentMalformed
	}
	if leaf == 0 {
		return nil
	}
	if leaf == 1 {
		return errFormalComponentMalformed
	}
	terminal, err := authority.terminal(leaf)
	if err != nil {
		return err
	}
	switch descriptor.role {
	case formalFiberMiddleValue:
		if terminal.kind != formalComponentBindings {
			return errFormalComponentMalformed
		}
	case formalFiberMiddlePath:
		if terminal.kind != formalComponentPathTerms {
			return errFormalComponentMalformed
		}
		// A symbol's ValueTerm and PathTerm are one aligned call-frame
		// binding. Independent powersets would invent value/path cross-pairs at
		// a join, so path publication remains fail-closed until the descriptor
		// is replaced by one atomic qualified-binding set.
		return errFormalSymbolicPathCorrelation
	case formalFiberOutcome:
		if terminal.kind != formalComponentOutcomeOccurrence || terminal.outcome.ref != descriptor.outcome {
			return errFormalComponentMalformed
		}
	case formalFiberDiagnostics:
		if terminal.kind != formalComponentDiagnostics || !terminal.diagnostics.Valid(authority.product.Registry()) ||
			diagnosticContainsAllocationTemplate(authority.product.Registry(), terminal.diagnostics) {
			return errFormalComponentMalformed
		}
	case formalFiberCallOutcome:
		if terminal.kind != formalComponentCallOutcomes {
			return errFormalComponentMalformed
		}
	case formalFiberOrdinaryLane:
		if terminal.kind != formalComponentOrdinaryLane || terminal.lane.Lane().Ordinal() != descriptor.lane.Ordinal() {
			return errFormalComponentMalformed
		}
	case formalFiberCoordinate:
		switch descriptor.coordinateKind {
		case formalFiberCoordinateFamilySkeleton:
			if terminal.kind != formalComponentCoordinateSkeleton || !coordinateFamilySame(terminal.skeleton.Family(), descriptor.family) {
				return errFormalComponentMalformed
			}
		case formalFiberCoordinateFamilyScalar:
			if terminal.kind != formalComponentCoordinateScalar {
				return errFormalComponentMalformed
			}
			equal, equalErr := authority.product.CoordinateSlotEqual(terminal.scalar.Slot(), descriptor.coordinate)
			if equalErr != nil || !equal {
				if equalErr != nil {
					return equalErr
				}
				return errFormalComponentMalformed
			}
		default:
			return errFormalComponentMalformed
		}
	case formalFiberGroundValue:
		if terminal.kind != formalComponentGroundValue {
			return errFormalComponentMalformed
		}
	default:
		return errFormalComponentMalformed
	}
	return nil
}

func (a *formalTupleAlgebra) validateDescriptorRoot(authority *formalComponentTerminalAuthority, descriptor formalFiberDescriptor, root decisionRef) error {
	if int(root) >= len(a.decisions.nodes) {
		return errDecisionMalformed
	}
	seen := make(map[decisionRef]bool)
	stack := []decisionRef{root}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		node, ok := a.decisions.node(ref)
		if !ok {
			return errDecisionMalformed
		}
		if node.terminal {
			if err := a.validateDescriptorLeaf(authority, descriptor, node.leaf); err != nil {
				return err
			}
			continue
		}
		stack = append(stack, node.low, node.high)
	}
	return nil
}

// writeScalar publishes exactly one independent scalar fiber. Dependent
// Values and coordinate carriers are deliberately rejected: they are only
// writable by their complete typed group operation.
func (a *formalTupleAlgebra) writeScalar(tuple formalRelationTuple, descriptor formalFiberDescriptor, root decisionRef) (formalRelationTuple, error) {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, fmt.Errorf("transformer: cannot write scalar fiber to Bottom")
	}
	span, directory, authority, ok := a.span(tuple.variable)
	ordinal, ordinalOK := span.ordinal(descriptor)
	if !ok || !ordinalOK || tuple.root.owner != directory {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal scalar write has foreign ownership")
	}
	if descriptor.role == formalFiberCoordinate || descriptor.role == formalFiberGroundValueTop || descriptor.role == formalFiberGroundValue {
		return formalRelationTuple{}, fmt.Errorf("transformer: dependent formal carrier requires atomic group write")
	}
	if err := a.validateDescriptorRoot(authority, descriptor, root); err != nil {
		return formalRelationTuple{}, err
	}
	next, _, err := directory.update(tuple.root, ordinal, formalFiberValue(root))
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: tuple.variable, root: next}), nil
}

func (a *formalTupleAlgebra) writeCare(tuple formalRelationTuple, root decisionRef) (formalRelationTuple, error) {
	if tuple.variable == 0 || tuple.root.owner == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal care write has no lexical tuple")
	}
	span, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory || span.count == 0 {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal care write has foreign ownership")
	}
	descriptor := span.forest.descriptors[span.first]
	if descriptor.role != formalFiberCare {
		return formalRelationTuple{}, errFormalComponentMalformed
	}
	if err := a.validateDescriptorRoot(authority, descriptor, root); err != nil {
		return formalRelationTuple{}, err
	}
	next, _, err := directory.update(tuple.root, 0, formalFiberValue(root))
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: tuple.variable, root: next}), nil
}
