package programschema

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// CallIdentityInput is the scalar input to the canonical authored-call
// equations.  The caller proves the fields against its exact Program/Flow/
// Static owners; this package only owns their common, framed codec.
//
// BodyPath, CallPath, ValuesWidth, ValuesOpen, and TypeArgumentTerms are
// optional formal/child inputs.  The root call and operand identities remain
// available without them, matching the former Program projections' failure
// boundaries.  Artifact construction supplies every field and therefore
// requires the complete result before publishing a row.
type CallIdentityInput struct {
	ProgramID identity.ContentID
	Call      keyspace.Term
	Form      CallForm
	Body      identity.ContentID
	Span      identity.ContentID
	Callee    keyspace.Term
	Receiver  keyspace.Term
	Actuals   keyspace.Term
	Values    identity.ContentID

	TypeArgumentCount   int
	TypeArguments       identity.ContentID
	BodyPath            identity.ContentID
	CallPath            identity.ContentID
	ValuesWidth         int
	ValuesOpen          bool
	TypeArgumentTerms   []keyspace.Term
	FormalGeometryKnown bool
}

// CallIdentitySet is the complete transient scalar result for one authored
// call.  Its fields are copied into Artifact or Link rows; no caller should
// retain this construction value as an owner or proof object.
type CallIdentitySet struct {
	Call           identity.ContentID
	Callee         identity.ContentID
	Receiver       identity.ContentID
	Actuals        identity.ContentID
	TypeArguments  identity.ContentID
	Formal         identity.ContentID
	TypeArgumentAt []identity.ContentID
}

// CallIdentities issues the exact call, operand, type-argument, and formal
// scalar equations formerly exposed through Program methods.  The hash
// domains, versions, field order, and framing are intentionally byte-for-
// byte compatible with those projections.
//
// The returned bool authenticates the root call equation only.  Formal and
// ordered type-argument IDs are independently available in the result so a
// caller that does not need those optional projections can preserve the old
// CallID failure boundary.
func CallIdentities(input CallIdentityInput) (CallIdentitySet, bool) {
	var result CallIdentitySet
	if !validCallRootInput(input) {
		return result, false
	}

	result.Callee, _ = callOperandIdentity(input.ProgramID, input.Call, input.Callee, callOperandRoleCallee)
	result.Actuals, _ = callOperandIdentity(input.ProgramID, input.Call, input.Actuals, callOperandRoleActuals)
	if input.Receiver != 0 {
		result.Receiver, _ = callOperandIdentity(input.ProgramID, input.Call, input.Receiver, callOperandRoleReceiver)
	}
	if !result.Callee.Available() || !result.Actuals.Available() || (input.Receiver != 0 && !result.Receiver.Available()) {
		return CallIdentitySet{}, false
	}

	result.TypeArguments = callTypeArgumentsIdentity(input.ProgramID, input.Call, input.TypeArgumentCount, input.TypeArguments)
	if !result.TypeArguments.Available() {
		return CallIdentitySet{}, false
	}
	result.Call = callRoleIdentity("program/transformer/call-occurrence", input.ProgramID, func(writer *framing.Writer) bool {
		return writeCallTerm(writer, input.Call) && writer.Uint(uint64(input.Form)) == nil &&
			writer.Bytes(input.Body[:]) == nil && writer.Bytes(input.Span[:]) == nil &&
			writer.Bytes(result.Callee[:]) == nil && writer.Bytes(result.Receiver[:]) == nil &&
			writer.Bytes(result.Actuals[:]) == nil && writer.Bytes(result.TypeArguments[:]) == nil &&
			writer.Bytes(input.Values[:]) == nil
	})
	if !result.Call.Available() {
		return CallIdentitySet{}, false
	}

	if input.FormalGeometryKnown {
		result.Formal = callFormalIdentity(input.BodyPath, input.CallPath, input.Form, input.ValuesWidth, input.ValuesOpen, input.TypeArgumentCount)
	}
	if len(input.TypeArgumentTerms) != 0 {
		result.TypeArgumentAt = make([]identity.ContentID, len(input.TypeArgumentTerms))
		for index, term := range input.TypeArgumentTerms {
			result.TypeArgumentAt[index] = callRoleIdentity("program/transformer/call-type-argument", input.ProgramID, func(writer *framing.Writer) bool {
				return writeCallTerm(writer, input.Call) && writer.Uint(uint64(index)) == nil && writeCallTerm(writer, term)
			})
		}
	}
	return result, true
}

const (
	callOperandRoleInvalid uint64 = iota
	callOperandRoleCallee
	callOperandRoleReceiver
	callOperandRoleActuals
)

func validCallRootInput(input CallIdentityInput) bool {
	return input.ProgramID.Available() && validCallTerm(input.Call) && input.Form.Valid() &&
		input.Body.Available() && input.Span.Available() && validCallTerm(input.Callee) &&
		(input.Receiver == 0 || validCallTerm(input.Receiver)) && validValuesTerm(input.Actuals) &&
		input.Values.Available() && input.TypeArgumentCount >= 0 && input.TypeArguments.Available()
}

func validCallTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0
}

func validValuesTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyValues && keyspace.TermOrdinal(term) != 0
}

func writeCallTerm(writer *framing.Writer, term keyspace.Term) bool {
	return writer != nil && validCallTerm(term) &&
		writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
}

func callOperandIdentity(owner identity.ContentID, call, term keyspace.Term, role uint64) (identity.ContentID, bool) {
	if !owner.Available() || !validCallTerm(call) || !validCallTerm(term) ||
		(role != callOperandRoleCallee && role != callOperandRoleReceiver && role != callOperandRoleActuals) {
		return identity.ContentID{}, false
	}
	id := callRoleIdentity("program/transformer/call-operand", owner, func(writer *framing.Writer) bool {
		return writer.Uint(role) == nil && writeCallTerm(writer, call) && writeCallTerm(writer, term)
	})
	return id, id.Available()
}

func callTypeArgumentsIdentity(owner identity.ContentID, call keyspace.Term, count int, sequence identity.ContentID) identity.ContentID {
	if count < 0 || !owner.Available() || !validCallTerm(call) || !sequence.Available() {
		return identity.ContentID{}
	}
	return callRoleIdentity("program/transformer/call-type-arguments", owner, func(writer *framing.Writer) bool {
		return writeCallTerm(writer, call) && writer.Count(uint64(count)) == nil && writer.Bytes(sequence[:]) == nil
	})
}

func callFormalIdentity(bodyPath, callPath identity.ContentID, form CallForm, width int, open bool, typeCount int) identity.ContentID {
	if !bodyPath.Available() || !callPath.Available() || !form.Valid() || width < 0 || typeCount < 0 ||
		uint64(width) > uint64(^uint32(0)) || uint64(typeCount) > uint64(^uint32(0)) {
		return identity.ContentID{}
	}
	roles := uint64(2)
	if form == CallFormMethod {
		roles = 3
	}
	return callSemanticIdentity("program/call-formal", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(callPath[:]) == nil && writer.Uint(1) == nil &&
			writer.Uint(uint64(form)) == nil && writer.Count(roles) == nil && writer.Uint(callOperandRoleCallee) == nil &&
			(form != CallFormMethod || writer.Uint(callOperandRoleReceiver) == nil) && writer.Uint(callOperandRoleActuals) == nil &&
			writer.Count(uint64(width)) == nil && writer.Bool(open) == nil && writer.Count(uint64(typeCount)) == nil
	})
}

func callSemanticIdentity(domain string, write func(*framing.Writer) bool) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func callRoleIdentity(domain string, owner identity.ContentID, write func(*framing.Writer) bool) identity.ContentID {
	if !owner.Available() || write == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(owner[:]) != nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
