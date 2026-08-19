package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

type receiptQueryMatrixFixture struct {
	solver       *Solver
	queries      []ProgramQuery
	expected     []uint64
	schemaID     CompositionID
	topologyKey  composition.Key
	transferRuns *int
	projectRuns  *int
	freezeRuns   *int
	// binding, graph and addressed are the construction inputs the sealed
	// program was built from. They are retained so a construction-plane law can
	// resolve the published query set the seal resolved against instead of
	// reconstructing one.
	binding              *SchemaBinding
	graph                *CommittedProgram
	addressed            []composition.Key
	queryImplementations []*ExactQueryImplementation[uint64, uint64]
	// observations are the solve-local observation handles attached before the
	// seal, in attach order, and observationIDs their identities in the same
	// order. Both are empty unless the fixture was built observed.
	observations   []receiptObservation[uint64]
	observationIDs []identity.ContentID
	// declaration is the same sealed program stated as one geometry
	// declaration, for the transitional law that folds it through the pure
	// constructor and measures the result against this committed geometry.
	declaration topologyDeclaration
}

// TestReceiptQueryWarmStateAndCancellation keeps both terminal lifecycle
// guarantees on the same receipt-native query authority: a warm state remains
// readable without rerunning transfers, while cancellation publishes no row.
func TestReceiptQueryWarmStateAndCancellation(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, []int{0, 1, 2, 3}, []int{0, 1, 2, 3})
	first, status, report := fixture.solver.SolveWithReport(context.Background())
	if first == nil || status != SolveComplete {
		t.Fatalf("first receipt solve = state:%v status:%v report={available:%t reason:%v phase:%v point:%v group:%v member:%v rule:%v}", first, status, report.Available(), report.Reason(), report.Failure(), report.Point(), report.Group(), report.Member(), report.Rule())
	}
	firstKey, firstKeyed := fixture.queries[0].PublicationKey()
	if !firstKeyed {
		t.Fatal("receipt query has no snapshot key")
	}
	for index, query := range fixture.queries {
		key, keyed := query.PublicationKey()
		if !keyed {
			t.Fatalf("query[%d] has no snapshot key", index)
		}
		value, readable := testSnapshotQueryValue[uint64](fixture.solver, first, key)
		if !readable || value != fixture.expected[index] {
			t.Fatalf("first query[%d] = %d/%v, want %d/true", index, value, readable, fixture.expected[index])
		}
	}
	runs := *fixture.transferRuns
	projects := *fixture.projectRuns
	freezes := *fixture.freezeRuns
	second, status := fixture.solver.Solve(context.Background())
	if second == nil || status != SolveComplete || *fixture.transferRuns != runs || *fixture.projectRuns != projects || *fixture.freezeRuns != freezes {
		t.Fatalf("warm receipt solve reran callbacks: state:%v status:%v transfers:%d/%d projects:%d/%d freezes:%d/%d", second, status, *fixture.transferRuns, runs, *fixture.projectRuns, projects, *fixture.freezeRuns, freezes)
	}
	for index, query := range fixture.queries {
		key, keyed := query.PublicationKey()
		if !keyed {
			t.Fatalf("query[%d] has no snapshot key", index)
		}
		oldValue, oldReadable := testSnapshotQueryValue[uint64](fixture.solver, first, key)
		newValue, newReadable := testSnapshotQueryValue[uint64](fixture.solver, second, key)
		if !oldReadable || !newReadable || oldValue != fixture.expected[index] || newValue != oldValue {
			t.Fatalf("warm query[%d] old=%d/%v new=%d/%v", index, oldValue, oldReadable, newValue, newReadable)
		}
	}
	allocations := testing.AllocsPerRun(100, func() {
		state, warmStatus := fixture.solver.Solve(context.Background())
		if state == nil || warmStatus != SolveComplete {
			panic("warm receipt solve")
		}
		if _, readable := testSnapshotQueryValue[uint64](fixture.solver, state, firstKey); !readable {
			panic("warm receipt query")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm receipt solve allocations = %v, want 0", allocations)
	}

	canceled := newReceiptQueryMatrixFixture(t, 4, []int{0, 1, 2, 3}, []int{0, 1, 2, 3})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state, canceledStatus := canceled.solver.Solve(ctx)
	if state != nil || canceledStatus != SolveCanceled {
		t.Fatalf("canceled receipt solve = state:%v status:%v", state, canceledStatus)
	}
	if _, readable := testSnapshotQueryValue[uint64](canceled.solver, state, firstKey); readable {
		t.Fatal("canceled receipt solve exposed a query result")
	}
}

func newReceiptQueryMatrixFixture(t testing.TB, count int, order, declarationOrder []int) receiptQueryMatrixFixture {
	t.Helper()
	return buildReceiptQueryMatrixFixture(t, count, order, declarationOrder, false)
}

// newObservedReceiptQueryMatrixFixture builds the same matrix and additionally
// attaches one exact observation per Rule member before the seal, so the
// observation table of the sealed program carries one row per member in attach
// order.
func newObservedReceiptQueryMatrixFixture(t testing.TB, count int, order, declarationOrder []int) receiptQueryMatrixFixture {
	t.Helper()
	return buildReceiptQueryMatrixFixture(t, count, order, declarationOrder, true)
}

func buildReceiptQueryMatrixFixture(t testing.TB, count int, order, declarationOrder []int, observed bool) receiptQueryMatrixFixture {
	t.Helper()
	if !validReceiptQueryPermutation(count, order) || !validReceiptQueryPermutation(count, declarationOrder) {
		t.Fatal("receipt query matrix order")
	}
	builder := NewSchema()
	factors := make([]*FactorSlot[uint64], count)
	reads := make([]SchemaReadForm[uint64], count)
	writes := make([]SchemaWriteForm[uint64], count)
	writeSlots := make([]SchemaWriteSlot[uint64], count)
	rules := make([]*RuleSlot[uint64, ruleUnit], count)
	queries := make([]*QuerySlot[uint64], count)
	for _, index := range declarationOrder {
		factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(951_000+index))
		write, writeOK := factor.ExactWrite()
		read, readOK := factor.ExactRead()
		if !factorOK || !writeOK || !readOK {
			t.Fatal("receipt query matrix Factor declaration")
		}
		factors[index], reads[index], writes[index] = factor, read, write
	}
	for _, index := range declarationOrder {
		rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
			Semantic: coldKey(952_000 + index), OperandFamily: unitOperandFamily, Inputs: 0,
			Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(952_100 + index)}, Output: factors[index].Ref(),
		})
		writeOK := false
		if ruleOK {
			writeSlots[index], writeOK = SchemaWrite(rule, writes[index])
		}
		if !ruleOK || !writeOK {
			t.Fatal("receipt query matrix Rule declaration")
		}
		rules[index] = rule
	}
	for _, index := range declarationOrder {
		query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(953_000 + index), Freezer: coldKey(953_100 + index)})
		if !queryOK || !SchemaQueryRead(query, reads[index]) {
			t.Fatal("receipt query matrix Query declaration")
		}
		queries[index] = query
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("receipt query matrix schema seal")
	}

	binding := NewSchemaBinding(schema)
	transferRuns := new(int)
	projectRuns := new(int)
	freezeRuns := new(int)
	if binding == nil {
		t.Fatal("receipt query matrix binding")
	}
	for index := 0; index < count; index++ {
		if !BindFactor(binding, factors[index], hotUintFactorSpec()) {
			t.Fatal("receipt query matrix Factor binding")
		}
		value := uint64(index + 1)
		ruleSpec := HotRuleSpec[uint64, ruleUnit]{
			OperandContent: ruleUnitContent,
			Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(952_100 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				*transferRuns++
				return Product(access, func(row Row) bool { return StageValue(access, row, value) })
			},
		}
		if !BindRule[uint64, uint64, ruleUnit](binding, rules[index], writeSlots[index], factors[index], ruleSpec, testRuleProjector[ruleUnit]) {
			t.Fatal("receipt query matrix Rule binding")
		}
	}
	for index := 0; index < count; index++ {
		spec := hotExactQuerySpec()
		spec.Result.Semantic = coldKey(953_100 + index)
		spec.Result.Freeze = func(value uint64) uint64 {
			*freezeRuns++
			return value
		}
		spec.Project = func(cells OrderedCells[uint64]) uint64 {
			*projectRuns++
			value, present, valid := cells.At(0)
			if !valid || !present {
				return 0
			}
			return value
		}
		if !BindExactQuery(binding, queries[index], factors[index], spec) {
			t.Fatal("receipt query matrix Query binding")
		}
	}
	if !binding.Seal() {
		t.Fatal("receipt query matrix binding seal")
	}

	ruleImplementations := make([]*RuleImplementation[uint64, uint64, ruleUnit], count)
	queryImplementations := make([]*ExactQueryImplementation[uint64, uint64], count)
	for index := 0; index < count; index++ {
		var ruleOK, queryOK bool
		ruleImplementations[index], ruleOK = RuleImplementationAt[uint64, uint64, ruleUnit](binding, rules[index])
		queryImplementations[index], queryOK = ExactQueryImplementationAt[uint64, uint64](binding, queries[index])
		if !ruleOK || ruleImplementations[index] == nil || !queryOK || queryImplementations[index] == nil {
			t.Fatal("receipt query matrix implementation receipt")
		}
	}
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !assemblyOK || assembly == nil {
		t.Fatal("receipt query matrix assembly")
	}
	sites := make([]equation.Site, count)
	occurrences := make([]equation.Occurrence, count)
	operands := make([]equation.Operand, count)
	operandValues := make([]ruleUnit, count)
	for index := 0; index < count; index++ {
		site, siteOK := assembly.admitSite(compositionKeyOf(coldKey(951_100+index)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := assembly.admitAt(site)
		value := ruleUnitForSemantic(coldKey(955_000 + index))
		entity, entityOK := operandEntityForContent(value.content)
		operand, operandOK := assembly.admitOperand(occurrence, entity)
		if !siteOK || !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("receipt query matrix source admission")
		}
		sites[index], occurrences[index], operands[index], operandValues[index] = site, occurrence, operand, value
	}
	if assembly.sealSources().Available() {
		t.Fatal("receipt query matrix source seal")
	}
	pointRank := make([]int, count)
	declaration := topologyDeclaration{binding: binding, batch: assembly.inner.batch}
	for rank, index := range order {
		declaration.points = append(declaration.points, declaredPointRow{ID: receiptAssemblySemanticID(byte(10 + index)), Site: sites[index]})
		pointRank[index] = rank
	}
	for _, index := range order {
		proof := ruleImplementations[index].binding.proof
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index], Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}})
		draft, draftOK := ruleImplementations[index].beginBindingRuleRow(source)
		part, partOK := ruleImplementations[index].WritePart(source, 0)
		rowOK := sourceOK && draftOK && partOK && draft.AddWrite(part)
		row, issued := assembly.issueRuleRow(draft)
		if !rowOK || !issued {
			t.Fatal("receipt query matrix Rule topology")
		}
		declaration.members = append(declaration.members, declaredMemberRow{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(byte(60 + index)), Row: row.row})
	}
	for _, index := range order {
		queryOrdinal, queryOrdinalOK := queries[index].Ordinal()
		factorOrdinal, factorOrdinalOK := factors[index].Ordinal()
		if !queryOrdinalOK || !factorOrdinalOK {
			t.Fatal("receipt query matrix ordinal mapping")
		}
		instance := equation.QueryInstance{Family: schema.querySemanticAt(queryOrdinal), Point: equation.PointAt(pointRank[index]), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(factorOrdinal), Form: equation.SurfaceReadExact, Local: 1}}}
		declaration.queries = append(declaration.queries, declaredQueryRow{ID: receiptAssemblySemanticID(byte(110 + index)), Row: instance})
	}
	constructed, refusal := constructTopology(declaration)
	topology, graph, committed := constructed.topology, constructed.graph, !refusal.Available() && constructed.Available()
	if !committed || topology == nil || graph == nil || topology.topology == nil {
		t.Fatalf("receipt query matrix graph commit stage=%v step=%v ordinal=%d", refusal.Stage(), refusal.Step(), refusal.Ordinal())
	}
	program := CommittedProgramFrom(topology, graph)
	if program == nil {
		t.Fatal("receipt query matrix committed program")
	}
	compilation, compilationOK := BeginProgramConstruction(binding, program)
	if !compilationOK || compilation == nil {
		t.Fatal("receipt query matrix compilation")
	}
	for index := 0; index < count; index++ {
		if _, memberOK := program.RuleMember(receiptAssemblySemanticID(byte(60 + index))); !memberOK {
			t.Fatal("receipt query matrix Rule member receipt")
		}
		if !installConstOperandResolver(ruleImplementations[index], operandValues[index]) {
			t.Fatal("receipt query matrix resolver")
		}
		if attached := AttachRuleMember(compilation, ruleImplementations[index], receiptAssemblySemanticID(byte(60+index))); !attached {
			t.Fatal("receipt query matrix Rule attachment")
		}
	}
	queryReceipts := make([]ProgramQuery, count)
	for index := 0; index < count; index++ {
		query, queryOK := program.Query(receiptAssemblySemanticID(byte(110 + index)))
		if !queryOK || !AttachExactQuery(compilation, queryImplementations[index], receiptAssemblySemanticID(byte(110+index))) {
			t.Fatal("receipt query matrix Query attachment")
		}
		queryReceipts[index] = query
	}
	var observations []receiptObservation[uint64]
	var observationIDs []identity.ContentID
	if observed {
		observations = make([]receiptObservation[uint64], 0, count)
		observationIDs = make([]identity.ContentID, 0, count)
		for _, index := range order {
			member, memberOK := program.RuleMember(receiptAssemblySemanticID(byte(60 + index)))
			if !memberOK {
				t.Fatal("receipt query matrix observation member")
			}
			id := receiptAssemblySemanticID(byte(160 + index))
			observation, failure := AttachRuleExactObservationWithFailure(compilation, queryImplementations[index], id, member)
			if failure != receiptObservationAttachFailureNone || !observation.Available() {
				t.Fatalf("receipt query matrix observation %d failure=%v", index, failure)
			}
			observations = append(observations, observation)
			observationIDs = append(observationIDs, id)
		}
	}
	addressed, addressedOK := program.publishedQueryKeys()
	if !addressedOK {
		t.Fatal("receipt query matrix published query addresses")
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil {
		t.Fatal("receipt query matrix Solver")
	}
	expected := make([]uint64, count)
	for index := range expected {
		expected[index] = uint64(index + 1)
	}
	return receiptQueryMatrixFixture{
		solver: solver, queries: queryReceipts, expected: expected, topologyKey: topology.topology.Key(),
		transferRuns: transferRuns, projectRuns: projectRuns, freezeRuns: freezeRuns, schemaID: schema.ID(),
		binding: binding, graph: program, addressed: addressed, queryImplementations: queryImplementations,
		observations: observations, observationIDs: observationIDs, declaration: declaration,
	}
}

func validReceiptQueryPermutation(count int, order []int) bool {
	if count <= 0 || len(order) != count {
		return false
	}
	seen := make([]bool, count)
	for _, index := range order {
		if index < 0 || index >= count || seen[index] {
			return false
		}
		seen[index] = true
	}
	return true
}

func receiptQueryPermutations(width int) [][]int {
	values := make([]int, width)
	for index := range values {
		values[index] = index
	}
	result := make([][]int, 0)
	var visit func(int)
	visit = func(offset int) {
		if offset == len(values) {
			result = append(result, append([]int(nil), values...))
			return
		}
		for index := offset; index < len(values); index++ {
			values[offset], values[index] = values[index], values[offset]
			visit(offset + 1)
			values[offset], values[index] = values[index], values[offset]
		}
	}
	visit(0)
	return result
}
