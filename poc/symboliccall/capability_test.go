package symboliccall

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCapabilityRegistryDerivesStateCatalogAndNewLaneFailsClosed(t *testing.T) {
	lanes := state.DefaultLanes()
	registry := DefaultBoundaryCapabilityRegistry()
	ids := registry.IDs()
	if len(ids) < len(lanes) {
		t.Fatalf("capability IDs=%d state lanes=%d", len(ids), len(lanes))
	}
	for i, lane := range lanes {
		if ids[i] != stateCapabilityID(lane) {
			t.Fatalf("capability %d=%q want state lane %q", i, ids[i], lane)
		}
	}
	unsupported := registry.unsupportedStateLanes(lanes)
	if len(unsupported) != len(lanes)-1 {
		t.Fatalf("default state capability gap=%d, want %d (all except values)", len(unsupported), len(lanes)-1)
	}
	for _, lane := range unsupported {
		if lane == state.LaneValues {
			t.Fatal("product values lane unexpectedly contextual")
		}
	}

	synthetic := state.LaneID("synthetic-future-axis")
	extended := append(append([]state.LaneID(nil), lanes...), synthetic)
	withoutImplementation := NewBoundaryCapabilityRegistry(extended, productValueCapability{})
	tr := NewEffectTransformer(standard.Registry(), 0, 0, nil, nil, EffectPolicy{
		Capabilities: withoutImplementation,
		StateLanes:   []state.LaneID{synthetic},
	})
	if tr.ContextualReason() != "unsupported state lanes: synthetic-future-axis" {
		t.Fatalf("new lane did not default fail-closed: %q", tr.ContextualReason())
	}

	withImplementation := NewBoundaryCapabilityRegistry(extended, productValueCapability{}, passthroughCapability{id: stateCapabilityID(synthetic)})
	tr = NewEffectTransformer(standard.Registry(), 0, 0, nil, nil, EffectPolicy{
		Capabilities: withImplementation,
		StateLanes:   []state.LaneID{synthetic},
	})
	if tr.ContextualReason() != "" {
		t.Fatalf("adding one lane implementation required engine changes: %q", tr.ContextualReason())
	}
	capability := withImplementation.Capability(stateCapabilityID(synthetic))
	summary, ok := capability.Summarize("lane-state")
	if !ok {
		t.Fatal("synthetic adapter summarize failed")
	}
	bound, ok := capability.Substitute(summary, CapabilityBindings{})
	if !ok || bound != "lane-state" {
		t.Fatalf("synthetic adapter substitute=%#v ok=%t", bound, ok)
	}
	joined, ok := capability.JoinEffect(nil, bound, bound)
	if !ok || joined != "lane-state" {
		t.Fatalf("synthetic adapter idempotent effect join=%#v ok=%t", joined, ok)
	}
}

func TestBoundaryCapabilitiesImplementSubstitutionAndEffectJoin(t *testing.T) {
	reg := standard.Registry()
	registry := DefaultBoundaryCapabilityRegistry()
	left := testValue(runtimekind.String, 0)
	right := testValue(runtimekind.Number, 0)
	glob := GlobalRoot{Module: "module", Name: "name"}
	bindings := CapabilityBindings{
		Registry:  reg,
		CallID:    "caller#1",
		ClosureID: "closure#1",
		Captures:  []product.Value{left},
		Globals:   map[GlobalRoot]product.Value{glob: right},
	}

	for _, test := range []struct {
		id      BoundaryCapabilityID
		summary any
		want    product.Value
	}{
		{CapabilityCaptureRoot, Capture(0), left},
		{CapabilityGlobalRoot, Global(glob.Module, glob.Name), right},
	} {
		capability := registry.Capability(test.id)
		summary, ok := capability.Summarize(test.summary)
		if !ok {
			t.Fatalf("%s failed to summarize", test.id)
		}
		value, ok := capability.Substitute(summary, bindings)
		if !ok || !product.Equal(reg, value.(product.Value), test.want) {
			t.Fatalf("%s substitution=%#v ok=%t", test.id, value, ok)
		}
	}

	allocation := registry.Capability(CapabilityAllocation)
	summary, ok := allocation.Summarize(SymbolicLocation{Kind: LocationAllocation, Site: "site"})
	if !ok {
		t.Fatal("allocation did not summarize")
	}
	bound, ok := allocation.Substitute(summary, bindings)
	if !ok || bound.(ConcreteLocation).Allocation.Call != bindings.CallID {
		t.Fatalf("allocation substitution=%#v ok=%t", bound, ok)
	}

	heap := registry.Capability(CapabilityHeapEffects)
	location := bound.(ConcreteLocation)
	joined, ok := heap.JoinEffect(reg,
		map[ConcreteLocation]product.Value{location: left},
		map[ConcreteLocation]product.Value{location: right},
	)
	if !ok || !product.Equal(reg, joined.(map[ConcreteLocation]product.Value)[location], product.Join(reg, left, right)) {
		t.Fatalf("heap effect join=%#v ok=%t", joined, ok)
	}
}

type passthroughCapability struct{ id BoundaryCapabilityID }

func (c passthroughCapability) ID() BoundaryCapabilityID { return c.id }
func (passthroughCapability) Summarize(value any) (any, bool) {
	return value, true
}
func (passthroughCapability) Substitute(summary any, _ CapabilityBindings) (any, bool) {
	return summary, true
}
func (passthroughCapability) JoinEffect(_ *axis.Registry, left, right any) (any, bool) {
	if reflect.DeepEqual(left, right) {
		return left, true
	}
	return []any{left, right}, true
}
