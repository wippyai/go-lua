package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func installPositions(a *authority, index *indexStore, locations directLocations, input IndexInput) error {
	// Positions is the exact batch for Flow's reachable containment closure.
	// Identity/span cardinality is a separate denominator; direct Body source
	// occurrences are mandatory, while Terms outside that closure have no
	// source-position projection. Position.Term is the sole row identity, and
	// the explicit family/ordinal order is part of this boundary.
	// Allocate the retained batch exactly once. The input is already canonical
	// by (Family, Ordinal), so each family can be carved from this backing array
	// without per-family geometric growth or a sorting/counting pass.
	entries := make([]positionEntry, len(input.Positions))
	var directCounts [keyspace.FamilyCount]int
	familyStart := 0
	var installedFamily keyspace.Family
	var previousFamily keyspace.Family
	var previousOrdinal uint32
	for position, row := range input.Positions {
		if !a.validTerm(row.Term) || !a.validTerm(row.Root) || !a.validFamilyTerm(row.Body, keyspace.FamilyBody) ||
			!a.validFamilyTerm(row.FrontierBody, keyspace.FamilyBody) ||
			keyspace.TermFamily(row.Term) == keyspace.FamilyOutcome {
			return errors.New("program/source: invalid source position")
		}
		family, termOrdinal := keyspace.TermFamily(row.Term), keyspace.TermOrdinal(row.Term)
		if previousFamily != keyspace.FamilyInvalid &&
			(family < previousFamily || family == previousFamily && termOrdinal <= previousOrdinal) {
			return errors.New("program/source: noncanonical source position order")
		}
		if installedFamily != keyspace.FamilyInvalid && family != installedFamily {
			index.positions[installedFamily] = positionIndex(entries[familyStart:position:position])
			installedFamily = family
			familyStart = position
		} else if installedFamily == keyspace.FamilyInvalid {
			installedFamily = family
			familyStart = position
		}
		previousFamily, previousOrdinal = family, termOrdinal
		location, ok := locations.lookup(row.Root)
		if !ok {
			return errors.New("program/source: source position root is not a direct source Term")
		}
		if location.body != row.Body || location.offset != row.Offset || location.cursor != row.Cursor {
			return errors.New("program/source: inconsistent source position")
		}
		// A direct Body source occurrence is its own canonical source root. The
		// root lookup above proves that row.Term is a direct source row whenever
		// row.Term == row.Root. Counting those rows per family, then requiring
		// the exact direct-row count below, preserves direct omission,
		// substitution, and uniqueness without a second direct membership scan.
		if row.Term == row.Root {
			directCounts[family]++
		}
		if err := validateFrontier(index, row, location); err != nil {
			return err
		}
		entries[position] = positionEntry{
			ordinal: keyspace.TermOrdinal(row.Term),
			slot: positionSlot{
				root: row.Root, body: row.Body, offset: row.Offset, cursor: row.Cursor,
				frontierBody: row.FrontierBody, frontierCursor: row.FrontierCursor,
			},
		}
	}
	if installedFamily != keyspace.FamilyInvalid {
		index.positions[installedFamily] = positionIndex(entries[familyStart:len(entries):len(entries)])
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if directCounts[family] != len(locations[family].rows) {
			return errors.New("program/source: direct source Term lacks position")
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for _, entry := range index.positions[family] {
			slot := entry.slot
			rootFamily, rootOrdinal := keyspace.TermFamily(slot.root), keyspace.TermOrdinal(slot.root)
			if rootFamily == keyspace.FamilyInvalid {
				return errors.New("program/source: root lacks direct source position")
			}
			root, ok := index.positions[rootFamily].lookup(rootOrdinal)
			if !ok || root.root != slot.root || root.body != slot.body || root.offset != slot.offset || root.cursor != slot.cursor ||
				root.frontierBody != slot.frontierBody || root.frontierCursor != slot.frontierCursor {
				return errors.New("program/source: root position is not its direct source coordinate")
			}
		}
	}
	return nil
}

func validateFrontier(index *indexStore, row Position, location directLocation) error {
	// Flow's position seal supplies Repeat's kind and the exact Loop-to-child
	// selection. Source validates only the owner-local geometry represented by
	// this row: a direct Loop root, a Body child of the containing Body, and the
	// selected child's sealed root-tail cursor. It does not infer which of two
	// same-owner Body children Flow selected.
	if !row.Repeat {
		// A non-direct row inherits all six position fields from its direct
		// root. Defer its frontier check until the complete batch is installed;
		// this is what permits a descendant of a Repeat root to inherit that
		// root's adjusted frontier without opening a second frontier authority.
		if row.Term != row.Root {
			return nil
		}
		if row.FrontierBody != location.body || row.FrontierCursor != location.cursor {
			return errors.New("program/source: invalid ordinary source frontier")
		}
		return nil
	}
	if keyspace.TermFamily(row.Root) != keyspace.FamilyLoop || row.FrontierBody == location.body ||
		int(keyspace.TermOrdinal(row.FrontierBody)) > len(index.parents) ||
		index.parents[keyspace.TermOrdinal(row.FrontierBody)-1] != location.body {
		return errors.New("program/source: invalid Repeat source frontier")
	}
	r := index.rootRanges[keyspace.TermOrdinal(row.FrontierBody)-1]
	if row.FrontierCursor != r.end-r.start {
		return errors.New("program/source: invalid Repeat frontier cursor")
	}
	return nil
}

func validateBodyForest(a *authority, index *indexStore, locations directLocations, entry keyspace.Term) error {
	if a == nil || index == nil || !a.validFamilyTerm(entry, keyspace.FamilyBody) {
		return errors.New("program/source: invalid entry Body")
	}
	entryOrdinal := keyspace.TermOrdinal(entry) - 1
	rootCount := 0
	for ordinal, parent := range index.parents {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal+1))
		location, direct := locations.lookup(body)
		if parent == 0 {
			rootCount++
			if body != entry {
				return errors.New("program/source: non-entry root Body")
			}
			if direct {
				return errors.New("program/source: Entry Body has direct source occurrence")
			}
			continue
		}
		// A lexical Body parent is supplied by Flow's sealed Body forest. A
		// child Body may be represented only by a typed Function/Branch/Loop
		// witness, so it need not also occur as a direct Source term. When a
		// direct Body occurrence does exist, the sealed forest projection must
		// agree with that Source-owned witness.
		if direct && location.body != parent {
			return errors.New("program/source: direct Body parent mismatch")
		}
	}
	if rootCount != 1 || index.parents[entryOrdinal] != 0 {
		return errors.New("program/source: invalid Body root")
	}
	state := make([]uint8, len(index.parents))
	for start := range index.parents {
		if state[start] != 0 {
			continue
		}
		path := make([]uint32, 0, 4)
		for current := uint32(start); ; {
			if int(current) >= len(index.parents) {
				return errors.New("program/source: invalid Body parent ordinal")
			}
			switch state[current] {
			case 1:
				return errors.New("program/source: cyclic Body parent")
			case 2:
				for _, visited := range path {
					state[visited] = 2
				}
			default:
				state[current] = 1
				path = append(path, current)
				parent := index.parents[current]
				if parent == 0 {
					if current != entryOrdinal {
						return errors.New("program/source: Body forest does not reach entry")
					}
					for _, visited := range path {
						state[visited] = 2
					}
					path = nil
				}
				if path == nil {
					break
				}
				current = keyspace.TermOrdinal(parent) - 1
				continue
			}
			break
		}
	}
	return nil
}

// buildDirectLocations makes one temporary sparse validation index containing
// exactly the direct Body source occurrences. Build's authored order pass has
// already proved that those occurrences are valid and unique, so Commit need
// not allocate a second membership plane for every identity ordinal. The rows
// are discarded after position installation.
func buildDirectLocations(a *authority, index *indexStore) (directLocations, error) {
	var result directLocations
	for _, sourceRange := range a.order.bodyRanges {
		if !validRange(a.order.sourceTerms, sourceRange) {
			return directLocations{}, errors.New("program/source: invalid Body source range")
		}
		for _, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			family := keyspace.TermFamily(term)
			if !a.validDirectBodyTerm(term) || family == keyspace.FamilyInvalid {
				return directLocations{}, errors.New("program/source: invalid direct source Term")
			}
		}
	}
	for bodyOrdinal, sourceRange := range a.order.bodyRanges {
		rootRange := index.rootRanges[bodyOrdinal]
		rootAt := uint32(0)
		cursor := uint32(0)
		for offset, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			family := keyspace.TermFamily(term)
			location := directLocation{
				term:   term,
				body:   keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1)),
				offset: uint32(offset),
				cursor: cursor,
			}
			if err := result[family].add(keyspace.TermOrdinal(term), location); err != nil {
				return directLocations{}, err
			}
			if rootAt < rootRange.end-rootRange.start && index.rootTerms[rootRange.start+rootAt] == term {
				rootAt++
				cursor++
			}
		}
		if rootAt != rootRange.end-rootRange.start {
			return directLocations{}, errors.New("program/source: unordered or non-direct statement root")
		}
	}
	return result, nil
}
