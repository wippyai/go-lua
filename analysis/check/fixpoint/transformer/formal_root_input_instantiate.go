package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// instantiateRootEquation creates the run-owned lexical entry tuple for one
// exact root equation. Boundary inputs remain sealed symbolic IN terms bound
// atomically into their MID slots; concrete InitialState overlays enter only
// through the equation's already-frozen tuple constants.
func (a *formalTupleAlgebra) instantiateRootEquation(equation formalRelationEquation) (formalRelationTuple, error) {
	if a == nil || a.program == nil || a.firstError != nil || equation.Operator.rootInput == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal root equation is unowned")
	}
	root := equation.Operator.rootInput
	if err := a.validateRootInputEquation(root, equation); err != nil {
		return formalRelationTuple{}, err
	}
	if a.rootEntry != nil && a.rootEntry.variable == root.variable {
		if !a.rootEntry.validFor(a.program) {
			return formalRelationTuple{}, fmt.Errorf("transformer: formal root entry has foreign ownership")
		}
		return a.instantiatePreparedConstant(a.rootEntry.constant)
	}
	span, directory, authority, ok := a.span(root.variable)
	if !ok || span.forest != a.program.formalFibers {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal root input has no run-local span")
	}

	tuple := formalRelationTuple{variable: root.variable, root: directory.defaultRoot()}
	var err error
	tuple, err = a.writeCare(tuple, decisionTrue)
	if err != nil {
		return formalRelationTuple{}, err
	}
	for _, binding := range root.bindings {
		descriptor, descriptorErr := a.rootInputMiddleDescriptor(root, binding)
		if descriptorErr != nil {
			return formalRelationTuple{}, descriptorErr
		}
		leaf, internErr := authority.internBinding(formalQualifiedBinding{
			value:       relationArenaValueRef{owner: root.variable, arena: binding.arena, term: binding.inputValue},
			path:        relationArenaPathRef{owner: root.variable, arena: binding.arena, term: binding.inputPath},
			pathPresent: binding.hasPath,
		})
		if internErr != nil {
			return formalRelationTuple{}, internErr
		}
		tuple, err = a.writeScalar(tuple, descriptor, a.decisions.terminal(leaf))
		if err != nil {
			return formalRelationTuple{}, err
		}
	}
	// validateRootInputEquation proved that root.groups covers the exact frozen
	// product inventory. The directory default is canonical Bottom for every
	// such group, so invoking registered group laws merely to rewrite Bottom
	// would be redundant work (and could invent a different opaque spelling).
	for _, seed := range equation.Seeds {
		constant, constantErr := a.instantiateConstant(seed)
		if constantErr != nil {
			return formalRelationTuple{}, constantErr
		}
		tuple = a.combine(formalComponentJoin, tuple, constant)
		if err := a.err(); err != nil {
			return formalRelationTuple{}, err
		}
	}
	tuple, err = a.applyRootEntrySeeds(root, tuple)
	if err != nil {
		return formalRelationTuple{}, err
	}
	tuple = a.normalize(tuple)
	if tuple.bottom() {
		return formalRelationTuple{}, fmt.Errorf("transformer: live formal root input normalized to Bottom")
	}
	if err := a.validateTuple(tuple); err != nil {
		return formalRelationTuple{}, err
	}
	return tuple, nil
}

// applyRootEntrySeeds is the formal adapter for state.EntrySeedFactorPlan.
// Only Values Top and the plan's catalog-bound slots are partitioned.  Every
// other Values coordinate and every unrelated product lane remains an
// untouched directory subtree.
func (a *formalTupleAlgebra) applyRootEntrySeeds(root *formalRootInputTemplate, tuple formalRelationTuple) (formalRelationTuple, error) {
	if root == nil || !root.entrySeeds.Valid() || len(root.entrySeedOrdinals) == 0 ||
		len(root.entrySeedSlots) != len(root.entrySeedOrdinals) || tuple.variable != root.variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal root EntrySeed is unowned")
	}
	if root.entrySeeds.Len() == 0 {
		return tuple, nil
	}
	span, directory, authority, ok := a.span(root.variable)
	if !ok || tuple.root.owner != directory {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	values, ok := span.valuesGroup()
	if !ok {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal root EntrySeed has no Values group")
	}
	ordinals := append([]formalFiberOrdinal(nil), root.entrySeedOrdinals...)
	for index, ordinal := range ordinals {
		if index > 0 && ordinals[index-1] >= ordinal {
			return formalRelationTuple{}, fmt.Errorf("transformer: formal root EntrySeed selection is malformed")
		}
	}
	partitions, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{tuple: tuple, ordinals: ordinals}}, nil)
	if err != nil {
		return formalRelationTuple{}, err
	}
	roots := make([]decisionRef, len(ordinals))
	for index, ordinal := range ordinals {
		value, readErr := directory.valueAt(tuple.root, ordinal)
		if readErr != nil {
			return formalRelationTuple{}, readErr
		}
		roots[index] = decisionRef(value)
	}
	top, topPresent := values.descriptor.member(ordinals[0])
	if _, valid := top.address(values.descriptor); !topPresent || !valid {
		return formalRelationTuple{}, errFormalComponentMalformed
	}
	for _, partition := range partitions {
		if len(partition.views) != 1 {
			return formalRelationTuple{}, errFormalComponentMalformed
		}
		view := partition.views[0]
		factor := state.ValueFactor[FormalSlot]{}
		topLeaf, topPresent := view.leaf(ordinals[0])
		if !topPresent || topLeaf > 1 {
			return formalRelationTuple{}, errFormalComponentMalformed
		}
		if topLeaf == 1 {
			factor.Top = true
		} else {
			factor.Values = make(map[FormalSlot]product.Value, len(root.entrySeedOrdinals)-1)
			for index, ordinal := range root.entrySeedOrdinals[1:] {
				member, memberPresent := values.descriptor.member(ordinal)
				if !memberPresent {
					return formalRelationTuple{}, errFormalComponentMalformed
				}
				value, present := view.value(member, top)
				if !present {
					return formalRelationTuple{}, errFormalComponentMalformed
				}
				factor.Values[root.entrySeedSlots[index+1]] = value
			}
		}
		factor, err = root.entrySeeds.Apply(a.program.registry, factor)
		if err != nil {
			return formalRelationTuple{}, err
		}
		leaves := make([]decisionLeaf, len(ordinals))
		if factor.Top {
			leaves[0] = 1
		} else {
			for index, slot := range root.entrySeedSlots[1:] {
				value := factor.Values[slot]
				if product.Equal(a.program.registry, value, product.Bottom(a.program.registry)) {
					continue
				}
				leaves[index+1], err = authority.internGroundValue(value)
				if err != nil {
					return formalRelationTuple{}, err
				}
			}
		}
		for index, leaf := range leaves {
			roots[index], err = a.decisions.condition(a.ctx, partition.guard, a.decisions.terminal(leaf), roots[index])
			if err != nil {
				return formalRelationTuple{}, err
			}
		}
	}
	writes := make([]formalFiberWrite, len(ordinals))
	for index, ordinal := range ordinals {
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, roots[index]); err != nil {
			return formalRelationTuple{}, err
		}
		writes[index] = formalFiberWrite{ordinal: ordinal, value: formalFiberValue(roots[index])}
	}
	delta, err := directory.sealDelta(writes)
	if err != nil {
		return formalRelationTuple{}, err
	}
	next, _, err := directory.applyDelta(tuple.root, delta)
	if err != nil {
		return formalRelationTuple{}, err
	}
	return a.normalize(formalRelationTuple{variable: tuple.variable, root: next}), nil
}

func (a *formalTupleAlgebra) validateRootInputEquation(root *formalRootInputTemplate, equation formalRelationEquation) error {
	if root == nil || root.program != a.program || root.variable == 0 || !root.care ||
		a.program.formalRegion == nil || equation.Cell.region != a.program.formalRegion || !equation.Cell.valid() ||
		int(root.variable) > len(a.program.formalRegion.roots) || equation.Cell.cell != a.program.formalRegion.roots[root.variable-1] ||
		equation.Cell.cell.Variable != root.variable || equation.Cell.cell.Kind != formalRelationCellNode ||
		equation.Operator.kind != formalRelationCellNode || equation.Operator.code == nil ||
		equation.Operator.code != a.program.bodies[root.variable-1].relation.code || equation.Operator.root != equation.Cell.cell.Root {
		return fmt.Errorf("transformer: formal root equation has foreign ownership")
	}
	body, ok := formalRootInputBody(a.program, root.variable)
	span, spanOK := a.program.formalFibers.span(root.variable)
	if !ok || !spanOK || len(root.bindings) != body.relation.shape.InputCount() || len(root.groups) != span.groupCount {
		return fmt.Errorf("transformer: formal root input template is incomplete")
	}
	for index, binding := range root.bindings {
		if !binding.valid(body.relation.arena, body.relation.shape) {
			return fmt.Errorf("transformer: formal root input binding %d is malformed", index)
		}
		for prior := 0; prior < index; prior++ {
			if root.bindings[prior].input == binding.input || root.bindings[prior].middle == binding.middle {
				return fmt.Errorf("transformer: formal root input binding %d is duplicated", index)
			}
		}
	}
	for index, ref := range root.groups {
		global := span.groupFirst + index
		if !ref.valid() || ref.forest != a.program.formalFibers || ref.variable != root.variable || ref.groupGlobal != global {
			return fmt.Errorf("transformer: formal root input group %d is foreign or incomplete", index)
		}
		group := a.program.formalFibers.groups[global]
		if group.lane.Ordinal() != ref.lane ||
			ref.kind == formalInputGroupValues && group.kind != formalFiberGroupValues ||
			ref.kind == formalInputGroupOrdinaryLane && group.kind != formalFiberGroupOrdinaryLane ||
			ref.kind == formalInputGroupCoordinateLane && group.kind != formalFiberGroupCoordinateLane {
			return fmt.Errorf("transformer: formal root input group %d has malformed kind", index)
		}
	}
	for index, seed := range equation.Seeds {
		_, valid := seed.constant(equation.Cell)
		if !valid || !seed.entry || seed.point != body.graph.Entry() {
			return fmt.Errorf("transformer: formal root input seed %d is foreign or non-entry", index)
		}
	}
	return nil
}

func (a *formalTupleAlgebra) rootInputMiddleDescriptor(root *formalRootInputTemplate, binding formalRootInputBinding) (formalFiberDescriptor, error) {
	if a == nil || root == nil || root.program != a.program || a.program.formalSlots == nil {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal root input Middle descriptor is unowned")
	}
	body, ok := formalRootInputBody(a.program, root.variable)
	if !ok || !binding.valid(body.relation.arena, body.relation.shape) {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal root input binding is foreign")
	}
	slot, ok := a.program.formalSlots.Slot(body.body, binding.middle)
	if !ok {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal root input Middle has no formal slot")
	}
	span, _, _, ok := a.span(root.variable)
	if !ok {
		return formalFiberDescriptor{}, errFormalComponentForeignOwner
	}
	var found formalFiberDescriptor
	for _, descriptor := range span.descriptors() {
		if descriptor.role != formalFiberMiddleValue || descriptor.slot != slot {
			continue
		}
		if found.role != formalFiberInvalid {
			return formalFiberDescriptor{}, fmt.Errorf("transformer: formal root input Middle descriptor is ambiguous")
		}
		found = descriptor
	}
	if found.role == formalFiberInvalid {
		return formalFiberDescriptor{}, fmt.Errorf("transformer: formal root input Middle descriptor is missing")
	}
	return found, nil
}
