package callboundary

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	engineregistry "github.com/wippyai/go-lua/analysis/engine/registry"
)

// NormalReturnFactLaneID names one typed normal-return fact lane. The ID is the
// stable architectural owner for operations over the corresponding storage field.
type NormalReturnFactLaneID string

const (
	LanePathRefinements          NormalReturnFactLaneID = "path-refinements"
	LanePersistentPathWrites     NormalReturnFactLaneID = "persistent-path-writes"
	LanePathStaticMembers        NormalReturnFactLaneID = "path-static-members"
	LanePathStaticMemberDeltas   NormalReturnFactLaneID = "path-static-member-deltas"
	LanePathPresenceImplications NormalReturnFactLaneID = "path-presence-implications"
	LanePathInvalidations        NormalReturnFactLaneID = "path-invalidations"
	LaneDynamicIndexFacts        NormalReturnFactLaneID = "dynamic-index-facts"
	LaneKeyMemberships           NormalReturnFactLaneID = "key-memberships"
	LaneDynamicValueKeys         NormalReturnFactLaneID = "dynamic-value-keys"
	LaneDynamicAllValues         NormalReturnFactLaneID = "dynamic-all-values"
	LaneBranchProofs             NormalReturnFactLaneID = "branch-proofs"
	LaneChannelSelects           NormalReturnFactLaneID = "channel-selects"
	LaneFrozenTables             NormalReturnFactLaneID = "frozen-tables"
	LaneEffectDeltas             NormalReturnFactLaneID = "effect-deltas"
	LaneEscapeEvents             NormalReturnFactLaneID = "escape-events"
	LaneStoreRelations           NormalReturnFactLaneID = "store-relations"
	LaneLifecycleFacts           NormalReturnFactLaneID = "lifecycle-facts"
	LaneNumFloors                NormalReturnFactLaneID = "num-floors"
	LaneRelConstraints           NormalReturnFactLaneID = "rel-constraints"
)

// NormalReturnPathPredicate selects boundary paths for operations that need to
// retain only facts rooted in a caller-chosen path set, such as return-slot
// replay after assigning a call result into a root target.
type NormalReturnPathPredicate func(pathdom.Path) bool

// NormalReturnFactLane owns storage-level operations for one typed fact lane.
// Higher layers attach summary/project/apply behavior to the same lane IDs.
type NormalReturnFactLane struct {
	id            NormalReturnFactLaneID
	fieldName     string
	len           func(NormalReturnFacts) int
	append        func(NormalReturnFacts, NormalReturnFacts) NormalReturnFacts
	filterPaths   func(*NormalReturnFacts, NormalReturnFacts, NormalReturnPathPredicate)
	dropPaths     func(*NormalReturnFacts, NormalReturnFacts, NormalReturnPathPredicate)
	filtersByPath bool
}

func (l NormalReturnFactLane) ID() NormalReturnFactLaneID { return l.id }

func (l NormalReturnFactLane) FieldName() string { return l.fieldName }

func (l NormalReturnFactLane) Len(f NormalReturnFacts) int { return l.len(f) }

func (l NormalReturnFactLane) Append(dst NormalReturnFacts, src NormalReturnFacts) NormalReturnFacts {
	return l.append(dst, src)
}

// FiltersByPath reports whether the lane participates in FilterPaths. Lanes
// that do not participate are still explicitly registered; this avoids silent
// omission while preserving existing replay semantics.
func (l NormalReturnFactLane) FiltersByPath() bool { return l.filtersByPath }

func (l NormalReturnFactLane) filter(dst *NormalReturnFacts, src NormalReturnFacts, keep NormalReturnPathPredicate) {
	if l.filterPaths != nil {
		l.filterPaths(dst, src, keep)
	}
}

func (l NormalReturnFactLane) drop(dst *NormalReturnFacts, src NormalReturnFacts, drop NormalReturnPathPredicate) {
	if l.dropPaths != nil {
		l.dropPaths(dst, src, drop)
		return
	}
	*dst = l.Append(*dst, src)
}

// NormalReturnFactLanes returns the typed storage-lane registry in operation
// order. The returned slice is a copy; modifying it cannot affect analysis.
func NormalReturnFactLanes() []NormalReturnFactLane {
	out := make([]NormalReturnFactLane, len(normalReturnFactLanes))
	copy(out, normalReturnFactLanes)
	return out
}

// NormalReturnFactLaneBinding pairs a layer-owned handler with the canonical
// storage lane it extends.
type NormalReturnFactLaneBinding[T any] struct {
	ID      NormalReturnFactLaneID
	Storage NormalReturnFactLane
	Value   T
}

// BindNormalReturnFactLanes orders layer-owned handlers by the storage-lane
// registry and rejects missing, invalid, or orphan handlers. This keeps storage
// as the sole owner of the lane set while summary/projection/application keep
// ownership of their per-lane behavior.
func BindNormalReturnFactLanes[T any](
	owner string,
	handlers map[NormalReturnFactLaneID]T,
	valid func(T) bool,
) []NormalReturnFactLaneBinding[T] {
	if owner == "" {
		owner = "normal-return"
	}
	bindings := engineregistry.BindOrdered(engineregistry.BindOptions[NormalReturnFactLaneID, NormalReturnFactLane, T]{
		Owner:    owner + " lane",
		Roles:    normalReturnFactLaneRoles(),
		Handlers: handlers,
		Valid:    valid,
		KeyName:  func(id NormalReturnFactLaneID) string { return string(id) },
	})
	out := make([]NormalReturnFactLaneBinding[T], 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, NormalReturnFactLaneBinding[T]{
			ID:      binding.Key,
			Storage: binding.Role,
			Value:   binding.Handler,
		})
	}
	return out
}

func normalReturnFactLaneRoles() []engineregistry.Role[NormalReturnFactLaneID, NormalReturnFactLane] {
	storage := NormalReturnFactLanes()
	out := make([]engineregistry.Role[NormalReturnFactLaneID, NormalReturnFactLane], len(storage))
	for i, lane := range storage {
		out[i] = engineregistry.Role[NormalReturnFactLaneID, NormalReturnFactLane]{
			Key:   lane.ID(),
			Value: lane,
		}
	}
	return out
}

// FilterPaths returns the subset of registered facts whose lane-owned boundary
// paths satisfy keep. It is intentionally lane-defined: some storage lanes do
// not replay through this path-filtering operation and therefore register an
// explicit no-op.
func (f NormalReturnFacts) FilterPaths(keep NormalReturnPathPredicate) NormalReturnFacts {
	if f.Empty() || keep == nil {
		return NormalReturnFacts{}
	}
	var out NormalReturnFacts
	for _, lane := range normalReturnFactLanes {
		lane.filter(&out, f, keep)
	}
	return out
}

// DropFactsTouchingPaths returns facts from registered lanes except facts whose
// lane-owned boundary path touches shouldDrop. Lanes that do not participate in
// path filtering, such as PersistentPathWrites, are copied unchanged.
func (f NormalReturnFacts) DropFactsTouchingPaths(shouldDrop NormalReturnPathPredicate) NormalReturnFacts {
	if f.Empty() || shouldDrop == nil {
		return f
	}
	var out NormalReturnFacts
	for _, lane := range normalReturnFactLanes {
		lane.drop(&out, f, shouldDrop)
	}
	return out
}

func normalReturnSliceLane[T any](
	id NormalReturnFactLaneID,
	fieldName string,
	get func(NormalReturnFacts) []T,
	set func(NormalReturnFacts, []T) NormalReturnFacts,
	keepFact func(T, NormalReturnPathPredicate) bool,
) NormalReturnFactLane {
	lane := NormalReturnFactLane{
		id:        id,
		fieldName: fieldName,
		len: func(f NormalReturnFacts) int {
			return len(get(f))
		},
		append: func(dst NormalReturnFacts, src NormalReturnFacts) NormalReturnFacts {
			return set(dst, appendNormalReturnSlice(get(dst), get(src)))
		},
	}
	if keepFact != nil {
		lane.filtersByPath = true
		lane.filterPaths = func(dst *NormalReturnFacts, src NormalReturnFacts, keep NormalReturnPathPredicate) {
			dstSlice := get(*dst)
			for _, fact := range get(src) {
				if keepFact(fact, keep) {
					dstSlice = append(dstSlice, fact)
				}
			}
			*dst = set(*dst, dstSlice)
		}
		lane.dropPaths = func(dst *NormalReturnFacts, src NormalReturnFacts, drop NormalReturnPathPredicate) {
			dstSlice := get(*dst)
			for _, fact := range get(src) {
				if keepFact(fact, drop) {
					continue
				}
				dstSlice = append(dstSlice, fact)
			}
			*dst = set(*dst, dstSlice)
		}
	}
	return lane
}

func keepPath(p pathdom.Path, keep NormalReturnPathPredicate) bool {
	return !p.IsEmpty() && keep(p)
}

func keepRelOperand(operand RelOperand, keep NormalReturnPathPredicate) bool {
	return keepPath(operand.Path, keep)
}

var normalReturnFactLanes = derivedNormalReturnFactLanes()
