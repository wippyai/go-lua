package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// These versions are part of the pinned Artifact identity preimage. They are
// owned by the Program publication because the publication owns the call
// rows, their ordered child planes, and their result geometry.
const (
	CallRowsLawVersion       uint64 = 2
	CallResultRowsLawVersion uint64 = 4
)

// WriteCallIdentityFields replays the canonical call portion of the Artifact
// identity preimage from the sealed Program publication. The publication
// owns both the field order and the row/span validation; the caller supplies
// only a primitive writer for the surrounding identity domain.
//
// The order and scalar shapes intentionally match the historical Artifact
// walk byte-for-byte. In particular, optional receiver/target/tail identities
// retain their presence booleans and zero payloads, and an open CallResult
// still emits its zero count field.
func (row Program) WriteCallIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	callCount, callsPublished := CallFamily().Count(&row.Frozen, catalog)
	operandCount, operandsPublished := CallOperandFamily().Count(&row.Frozen, catalog)
	argumentCount, argumentsPublished := CallArgumentFamily().Count(&row.Frozen, catalog)
	typeArgumentCount, typeArgumentsPublished := CallTypeArgumentFamily().Count(&row.Frozen, catalog)
	if !callsPublished || !operandsPublished || !argumentsPublished || !typeArgumentsPublished {
		return false
	}
	if !writer.WriteUint(CallRowsLawVersion) || !writer.WriteUint(uint64(callCount)) {
		return false
	}
	for index := 0; index < callCount; index++ {
		call, held := CallFamily().At(&row.Frozen, catalog, index)
		operandStart, operandWidth, operandSpanOK := call.OperandSpan()
		argumentStart, argumentWidth, argumentSpanOK := call.ArgumentSpan()
		typeArgumentStart, typeArgumentWidth, typeArgumentSpanOK := call.TypeArgumentSpan()
		if !held || !call.Available() || !operandSpanOK || !argumentSpanOK || !typeArgumentSpanOK ||
			uint64(operandStart)+uint64(operandWidth) > uint64(operandCount) ||
			uint64(argumentStart)+uint64(argumentWidth) > uint64(argumentCount) ||
			uint64(typeArgumentStart)+uint64(typeArgumentWidth) > uint64(typeArgumentCount) {
			return false
		}
		receiver, hasReceiver := call.ReceiverID()
		tail, hasTail := call.TailID()
		target, _ := call.DirectTargetBody()
		if !writer.WriteContentID(call.ID()) || !writer.WriteContentID(call.BodyID()) ||
			!writer.WriteContentID(call.SpanID()) || !writer.WriteContentID(call.FormalID()) ||
			!writer.WriteContentID(call.ValuesID()) || !writer.WriteContentID(call.ValuesRootID()) ||
			!writer.WriteContentID(call.TypeArgumentsID()) || !writer.WriteContentID(call.CalleeID()) ||
			!writer.WriteContentID(call.ActualsID()) || !writer.WriteContentID(target) ||
			!writer.WriteUint(uint64(call.Form())) || !writer.WriteBool(hasReceiver) ||
			!writer.WriteContentID(receiver) || !writer.WriteBool(hasTail) ||
			!writer.WriteContentID(tail) || !writer.WriteUint(uint64(operandWidth)) ||
			!writer.WriteUint(uint64(argumentWidth)) || !writer.WriteUint(uint64(typeArgumentWidth)) {
			return false
		}
		for childIndex := uint32(0); childIndex < operandWidth; childIndex++ {
			operand, childHeld := CallOperandFamily().At(&row.Frozen, catalog, int(operandStart+childIndex))
			if !childHeld || !operand.Available() {
				return false
			}
			if !writer.WriteContentID(operand.ID()) || !writer.WriteContentID(operand.CallID()) ||
				!writer.WriteContentID(operand.ValueID()) || !writer.WriteContentID(operand.SpanID()) ||
				!writer.WriteUint(uint64(operand.Kind())) {
				return false
			}
		}
		for childIndex := uint32(0); childIndex < argumentWidth; childIndex++ {
			argument, childHeld := CallArgumentFamily().At(&row.Frozen, catalog, int(argumentStart+childIndex))
			if !childHeld || !argument.Available() {
				return false
			}
			if !writer.WriteContentID(argument.ID()) || !writer.WriteContentID(argument.CallID()) ||
				!writer.WriteContentID(argument.ValuesID()) || !writer.WriteContentID(argument.MemberID()) ||
				!writer.WriteContentID(argument.SpanID()) || !writer.WriteUint(uint64(argument.Index())) {
				return false
			}
		}
		for childIndex := uint32(0); childIndex < typeArgumentWidth; childIndex++ {
			argument, childHeld := CallTypeArgumentFamily().At(&row.Frozen, catalog, int(typeArgumentStart+childIndex))
			if !childHeld || !argument.Available() {
				return false
			}
			if !writer.WriteContentID(argument.ID()) || !writer.WriteContentID(argument.CallID()) ||
				!writer.WriteContentID(argument.TypesID()) || !writer.WriteContentID(argument.ReferenceID()) ||
				!writer.WriteUint(uint64(argument.Index())) {
				return false
			}
		}
	}

	callResultCount, callResultsPublished := CallResultFamily().Count(&row.Frozen, catalog)
	if !callResultsPublished || !writer.WriteUint(CallResultRowsLawVersion) || !writer.WriteUint(uint64(callResultCount)) {
		return false
	}
	for index := 0; index < callResultCount; index++ {
		result, held := CallResultFamily().At(&row.Frozen, catalog, index)
		value, hasValue := result.ValueID()
		tail, hasTail := result.ValuesTailID()
		position, hasPosition := result.Position()
		multiplicity := result.Multiplicity()
		count, hasCount := result.ResultCount()
		open, openOK := result.ResultsOpen()
		slotOffset, slotWidth, slotSpanOK := result.SlotSpan()
		if !held || !result.Available() || !slotSpanOK || hasValue == hasTail || hasPosition != (result.Form() == CallResultValue) ||
			!multiplicity.Valid() || !openOK || open != (multiplicity == CallResultMultiplicityOpen) ||
			hasCount != (multiplicity == CallResultMultiplicityExact) {
			return false
		}
		if !writer.WriteContentID(result.CallID()) || !writer.WriteContentID(result.ValuesID()) ||
			!writer.WriteUint(uint64(result.Form())) || !writer.WriteUint(uint64(multiplicity)) ||
			!writer.WriteUint(uint64(count)) || !writer.WriteUint(uint64(slotOffset)) || !writer.WriteUint(uint64(slotWidth)) ||
			!writer.WriteBool(open) || !writer.WriteBool(hasValue) ||
			!writer.WriteContentID(value) || !writer.WriteBool(hasTail) || !writer.WriteContentID(tail) ||
			!writer.WriteBool(hasPosition) || !writer.WriteUint(uint64(position)) {
			return false
		}
	}
	return row.WriteCallResultSlotIdentityFields(writer)
}
