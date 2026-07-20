package state

import (
	"errors"
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

var ErrInvalidProductPatch = errors.New("state: invalid product patch")

// ProductPatchPlan seals the exact product factors, Values slots, Values-top
// bit, and reachability bit one evaluator is allowed to publish. Descriptor
// membership is compiled once; a leaf can never manufacture a nominal axis.
type ProductPatchPlan struct {
	domain            ProductDomain
	carry             []ProductLane
	writes            []ProductLane
	undeclared        []*productLaneRuntime
	valueCarry        []statekey.Value
	valueCarryAll     bool
	valueWrites       []statekey.Value
	valuesTopWrite    bool
	reachabilityWrite bool
}

// SealProductPatch validates and canonicalizes one sparse evaluator contract.
// carry and writes may arrive in any order; the sealed plan owns catalog order.
func (d ProductDomain) SealProductPatch(
	carry, writes []ProductLane,
	valueCarry []statekey.Value, valueCarryAll bool, valueWrites []statekey.Value,
	valuesTopWrite, reachabilityWrite bool,
) (ProductPatchPlan, error) {
	if !d.Valid() {
		return ProductPatchPlan{}, fmt.Errorf("%w: invalid product domain", ErrInvalidProductPatch)
	}
	// Whole Values carry is a structural read/preserve role, never another
	// spelling of finite carry or whole Values ownership.
	if valueCarryAll && (len(valueCarry) != 0 || valuesTopWrite) {
		return ProductPatchPlan{}, fmt.Errorf("%w: whole Values carry overlaps explicit carry or whole write", ErrInvalidProductPatch)
	}
	sealLanes := func(input []ProductLane) ([]ProductLane, error) {
		selected := make([]bool, len(d.factorLanes))
		for _, lane := range input {
			runtime, err := d.validateLane(lane)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidProductPatch, err)
			}
			ordinal := int(runtime.lane.ordinal)
			if runtime.lane.slotFactored || selected[ordinal] {
				return nil, fmt.Errorf("%w: duplicate or slot-factored lane %q", ErrInvalidProductPatch, runtime.lane.id)
			}
			selected[ordinal] = true
		}
		out := make([]ProductLane, 0, len(input))
		for ordinal, present := range selected {
			if present {
				out = append(out, d.factorLanes[ordinal].lane)
			}
		}
		return out, nil
	}
	sealedCarry, err := sealLanes(carry)
	if err != nil {
		return ProductPatchPlan{}, err
	}
	sealedWrites, err := sealLanes(writes)
	if err != nil {
		return ProductPatchPlan{}, err
	}
	sealValues := func(input []statekey.Value) ([]statekey.Value, error) {
		sealed := append([]statekey.Value(nil), input...)
		sort.Slice(sealed, func(i, j int) bool { return sealed[i] < sealed[j] })
		for index, slot := range sealed {
			if slot == 0 || index != 0 && sealed[index-1] == slot {
				return nil, fmt.Errorf("%w: zero or duplicate Values slot at %d in %v", ErrInvalidProductPatch, index, sealed)
			}
		}
		return sealed, nil
	}
	sealedValueCarry, err := sealValues(valueCarry)
	if err != nil {
		return ProductPatchPlan{}, err
	}
	sealedValueWrites, err := sealValues(valueWrites)
	if err != nil {
		return ProductPatchPlan{}, err
	}
	writeOrdinals := make([]bool, len(d.factorLanes))
	for _, lane := range sealedWrites {
		writeOrdinals[lane.ordinal] = true
	}
	undeclared := make([]*productLaneRuntime, 0, len(d.factorLanes)-len(sealedWrites))
	for ordinal := range d.factorLanes {
		runtime := &d.factorLanes[ordinal]
		if !runtime.lane.slotFactored && !writeOrdinals[ordinal] {
			undeclared = append(undeclared, runtime)
		}
	}
	return ProductPatchPlan{
		domain: d, carry: sealedCarry, writes: sealedWrites, undeclared: undeclared,
		valueCarry: sealedValueCarry, valueCarryAll: valueCarryAll, valueWrites: sealedValueWrites,
		valuesTopWrite: valuesTopWrite, reachabilityWrite: reachabilityWrite,
	}, nil
}

// ProductPatchBuilder is a one-leaf transaction. Its only State-taking method
// extracts the plan's declared sparse result; Finish cannot expose a State.
type ProductPatchBuilder struct {
	plan           ProductPatchPlan
	carryFactors   []LaneFactor
	carryValues    ValueLaneFactor
	carryReachable bool
	writeFactors   []LaneFactor
	values         ValueLaneFactor
	reachable      bool
	used           bool
}

// NewBuilder binds exact catalog-ordered carry factors to one leaf.
func (p ProductPatchPlan) NewBuilder(
	carryFactors []LaneFactor,
	carryValues ValueLaneFactor,
	carryReachable bool,
) (*ProductPatchBuilder, error) {
	if !p.domain.Valid() || len(carryFactors) != len(p.carry) {
		return nil, fmt.Errorf("%w: carry factor inventory differs from plan", ErrInvalidProductPatch)
	}
	ownedCarry := make([]LaneFactor, len(carryFactors))
	for index, factor := range carryFactors {
		if _, err := p.domain.validateFactorFor(&p.domain.factorLanes[p.carry[index].ordinal], factor); err != nil || factor.lane != p.carry[index] {
			return nil, fmt.Errorf("%w: carry factor %d is foreign or reordered", ErrInvalidProductPatch, index)
		}
		ownedCarry[index] = factor
	}
	builder := &ProductPatchBuilder{
		plan: p, carryFactors: ownedCarry, carryValues: cloneValueLaneFactor(carryValues),
		carryReachable: carryReachable, reachable: carryReachable,
		writeFactors: make([]LaneFactor, len(p.writes)),
	}
	for index, lane := range p.writes {
		if carryIndex := productLaneIndex(p.carry, lane); carryIndex >= 0 {
			builder.writeFactors[index] = ownedCarry[carryIndex]
			continue
		}
		factor, err := p.domain.LaneBottom(lane)
		if err != nil {
			return nil, err
		}
		builder.writeFactors[index] = factor
	}
	if p.valuesTopWrite {
		builder.values = cloneValueLaneFactor(carryValues)
	} else {
		builder.values = ValueLaneFactor{Values: make(map[statekey.Value]product.Value, len(p.valueWrites))}
	}
	return builder, nil
}

// MaterializeInputs is the sole whole-State semantic leaf adapter. Sparse
// factor storage remains canonical on both sides of that evaluator boundary.
func (p ProductPatchPlan) MaterializeInputs(
	carryFactors []LaneFactor,
	carryValues ValueLaneFactor,
	reachable bool,
) (State, error) {
	builder, err := p.NewBuilder(carryFactors, carryValues, reachable)
	if err != nil {
		return State{}, err
	}
	residual, err := p.domain.ComposeSparse(builder.carryFactors)
	if err != nil {
		return State{}, err
	}
	out := RecomposeValueLane(p.domain.reg, p.domain.lattice, residual, builder.carryValues)
	if !reachable {
		return p.domain.lattice.Bottom(), nil
	}
	return Reachable(out), nil
}

// WriteDeclaredFragment extracts only the sealed writes from fragment. Bottom
// in every undeclared lane is omission, never an instruction to clear it.
func (b *ProductPatchBuilder) WriteDeclaredFragment(fragment State, normal bool) error {
	if b == nil || !b.plan.domain.Valid() || b.used {
		return fmt.Errorf("%w: patch builder already consumed", ErrInvalidProductPatch)
	}
	if !normal && !b.plan.reachabilityWrite {
		return fmt.Errorf("%w: evaluator terminated without reachability ownership", ErrInvalidProductPatch)
	}
	// Unreachable is the product bottom transaction. It owns only the explicit
	// reachability publication: normalization necessarily erases every product
	// axis, so comparing that Bottom spelling with carried axes would mistake
	// semantic termination for writes to each carried coordinate. The guarded
	// carrier retains arbitrary axis roots behind a false reachability root;
	// those roots are semantically unobservable and are never published as
	// evaluator writes.
	if !normal {
		b.reachable = false
		b.used = true
		return nil
	}
	fragment = b.plan.domain.Normalize(fragment)
	bottomState := b.plan.domain.lattice.Bottom()
	neutral := bottomState
	if normal {
		neutral = Reachable(neutral)
	}
	for _, runtime := range b.plan.undeclared {
		// A partial fragment may omit an undeclared lane as Bottom. State write
		// APIs may also stamp its reachable-empty spelling when normal=true;
		// reachability is a separate declared coordinate, so both are neutral.
		if runtime.ops.equalState(fragment, bottomState) || runtime.ops.equalState(fragment, neutral) {
			continue
		}
		carryIndex := productLaneIndex(b.plan.carry, runtime.lane)
		if carryIndex < 0 || !runtime.ops.equal(runtime.ops.extract(fragment), b.carryFactors[carryIndex].payload) {
			return fmt.Errorf("%w: fragment writes undeclared product lane %q", ErrInvalidProductPatch, runtime.lane.id)
		}
	}
	_, values := DecomposeValueLane(b.plan.domain.lattice, fragment)
	if !b.plan.valuesTopWrite && values.Top != b.carryValues.Top {
		return fmt.Errorf("%w: fragment writes undeclared Values Top", ErrInvalidProductPatch)
	}
	if !values.Top && !b.plan.valuesTopWrite {
		checked := make(map[statekey.Value]struct{}, len(values.Values)+len(b.carryValues.Values)+len(b.plan.valueCarry))
		check := func(slot statekey.Value) error {
			if slot == 0 || b.plan.declaresValue(slot) {
				return nil
			}
			if _, done := checked[slot]; done {
				return nil
			}
			checked[slot] = struct{}{}
			if !b.plan.valueCarryAll && !b.plan.carriesValue(slot) {
				return fmt.Errorf("%w: fragment writes undeclared Values slot %d", ErrInvalidProductPatch, slot)
			}
			if !product.Equal(b.plan.domain.reg, readValueLaneFactor(b.plan.domain, values, slot), readValueLaneFactor(b.plan.domain, b.carryValues, slot)) {
				return fmt.Errorf("%w: fragment writes undeclared Values slot %d", ErrInvalidProductPatch, slot)
			}
			return nil
		}
		for slot := range values.Values {
			if err := check(slot); err != nil {
				return err
			}
		}
		if b.plan.valueCarryAll {
			for slot := range b.carryValues.Values {
				if err := check(slot); err != nil {
					return err
				}
			}
		} else {
			for _, slot := range b.plan.valueCarry {
				if err := check(slot); err != nil {
					return err
				}
			}
		}
	}
	factors, err := b.plan.domain.DecomposeLanes(fragment, b.plan.writes)
	if err != nil {
		return err
	}
	b.writeFactors = factors
	if b.plan.valuesTopWrite {
		b.values = values
	} else {
		b.values = ValueLaneFactor{Values: make(map[statekey.Value]product.Value, len(b.plan.valueWrites))}
		// Finite point writes are absorbed by the lifted-map Top carried on
		// entry. Their canonical sparse publication is an empty finite patch;
		// the separately carried Top bit remains authoritative.
		if !values.Top {
			bottom := b.plan.domain.ValueBottom()
			for _, slot := range b.plan.valueWrites {
				value, present := values.Values[slot]
				if !present {
					value = bottom
				}
				if !product.Equal(b.plan.domain.reg, value, bottom) {
					b.values.Values[slot] = value
				}
			}
		}
	}
	if b.plan.reachabilityWrite {
		b.reachable = normal
	}
	b.used = true
	return nil
}

// ApplyBoundary applies one already-sealed boundary transaction directly to
// the declared factor carriers. It never materializes a whole State.
func (b *ProductPatchBuilder) ApplyBoundary(patch BoundaryPatch) error {
	if b == nil || b.used || !patch.valid() || patch.domain.seal != b.plan.domain.seal {
		return fmt.Errorf("%w: foreign or consumed boundary patch", ErrInvalidProductPatch)
	}
	all := b.plan.domain.NonValuesLaneInventory()
	if len(b.plan.writes) != len(all) {
		return fmt.Errorf("%w: boundary application must own every product lane", ErrInvalidProductPatch)
	}
	for index := range all {
		if b.plan.writes[index] != all[index] {
			return fmt.Errorf("%w: boundary product ownership is incomplete", ErrInvalidProductPatch)
		}
	}
	if !b.plan.valuesTopWrite {
		for slot := range patch.closure.slots {
			if !b.plan.declaresValue(slot) {
				return fmt.Errorf("%w: boundary writes undeclared Values slot %d", ErrInvalidProductPatch, slot)
			}
		}
		for _, root := range patch.rootPlan.slots {
			if !b.plan.declaresValue(root.Slot) {
				return fmt.Errorf("%w: boundary root writes undeclared Values slot %d", ErrInvalidProductPatch, root.Slot)
			}
		}
	}
	for index := range b.writeFactors {
		var factor LaneFactor
		var err error
		switch b.writeFactors[index].lane.id {
		case LaneHeapTableIdentity:
			heap, heapErr := patch.HeapFactors()
			if heapErr != nil {
				return heapErr
			}
			factor, err = heap.applyFactor(b.writeFactors[index])
		case LanePlacement:
			placements, placementErr := patch.PlacementFactors()
			if placementErr != nil {
				return placementErr
			}
			factor, err = placements.applyFactor(b.writeFactors[index])
		default:
			factor, err = patch.ApplyLane(b.writeFactors[index])
		}
		if err != nil {
			return err
		}
		b.writeFactors[index] = factor
	}
	values, err := patch.ApplyValues(b.carryValues)
	if err != nil {
		return err
	}
	if b.plan.valuesTopWrite {
		b.values = values
	} else {
		bottom := b.plan.domain.ValueBottom()
		for _, slot := range b.plan.valueWrites {
			value, present := values.Values[slot]
			if !present {
				value = bottom
			}
			if !product.Equal(b.plan.domain.reg, value, bottom) {
				b.values.Values[slot] = value
			}
		}
	}
	if patch.rootPlan.establishesReachability && !b.carryReachable {
		if !b.plan.reachabilityWrite {
			return fmt.Errorf("%w: boundary establishes undeclared reachability", ErrInvalidProductPatch)
		}
		b.reachable = true
	}
	b.used = true
	return nil
}

// Finish publishes only declared writes. The returned factors are in catalog
// order; untouched roots are structurally carried by the transformer.
func (b *ProductPatchBuilder) Finish() ([]LaneFactor, ValueLaneFactor, bool, error) {
	if b == nil || !b.plan.domain.Valid() || !b.used {
		return nil, ValueLaneFactor{}, false, fmt.Errorf("%w: unfinished product patch", ErrInvalidProductPatch)
	}
	return append([]LaneFactor(nil), b.writeFactors...), cloneValueLaneFactor(b.values), b.reachable, nil
}

func (p ProductPatchPlan) declaresValue(slot statekey.Value) bool {
	index := sort.Search(len(p.valueWrites), func(index int) bool { return p.valueWrites[index] >= slot })
	return index < len(p.valueWrites) && p.valueWrites[index] == slot
}

func (p ProductPatchPlan) carriesValue(slot statekey.Value) bool {
	index := sort.Search(len(p.valueCarry), func(index int) bool { return p.valueCarry[index] >= slot })
	return index < len(p.valueCarry) && p.valueCarry[index] == slot
}

func readValueLaneFactor(domain ProductDomain, factor ValueLaneFactor, slot statekey.Value) product.Value {
	if factor.Top {
		return domain.ValueTop()
	}
	if value, ok := factor.Values[slot]; ok {
		return value
	}
	return domain.ValueBottom()
}

func productLaneIndex(lanes []ProductLane, target ProductLane) int {
	index := sort.Search(len(lanes), func(index int) bool { return lanes[index].ordinal >= target.ordinal })
	if index < len(lanes) && lanes[index] == target {
		return index
	}
	return -1
}

func cloneValueLaneFactor(in ValueLaneFactor) ValueLaneFactor {
	out := ValueLaneFactor{Top: in.Top}
	if len(in.Values) != 0 {
		out.Values = make(map[statekey.Value]product.Value, len(in.Values))
		for slot, value := range in.Values {
			out.Values[slot] = value
		}
	}
	return out
}
