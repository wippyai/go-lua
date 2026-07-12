// Package observationartifact defines the durable identity and wire envelope
// for diagnostic evidence produced by symbolic function relations.
//
// The package deliberately contains no CFG, expression-table, or relation-cell
// identifiers. Those are solve-local routing aids and must be resolved before
// an observation can cross an artifact boundary.
package observationartifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

const SchemaVersion uint16 = 1

type Kind = engineobservation.Kind

const (
	KindInvalid      = engineobservation.Invalid
	KindAssignment   = engineobservation.Assignment
	KindCallArgument = engineobservation.CallArgument
	KindCallResult   = engineobservation.CallResult
)

// SourceAnchor is the durable projection of one lowering-owned occurrence.
// ArtifactDigest is SHA-256 of service.StaticArtifactID.String(). Projection
// owns that mapping; this layer never computes an independent source map.
type SourceAnchor struct {
	ArtifactDigest [sha256.Size]byte
	Occurrence     engineobservation.Occurrence
}

func NewSourceAnchor(artifactDigest [sha256.Size]byte, occurrence engineobservation.Occurrence) (SourceAnchor, bool) {
	a := SourceAnchor{ArtifactDigest: artifactDigest, Occurrence: occurrence}
	return a, a.Valid()
}

func (a SourceAnchor) Valid() bool {
	return a.ArtifactDigest != ([sha256.Size]byte{}) && a.Occurrence.Valid()
}

func (a SourceAnchor) Less(b SourceAnchor) bool {
	if c := bytes.Compare(a.ArtifactDigest[:], b.ArtifactDigest[:]); c != 0 {
		return c < 0
	}
	return a.Occurrence.Less(b.Occurrence)
}

// InvocationID is the collision-resistant digest of the ordered call
// occurrence path through which an observation was instantiated. Zero means
// the observation belongs to its lexical owner directly.
type InvocationID = engineobservation.InvocationID

type Identity [sha256.Size]byte

type Record struct {
	Owner       lexicalidentity.StableLexicalBodyID
	Invocation  InvocationID
	Anchor      SourceAnchor
	Actual      EncodedValue
	Expected    EncodedValue
	HasExpected bool
}

func (r Record) Valid() bool {
	return r.Owner != (lexicalidentity.StableLexicalBodyID{}) && r.Anchor.Valid() && r.Anchor.Occurrence.Kind != engineobservation.CallInvocation && r.Actual.Valid() &&
		(r.HasExpected == r.Expected.Valid())
}

// EncodedValue can only be produced by an explicit, sealed universe codec.
// The observation layer never hashes or formats product.Value itself.
type EncodedValue struct {
	codecID string
	payload []byte
}

func (v EncodedValue) Valid() bool   { return v.codecID != "" }
func (v EncodedValue) Bytes() []byte { return append([]byte(nil), v.payload...) }

type CanonicalValueCodec interface {
	ID() string
	ValidateCanonical([]byte) bool
}

func (r Record) OccurrenceIdentity() (Identity, bool) {
	if !r.Valid() {
		return Identity{}, false
	}
	h := sha256.New()
	writeBytes(h, []byte("wippy.observation.identity.v1"))
	writeBytes(h, r.Owner[:])
	writeBytes(h, r.Invocation[:])
	writeAnchor(h, r.Anchor)
	var out Identity
	copy(out[:], h.Sum(nil))
	return out, true
}

// Identity includes the correlated value alternative; multiple legitimate
// actual/expected alternatives at one occurrence therefore remain distinct.
func (r Record) Identity() (Identity, bool) {
	occurrence, ok := r.OccurrenceIdentity()
	if !ok {
		return Identity{}, false
	}
	h := sha256.New()
	writeBytes(h, []byte("wippy.observation.record.v1"))
	writeBytes(h, occurrence[:])
	writeBytes(h, []byte(r.Actual.codecID))
	writeBytes(h, r.Actual.payload)
	if r.HasExpected {
		_, _ = h.Write([]byte{1})
		writeBytes(h, r.Expected.payload)
	} else {
		_, _ = h.Write([]byte{0})
	}
	var out Identity
	copy(out[:], h.Sum(nil))
	return out, true
}

// Universe fences values to one exact semantic axis/codec inventory. Both
// digests are full-width and caller-produced; a process-local registry pointer
// or axis ordinal is never part of the artifact.
type Universe struct {
	semanticDigest [sha256.Size]byte
	axisDigest     [sha256.Size]byte
	codec          CanonicalValueCodec
}

func SealUniverse(semanticDigest, axisDescriptorInventoryDigest [sha256.Size]byte, codec CanonicalValueCodec) (Universe, bool) {
	u := Universe{semanticDigest: semanticDigest, axisDigest: axisDescriptorInventoryDigest, codec: codec}
	_, ok := u.Digest()
	return u, ok
}

func (u Universe) CodecID() string {
	if u.codec == nil {
		return ""
	}
	return u.codec.ID()
}

func (u Universe) NewEncodedValue(payload []byte) (EncodedValue, bool) {
	if u.codec == nil || !u.codec.ValidateCanonical(payload) {
		return EncodedValue{}, false
	}
	return EncodedValue{codecID: u.codec.ID(), payload: append([]byte(nil), payload...)}, true
}

func (u Universe) Digest() ([sha256.Size]byte, bool) {
	if u.semanticDigest == ([sha256.Size]byte{}) || u.axisDigest == ([sha256.Size]byte{}) || u.codec == nil || u.codec.ID() == "" {
		return [sha256.Size]byte{}, false
	}
	h := sha256.New()
	writeBytes(h, []byte("wippy.observation.universe.v1"))
	writeBytes(h, u.semanticDigest[:])
	writeBytes(h, u.axisDigest[:])
	writeBytes(h, []byte(u.codec.ID()))
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out, true
}

type Artifact struct {
	SchemaVersion  uint16
	UniverseDigest [sha256.Size]byte
	Records        []Record
}

var ErrInvalid = errors.New("invalid observation artifact")

// Encode returns the canonical wire form. Records are ordered by their full
// canonical bytes and duplicate identities with unequal content are rejected.
func Encode(universe Universe, records []Record) ([]byte, error) {
	digest, ok := universe.Digest()
	if !ok {
		return nil, fmt.Errorf("%w: zero universe", ErrInvalid)
	}
	encoded := make([]encodedRecord, len(records))
	for i, record := range records {
		if record.Actual.codecID != universe.CodecID() || !universe.codec.ValidateCanonical(record.Actual.payload) ||
			(record.HasExpected && (record.Expected.codecID != universe.CodecID() || !universe.codec.ValidateCanonical(record.Expected.payload))) {
			return nil, fmt.Errorf("%w: record %d value codec differs from universe", ErrInvalid, i)
		}
		id, valid := record.Identity()
		if !valid {
			return nil, fmt.Errorf("%w: record %d", ErrInvalid, i)
		}
		encoded[i] = encodedRecord{id: id, body: encodeRecord(record)}
	}
	sort.Slice(encoded, func(i, j int) bool { return bytes.Compare(encoded[i].body, encoded[j].body) < 0 })
	unique := encoded[:0]
	byID := make(map[Identity][]byte, len(encoded))
	for _, record := range encoded {
		if prior, exists := byID[record.id]; exists {
			if !bytes.Equal(prior, record.body) {
				return nil, fmt.Errorf("%w: identity collision %x", ErrInvalid, record.id)
			}
			continue
		}
		byID[record.id] = record.body
		unique = append(unique, record)
	}
	var out bytes.Buffer
	out.WriteString("WOBS")
	writeUint16(&out, SchemaVersion)
	_, _ = out.Write(digest[:])
	writeUint32(&out, uint32(len(unique)))
	for _, record := range unique {
		writeBytes(&out, record.body)
	}
	return out.Bytes(), nil
}

// Decode validates schema and universe before exposing records.
func Decode(universe Universe, raw []byte) (Artifact, error) {
	digest, ok := universe.Digest()
	if !ok {
		return Artifact{}, fmt.Errorf("%w: zero universe", ErrInvalid)
	}
	r := wireReader{raw: raw}
	if string(r.take(4)) != "WOBS" || r.u16() != SchemaVersion || !bytes.Equal(r.take(sha256.Size), digest[:]) {
		return Artifact{}, fmt.Errorf("%w: schema or universe mismatch", ErrInvalid)
	}
	count := r.u32()
	if r.err != nil || uint64(count) > uint64(len(raw)) {
		return Artifact{}, fmt.Errorf("%w: record count", ErrInvalid)
	}
	records := make([]Record, 0, count)
	var prior []byte
	ids := make(map[Identity][]byte, count)
	for i := uint32(0); i < count; i++ {
		body := r.bytes()
		if r.err != nil || (prior != nil && bytes.Compare(prior, body) >= 0) {
			return Artifact{}, fmt.Errorf("%w: non-canonical record order", ErrInvalid)
		}
		record, err := decodeRecord(universe, body)
		if err != nil {
			return Artifact{}, err
		}
		id, _ := record.Identity()
		if other, exists := ids[id]; exists && !bytes.Equal(other, body) {
			return Artifact{}, fmt.Errorf("%w: identity collision %x", ErrInvalid, id)
		}
		ids[id], prior = append([]byte(nil), body...), append([]byte(nil), body...)
		records = append(records, record)
	}
	if r.err != nil || r.at != len(raw) {
		return Artifact{}, fmt.Errorf("%w: trailing or truncated bytes", ErrInvalid)
	}
	return Artifact{SchemaVersion: SchemaVersion, UniverseDigest: digest, Records: records}, nil
}

type encodedRecord struct {
	id   Identity
	body []byte
}

func encodeRecord(r Record) []byte {
	var out bytes.Buffer
	_, _ = out.Write(r.Owner[:])
	_, _ = out.Write(r.Invocation[:])
	writeAnchor(&out, r.Anchor)
	if r.HasExpected {
		_ = out.WriteByte(1)
	} else {
		_ = out.WriteByte(0)
	}
	writeBytes(&out, r.Actual.payload)
	writeBytes(&out, r.Expected.payload)
	return out.Bytes()
}

func decodeRecord(universe Universe, raw []byte) (Record, error) {
	r := wireReader{raw: raw}
	var out Record
	copy(out.Owner[:], r.take(sha256.Size))
	copy(out.Invocation[:], r.take(sha256.Size))
	out.Anchor = r.anchor()
	hasExpected := r.u8()
	if hasExpected > 1 {
		r.err = ErrInvalid
	}
	out.HasExpected = hasExpected == 1
	actual, expected := r.bytes(), r.bytes()
	var valid bool
	out.Actual, valid = universe.NewEncodedValue(actual)
	if !valid {
		r.err = ErrInvalid
	}
	if out.HasExpected {
		out.Expected, valid = universe.NewEncodedValue(expected)
		if !valid {
			r.err = ErrInvalid
		}
	} else if len(expected) != 0 {
		r.err = ErrInvalid
	}
	if r.err != nil || r.at != len(raw) || !out.Valid() {
		return Record{}, fmt.Errorf("%w: record payload", ErrInvalid)
	}
	return out, nil
}

type wireReader struct {
	raw []byte
	at  int
	err error
}

func (r *wireReader) take(n int) []byte {
	if r.err != nil || n < 0 || r.at+n > len(r.raw) {
		r.err = ErrInvalid
		return nil
	}
	out := r.raw[r.at : r.at+n]
	r.at += n
	return out
}
func (r *wireReader) u8() byte {
	b := r.take(1)
	if len(b) == 0 {
		return 0
	}
	return b[0]
}
func (r *wireReader) u16() uint16 {
	b := r.take(2)
	if len(b) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}
func (r *wireReader) u32() uint32 {
	b := r.take(4)
	if len(b) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}
func (r *wireReader) u64() uint64 {
	b := r.take(8)
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
func (r *wireReader) bytes() []byte {
	n := r.u32()
	if uint64(n) > uint64(len(r.raw)) {
		r.err = ErrInvalid
		return nil
	}
	return append([]byte(nil), r.take(int(n))...)
}
func (r *wireReader) anchor() SourceAnchor {
	var a SourceAnchor
	copy(a.ArtifactDigest[:], r.take(sha256.Size))
	a.Occurrence.Point.Ordinal = r.u32()
	a.Occurrence.Point.Phase = wir.DebugPhase(r.u8())
	a.Occurrence.Kind = engineobservation.Kind(r.u8())
	a.Occurrence.Slot = r.u32()
	return a
}

type writer interface{ Write([]byte) (int, error) }

func writeBytes(w writer, value []byte) { writeUint32(w, uint32(len(value))); _, _ = w.Write(value) }
func writeUint16(w writer, value uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], value)
	_, _ = w.Write(b[:])
}
func writeUint32(w writer, value uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], value)
	_, _ = w.Write(b[:])
}
func writeUint64(w writer, value uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	_, _ = w.Write(b[:])
}
func writeAnchor(w writer, a SourceAnchor) {
	_, _ = w.Write(a.ArtifactDigest[:])
	writeUint32(w, a.Occurrence.Point.Ordinal)
	_, _ = w.Write([]byte{byte(a.Occurrence.Point.Phase), byte(a.Occurrence.Kind)})
	writeUint32(w, a.Occurrence.Slot)
}
