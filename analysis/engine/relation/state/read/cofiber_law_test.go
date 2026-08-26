package read

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestCommonFiberPartitionIsDisjointExactCover(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal("guard manager")
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("support work")
	}
	left, ok := work.Literal(1, true)
	if !ok {
		t.Fatal("left fiber")
	}
	right, ok := work.Literal(1, false)
	if !ok || !work.Seal() {
		t.Fatal("right fiber")
	}
	within, ok := support.True(manager)
	if !ok {
		t.Fatal("cover")
	}
	reader := &reader{}
	if !reader.validFiberPartition([]commonFiber{{region: left}, {region: right}}, within) {
		t.Fatal("exact disjoint cover rejected")
	}
	if reader.validFiberPartition([]commonFiber{{region: left}, {region: left}, {region: right}}, within) {
		t.Fatal("overlapping fibers accepted")
	}
	if reader.validFiberPartition([]commonFiber{{region: left}}, within) {
		t.Fatal("incomplete cover accepted")
	}
}
