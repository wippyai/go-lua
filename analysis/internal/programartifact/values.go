package programartifact

import (
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
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
	id      keyspace.ContentID
	kind    ValuesTailKind
	present bool
}

func (tail ValuesTail) Available() bool {
	if !tail.present {
		return !tail.id.Available() && tail.kind == ValuesTailInvalid
	}
	return tail.id.Available() && tail.kind.valid()
}

func (tail ValuesTail) Present() bool { return tail.Available() && tail.present }

func (tail ValuesTail) ID() keyspace.ContentID {
	if !tail.Available() || !tail.present {
		return keyspace.ContentID{}
	}
	return tail.id
}

func (tail ValuesTail) Kind() ValuesTailKind {
	if !tail.Available() || !tail.present {
		return ValuesTailInvalid
	}
	return tail.kind
}

// ValuesMember is one ordered, pointer-free semantic member row.
type ValuesMember struct{ id keyspace.ContentID }

func (member ValuesMember) Available() bool { return member.id.Available() }
func (member ValuesMember) ID() keyspace.ContentID {
	if !member.Available() {
		return keyspace.ContentID{}
	}
	return member.id
}

// ValuesRow is one immutable row in the exact Program Values denominator.
// Its body is an owner-neutral semantic BodyPathID; no raw Term or owner
// pointer crosses the artifact boundary.
type ValuesRow struct {
	id      keyspace.ContentID
	body    keyspace.ContentID
	members []ValuesMember
	tail    ValuesTail
	sealed  bool
}

func (row ValuesRow) Available() bool {
	return row.sealed && row.id.Available() && row.body.Available() && row.tail.Available()
}

func (row ValuesRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}

func (row ValuesRow) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body
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

func valuesTailKind(kind program.TailProducerKind) (ValuesTailKind, bool) {
	switch kind {
	case program.TailProducerCall:
		return ValuesTailCall, true
	case program.TailProducerVararg:
		return ValuesTailVararg, true
	default:
		return ValuesTailInvalid, false
	}
}

func (compiler *compiler) copyValuesFailure() CompileFailure {
	view := compiler.input.Values()
	if !view.Available() {
		return compileFailure(CompileStageValues, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	count := view.Count()
	if count < 0 {
		return compileFailure(CompileStageValues, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	rows := make([]ValuesRow, count)
	seenRows := make(map[keyspace.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		occurrence, ok := view.At(index)
		if !ok || !occurrence.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if !compiler.input.OwnsValuesOccurrence(occurrence) {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesForeign)
		}
		rowID, bodyPath := occurrence.ID(), occurrence.BodyPathID()
		if !rowID.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesIdentity)
		}
		if !bodyPath.Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesBody)
		}
		if _, duplicate := seenRows[rowID]; duplicate {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesDuplicate)
		}
		seenRows[rowID] = struct{}{}

		members := make([]ValuesMember, occurrence.Count())
		seenMembers := make(map[keyspace.ContentID]struct{}, len(members))
		for memberIndex := range members {
			member, memberOK := occurrence.At(memberIndex)
			parent, parentOK := member.Values()
			position, positionOK := member.Position()
			memberID := member.ID()
			if !memberOK || !member.Available() || !parentOK || !parent.Available() ||
				!compiler.input.OwnsValuesMember(member) || !compiler.input.OwnsValuesOccurrence(parent) ||
				parent.ID() != rowID || !positionOK || position != memberIndex || !memberID.Available() {
				return compileFailure(CompileStageValues, CompileRowValues, index, memberIndex, CompileReasonValuesMember)
			}
			if _, duplicate := seenMembers[memberID]; duplicate {
				return compileFailure(CompileStageValues, CompileRowValues, index, memberIndex, CompileReasonValuesDuplicate)
			}
			seenMembers[memberID] = struct{}{}
			members[memberIndex] = ValuesMember{id: memberID}
		}

		tail := ValuesTail{}
		if producer, present := occurrence.Tail(); present {
			if !compiler.input.OwnsTailProducer(producer) {
				return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesForeign)
			}
			kind, kindOK := valuesTailKind(producer.Kind())
			producerID := producer.ContextID()
			if !kindOK || !producerID.Available() {
				return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesTail)
			}
			tail = ValuesTail{id: producerID, kind: kind, present: true}
		}
		rows[index] = ValuesRow{id: rowID, body: bodyPath, members: members, tail: tail, sealed: true}
		if !rows[index].Available() {
			return compileFailure(CompileStageValues, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
	}
	compiler.values = rows
	return CompileFailure{}
}
