package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// descendingAcyclicRefold selects a settled acyclic point together with an RHS
// its published state does not cover: the point's own base, the empty fold its
// candidates were joined onto. Refolding onto that base is exactly the shape a
// non-additive candidate replacement produces, and its descent is sourced in
// this point's own fold rather than in any incoming point.
func descendingAcyclicRefold(t *testing.T, epoch *executorEpoch) (int, carrier.PointRHS) {
	t.Helper()
	for index := range epoch.points {
		if index >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[index] || epoch.runtime.pointRegion[index] != schedule.NoRegion {
			continue
		}
		point, addressed := epoch.runtime.graph.PointAt(schedule.Node(index))
		if !addressed {
			continue
		}
		base, based := epoch.pointBase(point, index)
		if !based || !epoch.work.OwnsPointState(epoch.points[index]) || epoch.work.LessOrEqPointStateRHS(epoch.points[index], base) {
			continue
		}
		return index, base
	}
	t.Fatal("no settled acyclic point stands above its own base: the law selects nothing and would pass vacuously")
	return 0, carrier.PointRHS{}
}

// A publication's direction is a property of the value published, never of the
// points that fed it. A refold that lands below the state it replaces descends
// even when no incoming point recorded a descent, because a fold descends on
// its own account whenever a candidate is replaced non-additively or its base
// shrinks -- movement no ledger over incoming points observes. The
// classification must say descent, and the publication must record it: a point
// that descends without a record leaves every consumer certified ascending
// against a value that went down.
func TestAcyclicRefoldBelowItsPredecessorIsClassifiedAndRecordedAsDescent(t *testing.T) {
	fixture := newSelectedOverlayInstallFixture(t)
	epoch := fixture.epoch
	pointIndex, base := descendingAcyclicRefold(t, epoch)
	current := epoch.points[pointIndex]

	order := epoch.acyclicPublicationOrder(current, base)
	if order != publicationMayDescend {
		t.Fatalf("point %d refolded below its published state classified order=%d, want publicationMayDescend", pointIndex, order)
	}

	published, changes, publishable := epoch.work.ReplacePointWithRHS(current, base)
	if !publishable {
		t.Fatalf("point %d descending RHS was not publishable", pointIndex)
	}
	changed, publishedOK := epoch.publishAcyclicExact(pointIndex, current, published, order, changes)
	if !publishedOK || !changed {
		t.Fatalf("point %d descending publication changed=%t ok=%t", pointIndex, changed, publishedOK)
	}
	if epoch.structural.pointDescent[pointIndex] == 0 {
		t.Fatalf("point %d published below its predecessor without recording a descent", pointIndex)
	}
}
