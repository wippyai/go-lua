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

// SubjectLiveness is one all-normal-arms result for one Yield route and one
// subject. Yield endpoint paths are provenance coordinates copied from Flow's
// causal schedule; they may be unavailable for a terminal/unknown route, but
// the route identity and subject identity are always required.
type SubjectLiveness struct {
	id            identity.ContentID
	yieldRoute    identity.ContentID
	yieldFromPath identity.ContentID
	yieldToPath   identity.ContentID
	subjectKind   SubjectLivenessKind
	subject       identity.ContentID
	state         SubjectLivenessState
}

func SubjectLivenessIdentity(yieldRoute identity.ContentID, kind SubjectLivenessKind, subject identity.ContentID) (identity.ContentID, bool) {
	if !yieldRoute.Available() || !kind.Valid() || !subject.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/subject-liveness-v1", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(yieldRoute[:]) != nil || writer.Uint(uint64(kind)) != nil || writer.Bytes(subject[:]) != nil ||
		writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func NewSubjectLiveness(id, yieldRoute, yieldFromPath, yieldToPath, subject identity.ContentID, kind SubjectLivenessKind, state SubjectLivenessState) (SubjectLiveness, bool) {
	row := SubjectLiveness{
		id: id, yieldRoute: yieldRoute, yieldFromPath: yieldFromPath, yieldToPath: yieldToPath,
		subjectKind: kind, subject: subject, state: state,
	}
	return row, row.Available()
}

func (row SubjectLiveness) Available() bool {
	return row.id.Available() && row.yieldRoute.Available() && row.subject.Available() && row.subjectKind.Valid() && row.state.Valid() &&
		(row.yieldFromPath.Available() == row.yieldToPath.Available()) && row.identityValid()
}

// identityValid recomputes the row's owner-issued identity from the exact
// neutral coordinates. State is deliberately excluded: changing a proven
// answer must never silently mint a second subject coordinate.
func (row SubjectLiveness) identityValid() bool {
	if !row.id.Available() || !row.yieldRoute.Available() || !row.subject.Available() || !row.subjectKind.Valid() {
		return false
	}
	id, ok := SubjectLivenessIdentity(row.yieldRoute, row.subjectKind, row.subject)
	return ok && id == row.id
}

func (row SubjectLiveness) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row SubjectLiveness) YieldRouteID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.yieldRoute
}

func (row SubjectLiveness) YieldFromPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.yieldFromPath
}

func (row SubjectLiveness) YieldToPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.yieldToPath
}

func (row SubjectLiveness) SubjectKind() SubjectLivenessKind {
	if !row.Available() {
		return SubjectLivenessInvalid
	}
	return row.subjectKind
}

func (row SubjectLiveness) SubjectID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.subject
}

func (row SubjectLiveness) State() SubjectLivenessState {
	if !row.Available() {
		return SubjectLivenessUnknown
	}
	return row.state
}
