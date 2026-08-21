// Package digest owns the canonical tagged identity framing shared by the
// artifact and compiler packages. It commits only opaque scalar fields; the
// callers retain ownership of the domain-specific preimage order.
package digest

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/canonical"
)

var sinks sync.Pool

// Field is one opaque canonical identity field. The zero value is invalid and
// causes a fail-closed digest.
type Field struct {
	bytes []byte
	uint  uint64
	kind  uint8
}

const (
	fieldBytes uint8 = iota + 1
	fieldUint
	fieldBool
)

// Bytes commits one raw byte-string field. The bytes are consumed when Add or
// Digest is called; canonical.DigestWriter copies them into its own scratch
// storage before hashing.
func Bytes(value []byte) Field { return Field{bytes: value, kind: fieldBytes} }

// String commits one string as the bytes of a canonical byte-string field.
func String(value string) Field { return Field{bytes: []byte(value), kind: fieldBytes} }

// ContentID commits an identity.ContentID as its raw 32-byte payload.
func ContentID(value identity.ContentID) Field { return Bytes(value[:]) }

// Key commits a schema key as its raw string payload.
func Key(value schema.Key) Field { return String(string(value)) }

// Uint commits one canonical unsigned integer field.
func Uint(value uint64) Field { return Field{uint: value, kind: fieldUint} }

// Bool commits one canonical boolean field as the tagged scalar values zero
// and one.
func Bool(value bool) Field {
	if value {
		return Field{uint: 1, kind: fieldBool}
	}
	return Field{kind: fieldBool}
}

// Digest returns the content identity for one canonical domain/version
// preimage. Unknown fields and any framing failure produce an unavailable
// zero identity.
func Digest(domain string, version uint64, fields ...Field) identity.ContentID {
	sink, _ := sinks.Get().(*Sink)
	if sink == nil {
		sink = new(Sink)
	}
	sink.ok = sink.writer.Reset(domain, version) == nil
	sink.Add(fields...)
	id := sink.Sum()
	sinks.Put(sink)
	return id
}

// Sink incrementally constructs one canonical domain/version preimage.
// Callers may add fields in their semantic identity order and call Sum once.
type Sink struct {
	writer canonical.DigestWriter
	ok     bool
}

// WriteContentID implements identity.IdentityWriter without exposing the
// artifact digest field vocabulary to schema owners.
func (sink *Sink) WriteContentID(value identity.ContentID) bool {
	if sink == nil {
		return false
	}
	sink.Add(ContentID(value))
	return sink.Available()
}

// WriteUint implements identity.IdentityWriter for one unsigned field.
func (sink *Sink) WriteUint(value uint64) bool {
	if sink == nil {
		return false
	}
	sink.Add(Uint(value))
	return sink.Available()
}

// WriteBool implements identity.IdentityWriter for one canonical boolean
// field. Bool fields use the historical tagged unsigned representation.
func (sink *Sink) WriteBool(value bool) bool {
	if sink == nil {
		return false
	}
	sink.Add(Bool(value))
	return sink.Available()
}

// WriteString implements identity.StringIdentityWriter for one canonical
// byte-string field without exposing the artifact digest field vocabulary to
// schema owners.
func (sink *Sink) WriteString(value string) bool {
	if sink == nil {
		return false
	}
	sink.Add(String(value))
	return sink.Available()
}

// NewSink starts one streaming canonical digest. A reset failure leaves the
// sink unavailable, and Sum will return the zero identity.
func NewSink(domain string, version uint64) Sink {
	var sink Sink
	sink.ok = sink.writer.Reset(domain, version) == nil
	return sink
}

// Add appends fields to the preimage. A nil, failed, or invalid sink remains
// failed and ignores later fields.
func (sink *Sink) Add(values ...Field) {
	if sink == nil || !sink.ok {
		return
	}
	for _, value := range values {
		var err error
		switch value.kind {
		case fieldBytes:
			err = sink.writer.Bytes(value.bytes)
		case fieldUint, fieldBool:
			err = sink.writer.Uint(value.uint)
		default:
			sink.ok = false
			return
		}
		if err != nil {
			sink.ok = false
			return
		}
	}
}

// Fail marks the sink unavailable. It is irreversible until the sink is
// started again with NewSink.
func (sink *Sink) Fail() {
	if sink != nil {
		sink.ok = false
	}
}

// Available reports whether the sink is still accepting a valid preimage.
func (sink *Sink) Available() bool { return sink != nil && sink.ok }

// Sum closes the preimage and returns its identity. Any invalid field,
// previous failure, or framing error returns the unavailable zero identity.
func (sink *Sink) Sum() identity.ContentID {
	if sink == nil || !sink.ok || sink.writer.Finish() != nil {
		return identity.ContentID{}
	}
	return identity.ContentID(sink.writer.Sum())
}
