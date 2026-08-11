package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/observation"
)

func TestActionQueueIsExactFIFOAndAllowsRequeueAfterDrain(t *testing.T) {
	queue := newActionQueue(3)
	if queue == nil || !queue.Seed(observation.NewEquation(2)) || !queue.Seed(observation.NewEquation(0)) || !queue.Seed(observation.NewEquation(2)) {
		t.Fatal("seed")
	}
	for _, want := range []observation.Equation{2, 0} {
		got, ok := queue.Next()
		if !ok || got != want {
			t.Fatalf("Next() = %d, %t; want %d, true", got, ok, want)
		}
	}
	if _, ok := queue.Next(); ok || !queue.Seed(observation.NewEquation(2)) {
		t.Fatal("drained action did not requeue exactly once")
	}
	if got, ok := queue.Next(); !ok || got != 2 {
		t.Fatalf("requeued Next() = %d, %t; want 2, true", got, ok)
	}
}

func TestActionQueueRejectsOutOfUniverseAndDiscard(t *testing.T) {
	queue := newActionQueue(1)
	if queue == nil || queue.Seed(observation.NewEquation(1)) {
		t.Fatal("accepted out-of-universe action")
	}
	queue.Discard()
	if queue.Seed(observation.NewEquation(0)) {
		t.Fatal("discarded queue accepted work")
	}
}
