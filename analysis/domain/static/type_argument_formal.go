package static

import (
	"context"
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/keyspace"
)

const typeArgumentFormalDomain = "wippy.analysis.static.type-argument-formal.v1\x00"

// TypeArgumentFormal is the owner-neutral semantic content of one authored
// call type argument. It carries neither a Program/Static owner nor the local
// Static Term used to resolve it.
type TypeArgumentFormal struct{ id keyspace.ContentID }

// typeArgumentFormalTable is the seal-time semantic projection of exact
// Program call type-argument proofs. The opaque proof itself is the sole live
// lookup key, so an equivalent replay Program cannot rebind its local term
// into this Authority. Semantic graphs and the construction type authority
// are discarded after these scalar receipts are issued.
type typeArgumentFormalTable struct {
	byArgument map[keyspace.ContentID]TypeArgumentFormal
	mounted    map[mountedTypeArgumentsKey][]TypeArgumentFormal
	sealed     bool
}

// mountedTypeArgumentsKey is the sole Link-local substitution key for a
// reusable Program type-argument sequence. ModuleKey keeps duplicate mounts
// distinct; the sequence ID is Program-issued and carries no local term.
type mountedTypeArgumentsKey struct {
	module keyspace.ContentID
	types  keyspace.ContentID
}

// MountedTypeArguments is Static's opaque, ordered receipt for one mounted
// Program CallTypeArguments row. It exposes only already-issued formal
// semantic values; Pack cannot reconstruct a Program or Static join.
type MountedTypeArguments struct {
	owner *Authority
	key   mountedTypeArgumentsKey
}

func (receipt MountedTypeArguments) rows() ([]TypeArgumentFormal, bool) {
	if receipt.owner == nil || !receipt.owner.typeArguments.sealed || !receipt.key.module.Available() || !receipt.key.types.Available() {
		return nil, false
	}
	rows, ok := receipt.owner.typeArguments.mounted[receipt.key]
	if !ok {
		return nil, false
	}
	for _, row := range rows {
		if !row.Available() {
			return nil, false
		}
	}
	return rows, true
}
func (receipt MountedTypeArguments) Available() bool { _, ok := receipt.rows(); return ok }
func (receipt MountedTypeArguments) Count() int {
	rows, ok := receipt.rows()
	if !ok {
		return 0
	}
	return len(rows)
}
func (receipt MountedTypeArguments) At(index int) (TypeArgumentFormal, bool) {
	rows, ok := receipt.rows()
	if !ok || index < 0 || index >= len(rows) {
		return TypeArgumentFormal{}, false
	}
	return rows[index], true
}

func (formal TypeArgumentFormal) Available() bool { return formal.id.Available() }

func (formal TypeArgumentFormal) ContentID() (keyspace.ContentID, bool) {
	return formal.id, formal.Available()
}

func (formal TypeArgumentFormal) Equal(other TypeArgumentFormal) bool {
	return formal.Available() && other.Available() && formal == other
}

// MountedCallTypeArguments resolves Static's sealed module substitution for
// an artifact CallTypeArguments semantic ID. No Program handle, ordinal, or
// raw type term is accepted after Authority sealing.
func (a *Authority) MountedCallTypeArguments(module, types keyspace.ContentID) (MountedTypeArguments, bool) {
	if a == nil || !a.typeArguments.sealed || !module.Available() || !types.Available() {
		return MountedTypeArguments{}, false
	}
	receipt := MountedTypeArguments{owner: a, key: mountedTypeArgumentsKey{module: module, types: types}}
	return receipt, receipt.Available()
}

func issueMountedTypeArgumentFormal(types *typeauthority.Authority, referenceID keyspace.ContentID) (TypeArgumentFormal, bool) {
	if types == nil || !referenceID.Available() {
		return TypeArgumentFormal{}, false
	}
	reference, referenceOK := types.FindByReferenceID(referenceID)
	value, valueOK := types.Resolve(reference)
	if !referenceOK || !valueOK || value == nil {
		return TypeArgumentFormal{}, false
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil || len(encoded) == 0 {
		return TypeArgumentFormal{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(typeArgumentFormalDomain))
	_, _ = hash.Write(encoded)
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	formal := TypeArgumentFormal{id: id}
	return formal, formal.Available()
}

// sealMountedTypeArgumentFormals consumes only ProgramArtifact rows and the
// Link-local type authority. It does not reopen source graph authority.
func (a *Authority) sealMountedTypeArgumentFormals() bool {
	if a == nil || a.types == nil || len(a.mounts) == 0 {
		return false
	}
	table := typeArgumentFormalTable{byArgument: make(map[keyspace.ContentID]TypeArgumentFormal), mounted: make(map[mountedTypeArgumentsKey][]TypeArgumentFormal), sealed: true}
	for _, mount := range a.mounts {
		if mount.Artifact == nil || !mount.Artifact.Available() || !mount.ModuleID.Available() {
			return false
		}
		grouped := make(map[keyspace.ContentID][]programartifact.StaticTypeArgumentRow)
		if receipt, receiptOK := mount.Artifact.PackReceipt(); receiptOK {
			for callIndex := 0; callIndex < receipt.CallCount(); callIndex++ {
				call, callOK := receipt.CallAt(callIndex)
				typesID := call.TypeArgumentsID()
				if !callOK || !typesID.Available() {
					return false
				}
				if _, exists := grouped[typesID]; !exists {
					grouped[typesID] = nil
				}
			}
		} else {
			return false
		}
		for index := 0; index < mount.Artifact.StaticTypeArgumentCount(); index++ {
			row, rowOK := mount.Artifact.StaticTypeArgumentAt(index)
			if !rowOK || !row.Available() {
				return false
			}
			grouped[row.TypesID()] = append(grouped[row.TypesID()], row)
		}
		for typesID, rows := range grouped {
			formals := make([]TypeArgumentFormal, len(rows))
			for index, row := range rows {
				if row.Index() != uint32(index) {
					return false
				}
				formal, formalOK := issueMountedTypeArgumentFormal(a.types, row.ReferenceID())
				if !formalOK {
					return false
				}
				if prior, duplicate := table.byArgument[row.ID()]; duplicate && !prior.Equal(formal) {
					return false
				}
				table.byArgument[row.ID()] = formal
				formals[index] = formal
			}
			key := mountedTypeArgumentsKey{module: mount.ModuleID, types: typesID}
			if prior, duplicate := table.mounted[key]; duplicate {
				if len(prior) != len(formals) {
					return false
				}
				for index := range formals {
					if !prior[index].Equal(formals[index]) {
						return false
					}
				}
			} else {
				table.mounted[key] = formals
			}
		}
	}
	a.typeArguments = table
	return true
}
