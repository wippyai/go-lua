package placementplan

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	checkprogram "github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

type Target uint8

const (
	TargetNoFact Target = iota
	TargetStack
	TargetOwnedHeap
	TargetSharedHeap
	TargetUnknown
)

func (t Target) String() string {
	switch t {
	case TargetNoFact:
		return "no-placement-fact"
	case TargetStack:
		return "stack"
	case TargetOwnedHeap:
		return "owned-heap"
	case TargetSharedHeap:
		return "shared-heap"
	case TargetUnknown:
		return "unknown"
	default:
		return "placement-target(invalid)"
	}
}

type Reason string

const (
	ReasonLocalMaterialized Reason = "local-materialized"
	ReasonStoredOrRetained  Reason = "stored-or-retained"
	ReasonSharedEscape      Reason = "shared-escape"
	ReasonFrozen            Reason = "frozen"
)

type Obligation string

const (
	ObligationOwnerIdentity   Obligation = "owner-identity"
	ObligationSealBeforeShare Obligation = "seal-before-share"
)

type Blocker string

const (
	BlockerMissingExitState     Blocker = "missing-exit-state"
	BlockerHeapObjectsTop       Blocker = "heap-objects-top"
	BlockerPlacementsTop        Blocker = "placements-top"
	BlockerMissingPlacementFact Blocker = "missing-placement-fact"
	BlockerUnknownPlacement     Blocker = "unknown-placement"
)

type Entry struct {
	ID          identity.ID
	Target      Target
	Placement   placement.Value
	HasObject   bool
	Frozen      bool
	Reasons     []Reason
	Obligations []Obligation
	Blockers    []Blocker
	Children    []identity.ID
}

type Plan struct {
	Top        bool
	Incomplete bool
	Blockers   []Blocker
	Entries    []Entry
}

func FromProgramResult(result checkprogram.Result) Plan {
	return FromBodyResult(result.RootResult())
}

func FromBodyResult(result *body.Result) Plan {
	return FromBodyResults(result)
}

func FromBodyResults(results ...*body.Result) Plan {
	aggregate := newAggregate()
	for _, result := range results {
		aggregate.addResult(result)
	}
	return aggregate.plan()
}

func FromResult(result *body.Result) Plan {
	return FromBodyResult(result)
}

func Merge(plans ...Plan) Plan {
	aggregate := newAggregate()
	for _, plan := range plans {
		if plan.Top {
			aggregate.top = true
		}
		if plan.Incomplete {
			aggregate.incomplete = true
		}
		for _, blocker := range plan.Blockers {
			aggregate.blockers[blocker] = struct{}{}
		}
		for _, entry := range plan.Entries {
			if entry.HasObject {
				aggregate.objects[entry.ID] = struct{}{}
			}
			aggregate.addChildren(entry.ID, entry.Children)
			if entry.Frozen {
				aggregate.frozen[entry.ID] = struct{}{}
			}
			if entry.Target == TargetNoFact {
				continue
			}
			if prev, ok := aggregate.placements[entry.ID]; ok {
				aggregate.placements[entry.ID] = placement.Join(prev, entry.Placement)
				continue
			}
			aggregate.placements[entry.ID] = entry.Placement
		}
	}
	return aggregate.plan()
}

func (p Plan) Placement(id identity.ID) (placement.Value, bool) {
	for _, entry := range p.Entries {
		if entry.ID == id && entry.Target != TargetNoFact {
			return entry.Placement, true
		}
	}
	return placement.Bottom, false
}

func (p Plan) MaxTargetDepth(target Target) int {
	byID := make(map[identity.ID]Entry, len(p.Entries))
	for _, entry := range p.Entries {
		byID[entry.ID] = entry
	}
	var walk func(identity.ID, map[identity.ID]struct{}) int
	walk = func(id identity.ID, seen map[identity.ID]struct{}) int {
		if id == (identity.ID{}) {
			return 0
		}
		entry, ok := byID[id]
		if !ok || entry.Target != target {
			return 0
		}
		if _, ok := seen[id]; ok {
			return 0
		}
		nextSeen := make(map[identity.ID]struct{}, len(seen)+1)
		for seenID := range seen {
			nextSeen[seenID] = struct{}{}
		}
		nextSeen[id] = struct{}{}
		depth := 1
		for _, child := range entry.Children {
			if childDepth := 1 + walk(child, nextSeen); childDepth > depth {
				depth = childDepth
			}
		}
		return depth
	}
	depth := 0
	for _, entry := range p.Entries {
		if candidate := walk(entry.ID, nil); candidate > depth {
			depth = candidate
		}
	}
	return depth
}

func FromState(st state.State) Plan {
	aggregate := newAggregate()
	aggregate.addState(nil, st)
	return aggregate.plan()
}

type aggregate struct {
	top        bool
	incomplete bool
	blockers   map[Blocker]struct{}
	objects    map[identity.ID]struct{}
	children   map[identity.ID]map[identity.ID]struct{}
	placements map[identity.ID]placement.Value
	frozen     map[identity.ID]struct{}
}

func newAggregate() aggregate {
	return aggregate{
		blockers:   make(map[Blocker]struct{}),
		objects:    make(map[identity.ID]struct{}),
		children:   make(map[identity.ID]map[identity.ID]struct{}),
		placements: make(map[identity.ID]placement.Value),
		frozen:     make(map[identity.ID]struct{}),
	}
}

func (a *aggregate) addResult(result *body.Result) {
	if result == nil {
		a.addBlocker(BlockerMissingExitState)
		return
	}
	exit, ok := result.ExitState()
	if !ok {
		a.addBlocker(BlockerMissingExitState)
	} else {
		a.addState(result.Registry(), exit)
	}
	for _, child := range result.FunctionResults() {
		a.addResult(child)
	}
}

func (a *aggregate) addState(reg *axis.Registry, st state.State) {
	heap := st.HeapTableObjectsSnapshot()
	placements := st.PlacementsSnapshot()
	frozen := st.FrozenTablesSnapshot()

	if heap.Top {
		a.addTopBlocker(BlockerHeapObjectsTop)
	}
	if placements.Top {
		a.addTopBlocker(BlockerPlacementsTop)
	}
	for id := range heap.Objects {
		a.objects[id] = struct{}{}
	}
	if reg != nil {
		for id, object := range heap.Objects {
			a.addChildren(id, heapObjectChildren(reg, object))
		}
	}
	for id, value := range placements.Placements {
		if prev, ok := a.placements[id]; ok {
			a.placements[id] = placement.Join(prev, value)
			continue
		}
		a.placements[id] = value
	}
	for _, id := range frozen.Tables {
		a.frozen[id] = struct{}{}
	}
}

func (a *aggregate) plan() Plan {
	ids := make(map[identity.ID]struct{}, len(a.objects)+len(a.placements))
	for id := range a.objects {
		ids[id] = struct{}{}
	}
	for id, children := range a.children {
		ids[id] = struct{}{}
		for child := range children {
			if _, ok := a.objects[child]; ok {
				ids[child] = struct{}{}
				continue
			}
			if _, ok := a.placements[child]; ok {
				ids[child] = struct{}{}
			}
		}
	}
	for id := range a.placements {
		ids[id] = struct{}{}
	}
	ordered := orderedIDs(ids)
	out := Plan{
		Top:        a.top,
		Incomplete: a.incomplete,
		Blockers:   orderedBlockers(a.blockers),
		Entries:    make([]Entry, 0, len(ordered)),
	}
	for _, id := range ordered {
		value, hasPlacement := a.placements[id]
		if !hasPlacement {
			value = placement.Bottom
		}
		entry := Entry{
			ID:        id,
			Placement: value,
			Target:    targetForPlacement(value, hasPlacement),
			HasObject: mapContains(a.objects, id),
			Frozen:    mapContains(a.frozen, id),
			Children:  orderedIDs(a.children[id]),
		}
		entry = annotate(entry, hasPlacement)
		out.Entries = append(out.Entries, entry)
	}
	return out
}

func (a *aggregate) addChildren(parent identity.ID, children []identity.ID) {
	if parent == (identity.ID{}) || len(children) == 0 {
		return
	}
	set := a.children[parent]
	if set == nil {
		set = make(map[identity.ID]struct{}, len(children))
		a.children[parent] = set
	}
	for _, child := range children {
		if child != (identity.ID{}) {
			set[child] = struct{}{}
		}
	}
}

func (a *aggregate) addBlocker(blocker Blocker) {
	a.incomplete = true
	a.blockers[blocker] = struct{}{}
}

func (a *aggregate) addTopBlocker(blocker Blocker) {
	a.top = true
	a.addBlocker(blocker)
}

func targetForPlacement(value placement.Value, hasPlacement bool) Target {
	if !hasPlacement || value == placement.Bottom {
		return TargetNoFact
	}
	switch value {
	case placement.Stack:
		return TargetStack
	case placement.OwnedHeap:
		return TargetOwnedHeap
	case placement.SharedHeap:
		return TargetSharedHeap
	default:
		return TargetUnknown
	}
}

func annotate(entry Entry, hasPlacement bool) Entry {
	if !hasPlacement || entry.Placement == placement.Bottom {
		entry.Blockers = append(entry.Blockers, BlockerMissingPlacementFact)
		return entry
	}
	switch entry.Target {
	case TargetStack:
		entry.Reasons = append(entry.Reasons, ReasonLocalMaterialized)
	case TargetOwnedHeap:
		entry.Reasons = append(entry.Reasons, ReasonStoredOrRetained)
		entry.Obligations = append(entry.Obligations, ObligationOwnerIdentity)
	case TargetSharedHeap:
		entry.Reasons = append(entry.Reasons, ReasonSharedEscape)
		if !entry.Frozen {
			entry.Obligations = append(entry.Obligations, ObligationSealBeforeShare)
		}
	case TargetUnknown:
		entry.Blockers = append(entry.Blockers, BlockerUnknownPlacement)
	}
	if entry.Frozen {
		entry.Reasons = append(entry.Reasons, ReasonFrozen)
	}
	return entry
}

func mapContains[T any](in map[identity.ID]T, id identity.ID) bool {
	_, ok := in[id]
	return ok
}

func orderedIDs(ids map[identity.ID]struct{}) []identity.ID {
	out := make([]identity.ID, 0, len(ids))
	for id := range ids {
		if id != (identity.ID{}) {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Site != right.Site {
			return left.Site < right.Site
		}
		return left.Index < right.Index
	})
	return out
}

func heapObjectChildren(reg *axis.Registry, object heapidentity.TableObject) []identity.ID {
	ids := make(map[identity.ID]struct{})
	for _, value := range object.StaticMembers() {
		if id, ok := valueIdentity(reg, value); ok {
			ids[id] = struct{}{}
		}
	}
	for _, fact := range object.DynamicIndexFacts() {
		if id, ok := valueIdentity(reg, fact.Value); ok {
			ids[id] = struct{}{}
		}
	}
	return orderedIDs(ids)
}

func valueIdentity(reg *axis.Registry, value product.Value) (identity.ID, bool) {
	if reg == nil {
		return identity.ID{}, false
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || id == (identity.ID{}) {
		return identity.ID{}, false
	}
	return id, true
}

func orderedBlockers(in map[Blocker]struct{}) []Blocker {
	if len(in) == 0 {
		return nil
	}
	out := make([]Blocker, 0, len(in))
	for blocker := range in {
		out = append(out, blocker)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}
