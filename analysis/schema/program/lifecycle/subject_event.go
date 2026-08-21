package lifecycle

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// SubjectEventKind is the intentionally small Artifact projection of Flow's
// subject events.  Define/Use rows stay in Flow; Artifact carries only the
// alias and unresolved boundaries needed by mounted lifecycle consumers.
type SubjectEventKind uint8

const (
	SubjectEventInvalid SubjectEventKind = iota
	SubjectEventUnknown
	SubjectEventAlias
)

func (kind SubjectEventKind) Valid() bool {
	return kind == SubjectEventUnknown || kind == SubjectEventAlias
}

// SubjectEvent is one authenticated alias or Unknown event. Subject and
// Related retain Flow's semantic-path identities; the artifact compiler
// verifies those paths against the owner-fenced Flow view before admission.
// SourceEventID is carried so the Artifact row remains traceable to the
// already-authenticated Flow event identity.
type SubjectEvent struct {
	id          identity.ContentID
	sourceEvent identity.ContentID
	path        identity.ContentID
	kind        SubjectEventKind
	role        uint8
	index       uint32
	subjectKind SubjectLivenessKind
	subject     identity.ContentID
	relatedKind SubjectLivenessKind
	related     identity.ContentID
}

func SubjectEventIdentity(sourceEvent, path identity.ContentID, kind SubjectEventKind, role uint8, index uint32, subjectKind SubjectLivenessKind, subject identity.ContentID, relatedKind SubjectLivenessKind, related identity.ContentID) (identity.ContentID, bool) {
	if !sourceEvent.Available() || !path.Available() || !kind.Valid() || role == 0 || !subjectKind.Valid() || !subject.Available() {
		return identity.ContentID{}, false
	}
	if related.Available() != relatedKind.Valid() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/subject-event-v1", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(sourceEvent[:]) != nil || writer.Bytes(path[:]) != nil || writer.Uint(uint64(kind)) != nil ||
		writer.Uint(uint64(role)) != nil || writer.Uint(uint64(index)) != nil || writer.Uint(uint64(subjectKind)) != nil ||
		writer.Bytes(subject[:]) != nil || writer.Uint(uint64(relatedKind)) != nil || writer.Bytes(related[:]) != nil ||
		writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func NewSubjectEvent(id, sourceEvent, path identity.ContentID, kind SubjectEventKind, role uint8, index uint32, subjectKind SubjectLivenessKind, subject identity.ContentID, relatedKind SubjectLivenessKind, related identity.ContentID) (SubjectEvent, bool) {
	row := SubjectEvent{id: id, sourceEvent: sourceEvent, path: path, kind: kind, role: role, index: index, subjectKind: subjectKind, subject: subject, relatedKind: relatedKind, related: related}
	return row, row.Available()
}

func (row SubjectEvent) Available() bool {
	if !row.id.Available() || !row.sourceEvent.Available() || !row.path.Available() || !row.kind.Valid() || row.role == 0 || !row.subjectKind.Valid() || !row.subject.Available() {
		return false
	}
	if row.related.Available() != row.relatedKind.Valid() {
		return false
	}
	id, ok := SubjectEventIdentity(row.sourceEvent, row.path, row.kind, row.role, row.index, row.subjectKind, row.subject, row.relatedKind, row.related)
	return ok && id == row.id
}

func (row SubjectEvent) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row SubjectEvent) SourceEventID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.sourceEvent
}

func (row SubjectEvent) PathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.path
}

func (row SubjectEvent) Kind() SubjectEventKind {
	if !row.Available() {
		return SubjectEventInvalid
	}
	return row.kind
}

func (row SubjectEvent) Role() uint8 {
	if !row.Available() {
		return 0
	}
	return row.role
}

func (row SubjectEvent) Index() (uint32, bool) {
	return row.index, row.Available()
}

func (row SubjectEvent) SubjectKind() SubjectLivenessKind {
	if !row.Available() {
		return SubjectLivenessInvalid
	}
	return row.subjectKind
}

func (row SubjectEvent) SubjectID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.subject
}

func (row SubjectEvent) RelatedKind() SubjectLivenessKind {
	if !row.Available() {
		return SubjectLivenessInvalid
	}
	return row.relatedKind
}

func (row SubjectEvent) RelatedID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.related
}
