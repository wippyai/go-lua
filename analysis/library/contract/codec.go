package contract

import (
	"bytes"
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// codecDomain separates this stream from every other framed stream in the
// analyzer. The declared codec format identity is written inside the stream as
// well: the domain says which codec wrote the bytes, and the format identity
// says which declared kind's codec it was, so an instance published under one
// kind cannot be decoded as another.
const codecDomain = "analysis/library/contract"

// Wire record markers. They separate the header from the member rows, so a
// member can never be read as the header.
const (
	recordHeader uint64 = iota + 1
	recordMember
	recordStep
	recordConstant
	recordAttachment
	recordCapability
)

// Decoding limits. A contract is authored data, not a network input, but the
// decoder is the boundary at which authored data becomes trusted, so it refuses
// implausible arities instead of reserving on a hostile count.
const (
	maxMembers = 1 << 16
	maxSteps   = 1 << 8
	maxKey     = 1 << 12
	maxBody    = 1 << 20
)

var (
	// ErrMalformed rejects a stream that is not a contract instance.
	ErrMalformed = errors.New("library/contract: malformed instance stream")
	// ErrUnknownKind rejects a stream whose declared kind is not in the sealed
	// library surface the decoder was handed.
	ErrUnknownKind = errors.New("library/contract: instance names an undeclared contract kind")
	// ErrRejected rejects a stream that decodes but does not satisfy the kind
	// it claims. It is the same law set New states; a decoded instance is not
	// admitted on weaker grounds than an authored one.
	ErrRejected = errors.New("library/contract: instance rejected by its declared kind")
)

// Encode writes one admitted instance. The stream is complete and
// self-delimiting: it carries its own domain, the version its kind declared,
// and the kind's declared codec format identity.
func Encode(instance *Instance) ([]byte, error) {
	if instance == nil {
		return nil, ErrMalformed
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, codecDomain, uint64(instance.codec.Version)); err != nil {
		return nil, err
	}
	if err := writeInstance(&writer, instance); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ContentID is the instance's content identity: the digest of exactly the bytes
// Encode writes. Two instances with one identity are one contract, so a member
// added, moved, or re-encoded changes it.
func ContentID(instance *Instance) (id identity.ContentID) {
	if instance == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, codecDomain, uint64(instance.codec.Version)) != nil ||
		writeInstance(&writer, instance) != nil || writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func writeInstance(writer *framing.Writer, instance *Instance) error {
	if err := writer.Record(recordHeader); err != nil {
		return err
	}
	if err := writer.Bytes(instance.codec.Format[:]); err != nil {
		return err
	}
	if err := writer.String(string(instance.kind)); err != nil {
		return err
	}
	if err := writer.String(instance.root); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(instance.members))); err != nil {
		return err
	}
	for _, member := range instance.members {
		if err := writeMember(writer, member); err != nil {
			return err
		}
	}
	return nil
}

func writeMember(writer *framing.Writer, member Member) error {
	if err := writer.Record(recordMember); err != nil {
		return err
	}
	if err := writer.Uint(uint64(member.Form)); err != nil {
		return err
	}
	if err := writer.Count(uint64(member.Path.Len())); err != nil {
		return err
	}
	for _, step := range member.Path.steps {
		if err := writer.Record(recordStep); err != nil {
			return err
		}
		if err := writer.Uint(uint64(step.Kind)); err != nil {
			return err
		}
		if err := writer.String(step.Key); err != nil {
			return err
		}
	}
	if err := writer.Bytes(member.Payload[:]); err != nil {
		return err
	}
	if err := writer.Uint(uint64(member.Encoding)); err != nil {
		return err
	}
	return writer.Bytes(member.Body)
}

// Decode reads one instance and admits it against the sealed kind it names. The
// table is the reader's authority: an instance is never trusted to describe the
// kind it is published under, only to name it.
func Decode(data []byte, table library.Table) (*Instance, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return nil, ErrMalformed
	}
	spec, version, err := readSpec(reader)
	if err != nil {
		return nil, err
	}
	kind, kindOK := resolveKind(table, spec.Kind)
	if !kindOK {
		return nil, ErrUnknownKind
	}
	// The stream version is the version the codec header carried; the format
	// identity is the one the header named. Both must be the kind's own, or the
	// bytes were written by a codec this reader is not.
	spec.Codec.Version = version
	instance, ok := New(spec, kind)
	if !ok {
		return nil, ErrRejected
	}
	return instance, nil
}

// readSpec reads the wire form without admitting it. Admission is New's, so
// there is one law set and the decoder cannot pass an instance the authoring
// path would refuse.
func readSpec(reader *framing.Reader) (Spec, uint32, error) {
	var spec Spec
	version, err := readHeaderVersion(reader)
	if err != nil {
		return Spec{}, 0, err
	}
	if record, err := reader.Record(); err != nil || record != recordHeader {
		return Spec{}, 0, ErrMalformed
	}
	format, err := reader.Bytes(len(spec.Codec.Format))
	if err != nil || len(format) != len(spec.Codec.Format) {
		return Spec{}, 0, ErrMalformed
	}
	copy(spec.Codec.Format[:], format)
	key, err := reader.String(maxKey)
	if err != nil {
		return Spec{}, 0, ErrMalformed
	}
	spec.Kind = schema.Key(key)
	root, err := reader.String(maxKey)
	if err != nil {
		return Spec{}, 0, ErrMalformed
	}
	spec.Root = root
	count, err := reader.Count()
	if err != nil || count > maxMembers {
		return Spec{}, 0, ErrMalformed
	}
	spec.Members = make([]Member, 0, count)
	for index := uint64(0); index < count; index++ {
		member, err := readMember(reader)
		if err != nil {
			return Spec{}, 0, err
		}
		spec.Members = append(spec.Members, member)
	}
	if err := reader.Finish(); err != nil {
		return Spec{}, 0, ErrMalformed
	}
	return spec, version, nil
}

// readHeaderVersion recovers the codec version the stream was written under.
// framing.Reader.Header checks a version the caller already knows; here the
// version is what the stream is being asked for, so the domain is checked by
// replaying the header against each admissible version until one matches.
func readHeaderVersion(reader *framing.Reader) (uint32, error) {
	for version := uint32(1); version <= maxCodecVersion; version++ {
		probe := *reader
		if probe.Header(codecDomain, uint64(version)) == nil {
			*reader = probe
			return version, nil
		}
	}
	return 0, ErrMalformed
}

// maxCodecVersion bounds the version probe. A kind declares its own version and
// the admission compares against it; this bound only keeps the probe finite.
const maxCodecVersion = 1 << 8

func readMember(reader *framing.Reader) (Member, error) {
	if record, err := reader.Record(); err != nil || record != recordMember {
		return Member{}, ErrMalformed
	}
	form, err := reader.Uint()
	if err != nil || form > uint64(^uint8(0)) {
		return Member{}, ErrMalformed
	}
	member := Member{Form: library.Form(form)}
	steps, err := reader.Count()
	if err != nil || steps > maxSteps {
		return Member{}, ErrMalformed
	}
	path := make([]Step, 0, steps)
	for index := uint64(0); index < steps; index++ {
		if record, err := reader.Record(); err != nil || record != recordStep {
			return Member{}, ErrMalformed
		}
		kind, err := reader.Uint()
		if err != nil || kind > uint64(^uint8(0)) {
			return Member{}, ErrMalformed
		}
		key, err := reader.String(maxKey)
		if err != nil {
			return Member{}, ErrMalformed
		}
		path = append(path, Step{Kind: StepKind(kind), Key: key})
	}
	member.Path = NewPath(path...)
	payload, err := reader.Bytes(len(member.Payload))
	if err != nil || len(payload) != len(member.Payload) {
		return Member{}, ErrMalformed
	}
	copy(member.Payload[:], payload)
	encoding, err := reader.Uint()
	if err != nil || encoding > uint64(^uint8(0)) {
		return Member{}, ErrMalformed
	}
	member.Encoding = Encoding(encoding)
	body, err := reader.Bytes(maxBody)
	if err != nil {
		return Member{}, ErrMalformed
	}
	member.Body = append([]byte(nil), body...)
	return member, nil
}

// resolveKind finds the declared kind one instance names. The table is the
// projection of the sealed library surface, so resolution is a table read and
// never a restatement of the catalog.
func resolveKind(table library.Table, key schema.Key) (*library.Entry, bool) {
	for position := 0; position < table.Count(); position++ {
		entry, ok := table.At(position)
		if !ok || entry == nil {
			return nil, false
		}
		if entry.Key() == key {
			return entry, true
		}
	}
	return nil, false
}
