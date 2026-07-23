package callboundary

// BoundaryFactKind is the stable name of one boundary-schema fact lane within a
// family. It is the architectural owner key used to bind a family's per-kind
// behavior to the shared descriptor spine. For the NormalReturnFacts family a
// kind equals a NormalReturnFactLaneID; other families reuse their own field or
// lane names as kinds.
type BoundaryFactKind string

// BoundaryFactDescriptor is the family-agnostic spine shared by every
// boundary-schema lane family (summary slots, NormalReturnFacts lanes, and
// CallOutcome lanes). One descriptor fully names a kind and carries:
//
//   - Kind: the stable architectural owner name.
//   - WireRef: the manifest OperationalEffects wire lane field names this kind
//     corresponds to, linking a boundary fact to its serialized form. It is nil
//     for kinds that never cross a signature boundary (local-only or
//     result/authority lanes). A kind may map to more than one wire lane when a
//     single boundary lane is lowered from several manifest fields.
//   - Ops: the family-specific behavior payload. Each family composes its own
//     Ops type (append/filter for NormalReturnFacts, join/widen/equal for
//     summary slots, presence predicate for CallOutcome) rather than forcing one
//     operation shape across structurally different families.
//
// The spine lets each boundary-schema family derive its lane registry from one
// descriptor table.
type BoundaryFactDescriptor[Ops any] struct {
	Kind    BoundaryFactKind
	WireRef []string
	Ops     Ops
}

// BoundaryFactTable is an ordered descriptor table for one lane family. Order is
// significant: it is the canonical operation order every family driver walks.
type BoundaryFactTable[Ops any] []BoundaryFactDescriptor[Ops]

// Validate panics when the table has an empty or duplicate kind. Families call
// it once at package init so a malformed descriptor table fails loudly rather
// than silently dropping or double-owning a lane.
func (t BoundaryFactTable[Ops]) Validate(family string) {
	seen := make(map[BoundaryFactKind]struct{}, len(t))
	for _, d := range t {
		if d.Kind == "" {
			panic(family + " boundary fact descriptor with empty kind")
		}
		if _, ok := seen[d.Kind]; ok {
			panic(family + " boundary fact descriptor duplicate kind " + string(d.Kind))
		}
		seen[d.Kind] = struct{}{}
	}
}

// DeriveBoundaryLanes maps each descriptor to a family lane through build,
// preserving table order.
func DeriveBoundaryLanes[Ops, Lane any](t BoundaryFactTable[Ops], build func(BoundaryFactDescriptor[Ops]) Lane) []Lane {
	out := make([]Lane, len(t))
	for i, d := range t {
		out[i] = build(d)
	}
	return out
}

// NormalReturnLaneOps is the NormalReturnFacts family's per-kind behavior
// payload. It mirrors the storage-level fields of NormalReturnFactLane: the
// field name, length, append, and optional path filter/drop hooks. Building an
// Ops through normalReturnLaneDescriptor reuses normalReturnSliceLane so the
// descriptor table and lane registry share one construction path.
type NormalReturnLaneOps struct {
	fieldName     string
	len           func(NormalReturnFacts) int
	append        func(NormalReturnFacts, NormalReturnFacts) NormalReturnFacts
	filterPaths   func(*NormalReturnFacts, NormalReturnFacts, NormalReturnPathPredicate)
	dropPaths     func(*NormalReturnFacts, NormalReturnFacts, NormalReturnPathPredicate)
	filtersByPath bool
}

// FieldName returns the storage field name the lane owns.
func (o NormalReturnLaneOps) FieldName() string { return o.fieldName }

// FiltersByPath reports whether the lane participates in path filtering.
func (o NormalReturnLaneOps) FiltersByPath() bool { return o.filtersByPath }

// deriveLane rebuilds the storage lane the ops describe.
func (d BoundaryFactDescriptor[Ops]) deriveNormalReturnLane() NormalReturnFactLane {
	ops := any(d.Ops).(NormalReturnLaneOps)
	return NormalReturnFactLane{
		id:            NormalReturnFactLaneID(d.Kind),
		fieldName:     ops.fieldName,
		len:           ops.len,
		append:        ops.append,
		filterPaths:   ops.filterPaths,
		dropPaths:     ops.dropPaths,
		filtersByPath: ops.filtersByPath,
	}
}

func normalReturnLaneDescriptor[T any](
	id NormalReturnFactLaneID,
	wireRef []string,
	fieldName string,
	get func(NormalReturnFacts) []T,
	set func(NormalReturnFacts, []T) NormalReturnFacts,
	keepFact func(T, NormalReturnPathPredicate) bool,
) BoundaryFactDescriptor[NormalReturnLaneOps] {
	lane := normalReturnSliceLane(id, fieldName, get, set, keepFact)
	return BoundaryFactDescriptor[NormalReturnLaneOps]{
		Kind:    BoundaryFactKind(id),
		WireRef: wireRef,
		Ops: NormalReturnLaneOps{
			fieldName:     lane.fieldName,
			len:           lane.len,
			append:        lane.append,
			filterPaths:   lane.filterPaths,
			dropPaths:     lane.dropPaths,
			filtersByPath: lane.filtersByPath,
		},
	}
}

// normalReturnFactDescriptors registers NormalReturnFacts storage lanes in
// canonical order. WireRef identifies manifest OperationalEffects lanes;
// local-only lanes carry nil.
var normalReturnFactDescriptors = func() BoundaryFactTable[NormalReturnLaneOps] {
	t := BoundaryFactTable[NormalReturnLaneOps]{
		normalReturnLaneDescriptor(LanePathRefinements,
			[]string{"NormalReturnPresenceRefinements", "NormalReturnTypeRefinements"}, "PathRefinements",
			func(f NormalReturnFacts) []PathValueFact { return f.PathRefinements },
			func(f NormalReturnFacts, facts []PathValueFact) NormalReturnFacts {
				f.PathRefinements = facts
				return f
			},
			func(f PathValueFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
		normalReturnLaneDescriptor[PathValueFact](LanePersistentPathWrites,
			nil, "PersistentPathWrites",
			func(f NormalReturnFacts) []PathValueFact { return f.PersistentPathWrites },
			func(f NormalReturnFacts, facts []PathValueFact) NormalReturnFacts {
				f.PersistentPathWrites = facts
				return f
			},
			nil),
		normalReturnLaneDescriptor(LanePathStaticMembers,
			[]string{"PathStaticMembers"}, "PathStaticMembers",
			func(f NormalReturnFacts) []PathStaticMemberFact { return f.PathStaticMembers },
			func(f NormalReturnFacts, facts []PathStaticMemberFact) NormalReturnFacts {
				f.PathStaticMembers = facts
				return f
			},
			func(f PathStaticMemberFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
		normalReturnLaneDescriptor(LanePathStaticMemberDeltas,
			[]string{"PathStaticMemberDeltas"}, "PathStaticMemberDeltas",
			func(f NormalReturnFacts) []PathStaticMemberDeltaFact { return f.PathStaticMemberDeltas },
			func(f NormalReturnFacts, facts []PathStaticMemberDeltaFact) NormalReturnFacts {
				f.PathStaticMemberDeltas = facts
				return f
			},
			func(f PathStaticMemberDeltaFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
		normalReturnLaneDescriptor(LanePathPresenceImplications,
			[]string{"PathPresenceImplications"}, "PathPresenceImplications",
			func(f NormalReturnFacts) []PathPresenceImplicationFact { return f.PathPresenceImplications },
			func(f NormalReturnFacts, facts []PathPresenceImplicationFact) NormalReturnFacts {
				f.PathPresenceImplications = facts
				return f
			},
			func(f PathPresenceImplicationFact, keep NormalReturnPathPredicate) bool {
				return keepPath(f.Trigger, keep) || keepPath(f.Target, keep)
			}),
		normalReturnLaneDescriptor(LanePathInvalidations,
			[]string{"PathInvalidations"}, "PathInvalidations",
			func(f NormalReturnFacts) []PathInvalidationFact { return f.PathInvalidations },
			func(f NormalReturnFacts, facts []PathInvalidationFact) NormalReturnFacts {
				f.PathInvalidations = facts
				return f
			},
			func(f PathInvalidationFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
		normalReturnLaneDescriptor(LaneDynamicIndexFacts,
			[]string{"DynamicIndexFacts"}, "DynamicIndexFacts",
			func(f NormalReturnFacts) []DynamicIndexFact { return f.DynamicIndexFacts },
			func(f NormalReturnFacts, facts []DynamicIndexFact) NormalReturnFacts {
				f.DynamicIndexFacts = facts
				return f
			},
			func(f DynamicIndexFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Table, keep) }),
		normalReturnLaneDescriptor(LaneKeyMemberships,
			[]string{"KeyMemberships"}, "KeyMemberships",
			func(f NormalReturnFacts) []KeyMembershipFact { return f.KeyMemberships },
			func(f NormalReturnFacts, facts []KeyMembershipFact) NormalReturnFacts {
				f.KeyMemberships = facts
				return f
			},
			func(f KeyMembershipFact, keep NormalReturnPathPredicate) bool {
				return keepPath(f.Key, keep) || keepPath(f.Table, keep)
			}),
		normalReturnLaneDescriptor(LaneDynamicValueKeys,
			[]string{"DynamicValueKeys"}, "DynamicValueKeys",
			func(f NormalReturnFacts) []DynamicValueKeyMembershipFact { return f.DynamicValueKeys },
			func(f NormalReturnFacts, facts []DynamicValueKeyMembershipFact) NormalReturnFacts {
				f.DynamicValueKeys = facts
				return f
			},
			func(f DynamicValueKeyMembershipFact, keep NormalReturnPathPredicate) bool {
				return keepPath(f.Container, keep) || keepPath(f.Table, keep)
			}),
		normalReturnLaneDescriptor(LaneDynamicAllValues,
			nil, "DynamicAllValues",
			func(f NormalReturnFacts) []DynamicAllValueKeyMembershipFact { return f.DynamicAllValues },
			func(f NormalReturnFacts, facts []DynamicAllValueKeyMembershipFact) NormalReturnFacts {
				f.DynamicAllValues = facts
				return f
			},
			func(f DynamicAllValueKeyMembershipFact, keep NormalReturnPathPredicate) bool {
				return keepPath(f.Container, keep) || keepPath(f.Table, keep)
			}),
		normalReturnLaneDescriptor(LaneBranchProofs,
			[]string{"BranchProofs"}, "BranchProofs",
			func(f NormalReturnFacts) []BranchProof { return f.BranchProofs },
			func(f NormalReturnFacts, facts []BranchProof) NormalReturnFacts {
				f.BranchProofs = facts
				return f
			},
			func(f BranchProof, keep NormalReturnPathPredicate) bool {
				return keepPath(f.Path, keep) || keepPath(f.Other, keep)
			}),
		normalReturnLaneDescriptor(LaneChannelSelects,
			nil, "ChannelSelects",
			func(f NormalReturnFacts) []ChannelSelectFact { return f.ChannelSelects },
			func(f NormalReturnFacts, facts []ChannelSelectFact) NormalReturnFacts {
				f.ChannelSelects = facts
				return f
			},
			func(f ChannelSelectFact, keep NormalReturnPathPredicate) bool {
				return keepPath(f.Result, keep) || keepPath(f.Case, keep)
			}),
		normalReturnLaneDescriptor(LaneFrozenTables,
			[]string{"FrozenTables"}, "FrozenTables",
			func(f NormalReturnFacts) []FrozenTableFact { return f.FrozenTables },
			func(f NormalReturnFacts, facts []FrozenTableFact) NormalReturnFacts {
				f.FrozenTables = facts
				return f
			},
			func(f FrozenTableFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
		normalReturnLaneDescriptor(LaneEffectDeltas,
			nil, "EffectDeltas",
			func(f NormalReturnFacts) []EffectDelta { return f.EffectDeltas },
			func(f NormalReturnFacts, facts []EffectDelta) NormalReturnFacts {
				f.EffectDeltas = facts
				return f
			},
			func(f EffectDelta, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
		normalReturnLaneDescriptor(LaneEscapeEvents,
			[]string{"EscapeEvents", "ParamRelations"}, "EscapeEvents",
			func(f NormalReturnFacts) []EscapeEventFact { return f.EscapeEvents },
			func(f NormalReturnFacts, facts []EscapeEventFact) NormalReturnFacts {
				f.EscapeEvents = facts
				return f
			},
			func(f EscapeEventFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
		normalReturnLaneDescriptor(LaneStoreRelations,
			[]string{"StoreRelations", "ParamRelations"}, "StoreRelations",
			func(f NormalReturnFacts) []StoreRelationFact { return f.StoreRelations },
			func(f NormalReturnFacts, facts []StoreRelationFact) NormalReturnFacts {
				f.StoreRelations = facts
				return f
			},
			func(f StoreRelationFact, keep NormalReturnPathPredicate) bool {
				return keepPath(f.Source, keep) || keepPath(f.Into, keep)
			}),
		normalReturnLaneDescriptor(LaneLifecycleFacts,
			[]string{"LifecycleEffects"}, "LifecycleFacts",
			func(f NormalReturnFacts) []LifecycleFact { return f.LifecycleFacts },
			func(f NormalReturnFacts, facts []LifecycleFact) NormalReturnFacts {
				f.LifecycleFacts = facts
				return f
			},
			func(f LifecycleFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Target, keep) }),
		normalReturnLaneDescriptor(LaneNumFloors,
			nil, "NumFloors",
			func(f NormalReturnFacts) []NumFloorFact { return f.NumFloors },
			func(f NormalReturnFacts, facts []NumFloorFact) NormalReturnFacts {
				f.NumFloors = facts
				return f
			},
			func(f NumFloorFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
		normalReturnLaneDescriptor(LaneNumCeils,
			nil, "NumCeils",
			func(f NormalReturnFacts) []NumCeilFact { return f.NumCeils },
			func(f NormalReturnFacts, facts []NumCeilFact) NormalReturnFacts {
				f.NumCeils = facts
				return f
			},
			func(f NumCeilFact, keep NormalReturnPathPredicate) bool { return keepPath(f.Path, keep) }),
		normalReturnLaneDescriptor(LaneRelConstraints,
			nil, "RelConstraints",
			func(f NormalReturnFacts) []RelConstraintFact { return f.RelConstraints },
			func(f NormalReturnFacts, facts []RelConstraintFact) NormalReturnFacts {
				f.RelConstraints = facts
				return f
			},
			func(f RelConstraintFact, keep NormalReturnPathPredicate) bool {
				return keepRelOperand(f.A, keep) || keepRelOperand(f.B, keep) || keepRelOperand(f.C, keep)
			}),
	}
	t.Validate("normal-return")
	return t
}()

// NormalReturnFactDescriptors returns the descriptor-driven NormalReturnFacts
// lane table. The returned slice is a copy.
func NormalReturnFactDescriptors() BoundaryFactTable[NormalReturnLaneOps] {
	out := make(BoundaryFactTable[NormalReturnLaneOps], len(normalReturnFactDescriptors))
	copy(out, normalReturnFactDescriptors)
	return out
}

// derivedNormalReturnFactLanes is the lane slice used by normalReturnFactLanes.
func derivedNormalReturnFactLanes() []NormalReturnFactLane {
	return DeriveBoundaryLanes(normalReturnFactDescriptors, BoundaryFactDescriptor[NormalReturnLaneOps].deriveNormalReturnLane)
}
