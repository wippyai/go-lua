package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

type formalFactorReachabilityKey struct {
	body *formalComponentTerminalBody
	lane state.LaneOrdinal
	hash uint64
}

type formalFactorReachabilityEntry struct {
	leaves  []decisionLeaf
	factor  state.LaneFactor
	program state.BoundaryReachabilityProgram
}

type formalFactorExecutionCapabilityKey struct {
	body     *formalComponentTerminalBody
	hash     uint64
	selector formalApplyCoordinateSelectorRef
}

type formalFactorExecutionCapabilityEntry struct {
	leaves     []decisionLeaf
	capability formalFactorExecutionCapability
}

// formalFactorExecutionCapability is the single immutable authority attached
// to a published complete product spelling. Reachability and identity support
// are observed together at the producer boundary, so Apply has exactly one
// dense-vector lookup and cannot drift into parallel discovery paths.
type formalFactorExecutionCapability struct {
	reachability state.BoundaryReachabilityProgramSet
	identities   state.IdentitySubstitutionSupport
}

// formalProductLeafEvaluator is the compact producer-side view used to freeze
// execution capability. Its leaves are Values followed by residual factors in
// the immutable layout order; no Middle/Outcome descriptor storage exists.
type formalProductLeafEvaluator struct {
	algebra   *formalTupleAlgebra
	authority *formalComponentTerminalAuthority
	span      formalFiberDescriptorSpan
	layout    *formalFactorExecutionLayout
	leaves    []decisionLeaf
}

func (e formalProductLeafEvaluator) productFactors() (state.ValueFactor[FormalSlot], []state.LaneFactor, error) {
	if e.algebra == nil || e.authority == nil || e.layout == nil || len(e.leaves) != len(e.layout.members) {
		return state.ValueFactor[FormalSlot]{}, nil, fmt.Errorf("transformer: outcome product vector ownership: %w", errFormalComponentForeignOwner)
	}
	valuesWidth := len(e.layout.values.members)
	values, err := e.algebra.materializeValuesGroup(e.authority, e.layout.values, e.leaves[:valuesWidth])
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, nil, fmt.Errorf("transformer: outcome Values materialization: %w", err)
	}
	factors := make([]state.LaneFactor, len(e.layout.nonValues))
	for index, group := range e.layout.nonValues {
		start := e.layout.offsets[index]
		end := start + len(group.members)
		switch group.kind {
		case formalFiberGroupOrdinaryLane:
			factors[index], err = e.algebra.materializeOrdinaryGroup(e.authority, group, e.leaves[start:end])
		case formalFiberGroupCoordinateLane:
			factors[index], err = e.algebra.materializeCoordinateGroup(e.authority, e.span, group, e.leaves[start:end])
		default:
			err = errFormalComponentMalformed
		}
		if err != nil {
			return state.ValueFactor[FormalSlot]{}, nil, fmt.Errorf("transformer: outcome lane %s materialization: %w", group.lane.ID(), err)
		}
	}
	return values, factors, nil
}

// prefreezeFormalBottomReachability admits the physical-zero spelling used by
// untouched directory fibers. Bottom never passes through a factor interner,
// so its per-lane programs and complete vector set must be frozen when the run
// arena is built rather than synthesized by a later consumer.
func (a *formalTupleAlgebra) prefreezeFormalBottomReachability() error {
	if a == nil || a.program == nil || a.components == nil {
		return errFormalComponentForeignOwner
	}
	for bodyIndex, span := range a.program.formalFibers.spans {
		if bodyIndex >= len(a.components.authorities) {
			return errFormalComponentForeignOwner
		}
		authority := a.components.authorities[bodyIndex]
		leaves := make([]decisionLeaf, span.count)
		if span.count == 0 {
			return errFormalComponentMalformed
		}
		// Care's exact-true terminal is the only nonzero member of the canonical
		// live full-product Bottom row.
		careFound := false
		for ordinal, descriptor := range span.descriptors() {
			if descriptor.role == formalFiberCare {
				leaves[ordinal] = decisionLeaf(decisionTrue)
				careFound = true
			}
		}
		if !careFound {
			return errFormalComponentMalformed
		}
		layout := &authority.body.factors
		for _, group := range layout.nonValues {
			bottom, err := authority.product.LaneBottom(group.lane)
			if err != nil {
				return err
			}
			groupLeaves, err := formalEffectGroupLeaves(group, leaves)
			if err != nil {
				return err
			}
			if err := a.cacheFormalFactorReachability(authority, group, groupLeaves, bottom); err != nil {
				return err
			}
		}
		if !layout.validFor(authority.product, span.variable) {
			return errFormalComponentMalformed
		}
		evaluator, err := a.newTupleLeafEvaluator(span.variable, leaves, decisionTrue)
		if err != nil {
			return err
		}
		for _, selector := range a.applyCoordinateSelectorRefs(span.variable) {
			if _, err := a.internSelectedFormalFactorExecutionCapabilityRef(evaluator, selector); err != nil {
				return err
			}
		}
	}
	return nil
}

func formalFactorLeafHash(leaves []decisionLeaf) uint64 {
	hash := uint64(0x9e3779b97f4a7c15)
	for index, leaf := range leaves {
		hash = formalComponentMix(hash, uint64(index+1))
		hash = formalComponentMix(hash, uint64(leaf))
	}
	return hash
}

func formalFactorLeavesEqual(left, right []decisionLeaf) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// cacheFormalFactorReachability compiles the declarative closure program at
// the factor→terminal boundary, where the complete factor already exists for
// lattice publication. This cost is therefore paid once per interned factor
// spelling, never by Apply and never once per correlated DD leaf.
func (a *formalTupleAlgebra) cacheFormalFactorReachability(
	authority *formalComponentTerminalAuthority,
	group formalFiberGroupDescriptor,
	leaves []decisionLeaf,
	factor state.LaneFactor,
) error {
	if a == nil || authority == nil || authority.body == nil || authority.coordinateKeys == nil ||
		!authority.coordinateKeys.Valid() || !group.valid() || group.kind == formalFiberGroupValues ||
		factor.Lane() != group.lane || len(leaves) != len(group.members) {
		return errFormalComponentForeignOwner
	}
	key := formalFactorReachabilityKey{body: authority.body, lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
	for _, entry := range a.factorReachability[key] {
		if formalFactorLeavesEqual(entry.leaves, leaves) {
			same, err := authority.product.LaneCanonicalRepresentationEqual(entry.factor, factor)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("transformer: factor %q exact leaf spelling has conflicting lane representations", group.lane.ID())
			}
			return nil
		}
	}
	program, err := authority.product.PrepareBoundaryFactorReachability(authority.coordinateKeys, factor)
	if err != nil {
		return fmt.Errorf("transformer: factor %q reachability freeze: %w", group.lane.ID(), err)
	}
	a.factorReachability[key] = append(a.factorReachability[key], formalFactorReachabilityEntry{
		leaves: append([]decisionLeaf(nil), leaves...), factor: factor, program: program,
	})
	return nil
}

// formalFactorSpelling returns the exact LaneFactor retained when a producer
// factored this physical leaf vector. Published tuple leaves may only name
// such producer-owned spellings; a miss is a broken publication seam, never
// permission to reconstruct a second representation in a consumer.
func (a *formalTupleAlgebra) formalFactorSpelling(
	authority *formalComponentTerminalAuthority,
	group formalFiberGroupDescriptor,
	leaves []decisionLeaf,
) (state.LaneFactor, error) {
	if a == nil || authority == nil || authority.body == nil || !group.valid() || group.kind == formalFiberGroupValues ||
		len(leaves) != len(group.members) {
		return state.LaneFactor{}, errFormalComponentForeignOwner
	}
	key := formalFactorReachabilityKey{body: authority.body, lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
	for _, entry := range a.factorReachability[key] {
		if formalFactorLeavesEqual(entry.leaves, leaves) {
			if entry.factor.Lane() != group.lane {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			return entry.factor, nil
		}
	}
	return state.LaneFactor{}, fmt.Errorf("transformer: factor %q exact leaf spelling has no producer entry", group.lane.ID())
}

// formalSelectedFactorSpelling is the ordinal-addressed equivalent of
// the former dense-only lookup. It preserves allocation-free dense lookup and
// also fails closed when a sparse transaction omitted any requested member.
func (a *formalTupleAlgebra) formalSelectedFactorSpelling(
	authority *formalComponentTerminalAuthority,
	group formalFiberGroupDescriptor,
	selection formalFiberLeafSelection,
) (state.LaneFactor, error) {
	if a == nil || authority == nil || authority.body == nil || !group.valid() || group.kind == formalFiberGroupValues {
		return state.LaneFactor{}, errFormalComponentForeignOwner
	}
	hash := uint64(0x9e3779b97f4a7c15)
	for index, ordinal := range group.members {
		leaf, present := selection.leaf(ordinal)
		if !present {
			return state.LaneFactor{}, errFormalComponentMalformed
		}
		hash = formalComponentMix(hash, uint64(index+1))
		hash = formalComponentMix(hash, uint64(leaf))
	}
	key := formalFactorReachabilityKey{body: authority.body, lane: group.lane.Ordinal(), hash: hash}
	for _, entry := range a.factorReachability[key] {
		if len(entry.leaves) != len(group.members) {
			continue
		}
		equal := true
		for index, ordinal := range group.members {
			leaf, present := selection.leaf(ordinal)
			if !present || entry.leaves[index] != leaf {
				equal = false
				break
			}
		}
		if !equal {
			continue
		}
		if entry.factor.Lane() != group.lane {
			return state.LaneFactor{}, errFormalComponentMalformed
		}
		return entry.factor, nil
	}
	return state.LaneFactor{}, fmt.Errorf("transformer: factor %q exact leaf spelling has no producer entry", group.lane.ID())
}

func (a *formalTupleAlgebra) formalFactorReachabilityProgram(
	authority *formalComponentTerminalAuthority,
	group formalFiberGroupDescriptor,
	leaves []decisionLeaf,
) (state.BoundaryReachabilityProgram, error) {
	if a == nil || authority == nil || authority.body == nil || !group.valid() || group.kind == formalFiberGroupValues ||
		len(leaves) != len(group.members) {
		return state.BoundaryReachabilityProgram{}, errFormalComponentForeignOwner
	}
	key := formalFactorReachabilityKey{body: authority.body, lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
	for _, entry := range a.factorReachability[key] {
		if formalFactorLeavesEqual(entry.leaves, leaves) {
			return entry.program, nil
		}
	}
	return state.BoundaryReachabilityProgram{}, fmt.Errorf("transformer: factor %q has no prefrozen reachability program", group.lane.ID())
}

func (e formalTupleLeafEvaluator) factorReachabilityPrograms() ([]state.BoundaryReachabilityProgram, error) {
	if e.algebra == nil || e.variable == 0 || e.authority == nil || e.span.variable != e.variable ||
		!e.leaves.valid(e.span) {
		return nil, errFormalComponentForeignOwner
	}
	programs := make([]state.BoundaryReachabilityProgram, len(e.layout.nonValues))
	for index, group := range e.layout.nonValues {
		leaves, err := e.leaves.group(group)
		if err != nil {
			return nil, err
		}
		program, err := e.algebra.formalFactorReachabilityProgram(e.authority, group, leaves)
		if err != nil {
			return nil, err
		}
		programs[index] = program
	}
	if len(programs) != e.authority.product.NonValuesLaneCount() {
		return nil, fmt.Errorf("transformer: formal factor reachability inventory is incomplete")
	}
	return programs, nil
}

func formalFactorExecutionVector(e formalTupleLeafEvaluator) ([]decisionLeaf, error) {
	if e.algebra == nil || e.authority == nil || e.span.variable != e.variable || !e.leaves.valid(e.span) {
		return nil, errFormalComponentForeignOwner
	}
	vector := make([]decisionLeaf, 0, len(e.layout.values.members)+e.layout.vectorWidth)
	values, err := e.leaves.group(e.layout.values)
	if err != nil {
		return nil, err
	}
	vector = append(vector, values...)
	for _, group := range e.layout.nonValues {
		leaves, err := e.leaves.group(group)
		if err != nil {
			return nil, err
		}
		vector = append(vector, leaves...)
	}
	return vector, nil
}

func (a *formalTupleAlgebra) applyCoordinateSelectorRefs(target relationVar) []formalApplyCoordinateSelectorRef {
	if a == nil || a.program == nil || a.program.formalFibers == nil || a.program.formalFibers.applySelectors == nil ||
		target == 0 || int(target) > len(a.program.bodies) {
		return nil
	}
	return a.program.formalFibers.applySelectors.references(target)
}

func (a *formalTupleAlgebra) internSelectedFormalFactorExecutionCapabilityRef(
	e formalTupleLeafEvaluator,
	selector formalApplyCoordinateSelectorRef,
) (formalFactorExecutionCapability, error) {
	vector, err := formalFactorExecutionVector(e)
	if err != nil {
		return formalFactorExecutionCapability{}, err
	}
	return a.internSelectedFormalProductExecutionCapabilityRef(formalProductLeafEvaluator{
		algebra: a, authority: e.authority, span: e.span, layout: e.layout, leaves: vector,
	}, selector)
}

func (a *formalTupleAlgebra) internSelectedFormalProductExecutionCapabilityRef(
	e formalProductLeafEvaluator,
	selectorRef formalApplyCoordinateSelectorRef,
) (formalFactorExecutionCapability, error) {
	if a == nil || e.algebra != a || e.authority == nil || e.layout == nil ||
		a.program == nil || a.program.formalFibers == nil || a.program.formalFibers.applySelectors == nil ||
		selectorRef.catalog != a.program.formalFibers.applySelectors || selectorRef.target != e.span.variable {
		return formalFactorExecutionCapability{}, fmt.Errorf("transformer: execution capability input ownership: %w", errFormalComponentForeignOwner)
	}
	selector, err := selectorRef.catalog.inventory(selectorRef)
	if err != nil {
		return formalFactorExecutionCapability{}, err
	}
	vector := e.leaves
	key := formalFactorExecutionCapabilityKey{body: e.authority.body, hash: formalFactorLeafHash(vector), selector: selectorRef}
	for _, entry := range a.factorExecutionCapabilities[key] {
		if formalFactorLeavesEqual(entry.leaves, vector) {
			return entry.capability, nil
		}
	}
	values, factors, err := e.productFactors()
	if err != nil {
		return formalFactorExecutionCapability{}, fmt.Errorf("transformer: execution capability product materialization: %w", err)
	}
	programs := make([]state.BoundaryReachabilityProgram, len(factors))
	for index, factor := range factors {
		families, familyErr := e.authority.product.CoordinateFamilies(factor.Lane())
		if familyErr != nil {
			return formalFactorExecutionCapability{}, fmt.Errorf("transformer: execution capability lane %s family inventory: %w", factor.Lane().ID(), familyErr)
		}
		if len(families) != 0 {
			factor, familyErr = e.authority.product.SelectCoordinateLaneFactor(factor, selector)
			if familyErr != nil {
				return formalFactorExecutionCapability{}, fmt.Errorf("transformer: execution capability lane %s selector: %w", factor.Lane().ID(), familyErr)
			}
			factors[index] = factor
		}
		programs[index], err = e.authority.product.PrepareBoundaryFactorReachability(e.authority.coordinateKeys, factor)
		if err != nil {
			return formalFactorExecutionCapability{}, fmt.Errorf("transformer: execution capability lane %s reachability: %w", factor.Lane().ID(), err)
		}
	}
	set, err := state.SealBoundaryReachabilityProgramSet(programs...)
	if err != nil {
		return formalFactorExecutionCapability{}, err
	}
	support, err := state.PrepareIdentitySubstitutionSupport(a.ctx, e.authority.product, values, factors)
	if err != nil {
		return formalFactorExecutionCapability{}, fmt.Errorf("transformer: execution capability identity support: %w", err)
	}
	capability := formalFactorExecutionCapability{reachability: set, identities: support}
	a.factorExecutionCapabilities[key] = append(a.factorExecutionCapabilities[key], formalFactorExecutionCapabilityEntry{
		leaves: append([]decisionLeaf(nil), vector...), capability: capability,
	})
	return capability, nil
}

func formalFactorExecutionVectorHash(e formalTupleLeafEvaluator) uint64 {
	hash := uint64(0x9e3779b97f4a7c15)
	index := 0
	for _, ordinal := range e.layout.values.members {
		leaf, present := e.leaves.leaf(ordinal)
		if !present {
			return 0
		}
		hash = formalComponentMix(hash, uint64(index+1))
		hash = formalComponentMix(hash, uint64(leaf))
		index++
	}
	for _, group := range e.layout.nonValues {
		for _, ordinal := range group.members {
			leaf, present := e.leaves.leaf(ordinal)
			if !present {
				return 0
			}
			hash = formalComponentMix(hash, uint64(index+1))
			hash = formalComponentMix(hash, uint64(leaf))
			index++
		}
	}
	return hash
}

func formalFactorExecutionVectorMatches(e formalTupleLeafEvaluator, vector []decisionLeaf) bool {
	if len(vector) != len(e.layout.values.members)+e.layout.vectorWidth {
		return false
	}
	index := 0
	for _, ordinal := range e.layout.values.members {
		leaf, present := e.leaves.leaf(ordinal)
		if !present || vector[index] != leaf {
			return false
		}
		index++
	}
	for _, group := range e.layout.nonValues {
		for _, ordinal := range group.members {
			leaf, present := e.leaves.leaf(ordinal)
			if !present || vector[index] != leaf {
				return false
			}
			index++
		}
	}
	return true
}

// formalApplySelectedFactorExecutionCapabilityRef is an Apply-only dense-vector
// lookup keyed by the operator's retained coordinate selector. A miss is a
// construction defect, never permission to inspect or seal a factor in Apply.
func (a *formalTupleAlgebra) formalApplySelectedFactorExecutionCapabilityRef(e formalTupleLeafEvaluator, selectorRef formalApplyCoordinateSelectorRef) (formalFactorExecutionCapability, error) {
	if !e.valid() || a.program == nil || a.program.formalFibers == nil || a.program.formalFibers.applySelectors == nil ||
		selectorRef.catalog != a.program.formalFibers.applySelectors || selectorRef.target != e.variable || !selectorRef.valid() {
		return formalFactorExecutionCapability{}, errFormalComponentForeignOwner
	}
	key := formalFactorExecutionCapabilityKey{body: e.authority.body, hash: formalFactorExecutionVectorHash(e), selector: selectorRef}
	for _, entry := range a.factorExecutionCapabilities[key] {
		if formalFactorExecutionVectorMatches(e, entry.leaves) {
			return entry.capability, nil
		}
	}
	return formalFactorExecutionCapability{}, fmt.Errorf("transformer: Apply target product vector has no prefrozen execution capability")
}

// cacheFormalOutcomeFactorSpellings records each stabilized lane spelling at
// the Outcome producer boundary. Coordinate members can be joined independently
// into a physical lane spelling that no earlier transfer factored, so Apply
// must not reconstruct it. Each lane is partitioned independently: capability
// construction remains owned by the exact caller/target region and no product
// Cartesian partition is formed here.
func (a *formalTupleAlgebra) cacheFormalOutcomeFactorSpellings(tuple formalRelationTuple) error {
	if err := a.validateTuple(tuple); err != nil || tuple.bottom() {
		if err != nil {
			return err
		}
		return fmt.Errorf("transformer: formal factor spelling source is Bottom")
	}
	_, directory, authority, ok := a.span(tuple.variable)
	if !ok || tuple.root.owner != directory {
		return fmt.Errorf("transformer: outcome factor spelling ownership: %w", errFormalComponentForeignOwner)
	}
	layout := &authority.body.factors
	for _, group := range layout.nonValues {
		regions, err := a.partitionSparseLeafViewsUnderCare(
			[]formalSparseTupleProjection{{tuple: tuple, ordinals: group.members}}, nil,
		)
		if err != nil {
			return err
		}
		for _, region := range regions {
			if len(region.views) != 1 {
				return errDecisionMalformed
			}
			view := region.views[0]
			switch group.kind {
			case formalFiberGroupOrdinaryLane:
				leaves := make([]decisionLeaf, len(group.members))
				for index, ordinal := range group.members {
					leaf, present := view.leaf(ordinal)
					if !present {
						return errFormalComponentMalformed
					}
					leaves[index] = leaf
				}
				factor, factorErr := a.materializeOrdinaryGroup(view.authority, group, leaves)
				if factorErr != nil {
					return factorErr
				}
				if factorErr = a.cacheFormalFactorReachability(view.authority, group, leaves, factor); factorErr != nil {
					return factorErr
				}
			case formalFiberGroupCoordinateLane:
				if err := a.cacheFormalSparseCoordinateGroup(view, group); err != nil {
					return err
				}
			default:
				return errFormalComponentMalformed
			}
		}
	}
	return nil
}
