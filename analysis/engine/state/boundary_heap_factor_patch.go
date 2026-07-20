package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// BoundaryFactorSource identifies the one operand that owns an output keyed
// coordinate after boundary replacement.  Boundary application never joins
// the two operands: identities in the projected closure are replaced by the
// transported fragment and every other identity is retained from the
// destination.
type BoundaryFactorSource uint8

const (
	BoundaryFactorSourceInvalid BoundaryFactorSource = iota
	BoundaryFactorSourceDestination
	BoundaryFactorSourceFragment
)

// HeapBoundaryPatch is the heap-coordinate transpose of one already
// projected and rebased BoundaryPatch.  Its fragment skeleton, roots, and
// members remain independently owned; no opaque HeapTableIdentity lane can
// escape through this API.
type HeapBoundaryPatch struct {
	domain   ProductDomain
	keys     *keyspace.KeySpace
	closure  BoundaryClosure
	skeleton HeapTableIdentitySkeletonFactor
	roots    map[identity.Term]HeapObjectRootFactor
	members  map[heapBoundaryMemberCoordinate]HeapStaticMemberFactor
	// fragmentRootSources/fragmentMemberSources are the original, pre-project
	// and pre-rebase scalar coordinates selected by a factorwise transport.
	// Multiple sources denote one exact quotient collision and are joined only
	// after their guarded scalar roots have been mapped independently.
	fragmentRootSources   map[identity.Term][]HeapObjectRootSlot
	fragmentMemberSources map[heapBoundaryMemberCoordinate][]HeapStaticMemberSlot
	mapFragmentValue      func(product.Value) (product.Value, error)
}

type heapBoundaryMemberCoordinate struct {
	id  identity.Term
	key keyspace.Key
}

// HeapBoundaryPlan is the exact structural result of applying a transported
// heap fragment to one destination skeleton.  Values are not aligned or read
// while the plan is built.  Each output root/member names exactly one source
// coordinate, allowing guarded evaluators to route its decision root without
// constructing a Cartesian product of unrelated heap values.
type HeapBoundaryPlan struct {
	patch    *HeapBoundaryPatch
	skeleton HeapTableIdentitySkeletonFactor
	roots    []HeapBoundaryRootSelection
	members  []HeapBoundaryMemberSelection
}

// HeapBoundaryRootSelection routes one output object-root coordinate.
type HeapBoundaryRootSelection struct {
	slot            HeapObjectRootSlot
	source          BoundaryFactorSource
	fragment        HeapObjectRootFactor
	fragmentSources []HeapObjectRootSlot
}

// HeapBoundaryMemberSelection routes one output static-member coordinate.
type HeapBoundaryMemberSelection struct {
	slot            HeapStaticMemberSlot
	source          BoundaryFactorSource
	fragment        HeapStaticMemberFactor
	fragmentSources []HeapStaticMemberSlot
}

// HeapFactors seals the independently factored heap fragment carried by this
// boundary patch.  Projection and rebasing have already happened; Plan owns
// the remaining ApplyBoundary replacement law.
func (p BoundaryPatch) HeapFactors() (HeapBoundaryPatch, error) {
	if !p.valid() {
		return HeapBoundaryPatch{}, fmt.Errorf("%w: invalid boundary patch", ErrInvalidLaneFactor)
	}
	runtime, ok := p.domain.runtimeForLaneID(LaneHeapTableIdentity)
	if !ok {
		return HeapBoundaryPatch{}, fmt.Errorf("%w: product has no heap-identity lane", ErrInvalidLaneFactor)
	}
	index := int(runtime.lane.ordinal)
	if index < 0 || index >= len(p.lanes) || p.lanes[index].lane != runtime.lane {
		return HeapBoundaryPatch{}, fmt.Errorf("%w: boundary heap inventory drift", ErrInvalidLaneFactor)
	}
	skeleton, roots, members, err := p.domain.DecomposeHeapTableIdentity(p.lanes[index].fragment, p.keys)
	if err != nil {
		return HeapBoundaryPatch{}, err
	}
	out := HeapBoundaryPatch{
		domain: p.domain, keys: p.keys, closure: p.closure, skeleton: skeleton,
		roots:   make(map[identity.Term]HeapObjectRootFactor, len(roots)),
		members: make(map[heapBoundaryMemberCoordinate]HeapStaticMemberFactor, len(members)),
	}
	for _, root := range roots {
		out.roots[root.id] = root
	}
	for _, member := range members {
		out.members[heapBoundaryMemberCoordinate{id: member.id, key: member.key}] = member
	}
	return out, nil
}

// Plan applies finite-map replacement to metadata only and emits the exact
// source of each independently variable output coordinate.
func (p HeapBoundaryPatch) Plan(destination HeapTableIdentitySkeletonFactor) (HeapBoundaryPlan, error) {
	if !p.domain.Valid() || p.keys == nil || !p.keys.Valid() || p.skeleton.seal == nil {
		return HeapBoundaryPlan{}, fmt.Errorf("%w: invalid heap boundary patch", ErrInvalidLaneFactor)
	}
	if _, err := p.domain.validateHeapTableIdentitySkeleton(destination, p.keys); err != nil {
		return HeapBoundaryPlan{}, err
	}
	if _, err := p.domain.validateHeapTableIdentitySkeleton(p.skeleton, p.keys); err != nil {
		return HeapBoundaryPlan{}, err
	}

	output := destination
	if destination.top || p.skeleton.top {
		output = HeapTableIdentitySkeletonFactor{seal: p.domain.seal, lane: p.skeleton.lane, keys: p.keys, top: true}
	} else {
		equal := func(left, right heapTableIdentityObjectSkeleton) bool {
			return p.domain.heapObjectSkeletonLessOrEq(p.keys, left, right) &&
				p.domain.heapObjectSkeletonLessOrEq(p.keys, right, left)
		}
		output.objects = applyFiniteMapEqual(destination.objects, p.skeleton.objects,
			func(id identity.Term, _ heapTableIdentityObjectSkeleton) bool {
				return p.closure.ContainsIdentityTerm(id)
			}, equal)
	}

	plan := HeapBoundaryPlan{patch: &p, skeleton: output}
	if output.top {
		return plan, nil
	}
	for _, id := range sortedHeapSkeletonIdentities(output.objects) {
		object := output.objects[id]
		if object.bottom {
			continue
		}
		source := BoundaryFactorSourceDestination
		if p.closure.ContainsIdentityTerm(id) {
			source = BoundaryFactorSourceFragment
		}
		rootSelection := HeapBoundaryRootSelection{
			slot:   HeapObjectRootSlot{seal: p.domain.seal, lane: p.skeleton.lane, keys: p.keys, id: id},
			source: source,
		}
		if source == BoundaryFactorSourceFragment {
			fragment, hasFactor := p.roots[id]
			sources := p.fragmentRootSources[id]
			if !hasFactor && len(sources) == 0 {
				return HeapBoundaryPlan{}, fmt.Errorf("%w: transported heap object %v has no root", ErrInvalidLaneFactor, id)
			}
			rootSelection.fragment, rootSelection.fragmentSources = fragment, append([]HeapObjectRootSlot(nil), sources...)
		}
		plan.roots = append(plan.roots, rootSelection)
		for _, memberKey := range object.staticKeys {
			memberSelection := HeapBoundaryMemberSelection{
				slot:   HeapStaticMemberSlot{seal: p.domain.seal, lane: p.skeleton.lane, keys: p.keys, id: id, key: memberKey},
				source: source,
			}
			if source == BoundaryFactorSourceFragment {
				coordinate := heapBoundaryMemberCoordinate{id: id, key: memberKey}
				fragment, hasFactor := p.members[coordinate]
				sources := p.fragmentMemberSources[coordinate]
				if !hasFactor && len(sources) == 0 {
					return HeapBoundaryPlan{}, fmt.Errorf("%w: transported heap member %v/%v is absent", ErrInvalidLaneFactor, id, memberKey)
				}
				memberSelection.fragment, memberSelection.fragmentSources = fragment, append([]HeapStaticMemberSlot(nil), sources...)
			}
			plan.members = append(plan.members, memberSelection)
		}
	}
	sort.Slice(plan.members, func(i, j int) bool {
		left, right := plan.members[i].slot, plan.members[j].slot
		if left.id != right.id {
			return identityTermLess(left.id, right.id)
		}
		return p.keys.Less(left.key, right.key)
	})
	return plan, nil
}

// Skeleton returns the exact output metadata/key-presence coordinate.
func (p HeapBoundaryPlan) Skeleton() HeapTableIdentitySkeletonFactor { return p.skeleton }

// Roots returns the sorted object-root routing inventory.
func (p HeapBoundaryPlan) Roots() []HeapBoundaryRootSelection {
	return append([]HeapBoundaryRootSelection(nil), p.roots...)
}

// Members returns the identity/key-sorted static-member routing inventory.
func (p HeapBoundaryPlan) Members() []HeapBoundaryMemberSelection {
	return append([]HeapBoundaryMemberSelection(nil), p.members...)
}

// Slot returns the output object-root coordinate.
func (s HeapBoundaryRootSelection) Slot() HeapObjectRootSlot { return s.slot }

// Source reports which operand owns the output value.
func (s HeapBoundaryRootSelection) Source() BoundaryFactorSource { return s.source }

// Fragment returns the transported factor when Source is Fragment.
func (s HeapBoundaryRootSelection) Fragment() (HeapObjectRootFactor, bool) {
	return s.fragment, s.source == BoundaryFactorSourceFragment && s.fragment.seal != nil
}

// FragmentSources returns the sorted original source coordinates whose mapped
// scalar values must be joined for this output root.
func (s HeapBoundaryRootSelection) FragmentSources() []HeapObjectRootSlot {
	return append([]HeapObjectRootSlot(nil), s.fragmentSources...)
}

// Slot returns the output static-member coordinate.
func (s HeapBoundaryMemberSelection) Slot() HeapStaticMemberSlot { return s.slot }

// Source reports which operand owns the output value.
func (s HeapBoundaryMemberSelection) Source() BoundaryFactorSource { return s.source }

// Fragment returns the transported factor when Source is Fragment.
func (s HeapBoundaryMemberSelection) Fragment() (HeapStaticMemberFactor, bool) {
	return s.fragment, s.source == BoundaryFactorSourceFragment && s.fragment.seal != nil
}

// FragmentSources returns the sorted original source coordinates whose mapped
// scalar values must be joined for this output member.
func (s HeapBoundaryMemberSelection) FragmentSources() []HeapStaticMemberSlot {
	return append([]HeapStaticMemberSlot(nil), s.fragmentSources...)
}

// MapFragmentValue applies the sole concrete boundary scalar law used by the
// factorwise Project+Rebase plan: boundary projection followed by allocation
// identity substitution. Concrete already-transported patches use identity.
func (p HeapBoundaryPlan) MapFragmentValue(value product.Value) (product.Value, error) {
	if p.patch == nil || !product.BelongsToRegistry(p.patch.domain.reg, value) {
		return product.Value{}, fmt.Errorf("%w: invalid heap boundary scalar", ErrInvalidLaneFactor)
	}
	if p.patch.mapFragmentValue == nil {
		return value, nil
	}
	return p.patch.mapFragmentValue(value)
}

// applyFactor is the state package's compatibility adapter for transactions
// that already own a concrete heap lane (not a guarded coordinate graph).  It
// executes the same public factor plan by structural coordinate lookup.  The
// generic BoundaryPatch.ApplyLane surface deliberately cannot reach it.
func (p HeapBoundaryPatch) applyFactor(destination LaneFactor) (LaneFactor, error) {
	skeleton, roots, members, err := p.domain.DecomposeHeapTableIdentity(destination, p.keys)
	if err != nil {
		return LaneFactor{}, err
	}
	plan, err := p.Plan(skeleton)
	if err != nil {
		return LaneFactor{}, err
	}
	destinationRoots := make(map[identity.Term]HeapObjectRootFactor, len(roots))
	for _, root := range roots {
		destinationRoots[root.id] = root
	}
	destinationMembers := make(map[heapBoundaryMemberCoordinate]HeapStaticMemberFactor, len(members))
	for _, member := range members {
		destinationMembers[heapBoundaryMemberCoordinate{id: member.id, key: member.key}] = member
	}
	outputRoots := make([]HeapObjectRootFactor, 0, len(plan.roots))
	for _, selection := range plan.roots {
		switch selection.source {
		case BoundaryFactorSourceDestination:
			factor, present := destinationRoots[selection.slot.id]
			if !present {
				return LaneFactor{}, fmt.Errorf("%w: destination heap object %v has no root", ErrInvalidLaneFactor, selection.slot.id)
			}
			outputRoots = append(outputRoots, factor)
		case BoundaryFactorSourceFragment:
			outputRoots = append(outputRoots, selection.fragment)
		default:
			return LaneFactor{}, fmt.Errorf("%w: invalid heap root source", ErrInvalidLaneFactor)
		}
	}
	outputMembers := make([]HeapStaticMemberFactor, 0, len(plan.members))
	for _, selection := range plan.members {
		switch selection.source {
		case BoundaryFactorSourceDestination:
			coordinate := heapBoundaryMemberCoordinate{id: selection.slot.id, key: selection.slot.key}
			factor, present := destinationMembers[coordinate]
			if !present {
				return LaneFactor{}, fmt.Errorf("%w: destination heap member %v/%v is absent", ErrInvalidLaneFactor, coordinate.id, coordinate.key)
			}
			outputMembers = append(outputMembers, factor)
		case BoundaryFactorSourceFragment:
			outputMembers = append(outputMembers, selection.fragment)
		default:
			return LaneFactor{}, fmt.Errorf("%w: invalid heap member source", ErrInvalidLaneFactor)
		}
	}
	return p.domain.ComposeHeapTableIdentity(plan.skeleton, outputRoots, outputMembers, p.keys)
}
