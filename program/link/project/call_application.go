package project

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

const (
	callApplicationIdentityVersion = 1
	callApplicationRoleOrdinary    = 1
)

// CallApplication is Project's opaque proof that one exact mounted Program
// CallOccurrence is the source of one existing ordinary-call Application.
// It retains both parent-issued proofs and reconstructs neither membership.
type CallApplication struct {
	application Application
	occurrence  program.CallOccurrence
	formal      keyspace.ContentID
}

// MountedIdentity consumes Project's already-sealed ordinary-call row into
// the exact detached scalars needed by artifact substitution. It validates
// only immutable Project rows and their owner-local inverse; it never opens
// the retained Program, TransformerInput, Flow, or CallOccurrence proof.
func (v Calls) MountedIdentity(application Application) (applicationKey, moduleID, occurrenceID keyspace.ContentID, ok bool) {
	if !v.live() || application.authority != v.authority || application.ordinal == 0 || uint64(application.ordinal) > uint64(len(v.authority.applications)) || !v.authority.applicationContentID.Available() {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	row := v.authority.applications[application.ordinal-1]
	if row.kind != applicationCall || row.shard == 0 || uint64(row.shard) > uint64(len(v.authority.mounts)) || !row.callContext.Available() || !row.callFormal.Available() ||
		v.authority.callApplicationsBySource[callSource{shard: row.shard, context: row.callContext}] != application.ordinal {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	mount := v.authority.mounts[row.shard-1]
	applicationKey, applicationOK := applicationID(v.authority.applicationContentID, row)
	if !applicationOK || !applicationKey.Available() || !mount.id.Available() || !mount.key.Available() {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
	}
	return applicationKey, mount.key, row.callContext, true
}

// MountedAt issues the complete proof at one canonical Calls position. The
// occurrence was retained from Program while Project sealed this row; no raw
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
	if row.kind != applicationCall || !row.callContext.Available() || !row.callFormal.Available() {
		return CallApplication{}, false
	}
	proof := CallApplication{application: application, occurrence: row.callProof, formal: row.callFormal}
	return proof, proof.Available()
}

// ForOccurrence joins an exact Project mount and its Program-issued call
// occurrence to the sole existing executable ordinary-call Application.
func (v Calls) ForOccurrence(shard Shard, occurrence program.CallOccurrence) (CallApplication, bool) {
	if !v.live() || shard.authority != v.authority || shard.ordinal == 0 || uint64(shard.ordinal) > uint64(len(v.authority.mounts)) {
		return CallApplication{}, false
	}
	mounted := v.authority.mounts[shard.ordinal-1].program
	if mounted == nil {
		return CallApplication{}, false
	}
	input := mounted.TransformerInput()
	context := occurrence.ContextID()
	if !input.OwnsCallOccurrence(occurrence) || !context.Available() {
		return CallApplication{}, false
	}
	ordinal := v.authority.callApplicationsBySource[callSource{shard: shard.ordinal, context: context}]
	if ordinal == 0 || uint64(ordinal) > uint64(len(v.authority.applications)) {
		return CallApplication{}, false
	}
	proof := CallApplication{
		application: Application{authority: v.authority, ordinal: ordinal},
		occurrence:  occurrence,
		formal:      v.authority.applications[ordinal-1].callFormal,
	}
	return proof, proof.Available()
}

// Available revalidates both exact parent owners and the sealed inverse row.
func (proof CallApplication) Available() bool {
	authority := proof.application.authority
	if authority == nil || proof.application.ordinal == 0 || uint64(proof.application.ordinal) > uint64(len(authority.applications)) {
		return false
	}
	row := authority.applications[proof.application.ordinal-1]
	if row.kind != applicationCall || row.shard == 0 || uint64(row.shard) > uint64(len(authority.mounts)) || !row.callContext.Available() || !row.callFormal.Available() || proof.formal != row.callFormal {
		return false
	}
	if authority.mounts[row.shard-1].program == nil || row.callProof != proof.occurrence {
		return false
	}
	return authority.callApplicationsBySource[callSource{shard: row.shard, context: row.callContext}] == proof.application.ordinal
}

// Mount returns the exact Project mount joined by this proof.
func (proof CallApplication) Mount() (Shard, bool) {
	if !proof.Available() {
		return Shard{}, false
	}
	row := proof.application.authority.applications[proof.application.ordinal-1]
	return Shard{authority: proof.application.authority, ordinal: row.shard}, true
}

// ContextID is Project's mount-independent source-join key for the exact
// Program CallOccurrence and this closed ordinary-application role. It is not
// an owner-neutral semantic CallFormal: Program.CallFormal supplies that
// separate ordinal-free identity to reusable transformer artifacts.
func (proof CallApplication) ContextID() (id keyspace.ContentID) {
	if !proof.Available() {
		return keyspace.ContentID{}
	}
	return proof.formal
}

// ApplicationID is the compact identity consumed by detached domain rows.
// It is issued only from an exact Project-owned proof and carries no Project
// handle into the consumer.
func (proof CallApplication) ApplicationID() (keyspace.ContentID, bool) {
	application, ok := proof.Application()
	if !ok {
		return keyspace.ContentID{}, false
	}
	return application.ContentID()
}

// ModuleID consumes the exact mounted proof into the Project mount identity.
// Consumers may retain this scalar without retaining the Shard authority.
func (proof CallApplication) ModuleID() (keyspace.ContentID, bool) {
	shard, ok := proof.Mount()
	if !ok || proof.application.authority == nil {
		return keyspace.ContentID{}, false
	}
	component := &Component{authority: proof.application.authority}
	return component.ModuleKey(shard)
}

// callApplicationID is construction-only. The formal identity is retained in
// the sealed application row, so hot proof queries return one scalar rather
// than rerunning the canonical codec.
func callApplicationID(occurrenceID keyspace.ContentID) (id keyspace.ContentID) {
	if !occurrenceID.Available() {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	var writer canonical.Writer
	if writer.Reset(h, "program/link/project/call-application", callApplicationIdentityVersion) != nil ||
		writer.Record(1) != nil || writer.Bytes(occurrenceID[:]) != nil ||
		writer.Uint(callApplicationRoleOrdinary) != nil || writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
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

// Occurrence returns the exact Program-owned occurrence used for the join.
func (proof CallApplication) Occurrence() (program.CallOccurrence, bool) {
	if !proof.Available() {
		return program.CallOccurrence{}, false
	}
	return proof.occurrence, true
}

// Owns authenticates a CallApplication against this exact Calls owner.
func (v Calls) Owns(proof CallApplication) bool {
	return v.live() && proof.application.authority == v.authority && proof.Available()
}
