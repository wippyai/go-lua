package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func summaryQuerySchemaFixture(t testing.TB) (*Schema, *FactorSlot[uint64], SchemaReadForm[uint64], *QuerySlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(949_001))
	form, formOK := factor.SummaryRead(coldKey(949_002))
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(949_003), Freezer: coldKey(949_004)})
	readOK := SchemaQueryRead(query, form)
	schema, schemaOK := builder.Seal()
	if !factorOK || !formOK || !queryOK || !readOK || !schemaOK || schema == nil {
		t.Fatal("summary query schema")
	}
	return schema, factor, form, query
}

func hotSummaryQuerySpec() HotSummaryQuerySpec[uint64, uint64] {
	return HotSummaryQuerySpec[uint64, uint64]{
		Project: func(cells OrderedCells[uint64]) uint64 { return uint64(cells.Count()) },
		Result: FrozenResult[uint64]{
			Semantic:    coldKey(949_004),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
		},
	}
}

func bindSummaryQueryFixture(binding *SchemaBinding, factor *FactorSlot[uint64], form SchemaReadForm[uint64], query *QuerySlot[uint64]) bool {
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		return false
	}
	if !BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](binding, factor, form,
		func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
		func(left, right OrderedCells[uint64]) bool {
			return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
		},
		func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) {
		return false
	}
	return BindSummaryQuery(binding, query, factor, form, hotSummaryQuerySpec())
}

func TestSchemaSummaryQueryReceiptBindsExactFormAndRejectsDuplicate(t *testing.T) {
	schema, factor, form, query := summaryQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !bindSummaryQueryFixture(binding, factor, form, query) || !binding.Seal() {
		t.Fatal("summary query receipt binding")
	}
	implementation, ok := SummaryQueryImplementationAt[uint64, uint64](binding, query)
	if !ok || implementation == nil {
		t.Fatal("summary query implementation")
	}
	project, ok := implementation.projector()
	if !ok || project == nil || project(OrderedCells[uint64]{}) != 0 {
		t.Fatal("summary query projector")
	}

	duplicate := NewSchemaBinding(schema)
	if duplicate == nil || !BindFactor(duplicate, factor, hotUintFactorSpec()) || !BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](duplicate, factor, form,
		func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
		func(left, right OrderedCells[uint64]) bool {
			return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
		},
		func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) || !BindSummaryQuery(duplicate, query, factor, form, hotSummaryQuerySpec()) || BindSummaryQuery(duplicate, query, factor, form, hotSummaryQuerySpec()) || !duplicate.Poisoned() {
		// A second cell for the same canonical query row is rejected; this
		// check deliberately occurs before Seal so no partial receipt exists.
		t.Fatal("duplicate summary query attachment")
	}
}

func TestSchemaSummaryQueryReceiptRejectsForeignEqualSchemaAndIncompleteInventory(t *testing.T) {
	schema, factor, form, _ := summaryQuerySchemaFixture(t)
	foreignSchema, _, _, foreignQuery := summaryQuerySchemaFixture(t)
	if schema.ID() != foreignSchema.ID() {
		t.Fatal("equal summary schemas")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || BindSummaryQuery(binding, foreignQuery, factor, form, hotSummaryQuerySpec()) || !binding.Poisoned() {
		t.Fatal("foreign equal-schema summary query crossed owner fence")
	}

	// A fresh binding makes the missing Query inventory check unambiguous.
	missing := NewSchemaBinding(schema)
	if missing == nil || !BindFactor(missing, factor, hotUintFactorSpec()) || !BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](missing, factor, form,
		func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
		func(left, right OrderedCells[uint64]) bool {
			return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
		},
		func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) || missing.Seal() || !missing.Poisoned() {
		t.Fatal("incomplete summary query inventory published")
	}
}

func TestReceiptCompilerBindsSummaryQueryGraphSurface(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(949_010))
	form, formOK := factor.SummaryRead(coldKey(949_011))
	writeForm, writeFormOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(949_012), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(949_013)}, Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(949_014), Freezer: coldKey(949_015)})
	queryReadOK := SchemaQueryRead(query, form)
	schema, schemaOK := builder.Seal()
	if !factorOK || !formOK || !writeFormOK || !ruleOK || !writeOK || !queryOK || !queryReadOK || !schemaOK || schema == nil {
		t.Fatal("summary graph schema")
	}

	batch := equation.NewBatch()
	site, siteOK := batch.AdmitSite(coldKey(949_016).compositionKey(), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	operandValue := ruleUnitForSemantic(coldKey(949_017))
	operandEntity, operandEntityOK := operandEntityForContent(operandValue.content)
	operand, operandOK := batch.AdmitOperand(occurrence, operandEntity)
	if !siteOK || !occurrenceOK || !operandEntityOK || !operandOK || !batch.Seal() {
		t.Fatal("summary graph batch")
	}
	surface := equation.Surface{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadSummary, Local: 1, Semantic: coldKey(949_011).compositionKey(), Normalizer: coldKey(949_011).compositionKey()}
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{
		Batch:  batch,
		Rules:  []equation.RuleInstance{{Schema: schema.ruleSemanticAt(0), OperandFamily: unitOperandFamily.compositionKey(), Occurrence: occurrence, Operand: operand, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}}},
		Points: []equation.PointSpec{{Site: site}}, Groups: []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)}}, Queries: []equation.QueryInstance{{Family: schema.querySemanticAt(0), Point: equation.PointAt(0), Surfaces: []equation.Surface{surface}}},
	})
	if !topologyOK || topology == nil {
		t.Fatal("summary graph topology")
	}
	graph, graphOK := topology.Graph(nil)
	if !graphOK || graph == nil {
		t.Fatal("summary graph identity")
	}
	identity, identityOK := graph.QueryAt(0)
	if !identityOK {
		t.Fatal("summary graph query identity")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](binding, factor, form,
		func(value OrderedCells[uint64]) OrderedCells[uint64] { return value },
		func(left, right OrderedCells[uint64]) bool {
			return equalOrderedCellRecords(left.record, right.record, func(left, right uint64) bool { return left == right })
		},
		func(value OrderedCells[uint64]) uint64 { return uint64(value.Count()) }) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, receiptExactQueryRuleSpec()) || !BindSummaryQuery(binding, query, factor, form, hotSummaryQuerySpec()) || !binding.Seal() {
		t.Fatal("summary graph binding")
	}
	implementation, implementationOK := SummaryQueryImplementationAt[uint64, uint64](binding, query)
	compilation, compiled := compileReceiptFactors(binding, graph)
	runtime, joined := bindReceiptSummaryQuery[uint64, uint64](compilation, implementation, identity)
	if !implementationOK || !compiled || !joined || runtime == nil || runtime.query().Key() != identity.Key() || runtime.surface.Form != equation.SurfaceReadSummary || runtime.surface.Semantic != coldKey(949_011).compositionKey() {
		t.Fatal("summary graph evidence join")
	}
}
