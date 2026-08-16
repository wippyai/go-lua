package imports

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// ReturnCount reports retained chunk-activation Returns.
func (e EntryView) ReturnCount() int {
	if e.data == nil {
		return 0
	}
	return len(e.data.returnTerms)
}

// ReturnAt returns one retained Return in stable source order.
func (e EntryView) ReturnAt(index int) (keyspace.Term, bool) {
	if e.data == nil || index < 0 || index >= len(e.data.returnTerms) {
		return 0, false
	}
	return e.data.returnTerms[index], true
}

// RootCount reports the original fixed value-slot width for one Return.
func (e EntryView) RootCount(returned keyspace.Term) (int, bool) {
	_, roots, _, ok := e.entryReturn(returned)
	if !ok {
		return 0, false
	}
	return int(roots.End - roots.Start), true
}

// RootFunction reports the exact Function at one original Return value
// ordinal. A non-Function slot is absent rather than shifted.
func (e EntryView) RootFunction(returned keyspace.Term, index int) (keyspace.Term, bool) {
	_, roots, _, ok := e.entryReturn(returned)
	if !ok {
		return 0, false
	}
	at, ok := entryRangeIndex(roots, index)
	if !ok {
		return 0, false
	}
	root := e.data.roots[at]
	return root, root != 0
}

// RootCell reports the exact Cell directly read at one Return value ordinal.
func (e EntryView) RootCell(returned keyspace.Term, index int) (keyspace.Term, bool) {
	_, roots, _, ok := e.entryReturn(returned)
	if !ok {
		return 0, false
	}
	at, ok := entryRangeIndex(roots, index)
	if !ok {
		return 0, false
	}
	cell := e.data.rootCells[at]
	return cell, cell != 0
}

// MemberCount reports retained exact named table members for one Return.
func (e EntryView) MemberCount(returned keyspace.Term) (int, bool) {
	_, _, members, ok := e.entryReturn(returned)
	if !ok {
		return 0, false
	}
	return int(members.End - members.Start), true
}

// MemberAt returns one retained TableField in stable source order.
func (e EntryView) MemberAt(returned keyspace.Term, index int) (keyspace.Term, bool) {
	_, _, members, ok := e.entryReturn(returned)
	if !ok {
		return 0, false
	}
	at, ok := entryRangeIndex(members, index)
	if !ok {
		return 0, false
	}
	return e.data.members[at].Field, true
}

// MemberParent reports one retained table-surface path step.
func (e EntryView) MemberParent(field keyspace.Term) (parent keyspace.Term, suffix keyspace.Key, ok bool) {
	member, ok := e.member(field)
	if !ok {
		return 0, 0, false
	}
	return member.Parent, member.Suffix, true
}

// MemberFunction reports the exact Function at a retained final leaf member.
func (e EntryView) MemberFunction(field keyspace.Term) (keyspace.Term, bool) {
	member, ok := e.member(field)
	if !ok || member.Value == 0 {
		return 0, false
	}
	return member.Value, true
}

// MemberOrigin reports the Return, original value ordinal, and Table root.
func (e EntryView) MemberOrigin(field keyspace.Term) (keyspace.Term, int, keyspace.Term, bool) {
	member, ok := e.member(field)
	if !ok {
		return 0, 0, 0, false
	}
	return member.Returned, int(member.Ordinal), member.Table, true
}

// MemberTotal reports all retained Entry table-surface rows.
func (e EntryView) MemberTotal() int {
	if e.data == nil {
		return 0
	}
	return len(e.data.members)
}

func (e EntryView) entryReturn(returned keyspace.Term) (uint32, EntryRange, EntryRange, bool) {
	if e.data == nil {
		return 0, EntryRange{}, EntryRange{}, false
	}
	ordinal := keyspace.TermOrdinal(returned)
	if keyspace.TermFamily(returned) != keyspace.FamilyReturn || ordinal == 0 ||
		uint64(ordinal) >= uint64(len(e.data.returnIndex)) {
		return 0, EntryRange{}, EntryRange{}, false
	}
	row := e.data.returnIndex[ordinal]
	if row == 0 || uint64(row) > uint64(len(e.data.returnTerms)) || e.data.returnTerms[row-1] != returned {
		return 0, EntryRange{}, EntryRange{}, false
	}
	return ordinal, e.data.rootRanges[ordinal], e.data.memberRanges[ordinal], true
}

func (e EntryView) member(field keyspace.Term) (EntryMember, bool) {
	if e.data == nil {
		return EntryMember{}, false
	}
	ordinal := keyspace.TermOrdinal(field)
	if keyspace.TermFamily(field) != keyspace.FamilyTableField || ordinal == 0 ||
		uint64(ordinal) >= uint64(len(e.data.memberIndex)) {
		return EntryMember{}, false
	}
	row := e.data.memberIndex[ordinal]
	if row == 0 || uint64(row) > uint64(len(e.data.members)) {
		return EntryMember{}, false
	}
	member := e.data.members[row-1]
	return member, member.Field == field
}

func entryRangeIndex(r EntryRange, index int) (int, bool) {
	if index < 0 || uint64(index) >= uint64(r.End-r.Start) {
		return 0, false
	}
	return int(r.Start) + index, true
}
