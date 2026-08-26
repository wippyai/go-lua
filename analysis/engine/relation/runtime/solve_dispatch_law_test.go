package runtime

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestRedeemFullRootUsesStepResult(t *testing.T) {
	fixture := testfixture.New(t, 0xEB)
	queue, ok := fixpoint.New(fixture.Mounted().Arrangement().Execution(), fixture.Mounted())
	if !ok || !queue.SeedFull(mustFullRootForRuntimeLaw(t, fixture.Base())) {
		t.Fatal("full queue")
	}
	work, ok := queue.Next()
	if !ok || !work.Available() {
		t.Fatal("full work")
	}
	entry, ok := queue.Entry(work)
	if !ok {
		t.Fatal("full entry")
	}
	result, ok := redeemFull(fixture.Mounted(), work.Root(), entry, fixture.Base(), fixture.Geometry())
	if !ok {
		t.Fatal("full redemption")
	}
	if !result.Available() {
		t.Fatal("full result was not a sealed step.Result")
	}
}

func TestRedeemLaterRootUsesDeltaResult(t *testing.T) {
	fixture := testfixture.New(t, 0xEC)
	inputDelta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("later input delta")
	}
	later, ok := fixpoint.Later(inputDelta)
	if !ok {
		t.Fatal("later root")
	}
	queue, ok := fixpoint.New(fixture.Mounted().Arrangement().Execution(), fixture.Mounted())
	if !ok || !queue.SeedFull(mustFullRootForRuntimeLaw(t, fixture.Base())) || !queue.SeedLater(later) {
		t.Fatal("later queue")
	}

	work, ok := nextRuntimeLawWork(&queue, fixture.DependencyLeft())
	if !ok {
		t.Fatal("later left work")
	}
	entry, ok := queue.Entry(work)
	if !ok {
		t.Fatal("later entry")
	}
	result, ok := redeemLater(fixture.Mounted(), work.Root(), entry, inputDelta.Next(), fixture.Geometry())
	if !ok || !result.Available() {
		t.Fatal("later left redemption")
	}
	if !result.Available() {
		t.Fatal("later result was not a sealed delta.Result")
	}
}

func TestRedeemRefusesForeignAndStaleRoots(t *testing.T) {
	fixture := testfixture.New(t, 0xED)
	foreign := testfixture.New(t, 0xEE)
	queue, ok := fixpoint.New(fixture.Mounted().Arrangement().Execution(), fixture.Mounted())
	if !ok || !queue.SeedFull(mustFullRootForRuntimeLaw(t, fixture.Base())) {
		t.Fatal("root queue")
	}
	work, ok := queue.Next()
	if !ok {
		t.Fatal("root work")
	}
	entry, ok := queue.Entry(work)
	if !ok {
		t.Fatal("root entry")
	}
	foreignRoot, ok := fixpoint.Full(foreign.Base())
	if !ok {
		t.Fatal("foreign root")
	}
	if _, ok := redeemFull(fixture.Mounted(), foreignRoot, entry, fixture.Base(), fixture.Geometry()); ok {
		t.Fatal("foreign root redeemed")
	}
	inputDelta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("stale input delta")
	}
	staleRoot, ok := fixpoint.Later(inputDelta)
	if !ok {
		t.Fatal("stale root")
	}
	if _, ok := redeemLater(fixture.Mounted(), staleRoot, entry, fixture.Base(), fixture.Geometry()); ok {
		t.Fatal("stale root redeemed")
	}
}

func nextRuntimeLawWork(queue *fixpoint.Queue, dependency model.DependencyID) (fixpoint.Work, bool) {
	if queue == nil || !dependency.Available() {
		return fixpoint.Work{}, false
	}
	for {
		work, ok := queue.Next()
		if !ok {
			return fixpoint.Work{}, false
		}
		if work.Dependency() == dependency {
			return work, true
		}
	}
}

func mustFullRootForRuntimeLaw(t *testing.T, version database.Version) fixpoint.Root {
	t.Helper()
	root, ok := fixpoint.Full(version)
	if !ok {
		t.Fatal("full root")
	}
	return root
}
