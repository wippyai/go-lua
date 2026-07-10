package placementplan

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	checkprogram "github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	internalreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
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
	TargetFrameLocal
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
	case TargetFrameLocal:
		return "frame-local"
	default:
		return "placement-target(invalid)"
	}
}

type Reason string

const (
	ReasonLocalMaterialized Reason = "local-materialized"
	ReasonFrameLocalProof   Reason = "frame-local-proof"
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
	ID                      identity.ID
	Target                  Target
	Placement               placement.Value
	HasObject               bool
	AllocationSite          bool
	Decomposable            bool
	FrameLocalUseProof      bool
	FrameLocal              bool
	Frozen                  bool
	Reasons                 []Reason
	Obligations             []Obligation
	Blockers                []Blocker
	Children                []identity.ID
	DiesBeforeSuspension    bool
	HasDiesBeforeSuspension bool
}

type Plan struct {
	Top            bool
	Incomplete     bool
	Blockers       []Blocker
	Entries        []Entry
	HoistableLoads []readapi.HoistableLoad

	licenses map[identity.ID]placement.AllocationSiteLicenses
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
			aggregate.addLicenses(entry.ID, plan.licensesFor(entry))
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
		for _, load := range plan.HoistableLoads {
			aggregate.addHoistableLoad(load)
		}
	}
	return aggregate.plan()
}

func (p Plan) licensesFor(entry Entry) placement.AllocationSiteLicenses {
	if licenses, ok := p.licenses[entry.ID]; ok && entryMatchesLicenseProjection(entry, licenses.Projection()) {
		return licenses
	}
	return allocationSiteLicensesFromEntry(entry)
}

func entryMatchesLicenseProjection(entry Entry, projection placement.AllocationSiteLicenseProjection) bool {
	return entry.AllocationSite == projection.AllocationSite &&
		entry.Decomposable == projection.Decomposable &&
		entry.FrameLocalUseProof == projection.FrameLocalUseProof &&
		entry.FrameLocal == projection.FrameLocal &&
		entry.DiesBeforeSuspension == projection.DiesBeforeSuspension &&
		entry.HasDiesBeforeSuspension == projection.HasDiesBeforeSuspension
}

func allocationSiteLicensesFromEntry(entry Entry) placement.AllocationSiteLicenses {
	licenses := placement.AllocationSiteLicenses{}.
		With(placement.LicenseAllocationSite, placement.LicenseStateFor(entry.AllocationSite)).
		With(placement.LicenseFrameLocal, placement.LicenseStateFor(entry.FrameLocal))
	if entry.AllocationSite {
		licenses = licenses.
			With(placement.LicenseDecomposable, placement.LicenseStateFor(entry.Decomposable)).
			With(placement.LicenseFrameLocalUse, placement.LicenseStateFor(entry.FrameLocalUseProof))
	}
	if entry.HasDiesBeforeSuspension {
		licenses = licenses.With(placement.LicenseDiesBeforeSuspension, placement.LicenseStateFor(entry.DiesBeforeSuspension))
	}
	return licenses
}

func (p Plan) Placement(id identity.ID) (placement.Value, bool) {
	for _, entry := range p.Entries {
		if entry.ID == id && entry.Target != TargetNoFact {
			return entry.Placement, true
		}
	}
	return placement.Bottom, false
}

func (p Plan) Decomposable(id identity.ID) bool {
	for _, entry := range p.Entries {
		if entry.ID == id {
			return entry.Decomposable
		}
	}
	return false
}

func (p Plan) FrameLocal(id identity.ID) bool {
	for _, entry := range p.Entries {
		if entry.ID == id {
			return entry.FrameLocal
		}
	}
	return false
}

func (p Plan) AllocationStats() (total, decomposable int) {
	for _, entry := range p.Entries {
		if !entry.AllocationSite {
			continue
		}
		total++
		if entry.Decomposable {
			decomposable++
		}
	}
	return total, decomposable
}

func (p Plan) FrameLocalStats() (total, frameLocal int) {
	for _, entry := range p.Entries {
		if !entry.AllocationSite {
			continue
		}
		total++
		if entry.FrameLocal {
			frameLocal++
		}
	}
	return total, frameLocal
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
		if !ok || !targetMatches(entry.Target, target) {
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
	top            bool
	incomplete     bool
	blockers       map[Blocker]struct{}
	objects        map[identity.ID]struct{}
	licenses       map[identity.ID]placement.AllocationSiteLicenses
	children       map[identity.ID]map[identity.ID]struct{}
	placements     map[identity.ID]placement.Value
	frozen         map[identity.ID]struct{}
	hoistableLoads map[hoistableLoadKey]readapi.HoistableLoad
}

type hoistableLoadKey struct {
	bodyID   uint64
	point    uint32
	loopHead uint32
	readPath pathdom.PathKey
}

func newAggregate() aggregate {
	return aggregate{
		blockers:       make(map[Blocker]struct{}),
		objects:        make(map[identity.ID]struct{}),
		licenses:       make(map[identity.ID]placement.AllocationSiteLicenses),
		children:       make(map[identity.ID]map[identity.ID]struct{}),
		placements:     make(map[identity.ID]placement.Value),
		frozen:         make(map[identity.ID]struct{}),
		hoistableLoads: make(map[hoistableLoadKey]readapi.HoistableLoad),
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
	result.ForEachAllocationSiteFact(func(fact body.AllocationSiteFact) bool {
		a.addLicenses(fact.Identity, fact.Licenses())
		return true
	})
	internalreadmodel.New(result).ForEachHoistableLoad(func(load internalreadmodel.HoistableLoad) bool {
		a.addHoistableLoad(load)
		return true
	})
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
	ids := make(map[identity.ID]struct{}, len(a.objects)+len(a.placements)+len(a.licenses))
	for id := range a.objects {
		ids[id] = struct{}{}
	}
	for id, licenses := range a.licenses {
		projection := licenses.Projection()
		if projection.AllocationSite || projection.HasDiesBeforeSuspension {
			ids[id] = struct{}{}
		}
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
		Top:            a.top,
		Incomplete:     a.incomplete,
		Blockers:       orderedBlockers(a.blockers),
		Entries:        make([]Entry, 0, len(ordered)),
		HoistableLoads: orderedHoistableLoads(a.hoistableLoads),
		licenses:       make(map[identity.ID]placement.AllocationSiteLicenses, len(ordered)),
	}
	for _, id := range ordered {
		value, hasPlacement := a.placements[id]
		if !hasPlacement {
			value = placement.Bottom
		}
		licenses := a.licenses[id]
		projection := licenses.Projection()
		target := targetForPlacement(value, hasPlacement)
		if projection.FrameLocal {
			target = TargetFrameLocal
		}
		entry := Entry{
			ID:                 id,
			Placement:          value,
			Target:             target,
			HasObject:          mapContains(a.objects, id),
			AllocationSite:     projection.AllocationSite,
			Decomposable:       projection.Decomposable,
			FrameLocalUseProof: projection.FrameLocalUseProof,
			FrameLocal:         projection.FrameLocal,
			Frozen:             mapContains(a.frozen, id),
			Children:           orderedIDs(a.children[id]),
		}
		if projection.HasDiesBeforeSuspension {
			entry.DiesBeforeSuspension = projection.DiesBeforeSuspension
			entry.HasDiesBeforeSuspension = true
		}
		entry = annotate(entry, hasPlacement)
		out.Entries = append(out.Entries, entry)
		out.licenses[id] = licenses
	}
	return out
}

func (a *aggregate) addLicenses(id identity.ID, licenses placement.AllocationSiteLicenses) {
	if id == (identity.ID{}) {
		return
	}
	if current, ok := a.licenses[id]; ok {
		a.licenses[id] = current.Join(licenses)
		return
	}
	a.licenses[id] = licenses
}

func (a *aggregate) addHoistableLoad(load readapi.HoistableLoad) {
	if load.SchemaVersion != readapi.HoistableLoadSchemaVersion ||
		load.BodyID == 0 || load.Point == 0 || load.LoopHead == 0 || load.ReadPath.IsEmpty() {
		return
	}
	key := hoistableLoadKey{
		bodyID:   load.BodyID,
		point:    uint32(load.Point),
		loopHead: uint32(load.LoopHead),
		readPath: load.ReadPath.Key(),
	}
	a.hoistableLoads[key] = load
}

func orderedHoistableLoads(loads map[hoistableLoadKey]readapi.HoistableLoad) []readapi.HoistableLoad {
	out := make([]readapi.HoistableLoad, 0, len(loads))
	for _, load := range loads {
		out = append(out, load)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.BodyID != right.BodyID {
			return left.BodyID < right.BodyID
		}
		if left.LoopHead != right.LoopHead {
			return left.LoopHead < right.LoopHead
		}
		if left.Point != right.Point {
			return left.Point < right.Point
		}
		return left.ReadPath.String() < right.ReadPath.String()
	})
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

func targetMatches(got, want Target) bool {
	if got == want {
		return true
	}
	return want == TargetStack && got == TargetFrameLocal
}

func annotate(entry Entry, hasPlacement bool) Entry {
	if !hasPlacement || entry.Placement == placement.Bottom {
		entry.Blockers = append(entry.Blockers, BlockerMissingPlacementFact)
		return entry
	}
	switch entry.Target {
	case TargetStack:
		entry.Reasons = append(entry.Reasons, ReasonLocalMaterialized)
	case TargetFrameLocal:
		entry.Reasons = append(entry.Reasons, ReasonLocalMaterialized, ReasonFrameLocalProof)
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
