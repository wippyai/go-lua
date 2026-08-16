package callsite

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program/keyspace"
)

const publicationDirectAllocationSubjectDomain = "wippy.analysis.effect.publication-direct-allocation-subject.v1\x00"

// PublicationDirectAllocationSubject is the detached admission of a direct
// subject/allocation identity into a proved publication correlation. It is not
// a placement fact: no alias, escape, uniqueness, frozen, COW, lifetime, or
// residence conclusion is represented here.
type PublicationDirectAllocationSubject struct {
	id          keyspace.ContentID
	correlation keyspace.ContentID
	direct      keyspace.ContentID
}

func publicationDirectAllocationSubjectID(correlation, direct keyspace.ContentID) keyspace.ContentID {
	if !correlation.Available() || !direct.Available() {
		return keyspace.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(publicationDirectAllocationSubjectDomain))
	_, _ = hash.Write(correlation[:])
	_, _ = hash.Write(direct[:])
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func (receipt PublicationDirectAllocationSubject) valid() bool {
	return receipt.id.Available() && receipt.id == publicationDirectAllocationSubjectID(receipt.correlation, receipt.direct)
}

// Valid reports the detached seal only. Its availability must never be read as
// a solved physical placement class.
func (receipt PublicationDirectAllocationSubject) Valid() bool { return receipt.valid() }

// NewPublicationDirectAllocationSubject reauthenticates live Pack binding
// evidence at the narrow cross-owner boundary. It requires the same subject
// binding and mounted call provenance already committed by correlation, then
// requires Value's direct source/allocation identity to match both the live
// source and the mounted requirement key. No raw IDs or coordinates are
// accepted as authority by this callsite join.
func NewPublicationDirectAllocationSubject(correlation PublicationPlacementCorrelationCandidate, subject pack.RuntimeAllocationContextBinding, direct valuedomain.DirectAllocationSubject) (PublicationDirectAllocationSubject, bool) {
	correlationID, correlationOK := correlation.ContentID()
	subjectID, subjectIDOK := correlation.SubjectBindingID()
	if !correlationOK || !subjectIDOK || !subject.Valid() || subject.ID() != subjectID || !direct.Valid() {
		return PublicationDirectAllocationSubject{}, false
	}
	mount, call, correlationProvenanceOK := correlation.CallProvenance()
	subjectMount, subjectCall, subjectProvenanceOK := subject.CallProvenance()
	source, sourceOK := subject.Source()
	mounted, mountedOK := subject.MountedAllocation()
	requirement, requirementOK := mounted.Requirement()
	if !correlationProvenanceOK || !subjectProvenanceOK || mount != subjectMount || call != subjectCall || !sourceOK || !mountedOK || !requirementOK ||
		!direct.MatchesRuntimeBinding(subject) || !direct.MatchesSource(source) || !direct.MatchesAllocationKeyID(requirement.KeyID()) {
		return PublicationDirectAllocationSubject{}, false
	}
	directID, directOK := direct.ContentID()
	if !directOK {
		return PublicationDirectAllocationSubject{}, false
	}
	receipt := PublicationDirectAllocationSubject{correlation: correlationID, direct: directID}
	receipt.id = publicationDirectAllocationSubjectID(receipt.correlation, receipt.direct)
	return receipt, receipt.valid()
}

func (receipt PublicationDirectAllocationSubject) ContentID() (keyspace.ContentID, bool) {
	return receipt.id, receipt.valid()
}

func (receipt PublicationDirectAllocationSubject) CorrelationID() (keyspace.ContentID, bool) {
	return receipt.correlation, receipt.valid()
}

func (receipt PublicationDirectAllocationSubject) DirectAllocationSubjectID() (keyspace.ContentID, bool) {
	return receipt.direct, receipt.valid()
}
