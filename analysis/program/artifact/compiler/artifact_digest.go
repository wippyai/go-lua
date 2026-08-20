package compiler

import (
	"crypto/sha256"
	"hash"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/canonical"
)

var digestSinks sync.Pool

// hashSinks pools raw SHA-256 hash.Hash state for callers that frame their
// own preimage bytes directly (internal/framing.Writer) instead of going
// through the canonical.DigestWriter tag scheme digestSink wraps. Pooling
// the hasher does not touch the framing: the caller's byte sequence into the
// hash is unchanged, only the hash.Hash allocation is reused.
var hashSinks sync.Pool

// acquireHash returns a reset SHA-256 hasher from the pool, allocating one
// only when the pool is empty.
func acquireHash() hash.Hash {
	if h, ok := hashSinks.Get().(hash.Hash); ok && h != nil {
		h.Reset()
		return h
	}
	return sha256.New()
}

// releaseHash returns a hasher to the pool for reuse. The caller must not
// use h again after this call.
func releaseHash(h hash.Hash) {
	if h == nil {
		return
	}
	hashSinks.Put(h)
}

type field struct {
	bytes []byte
	uint  uint64
	kind  uint8
}

const (
	fieldBytes uint8 = iota + 1
	fieldUint
	fieldBool
)

func bytesField(value identity.ContentID) field { return field{bytes: value[:], kind: fieldBytes} }
func keyField(value schema.Key) field           { return field{bytes: []byte(value), kind: fieldBytes} }
func uintField(value uint64) field              { return field{uint: value, kind: fieldUint} }
func boolField(value bool) field {
	if value {
		return field{uint: 1, kind: fieldBool}
	}
	return field{kind: fieldBool}
}

func digest(domain string, version uint64, fields ...field) identity.ContentID {
	sink, _ := digestSinks.Get().(*digestSink)
	if sink == nil {
		sink = new(digestSink)
	}
	sink.ok = sink.writer.Reset(domain, version) == nil
	sink.add(fields...)
	id := sink.sum()
	digestSinks.Put(sink)
	return id
}

type digestSink struct {
	writer canonical.DigestWriter
	ok     bool
}

func newDigestSink(domain string, version uint64) digestSink {
	var sink digestSink
	sink.ok = sink.writer.Reset(domain, version) == nil
	return sink
}

func (sink *digestSink) add(values ...field) {
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

func (sink *digestSink) fail() { sink.ok = false }

func (sink *digestSink) sum() identity.ContentID {
	if sink == nil || !sink.ok || sink.writer.Finish() != nil {
		return identity.ContentID{}
	}
	return identity.ContentID(sink.writer.Sum())
}
