package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// Relation activation support is one Mask carrier. A batch union must seal to
// the same canonical support as its direct Program literal, and repeating the
// same batch must be a semantic no-op.
func TestActivationSupportBatchUsesCanonicalMask(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("support work unavailable")
	}
	empty := work.False()
	positive, ok := work.Literal(1, true)
	if !ok || !work.Seal() {
		t.Fatal("support fixture did not seal")
	}

	transaction := &transaction{guards: manager, supports: []support.Mask{positive}}
	sweep := &regionSweep{supports: []support.Mask{empty}}
	region := compiledRegion{supports: []int{0}}
	advanced, ok := transaction.widenSweepSupports(region, sweep)
	if !ok || !advanced || !transaction.supports[0].Equal(positive) {
		t.Fatal("activation support union was not the canonical positive mask")
	}
	// A repeated complete batch has no alternate/raw support state to change.
	sweep.supports[0] = transaction.supports[0]
	advanced, ok = transaction.widenSweepSupports(region, sweep)
	if !ok || advanced || !transaction.supports[0].Equal(positive) {
		t.Fatal("equal activation support batch changed state")
	}
}
