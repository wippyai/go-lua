// Package typecontract owns the neutral, portable representation of one
// authored static-type declaration.
//
// This package deliberately does not know the analyzer's type implementation.
// A domain owns the conversion from its live type graph to the canonical bytes
// carried here, and it owns decoding, assignability, callable admission, and
// fresh-value admission. Program and Snapshot consumers may retain and
// compare Type values, but they must not interpret their bytes.
package typecontract

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

// FormatDomain separates this envelope from every other content stream. The
// canonical bytes inside Type remain owned by the domain encoder; this domain
// covers the envelope's own framing and digest.
const FormatDomain = "wippy.analysis.schema.typecontract"

// FormatVersion identifies the envelope framing, not the domain type algebra.
// A change to formal-count framing, primitive tags, or digest framing requires
// a new version. A change to a domain's canonical type encoding belongs to that
// domain and must not be silently accepted as this version.
const FormatVersion uint64 = 1

// Primitive is the small closed set of primitive declarations that a domain
// may use without carrying a graph encoding. These are declaration atoms, not
// a type checker: this package never assigns, widens, or decodes them.
type Primitive uint8

const (
	PrimitiveInvalid Primitive = iota
	PrimitiveNil
	PrimitiveBoolean
	PrimitiveNumber
	PrimitiveInteger
	PrimitiveString
	PrimitiveAny
	PrimitiveNever
)

// Available reports whether p is one of the portable primitive atoms.
func (p Primitive) Available() bool {
	return p > PrimitiveInvalid && p <= PrimitiveNever
}

// Type is an immutable portable type declaration.
//
// Exactly one representation is present: either primitive is available, or
// encoded is a non-empty copy of bytes produced and validated by a domain's
// canonical encoder. ExternalFormals is part of the identity because the same
// graph bytes under a different outer formal scope are not interchangeable.
// The zero value is unavailable.
type Type struct {
	encoded        []byte
	primitive      Primitive
	externalFormal uint32
	digest         identity.ContentID
}

// NewEncoded admits an ownership-isolated canonical encoding. It checks only
// envelope shape; interpreting or validating the graph is the domain owner's
// responsibility before calling this constructor.
func NewEncoded(encoded []byte, externalFormals uint32) (Type, bool) {
	if len(encoded) == 0 {
		return Type{}, false
	}
	value := Type{
		encoded:        append([]byte(nil), encoded...),
		externalFormal: externalFormals,
	}
	value.digest = digest(value)
	return value, value.digest.Available()
}

// NewPrimitive creates one opaque primitive declaration. Primitive values have
// no domain graph bytes; a domain adapter may lower them to its own canonical
// encoding when it needs a decoded semantic value.
func NewPrimitive(primitive Primitive) (Type, bool) {
	if !primitive.Available() {
		return Type{}, false
	}
	value := Type{primitive: primitive}
	value.digest = digest(value)
	return value, value.digest.Available()
}

// Available reports whether the declaration was admitted by a constructor.
func (value Type) Available() bool {
	return value.primitive.Available() || len(value.encoded) != 0
}

// Primitive returns the atom carried by value. The second result is false for
// encoded graph declarations.
func (value Type) Primitive() (Primitive, bool) {
	if !value.primitive.Available() {
		return PrimitiveInvalid, false
	}
	return value.primitive, true
}

// Bytes returns an ownership-isolated copy of the domain encoding. Primitive
// declarations intentionally return nil: their semantic encoding belongs to
// the domain adapter, not to this neutral package.
func (value Type) Bytes() []byte {
	if len(value.encoded) == 0 {
		return nil
	}
	return append([]byte(nil), value.encoded...)
}

// ExternalFormals is the number of outer formal coordinates bound by the
// domain encoding. Primitive declarations always have zero external formals.
func (value Type) ExternalFormals() uint32 { return value.externalFormal }

// Digest is the stable identity of the complete neutral envelope. It includes
// the envelope version, representation kind, formal count, and exact bytes.
func (value Type) Digest() identity.ContentID { return value.digest }

// Equal compares the complete declaration identity, including representation
// kind and external formal scope. It does not compare decoded semantic values.
func (value Type) Equal(other Type) bool {
	if value.digest != other.digest || value.Available() != other.Available() {
		return false
	}
	if value.primitive != other.primitive || value.externalFormal != other.externalFormal || len(value.encoded) != len(other.encoded) {
		return false
	}
	for index := range value.encoded {
		if value.encoded[index] != other.encoded[index] {
			return false
		}
	}
	return true
}

// digest uses length-delimited fields so an atom, a graph, and a graph under a
// different formal scope cannot alias. The bytes are never interpreted here.
func digest(value Type) (id identity.ContentID) {
	hash := sha256.New()
	writeFrame := func(raw []byte) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(raw)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write(raw)
	}
	writeFrame([]byte(FormatDomain))
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], FormatVersion)
	writeFrame(version[:])
	var formals [4]byte
	binary.BigEndian.PutUint32(formals[:], value.externalFormal)
	writeFrame(formals[:])
	var primitive [1]byte
	primitive[0] = byte(value.primitive)
	writeFrame(primitive[:])
	writeFrame(value.encoded)
	copy(id[:], hash.Sum(nil))
	return id
}
