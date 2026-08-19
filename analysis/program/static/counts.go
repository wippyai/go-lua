package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errStaticCounts = errors.New("program/static: invalid denominator counts")

// CountRows publishes Static's native measures under the generated schema
// denominator identities. The schema owns row identity and order; each typed
// child contributes its own sealed measure and Static only joins those values
// to the generated owner column.
//
// The primary row is the authored static type forest. ClaimTarget remains a
// sparse child-owned relation, and the call type-argument row is the sealed
// width of the contracts column rather than a row count.
func CountRows(view View) (denominator.CountRows, error) {
	component := view.componentOf()
	if component == nil || !view.Available() {
		return denominator.CountRows{}, errStaticCounts
	}
	typeRows, typeOK := component.types.CountRows()
	refRows, refOK := component.references.CountRows()
	declarationRows, declarationOK := component.declarations.CountRows()
	signatureRows, signatureOK := component.signatures.CountRows()
	contractRows, contractOK := component.contracts.CountRows()
	operatorRows, operatorOK := component.operators.CountRows()
	operandRows, operandOK := component.operands.CountRows()
	publicationRows, publicationOK := component.publications.CountRows()
	if !typeOK || !refOK || !declarationOK || !signatureOK || !contractOK || !operatorOK || !operandOK || !publicationOK {
		return denominator.CountRows{}, errStaticCounts
	}
	parts := []denominator.CountRows{
		typeRows, refRows, declarationRows, signatureRows,
		contractRows, operatorRows, operandRows, publicationRows,
	}
	rows, ok := denominator.SumCountRows(parts...)
	if !ok || !denominator.GeneratedCountRowsCompleteForOwners(rows, denominator.RelationOwnerProgramStatic) {
		return denominator.CountRows{}, errStaticCounts
	}
	return rows, nil
}
