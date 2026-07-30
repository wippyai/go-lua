package transformer

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalRelationTupleConstant is immutable equation syntax for one prepared
// State seed after it has been transposed into the sole formal product
// vocabulary. It retains complete registered factors, never a concrete State
// or an executor callback.
type formalRelationTupleConstant struct {
	forest   *formalFiberInventory
	variable relationVar
	care     bool
	groups   []formalTupleConstantGroup
}

type formalTupleConstantGroup struct {
	group  formalFiberGroupDescriptor
	factor state.LaneFactor
	values state.ValueFactor[FormalSlot]
}

func (c formalRelationTupleConstant) valid() bool {
	if c.forest == nil || c.variable == 0 || !c.care {
		return false
	}
	span, ok := c.forest.span(c.variable)
	if !ok || len(c.groups) != span.groupCount {
		return false
	}
	for index, constant := range c.groups {
		want := c.forest.groups[span.groupFirst+index]
		if !constant.group.same(want) {
			return false
		}
		switch want.kind {
		case formalFiberGroupValues:
			if constant.factor.Lane() != (state.ProductLane{}) || !(formalValuesFiberGroup{descriptor: want}).owns(constant.values) {
				return false
			}
		case formalFiberGroupOrdinaryLane, formalFiberGroupCoordinateLane:
			if constant.factor.Lane() != want.lane || constant.values.Top || len(constant.values.Values) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// formalRelationTupleConstantRef is an equation-owned capability to one
// template constant. Point and entry preserve the exact InitialStatePlan
// coordinate semantics without retaining the plan as a solve-time query.
type formalRelationTupleConstantRef struct {
	template *formalRelationTemplate
	index    int
	target   formalRelationCellRef
	point    cfg.Point
	entry    bool
}

func (r formalRelationTupleConstantRef) constant(target formalRelationCellRef) (formalRelationTupleConstant, bool) {
	if r.template == nil || !target.valid() || !r.target.valid() || r.target != target || target.region != r.template.region || target.cell.Kind != formalRelationCellNode ||
		r.index < 0 || r.index >= len(r.template.constants) {
		return formalRelationTupleConstant{}, false
	}
	constant := r.template.constants[r.index]
	if constant.variable == 0 || int(constant.variable) > len(r.template.rootInputs) {
		return formalRelationTupleConstant{}, false
	}
	root := r.template.rootInputs[constant.variable-1]
	valid := constant.valid() && root.program != nil && constant.forest == root.program.formalFibers && constant.variable == target.cell.Variable
	if r.entry {
		valid = valid && target.cell == r.template.region.roots[constant.variable-1]
	}
	return constant, valid
}

func freezeFormalInitialStateSeeds(program *RelationProgram, template *formalRelationTemplate) error {
	if program == nil || template == nil || template.region != program.formalRegion || program.formalFibers == nil ||
		program.formalFibers.slots == nil || len(program.bodies) != len(template.rootInputs) {
		return fmt.Errorf("transformer: formal initial-state constants have no sealed forest")
	}
	for bodyIndex := range program.bodies {
		body := &program.bodies[bodyIndex]
		variable := relationVar(bodyIndex + 1)
		if body.variable != variable || body.graph == nil || !body.initialStatePlan.ValidFor(body.body, body.graph.ID(), body.graph.Size()) {
			return fmt.Errorf("transformer: formal relation %d has a foreign initial-state plan", variable)
		}
		for seedIndex := 0; seedIndex < body.initialStatePlan.Len(); seedIndex++ {
			coordinate, raw, present := body.initialStatePlan.Seed(seedIndex)
			if !present {
				return fmt.Errorf("transformer: formal relation %d initial-state seed %d is absent", variable, seedIndex)
			}
			point := cfg.Point(coordinate)
			constant, err := freezeFormalRelationTupleConstant(program, variable, raw)
			if err != nil {
				return fmt.Errorf("transformer: formal relation %d initial-state seed %d: %w", variable, seedIndex, err)
			}
			constantIndex := len(template.constants)
			template.constants = append(template.constants, constant)
			target := template.region.roots[bodyIndex]
			entry := point == body.graph.Entry()
			if !entry {
				seen := make(map[relationRootRef]struct{})
				var matched relationRootRef
				for _, publication := range body.relation.code.publication.points {
					if publication.point != point {
						continue
					}
					if publication.ref == 0 || int(publication.ref) >= len(body.relation.code.nodes) {
						return fmt.Errorf("transformer: formal relation %d initial-state point %d has malformed publication", variable, point)
					}
					if _, duplicate := seen[publication.ref]; duplicate {
						return fmt.Errorf("transformer: formal relation %d initial-state point %d has duplicate publication", variable, point)
					}
					seen[publication.ref] = struct{}{}
					if matched != 0 && matched != publication.ref {
						return fmt.Errorf("transformer: formal relation %d initial-state point %d has ambiguous publications", variable, point)
					}
					matched = publication.ref
				}
				if matched == 0 {
					return fmt.Errorf("transformer: formal relation %d initial-state point %d is unpublished", variable, point)
				}
				target = formalRelationCell{Variable: variable, Root: matched, Kind: formalRelationCellNode}
			}
			equationIndex, ok := template.region.plan.CanonicalIndex(target)
			if !ok || equationIndex < 0 || equationIndex >= len(template.equations) || template.equations[equationIndex].Cell.cell != target {
				return fmt.Errorf("transformer: formal relation %d initial-state point %d has no equation", variable, point)
			}
			targetRef := template.equations[equationIndex].Cell
			ref := formalRelationTupleConstantRef{template: template, index: constantIndex, target: targetRef, point: point, entry: entry}
			if _, valid := ref.constant(targetRef); !valid {
				return fmt.Errorf("transformer: formal relation %d initial-state point %d has invalid constant ownership", variable, point)
			}
			for _, prior := range template.equations[equationIndex].Seeds {
				if prior.point == point {
					return fmt.Errorf("transformer: formal relation %d initial-state point %d is duplicated", variable, point)
				}
			}
			template.equations[equationIndex].Seeds = append(template.equations[equationIndex].Seeds, ref)
		}
	}
	return nil
}

func freezeFormalRelationTupleConstant(program *RelationProgram, variable relationVar, raw state.State) (formalRelationTupleConstant, error) {
	body, ok := formalRootInputBody(program, variable)
	span, spanOK := program.formalFibers.span(variable)
	if !ok || !spanOK || !body.productDomain.Valid() || body.productDomain.Registry() != program.registry || span.forest != program.formalFibers {
		return formalRelationTupleConstant{}, fmt.Errorf("initial-state constant has no formal product owner")
	}
	prepared := state.NormalizeForDomain(body.domain, state.Reachable(raw))
	_, concreteValues := state.DecomposeValueLane(body.productDomain.Lattice(), prepared)
	nonValues := body.productDomain.NonValuesLaneInventory()
	factors, err := body.productDomain.DecomposeLanes(prepared, nonValues)
	if err != nil || len(factors) != len(nonValues) {
		return formalRelationTupleConstant{}, fmt.Errorf("initial-state constant decomposition: %w", err)
	}
	byLane := make(map[state.LaneOrdinal]state.LaneFactor, len(factors))
	for index, factor := range factors {
		if factor.Lane() != nonValues[index] {
			return formalRelationTupleConstant{}, fmt.Errorf("initial-state constant lane order drifted")
		}
		byLane[factor.Lane().Ordinal()] = factor
	}
	groups := span.groupDescriptors()
	constant := formalRelationTupleConstant{forest: program.formalFibers, variable: variable, care: true, groups: make([]formalTupleConstantGroup, len(groups))}
	for index, group := range groups {
		entry := formalTupleConstantGroup{group: group}
		switch group.kind {
		case formalFiberGroupValues:
			mapped, mapErr := freezeFormalInitialValues(program.formalFibers.slots, body, concreteValues)
			if mapErr != nil || !(formalValuesFiberGroup{descriptor: group}).owns(mapped) {
				if mapErr == nil {
					mapErr = fmt.Errorf("Values escaped formal fiber inventory")
				}
				return formalRelationTupleConstant{}, mapErr
			}
			entry.values = mapped
		case formalFiberGroupOrdinaryLane:
			factor, present := byLane[group.lane.Ordinal()]
			if !present || factor.Lane() != group.lane {
				return formalRelationTupleConstant{}, fmt.Errorf("initial-state constant omits ordinary lane %q", group.lane.ID())
			}
			mapped, mapErr := body.productDomain.RekeyOrdinaryLaneFactorFormal(span.rekey, factor)
			if mapErr != nil {
				return formalRelationTupleConstant{}, fmt.Errorf("initial-state ordinary lane %q formal rekey: %w", group.lane.ID(), mapErr)
			}
			entry.factor = mapped
		case formalFiberGroupCoordinateLane:
			factor, present := byLane[group.lane.Ordinal()]
			if !present || factor.Lane() != group.lane {
				return formalRelationTupleConstant{}, fmt.Errorf("initial-state constant omits coordinate lane %q", group.lane.ID())
			}
			mapped, mapErr := freezeFormalInitialCoordinateFactor(body, span, group, factor)
			if mapErr != nil {
				return formalRelationTupleConstant{}, mapErr
			}
			entry.factor = mapped
		default:
			return formalRelationTupleConstant{}, fmt.Errorf("initial-state constant has invalid group kind")
		}
		constant.groups[index] = entry
	}
	if !constant.valid() {
		return formalRelationTupleConstant{}, fmt.Errorf("initial-state constant is incomplete")
	}
	return constant, nil
}

func freezeFormalInitialValues(slots *SlotSpace, body *relationProgramBody, concrete state.ValueLaneFactor) (state.ValueFactor[FormalSlot], error) {
	if slots == nil || body == nil || concrete.Top && len(concrete.Values) != 0 {
		return state.ValueFactor[FormalSlot]{}, fmt.Errorf("initial-state Values are malformed")
	}
	if concrete.Top {
		return state.ValueFactor[FormalSlot]{Top: true}, nil
	}
	values := make(map[FormalSlot]product.Value, len(concrete.Values))
	for source, value := range concrete.Values {
		target, ok := formalInitialValueSlot(slots, body, source)
		if !ok {
			return state.ValueFactor[FormalSlot]{}, fmt.Errorf("initial-state Values slot %d has no formal root", source)
		}
		if _, duplicate := values[target]; duplicate {
			return state.ValueFactor[FormalSlot]{}, fmt.Errorf("initial-state Values contain duplicate formal root")
		}
		values[target] = value
	}
	return state.ValueFactor[FormalSlot]{Values: values}, nil
}

func formalInitialValueSlot(slots *SlotSpace, body *relationProgramBody, source statekey.Value) (FormalSlot, bool) {
	if slots == nil || body == nil || body.relation.arena == nil || source == 0 {
		return FormalSlot{}, false
	}
	if middle, ok := body.relation.arena.middle.root(source); ok {
		return slots.Slot(body.body, middle)
	}
	for _, input := range body.roots.roots {
		if input.slot == source {
			return slots.Slot(body.body, input.root)
		}
	}
	return FormalSlot{}, false
}

func freezeFormalInitialCoordinateFactor(body *relationProgramBody, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, factor state.LaneFactor) (state.LaneFactor, error) {
	if body == nil || !group.valid() || group.kind != formalFiberGroupCoordinateLane || factor.Lane() != group.lane || span.keys == nil || !span.keys.Valid() {
		return state.LaneFactor{}, fmt.Errorf("initial-state coordinate factor is unowned")
	}
	skeletons := make([]state.CoordinateFamilySkeleton, len(group.coordinateFamilies))
	scalars := make([][]state.CoordinateScalarFactor, len(group.coordinateFamilies))
	for familyIndex, family := range group.coordinateFamilies {
		skeleton, concreteScalars, err := body.productDomain.DecomposeCoordinateFamily(factor, family.family, body.keys)
		if err != nil {
			return state.LaneFactor{}, err
		}
		skeletons[familyIndex], err = body.productDomain.RekeyCoordinateSkeletonFormal(span.rekey, skeleton)
		if err != nil {
			return state.LaneFactor{}, err
		}
		scalars[familyIndex] = make([]state.CoordinateScalarFactor, len(concreteScalars))
		for scalarIndex, scalar := range concreteScalars {
			scalars[familyIndex][scalarIndex], err = body.productDomain.RekeyCoordinateScalarFormal(span.rekey, scalar)
			if err != nil {
				return state.LaneFactor{}, err
			}
		}
	}
	return body.productDomain.ComposeCoordinateFamilies(group.lane, span.keys, skeletons, scalars)
}
