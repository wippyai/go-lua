package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// ValuesTailKind is the artifact-owned closed classification of an open
// Values tail. It deliberately does not retain Program's proof type.
type ValuesTailKind uint8

const (
	ValuesTailInvalid ValuesTailKind = iota
	ValuesTailCall
	ValuesTailVararg
)

func (kind ValuesTailKind) valid() bool {
	return kind == ValuesTailCall || kind == ValuesTailVararg
}

// ValuesTail is a scalar copy of one exact Program TailProducer. A closed
// Values row has no tail proof; an open row has both a semantic producer ID
// and its closed kind.
type ValuesTail struct {
	id      identity.ContentID
	span    identity.ContentID
	kind    ValuesTailKind
	present bool
}

func (tail ValuesTail) Available() bool {
	if !tail.present {
		return !tail.id.Available() && !tail.span.Available() && tail.kind == ValuesTailInvalid
	}
	return tail.id.Available() && tail.span.Available() && tail.kind.valid()
}

func (tail ValuesTail) Present() bool { return tail.Available() && tail.present }

func (tail ValuesTail) ID() identity.ContentID {
	if !tail.Available() || !tail.present {
		return identity.ContentID{}
	}
	return tail.id
}

// SpanID returns the exact Flow evaluation-span identity of this open tail.
// Closed Values rows intentionally have no tail span.
func (tail ValuesTail) SpanID() identity.ContentID {
	if !tail.Available() || !tail.present {
		return identity.ContentID{}
	}
	return tail.span
}

func (tail ValuesTail) Kind() ValuesTailKind {
	if !tail.Available() || !tail.present {
		return ValuesTailInvalid
	}
	return tail.kind
}

// ValuesMember is one ordered, pointer-free semantic member row.
type ValuesMember struct{ id identity.ContentID }

func (member ValuesMember) Available() bool { return member.id.Available() }
func (member ValuesMember) ID() identity.ContentID {
	if !member.Available() {
		return identity.ContentID{}
	}
	return member.id
}

// ValuesRow is one immutable row in the exact Program Values denominator.
// Its body is an owner-neutral semantic BodyPathID; no raw Term or owner
// pointer crosses the artifact boundary.
type ValuesRow struct {
	id      identity.ContentID
	body    identity.ContentID
	span    identity.ContentID
	members []ValuesMember
	tail    ValuesTail
	sealed  bool
}

func (row ValuesRow) Available() bool {
	return row.sealed && row.id.Available() && row.body.Available() && row.tail.Available()
}

func (row ValuesRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row ValuesRow) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

// RootSpanID returns the exact Flow span identity for this Values root when
// Flow publishes one. Values rows used only as static/source payloads may be
// spanless; callers must distinguish that from an unavailable row.
func (row ValuesRow) RootSpanID() (identity.ContentID, bool) {
	return row.span, row.Available() && row.span.Available()
}

func (row ValuesRow) MemberCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.members)
}

func (row ValuesRow) MemberAt(index int) (ValuesMember, bool) {
	if !row.Available() || index < 0 || index >= len(row.members) {
		return ValuesMember{}, false
	}
	return row.members[index], true
}

func (row ValuesRow) Tail() (ValuesTail, bool) {
	if !row.Available() || !row.tail.present {
		return ValuesTail{}, false
	}
	return row.tail, true
}

func valuesTailKind(family keyspace.Family) (ValuesTailKind, bool) {
	switch family {
	case keyspace.FamilyCall:
		return ValuesTailCall, true
	case keyspace.FamilyVararg:
		return ValuesTailVararg, true
	default:
		return ValuesTailInvalid, false
	}
}

func (compiler *compiler) copyValuesFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageValues, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	view := compiler.input.Flow().Authored().Values()
	count := view.Count()
	if count < 0 {
		return compileFailure(CompileStageValues, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	rows := make([]ValuesRow, count)
	seenRows := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		term, ok := view.At(index)
		owner, tailTerm, rowOK := view.Get(term)
		width, widthOK := view.Len(term)
		if !ok || !rowOK || !widthOK || width < 0 {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		body, bodyOK := compiler.input.Body(owner)
		bodyPathID, bodyPathOK := compiler.input.Flow().BodyPath(owner)
		if !bodyOK || !body.Available() || !bodyPathOK {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesBody)
		}
		rootSpanID, _, _, rootSpanOK := compiler.input.EvaluationSpan(term)
		if !rootSpanOK {
			rootSpanID = identity.ContentID{}
		} else if !rootSpanID.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		var rowID identity.ContentID
		members := make([]ValuesMember, width)
		seenMembers := make(map[identity.ContentID]struct{}, width)
		for memberIndex := range members {
			_, memberOK := view.Member(term, memberIndex)
			memberID, memberIDOK := compiler.input.Flow().ValuesMemberID(term, memberIndex)
			if !memberOK || !memberIDOK || !memberID.Available() {
				return compileFailure(CompileStageValues, CompileRowValues, index, memberIndex, CompileReasonValuesMember)
			}
			if _, duplicate := seenMembers[memberID]; duplicate {
				return compileFailure(CompileStageValues, CompileRowValues, index, memberIndex, CompileReasonValuesDuplicate)
			}
			seenMembers[memberID] = struct{}{}
			members[memberIndex] = ValuesMember{id: memberID}
		}
		tail := ValuesTail{}
		var tailKind ValuesTailKind
		if tailTerm != 0 {
			tailFamily := keyspace.TermFamily(tailTerm)
			var tailOK bool
			tailKind, tailOK = valuesTailKind(tailFamily)
			tailID, tailIDOK := compiler.input.Flow().ValuesTailID(term)
			tailSpanID, _, _, tailSpanOK := compiler.input.EvaluationSpan(tailTerm)
			if !tailOK || !tailIDOK || !tailSpanOK || !tailID.Available() || !tailSpanID.Available() {
				return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesTail)
			}
			tail = ValuesTail{id: tailID, span: tailSpanID, kind: tailKind, present: true}
		}
		var rowIDOK bool
		rowID, rowIDOK = compiler.input.Flow().ValuesOccurrenceID(term)
		if !rowIDOK || !rowID.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesIdentity)
		}
		if !bodyPathID.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesBody)
		}
		if _, duplicate := seenRows[rowID]; duplicate {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesDuplicate)
		}
		seenRows[rowID] = struct{}{}

		rows[index] = ValuesRow{id: rowID, body: bodyPathID, span: rootSpanID, members: members, tail: tail, sealed: true}
		if !rows[index].Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
	}
	compiler.values = rows
	return CompileFailure{}
}

// valueRowForTerm resolves an existing authored Values term to the row already
// copied into this artifact transaction. The ordinal is the canonical Flow
// Values order; no Program Values catalog or second occurrence handle is
// retained by the body/outcome compiler.
func (compiler *compiler) valueRowForTerm(term keyspace.Term) (ValuesRow, bool) {
	if compiler == nil || keyspace.TermFamily(term) != keyspace.FamilyValues {
		return ValuesRow{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) > uint64(len(compiler.values)) {
		return ValuesRow{}, false
	}
	index := int(ordinal) - 1
	authoredTerm, termOK := compiler.input.Flow().Authored().Values().At(index)
	row := compiler.values[index]
	return row, termOK && authoredTerm == term && row.Available()
}
