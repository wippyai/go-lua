package callboundary

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// NormalReturnFactLaneID names one typed normal-return fact lane. The ID is the
// stable architectural owner for operations over the corresponding storage field.
type NormalReturnFactLaneID string

const (
	LanePathRefinements          NormalReturnFactLaneID = "path-refinements"
	LanePersistentPathWrites     NormalReturnFactLaneID = "persistent-path-writes"
	LanePathStaticMembers        NormalReturnFactLaneID = "path-static-members"
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

var normalReturnFactLanes = []NormalReturnFactLane{
	normalReturnSliceLane(LanePathRefinements, "PathRefinements",
		func(f NormalReturnFacts) []PathValueFact { return f.PathRefinements },
		func(f NormalReturnFacts, facts []PathValueFact) NormalReturnFacts {
			f.PathRefinements = facts
			return f
		},
		func(f PathValueFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
	normalReturnSliceLane[PathValueFact](LanePersistentPathWrites, "PersistentPathWrites",
		func(f NormalReturnFacts) []PathValueFact { return f.PersistentPathWrites },
		func(f NormalReturnFacts, facts []PathValueFact) NormalReturnFacts {
			f.PersistentPathWrites = facts
			return f
		},
		nil),
	normalReturnSliceLane(LanePathStaticMembers, "PathStaticMembers",
		func(f NormalReturnFacts) []PathStaticMemberFact { return f.PathStaticMembers },
		func(f NormalReturnFacts, facts []PathStaticMemberFact) NormalReturnFacts {
			f.PathStaticMembers = facts
			return f
		},
		func(f PathStaticMemberFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
	normalReturnSliceLane(LanePathPresenceImplications, "PathPresenceImplications",
		func(f NormalReturnFacts) []PathPresenceImplicationFact { return f.PathPresenceImplications },
		func(f NormalReturnFacts, facts []PathPresenceImplicationFact) NormalReturnFacts {
			f.PathPresenceImplications = facts
			return f
		},
		func(f PathPresenceImplicationFact, keep NormalReturnPathPredicate) bool {
			return keepPath(f.Trigger, keep) || keepPath(f.Target, keep)
		}),
	normalReturnSliceLane(LanePathInvalidations, "PathInvalidations",
		func(f NormalReturnFacts) []PathInvalidationFact { return f.PathInvalidations },
		func(f NormalReturnFacts, facts []PathInvalidationFact) NormalReturnFacts {
			f.PathInvalidations = facts
			return f
		},
		func(f PathInvalidationFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
	normalReturnSliceLane(LaneDynamicIndexFacts, "DynamicIndexFacts",
		func(f NormalReturnFacts) []DynamicIndexFact { return f.DynamicIndexFacts },
		func(f NormalReturnFacts, facts []DynamicIndexFact) NormalReturnFacts {
			f.DynamicIndexFacts = facts
			return f
		},
		func(f DynamicIndexFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Table, keep) }),
	normalReturnSliceLane(LaneKeyMemberships, "KeyMemberships",
		func(f NormalReturnFacts) []KeyMembershipFact { return f.KeyMemberships },
		func(f NormalReturnFacts, facts []KeyMembershipFact) NormalReturnFacts {
			f.KeyMemberships = facts
			return f
		},
		func(f KeyMembershipFact, keep NormalReturnPathPredicate) bool {
			return keepPath(f.Key, keep) || keepPath(f.Table, keep)
		}),
	normalReturnSliceLane(LaneDynamicValueKeys, "DynamicValueKeys",
		func(f NormalReturnFacts) []DynamicValueKeyMembershipFact { return f.DynamicValueKeys },
		func(f NormalReturnFacts, facts []DynamicValueKeyMembershipFact) NormalReturnFacts {
			f.DynamicValueKeys = facts
			return f
		},
		func(f DynamicValueKeyMembershipFact, keep NormalReturnPathPredicate) bool {
			return keepPath(f.Container, keep) || keepPath(f.Table, keep)
		}),
	normalReturnSliceLane(LaneDynamicAllValues, "DynamicAllValues",
		func(f NormalReturnFacts) []DynamicAllValueKeyMembershipFact { return f.DynamicAllValues },
		func(f NormalReturnFacts, facts []DynamicAllValueKeyMembershipFact) NormalReturnFacts {
			f.DynamicAllValues = facts
			return f
		},
		func(f DynamicAllValueKeyMembershipFact, keep NormalReturnPathPredicate) bool {
			return keepPath(f.Container, keep) || keepPath(f.Table, keep)
		}),
	normalReturnSliceLane(LaneBranchProofs, "BranchProofs",
		func(f NormalReturnFacts) []BranchProof { return f.BranchProofs },
		func(f NormalReturnFacts, facts []BranchProof) NormalReturnFacts {
			f.BranchProofs = facts
			return f
		},
		func(f BranchProof, keep NormalReturnPathPredicate) bool {
			return keepPath(f.Path, keep) || keepPath(f.Other, keep)
		}),
	normalReturnSliceLane(LaneChannelSelects, "ChannelSelects",
		func(f NormalReturnFacts) []ChannelSelectFact { return f.ChannelSelects },
		func(f NormalReturnFacts, facts []ChannelSelectFact) NormalReturnFacts {
			f.ChannelSelects = facts
			return f
		},
		func(f ChannelSelectFact, keep NormalReturnPathPredicate) bool {
			return keepPath(f.Result, keep) || keepPath(f.Case, keep)
		}),
	normalReturnSliceLane(LaneFrozenTables, "FrozenTables",
		func(f NormalReturnFacts) []FrozenTableFact { return f.FrozenTables },
		func(f NormalReturnFacts, facts []FrozenTableFact) NormalReturnFacts {
			f.FrozenTables = facts
			return f
		},
		func(f FrozenTableFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
	normalReturnSliceLane(LaneEffectDeltas, "EffectDeltas",
		func(f NormalReturnFacts) []EffectDelta { return f.EffectDeltas },
		func(f NormalReturnFacts, facts []EffectDelta) NormalReturnFacts {
			f.EffectDeltas = facts
			return f
		},
		func(f EffectDelta, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
	normalReturnSliceLane(LaneEscapeEvents, "EscapeEvents",
		func(f NormalReturnFacts) []EscapeEventFact { return f.EscapeEvents },
		func(f NormalReturnFacts, facts []EscapeEventFact) NormalReturnFacts {
			f.EscapeEvents = facts
			return f
		},
		func(f EscapeEventFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
	normalReturnSliceLane(LaneStoreRelations, "StoreRelations",
		func(f NormalReturnFacts) []StoreRelationFact { return f.StoreRelations },
		func(f NormalReturnFacts, facts []StoreRelationFact) NormalReturnFacts {
			f.StoreRelations = facts
			return f
		},
		func(f StoreRelationFact, keep NormalReturnPathPredicate) bool {
			return keepPath(f.Source, keep) || keepPath(f.Into, keep)
		}),
	normalReturnSliceLane(LaneLifecycleFacts, "LifecycleFacts",
		func(f NormalReturnFacts) []LifecycleFact { return f.LifecycleFacts },
		func(f NormalReturnFacts, facts []LifecycleFact) NormalReturnFacts {
			f.LifecycleFacts = facts
			return f
		},
		func(f LifecycleFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
	normalReturnSliceLane(LaneNumFloors, "NumFloors",
		func(f NormalReturnFacts) []NumFloorFact { return f.NumFloors },
		func(f NormalReturnFacts, facts []NumFloorFact) NormalReturnFacts {
			f.NumFloors = facts
			return f
		},
		func(f NumFloorFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
	normalReturnSliceLane(LaneRelConstraints, "RelConstraints",
		func(f NormalReturnFacts) []RelConstraintFact { return f.RelConstraints },
		func(f NormalReturnFacts, facts []RelConstraintFact) NormalReturnFacts {
			f.RelConstraints = facts
			return f
		},
		func(f RelConstraintFact, keep NormalReturnPathPredicate) bool {
			return keepRelOperand(f.A, keep) || keepRelOperand(f.B, keep) || keepRelOperand(f.C, keep)
		}),
}
