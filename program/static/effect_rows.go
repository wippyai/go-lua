package static

import (
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// compactEffectRows owns the sparse Function -> authored RowSpec relation.
// It intentionally has no operation-label resolver: the current source
// grammar has no effect-operation identity, so nonempty occurrence vectors
// fail closed instead of acquiring a second label authority.
func compactEffectRows(component *Component, counts [keyspace.FamilyCount]uint32, input EffectRowsInput) error {
	if component == nil {
		return errors.New("program/static: missing component for effect rows")
	}
	if len(input.Rows) == 0 {
		return nil
	}
	// Sorting a private copy makes the retained sparse relation canonical while
	// preserving caller ownership and avoiding a map keyed by foreign terms.
	rows := append([]EffectRow(nil), input.Rows...)
	sort.Slice(rows, func(left, right int) bool { return rows[left].Function < rows[right].Function })
	store := &component.effectRows
	if count := counts[keyspace.FamilyFunction]; count != 0 {
		store.byFunction = make([]uint32, int(count)+1)
	}
	if uint64(len(rows)) > uint64(keyspace.MaxTermOrdinal) {
		return errors.New("program/static: too many effect rows")
	}
	for index, row := range rows {
		if !keyspace.ValidTerm(row.Function, keyspace.FamilyFunction, int(counts[keyspace.FamilyFunction])) {
			return errors.New("program/static: effect row has foreign function owner")
		}
		if index != 0 && row.Function == rows[index-1].Function {
			return errors.New("program/static: duplicate effect row owner")
		}
		if err := validateRowSpec(row.Row); err != nil {
			return err
		}
		start := len(store.occurrences)
		if uint64(start)+uint64(len(row.Row.Occurrences)) > uint64(math.MaxUint32) {
			return errors.New("program/static: oversized effect occurrence pool")
		}
		store.occurrences = append(store.occurrences, row.Row.Occurrences...)
		store.rows = append(store.rows, effectRow{
			function:    row.Function,
			occurrences: poolRange{Start: uint32(start), End: uint32(len(store.occurrences))},
			rowFormals:  row.Row.RowFormals,
			tail:        row.Row.Tail,
			variable:    row.Row.Var,
		})
		store.byFunction[keyspace.TermOrdinal(row.Function)] = uint32(len(store.rows))
	}
	return nil
}

func validateRowSpec(row RowSpec) error {
	if row.RowFormals > MaxRowFormals {
		return errors.New("program/static: effect row formals exceed local bound")
	}
	switch row.Tail {
	case RowClosed:
		if row.Var != 0 {
			return errors.New("program/static: closed effect row carries a variable")
		}
	case RowVariable:
		if row.RowFormals == 0 {
			return errors.New("program/static: variable effect row has zero formals")
		}
		if row.Var >= RowVar(row.RowFormals) {
			return errors.New("program/static: effect row variable is outside formals")
		}
	default:
		return errors.New("program/static: invalid effect row tail")
	}
	// No source/schema identity exists for an occurrence yet.  This check is
	// deliberately explicit: silently dropping labels would make artifacts and
	// ContentID unsound, while accepting them would create an ownerless effect
	// vocabulary.
	if len(row.Occurrences) != 0 {
		return errors.New("program/static: effect row occurrences are not admitted")
	}
	return nil
}

// writeEffectRowsContent writes the sparse rows in canonical Function order.
// Pool offsets and derived lookup state never enter the identity or artifact.
func writeEffectRowsContent(writer *canonical.Writer, store effectRowsStore) error {
	if writer == nil {
		return canonical.ErrNilDestination
	}
	if err := writer.Count(uint64(len(store.rows))); err != nil {
		return err
	}
	for _, row := range store.rows {
		if err := writer.Uint(uint64(row.function)); err != nil {
			return err
		}
		count := row.occurrences.End - row.occurrences.Start
		if err := writer.Count(uint64(count)); err != nil {
			return err
		}
		// Occurrence count is currently always zero by validateRowSpec.  Keep
		// the explicit loop so the wire schema cannot accidentally omit the
		// field when operation identity is introduced in a later source cut.
		for range count {
			// Deliberately no payload: EffectOccurrence is currently closed and
			// empty. A nonempty row cannot reach this writer today.
		}
		if err := writer.Uint(uint64(row.rowFormals)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.tail)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.variable)); err != nil {
			return err
		}
	}
	return nil
}
