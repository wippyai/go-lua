package state

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// BoundaryValueSlotBinding is one structural source-to-target address edge.
// Slot syntax is deliberately generic: concrete State and formal relation
// carriers share this relation without manufacturing statekey.Value aliases.
type BoundaryValueSlotBinding[S, T comparable] struct {
	Source S
	Target T
}

// BoundaryValueSlotRelation is a sealed finite relation. sourceOrder is the
// caller's canonical slot inventory and owns collision fold order; bindings
// may be supplied in any order without changing the result.
type BoundaryValueSlotRelation[S, T comparable] struct {
	sourceOrder []S
	targetOrder []T
	targets     map[S][]T
	sealed      bool
}

// SealBoundaryValueSlotRelation separates structural slot identity from value
// transport. Source and target orders must contain each admitted address
// exactly once; they make contribution and collision spelling independent of
// binding/map iteration. A source absent from the relation is omitted, never
// synthesized as Bottom.
func SealBoundaryValueSlotRelation[S, T comparable](sourceOrder []S, targetOrder []T, bindings []BoundaryValueSlotBinding[S, T]) (BoundaryValueSlotRelation[S, T], error) {
	relation := BoundaryValueSlotRelation[S, T]{
		sourceOrder: append([]S(nil), sourceOrder...),
		targetOrder: append([]T(nil), targetOrder...),
		targets:     make(map[S][]T, len(sourceOrder)),
	}
	admitted := make(map[S]struct{}, len(sourceOrder))
	for _, source := range sourceOrder {
		if _, duplicate := admitted[source]; duplicate {
			return BoundaryValueSlotRelation[S, T]{}, fmt.Errorf("state: Values boundary slot order contains a duplicate source")
		}
		admitted[source] = struct{}{}
	}
	targetRanks := make(map[T]int, len(targetOrder))
	for rank, target := range targetOrder {
		if _, duplicate := targetRanks[target]; duplicate {
			return BoundaryValueSlotRelation[S, T]{}, fmt.Errorf("state: Values boundary slot order contains a duplicate target")
		}
		targetRanks[target] = rank
	}
	seenTargets := make(map[S]map[T]struct{}, len(sourceOrder))
	for _, binding := range bindings {
		if _, ok := admitted[binding.Source]; !ok {
			return BoundaryValueSlotRelation[S, T]{}, fmt.Errorf("state: Values boundary binding has an undeclared source")
		}
		if _, ok := targetRanks[binding.Target]; !ok {
			return BoundaryValueSlotRelation[S, T]{}, fmt.Errorf("state: Values boundary binding has an undeclared target")
		}
		seen := seenTargets[binding.Source]
		if seen == nil {
			seen = make(map[T]struct{})
			seenTargets[binding.Source] = seen
		}
		if _, duplicate := seen[binding.Target]; duplicate {
			continue
		}
		seen[binding.Target] = struct{}{}
		relation.targets[binding.Source] = append(relation.targets[binding.Source], binding.Target)
	}
	for source, targets := range relation.targets {
		sort.Slice(targets, func(left, right int) bool { return targetRanks[targets[left]] < targetRanks[targets[right]] })
		relation.targets[source] = targets
	}
	relation.sealed = true
	return relation, nil
}

// ApplyBoundary performs the canonical Values Project/Rebase/Apply/root-write
// transaction for an arbitrary collision-free slot vocabulary.  Relation
// targets are cleared before the transported fragment is installed, and root
// writes are ordered last exactly like applyValuesBoundaryLane followed by
// applyValuesBoundaryRoots.  No concrete State slot is manufactured.
func (p BoundaryValueFactorTransport[S, T]) ApplyBoundary(
	destination ValueFactor[T],
	source ValueFactor[S],
	roots []BoundaryValueSlotContribution[T],
) (ValueFactor[T], error) {
	if !p.valid() || destination.Top && len(destination.Values) != 0 || source.Top && len(source.Values) != 0 {
		return ValueFactor[T]{}, fmt.Errorf("%w: Values boundary transaction is malformed", ErrInvalidLaneFactor)
	}
	fragment, err := p.Apply(source)
	if err != nil {
		return ValueFactor[T]{}, err
	}
	if destination.Top || fragment.Top {
		return ValueFactor[T]{Top: true}, nil
	}
	bottom := product.Bottom(p.domain.reg)
	out := make(map[T]product.Value, len(destination.Values)+len(fragment.Values)+len(roots))
	for slot, value := range destination.Values {
		if !product.BelongsToRegistry(p.domain.reg, value) {
			return ValueFactor[T]{}, fmt.Errorf("%w: Values destination contains a foreign product", ErrInvalidLaneFactor)
		}
		out[slot] = value
	}
	for _, slot := range p.relation.targetOrder {
		delete(out, slot)
	}
	for slot, value := range fragment.Values {
		if product.Equal(p.domain.reg, value, bottom) {
			delete(out, slot)
		} else {
			out[slot] = value
		}
	}
	for index, root := range roots {
		if !product.BelongsToRegistry(p.domain.reg, root.Value) {
			return ValueFactor[T]{}, fmt.Errorf("%w: Values root %d contains a foreign product", ErrInvalidLaneFactor, index)
		}
		if product.Equal(p.domain.reg, root.Value, bottom) {
			delete(out, root.Slot)
		} else {
			out[root.Slot] = root.Value
		}
	}
	if len(out) == 0 {
		return ValueFactor[T]{}, nil
	}
	return ValueFactor[T]{Values: out}, nil
}

func (r BoundaryValueSlotRelation[S, T]) valid() bool {
	return r.sealed && r.targets != nil
}

// BoundaryValueSlotContribution is one already-projected and identity-rebased
// scalar contribution. Multiple sources may contribute to the same target;
// Apply folds them through the registered product.Value Join.
type BoundaryValueSlotContribution[T comparable] struct {
	Slot  T
	Value product.Value
}

// BoundaryValueFactorTransport binds a generic structural slot relation to
// the canonical boundary product-value rebase authority. It retains no State,
// callback, transformer type, or per-slot compiled program.
type BoundaryValueFactorTransport[S, T comparable] struct {
	owner    *boundaryFactorTransportSeal
	domain   ProductDomain
	rebase   boundaryRebaseContext
	relation BoundaryValueSlotRelation[S, T]
	sealed   bool
}

// PrepareBoundaryValueFactorTransport binds a pre-sealed relation to one
// boundary plan. The plan remains the sole ProjectBoundary + identity/
// allocation rebase semantics for both concrete and formal Values carriers.
func PrepareBoundaryValueFactorTransport[S, T comparable](plan BoundaryFactorTransportPlan, relation BoundaryValueSlotRelation[S, T]) (BoundaryValueFactorTransport[S, T], error) {
	if plan.seal == nil || !plan.domain.Valid() || plan.rebaseCtx.reg != plan.domain.reg || plan.rebaseCtx.allocations == nil || !relation.valid() {
		return BoundaryValueFactorTransport[S, T]{}, fmt.Errorf("%w: Values factor transport is unowned", ErrInvalidLaneFactor)
	}
	return BoundaryValueFactorTransport[S, T]{owner: plan.seal, domain: plan.domain, rebase: plan.rebaseCtx, relation: relation, sealed: true}, nil
}

func (p BoundaryValueFactorTransport[S, T]) valid() bool {
	return p.sealed && p.owner != nil && p.domain.Valid() && p.rebase.reg == p.domain.reg && p.relation.valid()
}

// RebaseSlot applies the unary value law once and fans the result out through
// the sealed structural relation. Missing sources are omitted exactly.
func (p BoundaryValueFactorTransport[S, T]) RebaseSlot(source S, value product.Value) ([]BoundaryValueSlotContribution[T], error) {
	if !p.valid() || !product.BelongsToRegistry(p.domain.reg, value) {
		return nil, fmt.Errorf("%w: invalid Values boundary source coordinate", ErrInvalidLaneFactor)
	}
	targets := p.relation.targets[source]
	if len(targets) == 0 {
		return nil, nil
	}
	rebased, err := rebaseBoundarySlotProduct(&p.rebase, value)
	if err != nil {
		return nil, err
	}
	out := make([]BoundaryValueSlotContribution[T], len(targets))
	for index, target := range targets {
		out[index] = BoundaryValueSlotContribution[T]{Slot: target, Value: rebased}
	}
	return out, nil
}

// Apply transports one complete Values factor. Top and Bottom remain exact;
// finite many-to-one collisions use the registered product lattice Join in
// sealed source order, independent of map or binding iteration order.
func (p BoundaryValueFactorTransport[S, T]) Apply(source ValueFactor[S]) (ValueFactor[T], error) {
	if !p.valid() {
		return ValueFactor[T]{}, fmt.Errorf("%w: Values factor transport is unowned", ErrInvalidLaneFactor)
	}
	if source.Top {
		return ValueFactor[T]{Top: true}, nil
	}
	for _, value := range source.Values {
		if !product.BelongsToRegistry(p.domain.reg, value) {
			return ValueFactor[T]{}, fmt.Errorf("%w: Values factor contains a foreign product value", ErrInvalidLaneFactor)
		}
	}
	if len(source.Values) == 0 {
		return ValueFactor[T]{}, nil
	}
	domain := product.Domain(p.domain.reg)
	bottom := product.Bottom(p.domain.reg)
	out := make(map[T]product.Value)
	for _, sourceSlot := range p.relation.sourceOrder {
		value, present := source.Values[sourceSlot]
		if !present {
			continue
		}
		rebased, err := rebaseBoundarySlotProduct(&p.rebase, value)
		if err != nil {
			return ValueFactor[T]{}, err
		}
		for _, target := range p.relation.targets[sourceSlot] {
			candidate := rebased
			if prior, collision := out[target]; collision {
				candidate = domain.Join(prior, candidate)
			}
			if domain.Equal(candidate, bottom) {
				delete(out, target)
			} else {
				out[target] = candidate
			}
		}
	}
	if len(out) == 0 {
		return ValueFactor[T]{}, nil
	}
	return ValueFactor[T]{Values: out}, nil
}

func rebaseBoundarySlotProduct(ctx *boundaryRebaseContext, value product.Value) (product.Value, error) {
	if ctx == nil || ctx.reg == nil || !product.BelongsToRegistry(ctx.reg, value) {
		return product.Value{}, fmt.Errorf("%w: invalid Values boundary scalar", ErrInvalidLaneFactor)
	}
	value = product.ProjectBoundary(ctx.reg, value)
	rebased, ok := rebaseBoundaryProduct(ctx, value)
	if !ok {
		return product.Value{}, fmt.Errorf("state: Values boundary scalar rebase failed")
	}
	return rebased, nil
}

func sealConcreteBoundaryValueSlotRelation(project boundaryProjectContext, rebase boundaryRebaseContext) (BoundaryValueSlotRelation[statekey.Value, statekey.Value], error) {
	if project.reg == nil || rebase.reg != project.reg || project.closure.slots == nil {
		return BoundaryValueSlotRelation[statekey.Value, statekey.Value]{}, fmt.Errorf("%w: concrete Values slot relation is unowned", ErrInvalidLaneFactor)
	}
	sources := make([]statekey.Value, 0, len(project.closure.slots))
	for source := range project.closure.slots {
		if source == 0 {
			return BoundaryValueSlotRelation[statekey.Value, statekey.Value]{}, fmt.Errorf("%w: concrete Values slot relation contains zero", ErrInvalidLaneFactor)
		}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(left, right int) bool { return sources[left] < sources[right] })
	bindings := make([]BoundaryValueSlotBinding[statekey.Value, statekey.Value], 0, len(sources))
	for _, source := range sources {
		targets, ok := boundaryRebaseSlots(&rebase, source)
		if !ok {
			return BoundaryValueSlotRelation[statekey.Value, statekey.Value]{}, fmt.Errorf("state: Values boundary slot rebase failed")
		}
		for _, target := range targets {
			bindings = append(bindings, BoundaryValueSlotBinding[statekey.Value, statekey.Value]{Source: source, Target: target})
		}
	}
	targetSet := make(map[statekey.Value]struct{})
	for _, binding := range bindings {
		targetSet[binding.Target] = struct{}{}
	}
	targets := make([]statekey.Value, 0, len(targetSet))
	for target := range targetSet {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(left, right int) bool { return targets[left] < targets[right] })
	return SealBoundaryValueSlotRelation(sources, targets, bindings)
}
