package lifecycle

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// SubjectLivenessKind is the mounted-neutral subject plane.  These tags
// preserve the distinction between an allocation/value subject, an authored
// storage Cell, and a Values aggregate while keeping Placement out of the
// Program schema.
type SubjectLivenessKind uint8

const (
	SubjectLivenessInvalid SubjectLivenessKind = iota
	SubjectLivenessRoot
	SubjectLivenessCell
	SubjectLivenessValue
	SubjectLivenessValues
)

func (kind SubjectLivenessKind) Valid() bool {
	return kind >= SubjectLivenessRoot && kind <= SubjectLivenessValues
}

// SubjectLivenessState is the closed neutral suspension result.  Unknown is
// a real published answer: it means the producer could not prove a complete
// route/alias relation and must not be interpreted as DiesBefore.
type SubjectLivenessState uint8

const (
	SubjectLivenessUnknown SubjectLivenessState = iota + 1
	SubjectLivenessLive
	SubjectLivenessDiesBefore
)

func (state SubjectLivenessState) Valid() bool {
	return state >= SubjectLivenessUnknown && state <= SubjectLivenessDiesBefore
}

// The suspension plane is published as live ranges over an ordered yield
// boundary, not as one row per (boundary, subject) pair. A body's yield
// boundaries are numbered in program order, and each subject carries the
// maximal runs of one answer over that numbering. The pair answer is a range
// membership read (SubjectLivenessAtBoundary), so the plane costs
// O(boundaries + spans) rows while every pair remains derivable.
//
// The numbering is one sequence over the whole program: each body occupies a
// contiguous block of ordinals, so a run inside a body is a run inside the
// sequence and no row needs to name its body.

// SubjectYieldBoundary is one distinct yield route and its ordinal in the
// program-ordered boundary sequence. Call is the canonical artifact
// occurrence consumed by mounted Call; the yield endpoint paths are
// provenance copied from Flow's causal schedule and may be unavailable
// together for a terminal or unknown route.
type SubjectYieldBoundary struct {
	id            identity.ContentID
	call          identity.ContentID
	yieldRoute    identity.ContentID
	yieldFromPath identity.ContentID
	yieldToPath   identity.ContentID
	ordinal       uint32
}

// SubjectYieldBoundaryIdentity names one boundary by the route it is. The
// ordinal is excluded: it is a coordinate in a numbering, and a boundary that
// moves because an unrelated body grew is the same boundary.
func SubjectYieldBoundaryIdentity(call, yieldRoute identity.ContentID) (identity.ContentID, bool) {
	if !call.Available() || !yieldRoute.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/subject-yield-boundary-v1", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(call[:]) != nil || writer.Bytes(yieldRoute[:]) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func NewSubjectYieldBoundary(id, call, yieldRoute, yieldFromPath, yieldToPath identity.ContentID, ordinal uint32) (SubjectYieldBoundary, bool) {
	row := SubjectYieldBoundary{id: id, call: call, yieldRoute: yieldRoute, yieldFromPath: yieldFromPath, yieldToPath: yieldToPath, ordinal: ordinal}
	return row, row.Available()
}

func (row SubjectYieldBoundary) Available() bool {
	return row.id.Available() && row.call.Available() && row.yieldRoute.Available() &&
		(row.yieldFromPath.Available() == row.yieldToPath.Available()) && row.identityValid()
}

func (row SubjectYieldBoundary) identityValid() bool {
	if !row.id.Available() || !row.call.Available() || !row.yieldRoute.Available() {
		return false
	}
	id, ok := SubjectYieldBoundaryIdentity(row.call, row.yieldRoute)
	return ok && id == row.id
}

func (row SubjectYieldBoundary) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row SubjectYieldBoundary) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}

func (row SubjectYieldBoundary) YieldRouteID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.yieldRoute
}

func (row SubjectYieldBoundary) YieldFromPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.yieldFromPath
}

func (row SubjectYieldBoundary) YieldToPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.yieldToPath
}

// Ordinal is this boundary's position in the program-ordered sequence the
// liveness spans are ranges over.
func (row SubjectYieldBoundary) Ordinal() uint32 {
	if !row.Available() {
		return 0
	}
	return row.ordinal
}

// SubjectLivenessSpan is one subject's maximal run of a single answer over
// the boundary ordinal. Lo and Hi are inclusive. A subject whose answer
// changes k times carries k+1 spans; the spans of one subject are disjoint
// and cover exactly the boundaries of the body that owns it.
type SubjectLivenessSpan struct {
	id          identity.ContentID
	subject     identity.ContentID
	subjectKind SubjectLivenessKind
	lo          uint32
	hi          uint32
	state       SubjectLivenessState
}

// SubjectLivenessSpanIdentity names one span by the subject and the range it
// covers. State is deliberately excluded, exactly as the pair row excluded
// it: changing a proven answer must never silently mint a second subject
// coordinate.
func SubjectLivenessSpanIdentity(kind SubjectLivenessKind, subject identity.ContentID, lo, hi uint32) (identity.ContentID, bool) {
	if !kind.Valid() || !subject.Available() || lo > hi {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/subject-liveness-span-v1", 1) != nil || writer.Record(1) != nil ||
		writer.Uint(uint64(kind)) != nil || writer.Bytes(subject[:]) != nil ||
		writer.Uint(uint64(lo)) != nil || writer.Uint(uint64(hi)) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func NewSubjectLivenessSpan(id, subject identity.ContentID, kind SubjectLivenessKind, lo, hi uint32, state SubjectLivenessState) (SubjectLivenessSpan, bool) {
	row := SubjectLivenessSpan{id: id, subject: subject, subjectKind: kind, lo: lo, hi: hi, state: state}
	return row, row.Available()
}

func (row SubjectLivenessSpan) Available() bool {
	return row.id.Available() && row.subject.Available() && row.subjectKind.Valid() && row.state.Valid() &&
		row.lo <= row.hi && row.identityValid()
}

func (row SubjectLivenessSpan) identityValid() bool {
	if !row.id.Available() || !row.subject.Available() || !row.subjectKind.Valid() || row.lo > row.hi {
		return false
	}
	id, ok := SubjectLivenessSpanIdentity(row.subjectKind, row.subject, row.lo, row.hi)
	return ok && id == row.id
}

func (row SubjectLivenessSpan) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row SubjectLivenessSpan) SubjectKind() SubjectLivenessKind {
	if !row.Available() {
		return SubjectLivenessInvalid
	}
	return row.subjectKind
}

func (row SubjectLivenessSpan) SubjectID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.subject
}

func (row SubjectLivenessSpan) Lo() uint32 {
	if !row.Available() {
		return 0
	}
	return row.lo
}

func (row SubjectLivenessSpan) Hi() uint32 {
	if !row.Available() {
		return 0
	}
	return row.hi
}

// Covers reports whether this span answers the boundary at that ordinal.
func (row SubjectLivenessSpan) Covers(ordinal uint32) bool {
	return row.Available() && ordinal >= row.lo && ordinal <= row.hi
}

func (row SubjectLivenessSpan) State() SubjectLivenessState {
	if !row.Available() {
		return SubjectLivenessUnknown
	}
	return row.state
}
