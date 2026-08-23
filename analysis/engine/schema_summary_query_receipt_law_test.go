package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

func summaryQueryLawSchema(t testing.TB) (*Schema, *FactorSlot[uint64], SchemaReadForm[uint64], *QuerySlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(954_001))
	form, formOK := factor.SummaryRead(coldKey(954_002))
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(954_003), Freezer: coldKey(953_100), Population: queryschema.PopulationKindSelectedPoint})
	readOK := SchemaQueryRead(query, form)
	schema, schemaOK := builder.Seal()
	if !factorOK || !formOK || !queryOK || !readOK || !schemaOK || schema == nil {
		t.Fatal("summary query law schema")
	}
	return schema, factor, form, query
}

func bindSummaryQueryLaw(binding *SchemaBinding, factor *FactorSlot[uint64], form SchemaReadForm[uint64], query *QuerySlot[uint64]) bool {
	return BindFactor(binding, factor, hotUintFactorSpec()) &&
		BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](binding, factor, form,
			func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
			func(left, right OrderedCells[uint64]) bool {
				return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
			},
			func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) &&
		BindSummaryQuery(binding, query, factor, form, hotSummaryQueryLawSpec())
}

func TestSchemaSummaryQueryBindsExactFormAndRejectsDuplicate(t *testing.T) {
	schema, factor, form, query := summaryQueryLawSchema(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !bindSummaryQueryLaw(binding, factor, form, query) || !binding.Seal() {
		t.Fatal("summary query form did not seal")
	}
	implementation, ok := SummaryQueryImplementationAt[uint64, uint64](binding, query)
	if row, rowOK := implementation.sealedRow(); !ok || implementation == nil || !rowOK || row == nil {
		t.Fatal("summary query implementation lost its sealed form")
	}
	duplicateSchema, duplicateFactor, duplicateForm, duplicateQuery := summaryQueryLawSchema(t)
	duplicate := NewSchemaBinding(duplicateSchema)
	if duplicate == nil || !bindSummaryQueryLaw(duplicate, duplicateFactor, duplicateForm, duplicateQuery) || BindSummaryQuery(duplicate, duplicateQuery, duplicateFactor, duplicateForm, hotSummaryQueryLawSpec()) || !duplicate.Poisoned() {
		t.Fatal("duplicate summary query row was admitted")
	}
}

func TestSchemaSummaryQueryRejectsForeignEqualSchemaAndIncompleteInventory(t *testing.T) {
	schema, factor, form, _ := summaryQueryLawSchema(t)
	foreignSchema, _, _, foreignQuery := summaryQueryLawSchema(t)
	if schema.ID() != foreignSchema.ID() {
		t.Fatal("equal summary schemas did not canonicalize")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || BindSummaryQuery(binding, foreignQuery, factor, form, hotSummaryQueryLawSpec()) || !binding.Poisoned() {
		t.Fatal("foreign summary query crossed the binding authority")
	}
	missingSchema, missingFactor, missingForm, _ := summaryQueryLawSchema(t)
	missing := NewSchemaBinding(missingSchema)
	if missing == nil || !BindFactor(missing, missingFactor, hotUintFactorSpec()) || !BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](missing, missingFactor, missingForm,
		func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
		func(left, right OrderedCells[uint64]) bool {
			return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
		},
		func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) || missing.Seal() || !missing.Poisoned() {
		t.Fatal("incomplete summary query inventory published")
	}
}

func TestSummaryQueryPublishesItsDeclaredForm(t *testing.T) {
	schema, factor, form, query := summaryQueryLawSchema(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !bindSummaryQueryLaw(binding, factor, form, query) || !binding.Seal() {
		t.Fatal("summary query binding")
	}
	implementation, ok := SummaryQueryImplementationAt[uint64, uint64](binding, query)
	row, rowOK := implementation.sealedRow()
	projection, projectionOK := schema.queryProjectionShapeAt(row.ordinal, 0)
	if !ok || implementation == nil || !rowOK || !projectionOK || projection.Normalizer != compositionKeyOf(coldKey(954_002)) {
		t.Fatal("summary query implementation retained a foreign form")
	}
	surface := equation.Surface{Factor: compositionKeyOf(coldKey(954_001)), Form: equation.SurfaceReadSummary, Local: 1, Semantic: compositionKeyOf(coldKey(954_002)), Normalizer: compositionKeyOf(coldKey(954_002))}
	mapping, mappingOK := implementation.topologySummaryMapping(surface)
	if !mappingOK || mapping.Surface != surface || len(mapping.Keys) == 0 {
		t.Fatal("summary query evidence surface was not published")
	}
	wrong := surface
	wrong.Form = equation.SurfaceReadExact
	if _, accepted := implementation.topologySummaryMapping(wrong); accepted {
		t.Fatal("summary query accepted an exact surface in the summary lane")
	}
}
