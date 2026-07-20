package transformer

import (
	"fmt"
	"sort"
	"time"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalBranchRelationsStep is the formal binding of the one canonical E3
// factor program. The kernels and stage declaration remain owned by
// factapply; this type retains only checked tuple carrier capabilities.
type formalBranchRelationsStep struct {
	factors factapply.BranchRelationFactors
	stages  []factapply.BranchRelationFactorStage
	plans   []formalBranchRelationFactorPlan
	// stageFactorGroups are the connected components of physical publication
	// overlap inside each semantically independent stage. Disjoint groups may
	// evaluate from the same stage base; factors sharing representation fibers
	// compose in canonical factor order inside their group.
	stageFactorGroups [][][]int
	// stageCoordinateWriteGroups is the exact physical lane set whose family
	// fibers may change in each dependency stage. Family patches stay sparse
	// within a stage; each lane is composed once at the stage visibility
	// boundary before a dependent stage may consume it.
	stageCoordinateWriteGroups [][]formalFiberGroupDescriptor
	edge                       transfer.EdgeContext
}

type formalBranchRelationFactorPlan struct {
	current                   formalBranchRelationRolePlan
	original                  formalBranchRelationRolePlan
	consequence               *formalPresenceImplicationStep
	currentProjectionOrdinals []formalFiberOrdinal
	originalReadOrdinals      []formalFiberOrdinal
	writeOrdinals             []formalFiberOrdinal
}

type formalBranchRelationRolePlan struct {
	values      []formalBranchRelationValuePlan
	valueTop    int
	lanes       []formalFiberGroupDescriptor
	laneWrites  []int
	coordinates []formalBranchRelationCoordinatePlan
}

type formalBranchRelationValuePlan struct {
	slot     FormalSlot
	position int
}

// One coordinate plan addresses one registered family cone. The factor
// kernel sees only layout slots, while publication retains the complete
// family representation so a skeleton change cannot reinterpret siblings.
type formalBranchRelationCoordinatePlan struct {
	group          formalFiberGroupDescriptor
	family         formalCoordinateFamilyFiberGroup
	slots          []state.CoordinateSlot
	positions      []int
	writePositions []int
	writes         bool
	publication    factapply.BranchRelationCoordinatePublicationLaw
}

func freezeFormalBranchRelationsStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalBranchRelationsStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepBranchRelations {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() || body.pathSemantics == nil || !body.pathSemantics.Valid() ||
		body.relation.code != operator.code || !body.productDomain.Valid() {
		return nil, fmt.Errorf("BranchRelations has no formal product ownership")
	}
	if err := validateFormalFiberProductGroups(span, body.productDomain); err != nil {
		return nil, err
	}
	inventory, err := freezeFormalBranchPointCoordinateInventory(body, span.keys, span.rekey, step.branch.Point())
	if err != nil {
		return nil, fmt.Errorf("BranchRelations point coordinate inventory: %w", err)
	}
	if !inventory.ValidFor(body.productDomain, span.keys) {
		return nil, fmt.Errorf("BranchRelations has no frozen coordinate inventory")
	}
	factors, err := body.pathSemantics.PrepareFormalBranchRelationFactors(body.productDomain, step.branch, inventory, span.rekey, span.keys)
	if err != nil {
		return nil, fmt.Errorf("BranchRelations factor preparation: %w", err)
	}
	plans := make([]formalBranchRelationFactorPlan, factors.Len())
	for index := 0; index < factors.Len(); index++ {
		layout, factorNative := factors.FactorLayout(index)
		if !factorNative {
			dependency, consequence := factors.PresenceImplicationDependencyPlan(index)
			if !consequence {
				// Every executable atom must have exactly one factor law. There is
				// no solve-time State fallback for an unknown atom.
				return nil, fmt.Errorf("BranchRelations atom %d/%d (source %d) has neither a factor kernel nor a consequence law", index, factors.Len(), factors.FactorSource(index))
			}
			plans[index].consequence, err = freezeFormalPresenceImplicationDependency(program, body, span, dependency, nil)
			if err != nil {
				return nil, err
			}
			plans[index].currentProjectionOrdinals = append([]formalFiberOrdinal(nil), plans[index].consequence.projectionOrdinals...)
			plans[index].writeOrdinals = append([]formalFiberOrdinal(nil), plans[index].consequence.writeOrdinals...)
			continue
		}
		plans[index].current, err = freezeFormalBranchRelationRolePlan(program, body, span, layout.CurrentValueRoles(), layout.CurrentLanes(), layout.CurrentCoordinates())
		if err != nil {
			return nil, fmt.Errorf("BranchRelations atom %d current role: %w", index, err)
		}
		plans[index].current.laneWrites = layout.CurrentLaneWriteOrdinals()
		for coordinateIndex := range plans[index].current.coordinates {
			law, ok := factors.FactorCoordinatePublicationLaw(index, coordinateIndex)
			if !ok {
				return nil, fmt.Errorf("BranchRelations coordinate %d has no sealed publication law", coordinateIndex)
			}
			plans[index].current.coordinates[coordinateIndex].publication = law
		}
		for _, ordinal := range plans[index].current.laneWrites {
			if ordinal < 0 || ordinal >= len(plans[index].current.lanes) {
				return nil, fmt.Errorf("BranchRelations lane-write ordinal is outside its factor layout")
			}
		}
		plans[index].original, err = freezeFormalBranchRelationRolePlan(program, body, span, layout.OriginalValueRoles(), layout.OriginalLanes(), layout.OriginalCoordinates())
		if err != nil {
			return nil, fmt.Errorf("BranchRelations atom %d original role: %w", index, err)
		}
		appendRoleReads := func(ordinals []formalFiberOrdinal, role formalBranchRelationRolePlan) ([]formalFiberOrdinal, error) {
			if len(role.values) != 0 {
				if role.valueTop < 0 || role.valueTop >= span.count {
					return nil, errFormalComponentMalformed
				}
				ordinals = append(ordinals, formalFiberOrdinal(role.valueTop))
			}
			for _, value := range role.values {
				if value.position < 0 || value.position >= span.count {
					return nil, errFormalComponentMalformed
				}
				ordinals = append(ordinals, formalFiberOrdinal(value.position))
			}
			for _, lane := range role.lanes {
				ordinals = append(ordinals, lane.members...)
			}
			for _, coordinate := range role.coordinates {
				ordinals = append(ordinals, coordinate.family.skeleton)
				if coordinate.publication == factapply.BranchRelationCoordinatePublicationReconcile {
					// Only topology reconciliation observes prior sibling scalars.
					ordinals = append(ordinals, coordinate.family.scalars...)
					continue
				}
				for _, position := range coordinate.positions {
					if position < 0 || position >= len(coordinate.family.scalars) {
						return nil, errFormalComponentMalformed
					}
					ordinals = append(ordinals, coordinate.family.scalars[position])
				}
			}
			return ordinals, nil
		}
		plans[index].currentProjectionOrdinals, err = appendRoleReads(nil, plans[index].current)
		if err != nil {
			return nil, fmt.Errorf("BranchRelations atom %d current read footprint: %w", index, err)
		}
		plans[index].originalReadOrdinals, err = appendRoleReads(nil, plans[index].original)
		if err != nil {
			return nil, fmt.Errorf("BranchRelations atom %d original read footprint: %w", index, err)
		}
		writeOrdinals := []formalFiberOrdinal{0}
		for _, write := range layout.CurrentValueWriteOrdinals() {
			if write < 0 || write >= len(plans[index].current.values) {
				return nil, fmt.Errorf("BranchRelations Values-write ordinal is outside its factor layout")
			}
			writeOrdinals = append(writeOrdinals, formalFiberOrdinal(plans[index].current.values[write].position))
		}
		if layout.WritesValuesTop() {
			if plans[index].current.valueTop < 0 {
				return nil, fmt.Errorf("BranchRelations Values-top write has no formal fiber")
			}
			writeOrdinals = append(writeOrdinals, formalFiberOrdinal(plans[index].current.valueTop))
		}
		for _, write := range plans[index].current.laneWrites {
			writeOrdinals = append(writeOrdinals, plans[index].current.lanes[write].members...)
		}
		coordinateWrites := layout.CurrentCoordinateWriteOrdinals()
		skeletonWrites := layout.CurrentCoordinateSkeletonWrites()
		if len(coordinateWrites) != len(plans[index].current.coordinates) || len(skeletonWrites) != len(plans[index].current.coordinates) {
			return nil, fmt.Errorf("BranchRelations coordinate-write layout width mismatch")
		}
		for coordinateIndex, coordinate := range plans[index].current.coordinates {
			plans[index].current.coordinates[coordinateIndex].writes = skeletonWrites[coordinateIndex] || len(coordinateWrites[coordinateIndex]) != 0
			if skeletonWrites[coordinateIndex] {
				// Replace and reconcile own the complete family image. Patch laws
				// below retain exact scalar authority only.
				writeOrdinals = append(writeOrdinals, coordinate.family.skeleton)
				writeOrdinals = append(writeOrdinals, coordinate.family.scalars...)
				continue
			}
			for _, write := range coordinateWrites[coordinateIndex] {
				if write < 0 || write >= len(coordinate.positions) {
					return nil, fmt.Errorf("BranchRelations coordinate-write ordinal is outside its factor layout")
				}
				position := coordinate.positions[write]
				if position < 0 || position >= len(coordinate.family.scalars) {
					return nil, fmt.Errorf("BranchRelations coordinate-write position is outside its formal family")
				}
				writeOrdinals = append(writeOrdinals, coordinate.family.scalars[position])
				plans[index].current.coordinates[coordinateIndex].writePositions = append(
					plans[index].current.coordinates[coordinateIndex].writePositions, position,
				)
			}
		}
		plans[index].writeOrdinals, err = sealFormalPresenceOrdinals(span, writeOrdinals)
		if err != nil {
			return nil, err
		}
		plans[index].currentProjectionOrdinals = append(plans[index].currentProjectionOrdinals, plans[index].writeOrdinals...)
		plans[index].currentProjectionOrdinals, err = sealFormalPresenceOrdinals(span, plans[index].currentProjectionOrdinals)
		if err != nil {
			return nil, err
		}
		if len(plans[index].currentProjectionOrdinals) != 0 && plans[index].currentProjectionOrdinals[0] == 0 {
			plans[index].currentProjectionOrdinals = plans[index].currentProjectionOrdinals[1:]
		}
		plans[index].originalReadOrdinals, err = sealFormalPresenceOrdinals(span, plans[index].originalReadOrdinals)
		if err != nil {
			return nil, err
		}
	}
	stages := factors.Stages()
	stageCoordinateWriteGroups := make([][]formalFiberGroupDescriptor, len(stages))
	stageFactorGroups := make([][][]int, len(stages))
	seen := make([]bool, factors.Len())
	for stageIndex, stage := range stages {
		if !factors.StageIndependent(stageIndex) {
			return nil, fmt.Errorf("BranchRelations stage %d is not independent", stageIndex)
		}
		for _, factor := range stage.Factors() {
			if factor < 0 || factor >= len(plans) || seen[factor] {
				return nil, fmt.Errorf("BranchRelations stage inventory is malformed")
			}
			seen[factor] = true
			for _, coordinate := range plans[factor].current.coordinates {
				if !coordinate.writes {
					continue
				}
				duplicate := false
				for _, prior := range stageCoordinateWriteGroups[stageIndex] {
					duplicate = duplicate || prior.lane.Ordinal() == coordinate.group.lane.Ordinal()
				}
				if !duplicate {
					stageCoordinateWriteGroups[stageIndex] = append(stageCoordinateWriteGroups[stageIndex], coordinate.group)
				}
			}
		}
		stageFactorGroups[stageIndex] = groupFormalBranchStageFactors(stage.Factors(), plans)
	}
	for index := range seen {
		if !seen[index] {
			return nil, fmt.Errorf("BranchRelations factor %d is outside its stage inventory", index)
		}
	}
	return &formalBranchRelationsStep{
		factors: factors, stages: stages, plans: plans,
		stageFactorGroups:          stageFactorGroups,
		stageCoordinateWriteGroups: stageCoordinateWriteGroups,
		edge:                       transfer.EdgeContext{Graph: body.graph, Registry: body.productDomain.Registry(), Edge: cfg.Edge{From: step.branch.Point(), Cond: step.branch.Cond()}, HasCond: true},
	}, nil
}

func groupFormalBranchStageFactors(factors []int, plans []formalBranchRelationFactorPlan) [][]int {
	groups := make([][]int, 0, len(factors))
	for _, factor := range factors {
		merged := []int{factor}
		for groupIndex := 0; groupIndex < len(groups); {
			overlaps := false
			for _, member := range groups[groupIndex] {
				if formalBranchFactorWritesOverlap(plans[factor].writeOrdinals, plans[member].writeOrdinals) {
					overlaps = true
					break
				}
			}
			if !overlaps {
				groupIndex++
				continue
			}
			merged = append(merged, groups[groupIndex]...)
			groups = append(groups[:groupIndex], groups[groupIndex+1:]...)
		}
		sort.Ints(merged)
		insert := sort.Search(len(groups), func(index int) bool { return groups[index][0] > merged[0] })
		groups = append(groups, nil)
		copy(groups[insert+1:], groups[insert:])
		groups[insert] = merged
	}
	return groups
}

func formalBranchFactorWritesOverlap(left, right []formalFiberOrdinal) bool {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if left[leftIndex] == 0 {
			leftIndex++
			continue
		}
		if right[rightIndex] == 0 {
			rightIndex++
			continue
		}
		switch {
		case left[leftIndex] < right[rightIndex]:
			leftIndex++
		case right[rightIndex] < left[leftIndex]:
			rightIndex++
		default:
			return true
		}
	}
	return false
}

// freezeFormalBranchPointCoordinateInventory transports the canonical
// point-relative producer theorem into the body's formal keyspace. A formal
// tuple is body-wide storage, but a branch factor may close only over
// coordinates whose static producers can reach that branch. Feeding the
// body-wide inventory back into dependency planning would let unrelated or
// future equality facts manufacture a larger operation-specific universe.
func freezeFormalBranchPointCoordinateInventory(
	body *relationProgramBody,
	formalKeys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
	point cfg.Point,
) (state.CoordinateFactorInventory, error) {
	if body == nil || formalKeys == nil || !formalKeys.Valid() || !body.productDomain.Valid() ||
		!body.productDomain.OwnsCoordinateFormalRootRekey(rekey) {
		return state.CoordinateFactorInventory{}, fmt.Errorf("formal branch point inventory is unowned")
	}
	pointwise, err := freezeRelationCoordinateFactorInventory(body)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	concrete, err := pointwise.At(point)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	return rekeyFormalCoordinateFactorInventory(body.productDomain, formalKeys, rekey, concrete)
}

// rekeyFormalCoordinateFactorInventory is the sole concrete-to-formal
// coordinate inventory transport. Both tuple-schema closure and executable
// branch preparation use it, so their point-relative dependency universes
// cannot drift.
func rekeyFormalCoordinateFactorInventory(
	domain state.ProductDomain,
	formalKeys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
	concrete state.CoordinateFactorInventory,
) (state.CoordinateFactorInventory, error) {
	if formalKeys == nil || !formalKeys.Valid() || !domain.Valid() ||
		!domain.OwnsCoordinateFormalRootRekey(rekey) {
		return state.CoordinateFactorInventory{}, fmt.Errorf("formal coordinate inventory rekey is unowned")
	}
	mapped := make([]state.CoordinateSlot, 0, concrete.Len())
	for index, slot := range concrete.Slots() {
		next, mapErr := domain.RekeyCoordinateSlotFormal(rekey, slot)
		if mapErr != nil {
			return state.CoordinateFactorInventory{}, fmt.Errorf("coordinate %d in family %q: %w", index, slot.Family().ID(), mapErr)
		}
		mapped = append(mapped, next)
	}
	seed, err := domain.SealCoordinateFactorInventory(formalKeys, mapped)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	return domain.CloseCoordinateFactorInventory(formalKeys, seed)
}

func freezeFormalBranchRelationRolePlan(
	program *RelationProgram,
	body *relationProgramBody,
	span formalFiberDescriptorSpan,
	roles []factapply.BranchRelationValueRole,
	lanes []state.ProductLane,
	coordinates []factapply.BranchRelationCoordinateLayout,
) (formalBranchRelationRolePlan, error) {
	out := formalBranchRelationRolePlan{values: make([]formalBranchRelationValuePlan, len(roles)), valueTop: -1, lanes: make([]formalFiberGroupDescriptor, len(lanes)), coordinates: make([]formalBranchRelationCoordinatePlan, len(coordinates))}
	valuesGroup, valuesOK := span.valuesGroup()
	if valuesOK {
		out.valueTop = int(valuesGroup.descriptor.valueTop)
	}
	for index, role := range roles {
		symbol, ok := role.LexicalSymbol()
		if !ok {
			return formalBranchRelationRolePlan{}, fmt.Errorf("BranchRelations Values role is unsealed")
		}
		slot, ok := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(symbol))
		if !ok || !valuesOK {
			return formalBranchRelationRolePlan{}, fmt.Errorf("BranchRelations Values role has no evolving Middle slot")
		}
		member, memberOK := valuesGroup.slot(slot)
		if !memberOK {
			return formalBranchRelationRolePlan{}, fmt.Errorf("BranchRelations Values role is outside the formal Values group")
		}
		out.values[index] = formalBranchRelationValuePlan{slot: slot, position: int(member.ordinal)}
	}
	for index, lane := range lanes {
		if group, ok := span.coordinateLaneGroup(lane); ok {
			out.lanes[index] = group.descriptor
		} else if group, ok := span.ordinaryLaneGroup(lane); ok {
			out.lanes[index] = group.descriptor
		} else {
			return formalBranchRelationRolePlan{}, fmt.Errorf("BranchRelations lane %q is outside the formal product", lane.ID())
		}
	}
	for index, layout := range coordinates {
		group, ok := span.coordinateLaneGroup(layout.Family().Lane())
		if !ok {
			return formalBranchRelationRolePlan{}, fmt.Errorf("BranchRelations coordinate family has no formal lane")
		}
		var family formalCoordinateFamilyFiberGroup
		found := false
		for _, candidate := range group.families() {
			if coordinateFamilySame(candidate.family, layout.Family()) {
				family, found = candidate, true
				break
			}
		}
		if !found {
			return formalBranchRelationRolePlan{}, fmt.Errorf("BranchRelations coordinate family is outside the formal product")
		}
		slots := layout.Slots()
		positions := make([]int, len(slots))
		for slotIndex, slot := range slots {
			position, ok := freezeFormalBranchCoordinatePosition(body.productDomain, span, family, slot)
			if !ok {
				hash, hashErr := body.productDomain.CoordinateSlotHash(slot)
				return formalBranchRelationRolePlan{}, fmt.Errorf(
					"BranchRelations coordinate slot %d in family %q (hash=%x err=%v) is outside %d frozen scalars",
					slotIndex, layout.Family().ID(), hash, hashErr, len(family.scalars),
				)
			}
			positions[slotIndex] = position
		}
		out.coordinates[index] = formalBranchRelationCoordinatePlan{group: group.descriptor, family: family, slots: slots, positions: positions}
	}
	return out, nil
}

func freezeFormalBranchCoordinatePosition(domain state.ProductDomain, span formalFiberDescriptorSpan, family formalCoordinateFamilyFiberGroup, slot state.CoordinateSlot) (int, bool) {
	for index, ordinal := range family.scalars {
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		equal, err := domain.CoordinateSlotEqual(descriptor.coordinate, slot)
		if err != nil {
			return 0, false
		}
		if equal {
			return index, true
		}
	}
	return 0, false
}

func (a *formalTupleAlgebra) applyFormalBranchRelations(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if a == nil || operator.branchRelations == nil || operator.kind != formalRelationCellStep || operator.code == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal BranchRelations has no complete factor transaction")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		return predecessor, err
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || authority.code != operator.code {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	mark := a.decisions.checkpoint()
	if a.evalTrace != nil && a.evalTrace.active != nil {
		detail := a.evalTrace.active
		detail.branchRelationsPlan = operator.branchRelations
		detail.branchRelationFactors = make([]formalBranchRelationEvalTraceFactor, len(operator.branchRelations.plans))
		for index, plan := range operator.branchRelations.plans {
			detail.branchRelationFactors[index] = formalBranchRelationEvalTraceFactor{
				factor: index, source: int(operator.branchRelations.factors.FactorSource(index)), consequence: plan.consequence != nil,
				currentRoots: len(plan.currentProjectionOrdinals), originalRoots: len(plan.originalReadOrdinals), writeRoots: len(plan.writeOrdinals),
			}
		}
	}
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	current := predecessor
	for stageIndex := range operator.branchRelations.stages {
		stageBase := current
		stageCare, err := a.care(stageBase)
		if err != nil {
			return fail(err)
		}
		stageWrites := make([]formalFiberWrite, 0)
		for groupIndex, group := range operator.branchRelations.stageFactorGroups[stageIndex] {
			groupCurrent := stageBase
			for _, factorIndex := range group {
				result, factorErr := a.applyFormalBranchRelationFactor(predecessor, groupCurrent, operator.branchRelations, factorIndex)
				if factorErr != nil {
					return fail(fmt.Errorf("transformer: BranchRelations stage %d group %d factor %d: %w", stageIndex, groupIndex, factorIndex, factorErr))
				}
				groupCurrent, factorErr = a.applyFormalBranchRelationFactorResult(groupCurrent, result)
				if factorErr != nil {
					return fail(fmt.Errorf("transformer: BranchRelations stage %d group %d factor %d commit: %w", stageIndex, groupIndex, factorIndex, factorErr))
				}
				if groupCurrent.bottom() {
					break
				}
			}
			groupCare, careErr := a.care(groupCurrent)
			if careErr != nil {
				return fail(careErr)
			}
			stageCare, err = a.decisions.apply(a.ctx, uint8(decisionAnd), true, stageCare, groupCare, decisionLeafAnd)
			if err != nil {
				return fail(err)
			}
			groupWrites, groupErr := a.diffFormalBranchRelationGroup(stageBase, groupCurrent, group, operator.branchRelations.plans)
			if groupErr != nil {
				return fail(groupErr)
			}
			for _, write := range groupWrites {
				for _, prior := range stageWrites {
					if prior.ordinal == write.ordinal {
						return fail(fmt.Errorf("transformer: independent BranchRelations stage overlaps fiber %d", write.ordinal))
					}
				}
				stageWrites = append(stageWrites, write)
			}
		}
		priorCare, err := directory.valueAt(stageBase.root, 0)
		if err != nil {
			return fail(err)
		}
		if priorCare != formalFiberValue(stageCare) {
			careDescriptor := span.forest.descriptors[span.first]
			if err := a.validateDescriptorRoot(authority, careDescriptor, stageCare); err != nil {
				return fail(fmt.Errorf("transformer: BranchRelations stage %d Care: %w", stageIndex, err))
			}
			stageWrites = append(stageWrites, formalFiberWrite{ordinal: 0, value: formalFiberValue(stageCare)})
		}
		if len(stageWrites) != 0 {
			delta, deltaErr := directory.sealDelta(stageWrites)
			if deltaErr != nil {
				return fail(deltaErr)
			}
			root, _, applyErr := directory.applyDelta(stageBase.root, delta)
			if applyErr != nil {
				return fail(applyErr)
			}
			current = a.normalize(formalRelationTuple{variable: predecessor.variable, root: root})
		}
		if current.bottom() {
			return current, nil
		}
		for _, group := range operator.branchRelations.stageCoordinateWriteGroups[stageIndex] {
			if err := a.cacheFormalCoordinateGroupTuple(current, group); err != nil {
				return fail(fmt.Errorf("stage %d coordinate group commit: %w", stageIndex, err))
			}
		}
	}
	return current, nil
}

func (a *formalTupleAlgebra) applyFormalBranchRelationFactorResult(base formalRelationTuple, result formalBranchRelationFactorResult) (formalRelationTuple, error) {
	_, directory, _, ok := a.span(base.variable)
	if !ok || base.root.owner != directory {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	writes := append([]formalFiberWrite(nil), result.writes...)
	priorCare, err := directory.valueAt(base.root, 0)
	if err != nil {
		return formalRelationTuple{}, err
	}
	if priorCare != formalFiberValue(result.care) {
		writes = append(writes, formalFiberWrite{ordinal: 0, value: formalFiberValue(result.care)})
	}
	if len(writes) == 0 {
		return base, nil
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		return formalRelationTuple{}, err
	}
	root, _, err := directory.applyDelta(base.root, delta)
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: base.variable, root: root}), nil
}

func (a *formalTupleAlgebra) diffFormalBranchRelationGroup(
	base formalRelationTuple,
	result formalRelationTuple,
	factors []int,
	plans []formalBranchRelationFactorPlan,
) ([]formalFiberWrite, error) {
	_, directory, _, ok := a.span(base.variable)
	if !ok || base.root.owner != directory || (!result.bottom() && (result.variable != base.variable || result.root.owner != directory)) {
		return nil, errFormalComponentForeignOwner
	}
	ordinals := make([]formalFiberOrdinal, 0)
	for _, factor := range factors {
		if factor < 0 || factor >= len(plans) {
			return nil, errFormalComponentMalformed
		}
		for _, ordinal := range plans[factor].writeOrdinals {
			if ordinal != 0 {
				ordinals = append(ordinals, ordinal)
			}
		}
	}
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	unique := ordinals[:0]
	for _, ordinal := range ordinals {
		if len(unique) == 0 || unique[len(unique)-1] != ordinal {
			unique = append(unique, ordinal)
		}
	}
	writes := make([]formalFiberWrite, 0, len(unique))
	for _, ordinal := range unique {
		before, err := directory.valueAt(base.root, ordinal)
		if err != nil {
			return nil, err
		}
		after := before
		if !result.bottom() {
			after, err = directory.valueAt(result.root, ordinal)
			if err != nil {
				return nil, err
			}
		}
		if before != after {
			writes = append(writes, formalFiberWrite{ordinal: ordinal, value: after})
		}
	}
	return writes, nil
}

type formalBranchRelationFactorResult struct {
	care   decisionRef
	writes []formalFiberWrite
}

type formalBranchRelationLeafWrite struct {
	ordinal formalFiberOrdinal
	leaf    decisionLeaf
}

// sparseFormalBranchRelationLeafWrites is the publication boundary between a
// factor's complete semantic image and its physical DD update. Reconcile
// still materializes the complete family before this point; unchanged
// siblings are then represented by the predecessor root instead of being
// rebuilt through an equivalent ITE chain. Patch uses the same law for its
// exact owned fibers.
func sparseFormalBranchRelationLeafWrites(
	writes []formalBranchRelationLeafWrite,
	current, next formalSparseLeafView,
	ordinals []formalFiberOrdinal,
) ([]formalBranchRelationLeafWrite, error) {
	for _, ordinal := range ordinals {
		if ordinal == 0 {
			continue
		}
		prior, priorPresent := current.leaf(ordinal)
		leaf, nextPresent := next.leaf(ordinal)
		if !priorPresent || !nextPresent {
			return nil, errFormalComponentMalformed
		}
		if prior != leaf {
			writes = append(writes, formalBranchRelationLeafWrite{ordinal: ordinal, leaf: leaf})
		}
	}
	return writes, nil
}

func (a *formalTupleAlgebra) applyFormalBranchRelationFactor(
	original formalRelationTuple,
	current formalRelationTuple,
	step *formalBranchRelationsStep,
	factorIndex int,
) (result formalBranchRelationFactorResult, resultErr error) {
	if a == nil || step == nil || factorIndex < 0 || factorIndex >= len(step.plans) {
		return formalBranchRelationFactorResult{}, errFormalComponentForeignOwner
	}
	plan := step.plans[factorIndex]
	var trace *formalBranchRelationEvalTraceFactor
	var totalMark formalRelationEvalTracePhaseMark
	if a.evalTrace != nil && a.evalTrace.active != nil && factorIndex < len(a.evalTrace.active.branchRelationFactors) {
		trace = &a.evalTrace.active.branchRelationFactors[factorIndex]
		totalMark = beginFormalRelationEvalTracePhase(a)
		_, directory, _, ok := a.span(current.variable)
		if ok {
			roots := func(tuple formalRelationTuple, ordinals []formalFiberOrdinal) []decisionRef {
				out := make([]decisionRef, 0, len(ordinals))
				for _, ordinal := range ordinals {
					if value, err := directory.valueAt(tuple.root, ordinal); err == nil {
						out = append(out, decisionRef(value))
					}
				}
				return out
			}
			trace.currentSupport = formalRelationTraceSupportRanks(&a.decisions, roots(current, plan.currentProjectionOrdinals)...)
			trace.originalSupport = formalRelationTraceSupportRanks(&a.decisions, roots(original, plan.originalReadOrdinals)...)
		}
		defer func() { finishFormalRelationEvalTracePhase(a, &trace.total, totalMark) }()
	}
	if plan.consequence != nil {
		return a.applyFormalBranchConsequenceFactor(current, plan.consequence, trace)
	}
	projections := make([]formalSparseTupleProjection, 0, 2)
	if len(plan.originalReadOrdinals) != 0 {
		projections = append(projections, formalSparseTupleProjection{tuple: original, ordinals: plan.originalReadOrdinals})
	}
	projections = append(projections, formalSparseTupleProjection{tuple: current, ordinals: plan.currentProjectionOrdinals})
	var partitionMark formalRelationEvalTracePhaseMark
	if trace != nil {
		partitionMark = beginFormalRelationEvalTracePhase(a)
	}
	regions, err := a.partitionSparseLeafViewsUnderCare(projections, nil)
	if trace != nil {
		finishFormalRelationEvalTracePhase(a, &trace.partition, partitionMark)
		trace.regions = len(regions)
	}
	if err != nil {
		return formalBranchRelationFactorResult{}, err
	}
	span, directory, authority, ok := a.span(current.variable)
	if !ok {
		return formalBranchRelationFactorResult{}, errFormalComponentForeignOwner
	}
	care, err := a.care(current)
	if err != nil {
		return formalBranchRelationFactorResult{}, err
	}
	affected := make([]formalBranchRelationAffectedRoot, 0, len(plan.writeOrdinals))
	for _, ordinal := range plan.writeOrdinals {
		if ordinal == 0 {
			continue
		}
		root, readErr := directory.valueAt(current.root, ordinal)
		if readErr != nil {
			return formalBranchRelationFactorResult{}, readErr
		}
		affected = append(affected, formalBranchRelationAffectedRoot{ordinal: ordinal, root: decisionRef(root)})
	}
	edge := step.edge
	edge.Context = a.ctx
	regionWrites := make([]formalBranchRelationLeafWrite, 0, len(plan.writeOrdinals))
	var writeErr error
	for _, region := range regions {
		leafStarted := time.Time{}
		leafApplyOps := uint64(0)
		if trace != nil {
			leafStarted = time.Now()
			leafApplyOps = a.decisions.applyOps
		}
		currentIndex := 0
		var originalEvaluator formalSparseLeafView
		if len(plan.originalReadOrdinals) != 0 {
			if len(region.views) != 2 {
				return formalBranchRelationFactorResult{}, errDecisionMalformed
			}
			originalEvaluator = region.views[0]
			currentIndex = 1
		} else {
			if len(region.views) != 1 {
				return formalBranchRelationFactorResult{}, errDecisionMalformed
			}
			currentView := region.views[0]
			originalEvaluator = formalSparseLeafView{
				algebra: a, variable: currentView.variable, span: currentView.span, authority: currentView.authority,
				body: currentView.body, guard: region.guard,
			}
		}
		currentEvaluator := region.views[currentIndex]
		originalFrame, bindErr := a.bindFormalBranchRelationFrame(step.factors, factorIndex, factapply.BranchRelationFactorOriginal, originalEvaluator, plan.original)
		if bindErr != nil {
			return formalBranchRelationFactorResult{}, fmt.Errorf("original frame: %w", bindErr)
		}
		currentFrame, bindErr := a.bindFormalBranchRelationFrame(step.factors, factorIndex, factapply.BranchRelationFactorCurrent, currentEvaluator, plan.current)
		if bindErr != nil {
			return formalBranchRelationFactorResult{}, fmt.Errorf("current frame: %w", bindErr)
		}
		patch, canceled, applyErr := step.factors.ApplyFactorFrames(factorIndex, edge, originalFrame, currentFrame)
		if applyErr != nil {
			return formalBranchRelationFactorResult{}, fmt.Errorf("source %d: %w", step.factors.FactorSource(factorIndex), applyErr)
		}
		if canceled {
			return formalBranchRelationFactorResult{}, a.ctx.Err()
		}
		if !patch.Reachable() {
			care, err = a.decisions.condition(a.ctx, region.guard, decisionFalse, care)
			if err != nil {
				return formalBranchRelationFactorResult{}, err
			}
			continue
		}
		nextEvaluator := currentEvaluator
		nextEvaluator.leaves = append([]decisionLeaf(nil), currentEvaluator.leaves...)
		if publishErr := a.publishFormalBranchRelationPatch(step.factors, factorIndex, currentEvaluator, plan.current, patch, &nextEvaluator); publishErr != nil {
			return formalBranchRelationFactorResult{}, fmt.Errorf("publication: %w", publishErr)
		}
		regionWrites, writeErr = sparseFormalBranchRelationLeafWrites(regionWrites[:0], currentEvaluator, nextEvaluator, plan.writeOrdinals)
		if writeErr != nil {
			return formalBranchRelationFactorResult{}, writeErr
		}
		if trace != nil {
			trace.leafTime += time.Since(leafStarted)
			trace.leafApplyOps += a.decisions.applyOps - leafApplyOps
			trace.leafWrites += len(regionWrites)
		}
		for _, write := range regionWrites {
			index := sort.Search(len(affected), func(index int) bool { return affected[index].ordinal >= write.ordinal })
			if index >= len(affected) || affected[index].ordinal != write.ordinal {
				return formalBranchRelationFactorResult{}, errFormalComponentMalformed
			}
			affected[index].root, err = a.decisions.condition(a.ctx, region.guard, a.decisions.terminal(write.leaf), affected[index].root)
			if err != nil {
				return formalBranchRelationFactorResult{}, err
			}
		}
	}
	writes, err := a.sealFormalBranchRelationAffectedRoots(current, span, authority, affected)
	return formalBranchRelationFactorResult{care: care, writes: writes}, err
}

type formalBranchRelationAffectedRoot struct {
	ordinal formalFiberOrdinal
	root    decisionRef
}

func (a *formalTupleAlgebra) sealFormalBranchRelationAffectedRoots(
	base formalRelationTuple,
	span formalFiberDescriptorSpan,
	authority *formalComponentTerminalAuthority,
	affected []formalBranchRelationAffectedRoot,
) ([]formalFiberWrite, error) {
	_, directory, _, ok := a.span(base.variable)
	if !ok {
		return nil, errFormalComponentForeignOwner
	}
	writes := make([]formalFiberWrite, 0, len(affected))
	for _, candidate := range affected {
		descriptor := span.forest.descriptors[span.first+int(candidate.ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, candidate.root); err != nil {
			return nil, fmt.Errorf("output fiber %d (role %d): %w", candidate.ordinal, descriptor.role, err)
		}
		prior, err := directory.valueAt(base.root, candidate.ordinal)
		if err != nil {
			return nil, err
		}
		if prior != formalFiberValue(candidate.root) {
			writes = append(writes, formalFiberWrite{ordinal: candidate.ordinal, value: formalFiberValue(candidate.root)})
		}
	}
	return writes, nil
}

func (a *formalTupleAlgebra) applyFormalBranchConsequenceFactor(current formalRelationTuple, plan *formalPresenceImplicationStep, trace *formalBranchRelationEvalTraceFactor) (formalBranchRelationFactorResult, error) {
	var partitionMark formalRelationEvalTracePhaseMark
	if trace != nil {
		partitionMark = beginFormalRelationEvalTracePhase(a)
	}
	regions, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{tuple: current, ordinals: plan.projectionOrdinals}}, plan.demands)
	if trace != nil {
		finishFormalRelationEvalTracePhase(a, &trace.partition, partitionMark)
		trace.regions = len(regions)
	}
	if err != nil {
		return formalBranchRelationFactorResult{}, err
	}
	span, directory, authority, ok := a.span(current.variable)
	if !ok {
		return formalBranchRelationFactorResult{}, errFormalComponentForeignOwner
	}
	care, err := a.care(current)
	if err != nil {
		return formalBranchRelationFactorResult{}, err
	}
	affected := make([]formalBranchRelationAffectedRoot, 0, len(plan.writeOrdinals))
	for _, ordinal := range plan.writeOrdinals {
		if ordinal == 0 {
			continue
		}
		root, readErr := directory.valueAt(current.root, ordinal)
		if readErr != nil {
			return formalBranchRelationFactorResult{}, readErr
		}
		affected = append(affected, formalBranchRelationAffectedRoot{ordinal: ordinal, root: decisionRef(root)})
	}
	for _, region := range regions {
		leafStarted := time.Time{}
		leafApplyOps := uint64(0)
		if trace != nil {
			leafStarted = time.Now()
			leafApplyOps = a.decisions.applyOps
		}
		if len(region.views) != 1 {
			return formalBranchRelationFactorResult{}, errDecisionMalformed
		}
		leaves, reachable, leafErr := a.applyFormalPresenceImplicationsLeaf(region.views[0], plan)
		if leafErr != nil {
			return formalBranchRelationFactorResult{}, leafErr
		}
		if trace != nil {
			trace.leafTime += time.Since(leafStarted)
			trace.leafApplyOps += a.decisions.applyOps - leafApplyOps
			trace.leafWrites += len(leaves)
		}
		if !reachable {
			care, err = a.decisions.condition(a.ctx, region.guard, decisionFalse, care)
			if err != nil {
				return formalBranchRelationFactorResult{}, err
			}
			continue
		}
		for index, ordinal := range plan.writeOrdinals {
			if ordinal == 0 {
				continue
			}
			position := sort.Search(len(affected), func(i int) bool { return affected[i].ordinal >= ordinal })
			if position >= len(affected) || affected[position].ordinal != ordinal || index >= len(leaves) {
				return formalBranchRelationFactorResult{}, errFormalComponentMalformed
			}
			affected[position].root, err = a.decisions.condition(a.ctx, region.guard, a.decisions.terminal(leaves[index]), affected[position].root)
			if err != nil {
				return formalBranchRelationFactorResult{}, err
			}
		}
	}
	writes, err := a.sealFormalBranchRelationAffectedRoots(current, span, authority, affected)
	return formalBranchRelationFactorResult{care: care, writes: writes}, err
}

func (a *formalTupleAlgebra) cacheFormalCoordinateGroupTuple(tuple formalRelationTuple, group formalFiberGroupDescriptor) error {
	regions, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{tuple: tuple, ordinals: group.members}}, nil)
	if err != nil {
		return err
	}
	for _, region := range regions {
		if len(region.views) != 1 {
			return errDecisionMalformed
		}
		if err := a.cacheFormalSparseCoordinateGroup(region.views[0], group); err != nil {
			return err
		}
	}
	return nil
}

// cacheFormalSparseCoordinateGroup commits one physical lane spelling after
// all independent family patches in a dependency stage have accumulated. It
// is a no-op for an already interned spelling, so fixed-point re-execution
// does not rebuild the lane.
func (a *formalTupleAlgebra) cacheFormalSparseCoordinateGroup(view formalSparseLeafView, group formalFiberGroupDescriptor) error {
	if a == nil || view.authority == nil || group.kind != formalFiberGroupCoordinateLane {
		return errFormalComponentForeignOwner
	}
	leaves := make([]decisionLeaf, len(group.members))
	for index, ordinal := range group.members {
		leaf, present := view.leaf(ordinal)
		if !present {
			return errFormalComponentMalformed
		}
		leaves[index] = leaf
	}
	key := formalFactorReachabilityKey{body: view.authority.body, lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
	for _, entry := range a.factorReachability[key] {
		if formalFactorLeavesEqual(entry.leaves, leaves) {
			return nil
		}
	}
	factor, err := a.materializeCoordinateGroup(view.authority, view.span, group, leaves)
	if err != nil {
		return err
	}
	return a.cacheFormalFactorReachability(view.authority, group, leaves, factor)
}

func (a *formalTupleAlgebra) bindFormalBranchRelationFrame(factors factapply.BranchRelationFactors, index int, role factapply.BranchRelationFactorRole, evaluator formalSparseLeafView, plan formalBranchRelationRolePlan) (factapply.BranchRelationFactorFrame, error) {
	values := make([]product.Value, len(plan.values))
	valuesTop := false
	bottom := product.Bottom(evaluator.authority.product.Registry())
	if len(plan.values) != 0 {
		leaf, present := evaluator.leaf(formalFiberOrdinal(plan.valueTop))
		if !present || leaf > 1 {
			return factapply.BranchRelationFactorFrame{}, errFormalComponentMalformed
		}
		valuesTop = leaf == 1
	}
	for index, valuePlan := range plan.values {
		values[index] = bottom
		if valuesTop {
			values[index] = product.Top()
			continue
		}
		leaf, present := evaluator.leaf(formalFiberOrdinal(valuePlan.position))
		if !present {
			return factapply.BranchRelationFactorFrame{}, errFormalComponentMalformed
		}
		if leaf != 0 {
			terminal, terminalErr := evaluator.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentGroundValue {
				return factapply.BranchRelationFactorFrame{}, errFormalComponentMalformed
			}
			values[index] = terminal.ground
		}
	}
	var err error
	lanes := make([]state.LaneFactor, len(plan.lanes))
	for index, group := range plan.lanes {
		lanes[index], err = evaluator.laneFactor(group)
		if err != nil {
			return factapply.BranchRelationFactorFrame{}, err
		}
	}
	coordinates := make([]factapply.BranchRelationCoordinateOperands, len(plan.coordinates))
	for index := range coordinates {
		coordinate := plan.coordinates[index]
		coordinates[index], err = a.materializeFormalBranchCoordinateOperandsSparse(evaluator, coordinate)
		if err != nil {
			return factapply.BranchRelationFactorFrame{}, err
		}
	}
	return factors.BindFactorFrame(index, role, factapply.BranchRelationFactorOperands{
		Values: values, ValuesTop: valuesTop, Lanes: lanes, Coordinates: coordinates, Reachable: true,
	})
}

func (a *formalTupleAlgebra) materializeFormalBranchCoordinateOperandsSparse(evaluator formalSparseLeafView, plan formalBranchRelationCoordinatePlan) (factapply.BranchRelationCoordinateOperands, error) {
	skeleton, err := a.materializeFormalBranchCoordinateSkeletonSparse(evaluator, plan)
	if err != nil {
		return factapply.BranchRelationCoordinateOperands{}, err
	}
	scalars := make([]state.CoordinateScalarFactor, len(plan.slots))
	for index, slot := range plan.slots {
		position := plan.positions[index]
		if position < 0 || position >= len(plan.family.scalarPositions) {
			return factapply.BranchRelationCoordinateOperands{}, errFormalComponentMalformed
		}
		leaf, present := evaluator.leaf(plan.family.scalars[position])
		if !present {
			return factapply.BranchRelationCoordinateOperands{}, errFormalComponentMalformed
		}
		if leaf == 0 {
			scalars[index], err = evaluator.authority.product.CoordinateDefault(skeleton, slot)
		} else {
			terminal, terminalErr := evaluator.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
				return factapply.BranchRelationCoordinateOperands{}, errFormalComponentMalformed
			}
			scalars[index] = terminal.scalar
		}
		if err != nil {
			return factapply.BranchRelationCoordinateOperands{}, err
		}
	}
	return factapply.BranchRelationCoordinateOperands{Skeleton: skeleton, Scalars: scalars}, nil
}

func (a *formalTupleAlgebra) materializeFormalBranchCoordinateOperands(evaluator formalTupleLeafEvaluator, plan formalBranchRelationCoordinatePlan) (factapply.BranchRelationCoordinateOperands, error) {
	ordinals := []formalFiberOrdinal{plan.family.skeleton}
	for _, position := range plan.positions {
		if position < 0 || position >= len(plan.family.scalars) {
			return factapply.BranchRelationCoordinateOperands{}, errFormalComponentMalformed
		}
		ordinals = append(ordinals, plan.family.scalars[position])
	}
	ordinals, err := sealFormalPresenceOrdinals(evaluator.span, ordinals)
	if err != nil {
		return factapply.BranchRelationCoordinateOperands{}, err
	}
	leaves := make([]decisionLeaf, len(ordinals))
	for index, ordinal := range ordinals {
		leaf, present := evaluator.leaves.leaf(ordinal)
		if !present {
			return factapply.BranchRelationCoordinateOperands{}, errFormalComponentMalformed
		}
		leaves[index] = leaf
	}
	positions, err := sealFormalOrdinalPositions(evaluator.span.count, ordinals)
	if err != nil {
		return factapply.BranchRelationCoordinateOperands{}, err
	}
	return a.materializeFormalBranchCoordinateOperandsSparse(formalSparseLeafView{
		algebra: a, variable: evaluator.variable, span: evaluator.span, authority: evaluator.authority,
		body: evaluator.body, guard: evaluator.guard, ordinals: ordinals, positions: positions, leaves: leaves,
	}, plan)
}

func (a *formalTupleAlgebra) materializeFormalBranchCoordinateSkeletonSparse(evaluator formalSparseLeafView, plan formalBranchRelationCoordinatePlan) (state.CoordinateFamilySkeleton, error) {
	leaf, present := evaluator.leaf(plan.family.skeleton)
	if !present {
		return state.CoordinateFamilySkeleton{}, errFormalComponentMalformed
	}
	if leaf == 0 {
		return evaluator.authority.product.CoordinateSkeletonBottom(plan.family.family, evaluator.span.keys)
	}
	terminal, err := evaluator.authority.terminal(leaf)
	if err != nil || terminal.kind != formalComponentCoordinateSkeleton || !coordinateFamilySame(terminal.skeleton.Family(), plan.family.family) {
		return state.CoordinateFamilySkeleton{}, errFormalComponentMalformed
	}
	return terminal.skeleton, nil
}

func (a *formalTupleAlgebra) publishFormalBranchRelationPatch(factors factapply.BranchRelationFactors, factorIndex int, evaluator formalSparseLeafView, plan formalBranchRelationRolePlan, patch factapply.BranchRelationFactorPatch, next *formalSparseLeafView) error {
	values := patch.Values()
	lanes := patch.Lanes()
	coordinates := patch.Coordinates()
	if len(values) != 0 && len(values) != len(plan.values) || len(lanes) != 0 && len(lanes) != len(plan.lanes) ||
		len(coordinates) != 0 && len(coordinates) != len(plan.coordinates) {
		return fmt.Errorf("transformer: formal BranchRelations patch differs from declared carrier layout")
	}
	if len(values) != 0 {
		if next == nil {
			return errFormalComponentMalformed
		}
		if patch.ValuesTop() {
			if !next.setLeaf(formalFiberOrdinal(plan.valueTop), 1) {
				return errFormalComponentMalformed
			}
		} else {
			if !next.setLeaf(formalFiberOrdinal(plan.valueTop), 0) {
				return errFormalComponentMalformed
			}
			for index, value := range values {
				ordinal := formalFiberOrdinal(plan.values[index].position)
				if product.Equal(evaluator.authority.product.Registry(), value, product.Bottom(evaluator.authority.product.Registry())) {
					if !next.setLeaf(ordinal, 0) {
						return errFormalComponentMalformed
					}
					continue
				}
				leaf, err := evaluator.authority.internGroundValue(value)
				if err != nil {
					return err
				}
				if !next.setLeaf(ordinal, leaf) {
					return errFormalComponentMalformed
				}
			}
		}
	}
	for _, index := range plan.laneWrites {
		if index < 0 || index >= len(lanes) || index >= len(plan.lanes) {
			return errFormalComponentMalformed
		}
		current, err := evaluator.laneFactor(plan.lanes[index])
		if err != nil {
			return fmt.Errorf("BranchRelations lane %d materialization: %w", index, err)
		}
		same, err := evaluator.authority.product.LaneCanonicalRepresentationEqual(current, lanes[index])
		if err != nil {
			return err
		}
		if same {
			continue
		}
		factorLeaves, err := a.factorFormalSparseLane(evaluator.authority, evaluator.span, plan.lanes[index], lanes[index])
		if err != nil {
			return fmt.Errorf("transformer: BranchRelations lane publication %d: %w", index, err)
		}
		for memberIndex, ordinal := range plan.lanes[index].members {
			if !next.setLeaf(ordinal, factorLeaves[memberIndex]) {
				return errFormalComponentMalformed
			}
		}
	}
	for index, coordinate := range coordinates {
		if err := a.publishFormalBranchCoordinatePatch(factors, factorIndex, index, evaluator, plan.coordinates[index], coordinate, next); err != nil {
			return fmt.Errorf("BranchRelations coordinate %d publication: %w", index, err)
		}
	}
	return nil
}

func (a *formalTupleAlgebra) publishFormalBranchCoordinatePatch(factors factapply.BranchRelationFactors, factorIndex, coordinateIndex int, evaluator formalSparseLeafView, plan formalBranchRelationCoordinatePlan, patch factapply.BranchRelationCoordinateOperands, next *formalSparseLeafView) error {
	if next == nil || !coordinateFamilySame(patch.Skeleton.Family(), plan.family.family) || len(patch.Scalars) != len(plan.slots) {
		return errFormalComponentMalformed
	}
	for index, slot := range plan.slots {
		position := plan.positions[index]
		if position < 0 || position >= len(plan.family.scalarPositions) {
			return errFormalComponentMalformed
		}
		scalar := patch.Scalars[index]
		equal, equalErr := evaluator.authority.product.CoordinateSlotEqual(scalar.Slot(), slot)
		if equalErr != nil || !equal {
			return errFormalComponentMalformed
		}
	}
	if !plan.writes {
		return nil
	}
	// Same-stage factors are independent at their declared coordinates, but
	// several such coordinates may share one physical LaneFactor. The formal
	// carrier owns family fibers, so patch the accumulating family spelling
	// directly: materializing the whole physical lane would copy and refactor
	// every sibling family for a write whose authority is family-exclusive.
	selected := make([]formalPresenceCoordinateSlot, len(plan.positions))
	for index, position := range plan.positions {
		if position < 0 || position >= len(plan.family.scalars) {
			return errFormalComponentMalformed
		}
		selected[index] = formalPresenceCoordinateSlot{slot: plan.slots[index], ordinal: plan.family.scalars[position]}
	}
	bindings := selected
	if plan.publication == factapply.BranchRelationCoordinatePublicationReconcile {
		bindings = nil
	}
	base, err := a.materializeFormalCoordinateFamily(*next, plan.family, bindings)
	if err != nil {
		return fmt.Errorf("coordinate base family materialization: %w", err)
	}
	patched, err := factors.ApplyFactorCoordinateFamilyPatch(factorIndex, coordinateIndex, base, patch)
	if err != nil {
		return err
	}
	complete, err := a.factorFormalCoordinateFamily(evaluator.authority, evaluator.span, plan.family, patched)
	if err != nil {
		return fmt.Errorf("coordinate patched family factoring: %w", err)
	}
	for _, write := range complete {
		if plan.publication == factapply.BranchRelationCoordinatePublicationPatch {
			owned := false
			for _, position := range plan.writePositions {
				owned = owned || write.ordinal == plan.family.scalars[position]
			}
			if !owned {
				continue
			}
		}
		if !next.setLeaf(write.ordinal, write.leaf) {
			return errFormalComponentMalformed
		}
	}
	return nil
}
