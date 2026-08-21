package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errSourceCounts = errors.New("program/source: invalid denominator counts")

// authoredTermCount is the canonical Source family census. It deliberately
// excludes Outcome because that family is assigned by Flow after authored
// Source input is built and must not enter either Source's content identity
// or its portable artifact payload.
func authoredTermCount(identity *identityStore) (uint32, bool) {
	if identity == nil {
		return 0, false
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		total += uint64(identity.familyCount(family))
	}
	if total == 0 || total > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(total), true
}

// CountRows derives Source's native denominator rows. Source owns the rows
// that describe provenance, source order, keys, literals, faults, and the
// authored source order; Flow owns the sealed Body containment projection.
func CountRows(view View) (denominator.CountRows, error) {
	if !view.Identity().ContentID().Available() || view.Identity().Name() == "" {
		return denominator.CountRows{}, errSourceCounts
	}
	bodyCount := view.Identity().FamilyCount(keyspace.FamilyBody)
	if !sourceCountFits(bodyCount) {
		return denominator.CountRows{}, errSourceCounts
	}
	direct := 0
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		bodyLen, ok := view.Order().BodyLen(body)
		if !ok || !addSourceCount(&direct, bodyLen) {
			return denominator.CountRows{}, errSourceCounts
		}
	}
	literals := view.Literals()
	literalCount, ok := denominator.SumInts(
		literals.Nils().Count(), literals.Bools().Count(), literals.Integers().Count(),
		literals.Floats().Count(), literals.Strings().Count(),
	)
	if !ok {
		return denominator.CountRows{}, errSourceCounts
	}
	keys, exactKeys := view.Keys().Count(), view.Keys().ExactCount()
	faults := view.Faults().Count()
	if !sourceCountFits(literalCount) || !sourceCountFits(keys) || !sourceCountFits(exactKeys) || !sourceCountFits(faults) {
		return denominator.CountRows{}, errSourceCounts
	}
	ids := denominator.GeneratedProgramSourceIDs()
	values := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.ProgramSourceProvenance, direct},
		{ids.ProgramSourceOrder, direct},
		{ids.ProgramSourceKey, keys},
		{ids.ProgramSourceExactKey, exactKeys},
		{ids.ProgramSourceControlFault, faults},
		{ids.ProgramFlowLiterals, literalCount},
		{ids.ProgramFlowBody, bodyCount},
	}
	rows := make([]denominator.CountRow, 0, len(values))
	for _, value := range values {
		row, ok := sourceCountRow(value.id, value.value)
		if !ok {
			return denominator.CountRows{}, errSourceCounts
		}
		rows = append(rows, row)
	}
	sealed, ok := denominator.NewCountRows(rows)
	if !ok {
		return denominator.CountRows{}, errSourceCounts
	}
	return sealed, nil
}

func sourceCountRow(id schema.EntryID, value int) (denominator.CountRow, bool) {
	if !sourceCountFits(value) {
		return denominator.CountRow{}, false
	}
	return denominator.NewCountRow(id, uint64(value))
}

func sourceCountFits(value int) bool {
	return value >= 0 && uint64(value) <= uint64(keyspace.MaxTermOrdinal)
}

func addSourceCount(total *int, value int) bool {
	if total == nil || !sourceCountFits(value) {
		return false
	}
	sum, ok := denominator.SumInts(*total, value)
	if !ok || !sourceCountFits(sum) {
		return false
	}
	*total = sum
	return true
}
