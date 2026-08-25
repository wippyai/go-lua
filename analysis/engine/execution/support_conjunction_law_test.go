package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// support_conjunction_law_test.go states what one invocation concluded OVER
// when it read more than once.
//
// A conclusion may only hold where every read it consumed holds, so its
// support is the conjunction of what those reads proved. The reads of one
// invocation do not all answer over the same window - a member read at a
// coordinate this very rule writes is absent everywhere before the fixpoint
// puts anything there - so the conjunction is taken by entailment, and the
// case neither read contains is refused rather than resolved by arrival order.
//
// The laws are stated over the one surface both folds ask, so a product of
// exact cells and a vector spanned beside it cannot disagree about the same
// invocation.

// TestASupportContainingTheRunningMeetLeavesItAlone is the recursion law. A
// read proved over MORE than its siblings did not learn anything that narrows
// the conclusion, and holding the two to equality is what made a self-reaching
// call site unable to conclude at all.
func TestASupportContainingTheRunningMeetLeavesItAlone(t *testing.T) {
	fixture := newSelectedFixture(t)
	narrower := narrowerSupport(t, fixture)
	conjoined, ok := ConjoinSupport(narrower, fixture.whole)
	if !ok {
		t.Fatal("a support containing the running meet was refused")
	}
	if !conjoined.Equal(narrower) {
		t.Fatal("the meet moved to a support that contains it")
	}
}

// TestASupportProvingLessMovesTheMeetDown is the other half of the same law:
// a read that held over less than everything before it is exactly what the
// conclusion is bounded by.
func TestASupportProvingLessMovesTheMeetDown(t *testing.T) {
	fixture := newSelectedFixture(t)
	narrower := narrowerSupport(t, fixture)
	conjoined, ok := ConjoinSupport(fixture.whole, narrower)
	if !ok {
		t.Fatal("a support proving less than the running meet was refused")
	}
	if !conjoined.Equal(narrower) {
		t.Fatal("the meet did not move down to the narrower support")
	}
}

// TestAnInvalidSupportIsRefused keeps the absent mask out of the meet. A read
// that reported no support proved nothing, and treating that as the whole
// world would publish a conclusion over everywhere.
func TestAnInvalidSupportIsRefused(t *testing.T) {
	fixture := newSelectedFixture(t)
	if _, ok := ConjoinSupport(fixture.whole, support.Mask{}); ok {
		t.Fatal("a cell with no support was folded into the meet")
	}
	if _, ok := ConjoinSupport(support.Mask{}, fixture.whole); ok {
		t.Fatal("a meet with no support accepted a cell")
	}
}

// TestAVectorNarrowsTheConclusionToEveryCellItDelivered is the spanned-vector
// law. The cells of a vector are read one coordinate at a time and each
// answers over what its own read proved, so the conclusion folded from them
// holds over the conjunction of all of them together with whatever the rest of
// the invocation held - not over the window the invocation opened.
func TestAVectorNarrowsTheConclusionToEveryCellItDelivered(t *testing.T) {
	fixture := newSelectedFixture(t)
	narrower := narrowerSupport(t, fixture)
	cells := []MemberCell[uint64]{
		{Value: 1, Present: true, Region: fixture.whole},
		{Value: 2, Present: true, Region: narrower},
		{Value: 3, Present: true, Region: fixture.whole},
	}
	conjoined, ok := ConjoinCells(fixture.whole, cells)
	if !ok {
		t.Fatal("a vector of comparable cells was refused")
	}
	if !conjoined.Equal(narrower) {
		t.Fatal("the conclusion did not narrow to the cell that proved least")
	}
	empty, emptyOK := ConjoinCells[uint64](narrower, nil)
	if !emptyOK || !empty.Equal(narrower) {
		t.Fatal("a vector with no cells changed the support the rest of the invocation held")
	}
}

// TestACellWithNoSupportRefusesTheWholeVector states that one unproved cell
// refuses the conclusion rather than being skipped. A vector is a closed
// denominator: every coordinate of it was read, so every one of them bounds
// what the fold concluded.
func TestACellWithNoSupportRefusesTheWholeVector(t *testing.T) {
	fixture := newSelectedFixture(t)
	cells := []MemberCell[uint64]{
		{Value: 1, Present: true, Region: fixture.whole},
		{Value: 2, Present: true},
	}
	if _, ok := ConjoinCells(fixture.whole, cells); ok {
		t.Fatal("a vector containing a cell with no support was admitted")
	}
}
