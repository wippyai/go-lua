package factor

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

const mountedPublicationBatchDomain = "wippy.analysis.effect.mounted-publication-batch.v1\x00"

// MountedPublicationBatch is Effect's one-call publication denominator.  It
// contains every authenticated publication receipt issued for every statically
// selected Target operation on one exact MountedCall.  The rows retain
// canonical Target operation order, followed by each operation's canonical
// ordinary/callback publication order; they are not sorted by receipt ID.
//
// A zero-row batch is meaningful: the exact call was admitted and no selected
// operation carried an authored publication consequence.
type MountedPublicationBatch struct {
	owner        *Algebra
	mounted      MountedCall
	application  identity.ContentID
	module       identity.ContentID
	occurrence   identity.ContentID
	rows         []MountedPublication
	id           identity.ContentID
	sealed       bool
	sealedScalar uint64
}

func mountedPublicationBatchID(owner, application, module, occurrence identity.ContentID, rows []MountedPublication) identity.ContentID {
	if !owner.Available() || !application.Available() || !module.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(mountedPublicationBatchDomain))
	for _, value := range [...]identity.ContentID{owner, application, module, occurrence} {
		_, _ = hash.Write(value[:])
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(rows)))
	_, _ = hash.Write(count[:])
	for _, row := range rows {
		rowID, ok := row.ContentID()
		if !ok {
			return identity.ContentID{}
		}
		_, _ = hash.Write(rowID[:])
	}
	return identity.ContentID(sha256.Sum256(hash.Sum(nil)))
}

func mountedPublicationBatchScalar(id identity.ContentID) uint64 {
	if !id.Available() {
		return 0
	}
	return binary.BigEndian.Uint64(id[:8]) | 1
}

func (batch MountedPublicationBatch) available() bool {
	return batch.sealed && batch.sealedScalar != 0 && batch.owner != nil && batch.owner.Valid() && batch.mounted.Valid() && batch.mounted.owner == batch.owner && batch.application.Available() && batch.module.Available() && batch.occurrence.Available() && batch.id.Available() && mountedPublicationBatchScalar(batch.id) == batch.sealedScalar
}

func (batch MountedPublicationBatch) valid() bool {
	if !batch.available() {
		return false
	}
	application, module, occurrence, identityOK := batch.owner.MountedCallIdentity(batch.mounted)
	if !identityOK || application != batch.application || module != batch.module || occurrence != batch.occurrence {
		return false
	}
	root, rootOK := batch.owner.RootForMountedCall(batch.mounted)
	if !rootOK {
		return false
	}
	expected, expectedOK := batch.owner.collectMountedPublications(root, batch.mounted)
	if !expectedOK || len(expected) != len(batch.rows) {
		return false
	}
	for index, row := range batch.rows {
		if !row.Valid() || row.owner != batch.owner || row.mounted != batch.mounted || row.mounted.owner != batch.owner || row.application != batch.application || row.module != batch.module || row.occurrence != batch.occurrence {
			return false
		}
		rowID, rowOK := row.ContentID()
		expectedID, expectedIDOK := expected[index].ContentID()
		if !rowOK || !expectedIDOK || rowID != expectedID {
			return false
		}
	}
	return batch.id == mountedPublicationBatchID(batch.owner.LinkID(), batch.application, batch.module, batch.occurrence, batch.rows)
}

// Valid reports that the batch is a complete Effect-owned row set for its
// exact mounted call.  It replays selected operation admission and receipt
// order as the full structural audit; hot row access uses the sealed scalar.
func (batch MountedPublicationBatch) Valid() bool { return batch.valid() }

// ContentID returns the stable identity over this exact mounted provenance
// and ordered publication-receipt IDs.
func (batch MountedPublicationBatch) ContentID() (identity.ContentID, bool) {
	return batch.id, batch.valid()
}

// SealedContentID is the O(1) lookup identity for an already admitted batch.
// ContentID and Valid remain the full structural audit.
func (batch MountedPublicationBatch) SealedContentID() (identity.ContentID, bool) {
	return batch.id, batch.available()
}

// MountedCall returns the exact opaque call receipt retained by this batch.
func (batch MountedPublicationBatch) MountedCall() (MountedCall, bool) {
	return batch.mounted, batch.available()
}

func (batch MountedPublicationBatch) CallProvenance() (module, call identity.ContentID, ok bool) {
	if !batch.available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	return batch.module, batch.occurrence, true
}

func (batch MountedPublicationBatch) ApplicationID() (identity.ContentID, bool) {
	return batch.application, batch.available()
}

// RowCount returns the sealed row denominator without replaying Target
// operation validation.
func (batch MountedPublicationBatch) RowCount() int {
	if !batch.available() {
		return 0
	}
	return len(batch.rows)
}

func (batch MountedPublicationBatch) RowAt(index int) (MountedPublication, bool) {
	if !batch.available() || index < 0 || index >= len(batch.rows) {
		return MountedPublication{}, false
	}
	return batch.rows[index], batch.rows[index].available()
}

// Rows returns a defensive copy.  Mutating the returned slice cannot remove,
// reorder, or replace rows retained by the sealed batch.
func (batch MountedPublicationBatch) Rows() []MountedPublication {
	if !batch.Valid() {
		return nil
	}
	rows := make([]MountedPublication, len(batch.rows))
	copy(rows, batch.rows)
	return rows
}

// collectMountedPublications is the one call-level admission walk. Target
// operations are consumed in Core's canonical order. A statically unselected
// operation is skipped; selected operations must pass the complete Effect
// publication validation or the entire batch fails closed.
func (a *Algebra) collectMountedPublications(root Root, mounted MountedCall) ([]MountedPublication, bool) {
	if a == nil || !a.Valid() || !a.ownsRoot(root) || !mounted.Valid() || mounted.owner != a {
		return nil, false
	}
	rows := make([]MountedPublication, 0)
	for index := 0; index < a.contract.Operations.OperationCount(); index++ {
		operation, operationOK := a.contract.Operations.OperationAt(index)
		if !operationOK {
			return nil, false
		}
		if !a.selectedMountedCall(root, mounted, operation) {
			continue
		}
		publications, publicationsOK := a.SelectedCallMountedPublications(root, mounted, operation)
		if !publicationsOK {
			return nil, false
		}
		rows = append(rows, publications...)
	}
	return rows, true
}

// PublicationBatchForMountedCall issues the complete call-level publication
// operand. The root and selected operation set are derived from Effect's own
// mounted-call directory; callers cannot self-attest either coordinate.
func (a *Algebra) PublicationBatchForMountedCall(mounted MountedCall) (MountedPublicationBatch, bool) {
	if a == nil || !a.Valid() || !mounted.Valid() || mounted.owner != a {
		return MountedPublicationBatch{}, false
	}
	root, rootOK := a.RootForMountedCall(mounted)
	if !rootOK {
		return MountedPublicationBatch{}, false
	}
	application, module, occurrence, identityOK := a.MountedCallIdentity(mounted)
	if !identityOK {
		return MountedPublicationBatch{}, false
	}
	rows, rowsOK := a.collectMountedPublications(root, mounted)
	if !rowsOK {
		return MountedPublicationBatch{}, false
	}
	sealedRows := make([]MountedPublication, len(rows))
	copy(sealedRows, rows)
	batch := MountedPublicationBatch{
		owner: a, mounted: mounted, application: application, module: module, occurrence: occurrence,
		rows: sealedRows, sealed: true,
	}
	batch.id = mountedPublicationBatchID(a.LinkID(), application, module, occurrence, sealedRows)
	batch.sealedScalar = mountedPublicationBatchScalar(batch.id)
	return batch, batch.Valid()
}
