package engine

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

const (
	canonicalResultContractDomain  = "engine/result-contract"
	canonicalResultContractVersion = 1
	canonicalResultCellDomain      = "engine/result-cell"
	canonicalResultCellVersion     = 1
)

// CanonicalResultContract is the immutable identity contract for one query
// result family and codec. It is created at publication time and shared by
// every result cell encoded under that registration.
type CanonicalResultContract struct {
	family  identity.ContentID
	codec   identity.SemanticKey
	content identity.ContentID
}

// NewCanonicalResultContract seals the registration identity used by result
// cells. The codec version is part of the contract identity: changing the
// interpretation of an unchanged codec digest therefore cannot reuse old
// result cells.
func NewCanonicalResultContract(family identity.ContentID, codec identity.SemanticKey) (CanonicalResultContract, bool) {
	if !family.Available() || !codec.Available() {
		return CanonicalResultContract{}, false
	}
	digest := codec.Digest()
	content := framedContentID(canonicalResultContractDomain, canonicalResultContractVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(family[:]) == nil &&
			writer.Bytes(digest[:]) == nil &&
			writer.Uint(codec.Version()) == nil
	})
	if !content.Available() {
		return CanonicalResultContract{}, false
	}
	return CanonicalResultContract{family: family, codec: codec, content: content}, true
}

func (contract CanonicalResultContract) Available() bool {
	return contract.content.Available()
}

func (contract CanonicalResultContract) FamilyID() identity.ContentID {
	if !contract.Available() {
		return identity.ContentID{}
	}
	return contract.family
}

func (contract CanonicalResultContract) Codec() identity.SemanticKey {
	if !contract.Available() {
		return identity.SemanticKey{}
	}
	return contract.codec
}

func (contract CanonicalResultContract) ContentID() identity.ContentID {
	if !contract.Available() {
		return identity.ContentID{}
	}
	return contract.content
}

// CanonicalResultCell is the domain-blind, transitively immutable result of
// encoding one query answer. A domain validates its typed Answer and emits the
// canonical payload once; no callback, algebra, schema, typed observation, or
// mutable byte slice crosses this boundary.
//
// Payload is retained as a string deliberately. A string owns immutable bytes
// and can be borrowed by decoders without an accessor allocation or defensive
// copy. Construction performs the one ownership copy from the encoder's mutable
// scratch buffer and precomputes ContentID once.
type CanonicalResultCell struct {
	contract identity.ContentID
	content  identity.ContentID
	payload  string
	rows     uint64
	present  bool
}

// NewCanonicalResultCell seals one owner-encoded query answer against a
// publication contract. Presence and row cardinality remain independent
// domain semantics and both enter the canonical identity.
func NewCanonicalResultCell(contract CanonicalResultContract, present bool, rows uint64, payload []byte) (CanonicalResultCell, bool) {
	if !contract.Available() {
		return CanonicalResultCell{}, false
	}
	contractID := contract.ContentID()
	presence := uint64(0)
	if present {
		presence = 1
	}
	content := framedContentID(canonicalResultCellDomain, canonicalResultCellVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(contractID[:]) == nil &&
			writer.Uint(presence) == nil &&
			writer.Count(rows) == nil &&
			writer.Bytes(payload) == nil
	})
	if !content.Available() {
		return CanonicalResultCell{}, false
	}
	return CanonicalResultCell{
		contract: contractID, content: content,
		payload: string(payload), rows: rows, present: present,
	}, true
}

func (cell CanonicalResultCell) Available() bool {
	// Construction is the sole writer and content is populated last, after the
	// complete envelope validates. Rechecking the identities on every hot read
	// would turn the immutable construction proof into a second authority.
	return cell.content.Available()
}

func (cell CanonicalResultCell) ContractID() identity.ContentID {
	if !cell.Available() {
		return identity.ContentID{}
	}
	return cell.contract
}

func (cell CanonicalResultCell) ContentID() identity.ContentID {
	if !cell.Available() {
		return identity.ContentID{}
	}
	return cell.content
}

func (cell CanonicalResultCell) Present() bool { return cell.Available() && cell.present }

func (cell CanonicalResultCell) RowCount() uint64 {
	if !cell.Available() {
		return 0
	}
	return cell.rows
}

// Payload returns a zero-copy immutable binary string. Domain decoders must
// first match the complete versioned contract identity; unlike []byte, the
// returned value cannot mutate the sealed cell.
func (cell CanonicalResultCell) Payload() string {
	if !cell.Available() {
		return ""
	}
	return cell.payload
}
