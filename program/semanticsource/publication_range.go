package semanticsource

import (
	"crypto/sha256"
	"encoding/binary"
)

// PublicationRange is one detached, owner-local publication interval.  The
// caller supplies the interval's fixed definitions; this type does not own a
// catalog, registry, or erased relation stream.  Its only job is to preserve
// the already validated cardinality claims while exposing a bounded cursor.
//
// The range is deliberately stricter than Publisher: the rows must arrive in
// the exact order of definitions supplied by the owner.  That makes a child
// fragment's seal a proof of its fixed schema interval rather than a set
// membership check which could silently accept a reordered or duplicated
// denominator.
type PublicationRange struct {
	definitions []RelationDef
	rows        []Publication
	count       int
	digest      [sha256.Size]byte
	digested    bool
}

// SealPublicationRange validates and detaches one fixed owner interval.
// Definitions are the owner-local schema range, not a new global schema.  A
// row with a token outside the range is foreign; a repeated expected token is
// duplicate; a permutation is out of order; and a length mismatch is missing.
func SealPublicationRange(definitions []RelationDef, rows []Publication) (PublicationRange, error) {
	if len(definitions) == 0 {
		return PublicationRange{}, ErrMissingPublication
	}
	if len(rows) != len(definitions) {
		return PublicationRange{}, ErrMissingPublication
	}

	// Validate the fixed expected interval first.  A malformed definition is a
	// schema error, never something an owner can repair by supplying another
	// row.
	for _, definition := range definitions {
		if !definition.valid() {
			return PublicationRange{}, ErrInvalidDefinition
		}
	}

	seen := make(map[Token]struct{}, len(rows))
	for _, row := range rows {
		if !row.valid() {
			return PublicationRange{}, ErrInvalidPublication
		}
		token := row.Definition().Token()
		if _, duplicate := seen[token]; duplicate {
			return PublicationRange{}, ErrDuplicatePublication
		}
		seen[token] = struct{}{}
		if !containsDefinition(definitions, row.Definition()) {
			return PublicationRange{}, ErrUnexpectedPublication
		}
	}

	// Every expected token is now known to be present exactly once, so a
	// positional mismatch is specifically an order violation.  Keeping this
	// check separate makes malformed permutations diagnosable without retaining
	// a second index or registry in the sealed range.
	for index, definition := range definitions {
		if rows[index].Definition() != definition {
			return PublicationRange{}, ErrPublicationOrder
		}
	}

	ownedDefinitions := append([]RelationDef(nil), definitions...)
	ownedRows := append([]Publication(nil), rows...)
	digest, ok := digestPublicationRows(ownedDefinitions, ownedRows)
	if !ok {
		return PublicationRange{}, ErrInvalidPublication
	}
	return PublicationRange{
		definitions: ownedDefinitions,
		rows:        ownedRows,
		count:       len(ownedRows),
		digest:      digest,
		digested:    true,
	}, nil
}

func containsDefinition(definitions []RelationDef, wanted RelationDef) bool {
	for _, definition := range definitions {
		if definition == wanted {
			return true
		}
	}
	return false
}

// Valid reports whether the range is a complete detached interval.
func (rangeValue PublicationRange) Valid() bool {
	if !rangeValue.digested || rangeValue.count <= 0 || len(rangeValue.definitions) != rangeValue.count || len(rangeValue.rows) != rangeValue.count {
		return false
	}
	for index, definition := range rangeValue.definitions {
		if !definition.valid() || !rangeValue.rows[index].valid() || rangeValue.rows[index].Definition() != definition {
			return false
		}
	}
	computed, ok := digestPublicationRows(rangeValue.definitions, rangeValue.rows)
	return ok && computed == rangeValue.digest
}

// Count returns the fixed snapshot cardinality, including rows whose claims
// are zero.
func (rangeValue PublicationRange) Count() int {
	if !rangeValue.Valid() {
		return 0
	}
	return rangeValue.count
}

// At returns one exact owner-local publication.  At(Count()) is always false.
func (rangeValue PublicationRange) At(index int) (Publication, bool) {
	if !rangeValue.Valid() || index < 0 || index >= rangeValue.count {
		return Publication{}, false
	}
	return rangeValue.rows[index], true
}

// Publications returns a detached copy of the exact fixed interval.
func (rangeValue PublicationRange) Publications() []Publication {
	if !rangeValue.Valid() {
		return nil
	}
	return append([]Publication(nil), rangeValue.rows...)
}

// Snapshot returns the immutable range value without copying its sealed
// claims. The returned value is safe to retain because the underlying rows
// are private to the owning seal.
func (rangeValue PublicationRange) Snapshot() PublicationRange { return rangeValue }

// Digest returns the scalar identity captured by the one successful range
// seal. It retains no second denominator and never re-seals the claims.
func (rangeValue PublicationRange) Digest() ([sha256.Size]byte, bool) {
	if !rangeValue.Valid() {
		return [sha256.Size]byte{}, false
	}
	return rangeValue.digest, true
}

// DigestPublicationCounts computes the same scalar seal as a PublicationRange
// from one fixed definition interval and its already-validated cardinalities.
// It deliberately retains neither definitions nor claims, and does not seal a
// second range. Owner views use it to compare their live typed projections with
// the scalar captured by the one child fragment seal.
func DigestPublicationCounts(definitions []RelationDef, counts []int) ([sha256.Size]byte, bool) {
	if len(definitions) == 0 || len(definitions) != len(counts) {
		return [sha256.Size]byte{}, false
	}
	hash := sha256.New()
	for index, definition := range definitions {
		if !definition.valid() || counts[index] < 0 || !writePublicationDigest(hash, definition, counts[index]) {
			return [sha256.Size]byte{}, false
		}
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, true
}

func digestPublicationRows(definitions []RelationDef, rows []Publication) ([sha256.Size]byte, bool) {
	if len(definitions) == 0 || len(definitions) != len(rows) {
		return [sha256.Size]byte{}, false
	}
	hash := sha256.New()
	for index, definition := range definitions {
		if !definition.valid() || !rows[index].valid() || rows[index].Definition() != definition || !writePublicationDigest(hash, definition, rows[index].Count()) {
			return [sha256.Size]byte{}, false
		}
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, true
}

func writePublicationDigest(hash interface{ Write([]byte) (int, error) }, definition RelationDef, count int) bool {
	if hash == nil || !definition.valid() || count < 0 {
		return false
	}
	token := definition.Token()
	var frame [24]byte
	binary.BigEndian.PutUint32(frame[0:4], uint32(token.Origin()))
	binary.BigEndian.PutUint16(frame[4:6], uint16(token.Facet()))
	binary.BigEndian.PutUint16(frame[6:8], uint16(token.Revision()))
	binary.BigEndian.PutUint64(frame[8:16], token.Digest())
	binary.BigEndian.PutUint64(frame[16:24], uint64(count))
	_, err := hash.Write(frame[:])
	return err == nil
}

// PublicationCursor traverses one immutable owner-local interval.  The count
// is captured when Cursor is created, so traversal cannot grow or shrink if a
// caller retains another value copy of the range.
type PublicationCursor struct {
	rangeValue PublicationRange
	count      int
	index      int
}

// Cursor creates a detached cursor with a count snapshot.
func (rangeValue PublicationRange) Cursor() PublicationCursor {
	return PublicationCursor{rangeValue: rangeValue, count: rangeValue.Count()}
}

// Next returns each row at most once and then terminates at the snapshotted
// count, including for an interval containing zero-count publications.
func (cursor *PublicationCursor) Next() (Publication, bool) {
	if cursor == nil || !cursor.rangeValue.Valid() || cursor.index < 0 || cursor.index >= cursor.count {
		return Publication{}, false
	}
	row, ok := cursor.rangeValue.At(cursor.index)
	if !ok {
		return Publication{}, false
	}
	cursor.index++
	return row, true
}
