package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errStaticCounts = errors.New("program/static: invalid denominator counts")

// CountRows derives Static's native denominator rows. Static measures only
// its own type, declaration, signature, contract, operand, operator, and
// publication columns; Program performs no Static re-census.
func CountRows(view View) (denominator.CountRows, error) {
	if !view.Available() {
		return denominator.CountRows{}, errStaticCounts
	}
	types, declarations, signatures, contracts, operators, operands := view.Types(), view.Declarations(), view.Signatures(), view.Contracts(), view.Operators(), view.Operands()
	primaryParts := []int{
		declarations.Aliases().Count(), declarations.Interfaces().Count(), declarations.TypeParams().Count(),
		types.Primitives().Count(), types.Literals().Count(), types.Optionals().Count(), types.Unions().Count(), types.Intersections().Count(), types.Generics().Count(), types.Arrays().Count(), types.Maps().Count(), types.Records().Count(),
		view.References().Count(), signatures.TypeFunctions().Count(), signatures.Assertions().Count(), operators.TypeOfs().Count(), operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count(),
	}
	primary, ok := denominator.SumInts(primaryParts...)
	if !ok || !staticCountFits(primary) {
		return denominator.CountRows{}, errStaticCounts
	}
	callArguments := 0
	for index := 0; index < contracts.Calls().Count(); index++ {
		term, ok := contracts.Calls().At(index)
		if !ok {
			return denominator.CountRows{}, errStaticCounts
		}
		argumentCount, ok := contracts.Calls().TypeArgumentCount(term)
		if !ok || !addStaticCount(&callArguments, argumentCount) {
			return denominator.CountRows{}, errStaticCounts
		}
	}
	ids := denominator.GeneratedProgramStaticIDs()
	values := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.ProgramStatic, primary},
		{ids.ProgramStaticFunctionContract, contracts.Functions().Count()},
		{ids.ProgramStaticCallTypeArguments, callArguments},
		{ids.ProgramStaticCellDeclaredType, declarations.DeclaredTypes().Count()},
		{ids.ProgramStaticClaimTarget, operands.Claims().Count()},
		{ids.ProgramStaticTypeValueTarget, operands.TypeValues().Count()},
		{ids.ProgramStaticTypeof, operators.TypeOfs().Count()},
		{ids.ProgramStaticAnnotation, operands.Annotations().Count()},
		{ids.ProgramStaticPublication, view.Publications().Count()},
		{ids.ProgramStaticTypeRef, view.References().Count()},
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

func addStaticCount(total *int, value int) bool {
	if total == nil || !staticCountFits(value) {
		return false
	}
	sum, ok := denominator.SumInts(*total, value)
	if !ok || !staticCountFits(sum) {
		return false
	}
	*total = sum
	return true
}
