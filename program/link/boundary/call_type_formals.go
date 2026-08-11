package boundary

import (
	"crypto/sha256"
	"hash"
	"sync"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// TypeFormalArguments is an ephemeral, owner-fenced proof that one ordinary
// Project Call application supplies exactly the unconstrained type formals of
// one available Target operation. It retains only existing coordinates; the
// Boundary never stores an Application x Operation correspondence.
//
// A constrained Target formal deliberately has no proof here. Its static
// subtype check belongs to a later Static-to-Target correspondence, rather
// than being guessed from an authored Program term.
type TypeFormalArguments struct {
	component        *Component
	program          *program.Program
	staticID         keyspace.ContentID
	operationID      keyspace.ContentID
	correspondenceID keyspace.ContentID
	call             keyspace.Term
	count            int
	ready            bool
}

const typeFormalArgumentsVersion = 1

// This serial scratch retains only reusable hash/writer state, never a
// semantic correspondence, type term, Program, Target, or Boundary handle.
// The resulting identity is derived afresh on every query; this is not a
// registry or an identity cache.
type typeFormalIDScratch struct {
	sync.Mutex
	hash   hash.Hash
	writer canonical.Writer
}

var typeFormalIDHasher = typeFormalIDScratch{hash: sha256.New()}

// TypeFormalArguments proves the narrow ordinary-call correspondence for an
// existing Project Call application. It fails closed for non-Call applications,
// unavailable Target operations, mismatched static arity, or every constrained
// Target type formal.
func (v Calls) TypeFormalArguments(contract *target.Contract, application linkproject.Application, operation target.Operation) (TypeFormalArguments, bool) {
	if v.component == nil || v.component.authority == nil || contract == nil || contract != v.component.authority.target {
		return TypeFormalArguments{}, false
	}
	if !v.component.ApplicationOperationAvailable(contract, application, operation) {
		return TypeFormalArguments{}, false
	}
	applications := v.component.authority.project.Applications()
	shard, call, ok := applications.Call(application)
	if !ok || call == 0 {
		return TypeFormalArguments{}, false
	}
	p, ok := v.component.authority.project.Mounts().Program(shard)
	if !ok || p == nil {
		return TypeFormalArguments{}, false
	}
	count, ok := p.Static().Contracts().Calls().TypeArgumentCount(call)
	if !ok || count != contract.TypeFormalCount(operation) {
		return TypeFormalArguments{}, false
	}
	for index := 0; index < count; index++ {
		if _, constrained := contract.TypeFormalConstraint(operation, target.TypeFormal(index)); constrained {
			return TypeFormalArguments{}, false
		}
	}
	staticID := p.Static().ContentID()
	operationID, ok := contract.EffectOperationID(operation)
	if !staticID.Available() || !ok || !operationID.Available() {
		return TypeFormalArguments{}, false
	}
	result := TypeFormalArguments{
		component: v.component, program: p, staticID: staticID, operationID: operationID,
		call: call, count: count, ready: true,
	}
	id, ok := result.deriveCorrespondenceID()
	if !ok {
		return TypeFormalArguments{}, false
	}
	result.correspondenceID = id
	return result, true
}

func (v TypeFormalArguments) valid() bool {
	return v.ready && v.component != nil && v.component.authority != nil && v.program != nil && v.staticID.Available() && v.operationID.Available() && v.correspondenceID.Available() && v.call != 0 && v.count >= 0
}

// Count returns the exact static type-argument arity established by this
// correspondence. Zero is a valid exact arity.
func (v TypeFormalArguments) Count() int {
	if !v.valid() {
		return 0
	}
	return v.count
}

// At returns one existing Program Static Call type-argument term in authored
// order. It exposes no inferred type and performs no substitution.
func (v TypeFormalArguments) At(index int) (keyspace.Term, bool) {
	if !v.valid() || index < 0 || index >= v.count {
		return 0, false
	}
	term, ok := v.program.Static().Contracts().Calls().TypeArgumentAt(v.call, index)
	return term, ok && term != 0
}

// CorrespondenceID is the portable identity of this exact static-call and
// operation-ABI correspondence. It is framed from the Program Static content,
// call term, narrow Target effect-operation ABI, exact arity, and ordered
// authored type terms; Boundary and Link identities deliberately do not enter.
func (v TypeFormalArguments) CorrespondenceID() (id keyspace.ContentID, ok bool) {
	if !v.valid() {
		return id, false
	}
	return v.correspondenceID, true
}

func (v TypeFormalArguments) deriveCorrespondenceID() (id keyspace.ContentID, ok bool) {
	if !v.ready || v.program == nil || !v.staticID.Available() || !v.operationID.Available() || v.call == 0 || v.count < 0 {
		return id, false
	}
	typeFormalIDHasher.Lock()
	id, ok = v.correspondenceIDLocked(&typeFormalIDHasher)
	typeFormalIDHasher.Unlock()
	return id, ok
}

func (v TypeFormalArguments) correspondenceIDLocked(scratch *typeFormalIDScratch) (id keyspace.ContentID, ok bool) {
	if scratch == nil || scratch.hash == nil {
		return id, false
	}
	scratch.hash.Reset()
	if scratch.writer.Reset(scratch.hash, "program/link/boundary/call-type-formals", typeFormalArgumentsVersion) != nil ||
		scratch.writer.Record(1) != nil || scratch.writer.Bytes(v.staticID[:]) != nil || scratch.writer.Uint(uint64(v.call)) != nil ||
		scratch.writer.Bytes(v.operationID[:]) != nil || scratch.writer.Count(uint64(v.count)) != nil {
		return id, false
	}
	for index := 0; index < v.count; index++ {
		term, found := v.program.Static().Contracts().Calls().TypeArgumentAt(v.call, index)
		if !found || term == 0 || scratch.writer.Uint(uint64(term)) != nil {
			return id, false
		}
	}
	if scratch.writer.Finish() != nil {
		return id, false
	}
	sum := scratch.hash.Sum(id[:0])
	return id, len(sum) == len(id)
}
