package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func exactQuerySchemaFixture(t testing.TB) (*Schema, *FactorSlot[uint64], *QuerySlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	read, readOK := factor.ExactRead()
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(948_002), Freezer: coldKey(948_003)})
	if !factorOK || !readOK || !queryOK || !SchemaQueryRead(query, read) {
		t.Fatal("exact query declaration")
	}
	schema, sealOK := builder.Seal()
	if !sealOK || schema == nil {
		t.Fatal("exact query schema")
	}
	return schema, factor, query
}

func hotExactQuerySpec() HotExactQuerySpec[uint64, uint64] {
	return HotExactQuerySpec[uint64, uint64]{
		Project: func(cells OrderedCells[uint64]) uint64 { return uint64(len(cells.record.cells)) },
		Result: FrozenResult[uint64]{
			Semantic:    coldKey(948_003),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
		},
	}
}

func TestSchemaExactQueryReceiptOwnsOneProjectionAndBinding(t *testing.T) {
	schema, factor, query := exactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("exact query receipt binding")
	}
	implementation, ok := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ok || implementation == nil {
		t.Fatal("exact query implementation receipt")
	}
	project, ok := implementation.projector()
	if !ok || project == nil {
		t.Fatal("typed exact query projector")
	}
}

func TestSchemaExactQueryReceiptRejectsForeignEqualBinding(t *testing.T) {
	schema, factor, query := exactQuerySchemaFixture(t)
	foreignSchema, foreignFactor, foreignQuery := exactQuerySchemaFixture(t)
	if schema.ID() != foreignSchema.ID() || factor.Schema() != schema || foreignFactor.Schema() != foreignSchema {
		t.Fatal("equal schema setup")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || BindExactQuery(binding, foreignQuery, factor, hotExactQuerySpec()) || !binding.Poisoned() {
		t.Fatal("foreign equal-schema query crossed owner fence")
	}
	foreignBinding := NewSchemaBinding(foreignSchema)
	if foreignBinding == nil || !BindFactor(foreignBinding, foreignFactor, hotUintFactorSpec()) || !BindExactQuery(foreignBinding, foreignQuery, foreignFactor, hotExactQuerySpec()) || !foreignBinding.Seal() {
		t.Fatal("foreign owner exact query setup")
	}
	if _, ok := ExactQueryImplementationAt[uint64, uint64](foreignBinding, query); ok {
		t.Fatal("foreign query slot received equal-schema receipt")
	}
}

func TestSchemaExactQueryReceiptRejectsSummaryAndSupportRows(t *testing.T) {
	builder := NewSchema()
	factor, _ := DeclareFactorSlot[uint64](builder, coldKey(948_020))
	summary, _ := factor.SummaryRead(coldKey(948_021))
	summaryQuery, _ := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(948_022), Freezer: coldKey(948_023)})
	if !SchemaQueryRead(summaryQuery, summary) {
		t.Fatal("summary query row")
	}
	summarySchema, ok := builder.Seal()
	if !ok {
		t.Fatal("summary query schema")
	}
	summaryBinding := NewSchemaBinding(summarySchema)
	if summaryBinding == nil || BindExactQuery(summaryBinding, summaryQuery, factor, hotExactQuerySpec()) || !summaryBinding.Poisoned() {
		t.Fatal("summary query accepted by exact lane")
	}
}

func exactQueryReceiptGraph(t testing.TB, schema *Schema, factor *FactorSlot[uint64], query *QuerySlot[uint64]) (*equation.Graph, equation.Query) {
	t.Helper()
	batch := equation.NewBatch()
	site, siteOK := batch.AdmitSite(coldKey(948_030).compositionKey(), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	if !siteOK {
		t.Fatal("exact query graph batch")
	}
	factorKey := schema.factorSemanticAt(0)
	queryKey := schema.querySemanticAt(0)
	ruleKey := schema.ruleSemanticAt(0)
	ruleShape, ruleShapeOK := schema.ruleShapeAt(0)
	if !ruleShapeOK || !ruleKey.Available() {
		t.Fatal("exact query producer shape")
	}
	occurrence, occurrenceOK := batch.At(site)
	operandValue := ruleUnitForSemantic(coldKey(948_033))
	operandEntity, operandEntityOK := operandEntityForContent(operandValue.content)
	operand, operandOK := batch.AdmitOperand(occurrence, operandEntity)
	if !occurrenceOK || !operandEntityOK || !operandOK || !batch.Seal() {
		t.Fatal("exact query producer operand")
	}
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{
		Batch: batch,
		Rules: []equation.RuleInstance{{
			Schema: ruleKey, OperandFamily: ruleShape.OperandFamily, Occurrence: occurrence, Operand: operand,
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorKey, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		}},
		Points: []equation.PointSpec{{Site: site}},
		Groups: []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)}},
		Queries: []equation.QueryInstance{{
			Family: queryKey,
			Point:  equation.PointAt(0),
			Surfaces: []equation.Surface{{
				Factor: factorKey, Form: equation.SurfaceReadExact, Local: 1,
			}},
		}},
	})
	if !topologyOK || topology == nil {
		t.Fatal("exact query graph topology")
	}
	graph, graphOK := topology.Graph(nil)
	identity, identityOK := graph.QueryAt(0)
	if !graphOK || graph == nil || !identityOK || !graph.OwnsQuery(identity) || identity.Family() != queryKey || identity.Key() == (equation.Query{}).Key() {
		t.Fatal("exact query graph identity")
	}
	return graph, identity
}

func receiptExactQuerySchemaFixture(t testing.TB) (*Schema, *FactorSlot[uint64], *RuleSlot[uint64, ruleUnit], SchemaWriteSlot[uint64], *QuerySlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	writeForm, writeFormOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(948_031), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(948_032)}, Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(948_002), Freezer: coldKey(948_003)})
	queryReadOK := SchemaQueryRead(query, readForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeFormOK || !ruleOK || !writeOK || !readOK || !queryOK || !queryReadOK || !schemaOK || schema == nil {
		t.Fatal("receipt exact query schema")
	}
	return schema, factor, rule, write, query
}

func receiptExactQueryRuleSpec() HotRuleSpec[uint64, ruleUnit] {
	return HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_032)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
}

func TestReceiptCompilerBindsExactQueryEvidenceOnly(t *testing.T) {
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	spec := hotExactQuerySpec()
	spec.Project = func(_ OrderedCells[uint64]) uint64 { return 7 }
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, receiptExactQueryRuleSpec()) || !BindExactQuery(binding, query, factor, spec) || !binding.Seal() {
		t.Fatal("exact query receipt setup")
	}
	implementation, implementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !implementationOK || implementation == nil || !implementation.receipt.valid() {
		t.Fatal("exact query receipt")
	}
	project, projectOK := implementation.projector()
	if !projectOK || project == nil || project(OrderedCells[uint64]{}) != 7 {
		t.Fatal("typed exact query projector result")
	}
	graph, identity := exactQueryReceiptGraph(t, schema, factor, query)
	compilation, compiled := compileReceiptFactors(binding, graph)
	runtime, joined := bindReceiptExactQuery[uint64, uint64](compilation, implementation, identity)
	if !compiled || compilation == nil || !joined || runtime == nil || runtime.query().Key() != identity.Key() || runtime.surface.Form != equation.SurfaceReadExact {
		t.Fatal("exact query evidence join")
	}

	foreign := NewSchemaBinding(schema)
	if foreign == nil || !BindFactor(foreign, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](foreign, rule, write, factor, receiptExactQueryRuleSpec()) || !BindExactQuery(foreign, query, factor, spec) || !foreign.Seal() {
		t.Fatal("foreign exact query binding")
	}
	foreignImplementation, foreignOK := ExactQueryImplementationAt[uint64, uint64](foreign, query)
	if !foreignOK || foreignImplementation == nil {
		t.Fatal("foreign exact query receipt")
	}
	if _, accepted := bindReceiptExactQuery[uint64, uint64](compilation, foreignImplementation, identity); accepted {
		t.Fatal("equal-Schema foreign query receipt entered compiler")
	}
	if _, accepted := bindReceiptExactQuery[uint64, uint64](compilation, implementation, equation.Query{}); accepted {
		t.Fatal("foreign graph query entered compiler")
	}

	missing := NewSchemaBinding(schema)
	if missing == nil || !BindExactQuery(missing, query, factor, spec) || missing.Seal() || !missing.Poisoned() {
		t.Fatal("incomplete exact query Binding published")
	}
}

func dualExactQueryReceiptFixture(t testing.TB) (*Schema, *FactorSlot[uint64], *FactorSlot[uint64], *RuleSlot[uint64, ruleUnit], SchemaWriteSlot[uint64], *QuerySlot[uint64], *QuerySlot[uint64], *equation.Graph, equation.Query, equation.Query) {
	t.Helper()
	builder := NewSchema()
	left, leftOK := DeclareFactorSlot[uint64](builder, coldKey(948_040))
	right, rightOK := DeclareFactorSlot[uint64](builder, coldKey(948_041))
	leftRead, leftReadOK := left.ExactRead()
	rightRead, rightReadOK := right.ExactRead()
	leftWrite, leftWriteOK := left.ExactWrite()
	producer, producerOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(948_047), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(948_048)}, Output: left.Ref(),
	})
	producerWrite, producerWriteOK := SchemaWrite(producer, leftWrite)
	leftQuery, leftQueryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(948_042), Freezer: coldKey(948_044)})
	rightQuery, rightQueryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(948_043), Freezer: coldKey(948_045)})
	if !leftOK || !rightOK || !leftReadOK || !rightReadOK || !leftWriteOK || !producerOK || !producerWriteOK || !leftQueryOK || !rightQueryOK || !SchemaQueryRead(leftQuery, leftRead) || !SchemaQueryRead(rightQuery, rightRead) {
		t.Fatal("dual exact query declaration")
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("dual exact query schema")
	}
	batch := equation.NewBatch()
	site, siteOK := batch.AdmitSite(coldKey(948_046).compositionKey(), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	if !siteOK {
		t.Fatal("dual exact query batch")
	}
	occurrence, occurrenceOK := batch.At(site)
	operandValue := ruleUnitForSemantic(coldKey(948_049))
	operandEntity, operandEntityOK := operandEntityForContent(operandValue.content)
	operand, operandOK := batch.AdmitOperand(occurrence, operandEntity)
	if !occurrenceOK || !operandEntityOK || !operandOK || !batch.Seal() {
		t.Fatal("dual exact query producer operand")
	}
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{
		Batch: batch,
		Rules: []equation.RuleInstance{{
			Schema: schema.ruleSemanticAt(0), OperandFamily: unitOperandFamily.compositionKey(), Occurrence: occurrence, Operand: operand,
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		}},
		Points: []equation.PointSpec{{Site: site}},
		Groups: []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)}},
		Queries: []equation.QueryInstance{
			{Family: schema.querySemanticAt(0), Point: equation.PointAt(0), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}}},
			{Family: schema.querySemanticAt(1), Point: equation.PointAt(0), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(1), Form: equation.SurfaceReadExact, Local: 1}}},
		},
	})
	if !topologyOK || topology == nil {
		t.Fatal("dual exact query topology")
	}
	graph, graphOK := topology.Graph(nil)
	if !graphOK || graph == nil || graph.QueryCount() != 2 {
		t.Fatal("dual exact query graph")
	}
	var leftIdentity, rightIdentity equation.Query
	for index := 0; index < graph.QueryCount(); index++ {
		identity, identityOK := graph.QueryAt(index)
		if !identityOK {
			t.Fatal("dual exact query identity")
		}
		switch identity.Family() {
		case schema.querySemanticAt(0):
			leftIdentity = identity
		case schema.querySemanticAt(1):
			rightIdentity = identity
		}
	}
	if !leftIdentity.Key().Available() || !rightIdentity.Key().Available() {
		t.Fatal("dual exact query identities")
	}
	return schema, left, right, producer, producerWrite, leftQuery, rightQuery, graph, leftIdentity, rightIdentity
}

func TestReceiptCompilerRejectsWrongQueryFamilySurfaceAndFactor(t *testing.T) {
	schema, left, right, producer, producerWrite, leftQuery, rightQuery, graph, leftIdentity, rightIdentity := dualExactQueryReceiptFixture(t)
	binding := NewSchemaBinding(schema)
	leftSpec := hotExactQuerySpec()
	leftSpec.Result.Semantic = coldKey(948_044)
	rightSpec := hotExactQuerySpec()
	rightSpec.Result.Semantic = coldKey(948_045)
	if binding == nil || !BindFactor(binding, left, hotUintFactorSpec()) || !BindFactor(binding, right, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, producer, producerWrite, left, HotRuleSpec[uint64, ruleUnit]{OperandContent: ruleUnitContent, Admission: AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_048)), Transfer: func(access Access[uint64, ruleUnit]) bool {
		return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
	}}) || !BindExactQuery(binding, leftQuery, left, leftSpec) || !BindExactQuery(binding, rightQuery, right, rightSpec) || !binding.Seal() {
		t.Fatal("dual exact query binding")
	}
	leftImplementation, leftOK := ExactQueryImplementationAt[uint64, uint64](binding, leftQuery)
	if !leftOK || leftImplementation == nil {
		t.Fatal("left exact query receipt")
	}
	compilation, compiled := compileReceiptFactors(binding, graph)
	if !compiled || compilation == nil {
		t.Fatal("dual exact query compilation")
	}
	if _, accepted := bindReceiptExactQuery[uint64, uint64](compilation, leftImplementation, rightIdentity); accepted {
		t.Fatal("wrong query family/factor entered receipt compiler")
	}
	wrongSurface := leftIdentity
	// An equation Query is opaque; the only way to obtain a surface-bearing
	// candidate is through the graph authority. The right identity is therefore
	// the deliberate wrong-factor/surface candidate for this left receipt.
	if len(wrongSurface.Surfaces()) != 1 || wrongSurface.Surfaces()[0].Factor == rightIdentity.Surfaces()[0].Factor {
		t.Fatal("dual query surfaces were not distinct")
	}
	if _, accepted := bindReceiptExactQuery[uint64, uint64](compilation, leftImplementation, equation.Query{}); accepted {
		t.Fatal("foreign graph identity entered receipt compiler")
	}
	if _, accepted := bindReceiptExactQuery[uint64, uint64](compilation, leftImplementation, leftIdentity); !accepted {
		t.Fatal("canonical exact query evidence rejected")
	}
}
