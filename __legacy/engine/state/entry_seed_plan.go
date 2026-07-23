package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// EntrySeedPlan is an immutable prepared transaction for the declared values
// of one lexical body. Apply has the same missing-only semantics as SeedValues:
// route-supplied params, captures, globals, and other concrete values retain
// precedence over prepared defaults.
//
// The plan deliberately owns no reachability, lattice normalization, sparse
// point seeds, or call provider. Those policies belong to the invocation
// scheduler that admits the resulting State.
type EntrySeedPlan struct {
	seeds    []ValueSeed
	prepared bool
}

// EntrySeedFactorPlan is the vocabulary-bound form of EntrySeedPlan.  It is
// the sole missing-only law for concrete Values and for every symbolic Values
// carrier: callers bind their address vocabulary once, then apply exactly the
// same product-value transaction without reconstructing State.
//
// K is address syntax only.  The product lattice and duplicate/ordering
// semantics remain owned by EntrySeedPlan.
type EntrySeedFactorPlan[K comparable] struct {
	coordinates  []entrySeedCoordinate[K]
	declarations int
	prepared     bool
}

type entrySeedCoordinate[K comparable] struct {
	slot  K
	seeds []product.Value
}

// BindEntrySeedFactorPlan seals one collision-free address projection.  A
// missing mapping is an error rather than an omitted default: the producer's
// finite seed inventory must be represented exactly in the destination
// vocabulary.  Duplicate source slots retain source order and therefore the
// historical missing-only semantics.
func BindEntrySeedFactorPlan[K comparable](
	reg *axis.Registry,
	plan EntrySeedPlan,
	bind func(key.Value) (K, bool),
) (EntrySeedFactorPlan[K], error) {
	if reg == nil || !plan.Valid() || bind == nil {
		return EntrySeedFactorPlan[K]{}, fmt.Errorf("state: entry-seed factor plan is unowned")
	}
	out := EntrySeedFactorPlan[K]{prepared: true, coordinates: make([]entrySeedCoordinate[K], 0, len(plan.seeds))}
	bySource := make(map[key.Value]K, len(plan.seeds))
	byTarget := make(map[K]key.Value, len(plan.seeds))
	coordinateIndex := make(map[K]int, len(plan.seeds))
	for index, seed := range plan.seeds {
		if seed.Slot == 0 {
			continue
		}
		if !product.BelongsToRegistry(reg, seed.Value) {
			return EntrySeedFactorPlan[K]{}, fmt.Errorf("state: entry-seed factor %d contains a foreign product", index)
		}
		target, ok := bind(seed.Slot)
		if !ok {
			return EntrySeedFactorPlan[K]{}, fmt.Errorf("state: entry-seed factor %d source %d has no destination address", index, seed.Slot)
		}
		if prior, present := bySource[seed.Slot]; present && prior != target {
			return EntrySeedFactorPlan[K]{}, fmt.Errorf("state: entry-seed source %d has inconsistent destinations", seed.Slot)
		}
		if source, present := byTarget[target]; present && source != seed.Slot {
			return EntrySeedFactorPlan[K]{}, fmt.Errorf("state: entry-seed destination aliases sources %d and %d", source, seed.Slot)
		}
		bySource[seed.Slot], byTarget[target] = target, seed.Slot
		coordinate, present := coordinateIndex[target]
		if !present {
			coordinate = len(out.coordinates)
			coordinateIndex[target] = coordinate
			out.coordinates = append(out.coordinates, entrySeedCoordinate[K]{slot: target})
		}
		out.coordinates[coordinate].seeds = append(out.coordinates[coordinate].seeds, seed.Value)
		out.declarations++
	}
	return out, nil
}

// Valid reports whether this factor plan was sealed from an EntrySeedPlan.
// A valid plan may be empty.
func (p EntrySeedFactorPlan[K]) Valid() bool { return p.prepared }

// Len reports the number of non-zero seed declarations, including deliberate
// duplicate declarations whose order is semantically observable.
func (p EntrySeedFactorPlan[K]) Len() int { return p.declarations }

// Slots returns the finite destination inventory without duplicates.  The
// result preserves declaration order; carrier adapters may reorder it only by
// their own sealed catalog (for example a formal fiber ordinal).
func (p EntrySeedFactorPlan[K]) Slots() []K {
	if !p.Valid() || len(p.coordinates) == 0 {
		return nil
	}
	out := make([]K, len(p.coordinates))
	for index, coordinate := range p.coordinates {
		out[index] = coordinate.slot
	}
	return out
}

// Apply performs the canonical missing-only Values transaction.  Top is a
// fixed point; finite maps are copied only when a non-Bottom seed fills a
// Bottom coordinate.  Bottom entries are omitted from the returned canonical
// spelling.
func (p EntrySeedFactorPlan[K]) Apply(reg *axis.Registry, input ValueFactor[K]) (ValueFactor[K], error) {
	if reg == nil || !p.Valid() || input.Top && len(input.Values) != 0 {
		return ValueFactor[K]{}, fmt.Errorf("state: entry-seed Values transaction is malformed")
	}
	for _, value := range input.Values {
		if !product.BelongsToRegistry(reg, value) {
			return ValueFactor[K]{}, fmt.Errorf("state: entry-seed Values input contains a foreign product")
		}
	}
	if input.Top || len(p.coordinates) == 0 {
		return input, nil
	}
	out := input
	cloned := false
	for _, coordinate := range p.coordinates {
		current, present := out.Values[coordinate.slot]
		if !present {
			current = product.Bottom(reg)
		}
		next, err := applyEntrySeedCoordinate(reg, current, coordinate.seeds)
		if err != nil {
			return ValueFactor[K]{}, err
		}
		if product.Equal(reg, next, product.Bottom(reg)) {
			if present {
				if !cloned {
					out.Values = cloneEntrySeedValues(out.Values)
					cloned = true
				}
				delete(out.Values, coordinate.slot)
			}
			continue
		}
		if product.Equal(reg, current, next) {
			continue
		}
		if !cloned {
			out.Values = cloneEntrySeedValues(out.Values)
			cloned = true
		}
		if out.Values == nil {
			out.Values = make(map[K]product.Value)
		}
		out.Values[coordinate.slot] = next
	}
	return out, nil
}

// ApplyValue is the sparse-coordinate view of Apply.  It exists for DD
// carriers which partition only the seed coordinates they own; the result is
// exactly the coordinate projection of Apply on the same factor.
func (p EntrySeedFactorPlan[K]) ApplyValue(reg *axis.Registry, slot K, current product.Value) (product.Value, error) {
	if reg == nil || !p.Valid() || !product.BelongsToRegistry(reg, current) {
		return product.Value{}, fmt.Errorf("state: entry-seed Values coordinate is malformed")
	}
	for _, coordinate := range p.coordinates {
		if coordinate.slot == slot {
			return applyEntrySeedCoordinate(reg, current, coordinate.seeds)
		}
	}
	return current, nil
}

// applyEntrySeedCoordinate is the sole missing-only semantic kernel.  Full
// finite-map application and sparse DD application are only two projections
// of this one ordered coordinate law.
func applyEntrySeedCoordinate(reg *axis.Registry, current product.Value, seeds []product.Value) (product.Value, error) {
	if reg == nil || !product.BelongsToRegistry(reg, current) {
		return product.Value{}, fmt.Errorf("state: entry-seed Values coordinate is malformed")
	}
	bottom := product.Bottom(reg)
	out := current
	for _, seed := range seeds {
		if !product.BelongsToRegistry(reg, seed) {
			return product.Value{}, fmt.Errorf("state: entry-seed Values coordinate contains a foreign seed")
		}
		if product.Equal(reg, out, bottom) {
			out = seed
		}
	}
	return out, nil
}

func cloneEntrySeedValues[K comparable](input map[K]product.Value) map[K]product.Value {
	if len(input) == 0 {
		return nil
	}
	out := make(map[K]product.Value, len(input))
	for slot, value := range input {
		out[slot] = value
	}
	return out
}

// NewEntrySeedPlan detaches seeds from preparation-owned storage.
func NewEntrySeedPlan(seeds []ValueSeed) EntrySeedPlan {
	return EntrySeedPlan{seeds: append([]ValueSeed(nil), seeds...), prepared: true}
}

// Clone returns a detached plan with identical missing-only semantics.
func (p EntrySeedPlan) Clone() EntrySeedPlan {
	if !p.prepared {
		return EntrySeedPlan{}
	}
	return NewEntrySeedPlan(p.seeds)
}

// Valid reports whether the plan was minted from a prepared lexical body. A
// valid plan may be empty; the zero value denotes missing seed authority.
func (p EntrySeedPlan) Valid() bool {
	return p.prepared
}

// Len reports the number of prepared slot defaults.
func (p EntrySeedPlan) Len() int {
	return len(p.seeds)
}

// Empty reports whether Apply is an identity transaction.
func (p EntrySeedPlan) Empty() bool {
	return len(p.seeds) == 0
}

// Slots returns the exact finite Values inventory this prepared transaction
// may fill. The detached result is sorted and deduplicated so access planners
// can seal write ownership without depending on seed construction order.
func (p EntrySeedPlan) Slots() []key.Value {
	if !p.prepared || len(p.seeds) == 0 {
		return nil
	}
	slots := make([]key.Value, 0, len(p.seeds))
	for _, seed := range p.seeds {
		if seed.Slot != 0 {
			slots = append(slots, seed.Slot)
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i] < slots[j] })
	out := slots[:0]
	for _, slot := range slots {
		if len(out) == 0 || out[len(out)-1] != slot {
			out = append(out, slot)
		}
	}
	return out
}

// ValueForSlot returns the exact prepared default for slot. Definition-frame
// construction uses this same immutable authority as Apply, so declared and
// contextual parameter contracts cannot drift into a second plan-owned value.
// The first seed owns a duplicate slot, matching SeedValues' missing-only
// application order.
func (p EntrySeedPlan) ValueForSlot(slot key.Value) (product.Value, bool) {
	if !p.prepared || slot == 0 {
		return product.Value{}, false
	}
	for _, seed := range p.seeds {
		if seed.Slot == slot {
			return seed.Value, true
		}
	}
	return product.Value{}, false
}

// ValuesForSlots returns prepared values in the caller's exact slot order.
// It is the shared contract projection for consumers that must bind a tuple
// from the same immutable authority as Apply; a missing or zero slot fails
// closed instead of manufacturing a parallel default.
func (p EntrySeedPlan) ValuesForSlots(slots []key.Value) ([]product.Value, bool) {
	if !p.prepared {
		return nil, false
	}
	values := make([]product.Value, len(slots))
	for index, slot := range slots {
		value, ok := p.ValueForSlot(slot)
		if !ok {
			return nil, false
		}
		values[index] = value
	}
	return values, true
}

// Refine returns a detached plan with exact slot refinements installed. Every
// refinement must target an existing seed and be below its prepared value in
// the product lattice; this prevents contextual typing from widening or
// manufacturing a new entry namespace after body preparation.
func (p EntrySeedPlan) Refine(reg *axis.Registry, refinements []ValueSeed) (EntrySeedPlan, bool) {
	if !p.prepared || reg == nil {
		return EntrySeedPlan{}, false
	}
	out := p.Clone()
	for _, refinement := range refinements {
		if refinement.Slot == 0 || !product.BelongsToRegistry(reg, refinement.Value) {
			return EntrySeedPlan{}, false
		}
		matched := false
		for index := range out.seeds {
			if out.seeds[index].Slot != refinement.Slot {
				continue
			}
			current := out.seeds[index].Value
			if !product.LessOrEq(reg, refinement.Value, current) && !product.Get(reg, current, evidence.Key).IsGradualTop() {
				return EntrySeedPlan{}, false
			}
			out.seeds[index].Value = refinement.Value
			matched = true
			break
		}
		if !matched {
			return EntrySeedPlan{}, false
		}
	}
	return out, true
}

// Apply fills only value slots that are Bottom in entry.  The error-returning
// contract is deliberate: EntrySeedPlan is prepared before it is bound to an
// axis registry, so foreign products cannot be proven impossible until this
// exact application boundary.
func (p EntrySeedPlan) Apply(reg *axis.Registry, entry State) (State, error) {
	if !p.prepared {
		return entry, fmt.Errorf("state: entry-seed plan is unprepared")
	}
	return applyEntrySeedPlanToState(reg, p, entry)
}

func applyEntrySeedPlanToState(reg *axis.Registry, p EntrySeedPlan, entry State) (State, error) {
	if reg == nil || !p.Valid() || p.Empty() || !entry.laneEnabled(laneValuesBit) || entry.values.top {
		if reg == nil || !p.Valid() {
			return entry, fmt.Errorf("state: entry-seed State transaction is unowned")
		}
		return entry, nil
	}
	plan, err := BindEntrySeedFactorPlan(reg, p, func(slot key.Value) (key.Value, bool) { return slot, slot != 0 })
	if err != nil {
		return entry, err
	}
	values := valueLaneToFactor(entry.values)
	next, err := plan.Apply(reg, values)
	if err != nil {
		return entry, err
	}
	if ValueFactorLattice[key.Value](reg).Equal(values, next) {
		return entry, nil
	}
	out := entry.reachable()
	out.values = valueLaneFromFactor(reg, next)
	return out, nil
}
