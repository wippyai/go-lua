package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
)

func TestProjectContributionKeepsOnlySelectedRoot(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 2)
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	changed := contributionWrite(t, work, operations[1], initial, shape.Slot(1), 2)
	input, ok := work.EmptyContribution(changed)
	if !ok {
		t.Fatal("input contribution")
	}
	projected, ok := work.ProjectContribution(input, shape.Slot(1))
	if !ok || !projected.Valid() || !projected.Support().Equal(whole) {
		t.Fatal("projection")
	}
	left, ok := projected.HandleAt(shape.Slot(0))
	if !ok {
		t.Fatal("left root")
	}
	initialLeft, ok := initial.HandleAt(shape.Slot(0))
	if !ok || left != initialLeft {
		t.Fatal("unselected root leaked")
	}
	right, ok := projected.HandleAt(shape.Slot(1))
	if !ok {
		t.Fatal("selected root")
	}
	changedRight, ok := changed.HandleAt(shape.Slot(1))
	if !ok || right != changedRight {
		t.Fatal("selected root was not transported")
	}
	if projected.coverage.slot(shape.Slot(0)).targets != nil || projected.coverage.slot(shape.Slot(1)).targets != nil {
		t.Fatal("empty input coverage was not projected sparsely")
	}
}
