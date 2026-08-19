package static

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
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
func CountRows(view staticquery.View) (denominator.CountRows, error) {
	snapshot, ok := view.Snapshot()
	if !ok {
		return denominator.CountRows{}, errStaticCounts
	}
	types, references, declarations, signatures, contracts, operators, operands, publications := snapshot.Tables()
	typeRows, typeOK := types.CountRows()
	refRows, refOK := references.CountRows()
	declarationRows, declarationOK := declarations.CountRows()
	signatureRows, signatureOK := signatures.CountRows()
	contractRows, contractOK := contracts.CountRows()
	operatorRows, operatorOK := operators.CountRows()
	operandRows, operandOK := operands.CountRows()
	publicationRows, publicationOK := publications.CountRows()
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
