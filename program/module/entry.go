package module

import "github.com/wippyai/go-lua/program/keyspace"

// EntryRange selects one dense final segment in an Entry-owned pool.
type EntryRange struct {
	Start uint32
	End   uint32
}

// EntryMember is one exact named member retained from a directly returned
// chunk-activation table surface. Cross-owner validity is checked by the root;
// Module checks the local relation and family shape.
type EntryMember struct {
	Field    keyspace.Term
	Parent   keyspace.Term
	Value    keyspace.Term
	Returned keyspace.Term
	Table    keyspace.Term
	Suffix   keyspace.Key
	Ordinal  uint32
}

// Entry is the derived chunk-entry projection supplied to Finalizer.Commit.
// It is not authored identity and is absent from Module ContentID.
//
// The slices are consumed by value and copied into immutable Component
// storage. Callers may freely reuse or mutate this input after Commit returns.
type Entry struct {
	ReturnTerms  []keyspace.Term
	ReturnIndex  []uint32
	RootRanges   []EntryRange
	Roots        []keyspace.Term
	RootCells    []keyspace.Term
	MemberRanges []EntryRange
	Members      []EntryMember
	MemberIndex  []uint32
}

type entryData struct {
	returnTerms  []keyspace.Term
	returnIndex  []uint32
	rootRanges   []EntryRange
	roots        []keyspace.Term
	rootCells    []keyspace.Term
	memberRanges []EntryRange
	members      []EntryMember
	memberIndex  []uint32
}

// valid applies the one Entry structural law to retained component storage
// without rebuilding or copying the derived slices.
func (data entryData) valid() bool {
	return (Entry{
		ReturnTerms:  data.returnTerms,
		ReturnIndex:  data.returnIndex,
		RootRanges:   data.rootRanges,
		Roots:        data.roots,
		RootCells:    data.rootCells,
		MemberRanges: data.memberRanges,
		Members:      data.members,
		MemberIndex:  data.memberIndex,
	}).valid()
}

// EntryView is the immutable query view retained by a committed Component.
type EntryView struct{ data *entryData }

func cloneEntry(input Entry) entryData {
	return entryData{
		returnTerms:  append([]keyspace.Term(nil), input.ReturnTerms...),
		returnIndex:  append([]uint32(nil), input.ReturnIndex...),
		rootRanges:   append([]EntryRange(nil), input.RootRanges...),
		roots:        append([]keyspace.Term(nil), input.Roots...),
		rootCells:    append([]keyspace.Term(nil), input.RootCells...),
		memberRanges: append([]EntryRange(nil), input.MemberRanges...),
		members:      append([]EntryMember(nil), input.Members...),
		memberIndex:  append([]uint32(nil), input.MemberIndex...),
	}
}

func (input Entry) valid() bool {
	if len(input.ReturnIndex) == 0 || len(input.RootRanges) != len(input.ReturnIndex) ||
		len(input.MemberRanges) != len(input.ReturnIndex) || len(input.Roots) != len(input.RootCells) {
		return false
	}
	for index, root := range input.Roots {
		if root != 0 && !validFamilyTerm(root, keyspace.FamilyFunction) {
			return false
		}
		if cell := input.RootCells[index]; cell != 0 && !validFamilyTerm(cell, keyspace.FamilyCell) {
			return false
		}
	}
	var previousReturn uint32
	for index, returned := range input.ReturnTerms {
		ordinal := keyspace.TermOrdinal(returned)
		if keyspace.TermFamily(returned) != keyspace.FamilyReturn || ordinal == 0 || ordinal <= previousReturn ||
			uint64(ordinal) >= uint64(len(input.ReturnIndex)) || input.ReturnIndex[ordinal] != uint32(index+1) {
			return false
		}
		previousReturn = ordinal
	}
	var rootEnd, memberEnd uint32
	for ordinal, row := range input.ReturnIndex {
		roots, members := input.RootRanges[ordinal], input.MemberRanges[ordinal]
		if !entryRangeValid(roots, len(input.Roots)) || !entryRangeValid(members, len(input.Members)) {
			return false
		}
		if row == 0 {
			if roots.Start != 0 || roots.End != 0 || members.Start != 0 || members.End != 0 {
				return false
			}
			continue
		}
		if uint64(row) > uint64(len(input.ReturnTerms)) ||
			keyspace.TermOrdinal(input.ReturnTerms[row-1]) != uint32(ordinal) ||
			roots.Start != rootEnd || members.Start != memberEnd {
			return false
		}
		returned := input.ReturnTerms[row-1]
		for memberIndex := members.Start; memberIndex < members.End; memberIndex++ {
			member := input.Members[memberIndex]
			if !validMember(member, returned) || member.Ordinal >= roots.End-roots.Start {
				return false
			}
		}
		rootEnd = roots.End
		memberEnd = members.End
	}
	if int(rootEnd) != len(input.Roots) || int(memberEnd) != len(input.Members) {
		return false
	}
	for index, row := range input.MemberIndex {
		if row == 0 {
			continue
		}
		if uint64(row) > uint64(len(input.Members)) ||
			keyspace.TermOrdinal(input.Members[row-1].Field) != uint32(index) {
			return false
		}
	}
	for index, member := range input.Members {
		field := keyspace.TermOrdinal(member.Field)
		returned := keyspace.TermOrdinal(member.Returned)
		if !validFamilyTerm(member.Field, keyspace.FamilyTableField) || field >= uint32(len(input.MemberIndex)) ||
			input.MemberIndex[field] != uint32(index+1) || !validFamilyTerm(member.Returned, keyspace.FamilyReturn) ||
			returned >= uint32(len(input.ReturnIndex)) || input.ReturnIndex[returned] == 0 ||
			input.ReturnTerms[input.ReturnIndex[returned]-1] != member.Returned {
			return false
		}
	}
	return true
}

func validMember(member EntryMember, returned keyspace.Term) bool {
	if member.Returned != returned || !validFamilyTerm(member.Field, keyspace.FamilyTableField) ||
		!validFamilyTerm(member.Returned, keyspace.FamilyReturn) ||
		!validFamilyTerm(member.Table, keyspace.FamilyTable) || member.Suffix == 0 {
		return false
	}
	if !validFamilyTerm(member.Parent, keyspace.FamilyTable) &&
		!validFamilyTerm(member.Parent, keyspace.FamilyTableField) {
		return false
	}
	return member.Value == 0 || validFamilyTerm(member.Value, keyspace.FamilyFunction)
}

func entryRangeValid(r EntryRange, length int) bool {
	return r.Start <= r.End && uint64(r.End) <= uint64(length)
}
