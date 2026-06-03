package testutil

import "testing"

func TestDefaultFlowProducesSolvedProjection(t *testing.T) {
	mod := CheckAndExport("return {}", "default_flow_probe")
	if mod == nil || mod.Session == nil || mod.Session.RootResultValue() == nil {
		t.Fatal("CheckAndExport did not produce a root analysis result")
	}
	if mod.Session.RootResultValue().SolvedFlow() == nil {
		t.Fatal("default checker flow did not expose solved flow projection")
	}
}
