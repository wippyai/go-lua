package module

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

var (
	errSemanticSourceUnavailable  = errors.New("program/module: semantic-source publication requires a committed View")
	errSemanticSourceInconsistent = errors.New("program/module: inconsistent committed semantic-source View")
)

// SemanticSourceFragment publishes Module's complete contribution to the
// generated semantic-source denominator. Only a committed View is accepted:
// authored Request is present on every Import row, while derived Key and Entry
// are unavailable until the Module finalizer commits.
//
// The returned publications are in the generated canonical order, including
// zero-cardinality definitions. They are detached cardinality claims; no
// Module rows or derived maps become a second semantic authority.
func buildSemanticSourceFragment(view View) ([]semanticsource.Publication, error) {
	counts, err := moduleSemanticSourceCounts(view)
	if err != nil {
		return nil, err
	}

	definitions, err := moduleSemanticSourceDefinitions()
	if err != nil {
		return nil, err
	}
	if len(definitions) != len(counts) {
		return nil, errSemanticSourceInconsistent
	}
	publications := make([]semanticsource.Publication, 0, len(definitions))
	for index, definition := range definitions {
		count := counts[index]
		publication, err := semanticsource.SealPublication(definition, count)
		if err != nil {
			return nil, err
		}
		publications = append(publications, publication)
	}
	return publications, nil
}

// moduleSemanticSourceCounts validates the live authored Import rows and all
// derived Entry families, returning only the six scalar cardinalities needed
// to compare the live owner with its one committed publication range.
func moduleSemanticSourceCounts(view View) ([semanticSourceFragmentPublicationCount]int, error) {
	var counts [semanticSourceFragmentPublicationCount]int
	component, ok := view.componentForRead()
	if !ok || component == nil || !view.ContentID().Available() {
		return counts, errSemanticSourceUnavailable
	}

	imports := view.Count()
	if !semanticCountFits(imports) {
		return counts, errSemanticSourceInconsistent
	}
	requests, err := semanticRequestCount(view, imports)
	if err != nil {
		return counts, err
	}

	entry := view.Entry()
	returns := entry.ReturnCount()
	if !semanticCountFits(returns) {
		return counts, errSemanticSourceInconsistent
	}
	rootCells, rootFunctions, err := semanticEntryRootCounts(entry, returns)
	if err != nil {
		return counts, err
	}
	members := entry.MemberTotal()
	if !semanticCountFits(members) {
		return counts, errSemanticSourceInconsistent
	}
	if err := validateMemberQueries(entry, returns, members); err != nil {
		return counts, err
	}
	counts = [...]int{imports, requests, returns, rootCells, members, rootFunctions}
	return counts, nil
}

func semanticRequestCount(view View, imports int) (int, error) {
	count := 0
	for index := 0; index < imports; index++ {
		row, ok := view.ImportAt(index)
		if !ok || row.Term != keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1)) ||
			row.Request == 0 || keyspace.TermFamily(row.Request) != keyspace.FamilyString ||
			keyspace.TermOrdinal(row.Request) == 0 || row.Key == 0 {
			return 0, errSemanticSourceInconsistent
		}
		if !semanticIncrement(&count) {
			return 0, errSemanticSourceInconsistent
		}
	}
	if count != imports {
		return 0, errSemanticSourceInconsistent
	}
	return imports, nil
}

func semanticEntryRootCounts(entry EntryView, returns int) (cells, functions int, err error) {
	for index := 0; index < returns; index++ {
		returned, ok := entry.ReturnAt(index)
		if !ok || keyspace.TermFamily(returned) != keyspace.FamilyReturn || keyspace.TermOrdinal(returned) == 0 {
			return 0, 0, errSemanticSourceInconsistent
		}
		rootCount, ok := entry.RootCount(returned)
		if !ok || !semanticCountFits(rootCount) {
			return 0, 0, errSemanticSourceInconsistent
		}
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			cell, ok := entry.RootCell(returned, rootIndex)
			if !ok && cell != 0 {
				return 0, 0, errSemanticSourceInconsistent
			}
			if ok {
				if keyspace.TermFamily(cell) != keyspace.FamilyCell || keyspace.TermOrdinal(cell) == 0 || !semanticIncrement(&cells) {
					return 0, 0, errSemanticSourceInconsistent
				}
			}
			function, ok := entry.RootFunction(returned, rootIndex)
			if !ok && function != 0 {
				return 0, 0, errSemanticSourceInconsistent
			}
			if ok {
				if keyspace.TermFamily(function) != keyspace.FamilyFunction || keyspace.TermOrdinal(function) == 0 || !semanticIncrement(&functions) {
					return 0, 0, errSemanticSourceInconsistent
				}
			}
		}
	}
	return cells, functions, nil
}

func validateMemberQueries(entry EntryView, returns, members int) error {
	count := 0
	for returnIndex := 0; returnIndex < returns; returnIndex++ {
		returned, ok := entry.ReturnAt(returnIndex)
		if !ok {
			return errSemanticSourceInconsistent
		}
		memberCount, ok := entry.MemberCount(returned)
		if !ok || !semanticCountFits(memberCount) {
			return errSemanticSourceInconsistent
		}
		for memberIndex := 0; memberIndex < memberCount; memberIndex++ {
			field, ok := entry.MemberAt(returned, memberIndex)
			if !ok || keyspace.TermFamily(field) != keyspace.FamilyTableField || keyspace.TermOrdinal(field) == 0 {
				return errSemanticSourceInconsistent
			}
			origin, _, table, ok := entry.MemberOrigin(field)
			if !ok || origin != returned || keyspace.TermFamily(table) != keyspace.FamilyTable || keyspace.TermOrdinal(table) == 0 {
				return errSemanticSourceInconsistent
			}
			parent, suffix, ok := entry.MemberParent(field)
			if !ok || suffix == 0 || (keyspace.TermFamily(parent) != keyspace.FamilyTable && keyspace.TermFamily(parent) != keyspace.FamilyTableField) || keyspace.TermOrdinal(parent) == 0 {
				return errSemanticSourceInconsistent
			}
			function, ok := entry.MemberFunction(field)
			if ok && (keyspace.TermFamily(function) != keyspace.FamilyFunction || keyspace.TermOrdinal(function) == 0) {
				return errSemanticSourceInconsistent
			}
			if !semanticIncrement(&count) {
				return errSemanticSourceInconsistent
			}
		}
	}
	if count != members {
		return errSemanticSourceInconsistent
	}
	return nil
}

func semanticCountFits(count int) bool {
	return count >= 0 && uint64(count) <= uint64(keyspace.MaxTermOrdinal)
}

func semanticIncrement(count *int) bool {
	if count == nil || *count < 0 || uint64(*count) >= uint64(keyspace.MaxTermOrdinal) {
		return false
	}
	(*count)++
	return true
}
