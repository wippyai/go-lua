package engine

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

const (
	canonicalResultCellDomain  = "engine/result-cell"
	canonicalResultCellVersion = 1
)

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
	family  identity.ContentID
	codec   identity.SemanticKey
	content identity.ContentID
	payload string
	rows    uint64
	present bool
}

// NewCanonicalResultCell seals one owner-encoded query answer. family and
// codec are the identities from the sealed query registration, not identities
// supplied by the answer. Presence and row cardinality remain independent
// domain semantics and both enter the canonical identity.
func NewCanonicalResultCell(family identity.ContentID, codec identity.SemanticKey, present bool, rows uint64, payload []byte) (CanonicalResultCell, bool) {
	if !family.Available() || !codec.Available() {
		return CanonicalResultCell{}, false
	}
	digest := codec.Digest()
	presence := uint64(0)
	if present {
		presence = 1
	}
	content := framedContentID(canonicalResultCellDomain, canonicalResultCellVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Bytes(family[:]) == nil &&
			writer.Bytes(digest[:]) == nil &&
			writer.Uint(codec.Version()) == nil &&
			writer.Uint(presence) == nil &&
			writer.Count(rows) == nil &&
			writer.Bytes(payload) == nil
	})
	if !content.Available() {
		return CanonicalResultCell{}, false
	}
	return CanonicalResultCell{
		family: family, codec: codec, content: content,
		payload: string(payload), rows: rows, present: present,
	}, true
}

func (cell CanonicalResultCell) Available() bool {
	// Construction is the sole writer and content is populated last, after the
	// complete envelope validates. Rechecking the identities on every hot read
	// would turn the immutable construction proof into a second authority.
	return cell.content.Available()
}

func (cell CanonicalResultCell) FamilyID() identity.ContentID {
	if !cell.Available() {
		return identity.ContentID{}
	}
	return cell.family
}

func (cell CanonicalResultCell) Codec() identity.SemanticKey {
	if !cell.Available() {
		return identity.SemanticKey{}
	}
	return cell.codec
}

func (cell CanonicalResultCell) CodecID() identity.ContentID {
	if !cell.Available() {
		return identity.ContentID{}
	}
	return identity.ContentID(cell.codec.Digest())
}

func (cell CanonicalResultCell) CodecVersion() uint64 {
	if !cell.Available() {
		return 0
	}
	return cell.codec.Version()
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
// first match FamilyID and the complete versioned Codec identity; unlike
// []byte, the returned value cannot mutate the sealed cell.
func (cell CanonicalResultCell) Payload() string {
	if !cell.Available() {
		return ""
	}
	return cell.payload
}
