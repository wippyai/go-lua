package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestContextTransportTargetsOnlyAuthenticatedStateContext proves the runtime
// cut consumes the sealed (source StateOrdinal, target StateOrdinal) pair. A
// sibling context that carries the same graph Point is not a source or target
// substitute, even when its carrier scope is otherwise identical.
func TestContextTransportTargetsOnlyAuthenticatedStateContext(t *testing.T) {
	statePlane := newStatePlaneLawFixture(t, 2)
	carrierFixture := newNewtonLawFixture(t, 1)
	plan := carrierFixture.transport(t, map[guard.Atom]guard.Atom{carrierFixture.coordinates[0]: carrierFixture.coordinates[0]})
	whole := carrierFixture.whole
	from, fromOK := statePlane.plan.Lookup(0, contextfiber.PointOrdinal(0))
	to, toOK := statePlane.plan.Lookup(1, contextfiber.PointOrdinal(1))
	wrongContextTarget, wrongContextTargetOK := statePlane.plan.Lookup(0, contextfiber.PointOrdinal(1))
	unrelatedSource, unrelatedSourceOK := statePlane.plan.Lookup(1, contextfiber.PointOrdinal(0))
	if !fromOK || !toOK || !wrongContextTargetOK || !unrelatedSourceOK {
		t.Fatal("context transport state addresses")
	}
	work, workOK := carrierFixture.composition.NewWork()
	if !workOK {
		t.Fatal("context transport work")
	}
	defer work.Close()
	state, stateOK := carrier.NewState(carrierFixture.composition, carrierFixture.scope, whole)
	source, sourceOK := work.EmptyPointState(state)
	if !stateOK || !sourceOK {
		t.Fatal("context transport source PointState")
	}
	runtime := &solverRuntime{
		carrier:        carrierFixture.composition,
		graph:          statePlane.graph,
		artifactBacked: true,
		executionPlan:  statePlane.plan,
		pointScopes:    []carrier.Scope{carrierFixture.scope, carrierFixture.scope},
		contextTransports: []runtimeContextTransport{{
			from: int(from), to: int(to), sourcePoint: 0, targetPoint: 1,
			sourceContext: 0, targetContext: 1,
			plan: plan, pre: whole, post: whole,
		}},
		contextTransportSource: map[[2]int]int{{int(to), 0}: int(from)},
	}
	epoch := &executorEpoch{runtime: runtime, work: work, points: make([]carrier.PointState, int(statePlane.plan.StateCount())), currentState: int(to)}
	epoch.points[int(from)] = source
	transported, transportedOK := epoch.pointFoldContextTransport(0)
	if !transportedOK || !transported.Valid() || !transported.Scope().Same(carrierFixture.scope) {
		t.Fatal("authenticated target did not receive source PointState")
	}
	for _, unrelated := range []int{int(wrongContextTarget), int(unrelatedSource)} {
		epoch.currentState = unrelated
		if _, admitted := epoch.pointFoldContextTransport(0); admitted {
			t.Fatalf("transport admitted unrelated StateOrdinal %d", unrelated)
		}
	}
	if !epoch.points[int(from)].Valid() {
		t.Fatal("transport changed source PointState")
	}
	if epoch.points[int(wrongContextTarget)].Valid() || epoch.points[int(unrelatedSource)].Valid() {
		t.Fatal("transport touched an unrelated context row")
	}
}

func TestContextTransportOwnsExactPairAgainstDynamicFactorOverlay(t *testing.T) {
	runtime := &solverRuntime{contextTransports: []runtimeContextTransport{
		{sourcePoint: 4, targetPoint: 9},
	}}
	if !runtime.contextTransportOwnsPointPair(4, 9) {
		t.Fatal("authenticated ContextTransport pair was not retained")
	}
	for _, pair := range [][2]int{{4, 8}, {3, 9}, {9, 4}} {
		if runtime.contextTransportOwnsPointPair(pair[0], pair[1]) {
			t.Fatalf("dynamic factor pair %v borrowed a different transport", pair)
		}
	}
	if (&solverRuntime{}).contextTransportOwnsPointPair(4, 9) {
		t.Fatal("empty transport table owned an exact pair")
	}
}

func TestSelectedFactorAdmissionRejectsContextTransportPair(t *testing.T) {
	fixture := newSelectedOverlayInstallFixture(t)
	if fixture.overlay == nil || len(fixture.overlay.directEdges) == 0 {
		t.Fatal("selected factor admission fixture has no direct edge")
	}
	edge := fixture.overlay.directEdges[0]
	source, sourceOK := fixture.runtime.graph.PointIndex(edge.Source())
	target, targetOK := fixture.runtime.graph.PointIndex(edge.Target())
	if !sourceOK || !targetOK {
		t.Fatal("selected factor endpoint indices")
	}
	fixture.runtime.contextTransports = []runtimeContextTransport{{sourcePoint: source, targetPoint: target}}
	_, _, _, _, _, admitted := fixture.runtime.prevalidateSelectedFactorEdges([]equation.SelectedStructuralFactorEdge{edge})
	if admitted {
		t.Fatal("selected factor created a second authority for a ContextTransport pair")
	}
}
