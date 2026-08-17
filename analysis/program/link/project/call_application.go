package project

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
)

const (
	callApplicationIdentityVersion = 1
	callApplicationRoleOrdinary    = 1
)

// CallApplication is Project's opaque proof that one exact mounted Program
// call ID is the source of one existing ordinary-call Application. It retains
// only the immutable scalar IDs needed after Project sealing.
type CallApplication struct {
	application Application
	callID      identity.ContentID
	formal      identity.ContentID
}

// MountedIdentity consumes Project's already-sealed ordinary-call row into
// the exact detached scalars needed by artifact substitution. It validates
// only immutable Project rows and their owner-local inverse; it never opens a
// Program or Flow authority.
func (v Calls) MountedIdentity(application Application) (applicationKey, moduleID, callID identity.ContentID, ok bool) {
	if !v.live() || application.authority != v.authority || application.ordinal == 0 || uint64(application.ordinal) > uint64(len(v.authority.applications)) || !v.authority.applicationContentID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	row := v.authority.applications[application.ordinal-1]
	if row.kind != applicationCall || row.shard == 0 || uint64(row.shard) > uint64(len(v.authority.mounts)) || !row.callID.Available() || !row.callFormal.Available() ||
		v.authority.callApplicationsBySource[callSource{shard: row.shard, callID: row.callID}] != application.ordinal {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	mount := v.authority.mounts[row.shard-1]
	applicationKey, applicationOK := applicationID(v.authority.applicationContentID, row)
	if !applicationOK || !applicationKey.Available() || !mount.id.Available() || !mount.key.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	return applicationKey, mount.key, row.callID, true
}

// MountedAt issues the complete proof at one canonical Calls position. No raw
// term lookup, Program scan, or second index is performed here.
func (v Calls) MountedAt(index int) (CallApplication, bool) {
	application, ok := v.At(index)
	if !ok {
		return CallApplication{}, false
	}
	return v.ForApplication(application)
}

// ForApplication lifts one exact existing ordinary-call Application to its
// complete mounted proof. Application's owner/ordinal is already Project's
// sole row coordinate, so this is a checked row projection rather than an
// inverse or a reconstructed source join.
func (v Calls) ForApplication(application Application) (CallApplication, bool) {
	if !v.live() || application.authority != v.authority || application.ordinal == 0 || uint64(application.ordinal) > uint64(len(v.authority.applications)) {
		return CallApplication{}, false
	}
	row := v.authority.applications[application.ordinal-1]
	if row.kind != applicationCall || !row.callID.Available() || !row.callFormal.Available() {
		return CallApplication{}, false
	}
	proof := CallApplication{application: application, callID: row.callID, formal: row.callFormal}
	return proof, proof.Available()
}

// Available revalidates the exact parent owner and the sealed scalar inverse
// row.
func (proof CallApplication) Available() bool {
	authority := proof.application.authority
	if authority == nil || proof.application.ordinal == 0 || uint64(proof.application.ordinal) > uint64(len(authority.applications)) {
		return false
	}
	row := authority.applications[proof.application.ordinal-1]
	if row.kind != applicationCall || row.shard == 0 || uint64(row.shard) > uint64(len(authority.mounts)) || !row.callID.Available() || !row.callFormal.Available() || proof.callID != row.callID || proof.formal != row.callFormal {
		return false
	}
	if authority.mounts[row.shard-1].program == nil {
		return false
	}
	return authority.callApplicationsBySource[callSource{shard: row.shard, callID: row.callID}] == proof.application.ordinal
}

// Mount returns the exact Project mount joined by this proof.
func (proof CallApplication) Mount() (Shard, bool) {
	if !proof.Available() {
		return Shard{}, false
	}
	row := proof.application.authority.applications[proof.application.ordinal-1]
	return Shard{authority: proof.application.authority, ordinal: row.shard}, true
}

// ContextID is Project's mount-independent identity for this closed
// ordinary-application role. It is not the Program call ID; callers that need
// the mounted semantic call identity use MountedIdentity on the Application.
func (proof CallApplication) ContextID() (id identity.ContentID) {
	if !proof.Available() {
		return identity.ContentID{}
	}
	return proof.formal
}

// CallID returns the immutable scalar semantic identity of the mounted
// Program call. It carries no Program authority.
func (proof CallApplication) CallID() (id identity.ContentID) {
	if !proof.Available() {
		return identity.ContentID{}
	}
	return proof.callID
}

// ApplicationID is the compact identity consumed by detached domain rows.
// It is issued only from an exact Project-owned proof and carries no Project
// handle into the consumer.
func (proof CallApplication) ApplicationID() (identity.ContentID, bool) {
	application, ok := proof.Application()
	if !ok {
		return identity.ContentID{}, false
	}
	return application.ContentID()
}

// ModuleID consumes the exact mounted proof into the Project mount identity.
// Consumers may retain this scalar without retaining the Shard authority.
func (proof CallApplication) ModuleID() (identity.ContentID, bool) {
	shard, ok := proof.Mount()
	if !ok || proof.application.authority == nil {
		return identity.ContentID{}, false
	}
	component := &Component{authority: proof.application.authority}
	return component.ModuleKey(shard)
}

// callApplicationID is construction-only. The formal identity is retained in
// the sealed application row, so hot proof queries return one scalar rather
// than rerunning the canonical codec.
func callApplicationID(callID identity.ContentID) (id identity.ContentID) {
	if !callID.Available() {
		return identity.ContentID{}
	}
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/link/project/call-application", callApplicationIdentityVersion) != nil ||
		writer.Record(1) != nil || writer.Bytes(callID[:]) != nil ||
		writer.Uint(callApplicationRoleOrdinary) != nil || writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// Application returns the exact Project-owned ordinary-call Application.
func (proof CallApplication) Application() (Application, bool) {
	if !proof.Available() {
		return Application{}, false
	}
	return proof.application, true
}

// Owns authenticates a CallApplication against this exact Calls owner.
func (v Calls) Owns(proof CallApplication) bool {
	return v.live() && proof.application.authority == v.authority && proof.Available()
}
