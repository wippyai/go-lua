package cold

import "github.com/wippyai/go-lua/analysis/identity"

// The generic cold family declaration reserves append-only slots after the
// nine declared above it. Deriving the point slots from the last summary slot
// keeps that discipline visible here and makes it impossible to reuse a slot
// owned by another family.
const (
	slotPoint         = slotUnarySummary + 1
	slotPointDecision = slotPoint + 1
)

var (
	pointFamily         = Family[Point]{slot: slotPoint, name: "point"}
	pointDecisionFamily = Family[PointDecision]{slot: slotPointDecision, name: "point-decision"}
)

func PointFamily() Family[Point] { return pointFamily }

func PointDecisionFamily() Family[PointDecision] { return pointDecisionFamily }

// PointDecision is one decision identity a program point commits to. Its
// position is its ordinal in PointDecisionFamily and the parent point names
// the half-open span it owns, so no point retains a slice header.
type PointDecision struct{ id identity.ContentID }

// NewPointDecision copies one canonical point decision identity.
func NewPointDecision(id identity.ContentID) (PointDecision, bool) {
	row := PointDecision{id: id}
	return row, row.Available()
}

func (row PointDecision) Available() bool { return row.id.Available() }

func (row PointDecision) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

// Point is one program point's geometry. Its decisions are a span in
// PointDecisionFamily, preserving the canonical decision order while making
// this row flat and copy-safe.
type Point struct {
	id             identity.ContentID
	initial        bool
	decisionOffset uint32
	decisionCount  uint32
}

// NewPoint copies one canonical Point row and replaces its nested decision
// slice with a dense PointDecisionFamily span.
func NewPoint(id identity.ContentID, initial bool, decisionOffset, decisionCount uint32) (Point, bool) {
	row := Point{id: id, initial: initial, decisionOffset: decisionOffset, decisionCount: decisionCount}
	return row, row.Available()
}

func (row Point) Available() bool {
	return row.id.Available() && uint64(row.decisionOffset)+uint64(row.decisionCount) <= uint64(^uint32(0))
}

func (row Point) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Point) Initial() bool { return row.Available() && row.initial }

func (row Point) DecisionSpan() (offset, count uint32, ok bool) {
	return row.decisionOffset, row.decisionCount, row.Available()
}

func (row Point) DecisionCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.decisionCount)
}
