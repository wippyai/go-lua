package link

import (
	"fmt"
	"testing"

	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// This is a semantic scale law, not a timing benchmark. A selector in every
// shard must retain exactly its endpoint under one Seal, which exercises the
// sparse-many-shard construction without making a machine-speed assertion.
func TestHostSelectorScalePreservesEveryShardRelation(t *testing.T) {
	const shardCount = 513
	binding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"host"}, Member: []string{"invoke"}}
	contract := contract(t, binding)
	p := source(t, `host()`)
	access := hostGlobalRead(t, p, "host")
	modules := make([]linkproject.Module, 0, shardCount)
	exposures := make([]linkhost.HostExposureSpec, 0, shardCount)
	for index := 0; index < shardCount; index++ {
		name := fmt.Sprintf("module-%04d", index)
		modules = append(modules, linkproject.Module{Name: name, Program: p})
		exposures = append(exposures, linkhost.HostExposureSpec{Module: name, Access: access, Endpoint: "host.invoke", Dispatch: linkhost.HostDispatchLookup})
	}
	link, err := Seal(&Spec{
		Target:           contract,
		Modules:          modules,
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "host.invoke", Binding: binding}},
		Host:             linkhost.Spec{Exposures: exposures},
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := contract.Lookup(binding)
	if !ok {
		t.Fatal("missing fixture provider operation")
	}
	if link.Project().Mounts().Count() != shardCount {
		t.Fatalf("shards = %d, want %d", link.Project().Mounts().Count(), shardCount)
	}
	seenOutputs := make(map[linkboundary.Value]struct{}, shardCount)
	for index := 0; index < shardCount; index++ {
		shard, ok := link.Project().Mounts().At(index)
		if !ok || link.Host().Exposures().EndpointCount(shard, access) != 1 {
			t.Fatalf("shard %d lost its exact host exposure", index)
		}
		endpoint, ok := link.Host().Exposures().EndpointAt(shard, access, 0)
		if !ok {
			t.Fatalf("shard %d has no host endpoint", index)
		}
		if got, ok := link.Boundary().Endpoints().Operation(endpoint); !ok || got != operation {
			t.Fatalf("shard %d endpoint target = %v/%t, want %v", index, got, ok, operation)
		}
		output, selected, dispatch, ok := link.Host().Exposures().SelectorAt(shard, access, 0)
		if !ok || selected != endpoint || dispatch != linkhost.HostDispatchLookup {
			t.Fatalf("shard %d selector lost exact output/endpoint/dispatch", index)
		}
		if _, duplicate := seenOutputs[output]; duplicate {
			t.Fatalf("shard %d reused another shard's same-ordinal access Value", index)
		}
		seenOutputs[output] = struct{}{}
		owner, origin, ok := link.Boundary().Values().Origin(output)
		if !ok || owner != shard || origin != access {
			t.Fatalf("shard %d output origin = %v/%d/%t, want %v/%d", index, owner, origin, ok, shard, access)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		shard, _ := link.Project().Mounts().At(shardCount - 1)
		_, _ = link.Host().Exposures().EndpointAt(shard, access, 0)
	}); allocations != 0 {
		t.Fatalf("sealed scale selector query allocates %v", allocations)
	}
}

// Two rows with the same (shard, access, capability, key) force host selector
// normalization through Project.Keys.Compare before the distinct Boundary
// endpoint tie-breaker. This keeps the selector's key coordinate fenced to
// the exact finalized Project published before Host construction.
func TestHostSelectorNormalizationUsesExactProjectKeys(t *testing.T) {
	binding := target.BindingSpec{Namespace: target.BindingProvider, Owner: []string{"actor"}, Member: []string{"send"}}
	contract := contract(t, binding)
	p := source(t, `actor.send(1)`)
	actor := hostGlobalRead(t, p, "actor")
	member := artifactMemberRead(t, p)
	linked, err := Seal(&Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "actor.send.a", Binding: binding}, {Identity: "actor.send.b", Binding: binding}}, Host: linkhost.Spec{
		ProviderCapabilities:    []linkhost.ProviderCapabilitySpec{{Identity: "actor"}},
		ProviderCapabilitySeeds: []linkhost.ProviderCapabilitySeedSpec{{Capability: "actor", Source: linkhost.ProviderCapabilitySourceExposure, Module: "main", Access: actor}},
		Members: []linkhost.HostMemberSpec{
			{Module: "main", Access: member, Capability: "actor", Endpoint: "actor.send.b", Dispatch: linkhost.HostDispatchLookup},
			{Module: "main", Access: member, Capability: "actor", Endpoint: "actor.send.a", Dispatch: linkhost.HostDispatchLookup},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	shard, ok := linked.Project().Mounts().At(0)
	if !ok || linked.Host().Members().EndpointCount(shard, member) != 2 {
		t.Fatal("host member normalization lost an exact selector row")
	}
	_, key, _, first, _, ok := linked.Host().Members().SelectorAt(shard, member, 0)
	if !ok {
		t.Fatal("first normalized host member selector unavailable")
	}
	if _, ok := linked.Project().Keys().Index(key); !ok {
		t.Fatal("host selector retained a key outside its exact Project authority")
	}
	_, _, _, second, _, ok := linked.Host().Members().SelectorAt(shard, member, 1)
	if !ok || first == second {
		t.Fatal("distinct Boundary endpoints collapsed during host normalization")
	}
	endpoints := linked.Boundary().Endpoints()
	for _, endpoint := range []linkboundary.Endpoint{first, second} {
		if op, ok := endpoints.Operation(endpoint); !ok || op == 0 {
			t.Fatal("host selector endpoint escaped its Boundary authority")
		}
	}
}
