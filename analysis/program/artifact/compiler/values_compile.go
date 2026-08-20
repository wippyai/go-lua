package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// copyValuesFailure copies the authored Values denominator directly into the
// sole compiler transaction.
func (compiler *compiler) copyValuesFailure() CompileFailure {
	input := compiler.input
	if input == nil || !input.Available() {
		return compileFailure(CompileStageValues, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	view := input.Flow().Authored().Values()
	count := view.Count()
	if count < 0 {
		return compileFailure(CompileStageValues, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	values := make([]programschema.Values, count)
	members := make([]programschema.ValuesMember, 0)
	seenRows := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		term, termOK := view.At(index)
		owner, tailTerm, relationOK := view.Get(term)
		width, widthOK := view.Len(term)
		if !termOK || !relationOK || !widthOK || width < 0 {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		body, bodyOK := input.Body(owner)
		bodyPathID, bodyPathOK := input.Flow().BodyPath(owner)
		if !bodyOK || !body.Available() || !bodyPathOK || !bodyPathID.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesBody)
		}
		rootSpanID, _, _, rootSpanOK := input.EvaluationSpan(term)
		if !rootSpanOK {
			rootSpanID = identity.ContentID{}
		} else if !rootSpanID.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}

		rowID, rowIDOK := input.Flow().ValuesOccurrenceID(term)
		if !rowIDOK || !rowID.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesIdentity)
		}
		if _, duplicate := seenRows[rowID]; duplicate {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesDuplicate)
		}
		seenRows[rowID] = struct{}{}
		if !valuesFitsUint32(len(members)) || !valuesFitsUint32(width) || uint64(len(members))+uint64(width) > uint64(^uint32(0)) {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		memberOffset := uint32(len(members))
		seenMembers := make(map[identity.ContentID]struct{}, width)
		for memberIndex := 0; memberIndex < width; memberIndex++ {
			_, memberOK := view.Member(term, memberIndex)
			memberID, memberIDOK := input.Flow().ValuesMemberID(term, memberIndex)
			if !memberOK || !memberIDOK || !memberID.Available() {
				return compileFailure(CompileStageValues, CompileRowValues, index, memberIndex, CompileReasonValuesMember)
			}
			if _, duplicate := seenMembers[memberID]; duplicate {
				return compileFailure(CompileStageValues, CompileRowValues, index, memberIndex, CompileReasonValuesDuplicate)
			}
			seenMembers[memberID] = struct{}{}
			member, memberSealed := programschema.NewValuesMember(memberID)
			if !memberSealed {
				return compileFailure(CompileStageValues, CompileRowValues, index, memberIndex, CompileReasonValuesMember)
			}
			members = append(members, member)
		}

		tail := programschema.ValuesTail{}
		if tailTerm != 0 {
			kind, kindOK := valuesTailKind(keyspace.TermFamily(tailTerm))
			tailID, tailIDOK := input.Flow().ValuesTailID(term)
			tailSpanID, _, _, tailSpanOK := input.EvaluationSpan(tailTerm)
			if !kindOK || !tailIDOK || !tailSpanOK || !tailID.Available() || !tailSpanID.Available() {
				return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesTail)
			}
			var tailSealed bool
			tail, tailSealed = programschema.NewValuesTail(tailID, tailSpanID, kind, true)
			if !tailSealed {
				return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesTail)
			}
		} else {
			var tailOK bool
			tail, tailOK = programschema.NewValuesTail(identity.ContentID{}, identity.ContentID{}, programschema.ValuesTailInvalid, false)
			if !tailOK {
				return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesTail)
			}
		}
		row, rowSealed := programschema.NewValues(rowID, bodyPathID, rootSpanID, memberOffset, uint32(width), tail)
		if !rowSealed {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		values[index] = row
	}
	compiler.values, compiler.valuesMembers = values, members
	return CompileFailure{}
}

func valuesTailKind(family keyspace.Family) (programschema.ValuesTailKind, bool) {
	switch family {
	case keyspace.FamilyCall:
		return programschema.ValuesTailCall, true
	case keyspace.FamilyVararg:
		return programschema.ValuesTailVararg, true
	default:
		return programschema.ValuesTailInvalid, false
	}
}

func valuesFitsUint32(value int) bool {
	return value >= 0 && uint64(value) <= uint64(^uint32(0))
}
