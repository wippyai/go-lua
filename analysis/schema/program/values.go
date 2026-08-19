package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// ValuesTailKind is the neutral copy of the two open-tail producer ordinals.
// A closed Values row uses ValuesTailInvalid and no tail identities.
type ValuesTailKind uint8

const (
	ValuesTailInvalid ValuesTailKind = iota
	ValuesTailCall
	ValuesTailVararg
)

func (kind ValuesTailKind) valid() bool {
	return kind == ValuesTailCall || kind == ValuesTailVararg
}

// ValuesTail is the scalar tail relation formerly nested in a Program-owned
// Values row. Span is retained because it is part of the canonical row's
// proof, even though the ingress projection historically omitted it.
type ValuesTail struct {
	id      identity.ContentID
	span    identity.ContentID
	kind    ValuesTailKind
	present bool
}

// NewValuesTail copies one canonical tail row. A closed tail is represented by
// unavailable identities, ValuesTailInvalid and present=false.
func NewValuesTail(id, span identity.ContentID, kind ValuesTailKind, present bool) (ValuesTail, bool) {
	tail := ValuesTail{id: id, span: span, kind: kind, present: present}
	return tail, tail.Available()
}

func (tail ValuesTail) Available() bool {
	if !tail.present {
		return !tail.id.Available() && !tail.span.Available() && tail.kind == ValuesTailInvalid
	}
	return tail.id.Available() && tail.span.Available() && tail.kind.valid()
}

func (tail ValuesTail) Present() bool { return tail.Available() && tail.present }

func (tail ValuesTail) ID() identity.ContentID {
	if !tail.Present() {
		return identity.ContentID{}
	}
	return tail.id
}

func (tail ValuesTail) SpanID() identity.ContentID {
	if !tail.Present() {
		return identity.ContentID{}
	}
	return tail.span
}

func (tail ValuesTail) Kind() ValuesTailKind {
	if !tail.Present() {
		return ValuesTailInvalid
	}
	return tail.kind
}

// ValuesMember is one ordered, pointer-free semantic member row. Its position
// is its ordinal in ValuesMemberFamily; no position is duplicated in the row
// and no slice header is retained.
type ValuesMember struct{ id identity.ContentID }

func NewValuesMember(id identity.ContentID) (ValuesMember, bool) {
	member := ValuesMember{id: id}
	return member, member.Available()
}

func (member ValuesMember) Available() bool { return member.id.Available() }

func (member ValuesMember) ID() identity.ContentID {
	if !member.Available() {
		return identity.ContentID{}
	}
	return member.id
}

// Values is one immutable row in the Program Values denominator. Members live
// in ValuesMemberFamily and are named by [memberOffset, memberOffset+memberCount);
// the range is physical publication geometry, not a new content identity
// field. The old Artifact identity still hashes the member rows in this same
// order when the family is moved into Frozen.
type Values struct {
	id           identity.ContentID
	body         identity.ContentID
	span         identity.ContentID
	memberOffset uint32
	memberCount  uint32
	tail         ValuesTail
}

// NewValues copies the canonical Values row and replaces its nested member
// slice with a dense span in ValuesMemberFamily. The root span is optional
// exactly as it is in the Program artifact row.
func NewValues(id, body, span identity.ContentID, memberOffset, memberCount uint32, tail ValuesTail) (Values, bool) {
	row := Values{id: id, body: body, span: span, memberOffset: memberOffset, memberCount: memberCount, tail: tail}
	return row, row.Available()
}

func (row Values) Available() bool {
	return row.id.Available() && row.body.Available() && row.tail.Available() &&
		uint64(row.memberOffset)+uint64(row.memberCount) <= uint64(^uint32(0))
}

func (row Values) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Values) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row Values) RootSpanID() (identity.ContentID, bool) {
	return row.span, row.Available() && row.span.Available()
}

func (row Values) MemberOffset() (uint32, bool) {
	return row.memberOffset, row.Available()
}

func (row Values) MemberCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.memberCount)
}

func (row Values) MemberSpan() (offset, count uint32, ok bool) {
	return row.memberOffset, row.memberCount, row.Available()
}

func (row Values) Tail() (ValuesTail, bool) {
	if !row.Available() || !row.tail.Present() {
		return ValuesTail{}, false
	}
	return row.tail, true
}
