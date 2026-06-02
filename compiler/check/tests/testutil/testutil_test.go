package testutil

import "testing"

func TestCheckAndExportHonorsEnvCanonicalFlow(t *testing.T) {
	t.Setenv("WIPPY_FLOW", "canonical")

	mod := CheckAndExport("return {}", "env_flow_probe")
	if mod == nil || mod.Session == nil || mod.Session.RootResultValue() == nil {
		t.Fatal("CheckAndExport did not produce a root analysis result")
	}
	if mod.Session.RootResultValue().FlowSolution != nil {
		t.Fatal("CheckAndExport ignored WIPPY_FLOW=canonical and ran the legacy flow")
	}
}

func TestDefaultFlowIsCanonical(t *testing.T) {
	mod := CheckAndExport("return {}", "default_flow_probe")
	if mod == nil || mod.Session == nil || mod.Session.RootResultValue() == nil {
		t.Fatal("CheckAndExport did not produce a root analysis result")
	}
	if mod.Session.RootResultValue().FlowSolution != nil {
		t.Fatal("default checker flow ran legacy; want canonical")
	}
}

func TestCheckAndExportHonorsEnvLegacyFlow(t *testing.T) {
	t.Setenv("WIPPY_FLOW", "legacy")

	mod := CheckAndExport("return {}", "legacy_flow_probe")
	if mod == nil || mod.Session == nil || mod.Session.RootResultValue() == nil {
		t.Fatal("CheckAndExport did not produce a root analysis result")
	}
	if mod.Session.RootResultValue().FlowSolution == nil {
		t.Fatal("CheckAndExport ignored WIPPY_FLOW=legacy and ran canonical")
	}
}
