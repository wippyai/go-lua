package lifecycle

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// SubjectAliasCandidate is the scalar candidate-to-denominator relation.
// Absence is unknown, never proof of no-alias.
type SubjectAliasCandidate struct {
	id              identity.ContentID
	sourceCandidate identity.ContentID
	candidateKind   SubjectLivenessKind
	candidate       identity.ContentID
	scope           identity.ContentID
	closed          bool
	proven          bool
}

func SubjectAliasCandidateIdentity(sourceCandidate identity.ContentID, candidateKind SubjectLivenessKind, candidate, scope identity.ContentID, closed bool) (identity.ContentID, bool) {
	if !sourceCandidate.Available() || !candidateKind.Valid() || !candidate.Available() || !scope.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/subject-alias-candidate-v1", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(sourceCandidate[:]) != nil || writer.Uint(uint64(candidateKind)) != nil || writer.Bytes(candidate[:]) != nil ||
		writer.Bytes(scope[:]) != nil || writer.Bool(closed) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func NewSubjectAliasCandidate(id, sourceCandidate identity.ContentID, candidateKind SubjectLivenessKind, candidate, scope identity.ContentID, closed bool) (SubjectAliasCandidate, bool) {
	derived, derivedOK := SubjectAliasCandidateIdentity(sourceCandidate, candidateKind, candidate, scope, closed)
	row := SubjectAliasCandidate{id: id, sourceCandidate: sourceCandidate, candidateKind: candidateKind, candidate: candidate, scope: scope, closed: closed, proven: derivedOK && derived == id}
	return row, row.Available()
}

func (row SubjectAliasCandidate) Available() bool {
	return row.proven && row.id.Available() && row.sourceCandidate.Available() && row.candidateKind.Valid() && row.candidate.Available() && row.scope.Available()
}

func (row SubjectAliasCandidate) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row SubjectAliasCandidate) SourceCandidateID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.sourceCandidate
}

func (row SubjectAliasCandidate) CandidateKind() SubjectLivenessKind {
	if !row.Available() {
		return SubjectLivenessInvalid
	}
	return row.candidateKind
}

func (row SubjectAliasCandidate) CandidateID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.candidate
}

func (row SubjectAliasCandidate) ScopeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.scope
}

func (row SubjectAliasCandidate) Closed() bool { return row.Available() && row.closed }
