package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	sealedindex "github.com/wippyai/go-lua/analysis/program/source/index"
)

func installPositions(a *authority, input IndexInput) (*sealedindex.Table, error) {
	if a == nil {
		return nil, errors.New("program/source: invalid Source authority")
	}
	// Positions is the exact batch for Flow's reachable containment closure.
	// Identity/span cardinality is a separate denominator; direct Body source
	// occurrences are mandatory, while Terms outside that closure have no
	// source-position projection. Position.Term is the sole row identity, and
	// the explicit family/ordinal order is part of this boundary.
	// The owner-issued builder retains the canonical rows directly. There is no
	// caller-owned DTO batch for the index package to copy before publication.
	builder := sealedindex.NewBuilder(len(input.Positions))
	var directCounts [keyspace.FamilyCount]int
	for _, row := range input.Positions {
		if !a.validTerm(row.Term) || !a.validTerm(row.Root) || !a.validFamilyTerm(row.Body, keyspace.FamilyBody) ||
			!a.validFamilyTerm(row.FrontierBody, keyspace.FamilyBody) ||
			!a.validDirectBodyTerm(row.Root) || row.Offset > keyspace.MaxTermOrdinal || row.Cursor > keyspace.MaxTermOrdinal ||
			keyspace.TermFamily(row.Term) == keyspace.FamilyOutcome {
			return nil, errors.New("program/source: invalid source position")
		}
		if row.Term == row.Root {
			directCounts[keyspace.TermFamily(row.Term)]++
		}
		if err := builder.Add(
			row.Term, row.Root, row.Body, row.Offset, row.Cursor,
			row.FrontierBody, row.FrontierCursor,
		); err != nil {
			return nil, err
		}
	}
	table, err := builder.Seal()
	if err != nil {
		return nil, err
	}
	// The sealed table is the sole temporary and retained position row store.
	// Validate roots against it directly: every root must be a direct Source
	// row, and every descendant must inherit that row's complete coordinate.
	for _, row := range input.Positions {
		slot, ok := table.Lookup(row.Term)
		if !ok {
			return nil, errors.New("program/source: retained source position is unavailable")
		}
		root, ok := table.Lookup(row.Root)
		if !ok || root.Root() != row.Root || root.Body() != slot.Body() || root.Offset() != slot.Offset() || root.Cursor() != slot.Cursor() ||
			root.FrontierBody() != slot.FrontierBody() || root.FrontierCursor() != slot.FrontierCursor() {
			return nil, errors.New("program/source: root position is not its direct source coordinate")
		}
		if err := validateFrontier(row, root); err != nil {
			return nil, err
		}
	}
	if err := validateDirectSourceRows(a, table, directCounts); err != nil {
		return nil, err
	}
	return table, nil
}

func validateFrontier(row Position, root sealedindex.Slot) error {
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
		if row.FrontierBody != root.Body() || row.FrontierCursor != root.Cursor() {
			return errors.New("program/source: invalid ordinary source frontier")
		}
		return nil
	}
	if keyspace.TermFamily(row.Root) != keyspace.FamilyLoop || row.FrontierBody == root.Body() {
		return errors.New("program/source: invalid Repeat source frontier")
	}
	return nil
}

// validateDirectSourceRows checks the authored direct Body sequence against
// the one sealed position table. Looking up each authored row in that table
// proves exact identity, Body owner, and source offset without a second
// direct-location index or a full-denominator membership plane. The direct
// count closes the converse: a sealed table cannot contain an extra direct
// root that was not authored.
func validateDirectSourceRows(a *authority, table *sealedindex.Table, directCounts [keyspace.FamilyCount]int) error {
	if a == nil || table == nil {
		return errors.New("program/source: invalid direct source table")
	}
	var expected [keyspace.FamilyCount]int
	for bodyOrdinal, sourceRange := range a.order.bodyRanges {
		if !validRange(a.order.sourceTerms, sourceRange) {
			return errors.New("program/source: invalid Body source range")
		}
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1))
		for offset, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			family := keyspace.TermFamily(term)
			if !a.validDirectBodyTerm(term) || family == keyspace.FamilyInvalid {
				return errors.New("program/source: invalid direct source Term")
			}
			expected[family]++
			row, ok := table.Lookup(term)
			if !ok || row.Root() != term || row.Body() != body || row.Offset() != uint32(offset) {
				return errors.New("program/source: direct source Term lacks position")
			}
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if directCounts[family] != expected[family] {
			return errors.New("program/source: direct source position has no authored occurrence")
		}
	}
	return nil
}
