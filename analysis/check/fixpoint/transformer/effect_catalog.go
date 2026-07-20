package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// LaneUse is an effect's explicit relationship with one State axis. Read and
// write are separate because a lane can influence an effect without appearing
// in its output. Missing cells are invalid, never implicit Unaffected.
type LaneUse uint8

const (
	LaneUseInvalid LaneUse = iota
	LaneUseUnaffected
	LaneUseRead
	LaneUseWrite
	LaneUseReadWrite
	LaneUseUnsupported
)

type EffectDescriptor struct {
	kind          EffectKind
	sources       []operationplan.Kind
	lanes         map[state.LaneID]LaneUse
	boundaryKinds []callboundary.BoundaryFactKind
}

func NewEffectDescriptor(
	kind EffectKind,
	sources []operationplan.Kind,
	lanes map[state.LaneID]LaneUse,
	boundaryKinds []callboundary.BoundaryFactKind,
) EffectDescriptor {
	return EffectDescriptor{
		kind: kind, sources: append([]operationplan.Kind(nil), sources...),
		lanes: cloneLaneUses(lanes), boundaryKinds: append([]callboundary.BoundaryFactKind(nil), boundaryKinds...),
	}
}

func (d EffectDescriptor) Kind() EffectKind { return d.kind }
func (d EffectDescriptor) Sources() []operationplan.Kind {
	return append([]operationplan.Kind(nil), d.sources...)
}
func (d EffectDescriptor) BoundaryKinds() []callboundary.BoundaryFactKind {
	return append([]callboundary.BoundaryFactKind(nil), d.boundaryKinds...)
}
func (d EffectDescriptor) LaneUse(lane state.LaneID) LaneUse { return d.lanes[lane] }

// EffectCatalog is the exhaustive EffectKind x LaneCatalog binding plus point
// admission. The compiler uses it for effect capability admission and the
// coordinate evaluator uses the same descriptors as its exact axis footprint;
// neither side maintains an independent operation-to-lane switch.
type EffectCatalog struct {
	lanes       []state.LaneID
	descriptors map[EffectKind]EffectDescriptor
}

func NewEffectCatalog(catalog state.LaneCatalog, descriptors []EffectDescriptor) (*EffectCatalog, error) {
	return bindEffectCatalog(catalog.LaneSet().IDs(), descriptors)
}

func newDefaultEffectCatalog() *EffectCatalog {
	catalog, err := NewEffectCatalog(state.DefaultLaneCatalog(), defaultEffectDescriptors())
	if err != nil {
		panic(err)
	}
	return catalog
}

var defaultEffectCatalog = newDefaultEffectCatalog()

// DefaultEffectCatalog returns the immutable process-wide default catalog.
// Public observations copy all slice and map-backed descriptor data, so callers
// cannot mutate this shared registry.
func DefaultEffectCatalog() *EffectCatalog { return defaultEffectCatalog }

func bindEffectCatalog(lanes []state.LaneID, descriptors []EffectDescriptor) (*EffectCatalog, error) {
	out := &EffectCatalog{lanes: append([]state.LaneID(nil), lanes...), descriptors: make(map[EffectKind]EffectDescriptor, int(effectKindCount)-1)}
	seenLanes := make(map[state.LaneID]struct{}, len(lanes))
	for _, lane := range lanes {
		if _, duplicate := seenLanes[lane]; duplicate {
			return nil, fmt.Errorf("transformer: effect catalog duplicate state lane %q", lane)
		}
		seenLanes[lane] = struct{}{}
	}
	for _, descriptor := range descriptors {
		if descriptor.kind <= EffectInvalid || descriptor.kind >= effectKindCount {
			return nil, fmt.Errorf("transformer: effect catalog invalid effect kind %d", descriptor.kind)
		}
		if _, duplicate := out.descriptors[descriptor.kind]; duplicate {
			return nil, fmt.Errorf("transformer: effect catalog duplicate effect kind %d", descriptor.kind)
		}
		for _, lane := range lanes {
			use := descriptor.lanes[lane]
			if use < LaneUseUnaffected || use > LaneUseUnsupported {
				return nil, fmt.Errorf("transformer: effect %d missing state lane %q", descriptor.kind, lane)
			}
		}
		for lane := range descriptor.lanes {
			if _, known := seenLanes[lane]; !known {
				return nil, fmt.Errorf("transformer: effect %d has orphan state lane %q", descriptor.kind, lane)
			}
		}
		if err := validateEffectSources(descriptor); err != nil {
			return nil, err
		}
		out.descriptors[descriptor.kind] = NewEffectDescriptor(descriptor.kind, descriptor.sources, descriptor.lanes, descriptor.boundaryKinds)
	}
	for kind := EffectInvalidatePath; kind < effectKindCount; kind++ {
		if _, ok := out.descriptors[kind]; !ok {
			return nil, fmt.Errorf("transformer: effect catalog missing effect kind %d", kind)
		}
	}
	return out, nil
}

func validateEffectSources(descriptor EffectDescriptor) error {
	seen := make(map[operationplan.Kind]struct{}, len(descriptor.sources))
	for _, source := range descriptor.sources {
		if _, ok := operationplan.Describe(source); !ok {
			return fmt.Errorf("transformer: effect %d has unknown operation source %d", descriptor.kind, source)
		}
		if _, duplicate := seen[source]; duplicate {
			return fmt.Errorf("transformer: effect %d has duplicate operation source %s", descriptor.kind, source)
		}
		seen[source] = struct{}{}
	}
	want := map[EffectKind][]operationplan.Kind{
		EffectInvalidatePath:        {operationplan.PathDescendantInvalidation},
		EffectIndexMutation:         {operationplan.PathDescendantInvalidation, operationplan.DynamicIndexWrite},
		EffectAllocationTemplate:    {operationplan.CallSite},
		EffectObjectMaterialization: {operationplan.ObjectLiteral},
		EffectPathStore:             {operationplan.PathAssignment, operationplan.PathStaticMemberWrite},
	}[descriptor.kind]
	if !sameOperationKinds(descriptor.sources, want) {
		return fmt.Errorf("transformer: effect %d operation sources %v, want atomic %v", descriptor.kind, descriptor.sources, want)
	}
	return nil
}

func sameOperationKinds(left, right []operationplan.Kind) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[operationplan.Kind]int, len(left))
	for _, kind := range left {
		counts[kind]++
	}
	for _, kind := range right {
		counts[kind]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func (c *EffectCatalog) Descriptor(kind EffectKind) (EffectDescriptor, bool) {
	if c == nil {
		return EffectDescriptor{}, false
	}
	descriptor, ok := c.descriptors[kind]
	if !ok {
		return EffectDescriptor{}, false
	}
	return NewEffectDescriptor(descriptor.kind, descriptor.sources, descriptor.lanes, descriptor.boundaryKinds), true
}

func (c *EffectCatalog) Lanes() []state.LaneID {
	if c == nil {
		return nil
	}
	return append([]state.LaneID(nil), c.lanes...)
}

type EffectAdmission struct {
	Kind     EffectKind
	Consumes []operationplan.Kind
}

// AdmitPoint recognizes the canonical atomic point-effect families. Paired
// sources are admitted together; a partial pair fails closed.
func (c *EffectCatalog) AdmitPoint(active []operationplan.Kind) (EffectAdmission, bool, error) {
	if c == nil {
		return EffectAdmission{}, false, fmt.Errorf("transformer: nil effect catalog")
	}
	counts := make(map[operationplan.Kind]int)
	for _, kind := range active {
		if kind == operationplan.PathDescendantInvalidation || kind == operationplan.DynamicIndexWrite || kind == operationplan.PathAssignment || kind == operationplan.PathStaticMemberWrite {
			counts[kind]++
			if counts[kind] > 1 {
				return EffectAdmission{}, false, fmt.Errorf("transformer: duplicate point effect source %s", kind)
			}
		}
	}
	hasInvalidation := counts[operationplan.PathDescendantInvalidation] == 1
	hasWrite := counts[operationplan.DynamicIndexWrite] == 1
	hasPathAssignment := counts[operationplan.PathAssignment] == 1
	hasStaticWrite := counts[operationplan.PathStaticMemberWrite] == 1
	if hasPathAssignment != hasStaticWrite {
		return EffectAdmission{}, false, fmt.Errorf("transformer: path store requires paired assignment and static-member sources")
	}
	if hasPathAssignment {
		if hasInvalidation || hasWrite {
			return EffectAdmission{}, false, fmt.Errorf("transformer: multiple atomic point effects require ordered multi-effect admission")
		}
		descriptor := c.descriptors[EffectPathStore]
		return EffectAdmission{Kind: descriptor.kind, Consumes: descriptor.Sources()}, true, nil
	}
	switch {
	case hasWrite && !hasInvalidation:
		return EffectAdmission{}, false, fmt.Errorf("transformer: dynamic index write missing atomic path invalidation barrier")
	case hasWrite:
		descriptor := c.descriptors[EffectIndexMutation]
		return EffectAdmission{Kind: descriptor.kind, Consumes: descriptor.Sources()}, true, nil
	case hasInvalidation:
		descriptor := c.descriptors[EffectInvalidatePath]
		return EffectAdmission{Kind: descriptor.kind, Consumes: descriptor.Sources()}, true, nil
	default:
		return EffectAdmission{}, false, nil
	}
}

func defaultEffectDescriptors() []EffectDescriptor {
	invalidates := baseEffectLaneUses()
	for _, lane := range []state.LaneID{
		state.LaneValues, state.LanePathEvidence, state.LaneDynamicIndex, state.LaneHeapTableIdentity,
		state.LaneKeyMemberships, state.LaneLenFloors, state.LaneDiffRelations,
	} {
		invalidates[lane] = LaneUseReadWrite
	}
	mutation := cloneLaneUses(invalidates)
	mutation[state.LanePlacement] = LaneUseReadWrite
	mutation[state.LaneTypestates] = LaneUseReadWrite
	mutation[state.LaneEffectDeltas] = LaneUseReadWrite
	allocation := baseEffectLaneUses()
	allocation[state.LaneHeapTableIdentity] = LaneUseReadWrite
	allocation[state.LanePlacement] = LaneUseReadWrite
	objectMaterialization := baseEffectLaneUses()
	objectMaterialization[state.LaneValues] = LaneUseRead
	objectMaterialization[state.LaneHeapTableIdentity] = LaneUseWrite
	objectMaterialization[state.LanePlacement] = LaneUseReadWrite
	pathStore := baseEffectLaneUses()
	for _, lane := range []state.LaneID{
		state.LaneValues, state.LanePathEvidence, state.LaneDynamicIndex,
		state.LaneHeapTableIdentity, state.LaneKeyMemberships, state.LaneTypestates,
		state.LanePlacement, state.LaneLenFloors, state.LaneUserLattices,
	} {
		pathStore[lane] = LaneUseReadWrite
	}
	return []EffectDescriptor{
		NewEffectDescriptor(EffectInvalidatePath,
			[]operationplan.Kind{operationplan.PathDescendantInvalidation}, invalidates,
			[]callboundary.BoundaryFactKind{callboundary.BoundaryFactKind(callboundary.LanePathInvalidations)}),
		NewEffectDescriptor(EffectIndexMutation,
			[]operationplan.Kind{operationplan.PathDescendantInvalidation, operationplan.DynamicIndexWrite}, mutation,
			[]callboundary.BoundaryFactKind{
				callboundary.BoundaryFactKind(callboundary.LanePathInvalidations),
				callboundary.BoundaryFactKind(callboundary.LanePathStaticMembers),
				callboundary.BoundaryFactKind(callboundary.LaneDynamicIndexFacts),
				callboundary.BoundaryFactKind(callboundary.LaneKeyMemberships),
				callboundary.BoundaryFactKind(callboundary.LaneDynamicValueKeys),
				callboundary.BoundaryFactKind(callboundary.LaneDynamicAllValues),
				callboundary.BoundaryFactKind(callboundary.LaneRelConstraints),
				callboundary.BoundaryFactKind("HeapTableObjects"),
				callboundary.BoundaryFactKind("FreshHeapAllocations"),
			}),
		NewEffectDescriptor(EffectAllocationTemplate,
			[]operationplan.Kind{operationplan.CallSite}, allocation,
			[]callboundary.BoundaryFactKind{
				callboundary.BoundaryFactKind("Returns"),
				callboundary.BoundaryFactKind("HeapTableObjects"),
				callboundary.BoundaryFactKind("FreshHeapAllocations"),
				callboundary.BoundaryFactKind("HeapKeySpace"),
			}),
		NewEffectDescriptor(EffectObjectMaterialization,
			[]operationplan.Kind{operationplan.ObjectLiteral}, objectMaterialization,
			[]callboundary.BoundaryFactKind{callboundary.BoundaryFactKind("HeapTableObjects")}),
		NewEffectDescriptor(EffectPathStore,
			[]operationplan.Kind{operationplan.PathAssignment, operationplan.PathStaticMemberWrite}, pathStore,
			[]callboundary.BoundaryFactKind{
				callboundary.BoundaryFactKind(callboundary.LanePathStaticMembers),
				callboundary.BoundaryFactKind(callboundary.LaneBranchProofs),
				callboundary.BoundaryFactKind("HeapTableObjects"),
			}),
	}
}

// baseEffectLaneUses spells every current lane explicitly. A LaneCatalog growth
// therefore fails binding until this architecture decision is updated.
func baseEffectLaneUses() map[state.LaneID]LaneUse {
	return map[state.LaneID]LaneUse{
		state.LaneValues: LaneUseUnaffected, state.LanePathEvidence: LaneUseUnaffected,
		state.LaneDynamicIndex: LaneUseUnaffected, state.LaneHeapTableIdentity: LaneUseUnaffected,
		state.LaneFrozenTables: LaneUseUnaffected, state.LaneEffectDeltas: LaneUseUnaffected,
		state.LaneEscapeEvents: LaneUseUnaffected, state.LaneChannelSelect: LaneUseUnaffected,
		state.LaneStoreRelations: LaneUseUnaffected, state.LaneKeyMemberships: LaneUseUnaffected,
		state.LaneTypestates: LaneUseUnaffected, state.LanePlacement: LaneUseUnaffected,
		state.LaneLenFloors: LaneUseUnaffected, state.LaneNumFloors: LaneUseUnaffected,
		state.LaneNumCeils: LaneUseUnaffected, state.LaneDiffRelations: LaneUseUnaffected,
		state.LaneUserLattices: LaneUseUnaffected,
	}
}

func cloneLaneUses(in map[state.LaneID]LaneUse) map[state.LaneID]LaneUse {
	out := make(map[state.LaneID]LaneUse, len(in))
	for lane, use := range in {
		out[lane] = use
	}
	return out
}
