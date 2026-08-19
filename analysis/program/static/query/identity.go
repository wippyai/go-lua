package query

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// OccurrenceID is the detached identity of one authored static operand
// occurrence. It carries no runtime or inferred authority.
func OccurrenceID(owner identity.ContentID, family uint8, term keyspace.Term) (id identity.ContentID, ok bool) {
	if !owner.Available() || term == 0 || family == 0 {
		return identity.ContentID{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("program/static-occurrence/v1"))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(owner[:])
	_, _ = h.Write([]byte{family})
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(term))
	_, _ = h.Write(word[:])
	copy(id[:], h.Sum(nil))
	return id, id.Available()
}

// TypeReferenceID is the detached identity for one canonical type reference.
func TypeReferenceID(owner identity.ContentID, ref StaticTypeRef) (id identity.ContentID, ok bool) {
	if !owner.Available() || ref.Term() == 0 {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("program/static-type-reference/v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(owner[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(ref.Term()))
	_, _ = hash.Write(word[:])
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// ExpressionID is the identity of one authored static expression occurrence.
func ExpressionID(owner identity.ContentID, ref StaticTypeRef) (identity.ContentID, bool) {
	if !owner.Available() || ref.Term() == 0 {
		return identity.ContentID{}, false
	}
	return staticInputDigest("program/static-expression/v1", owner, ref.Term(), 0), true
}

// InputID issues a dense, index-bearing Static input identity.
func InputID(owner identity.ContentID, family uint8, source keyspace.Term, index uint32) (identity.ContentID, bool) {
	if !owner.Available() || source == 0 {
		return identity.ContentID{}, false
	}
	id := staticInputDigest("program/static-input/v1", owner, source, uint64(family)<<32|uint64(index))
	return id, id.Available()
}

// ScopeID is the identity of one authored static scope owner.
func ScopeID(owner identity.ContentID, scope keyspace.Term) (identity.ContentID, bool) {
	if !owner.Available() || scope == 0 {
		return identity.ContentID{}, false
	}
	id := staticInputDigest("program/static-scope/v1", owner, scope, 0)
	return id, id.Available()
}

func staticInputDigest(domain string, owner identity.ContentID, term keyspace.Term, index uint64) identity.ContentID {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(owner[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(term))
	_, _ = hash.Write(word[:])
	binary.BigEndian.PutUint64(word[:], index)
	_, _ = hash.Write(word[:])
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
