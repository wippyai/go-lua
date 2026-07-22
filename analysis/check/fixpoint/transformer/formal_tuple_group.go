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
	carrier, err := a.materializeFormalValuesGroup(authority, group, leaves)
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, err
	}
	concrete, err := formalConcreteValuesFactor(authority, carrier)
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, err
	}
	return state.ValueFactor[FormalSlot]{Top: carrier.Top, Values: concrete}, nil
}

// materializeFormalValuesGroup is the formal Values boundary.  Unlike the
// concrete State adapter above it preserves entry-dependent ValueTerms, so a
// sparse formal transaction can carry them through stabilization.
func (a *formalTupleAlgebra) materializeFormalValuesGroup(authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, leaves []decisionLeaf) (formalValuesFactor, error) {
	if group.kind != formalFiberGroupValues || len(leaves) != len(group.members) {
		return formalValuesFactor{}, errFormalComponentMalformed
	}
	if group.valueTopPosition < 0 || group.valueTopPosition >= len(leaves) || leaves[group.valueTopPosition] > 1 {
		return formalValuesFactor{}, errFormalComponentMalformed
	}
	if leaves[group.valueTopPosition] == 1 {
		return formalValuesFactor{Top: true}, nil
	}
	var values map[FormalSlot]formalValue
	bottom := product.Bottom(authority.product.Registry())
	for _, slot := range group.valueSlots {
		if slot.position < 0 || slot.position >= len(leaves) {
			return formalValuesFactor{}, errFormalComponentMalformed
		}
		value, err := formalValueFromLeaf(authority, leaves[slot.position])
		if err != nil {
			return formalValuesFactor{}, err
		}
		ground, concrete := value.concrete()
		if !concrete || !product.Equal(authority.product.Registry(), ground, bottom) {
			if values == nil {
				values = make(map[FormalSlot]formalValue)
			}
			values[slot.slot] = value
		}
	}
	return formalValuesFactor{Values: values}, nil
}

func (a *formalTupleAlgebra) factorValuesGroup(authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, value state.ValueFactor[FormalSlot]) ([]decisionLeaf, error) {
	carrier := formalValuesFactor{Top: value.Top}
	if len(value.Values) != 0 {
		carrier.Values = make(map[FormalSlot]formalValue, len(value.Values))
		for slot, ground := range value.Values {
			carrier.Values[slot] = formalGroundValue(ground)
		}
	}
	return a.factorFormalValuesGroup(authority, group, carrier)
}

func (a *formalTupleAlgebra) factorFormalValuesGroup(authority *formalComponentTerminalAuthority, group formalFiberGroupDescriptor, value formalValuesFactor) ([]decisionLeaf, error) {
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
		ground, concrete := coordinate.concrete()
		if concrete && product.Equal(authority.product.Registry(), ground, bottom) {
			continue
		}
		if slot.position < 0 || slot.position >= len(leaves) {
			return nil, errFormalComponentMalformed
		}
		leaf, err := authority.internFormalValue(coordinate)
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
	if group.kind == formalFiberGroupValues {
		return a.combineValuesGroupRoots(op, authority, group, leftRoots, rightRoots, leftCare, rightCare, resultCare)
	}
	if group.kind == formalFiberGroupCoordinateLane {
		return a.combineCoordinateGroupRoots(op, authority, span, group, leftRoots, rightRoots, leftCare, rightCare, resultCare)
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

// combineCoordinateGroupRoots lifts the registered dependent-coordinate law
// without constructing whole-family terminal rows. Every scalar is evaluated
// with both operand skeletons, so implicit defaults and heap support remain
// exact while unrelated scalar DDs never form a Cartesian product.
func (a *formalTupleAlgebra) combineCoordinateGroupRoots(
	op formalComponentBinaryOp,
	authority *formalComponentTerminalAuthority,
	span formalFiberDescriptorSpan,
	group formalFiberGroupDescriptor,
	leftRoots, rightRoots []decisionRef,
	leftCare, rightCare, resultCare decisionRef,
) ([]decisionRef, error) {
	if a == nil || authority == nil || group.kind != formalFiberGroupCoordinateLane ||
		len(leftRoots) != len(group.members) || len(rightRoots) != len(group.members) {
		return nil, errFormalComponentMalformed
	}
	out := append([]decisionRef(nil), leftRoots...)
	decodeSkeleton := func(family formalCoordinateFamilyFiberGroup, leaf decisionLeaf) (state.CoordinateFamilySkeleton, error) {
		if leaf == 0 {
			return authority.product.CoordinateSkeletonBottom(family.family, span.keys)
		}
		terminal, err := authority.terminal(leaf)
		if err != nil || terminal.kind != formalComponentCoordinateSkeleton || terminal.skeleton.Family() != family.family {
			if err != nil {
				return state.CoordinateFamilySkeleton{}, err
			}
			return state.CoordinateFamilySkeleton{}, errFormalComponentMalformed
		}
		return terminal.skeleton, nil
	}
	combineSkeleton := func(left, right state.CoordinateFamilySkeleton) (state.CoordinateFamilySkeleton, error) {
		switch op {
		case formalComponentJoin:
			return authority.product.CoordinateSkeletonJoin(left, right)
		case formalComponentMeet:
			return authority.product.CoordinateSkeletonMeet(left, right)
		case formalComponentWiden:
			return authority.product.CoordinateSkeletonWiden(left, right)
		case formalComponentNarrow:
			return authority.product.CoordinateSkeletonNarrow(left, right)
		default:
			return state.CoordinateFamilySkeleton{}, errFormalComponentMalformed
		}
	}
	combineScalar := func(left, right state.CoordinateScalarFactor) (state.CoordinateScalarFactor, error) {
		switch op {
		case formalComponentJoin:
			return authority.product.CoordinateScalarJoin(left, right)
		case formalComponentMeet:
			return authority.product.CoordinateScalarMeet(left, right)
		case formalComponentWiden:
			return authority.product.CoordinateScalarWiden(left, right)
		case formalComponentNarrow:
			return authority.product.CoordinateScalarNarrow(left, right)
		default:
			return state.CoordinateScalarFactor{}, errFormalComponentMalformed
		}
	}
	oneSided := func(leftLeaves, rightLeaves []decisionLeaf) ([]decisionLeaf, bool, error) {
		switch {
		case len(leftLeaves) != 0 && len(rightLeaves) == 0 && (op == formalComponentJoin || op == formalComponentWiden || op == formalComponentNarrow):
			return append([]decisionLeaf(nil), leftLeaves...), true, nil
		case len(leftLeaves) == 0 && len(rightLeaves) != 0 && (op == formalComponentJoin || op == formalComponentWiden):
			return append([]decisionLeaf(nil), rightLeaves...), true, nil
		case len(leftLeaves) == 0 || len(rightLeaves) == 0:
			return nil, true, errDecisionMalformed
		default:
			return nil, false, nil
		}
	}
	for _, family := range group.coordinateFamilies {
		position := family.skeletonPosition
		if position < 0 || position >= len(group.members) {
			return nil, errFormalComponentMalformed
		}
		var skeletonRoot decisionRef
		if leftRoots[position] == rightRoots[position] {
			skeletonRoot = leftRoots[position]
		} else {
			skeletonRoots, err := a.decisions.applyVectorUnderCare(
				a.ctx, resultCare, leftCare, rightCare,
				[]decisionRef{leftRoots[position]}, []decisionRef{rightRoots[position]},
				func(leftLeaves, rightLeaves []decisionLeaf) ([]decisionLeaf, error) {
					if result, handled, oneErr := oneSided(leftLeaves, rightLeaves); handled {
						return result, oneErr
					}
					left, decodeErr := decodeSkeleton(family, leftLeaves[0])
					if decodeErr != nil {
						return nil, decodeErr
					}
					right, decodeErr := decodeSkeleton(family, rightLeaves[0])
					if decodeErr != nil {
						return nil, decodeErr
					}
					result, combineErr := combineSkeleton(left, right)
					if combineErr != nil {
						return nil, combineErr
					}
					leaf, combineErr := a.internFormalCoordinateSkeleton(authority, span, family.family, result)
					return []decisionLeaf{leaf}, combineErr
				},
			)
			if err != nil || len(skeletonRoots) != 1 {
				if err == nil {
					err = errDecisionMalformed
				}
				return nil, err
			}
			skeletonRoot = skeletonRoots[0]
		}
		out[position] = skeletonRoot
		for _, scalarPosition := range family.scalarPositions {
			if scalarPosition < 0 || scalarPosition >= len(group.members) {
				return nil, errFormalComponentMalformed
			}
			ordinal := group.members[scalarPosition]
			descriptor := span.forest.descriptors[span.first+int(ordinal)]
			if descriptor.role != formalFiberCoordinate || descriptor.coordinateKind != formalFiberCoordinateFamilyScalar ||
				descriptor.coordinate.Family() != family.family {
				return nil, errFormalComponentMalformed
			}
			slot := descriptor.coordinate
			if leftRoots[position] == rightRoots[position] && leftRoots[scalarPosition] == rightRoots[scalarPosition] {
				out[scalarPosition] = leftRoots[scalarPosition]
				continue
			}
			roots, scalarErr := a.decisions.applyVectorUnderCare(
				a.ctx, resultCare, leftCare, rightCare,
				[]decisionRef{leftRoots[position], leftRoots[scalarPosition]},
				[]decisionRef{rightRoots[position], rightRoots[scalarPosition]},
				func(leftLeaves, rightLeaves []decisionLeaf) ([]decisionLeaf, error) {
					if result, handled, oneErr := oneSided(leftLeaves, rightLeaves); handled {
						return result, oneErr
					}
					leftSkeleton, decodeErr := decodeSkeleton(family, leftLeaves[0])
					if decodeErr != nil {
						return nil, decodeErr
					}
					rightSkeleton, decodeErr := decodeSkeleton(family, rightLeaves[0])
					if decodeErr != nil {
						return nil, decodeErr
					}
					outputSkeleton, decodeErr := combineSkeleton(leftSkeleton, rightSkeleton)
					if decodeErr != nil {
						return nil, decodeErr
					}
					decodeScalar := func(skeleton state.CoordinateFamilySkeleton, leaf decisionLeaf) (state.CoordinateScalarFactor, error) {
						if leaf == 0 {
							return authority.product.CoordinateDefault(skeleton, slot)
						}
						terminal, terminalErr := authority.terminal(leaf)
						if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
							if terminalErr != nil {
								return state.CoordinateScalarFactor{}, terminalErr
							}
							return state.CoordinateScalarFactor{}, errFormalComponentMalformed
						}
						equal, equalErr := authority.product.CoordinateSlotEqual(terminal.scalar.Slot(), slot)
						if equalErr != nil || !equal {
							if equalErr != nil {
								return state.CoordinateScalarFactor{}, equalErr
							}
							return state.CoordinateScalarFactor{}, errFormalComponentMalformed
						}
						return terminal.scalar, nil
					}
					leftScalar, decodeErr := decodeScalar(leftSkeleton, leftLeaves[1])
					if decodeErr != nil {
						return nil, decodeErr
					}
					rightScalar, decodeErr := decodeScalar(rightSkeleton, rightLeaves[1])
					if decodeErr != nil {
						return nil, decodeErr
					}
					result, decodeErr := combineScalar(leftScalar, rightScalar)
					if decodeErr != nil {
						return nil, decodeErr
					}
					scalarLeaf := decisionLeaf(0)
					omitted, omitErr := authority.product.CoordinateScalarIsOmitted(outputSkeleton, result)
					if omitErr != nil {
						return nil, omitErr
					}
					if !omitted {
						scalarLeaf, omitErr = authority.internCoordinateScalar(result)
						if omitErr != nil {
							return nil, omitErr
						}
					}
					// applyVectorUnderCare retains the operand width. The first
					// result is intentionally dead; the family skeleton was lifted
					// once above and is not re-interned per scalar valuation.
					return []decisionLeaf{0, scalarLeaf}, nil
				},
			)
			if scalarErr != nil || len(roots) != 2 {
				if scalarErr == nil {
					scalarErr = errFormalComponentMalformed
				}
				return nil, scalarErr
			}
			out[scalarPosition] = roots[1]
		}
	}
	return out, nil
}

// combineValuesGroupRoots is the product-isomorphic Values lattice lift. Top
// is shared by every coordinate, while finite slots are independent product
// values. Correlating the complete sparse slot inventory would manufacture a
// Cartesian terminal row that the registered ValueFactor lattice immediately
// factors again; lifting each Top+slot component preserves the exact law and
// the DD identity of every unrelated slot.
func (a *formalTupleAlgebra) combineValuesGroupRoots(
	op formalComponentBinaryOp,
	authority *formalComponentTerminalAuthority,
	group formalFiberGroupDescriptor,
	leftRoots, rightRoots []decisionRef,
	leftCare, rightCare, resultCare decisionRef,
) ([]decisionRef, error) {
	if a == nil || authority == nil || group.kind != formalFiberGroupValues ||
		len(leftRoots) != len(group.members) || len(rightRoots) != len(group.members) ||
		group.valueTopPosition < 0 || group.valueTopPosition >= len(group.members) {
		return nil, errFormalComponentMalformed
	}
	// product.Value has no registered Narrow, so ValueFactor's total narrowing
	// law retains the complete previous finite map unchanged.
	if op == formalComponentNarrow {
		return append([]decisionRef(nil), leftRoots...), nil
	}
	combineGround := func(left, right decisionLeaf) (decisionLeaf, error) {
		switch op {
		case formalComponentJoin, formalComponentWiden:
			if left == 0 {
				return right, nil
			}
			if right == 0 {
				return left, nil
			}
		case formalComponentMeet:
			if left == 0 || right == 0 {
				return 0, nil
			}
		}
		if left != 0 && right != 0 {
			if op == formalComponentJoin || op == formalComponentWiden {
				return authority.joinFormalValueLeaves(left, right)
			}
			return authority.combine(a.ctx, op, left, right)
		}
		return 0, errFormalComponentMalformed
	}
	combine := func(left, right []decisionLeaf, slot *formalValueSlotFiber) ([]decisionLeaf, error) {
		switch {
		case len(left) != 0 && len(right) == 0 && (op == formalComponentJoin || op == formalComponentWiden):
			return append([]decisionLeaf(nil), left...), nil
		case len(left) == 0 && len(right) != 0 && (op == formalComponentJoin || op == formalComponentWiden):
			return append([]decisionLeaf(nil), right...), nil
		case len(left) != 0 && len(right) != 0:
		default:
			return nil, errDecisionMalformed
		}
		width := 1
		if slot != nil {
			width = 2
		}
		if len(left) != width || len(right) != width || left[0] > 1 || right[0] > 1 {
			return nil, errDecisionMalformed
		}
		leaves := make([]decisionLeaf, width)
		switch op {
		case formalComponentJoin, formalComponentWiden:
			if left[0] == 1 || right[0] == 1 {
				leaves[0] = 1
			}
		case formalComponentMeet, formalComponentNarrow:
			if left[0] == 1 && right[0] == 1 {
				leaves[0] = 1
			}
		default:
			return nil, errFormalComponentMalformed
		}
		if slot == nil || leaves[0] == 1 {
			return leaves, nil
		}
		switch {
		case left[0] == 1:
			leaves[1] = right[1]
		case right[0] == 1:
			leaves[1] = left[1]
		default:
			var err error
			leaves[1], err = combineGround(left[1], right[1])
			if err != nil {
				return nil, err
			}
		}
		return leaves, nil
	}
	topPosition := group.valueTopPosition
	apply := func(slot *formalValueSlotFiber, left, right []decisionRef) ([]decisionRef, error) {
		return a.decisions.applyVectorUnderCare(a.ctx, resultCare, leftCare, rightCare, left, right,
			func(leftLeaves, rightLeaves []decisionLeaf) ([]decisionLeaf, error) {
				return combine(leftLeaves, rightLeaves, slot)
			})
	}
	var top []decisionRef
	var err error
	if leftRoots[topPosition] == rightRoots[topPosition] {
		top = []decisionRef{leftRoots[topPosition]}
	} else {
		top, err = apply(nil, []decisionRef{leftRoots[topPosition]}, []decisionRef{rightRoots[topPosition]})
		if err != nil || len(top) != 1 {
			if err == nil {
				err = errDecisionMalformed
			}
			return nil, err
		}
	}
	out := append([]decisionRef(nil), leftRoots...)
	out[topPosition] = top[0]
	for index := range group.valueSlots {
		slot := &group.valueSlots[index]
		if slot.position < 0 || slot.position >= len(out) {
			return nil, errFormalComponentMalformed
		}
		if leftRoots[slot.position] == decisionFalse && rightRoots[slot.position] == decisionFalse {
			out[slot.position] = decisionFalse
			continue
		}
		if leftRoots[topPosition] == rightRoots[topPosition] && leftRoots[slot.position] == rightRoots[slot.position] {
			out[slot.position] = leftRoots[slot.position]
			continue
		}
		roots, err := apply(slot,
			[]decisionRef{leftRoots[topPosition], leftRoots[slot.position]},
			[]decisionRef{rightRoots[topPosition], rightRoots[slot.position]},
		)
		if err != nil || len(roots) != 2 || roots[0] != top[0] {
			if err == nil {
				err = errFormalComponentMalformed
			}
			return nil, err
		}
		out[slot.position] = roots[1]
	}
	return out, nil
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
	carrier := formalValuesFactor{Top: value.Top}
	if len(value.Values) != 0 {
		carrier.Values = make(map[FormalSlot]formalValue, len(value.Values))
		for slot, ground := range value.Values {
			carrier.Values[slot] = formalGroundValue(ground)
		}
	}
	return a.writeFormalValuesFactor(tuple, group, carrier)
}

// writeFormalValuesFactor is the canonical seed/publication transaction for
// the sum carrier. Concrete State callers use writeValuesFactor above; a
// symbolic seed remains a typed Values leaf until entry specialization.
func (a *formalTupleAlgebra) writeFormalValuesFactor(tuple formalRelationTuple, group formalValuesFiberGroup, value formalValuesFactor) (formalRelationTuple, error) {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() || !group.valid() || group.descriptor.variable != tuple.variable {
		if err != nil {
			return formalRelationTuple{}, err
		}
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	span, directory, authority, _ := a.span(tuple.variable)
	leaves, err := a.factorFormalValuesGroup(authority, group.descriptor, value)
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
