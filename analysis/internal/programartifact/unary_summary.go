package programartifact

import (
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// UnarySummaryRow is one reusable Program-owned unary numeric abstract
// interpretation at an exact output point. Program proves the representation
// once; Link adds only mount identity and never reopens the body.
type UnarySummaryRow struct {
	id, occurrence, body, point keyspace.ContentID
	op                          flowkind.UnaryOp
	operand, result             NumericRepresentation
}

func (artifact *Artifact) UnarySummaryCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.unarySummaries)
}

func (artifact *Artifact) UnarySummaryAt(index int) (UnarySummaryRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.unarySummaries) {
		return UnarySummaryRow{}, false
	}
	row := artifact.unarySummaries[index]
	return row, row.Available()
}

func (row UnarySummaryRow) Available() bool {
	return row.id.Available() && row.occurrence.Available() && row.body.Available() && row.point.Available() &&
		row.op == flowkind.UnaryNeg && row.operand.Valid() && row.result.Valid() && row.operand == row.result &&
		row.id == unarySummaryID(row.occurrence, row.body, row.point, row.op, row.operand, row.result)
}

func (row UnarySummaryRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}

func (row UnarySummaryRow) OccurrenceID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.occurrence
}

func (row UnarySummaryRow) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body
}

func (row UnarySummaryRow) OutputPointID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.point
}

func (row UnarySummaryRow) Operator() flowkind.UnaryOp {
	if !row.Available() {
		return 0
	}
	return row.op
}

func (row UnarySummaryRow) Representations() (operand, result NumericRepresentation, ok bool) {
	if !row.Available() {
		return 0, 0, false
	}
	return row.operand, row.result, true
}

func unarySummaryID(occurrence, body, point keyspace.ContentID, op flowkind.UnaryOp, operand, result NumericRepresentation) keyspace.ContentID {
	if !occurrence.Available() || !body.Available() || !point.Available() || op != flowkind.UnaryNeg ||
		!operand.Valid() || !result.Valid() || operand != result {
		return keyspace.ContentID{}
	}
	return digest("analysis/program-artifact/unary-summary", artifactFormat,
		bytesField(occurrence), bytesField(body), bytesField(point), uintField(uint64(op)), uintField(uint64(operand)), uintField(uint64(result)))
}
