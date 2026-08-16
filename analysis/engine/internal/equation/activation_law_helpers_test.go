package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func boundaryKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func boundaryDecision(t testing.TB, value byte) Decision {
	t.Helper()
	decision, ok := NewDecision(boundaryKey(value))
	if !ok {
		t.Fatal("decision")
	}
	return decision
}

func operandClosureFixture(t testing.TB, count int) (*Batch, Occurrence, []Operand) {
	t.Helper()
	batch := NewBatch()
	site, siteOK := batch.AdmitSite(boundaryKey(230), EmptyScope(), TrueExpr(), InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	operands := make([]Operand, count)
	for index := range operands {
		operand, ok := batch.AdmitOperand(occurrence, boundaryKey(byte(231+index)))
		if !ok {
			t.Fatalf("operand %d", index)
		}
		operands[index] = operand
	}
	if !siteOK || !occurrenceOK || !batch.Seal() {
		t.Fatal("operand closure batch")
	}
	return batch, occurrence, operands
}
