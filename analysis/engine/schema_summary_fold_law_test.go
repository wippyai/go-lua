package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// TestSummaryReadFoldIsSealedIntoTheDeclaredForm proves the coordinate-wise
// fold is a property of the declared cold read form. Two summary forms over
// the same Factor carry different cold rows, and the fold is recovered from
// the sealed schema alone: no Rule, Query, or observation supplies it.
func TestSummaryReadFoldIsSealedIntoTheDeclaredForm(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	correlated, correlatedOK := factor.SummaryRead(coldKey(948_002))
	distributive, distributiveOK := factor.DistributiveSummaryRead(coldKey(948_003))
	schema, schemaOK := builder.Seal()
	if !factorOK || !correlatedOK || !distributiveOK || !schemaOK || schema == nil {
		t.Fatal("two-fold summary schema")
	}
	if correlated.Kind() != SchemaFormReadSummary || distributive.Kind() != SchemaFormReadDistributiveSummary {
		t.Fatalf("declared form kinds = %d/%d", correlated.Kind(), distributive.Kind())
	}
	ordinal, ordinalOK := factor.Ordinal()
	if !ordinalOK {
		t.Fatal("factor ordinal")
	}
	count, countOK := schema.factorFormCount(ordinal)
	if !countOK || count != 2 {
		t.Fatalf("declared cold form rows = %d, want 2", count)
	}
	kinds := map[composition.Key]composition.FactorFormKind{}
	for index := 0; index < count; index++ {
		shape, shapeOK := schema.factorFormShapeAt(ordinal, uint64(index))
		if !shapeOK {
			t.Fatal("cold form shape")
		}
		kinds[shape.Semantic] = shape.Kind
	}
	if kinds[coldKey(948_002).compositionKey()] != composition.FactorSummaryRead {
		t.Fatal("correlated summary lost its cold row kind")
	}
	if kinds[coldKey(948_003).compositionKey()] != composition.FactorDistributiveSummaryRead {
		t.Fatal("distributive summary lost its cold row kind")
	}

	correlatedFold, correlatedFoldOK := summaryReadFormFold(schema, ordinal, coldKey(948_002).compositionKey())
	distributiveFold, distributiveFoldOK := summaryReadFormFold(schema, ordinal, coldKey(948_003).compositionKey())
	if !correlatedFoldOK || correlatedFold || !distributiveFoldOK || !distributiveFold {
		t.Fatalf("recovered folds = %t/%t (%t/%t)", correlatedFold, distributiveFold, correlatedFoldOK, distributiveFoldOK)
	}
	if _, ok := summaryReadFormFold(schema, ordinal, coldKey(948_004).compositionKey()); ok {
		t.Fatal("undeclared normalizer resolved a fold")
	}
}

// TestSummaryReadFoldsAreDistinctDeclaredSemantics proves the second fold is a
// distinct declaration rather than a re-reading of the first: one semantic key
// names exactly one form and one fold.
func TestSummaryReadFoldsAreDistinctDeclaredSemantics(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_010))
	_, correlatedOK := factor.SummaryRead(coldKey(948_011))
	if !factorOK || !correlatedOK {
		t.Fatal("correlated summary form")
	}
	if _, ok := factor.DistributiveSummaryRead(coldKey(948_011)); ok {
		t.Fatal("one semantic key declared both folds")
	}
	if _, ok := builder.Seal(); ok {
		t.Fatal("poisoned builder sealed")
	}
}
