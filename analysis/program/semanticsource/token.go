package semanticsource

import (
	"crypto/sha256"
	"encoding/binary"
)

// Origin is the generated numeric owner of a semantic relation family.
// Values are intentionally not derived from Go names, package paths, source
// order, or source locations.
type Origin uint32

// Revision distinguishes incompatible generated definitions for one Origin.
type Revision uint16

// Facet selects one relation facet of an Origin. Facet zero is the primary
// relation; every non-zero facet is required to have that primary relation in
// the same Schema.
type Facet uint16

// Token is the opaque, comparable identity of one semantic relation. Its
// identity protocol is the tuple (origin, facet, revision, digest), where the
// digest is derived only from the canonical fixed-width numeric encoding of
// the first three fields.
type Token struct {
	origin   Origin
	facet    Facet
	revision Revision
	digest   uint64
}

// Origin reports the generated numeric family owner.
func (t Token) Origin() Origin { return t.origin }

// Facet reports the generated facet selector. Zero denotes the primary
// relation.
func (t Token) Facet() Facet { return t.facet }

// Revision reports the generated definition revision.
func (t Token) Revision() Revision { return t.revision }

// Digest reports the integrity word computed from the canonical numeric
// protocol. It is not a name-derived identifier.
func (t Token) Digest() uint64 { return t.digest }

// Identity reports the canonical collision-resistant identity for an issued
// token. The identity is SHA-256 over the fixed-width origin, facet, and
// revision protocol; it contains no names, source order, or package data.
//
// Invalid, zero, or forged Tokens have no usable identity.
func (t Token) Identity() ([sha256.Size]byte, bool) {
	if !t.valid() {
		return [sha256.Size]byte{}, false
	}
	return tokenIdentity(t.origin, t.facet, t.revision), true
}

// Primary reports whether the token names an Origin's primary relation.
func (t Token) Primary() bool { return t.facet == 0 }

func (t Token) valid() bool {
	return t.origin != 0 && t.revision != 0 && t.digest == tokenDigest(t.origin, t.facet, t.revision)
}

func (t Token) parent() Token {
	return issuedToken(t.origin, 0, t.revision)
}

func compareToken(left, right Token) int {
	if left.origin != right.origin {
		if left.origin < right.origin {
			return -1
		}
		return 1
	}
	if left.facet != right.facet {
		if left.facet < right.facet {
			return -1
		}
		return 1
	}
	if left.revision != right.revision {
		if left.revision < right.revision {
			return -1
		}
		return 1
	}
	if left.digest != right.digest {
		if left.digest < right.digest {
			return -1
		}
		return 1
	}
	return 0
}

// issuedToken is available only to same-package generated definitions.
func issuedToken(origin Origin, facet Facet, revision Revision) Token {
	return Token{origin: origin, facet: facet, revision: revision, digest: tokenDigest(origin, facet, revision)}
}

func tokenDigest(origin Origin, facet Facet, revision Revision) uint64 {
	identity := tokenIdentity(origin, facet, revision)
	return binary.BigEndian.Uint64(identity[:8])
}

func tokenIdentity(origin Origin, facet Facet, revision Revision) [sha256.Size]byte {
	var data [8]byte
	binary.BigEndian.PutUint32(data[0:4], uint32(origin))
	binary.BigEndian.PutUint16(data[4:6], uint16(facet))
	binary.BigEndian.PutUint16(data[6:8], uint16(revision))
	return sha256.Sum256(data[:])
}
