package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
)

type EffectDescriptor struct {
	kind          EffectKind
	sources       []operationplan.Kind
	boundaryKinds []callboundary.BoundaryFactKind
}

func NewEffectDescriptor(
	kind EffectKind,
	sources []operationplan.Kind,
	boundaryKinds []callboundary.BoundaryFactKind,
) EffectDescriptor {
	return EffectDescriptor{
		kind: kind, sources: append([]operationplan.Kind(nil), sources...),
		boundaryKinds: append([]callboundary.BoundaryFactKind(nil), boundaryKinds...),
	}
}

func (d EffectDescriptor) Kind() EffectKind { return d.kind }
func (d EffectDescriptor) Sources() []operationplan.Kind {
	return append([]operationplan.Kind(nil), d.sources...)
}
func (d EffectDescriptor) BoundaryKinds() []callboundary.BoundaryFactKind {
	return append([]callboundary.BoundaryFactKind(nil), d.boundaryKinds...)
}

// EffectCatalog owns only the structured Effect syntax: atomic source
// admission and boundary fact authority. ProductDomain transaction programs
// own State-axis access; syntax must not redeclare that product matrix.
type EffectCatalog struct {
	descriptors map[EffectKind]EffectDescriptor
}

func NewEffectCatalog(descriptors []EffectDescriptor) (*EffectCatalog, error) {
	return bindEffectCatalog(descriptors)
}

func newDefaultEffectCatalog() *EffectCatalog {
	catalog, err := NewEffectCatalog(defaultEffectDescriptors())
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

func bindEffectCatalog(descriptors []EffectDescriptor) (*EffectCatalog, error) {
	out := &EffectCatalog{descriptors: make(map[EffectKind]EffectDescriptor, int(effectKindCount)-1)}
	for _, descriptor := range descriptors {
		if descriptor.kind <= EffectInvalid || descriptor.kind >= effectKindCount {
			return nil, fmt.Errorf("transformer: effect catalog invalid effect kind %d", descriptor.kind)
		}
		if _, duplicate := out.descriptors[descriptor.kind]; duplicate {
			return nil, fmt.Errorf("transformer: effect catalog duplicate effect kind %d", descriptor.kind)
		}
		if err := validateEffectSources(descriptor); err != nil {
			return nil, err
		}
		out.descriptors[descriptor.kind] = NewEffectDescriptor(descriptor.kind, descriptor.sources, descriptor.boundaryKinds)
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
	return NewEffectDescriptor(descriptor.kind, descriptor.sources, descriptor.boundaryKinds), true
}

// OwnsSource reports whether kind is part of any registered atomic Effect.
// State-axis support is deliberately not answered here.
func (c *EffectCatalog) OwnsSource(kind operationplan.Kind) bool {
	if c == nil {
		return false
	}
	for _, descriptor := range c.descriptors {
		for _, source := range descriptor.sources {
			if source == kind {
				return true
			}
		}
	}
	return false
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
	return []EffectDescriptor{
		NewEffectDescriptor(EffectInvalidatePath,
			[]operationplan.Kind{operationplan.PathDescendantInvalidation},
			[]callboundary.BoundaryFactKind{callboundary.BoundaryFactKind(callboundary.LanePathInvalidations)}),
		NewEffectDescriptor(EffectIndexMutation,
			[]operationplan.Kind{operationplan.PathDescendantInvalidation, operationplan.DynamicIndexWrite},
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
			[]operationplan.Kind{operationplan.CallSite},
			[]callboundary.BoundaryFactKind{
				callboundary.BoundaryFactKind("Returns"),
				callboundary.BoundaryFactKind("HeapTableObjects"),
				callboundary.BoundaryFactKind("FreshHeapAllocations"),
				callboundary.BoundaryFactKind("HeapKeySpace"),
			}),
		NewEffectDescriptor(EffectObjectMaterialization,
			[]operationplan.Kind{operationplan.ObjectLiteral},
			[]callboundary.BoundaryFactKind{callboundary.BoundaryFactKind("HeapTableObjects")}),
		NewEffectDescriptor(EffectPathStore,
			[]operationplan.Kind{operationplan.PathAssignment, operationplan.PathStaticMemberWrite},
			[]callboundary.BoundaryFactKind{
				callboundary.BoundaryFactKind(callboundary.LanePathStaticMembers),
				callboundary.BoundaryFactKind(callboundary.LaneBranchProofs),
				callboundary.BoundaryFactKind("HeapTableObjects"),
			}),
	}
}
