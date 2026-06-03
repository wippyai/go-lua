package testutil

import "testing"

func TestCheckAndExportIgnoresFlowEnv(t *testing.T) {
	t.Setenv("WIPPY_FLOW", "normal")

	mod := CheckAndExport("return {}", "env_flow_probe")
	if mod == nil || mod.Session == nil || mod.Session.RootResultValue() == nil {
		t.Fatal("CheckAndExport did not produce a root analysis result")
	}
	if mod.Session.RootResultValue().FlowSolution != nil {
		t.Fatal("CheckAndExport did not run the normal flow")
	}
}

func TestDefaultFlowIsCanonical(t *testing.T) {
	mod := CheckAndExport("return {}", "default_flow_probe")
	if mod == nil || mod.Session == nil || mod.Session.RootResultValue() == nil {
		t.Fatal("CheckAndExport did not produce a root analysis result")
	}
	if mod.Session.RootResultValue().FlowSolution != nil {
		t.Fatal("default checker flow ran removed legacy machinery")
	}
}

func TestLegacyFlowEnvCannotSelectOldPipeline(t *testing.T) {
	t.Setenv("WIPPY_FLOW", "legacy")

	mod := CheckAndExport("return {}", "legacy_flow_probe")
	if mod == nil || mod.Session == nil || mod.Session.RootResultValue() == nil {
		t.Fatal("CheckAndExport did not produce a root analysis result")
	}
	if mod.Session.RootResultValue().FlowSolution != nil {
		t.Fatal("WIPPY_FLOW=legacy selected the removed legacy flow")
	}
}
