package program

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	programstatic "github.com/wippyai/go-lua/program/static"
)

const callFormalVersion = 1

// StaticTypeReferenceID is the owner-issued, term-free identity used when a
// Static type capability crosses the reusable ProgramArtifact boundary.  The
// authored Term is consumed only while the exact Program proof is live; users
// of this identity cannot reconstruct it.
func StaticTypeReferenceID(owner keyspace.ContentID, ref programstatic.StaticTypeRef) (id keyspace.ContentID, ok bool) {
	if !owner.Available() || ref.Term() == 0 {
		return keyspace.ContentID{}, false
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

// CallFormal is the owner-neutral semantic occurrence of one authored call.
// Its identity contains only the lexical Body path, the call's local position,
// and its closed operand geometry. Program/Static IDs and raw Terms are absent.
// A live CallOccurrence is required separately to mount this formal.
type CallFormal struct {
	id         keyspace.ContentID
	body       keyspace.ContentID
	coordinate keyspace.ContentID
	position   uint32
	width      uint32
	typeCount  uint32
	form       flow.CallForm
	open       bool
}

func (formal CallFormal) Available() bool {
	return formal.id.Available() && formal.body.Available() && formal.coordinate.Available() && formal.position != 0 &&
		(formal.form == flow.CallFormPlain || formal.form == flow.CallFormMethod)
}

func (formal CallFormal) ID() keyspace.ContentID {
	if !formal.Available() {
		return keyspace.ContentID{}
	}
	return formal.id
}

func (formal CallFormal) Equal(other CallFormal) bool {
	return formal.Available() && other.Available() && formal == other
}

// Formal derives the call's reusable semantic occurrence after validating the
// exact Program-owned call proof. This is a cold proof constructor; mounted
// consumers retain its scalar result instead of repeating the local census.
func (call CallOccurrence) Formal() (CallFormal, bool) {
	if !call.Available() || !call.input.OwnsCallOccurrence(call) {
		return CallFormal{}, false
	}
	bodyPath, bodyOK := semanticBodyPath(call.input, call.body)
	positionID, positionOK := call.input.owner.Flow().CallPath(call.call)
	values, valuesOK := call.Values()
	types, typesOK := call.TypeArguments()
	if !bodyOK || !positionOK || !valuesOK || !typesOK || values.Count() < 0 || types.Count() < 0 ||
		uint64(values.Count()) > uint64(^uint32(0)) || uint64(types.Count()) > uint64(^uint32(0)) {
		return CallFormal{}, false
	}
	_, open := values.Tail()
	formal := CallFormal{
		body: bodyPath, coordinate: positionID, position: 1, width: uint32(values.Count()), typeCount: uint32(types.Count()),
		form: call.form, open: open,
	}
	formal.id = callFormalID(formal)
	return formal, formal.Available()
}

// OwnsCallFormal binds an owner-neutral formal back to this exact Program call
// proof. Equivalent replay formals are accepted only through a current exact
// occurrence; an occurrence from another Program owner is rejected.
func (input TransformerInput) OwnsCallFormal(formal CallFormal, call CallOccurrence) bool {
	if !input.Available() || !formal.Available() || !input.OwnsCallOccurrence(call) {
		return false
	}
	expected, ok := call.Formal()
	return ok && formal.Equal(expected)
}

// StaticType returns the exact owner-bound authored Static type capability for
// this call type argument. The underlying Term stays inside Static's existing
// capability surface and is not part of CallFormal identity.
func (argument CallTypeArgument) StaticType() (programstatic.StaticTypeRef, bool) {
	if !argument.Available() {
		return programstatic.StaticTypeRef{}, false
	}
	ref, ok := argument.input.owner.Static().StaticTypes().Ref(argument.term)
	return ref, ok
}

// ProgramID is the exact owner fence used by semantic type authorities. It is
// never written into an owner-neutral call or type-formal descriptor.
func (argument CallTypeArgument) ProgramID() keyspace.ContentID {
	if !argument.Available() {
		return keyspace.ContentID{}
	}
	return argument.input.programID
}

// StaticTypeReferenceID returns the detached identity of this exact call
// formal's authored static type.  It does not expose the underlying Term.
func (argument CallTypeArgument) StaticTypeReferenceID() (keyspace.ContentID, bool) {
	if !argument.Available() {
		return keyspace.ContentID{}, false
	}
	ref, ok := argument.StaticType()
	if !ok {
		return keyspace.ContentID{}, false
	}
	return StaticTypeReferenceID(argument.ProgramID(), ref)
}

func callFormalID(formal CallFormal) keyspace.ContentID {
	if !formal.body.Available() || !formal.coordinate.Available() || formal.position == 0 ||
		(formal.form != flow.CallFormPlain && formal.form != flow.CallFormMethod) {
		return keyspace.ContentID{}
	}
	hash := sha256.New()
	var writer canonical.Writer
	roles := uint64(2)
	if formal.form == flow.CallFormMethod {
		roles = 3
	}
	if writer.Reset(hash, "program/call-formal", callFormalVersion) != nil || writer.Record(1) != nil ||
		writer.Bytes(formal.body[:]) != nil || writer.Bytes(formal.coordinate[:]) != nil || writer.Uint(uint64(formal.position)) != nil || writer.Uint(uint64(formal.form)) != nil ||
		writer.Count(roles) != nil || writer.Uint(uint64(CallOperandCallee)) != nil ||
		(formal.form == flow.CallFormMethod && writer.Uint(uint64(CallOperandReceiver)) != nil) ||
		writer.Uint(uint64(CallOperandActuals)) != nil || writer.Count(uint64(formal.width)) != nil ||
		writer.Bool(formal.open) != nil || writer.Count(uint64(formal.typeCount)) != nil || writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
