package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errStaticCounts = errors.New("program/static: invalid denominator counts")

// CountRows projects Static's native denominator rows off the census column
// Build sealed. Static measures only its own type, declaration, signature,
// contract, operand, operator, and publication columns; Program performs no
// Static re-census, and neither does this projection: every row reads a sealed
// number rather than walking the rows again to recount it.
//
// The primary row is the authored static type forest, which is exactly the
// census restricted to the inventory's forest window. The two rows that are
// not census entries are the sparse ClaimTarget relation, whose denominator is
// its nonzero rows rather than the owning ValueClaim census, and the call
// type-argument column, whose sealed width contracts owns.
func CountRows(view View) (denominator.CountRows, error) {
	component := view.componentOf()
	if component == nil || !view.Available() {
		return denominator.CountRows{}, errStaticCounts
	}
	ids := denominator.GeneratedProgramStaticIDs()
	values := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.ProgramStatic, component.StaticTypeTermCount()},
		{ids.ProgramStaticFunctionContract, int(component.census[keyspace.FamilyFunction])},
		{ids.ProgramStaticCallTypeArguments, int(component.contracts.callTypeArguments)},
		{ids.ProgramStaticCellDeclaredType, int(component.census[keyspace.FamilyDeclaredType])},
		{ids.ProgramStaticClaimTarget, len(component.operands.claims)},
		{ids.ProgramStaticTypeValueTarget, int(component.census[keyspace.FamilyTypeValue])},
		{ids.ProgramStaticTypeof, int(component.census[keyspace.FamilyTypeOf])},
		{ids.ProgramStaticAnnotation, int(component.census[keyspace.FamilyAnnotation])},
		{ids.ProgramStaticPublication, int(component.census[keyspace.FamilyTypePublication])},
		{ids.ProgramStaticTypeRef, int(component.census[keyspace.FamilyTypeRef])},
	}
	rows := make([]denominator.CountRow, 0, len(values))
	for _, value := range values {
		row, ok := staticCountRow(value.id, value.value)
		if !ok {
			return denominator.CountRows{}, errStaticCounts
		}
		rows = append(rows, row)
	}
	sealed, ok := denominator.NewCountRows(rows)
	if !ok {
		return denominator.CountRows{}, errStaticCounts
	}
	return sealed, nil
}

func staticCountRow(id schema.EntryID, value int) (denominator.CountRow, bool) {
	if !staticCountFits(value) {
		return denominator.CountRow{}, false
	}
	return denominator.NewCountRow(id, uint64(value))
}

func staticCountFits(value int) bool {
	return value >= 0 && uint64(value) <= uint64(keyspace.MaxTermOrdinal)
}
