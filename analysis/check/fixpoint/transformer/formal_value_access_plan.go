package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalValueAccessPlan is the operation-independent sparse capability for
// Values. Reads and writes are sealed as group-owned members; Values Top is an
// implicit read whenever either set is non-empty because it changes every
// finite coordinate into product Top. Top is deliberately not writable.
//
// readOrdinals is the semantic dependency projection. It does not include a
// write-only coordinate merely to compare the old value: the tuple directory
// is the structural carry, and publication conditions a new leaf over it.
type formalValueAccessPlan struct {
	group         formalFiberGroupDescriptor
	top           formalFiberGroupMember
	reads         []formalFiberGroupMember
	writes        []formalFiberGroupMember
	readOrdinals  []formalFiberOrdinal
	writeOrdinals []formalFiberOrdinal
	readSlots     map[FormalSlot]int
	writeSlots    map[FormalSlot]int
}

type formalValueLeafWrite struct {
	slot    FormalSlot
	ordinal formalFiberOrdinal
	leaf    decisionLeaf
}

func formalValueSlotForMember(group formalFiberGroupDescriptor, member formalFiberGroupMember) (FormalSlot, bool) {
	if _, ok := member.address(group); !ok || group.kind != formalFiberGroupValues {
		return FormalSlot{}, false
	}
	for _, candidate := range group.valueSlots {
		if candidate.position == member.position && candidate.ordinal == member.ordinal {
			return candidate.slot, true
		}
	}
	return FormalSlot{}, false
}

// freezeFormalValueTermSlots is the Values projection of the canonical
// ValueTerm dependency walk. Root terms retain their typed formal vocabulary
// directly; evolving State-key dependencies are rebound to their Middle slot.
// This is the same distinction used by formalTupleLeafEvaluator.valueAtRoot.
func freezeFormalValueTermSlots(program *RelationProgram, owner relationVar, terms ...ValueTerm) ([]FormalSlot, error) {
	if program == nil || owner == 0 || int(owner) > len(program.bodies) {
		return nil, errFormalComponentForeignOwner
	}
	body := &program.bodies[owner-1]
	arena := body.relation.arena
	if arena == nil || !arena.Sealed() {
		return nil, fmt.Errorf("transformer: formal Values access has no sealed term arena")
	}
	if len(terms) == 0 {
		return nil, nil
	}
	if program.formalSlots == nil {
		return nil, errFormalComponentForeignOwner
	}
	seenTerms := make(map[ValueTerm]struct{})
	seenSlots := make(map[FormalSlot]struct{})
	stack := append([]ValueTerm(nil), terms...)
	for len(stack) != 0 {
		term := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if term == 0 || int(term) >= len(arena.values) {
			return nil, fmt.Errorf("transformer: formal Values access contains a foreign term")
		}
		if _, seen := seenTerms[term]; seen {
			continue
		}
		seenTerms[term] = struct{}{}
		node := arena.values[term]
		if node.op == valueRoot {
			slot, exact := program.formalSlots.Slot(body.body, node.root)
			if !exact {
				return nil, fmt.Errorf("transformer: formal Values root has no typed slot")
			}
			seenSlots[slot] = struct{}{}
		} else {
			direct, err := body.valueTermNodeFactorAccess(term)
			if err != nil {
				return nil, err
			}
			for _, concrete := range direct.Values {
				slot, exact := formalMiddleSlotForStateKey(program, body, concrete)
				if !exact {
					return nil, fmt.Errorf("transformer: formal Values dependency %d has no Middle slot", concrete)
				}
				seenSlots[slot] = struct{}{}
			}
		}
		if node.op == valueSelect {
			atoms := make(map[ValueTerm]struct{})
			if err := collectRelationGuardAtoms(arena, node.guard, atoms, make(map[Guard]uint8)); err != nil {
				return nil, err
			}
			for atom := range atoms {
				stack = append(stack, atom)
			}
		}
		stack = append(stack, node.args...)
	}
	out := make([]FormalSlot, 0, len(seenSlots))
	for slot := range seenSlots {
		out = append(out, slot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].root.Less(out[j].root) })
	return out, nil
}

func sealFormalValueAccessPlan(values formalValuesFiberGroup, reads, writes []FormalSlot) (formalValueAccessPlan, error) {
	if !values.valid() {
		return formalValueAccessPlan{}, errFormalComponentMalformed
	}
	if len(reads) == 0 && len(writes) == 0 {
		return formalValueAccessPlan{}, nil
	}
	plan := formalValueAccessPlan{group: values.descriptor}
	var ok bool
	plan.top, ok = values.top()
	if !ok {
		return formalValueAccessPlan{}, errFormalComponentMalformed
	}
	seal := func(slots []FormalSlot) ([]formalFiberGroupMember, map[FormalSlot]int, error) {
		members := make([]formalFiberGroupMember, 0, len(slots))
		seen := make(map[FormalSlot]struct{}, len(slots))
		for _, slot := range slots {
			if _, duplicate := seen[slot]; duplicate {
				continue
			}
			member, ok := values.slot(slot)
			if !ok {
				return nil, nil, fmt.Errorf("transformer: sparse Values slot is outside its declared group")
			}
			seen[slot] = struct{}{}
			members = append(members, member)
		}
		sort.Slice(members, func(i, j int) bool { return members[i].ordinal < members[j].ordinal })
		positions := make(map[FormalSlot]int, len(members))
		for index, member := range members {
			slot, ok := formalValueSlotForMember(values.descriptor, member)
			if !ok {
				return nil, nil, errFormalComponentMalformed
			}
			positions[slot] = index
		}
		return members, positions, nil
	}
	var err error
	plan.reads, plan.readSlots, err = seal(reads)
	if err != nil {
		return formalValueAccessPlan{}, err
	}
	plan.writes, plan.writeSlots, err = seal(writes)
	if err != nil {
		return formalValueAccessPlan{}, err
	}
	plan.readOrdinals = make([]formalFiberOrdinal, 0, len(plan.reads)+1)
	if len(plan.reads) != 0 || len(plan.writes) != 0 {
		plan.readOrdinals = append(plan.readOrdinals, plan.top.ordinal)
	}
	for _, member := range plan.reads {
		plan.readOrdinals = append(plan.readOrdinals, member.ordinal)
	}
	sort.Slice(plan.readOrdinals, func(i, j int) bool { return plan.readOrdinals[i] < plan.readOrdinals[j] })
	plan.writeOrdinals = make([]formalFiberOrdinal, len(plan.writes))
	for index, member := range plan.writes {
		plan.writeOrdinals[index] = member.ordinal
	}
	return plan, nil
}

func (p formalValueAccessPlan) valid() bool {
	if !p.group.valid() {
		return len(p.reads) == 0 && len(p.writes) == 0 && len(p.readOrdinals) == 0 && len(p.writeOrdinals) == 0 && len(p.readSlots) == 0 && len(p.writeSlots) == 0
	}
	if !p.group.valid() || p.group.kind != formalFiberGroupValues || len(p.reads) != len(p.readSlots) || len(p.writes) != len(p.writeSlots) {
		return false
	}
	if len(p.reads) == 0 && len(p.writes) == 0 {
		return len(p.readOrdinals) == 0 && len(p.writeOrdinals) == 0
	}
	if _, ok := p.top.address(p.group); !ok || len(p.readOrdinals) != len(p.reads)+1 || len(p.writeOrdinals) != len(p.writes) {
		return false
	}
	for index, member := range p.reads {
		if _, ok := member.address(p.group); !ok || index > 0 && p.reads[index-1].ordinal >= member.ordinal {
			return false
		}
	}
	for index, member := range p.writes {
		if _, ok := member.address(p.group); !ok || index > 0 && p.writes[index-1].ordinal >= member.ordinal || p.writeOrdinals[index] != member.ordinal {
			return false
		}
	}
	return sort.SliceIsSorted(p.readOrdinals, func(i, j int) bool { return p.readOrdinals[i] < p.readOrdinals[j] })
}

// materialize reads only the plan's exact scalar dependency set. Missing map
// coordinates denote Bottom; an unselected declared coordinate is an error.
func (p formalValueAccessPlan) materialize(view formalSparseLeafView) (state.ValueFactor[FormalSlot], error) {
	if !p.valid() {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentForeignOwner
	}
	if !p.group.valid() {
		return state.ValueFactor[FormalSlot]{}, nil
	}
	if view.authority == nil || view.variable != p.group.variable {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentForeignOwner
	}
	top, present := view.leaf(p.top.ordinal)
	if !present || top > 1 {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
	}
	if top == 1 {
		return state.ValueFactor[FormalSlot]{Top: true}, nil
	}
	bottom := product.Bottom(view.authority.product.Registry())
	var out map[FormalSlot]product.Value
	for _, member := range p.reads {
		value, exact := view.value(member, p.top)
		if !exact {
			return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
		}
		if product.Equal(view.authority.product.Registry(), value, bottom) {
			continue
		}
		if out == nil {
			out = make(map[FormalSlot]product.Value, len(p.reads))
		}
		slot, ok := formalValueSlotForMember(p.group, member)
		if !ok {
			return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
		}
		out[slot] = value
	}
	return state.ValueFactor[FormalSlot]{Values: out}, nil
}

// factorPublication validates that a kernel changed no undeclared coordinate
// and returns the complete declared patch in deterministic physical order.
// Under Values Top, scalar roots are semantically dormant and remain exact
// structural carry instead of being redundantly rewritten to Bottom.
func (p formalValueAccessPlan) factorPublication(view formalSparseLeafView, result state.ValueFactor[FormalSlot]) ([]formalValueLeafWrite, error) {
	input, err := p.materialize(view)
	if err != nil {
		return nil, err
	}
	if !p.group.valid() {
		if result.Top || len(result.Values) != 0 {
			return nil, fmt.Errorf("transformer: empty sparse Values publication is non-empty")
		}
		return nil, nil
	}
	if result.Top != input.Top || result.Top && len(result.Values) != 0 {
		return nil, fmt.Errorf("transformer: sparse Values publication changed Top")
	}
	reg := view.authority.product.Registry()
	bottom := product.Bottom(reg)
	valueAt := func(values map[FormalSlot]product.Value, slot FormalSlot) product.Value {
		if value, ok := values[slot]; ok {
			return value
		}
		return bottom
	}
	for slot, value := range result.Values {
		if !product.BelongsToRegistry(reg, value) {
			return nil, fmt.Errorf("transformer: sparse Values publication contains a foreign product")
		}
		if _, writable := p.writeSlots[slot]; writable {
			continue
		}
		if _, readable := p.readSlots[slot]; !readable || !product.Equal(reg, value, valueAt(input.Values, slot)) {
			return nil, fmt.Errorf("transformer: sparse Values publication writes an undeclared slot")
		}
	}
	for slot := range p.readSlots {
		if _, writable := p.writeSlots[slot]; writable {
			continue
		}
		if !product.Equal(reg, valueAt(result.Values, slot), valueAt(input.Values, slot)) {
			return nil, fmt.Errorf("transformer: sparse Values publication changes a read-only slot")
		}
	}
	if input.Top {
		return nil, nil
	}
	out := make([]formalValueLeafWrite, len(p.writes))
	for index, member := range p.writes {
		slot, ok := formalValueSlotForMember(p.group, member)
		if !ok {
			return nil, errFormalComponentMalformed
		}
		value := valueAt(result.Values, slot)
		leaf := decisionLeaf(0)
		if !product.Equal(reg, value, bottom) {
			leaf, err = view.authority.internGroundValue(value)
			if err != nil {
				return nil, err
			}
		}
		if prior, selected := view.leaf(member.ordinal); selected && prior == leaf {
			continue
		}
		out[index] = formalValueLeafWrite{slot: slot, ordinal: member.ordinal, leaf: leaf}
	}
	write := 0
	for _, candidate := range out {
		if candidate.slot.Valid() {
			out[write] = candidate
			write++
		}
	}
	return out[:write], nil
}
