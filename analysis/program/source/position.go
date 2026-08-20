package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	sealedindex "github.com/wippyai/go-lua/analysis/program/source/index"
)

func installPositions(a *authority, index **sealedindex.Table, locations directLocations, input IndexInput) error {
	// Positions is the exact batch for Flow's reachable containment closure.
	// Identity/span cardinality is a separate denominator; direct Body source
	// occurrences are mandatory, while Terms outside that closure have no
	// source-position projection. Position.Term is the sole row identity, and
	// the explicit family/ordinal order is part of this boundary.
	// Allocate the retained batch exactly once. The input is already canonical
	// by (Family, Ordinal), so each family can be carved from this backing array
	// without per-family geometric growth or a sorting/counting pass.
	entries := make([]sealedindex.Row, len(input.Positions))
	var directCounts [keyspace.FamilyCount]int
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
		if err := validateFrontier(row, location); err != nil {
			return err
		}
		entries[position] = sealedindex.NewRow(
			row.Term, row.Root, row.Body, row.Offset, row.Cursor,
			row.FrontierBody, row.FrontierCursor,
		)
	}
	table, err := sealedindex.Seal(entries)
	if err != nil {
		return err
	}
	*index = table
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if directCounts[family] != len(locations[family].rows) {
			return errors.New("program/source: direct source Term lacks position")
		}
	}
	for _, row := range input.Positions {
		slot, ok := table.Lookup(row.Term)
		if !ok {
			return errors.New("program/source: retained source position is unavailable")
		}
		rootFamily := keyspace.TermFamily(slot.Root())
		if rootFamily == keyspace.FamilyInvalid {
			return errors.New("program/source: root lacks direct source position")
		}
		root, ok := table.Lookup(slot.Root())
		if !ok || root.Root() != slot.Root() || root.Body() != slot.Body() || root.Offset() != slot.Offset() || root.Cursor() != slot.Cursor() ||
			root.FrontierBody() != slot.FrontierBody() || root.FrontierCursor() != slot.FrontierCursor() {
			return errors.New("program/source: root position is not its direct source coordinate")
		}
	}
	return nil
}

func validateFrontier(row Position, location directLocation) error {
	// Flow's position seal supplies Repeat's kind and the exact Loop-to-child
	// selection. Source validates only the owner-local geometry represented by
	// this row: a direct Loop root and a frontier Body distinct from the direct
	// root owner. It does not infer the parent/root facts or which of two
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
	if keyspace.TermFamily(row.Root) != keyspace.FamilyLoop || row.FrontierBody == location.body {
		return errors.New("program/source: invalid Repeat source frontier")
	}
	return nil
}

// buildDirectLocations makes one temporary sparse validation index containing
// exactly the direct Body source occurrences. Build's authored order pass has
// already proved that those occurrences are valid and unique, so Commit need
// not allocate a second membership plane for every identity ordinal. The rows
// are discarded after position installation.
func buildDirectLocations(a *authority, positions []Position) (directLocations, error) {
	if a == nil {
		return directLocations{}, errors.New("program/source: invalid Source authority")
	}
	var result directLocations
	for _, row := range positions {
		if row.Term != row.Root {
			continue
		}
		if !a.validDirectBodyTerm(row.Term) || !a.validFamilyTerm(row.Body, keyspace.FamilyBody) ||
			row.Offset > keyspace.MaxTermOrdinal || row.Cursor > keyspace.MaxTermOrdinal {
			return directLocations{}, errors.New("program/source: invalid direct source position")
		}
		bodyRange := a.order.bodyRanges[keyspace.TermOrdinal(row.Body)-1]
		if !validRange(a.order.sourceTerms, bodyRange) || uint64(row.Offset) >= uint64(bodyRange.end-bodyRange.start) ||
			a.order.sourceTerms[bodyRange.start+row.Offset] != row.Term {
			return directLocations{}, errors.New("program/source: direct source position disagrees with authored order")
		}
		family := keyspace.TermFamily(row.Term)
		location := directLocation{term: row.Term, body: row.Body, offset: row.Offset, cursor: row.Cursor}
		if err := result[family].add(keyspace.TermOrdinal(row.Term), location); err != nil {
			return directLocations{}, err
		}
	}
	var expected [keyspace.FamilyCount]int
	for bodyOrdinal, sourceRange := range a.order.bodyRanges {
		if !validRange(a.order.sourceTerms, sourceRange) {
			return directLocations{}, errors.New("program/source: invalid Body source range")
		}
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1))
		for offset, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			family := keyspace.TermFamily(term)
			if !a.validDirectBodyTerm(term) || family == keyspace.FamilyInvalid {
				return directLocations{}, errors.New("program/source: invalid direct source Term")
			}
			expected[family]++
			location, ok := result.lookup(term)
			if !ok || location.body != body || location.offset != uint32(offset) {
				return directLocations{}, errors.New("program/source: direct source Term lacks position")
			}
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if expected[family] != len(result[family].rows) {
			return directLocations{}, errors.New("program/source: direct source position has no authored occurrence")
		}
	}
	return result, nil
}
