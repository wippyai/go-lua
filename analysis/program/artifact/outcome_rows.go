package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// OutcomeKind is the artifact-owned closed Body Outcome vocabulary. It is
// converted exhaustively from Program and is not an alias of Flow's enum.
type OutcomeKind uint8

const (
	OutcomeInvalid OutcomeKind = iota
	OutcomeNormal
	OutcomeReturn
	OutcomeThrow
	OutcomeBreak
	OutcomeGoto
	OutcomeYield
	OutcomeCancel
)

func (kind OutcomeKind) valid() bool { return kind >= OutcomeNormal && kind <= OutcomeCancel }

// OutcomeRow is one immutable Body-owned semantic Outcome. Target and
// propagation are optional semantic references; returnStart/returnEnd name a
// contiguous range in the artifact's ordered ReturnValue plane.
type OutcomeRow struct {
	id             identity.ContentID
	body           identity.ContentID
	target         identity.ContentID
	propagation    identity.ContentID
	kind           OutcomeKind
	hasTarget      bool
	hasPropagation bool
	returnStart    uint32
	returnEnd      uint32
	points         []identity.ContentID
	sealed         bool
}

func (row OutcomeRow) Available() bool {
	if !row.sealed || !row.id.Available() || !row.body.Available() || !row.kind.valid() || row.returnEnd < row.returnStart ||
		row.hasPropagation != row.propagation.Available() {
		return false
	}
	switch row.kind {
	case OutcomeBreak, OutcomeGoto:
		if !row.hasTarget || !row.target.Available() {
			return false
		}
	default:
		if row.hasTarget || row.target.Available() {
			return false
		}
	}
	return row.returnStart == row.returnEnd || row.kind == OutcomeReturn
}

func (row OutcomeRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row OutcomeRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row OutcomeRow) Kind() OutcomeKind {
	if !row.Available() {
		return OutcomeInvalid
	}
	return row.kind
}

func (row OutcomeRow) TargetID() (identity.ContentID, bool) {
	return row.target, row.Available() && row.hasTarget
}

func (row OutcomeRow) PropagationID() (identity.ContentID, bool) {
	return row.propagation, row.Available() && row.hasPropagation
}

func (row OutcomeRow) ReturnValueCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.returnEnd - row.returnStart)
}

// PointCount and PointAt expose the exact LocalWTO memberships of the
// Outcome Causal Site. A terminal without a Causal Site, or a sealed Site that
// is intentionally unscheduled, retains no point membership; callers must
// fail closed rather than invent one from Outcome ID.
func (row OutcomeRow) PointCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.points)
}
func (row OutcomeRow) PointAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.points) {
		return identity.ContentID{}, false
	}
	return row.points[index], row.points[index].Available()
}

// ReturnValue is one ordered reference to an already-copied Values row. The
// same Values ID may occur under several propagated Return Outcomes; order and
// multiplicity belong to each Outcome range and are intentionally retained.
type ReturnValue struct{ id identity.ContentID }

func (value ReturnValue) Available() bool { return value.id.Available() }
func (value ReturnValue) ID() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.id
}
