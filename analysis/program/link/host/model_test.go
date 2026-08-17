package host

import "testing"

func TestHostColdSpecIsDetachedFromComponentStorage(t *testing.T) {
	project, boundary, module := hostFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Module: module, Spec: Spec{
		ProviderCapabilities: []ProviderCapabilitySpec{{Identity: "global"}},
		ProviderCapabilitySeeds: []ProviderCapabilitySeedSpec{{
			Capability: "global", Source: ProviderCapabilitySourceInitialRoot, InitialRoot: "GlobalEnvRoot",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	cold, ok := component.Cold().ReplaySpec()
	if !ok || len(cold.Capabilities) != 1 {
		t.Fatalf("Host replay spec = %#v/%t", cold, ok)
	}
	cold.Capabilities[0] = "mutated"
	again, ok := component.Cold().ReplaySpec()
	if !ok || again.Capabilities[0] != "global" {
		t.Fatal("Host Cold replay spec leaked mutable storage")
	}
}
