package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

func (a *formalTupleAlgebra) groupRoots(tuple formalRelationTuple, group formalFiberGroupDescriptor) ([]decisionRef, error) {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() || !group.valid() || group.variable != tuple.variable {
		if err != nil {
			return nil, err
		}
		return nil, errFormalComponentForeignOwner
	}
	_, directory, _, ok := a.span(tuple.variable)
	if !ok {
		return nil, errFormalComponentForeignOwner
	}
	roots := make([]decisionRef, len(group.members))
	for index, ordinal := range group.members {
		value, err := directory.valueAt(tuple.root, ordinal)
		if err != nil {
			return nil, err
		}
		root := decisionRef(value)
		if int(root) >= len(a.decisions.nodes) {
			return nil, errDecisionMalformed
		}
		roots[index] = root
	}
	return roots, nil
}

func (a *formalTupleAlgebra) materializeValuesGroup(authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, leaves []decisionLeaf) (state.ValueFactor[FormalSlot], error) {
	if group.kind != formalFiberGroupValues || len(leaves) != len(group.members) {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
	}
	if group.valueTopPosition < 0 || group.valueTopPosition >= len(leaves) || leaves[group.valueTopPosition] > 1 {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
	}
	if leaves[group.valueTopPosition] == 1 {
		return state.ValueFactor[FormalSlot]{Top: true}, nil
	}
	var values map[FormalSlot]product.Value
	bottom := product.Bottom(authority.product.Registry())
	for _, slot := range group.valueSlots {
		if slot.position < 0 || slot.position >= len(leaves) {
			return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
		}
		leaf := leaves[slot.position]
		if leaf == 0 {
			continue
		}
		terminal, err := authority.terminal(leaf)
		if err != nil || terminal.kind != formalComponentGroundValue {
			if err != nil {
				return state.ValueFactor[FormalSlot]{}, err
			}
			return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
		}
		if !product.Equal(authority.product.Registry(), terminal.ground, bottom) {
			if values == nil {
				values = make(map[FormalSlot]product.Value)
			}
			values[slot.slot] = terminal.ground
		}
	}
	return state.ValueFactor[FormalSlot]{Values: values}, nil
}

func (a *formalTupleAlgebra) factorValuesGroup(authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, value state.ValueFactor[FormalSlot]) ([]decisionLeaf, error) {
	if group.kind != formalFiberGroupValues || value.Top && len(value.Values) != 0 {
		return nil, errFormalComponentMalformed
	}
	leaves := make([]decisionLeaf, len(group.members))
	if group.valueTopPosition < 0 || group.valueTopPosition >= len(leaves) {
		return nil, errFormalComponentMalformed
	}
	if value.Top {
		leaves[group.valueTopPosition] = 1
		return leaves, nil
	}
	bottom := product.Bottom(authority.product.Registry())
	consumed := 0
	for _, slot := range group.valueSlots {
		coordinate, exists := value.Values[slot.slot]
		if !exists {
			continue
		}
		consumed++
		if product.Equal(authority.product.Registry(), coordinate, bottom) {
			continue
		}
		if slot.position < 0 || slot.position >= len(leaves) {
			return nil, errFormalComponentMalformed
		}
		leaf, err := authority.internGroundValue(coordinate)
		if err != nil {
			return nil, err
		}
		leaves[slot.position] = leaf
	}
	if consumed != len(value.Values) {
		return nil, errFormalComponentForeignOwner
	}
	return leaves, nil
}

func (a *formalTupleAlgebra) materializeOrdinaryGroup(authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, leaves []decisionLeaf) (state.LaneFactor, error) {
	if authority == nil || group.kind != formalFiberGroupOrdinaryLane || len(group.members) != 1 || len(leaves) != 1 {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	if leaves[0] == 0 {
		return authority.product.LaneBottom(group.lane)
	}
	terminal, err := authority.terminal(leaves[0])
	if err != nil {
		return state.LaneFactor{}, err
	}
	if terminal.kind != formalComponentOrdinaryLane || terminal.lane.Lane() != group.lane {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	return terminal.lane, nil
}

func (a *formalTupleAlgebra) factorOrdinaryGroup(authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, value state.LaneFactor) ([]decisionLeaf, error) {
	if authority == nil || group.kind != formalFiberGroupOrdinaryLane || len(group.members) != 1 || value.Lane() != group.lane {
		return nil, errFormalComponentMalformed
	}
	bottom, err := authority.product.LaneBottom(group.lane)
	if err != nil {
		return nil, err
	}
	// Physical zero is the canonical encoding of semantic lane Bottom. Unlike
	// operand reuse, factoring is a quotient operation, so lattice equality is
	// the exact criterion for erasing an alternative Bottom spelling.
	sameBottom, err := authority.product.LaneEqual(value, bottom)
	if err != nil {
		return nil, err
	}
	if sameBottom {
		leaves := []decisionLeaf{0}
		if err := a.cacheFormalFactorReachability(authority, group, leaves, value); err != nil {
			return nil, err
		}
		return leaves, nil
	}
	leaf, err := authority.internLane(a.ctx, value)
	if err != nil {
		return nil, err
	}
	leaves := []decisionLeaf{leaf}
	if err := a.cacheFormalFactorReachability(authority, group, leaves, value); err != nil {
		return nil, err
	}
	return leaves, nil
}

func (a *formalTupleAlgebra) materializeCoordinateGroup(authority *formalComponentTerminalAuthority, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, leaves []decisionLeaf) (state.LaneFactor, error) {
	if group.kind != formalFiberGroupCoordinateLane || len(leaves) != len(group.members) {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	skeletons := make([]state.CoordinateFamilySkeleton, len(group.coordinateFamilies))
	scalars := make([][]state.CoordinateScalarFactor, len(group.coordinateFamilies))
	for familyIndex, family := range group.coordinateFamilies {
		if family.skeletonPosition < 0 || family.skeletonPosition >= len(leaves) {
			return state.LaneFactor{}, errFormalComponentMalformed
		}
		leaf := leaves[family.skeletonPosition]
		if leaf == 0 {
			skeleton, err := authority.product.CoordinateSkeletonBottom(family.family, span.keys)
			if err != nil {
				return state.LaneFactor{}, err
			}
			skeletons[familyIndex] = skeleton
		} else {
			terminal, err := authority.terminal(leaf)
			if err != nil || terminal.kind != formalComponentCoordinateSkeleton || !coordinateFamilySame(terminal.skeleton.Family(), family.family) {
				if err != nil {
					return state.LaneFactor{}, err
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			skeletons[familyIndex] = terminal.skeleton
		}
		for scalarIndex := range family.scalars {
			if scalarIndex >= len(family.scalarPositions) || family.scalarPositions[scalarIndex] < 0 || family.scalarPositions[scalarIndex] >= len(leaves) {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			leaf := leaves[family.scalarPositions[scalarIndex]]
			if leaf == 0 {
				continue
			}
			terminal, err := authority.terminal(leaf)
			if err != nil || terminal.kind != formalComponentCoordinateScalar {
				if err != nil {
					return state.LaneFactor{}, err
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			omitted, err := authority.product.CoordinateScalarIsOmitted(skeletons[familyIndex], terminal.scalar)
			if err != nil {
				return state.LaneFactor{}, err
			}
			if !omitted {
				scalars[familyIndex] = append(scalars[familyIndex], terminal.scalar)
			}
		}
	}
	return authority.product.ComposeCoordinateFamilies(group.lane, span.keys, skeletons, scalars)
}

func (a *formalTupleAlgebra) factorCoordinateGroup(authority *formalComponentTerminalAuthority, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, value state.LaneFactor) ([]decisionLeaf, error) {
	if group.kind != formalFiberGroupCoordinateLane || value.Lane().Ordinal() != group.lane.Ordinal() {
		return nil, errFormalComponentMalformed
	}
	leaves := make([]decisionLeaf, len(group.members))
	for _, family := range group.coordinateFamilies {
		skeleton, scalars, err := authority.product.DecomposeCoordinateFamily(value, family.family, span.keys)
		if err != nil {
			return nil, err
		}
		if family.skeletonPosition < 0 || family.skeletonPosition >= len(leaves) {
			return nil, errFormalComponentMalformed
		}
		bottom, err := authority.product.CoordinateSkeletonBottom(family.family, span.keys)
		if err != nil {
			return nil, err
		}
		sameBottom, err := authority.product.CoordinateSkeletonRepresentationEqual(skeleton, bottom)
		if err != nil {
			return nil, err
		}
		if !sameBottom {
			leaf, internErr := authority.internCoordinateSkeleton(skeleton)
			if internErr != nil {
				return nil, internErr
			}
			leaves[family.skeletonPosition] = leaf
		}
		scalarIndex := 0
		for ordinalIndex, ordinal := range family.scalars {
			descriptor := span.forest.descriptors[span.first+int(ordinal)]
			if scalarIndex >= len(scalars) {
				continue
			}
			scalar := scalars[scalarIndex]
			equal, equalErr := authority.product.CoordinateSlotEqual(descriptor.coordinate, scalar.Slot())
			if equalErr != nil {
				return nil, equalErr
			}
			if !equal {
				scalarLess, lessErr := authority.product.CoordinateSlotLess(scalar.Slot(), descriptor.coordinate)
				if lessErr != nil {
					return nil, lessErr
				}
				if scalarLess {
					hash, _ := authority.product.CoordinateSlotHash(scalar.Slot())
					paths, _ := authority.product.PathCoordinateSupportPaths([]state.CoordinateSlot{scalar.Slot()})
					return nil, fmt.Errorf("transformer: coordinate factor escaped frozen fiber inventory: lane=%s family=%s slot=%016x paths=%v", group.lane.ID(), family.family.ID(), hash, formatFormalCoordinatePaths(span.keys, paths))
				}
				continue
			}
			if ordinalIndex >= len(family.scalarPositions) || family.scalarPositions[ordinalIndex] < 0 || family.scalarPositions[ordinalIndex] >= len(leaves) {
				return nil, errFormalComponentMalformed
			}
			leaf, err := authority.internCoordinateScalar(scalar)
			if err != nil {
				return nil, err
			}
			leaves[family.scalarPositions[ordinalIndex]] = leaf
			scalarIndex++
		}
		if scalarIndex != len(scalars) {
			hash, _ := authority.product.CoordinateSlotHash(scalars[scalarIndex].Slot())
			paths, _ := authority.product.PathCoordinateSupportPaths([]state.CoordinateSlot{scalars[scalarIndex].Slot()})
			return nil, fmt.Errorf("transformer: coordinate factor escaped frozen fiber inventory: lane=%s family=%s slot=%016x paths=%v", group.lane.ID(), family.family.ID(), hash, formatFormalCoordinatePaths(span.keys, paths))
		}
	}
	if err := a.cacheFormalFactorReachability(authority, group, leaves, value); err != nil {
		return nil, err
	}
	return leaves, nil
}

func formatFormalCoordinatePaths(keys *keyspace.KeySpace, paths []keyspace.Key) []string {
	out := make([]string, len(paths))
	for index, path := range paths {
		out[index] = string(keys.FormatReadOnly(path))
	}
	return out
}

func (a *formalTupleAlgebra) combineGroupLeaves(op formalComponentBinaryOp, authority *formalComponentTerminalAuthority, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, left, right []decisionLeaf) ([]decisionLeaf, error) {
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		leftFactor, err := a.materializeOrdinaryGroup(authority, group, left)
		if err != nil {
			return nil, err
		}
		rightFactor, err := a.materializeOrdinaryGroup(authority, group, right)
		if err != nil {
			return nil, err
		}
		var result state.LaneFactor
		switch op {
		case formalComponentJoin:
			result, err = authority.product.LaneJoin(leftFactor, rightFactor)
		case formalComponentMeet:
			result, err = authority.product.LaneMeet(leftFactor, rightFactor)
		case formalComponentWiden:
			result, err = authority.product.LaneWiden(leftFactor, rightFactor)
		case formalComponentNarrow:
			result, err = authority.product.LaneNarrow(leftFactor, rightFactor)
		default:
			err = errFormalComponentMalformed
		}
		if err != nil {
			return nil, err
		}
		return a.factorOrdinaryGroup(authority, group, result)
	case formalFiberGroupValues:
		leftFactor, err := a.materializeValuesGroup(authority, group, left)
		if err != nil {
			return nil, err
		}
		rightFactor, err := a.materializeValuesGroup(authority, group, right)
		if err != nil {
			return nil, err
		}
		domain := group.valueDomain
		var result state.ValueFactor[FormalSlot]
		switch op {
		case formalComponentJoin:
			result = domain.Join(leftFactor, rightFactor)
		case formalComponentMeet:
			if domain.Meet == nil {
				return nil, errFormalComponentMalformed
			}
			result = domain.Meet(leftFactor, rightFactor)
		case formalComponentWiden:
			result = domain.Widen(leftFactor, rightFactor)
		case formalComponentNarrow:
			if domain.Narrow == nil {
				return nil, errFormalComponentMalformed
			}
			result = domain.Narrow(leftFactor, rightFactor)
		default:
			return nil, errFormalComponentMalformed
		}
		return a.factorValuesGroup(authority, group, result)
	case formalFiberGroupCoordinateLane:
		leftFactor, err := a.materializeCoordinateGroup(authority, span, group, left)
		if err != nil {
			return nil, err
		}
		rightFactor, err := a.materializeCoordinateGroup(authority, span, group, right)
		if err != nil {
			return nil, err
		}
		var result state.LaneFactor
		switch op {
		case formalComponentJoin:
			result, err = authority.product.LaneJoin(leftFactor, rightFactor)
		case formalComponentMeet:
			result, err = authority.product.LaneMeet(leftFactor, rightFactor)
		case formalComponentWiden:
			result, err = authority.product.LaneWiden(leftFactor, rightFactor)
		case formalComponentNarrow:
			result, err = authority.product.LaneNarrow(leftFactor, rightFactor)
		default:
			err = errFormalComponentMalformed
		}
		if err != nil {
			return nil, err
		}
		return a.factorCoordinateGroup(authority, span, group, result)
	default:
		return nil, errFormalComponentMalformed
	}
}

func (a *formalTupleAlgebra) groupRelation(authority *formalComponentTerminalAuthority, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, left, right []decisionLeaf, order bool) (bool, error) {
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		leftFactor, err := a.materializeOrdinaryGroup(authority, group, left)
		if err != nil {
			return false, err
		}
		rightFactor, err := a.materializeOrdinaryGroup(authority, group, right)
		if err != nil {
			return false, err
		}
		if order {
			return authority.product.LaneLessOrEq(leftFactor, rightFactor)
		}
		return authority.product.LaneEqual(leftFactor, rightFactor)
	case formalFiberGroupValues:
		leftFactor, err := a.materializeValuesGroup(authority, group, left)
		if err != nil {
			return false, err
		}
		rightFactor, err := a.materializeValuesGroup(authority, group, right)
		if err != nil {
			return false, err
		}
		if order {
			return group.valueDomain.LessOrEq(leftFactor, rightFactor), nil
		}
		return group.valueDomain.Equal(leftFactor, rightFactor), nil
	case formalFiberGroupCoordinateLane:
		leftFactor, err := a.materializeCoordinateGroup(authority, span, group, left)
		if err != nil {
			return false, err
		}
		rightFactor, err := a.materializeCoordinateGroup(authority, span, group, right)
		if err != nil {
			return false, err
		}
		if order {
			return authority.product.LaneLessOrEq(leftFactor, rightFactor)
		}
		return authority.product.LaneEqual(leftFactor, rightFactor)
	default:
		return false, errFormalComponentMalformed
	}
}

func (a *formalTupleAlgebra) canonicalGroupLeaves(authority *formalComponentTerminalAuthority, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, leaves []decisionLeaf) ([]decisionLeaf, error) {
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		factor, err := a.materializeOrdinaryGroup(authority, group, leaves)
		if err != nil {
			return nil, err
		}
		return a.factorOrdinaryGroup(authority, group, factor)
	case formalFiberGroupValues:
		factor, err := a.materializeValuesGroup(authority, group, leaves)
		if err != nil {
			return nil, err
		}
		return a.factorValuesGroup(authority, group, factor)
	case formalFiberGroupCoordinateLane:
		factor, err := a.materializeCoordinateGroup(authority, span, group, leaves)
		if err != nil {
			return nil, err
		}
		return a.factorCoordinateGroup(authority, span, group, factor)
	default:
		return nil, errFormalComponentMalformed
	}
}

// combineGroupRoots applies the complete dependent carrier directly under the
// shared guard forest. The decision kernel reconstructs every member root in
// one multi-output traversal; independent product groups are never placed in
// one explicit leaf partition.
func (a *formalTupleAlgebra) combineGroupRoots(op formalComponentBinaryOp, span formalFiberDescriptorSpan, authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, left, right formalRelationTuple, leftCare, rightCare, resultCare decisionRef) ([]decisionRef, error) {
	leftRoots, err := a.groupRoots(left, group)
	if err != nil {
		return nil, err
	}
	rightRoots, err := a.groupRoots(right, group)
	if err != nil {
		return nil, err
	}
	// The directory zip deliberately carries dependent-group members from the
	// left until this atomic group transaction runs. If both operands already
	// name the same complete physical carrier, every registered lattice law is
	// reflexive and the carried roots are the exact result under any Care
	// combination. Avoid repartitioning and refactoring that unchanged group.
	physicalIdentity := len(leftRoots) == len(rightRoots)
	for index := range leftRoots {
		physicalIdentity = physicalIdentity && leftRoots[index] == rightRoots[index]
	}
	if physicalIdentity {
		return leftRoots, nil
	}
	return a.decisions.applyVectorUnderCare(a.ctx, resultCare, leftCare, rightCare, leftRoots, rightRoots, func(leftLeaves, rightLeaves []decisionLeaf) ([]decisionLeaf, error) {
		switch {
		case len(leftLeaves) != 0 && len(rightLeaves) != 0:
			return a.combineGroupLeaves(op, authority, span, group, leftLeaves, rightLeaves)
		case len(leftLeaves) != 0 && (op == formalComponentJoin || op == formalComponentWiden || op == formalComponentNarrow):
			return a.canonicalGroupLeaves(authority, span, group, leftLeaves)
		case len(rightLeaves) != 0 && (op == formalComponentJoin || op == formalComponentWiden):
			return a.canonicalGroupLeaves(authority, span, group, rightLeaves)
		default:
			return nil, errDecisionMalformed
		}
	})
}

func (a *formalTupleAlgebra) compareGroupRoots(span formalFiberDescriptorSpan, authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, left, right formalRelationTuple, care decisionRef, order bool) (bool, error) {
	leftRoots, err := a.groupRoots(left, group)
	if err != nil {
		return false, err
	}
	rightRoots, err := a.groupRoots(right, group)
	if err != nil {
		return false, err
	}
	demands := append(append(make([]decisionRef, 0, len(leftRoots)+len(rightRoots)), leftRoots...), rightRoots...)
	regions, err := a.decisions.partitionLeafTuplesUnderCare(a.ctx, care, demands)
	if err != nil {
		return false, err
	}
	for _, region := range regions {
		if len(region.leaves) != len(demands) {
			return false, errDecisionMalformed
		}
		ok, relationErr := a.groupRelation(authority, span, group, region.leaves[:len(leftRoots)], region.leaves[len(leftRoots):], order)
		if relationErr != nil || !ok {
			return ok, relationErr
		}
	}
	return true, nil
}

func (a *formalTupleAlgebra) applyGroupRoots(span formalFiberDescriptorSpan, directory *formalFiberDirectoryArena, root formalFiberDirectoryRoot, authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, roots []decisionRef) (formalFiberDirectoryRoot, error) {
	if !group.valid() || group.variable != span.variable || len(roots) != len(group.members) {
		return formalFiberDirectoryRoot{}, errFormalComponentForeignOwner
	}
	writes := make([]formalFiberWrite, len(group.members))
	for index, ordinal := range group.members {
		member, ok := group.member(ordinal)
		address, addressOK := member.address(group)
		if !ok || !addressOK || address != ordinal || int(ordinal) < 0 || int(ordinal) >= span.count {
			return formalFiberDirectoryRoot{}, errFormalComponentForeignOwner
		}
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, roots[index]); err != nil {
			return formalFiberDirectoryRoot{}, err
		}
		writes[index] = formalFiberWrite{ordinal: address, value: formalFiberValue(roots[index])}
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		return formalFiberDirectoryRoot{}, err
	}
	next, _, err := directory.applyDelta(root, delta)
	return next, err
}

func (a *formalTupleAlgebra) writeOrdinaryFactor(tuple formalRelationTuple, group formalOrdinaryLaneFiberGroup, value state.LaneFactor) (formalRelationTuple, error) {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() || !group.valid() || group.descriptor.variable != tuple.variable {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	span, directory, authority, _ := a.span(tuple.variable)
	leaves, err := a.factorOrdinaryGroup(authority, group.descriptor, value)
	if err != nil {
		return formalRelationTuple{}, err
	}
	roots := make([]decisionRef, len(leaves))
	for index, leaf := range leaves {
		roots[index] = a.decisions.terminal(leaf)
	}
	root, err := a.applyGroupRoots(span, directory, tuple.root, authority, group.descriptor, roots)
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: tuple.variable, root: root}), nil
}

func (a *formalTupleAlgebra) writeValuesFactor(tuple formalRelationTuple, group formalValuesFiberGroup, value state.ValueFactor[FormalSlot]) (formalRelationTuple, error) {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() || !group.valid() || group.descriptor.variable != tuple.variable {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	span, directory, authority, _ := a.span(tuple.variable)
	leaves, err := a.factorValuesGroup(authority, group.descriptor, value)
	if err != nil {
		return formalRelationTuple{}, err
	}
	roots := make([]decisionRef, len(leaves))
	for index, leaf := range leaves {
		roots[index] = a.decisions.terminal(leaf)
	}
	root, err := a.applyGroupRoots(span, directory, tuple.root, authority, group.descriptor, roots)
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: tuple.variable, root: root}), nil
}

func (a *formalTupleAlgebra) writeCoordinateFactor(tuple formalRelationTuple, group formalCoordinateLaneFiberGroup, value state.LaneFactor) (formalRelationTuple, error) {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() || !group.valid() || group.descriptor.variable != tuple.variable {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	span, directory, authority, _ := a.span(tuple.variable)
	leaves, err := a.factorCoordinateGroup(authority, span, group.descriptor, value)
	if err != nil {
		return formalRelationTuple{}, err
	}
	roots := make([]decisionRef, len(leaves))
	for index, leaf := range leaves {
		roots[index] = a.decisions.terminal(leaf)
	}
	root, err := a.applyGroupRoots(span, directory, tuple.root, authority, group.descriptor, roots)
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: tuple.variable, root: root}), nil
}
