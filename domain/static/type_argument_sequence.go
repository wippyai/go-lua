package static

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/type/authority"
)

const typeArgumentSequenceDomain = "wippy.analysis.static.type-argument-sequence.v1\x00"

// TypeArgumentSequence is the owner-neutral semantic identity of one ordered
// mounted call type-argument sequence. Static issues it where raw Program
// references become resolved semantic formals.
type TypeArgumentSequence struct {
	id    identity.ContentID
	count uint32
}

func (sequence TypeArgumentSequence) Available() bool { return sequence.id.Available() }
func (sequence TypeArgumentSequence) ContentID() (identity.ContentID, bool) {
	return sequence.id, sequence.Available()
}
func (sequence TypeArgumentSequence) Count() int {
	if !sequence.Available() {
		return 0
	}
	return int(sequence.count)
}
func (sequence TypeArgumentSequence) Same(other TypeArgumentSequence) bool {
	return sequence.Available() && other.Available() && sequence == other
}

// typeArgumentSequenceTable is the seal-time semantic projection of exact
// Program call type-argument proofs. Semantic graphs, row identities, and the
// construction type authority are discarded after sequence issuance.
type typeArgumentSequenceTable struct {
	mounted map[mountedTypeArgumentsKey]TypeArgumentSequence
	sealed  bool
}

// mountedTypeArgumentsKey is the sole Link-local substitution key for a
// reusable Program type-argument sequence. ModuleKey keeps duplicate mounts
// distinct; types is the Program-issued lookup coordinate, while the returned
// semantic sequence is Static-issued.
type mountedTypeArgumentsKey struct {
	module identity.ContentID
	types  identity.ContentID
}

// MountedCallTypeArgumentSequence returns Static's sealed, owner-neutral
// semantic sequence for an artifact CallTypeArguments ID.
func (a *Authority) MountedCallTypeArgumentSequence(module, types identity.ContentID) (TypeArgumentSequence, bool) {
	if a == nil || !a.typeArguments.sealed || !module.Available() || !types.Available() {
		return TypeArgumentSequence{}, false
	}
	sequence, ok := a.typeArguments.mounted[mountedTypeArgumentsKey{module: module, types: types}]
	return sequence, ok && sequence.Available()
}

func issueMountedTypeArgumentFormalID(types *typeauthority.Authority, referenceID identity.ContentID) (identity.ContentID, bool) {
	if types == nil || !referenceID.Available() {
		return identity.ContentID{}, false
	}
	projection, ok := types.ProjectionByReferenceID(referenceID)
	if !ok {
		return identity.ContentID{}, false
	}
	return projection.SemanticIdentity()
}

func typeArgumentSequenceIdentity(formals []identity.ContentID) (TypeArgumentSequence, bool) {
	if uint64(len(formals)) > uint64(^uint32(0)) {
		return TypeArgumentSequence{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(typeArgumentSequenceDomain))
	var word [4]byte
	binary.BigEndian.PutUint32(word[:], uint32(len(formals)))
	_, _ = hash.Write(word[:])
	for _, formal := range formals {
		if !formal.Available() {
			return TypeArgumentSequence{}, false
		}
		_, _ = hash.Write(formal[:])
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	sequence := TypeArgumentSequence{id: id, count: uint32(len(formals))}
	return sequence, sequence.Available()
}

// sealMountedTypeArgumentSequences consumes only canonical Program rows and
// the Link-local type authority. It resolves each ordered sequence once and
// retains no downstream-reconstructable row facade.
func (a *Authority) sealMountedTypeArgumentSequences() bool {
	if a == nil || a.types == nil || len(a.mounts) == 0 {
		return false
	}
	table := typeArgumentSequenceTable{mounted: make(map[mountedTypeArgumentsKey]TypeArgumentSequence), sealed: true}
	byArgument := make(map[identity.ContentID]identity.ContentID)
	for _, mount := range a.mounts {
		if !mount.Program.Available() || !mount.ModuleID.Available() {
			return false
		}
		grouped := make(map[identity.ContentID][]programschema.CallTypeArgument)
		callCount, callsOK := mount.Program.CallCount()
		if !callsOK {
			return false
		}
		for callIndex := 0; callIndex < callCount; callIndex++ {
			call, callOK := mount.Program.CallAt(callIndex)
			typesID := call.TypeArgumentsID()
			if !callOK || !typesID.Available() {
				return false
			}
			if _, exists := grouped[typesID]; !exists {
				grouped[typesID] = nil
			}
		}
		typeArgumentCount, typeArgumentsOK := mount.Program.CallTypeArgumentCount()
		if !typeArgumentsOK {
			return false
		}
		for index := 0; index < typeArgumentCount; index++ {
			row, rowOK := mount.Program.CallTypeArgumentAt(index)
			if !rowOK || !row.Available() {
				return false
			}
			grouped[row.TypesID()] = append(grouped[row.TypesID()], row)
		}
		for typesID, rows := range grouped {
			formals := make([]identity.ContentID, len(rows))
			for index, row := range rows {
				if row.Index() != uint32(index) {
					return false
				}
				formal, formalOK := issueMountedTypeArgumentFormalID(a.types, row.ReferenceID())
				if !formalOK {
					return false
				}
				if prior, duplicate := byArgument[row.ID()]; duplicate && prior != formal {
					return false
				}
				byArgument[row.ID()] = formal
				formals[index] = formal
			}
			sequence, sequenceOK := typeArgumentSequenceIdentity(formals)
			if !sequenceOK {
				return false
			}
			key := mountedTypeArgumentsKey{module: mount.ModuleID, types: typesID}
			if prior, duplicate := table.mounted[key]; duplicate {
				if !prior.Same(sequence) {
					return false
				}
			} else {
				table.mounted[key] = sequence
			}
		}
	}
	a.typeArguments = table
	return true
}
