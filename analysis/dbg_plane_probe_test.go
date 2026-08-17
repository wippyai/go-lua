package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

func dbgRunFixture(t *testing.T, name string) {
	t.Helper()
	linked := fixtureLink(t, name)
	engine.DbgPlaneReset()
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil {
		t.Fatalf("Analyze %s = %v result=%t", name, status, result != nil)
	}
	engine.DbgPlaneDump(name)
}

func TestDbgPlaneEdgeMatrix(t *testing.T) {
	dbgRunFixture(t, "semantic/type-engine-edge-matrix")
}

func TestDbgPlaneActorSupervisor(t *testing.T) {
	dbgRunFixture(t, "semantic/actor-supervisor-recursive-app")
}

func TestDbgPlaneChannelLifecycle(t *testing.T) {
	dbgRunFixture(t, "semantic/channel-lifecycle-typestate")
}
