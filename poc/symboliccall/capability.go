package symboliccall

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// BoundaryCapabilityID names either an engine State lane or one symbolic
// boundary component. State IDs are derived from state.DefaultLanes; adding a
// new State lane therefore creates a contextual capability automatically.
type BoundaryCapabilityID string

const (
	CapabilityCaptureRoot BoundaryCapabilityID = "root:capture"
	CapabilityGlobalRoot  BoundaryCapabilityID = "root:global"
	CapabilityAllocation  BoundaryCapabilityID = "effect:allocation"
	CapabilityHeapEffects BoundaryCapabilityID = "effect:heap"
)

func stateCapabilityID(lane state.LaneID) BoundaryCapabilityID {
	return BoundaryCapabilityID("state:" + string(lane))
}

type CapabilityBindings struct {
	Registry  *axis.Registry
	CallID    string
	ClosureID string
	Params    []product.Value
	Captures  []product.Value
	Varargs   []product.Value
	Globals   map[GlobalRoot]product.Value
}

// BoundaryCapability is the modular transformer seam. A lane implementation
// must be able to summarize, substitute caller bindings, and join effects. A
// false result is an atomic contextual fallback, never a guessed projection.
type BoundaryCapability interface {
	ID() BoundaryCapabilityID
	Summarize(value any) (summary any, ok bool)
	Substitute(summary any, bindings CapabilityBindings) (value any, ok bool)
	JoinEffect(reg *axis.Registry, left, right any) (joined any, ok bool)
}

type BoundaryCapabilityRegistry struct {
	order        []BoundaryCapabilityID
	capabilities map[BoundaryCapabilityID]BoundaryCapability
}

// NewBoundaryCapabilityRegistry creates entries for every supplied State lane.
// Implementations are opt-in; omitted lanes retain a contextual capability.
func NewBoundaryCapabilityRegistry(lanes []state.LaneID, implementations ...BoundaryCapability) *BoundaryCapabilityRegistry {
	registry := &BoundaryCapabilityRegistry{capabilities: make(map[BoundaryCapabilityID]BoundaryCapability)}
	seen := make(map[BoundaryCapabilityID]struct{})
	add := func(id BoundaryCapabilityID) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		registry.order = append(registry.order, id)
		registry.capabilities[id] = contextualBoundaryCapability{id: id}
	}
	for _, lane := range lanes {
		add(stateCapabilityID(lane))
	}
	for _, id := range []BoundaryCapabilityID{
		CapabilityCaptureRoot,
		CapabilityGlobalRoot,
		CapabilityAllocation,
		CapabilityHeapEffects,
	} {
		add(id)
	}
	for _, implementation := range implementations {
		if implementation == nil {
			continue
		}
		id := implementation.ID()
		add(id)
		registry.capabilities[id] = implementation
	}
	return registry
}

func DefaultBoundaryCapabilityRegistry() *BoundaryCapabilityRegistry {
	return NewBoundaryCapabilityRegistry(
		state.DefaultLanes(),
		productValueCapability{},
		rootValueCapability{id: CapabilityCaptureRoot},
		rootValueCapability{id: CapabilityGlobalRoot},
		allocationCapability{},
		heapEffectCapability{},
	)
}

func (r *BoundaryCapabilityRegistry) Capability(id BoundaryCapabilityID) BoundaryCapability {
	if r == nil {
		return contextualBoundaryCapability{id: id}
	}
	if capability, ok := r.capabilities[id]; ok {
		return capability
	}
	return contextualBoundaryCapability{id: id}
}

func (r *BoundaryCapabilityRegistry) IDs() []BoundaryCapabilityID {
	if r == nil {
		return nil
	}
	return append([]BoundaryCapabilityID(nil), r.order...)
}

func (r *BoundaryCapabilityRegistry) unsupportedStateLanes(lanes []state.LaneID) []state.LaneID {
	var unsupported []state.LaneID
	for _, lane := range lanes {
		capability := r.Capability(stateCapabilityID(lane))
		if _, contextual := capability.(contextualBoundaryCapability); contextual {
			unsupported = append(unsupported, lane)
		}
	}
	sort.Slice(unsupported, func(i, j int) bool { return unsupported[i] < unsupported[j] })
	return unsupported
}

type contextualBoundaryCapability struct{ id BoundaryCapabilityID }

func (c contextualBoundaryCapability) ID() BoundaryCapabilityID { return c.id }
func (contextualBoundaryCapability) Summarize(any) (any, bool)  { return nil, false }
func (contextualBoundaryCapability) Substitute(any, CapabilityBindings) (any, bool) {
	return nil, false
}
func (contextualBoundaryCapability) JoinEffect(*axis.Registry, any, any) (any, bool) {
	return nil, false
}

type productValueCapability struct{}

func (productValueCapability) ID() BoundaryCapabilityID { return stateCapabilityID(state.LaneValues) }
func (productValueCapability) Summarize(value any) (any, bool) {
	productValue, ok := value.(product.Value)
	return productValue, ok
}
func (productValueCapability) Substitute(summary any, _ CapabilityBindings) (any, bool) {
	productValue, ok := summary.(product.Value)
	return productValue, ok
}
func (productValueCapability) JoinEffect(reg *axis.Registry, left, right any) (any, bool) {
	a, aOK := left.(product.Value)
	b, bOK := right.(product.Value)
	if !aOK || !bOK || reg == nil {
		return nil, false
	}
	return product.Join(reg, a, b), true
}

type rootValueCapability struct{ id BoundaryCapabilityID }

func (c rootValueCapability) ID() BoundaryCapabilityID { return c.id }
func (rootValueCapability) Summarize(value any) (any, bool) {
	expr, ok := value.(Expr)
	return expr, ok
}
func (rootValueCapability) Substitute(summary any, bindings CapabilityBindings) (any, bool) {
	expr, ok := summary.(Expr)
	if !ok || bindings.Registry == nil {
		return nil, false
	}
	value, err := evalEnvironment(bindings.Registry, expr, bindings.Params, bindings.Captures, bindings.Varargs, bindings.Globals)
	return value, err == nil
}
func (rootValueCapability) JoinEffect(reg *axis.Registry, left, right any) (any, bool) {
	return productValueCapability{}.JoinEffect(reg, left, right)
}

type allocationCapability struct{}

func (allocationCapability) ID() BoundaryCapabilityID { return CapabilityAllocation }
func (allocationCapability) Summarize(value any) (any, bool) {
	location, ok := value.(SymbolicLocation)
	return location, ok && location.Kind == LocationAllocation
}
func (allocationCapability) Substitute(summary any, bindings CapabilityBindings) (any, bool) {
	location, ok := summary.(SymbolicLocation)
	if !ok || location.Kind != LocationAllocation || bindings.CallID == "" {
		return nil, false
	}
	concrete, _, err := resolveLocation(bindings.CallID, bindings.ClosureID, location)
	return concrete, err == nil
}
func (allocationCapability) JoinEffect(_ *axis.Registry, left, right any) (any, bool) {
	a, aOK := left.(ConcreteLocation)
	b, bOK := right.(ConcreteLocation)
	return a, aOK && bOK && a == b
}

type heapEffectCapability struct{}

func (heapEffectCapability) ID() BoundaryCapabilityID { return CapabilityHeapEffects }
func (heapEffectCapability) Summarize(value any) (any, bool) {
	heap, ok := value.(map[ConcreteLocation]product.Value)
	if !ok {
		return nil, false
	}
	return cloneHeap(heap), true
}
func (heapEffectCapability) Substitute(summary any, _ CapabilityBindings) (any, bool) {
	heap, ok := summary.(map[ConcreteLocation]product.Value)
	if !ok {
		return nil, false
	}
	return cloneHeap(heap), true
}
func (heapEffectCapability) JoinEffect(reg *axis.Registry, left, right any) (any, bool) {
	a, aOK := left.(map[ConcreteLocation]product.Value)
	b, bOK := right.(map[ConcreteLocation]product.Value)
	if !aOK || !bOK || reg == nil {
		return nil, false
	}
	out := cloneHeap(a)
	for location, value := range b {
		if prior, ok := out[location]; ok {
			out[location] = product.Join(reg, prior, value)
		} else {
			out[location] = value
		}
	}
	return out, true
}

func unsupportedLaneReason(lanes []state.LaneID) string {
	parts := make([]string, len(lanes))
	for i, lane := range lanes {
		parts[i] = string(lane)
	}
	return fmt.Sprintf("unsupported state lanes: %s", strings.Join(parts, ","))
}
