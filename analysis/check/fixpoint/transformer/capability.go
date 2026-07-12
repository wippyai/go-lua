package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// OutputKind identifies a relation output. It is intentionally distinct from
// operationplan.Kind: source semantics and emitted outputs have different
// exhaustiveness obligations.
type OutputKind uint8

const (
	OutputReturn OutputKind = iota
	OutputObligation
	OutputEffect
	OutputCellResult
	outputKindCount
)

type Capability uint8

const (
	CapabilityUnsupported Capability = iota
	CapabilityUnaffected
	CapabilitySupported
)

// OutputCapabilityRegistry is a dense output x State-lane certificate.
type OutputCapabilityRegistry struct {
	lanes        []state.LaneID
	index        map[state.LaneID]int
	table        []Capability
	summaryKinds []callboundary.BoundaryFactKind
	summaryIndex map[callboundary.BoundaryFactKind]int
	summary      []Capability
}

func NewOutputCapabilityRegistry(catalog state.LaneCatalog) *OutputCapabilityRegistry {
	lanes := catalog.LaneSet().IDs()
	index := make(map[state.LaneID]int, len(lanes))
	for i, lane := range lanes {
		index[lane] = i
	}
	descriptors := summary.SummaryFactDescriptors()
	summaryKinds := make([]callboundary.BoundaryFactKind, len(descriptors))
	summaryIndex := make(map[callboundary.BoundaryFactKind]int, len(descriptors))
	for i, descriptor := range descriptors {
		summaryKinds[i] = descriptor.Kind
		summaryIndex[descriptor.Kind] = i
	}
	return &OutputCapabilityRegistry{
		lanes:        lanes,
		index:        index,
		table:        make([]Capability, int(outputKindCount)*len(lanes)),
		summaryKinds: summaryKinds,
		summaryIndex: summaryIndex,
		summary:      make([]Capability, len(summaryKinds)*len(lanes)),
	}
}

// DefaultOutputCapabilityRegistry enables only pure return and entry
// obligation values. Effect and scalar cell results remain unsupported on all
// 17 lanes: a call may mutate any lane, and requires an explicit relational
// composition certificate rather than a value-only callback.
func newDefaultOutputCapabilityRegistry() *OutputCapabilityRegistry {
	r := NewOutputCapabilityRegistry(state.DefaultLaneCatalog())
	for _, output := range []OutputKind{OutputReturn, OutputObligation} {
		for _, lane := range r.lanes {
			_ = r.Set(output, lane, CapabilityUnaffected)
		}
		_ = r.Set(output, state.LaneValues, CapabilitySupported)
	}
	for _, kind := range []callboundary.BoundaryFactKind{
		callboundary.BoundaryFactKind(DescriptorReturn),
		callboundary.BoundaryFactKind(DescriptorObligation),
	} {
		for _, lane := range r.lanes {
			_ = r.SetSummary(kind, lane, CapabilityUnaffected)
		}
		_ = r.SetSummary(kind, state.LaneValues, CapabilitySupported)
	}
	return r
}

var defaultOutputCapabilityRegistry = newDefaultOutputCapabilityRegistry()

// DefaultOutputCapabilityRegistry returns an independently mutable capability
// matrix over the immutable default lane and Summary schemas. The schema
// slices and indexes are shared; only the two matrices are copied per caller.
func DefaultOutputCapabilityRegistry() *OutputCapabilityRegistry {
	r := defaultOutputCapabilityRegistry
	return &OutputCapabilityRegistry{
		lanes: r.lanes, index: r.index,
		table:        append([]Capability(nil), r.table...),
		summaryKinds: r.summaryKinds, summaryIndex: r.summaryIndex,
		summary: append([]Capability(nil), r.summary...),
	}
}

func (r *OutputCapabilityRegistry) Set(output OutputKind, lane state.LaneID, capability Capability) error {
	if r == nil || output >= outputKindCount {
		return fmt.Errorf("transformer: invalid output %d", output)
	}
	i, ok := r.index[lane]
	if !ok {
		return fmt.Errorf("transformer: unknown state lane %q", lane)
	}
	if capability > CapabilitySupported {
		return fmt.Errorf("transformer: invalid capability %d", capability)
	}
	r.table[int(output)*len(r.lanes)+i] = capability
	return nil
}
func (r *OutputCapabilityRegistry) Capability(output OutputKind, lane state.LaneID) Capability {
	if r == nil || output >= outputKindCount {
		return CapabilityUnsupported
	}
	i, ok := r.index[lane]
	if !ok {
		return CapabilityUnsupported
	}
	return r.table[int(output)*len(r.lanes)+i]
}

// SetSummary records the specialization capability for one canonical Summary
// field and State lane. Unknown fields fail closed rather than creating a
// parallel output vocabulary.
func (r *OutputCapabilityRegistry) SetSummary(kind callboundary.BoundaryFactKind, lane state.LaneID, capability Capability) error {
	if r == nil {
		return fmt.Errorf("transformer: nil output capability registry")
	}
	ordinal, known := r.summaryIndex[kind]
	i, ok := r.index[lane]
	if !known {
		return fmt.Errorf("transformer: unknown Summary output %q", kind)
	}
	if !ok {
		return fmt.Errorf("transformer: unknown state lane %q", lane)
	}
	if capability > CapabilitySupported {
		return fmt.Errorf("transformer: invalid capability %d", capability)
	}
	r.summary[ordinal*len(r.lanes)+i] = capability
	return nil
}

// SummaryCapability returns the explicit verdict for a canonical Summary
// field and State lane. Unknown fields and lanes are unsupported.
func (r *OutputCapabilityRegistry) SummaryCapability(kind callboundary.BoundaryFactKind, lane state.LaneID) Capability {
	if r == nil {
		return CapabilityUnsupported
	}
	ordinal, known := r.summaryIndex[kind]
	i, ok := r.index[lane]
	if !known || !ok {
		return CapabilityUnsupported
	}
	return r.summary[ordinal*len(r.lanes)+i]
}

func (r *OutputCapabilityRegistry) UnsupportedSummary(kind callboundary.BoundaryFactKind) []state.LaneID {
	if r == nil {
		return state.DefaultLaneCatalog().LaneSet().IDs()
	}
	var out []state.LaneID
	for _, lane := range r.lanes {
		if r.SummaryCapability(kind, lane) == CapabilityUnsupported {
			out = append(out, lane)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// SummaryKinds returns the canonical Summary output schema covered by the
// matrix. The returned slice is independent of the registry.
func (r *OutputCapabilityRegistry) SummaryKinds() []callboundary.BoundaryFactKind {
	if r == nil {
		return nil
	}
	return append([]callboundary.BoundaryFactKind(nil), r.summaryKinds...)
}
func (r *OutputCapabilityRegistry) Lanes() []state.LaneID {
	if r == nil {
		return nil
	}
	return append([]state.LaneID(nil), r.lanes...)
}
func (r *OutputCapabilityRegistry) Unsupported(output OutputKind) []state.LaneID {
	if r == nil {
		return state.DefaultLaneCatalog().LaneSet().IDs()
	}
	var out []state.LaneID
	for _, lane := range r.lanes {
		if r.Capability(output, lane) == CapabilityUnsupported {
			out = append(out, lane)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func (r *OutputCapabilityRegistry) Complete(catalog state.LaneCatalog) error {
	if r == nil {
		return fmt.Errorf("transformer: nil output capability registry")
	}
	want := catalog.LaneSet().IDs()
	if len(want) != len(r.lanes) {
		return fmt.Errorf("transformer: output capability lane count %d, want %d", len(r.lanes), len(want))
	}
	for i, lane := range want {
		if r.lanes[i] != lane {
			return fmt.Errorf("transformer: output capability lane %d is %q, want %q", i, r.lanes[i], lane)
		}
	}
	if len(r.table) != int(outputKindCount)*len(r.lanes) {
		return fmt.Errorf("transformer: incomplete output capability matrix")
	}
	descriptors := summary.SummaryFactDescriptors()
	if len(descriptors) != len(r.summaryKinds) || len(r.summary) != len(descriptors)*len(r.lanes) {
		return fmt.Errorf("transformer: incomplete Summary output capability matrix")
	}
	for i, descriptor := range descriptors {
		if r.summaryKinds[i] != descriptor.Kind {
			return fmt.Errorf("transformer: stale Summary output kind catalog")
		}
	}
	return nil
}

// SemanticCapabilityRegistry is the exhaustive production
// operationplan.Kind x State-lane matrix. Every cell defaults unsupported. A
// future compiler must certify each source fact family before emitting IR.
type SemanticCapabilityRegistry struct {
	lanes          []state.LaneID
	index          map[state.LaneID]int
	factKinds      []operationplan.Kind
	factIndex      map[operationplan.Kind]int
	extensionKinds []operationplan.ExtensionKind
	extensionIndex map[operationplan.ExtensionKind]int
	facts          []Capability
	extensions     []Capability
}

func NewSemanticCapabilityRegistry(catalog state.LaneCatalog) *SemanticCapabilityRegistry {
	lanes := catalog.LaneSet().IDs()
	index := make(map[state.LaneID]int, len(lanes))
	for i, lane := range lanes {
		index[lane] = i
	}
	factKinds := operationplan.Kinds()
	factIndex := make(map[operationplan.Kind]int, len(factKinds))
	for i, kind := range factKinds {
		factIndex[kind] = i
	}
	extensionKinds := operationplan.ExtensionKinds()
	extensionIndex := make(map[operationplan.ExtensionKind]int, len(extensionKinds))
	for i, kind := range extensionKinds {
		extensionIndex[kind] = i
	}
	return &SemanticCapabilityRegistry{lanes: lanes, index: index, factKinds: factKinds, factIndex: factIndex, extensionKinds: extensionKinds, extensionIndex: extensionIndex, facts: make([]Capability, len(factKinds)*len(lanes)), extensions: make([]Capability, len(extensionKinds)*len(lanes))}
}

var defaultSemanticCapabilityRegistry = NewSemanticCapabilityRegistry(state.DefaultLaneCatalog())

// DefaultSemanticCapabilityRegistry returns an independently mutable matrix
// over the immutable default lane and operation schemas. Reusing those schemas
// avoids rebuilding four maps and three catalogs for every prepared function.
func DefaultSemanticCapabilityRegistry() *SemanticCapabilityRegistry {
	r := defaultSemanticCapabilityRegistry
	return &SemanticCapabilityRegistry{
		lanes: r.lanes, index: r.index,
		factKinds: r.factKinds, factIndex: r.factIndex,
		extensionKinds: r.extensionKinds, extensionIndex: r.extensionIndex,
		facts:      make([]Capability, len(r.facts)),
		extensions: make([]Capability, len(r.extensions)),
	}
}
func (r *SemanticCapabilityRegistry) SetFact(kind operationplan.Kind, lane state.LaneID, capability Capability) error {
	if r == nil {
		return fmt.Errorf("transformer: nil semantic capability registry")
	}
	ordinal, known := r.factIndex[kind]
	if !known {
		return fmt.Errorf("transformer: invalid operation-plan kind %d", kind)
	}
	i, ok := r.index[lane]
	if !ok {
		return fmt.Errorf("transformer: unknown state lane %q", lane)
	}
	if capability > CapabilitySupported {
		return fmt.Errorf("transformer: invalid capability %d", capability)
	}
	r.facts[ordinal*len(r.lanes)+i] = capability
	return nil
}
func (r *SemanticCapabilityRegistry) SetExtension(kind operationplan.ExtensionKind, lane state.LaneID, capability Capability) error {
	if r == nil {
		return fmt.Errorf("transformer: nil semantic capability registry")
	}
	ordinal, known := r.extensionIndex[kind]
	if !known {
		return fmt.Errorf("transformer: invalid operation-plan extension %d", kind)
	}
	i, ok := r.index[lane]
	if !ok {
		return fmt.Errorf("transformer: unknown state lane %q", lane)
	}
	if capability > CapabilitySupported {
		return fmt.Errorf("transformer: invalid capability %d", capability)
	}
	r.extensions[ordinal*len(r.lanes)+i] = capability
	return nil
}
func (r *SemanticCapabilityRegistry) Fact(kind operationplan.Kind, lane state.LaneID) Capability {
	if r == nil {
		return CapabilityUnsupported
	}
	i, ok := r.index[lane]
	ordinal, known := r.factIndex[kind]
	if !ok || !known {
		return CapabilityUnsupported
	}
	return r.facts[ordinal*len(r.lanes)+i]
}
func (r *SemanticCapabilityRegistry) Extension(kind operationplan.ExtensionKind, lane state.LaneID) Capability {
	if r == nil {
		return CapabilityUnsupported
	}
	i, ok := r.index[lane]
	ordinal, known := r.extensionIndex[kind]
	if !ok || !known {
		return CapabilityUnsupported
	}
	return r.extensions[ordinal*len(r.lanes)+i]
}
func (r *SemanticCapabilityRegistry) UnsupportedFact(kind operationplan.Kind) []state.LaneID {
	if r == nil {
		return state.DefaultLaneCatalog().LaneSet().IDs()
	}
	var out []state.LaneID
	for _, lane := range r.lanes {
		if r.Fact(kind, lane) == CapabilityUnsupported {
			out = append(out, lane)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func (r *SemanticCapabilityRegistry) UnsupportedExtension(kind operationplan.ExtensionKind) []state.LaneID {
	if r == nil {
		return state.DefaultLaneCatalog().LaneSet().IDs()
	}
	var out []state.LaneID
	for _, lane := range r.lanes {
		if r.Extension(kind, lane) == CapabilityUnsupported {
			out = append(out, lane)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func (r *SemanticCapabilityRegistry) Complete(catalog state.LaneCatalog) error {
	if r == nil {
		return fmt.Errorf("transformer: nil semantic capability registry")
	}
	want := catalog.LaneSet().IDs()
	if len(want) != len(r.lanes) {
		return fmt.Errorf("transformer: semantic capability lane count %d, want %d", len(r.lanes), len(want))
	}
	for i, lane := range want {
		if r.lanes[i] != lane {
			return fmt.Errorf("transformer: semantic capability lane %d is %q, want %q", i, r.lanes[i], lane)
		}
	}
	facts, extensions := operationplan.Kinds(), operationplan.ExtensionKinds()
	if len(facts) != len(r.factKinds) || len(extensions) != len(r.extensionKinds) || len(r.facts) != len(facts)*len(want) || len(r.extensions) != len(extensions)*len(want) {
		return fmt.Errorf("transformer: incomplete semantic capability matrix")
	}
	for i, kind := range facts {
		if r.factKinds[i] != kind {
			return fmt.Errorf("transformer: stale operation-plan kind catalog")
		}
		if _, ok := operationplan.Describe(kind); !ok {
			return fmt.Errorf("transformer: unclassified operation-plan kind %d", kind)
		}
	}
	for i, kind := range extensions {
		if r.extensionKinds[i] != kind {
			return fmt.Errorf("transformer: stale operation-plan extension catalog")
		}
	}
	return nil
}
