// solve_law_test.go proves the solve diagnostics, publication generation and report laws.

package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

func TestSolveWithDiagnosticsDisabledParity(t *testing.T) {
	ordinary, ordinaryReceipt := newDiagnosticsReceiptSolver(t, false)
	ordinaryState, ordinaryStatus := ordinary.Solve(context.Background())

	diagnostic, diagnosticReceipt := newDiagnosticsReceiptSolver(t, false)
	diagnosticState, diagnosticStatus, report := diagnostic.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{})
	if ordinaryStatus != diagnosticStatus || (ordinaryState == nil) != (diagnosticState == nil) {
		t.Fatalf("disabled diagnostics changed solve parity: ordinary=%v/%t diagnostic=%v/%t", ordinaryStatus, ordinaryState != nil, diagnosticStatus, diagnosticState != nil)
	}
	if report.Flags != 0 || len(report.Rows) != 0 || report.DroppedRows != 0 || report.Failure.Available() {
		t.Fatalf("disabled diagnostics retained data: %#v", report)
	}
	ordinaryKey, ordinaryKeyed := ordinaryReceipt.PublicationKey()
	diagnosticKey, diagnosticKeyed := diagnosticReceipt.PublicationKey()
	if !ordinaryKeyed || !diagnosticKeyed {
		t.Fatal("diagnostic parity query has no snapshot key")
	}
	ordinaryValue, ordinaryReadable := testSnapshotQueryValue[uint64](ordinary, ordinaryState, ordinaryKey)
	diagnosticValue, diagnosticReadable := testSnapshotQueryValue[uint64](diagnostic, diagnosticState, diagnosticKey)
	if ordinaryReadable != diagnosticReadable || ordinaryReadable && ordinaryValue != diagnosticValue {
		t.Fatalf("disabled diagnostics changed query result: ordinary=%d/%t diagnostic=%d/%t", ordinaryValue, ordinaryReadable, diagnosticValue, diagnosticReadable)
	}
}

func TestSolveDiagnosticPresentationDoesNotChangeSnapshotContent(t *testing.T) {
	ordinary, _ := newDiagnosticsReceiptSolver(t, false)
	ordinaryState, ordinaryStatus := ordinary.Solve(context.Background())
	ordinarySnapshot, ordinaryOK := ordinary.PublishedSnapshot(ordinaryState)
	if ordinaryStatus != SolveComplete || !ordinaryOK || !ordinarySnapshot.Content().Available() {
		t.Fatalf("ordinary solve = status:%v snapshot:%t", ordinaryStatus, ordinaryOK)
	}
	presentations := []SolveDiagnosticPresentation{
		{},
		{Flags: SolveDiagnosticSchedule},
		{Flags: SolveDiagnosticRestart},
		{Flags: SolveDiagnosticPublication},
		{Flags: SolveDiagnosticFold},
		{Flags: SolveDiagnosticAll},
	}
	resources := []SolveDiagnosticResources{
		{},
		{MaxRows: 1},
		{MaxRows: 8},
		{MaxRows: maxSolveDiagnosticMaxRows},
	}
	for _, presentation := range presentations {
		for _, resource := range resources {
			options := SolveDiagnosticOptions{Presentation: presentation, Resources: resource}
			if presentation.Flags == 0 && resource.MaxRows != 0 {
				if options.Valid() {
					t.Fatalf("resource bound without presentation admitted: %#v", options)
				}
				continue
			}
			if !options.Valid() {
				t.Fatalf("valid diagnostic options rejected: %#v", options)
			}
			solver, _ := newDiagnosticsReceiptSolver(t, false)
			state, status, _ := solver.SolveWithDiagnostics(context.Background(), options)
			sealed, sealedOK := solver.PublishedSnapshot(state)
			if status != SolveComplete || !sealedOK {
				t.Fatalf("presentation solve failed: options=%#v status=%v snapshot=%t", options, status, sealedOK)
			}
			if sealed.Content() != ordinarySnapshot.Content() {
				t.Fatalf("presentation or resource settings changed Snapshot content: options=%#v ordinary=%v got=%v", options, ordinarySnapshot.Content(), sealed.Content())
			}
		}
	}
}

func TestSolveDiagnosticOptionsAreClosedAndRejectedBeforeExecution(t *testing.T) {
	valid := []SolveDiagnosticOptions{
		{},
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: maxSolveDiagnosticMaxRows}},
	}
	for _, options := range valid {
		if !options.Valid() {
			t.Fatalf("valid diagnostic options rejected: %#v", options)
		}
	}
	invalid := []SolveDiagnosticOptions{
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll << 1}},
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: -1}},
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: maxSolveDiagnosticMaxRows + 1}},
		{Resources: SolveDiagnosticResources{MaxRows: 1}},
	}
	for _, options := range invalid {
		if options.Valid() {
			t.Fatalf("invalid diagnostic options admitted: %#v", options)
		}
	}
	solver, _ := newDiagnosticsReceiptSolver(t, false)
	state, status, diagnostics := solver.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{Resources: SolveDiagnosticResources{MaxRows: 1}})
	if state != nil || status != SolveInvalid || diagnostics.Failure.Available() || diagnostics.Flags != 0 {
		t.Fatalf("invalid options executed solver: state:%t status:%v diagnostics:%#v", state != nil, status, diagnostics)
	}
}

func TestSolveWithDiagnosticsCarriesSameIncompleteFailureCertificate(t *testing.T) {
	reported, _ := newDiagnosticsReceiptSolver(t, true)
	reportedState, reportedStatus, want := reported.SolveWithReport(context.Background())
	if reportedState != nil || reportedStatus != SolveIncomplete || !want.Available() {
		t.Fatalf("report fixture = state:%t status:%v available:%t", reportedState != nil, reportedStatus, want.Available())
	}

	diagnostic, _ := newDiagnosticsReceiptSolver(t, true)
	diagnosticState, diagnosticStatus, got := diagnostic.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{})
	if diagnosticState != nil || diagnosticStatus != SolveIncomplete || got.Flags != 0 || !got.Failure.Available() || !sameSolveReport(got.Failure, want) {
		t.Fatalf("diagnostic failure = state:%t status:%v got:%#v want:%#v", diagnosticState != nil, diagnosticStatus, got.Failure, want)
	}

	complete, _ := newDiagnosticsReceiptSolver(t, false)
	completeState, completeStatus, completeDiagnostics := complete.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: 8}})
	if completeState == nil || completeStatus != SolveComplete || completeDiagnostics.Failure.Available() {
		t.Fatalf("complete failure certificate = state:%t status:%v available:%t", completeState != nil, completeStatus, completeDiagnostics.Failure.Available())
	}

	canceled, _ := newDiagnosticsReceiptSolver(t, false)
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	canceledState, canceledStatus, canceledDiagnostics := canceled.SolveWithDiagnostics(canceledContext, SolveDiagnosticOptions{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: 8}})
	if canceledState != nil || canceledStatus != SolveCanceled || canceledDiagnostics.Failure.Available() {
		t.Fatalf("canceled failure certificate = state:%t status:%v available:%t", canceledState != nil, canceledStatus, canceledDiagnostics.Failure.Available())
	}
}

func sameSolveReport(left, right SolveReport) bool {
	return left.Reason() == right.Reason() && left.Failure() == right.Failure() &&
		left.Point() == right.Point() && left.Group() == right.Group() &&
		left.Member() == right.Member() && left.Rule() == right.Rule()
}

func TestSolveWithDiagnosticsBoundedDetachedSortedAndIsolated(t *testing.T) {
	// Rows only arise after an established Region observes a changed external
	// interface. This receipt-native epoch performs exactly that revision.
	report := collectDiagnosticsExternalInterfaceRows(t, 8)
	if report.DroppedRows != 0 || len(report.Rows) != 2 {
		t.Fatalf("external interface revision did not expose sortable diagnostics: rows=%d dropped=%d", len(report.Rows), report.DroppedRows)
	}
	if report.Rows[0].Site == report.Rows[1].Site {
		t.Fatalf("external interface revision did not retain distinct recurrence identities: first=%#v second=%#v", report.Rows[0], report.Rows[1])
	}
	for _, row := range report.Rows {
		if row.Kind != SolveDiagnosticKindRestart || row.callSite != solveDiagnosticRestartHeadInterface || row.reason != solveDiagnosticRestartInterfaceChanged {
			t.Fatalf("external interface revision row has wrong cause: %#v", row)
		}
	}
	for index := 1; index < len(report.Rows); index++ {
		if diagnosticRowLess(report.Rows[index], report.Rows[index-1]) {
			t.Fatalf("diagnostic rows are not sorted: previous=%#v current=%#v", report.Rows[index-1], report.Rows[index])
		}
	}
	assertSolveDiagnosticCollectorSortAndDetach(t)

	boundedReport := collectDiagnosticsExternalInterfaceRows(t, 1)
	if len(boundedReport.Rows) != 1 || boundedReport.DroppedRows != 1 {
		t.Fatalf("diagnostic row bound did not retain one/drop later rows: rows=%d dropped=%d", len(boundedReport.Rows), boundedReport.DroppedRows)
	}
	first, firstReceipt := newDiagnosticsReceiptSolver(t, false)
	firstState, firstStatus := first.Solve(context.Background())
	second, secondReceipt := newDiagnosticsReceiptSolver(t, false)
	secondState, secondStatus := second.Solve(context.Background())
	if firstStatus != SolveComplete || secondStatus != SolveComplete || firstState == nil || secondState == nil {
		t.Fatalf("receipt ownership solve failed: first=%v/%t second=%v/%t", firstStatus, firstState != nil, secondStatus, secondState != nil)
	}
	firstKey, firstKeyed := firstReceipt.PublicationKey()
	secondKey, secondKeyed := secondReceipt.PublicationKey()
	if !firstKeyed || !secondKeyed {
		t.Fatal("receipt ownership query has no snapshot key")
	}
	if _, readable := testSnapshotQueryValue[uint64](first, secondState, firstKey); readable {
		t.Fatal("first receipt read a foreign complete solver state")
	}
	if _, readable := testSnapshotQueryValue[uint64](second, firstState, secondKey); readable {
		t.Fatal("second receipt read a foreign complete solver state")
	}

	secondReport := collectDiagnosticsExternalInterfaceRows(t, 8)
	if secondReport.DroppedRows != 0 || len(secondReport.Rows) != 2 {
		t.Fatalf("independent external-interface receipt lost diagnostics: rows=%d dropped=%d", len(secondReport.Rows), secondReport.DroppedRows)
	}
	if !reflect.DeepEqual(report.Rows, secondReport.Rows) {
		t.Fatalf("independent external-interface receipts produced different diagnostics: first=%#v second=%#v", report.Rows, secondReport.Rows)
	}
}

// assertSolveDiagnosticCollectorSortAndDetach isolates the collector contract
// from executor scheduling: rows are deliberately inserted in reverse
// canonical-key order, then a caller mutation of snapshot A must not alter a
// later snapshot B from that very collector. The receipt-native law above
// separately proves that executor interface refreshes produce the rows.
func assertSolveDiagnosticCollectorSortAndDetach(t testing.TB) {
	t.Helper()
	collector := newSolveDiagnosticState(SolveDiagnosticOptions{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticRestart}, Resources: SolveDiagnosticResources{MaxRows: 2}})
	if collector == nil {
		t.Fatal("diagnostic collector")
	}
	first := solveDiagnosticRestartSample{
		key: solveDiagnosticRowKey{
			revision: 4, kind: SolveDiagnosticKindRestart,
			callSite: solveDiagnosticRestartHeadInterface, reason: solveDiagnosticRestartInterfaceChanged,
			phase: solveDiagnosticRegionAscent, region: 2, head: 3,
		},
		attempts: 5,
	}
	second := solveDiagnosticRestartSample{
		key: solveDiagnosticRowKey{
			revision: 4, kind: SolveDiagnosticKindRestart,
			callSite: solveDiagnosticRestartHeadInterface, reason: solveDiagnosticRestartInterfaceChanged,
			phase: solveDiagnosticRegionAscent, region: 9, head: 11,
		},
		attempts: 13,
	}
	// Insert the later key first. This proves snapshot ordering rather than
	// relying on executor/WTO dequeue order.
	collector.finishRestart(second, true)
	collector.finishRestart(first, true)
	snapshotA := collector.snapshot()
	if len(snapshotA.Rows) != 2 || !diagnosticRowLess(snapshotA.Rows[0], snapshotA.Rows[1]) ||
		snapshotA.Rows[0].region != first.key.region || snapshotA.Rows[0].head != first.key.head ||
		snapshotA.Rows[1].region != second.key.region || snapshotA.Rows[1].head != second.key.head {
		t.Fatalf("collector did not canonically sort reverse insertion: %#v", snapshotA.Rows)
	}
	if snapshotA.Rows[0].Attempts == 0 {
		t.Fatalf("collector snapshot lacks nonzero attempt evidence: %#v", snapshotA.Rows[0])
	}
	wantAttempts := snapshotA.Rows[0].Attempts
	snapshotA.Rows[0].Attempts = 0
	snapshotB := collector.snapshot()
	if len(snapshotB.Rows) != 2 || snapshotB.Rows[0].Attempts != wantAttempts {
		t.Fatalf("collector snapshot B retained caller mutation or lost attempt evidence: A=%#v B=%#v", snapshotA.Rows, snapshotB.Rows)
	}
}

func diagnosticRowLess(left, right SolveDiagnosticRow) bool {
	if left.Revision != right.Revision {
		return left.Revision < right.Revision
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.callSite != right.callSite {
		return left.callSite < right.callSite
	}
	if left.reason != right.reason {
		return left.reason < right.reason
	}
	if left.phase != right.phase {
		return left.phase < right.phase
	}
	if left.region != right.region {
		return left.region < right.region
	}
	return left.head < right.head
}

// newDiagnosticsReceiptSolver keeps this law suite on the only supported
// solver construction route: a sealed SchemaBinding, committed program,
// and typed Rule/Query attachment transaction. A failing transfer gives the
// incomplete-certificate laws a real execution boundary without restoring the
// retired synthetic admission fixture.
func newDiagnosticsReceiptSolver(t testing.TB, failTransfer bool) (*Solver, ProgramQuery) {
	t.Helper()
	querySpec := hotExactQuerySpec()
	querySpec.Project = func(cells OrderedCells[uint64]) uint64 { return uint64(len(cells.record.cells)) }
	return newDiagnosticsReceiptSolverOf(t, failTransfer, querySpec)
}

// newDiagnosticsReceiptSolverOf is the same solver over an arbitrary declared
// query result type, so a law can read a published result whose value carries a
// mutable backing store.
func newDiagnosticsReceiptSolverOf[R any](t testing.TB, failTransfer bool, querySpec HotExactQuerySpec[uint64, R]) (*Solver, ProgramQuery) {
	t.Helper()
	schema, factor, rule, write, query := receiptExactQuerySchemaFixtureOf[R](t, querySpec.Result.Semantic)
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_032)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			if failTransfer {
				return false
			}
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, factor, querySpec) || !binding.Seal() {
		t.Fatal("diagnostics receipt binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, R](binding, query)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || implementation == nil || !queryImplementationOK || queryImplementation == nil || !assemblyOK || assembly == nil {
		t.Fatal("diagnostics receipt assembly")
	}

	proof := implementation.binding.proof
	scope := equation.EmptyScope()
	sites := make([]equation.Site, 1)
	occurrences := make([]equation.Occurrence, 1)
	operands := make([]equation.Operand, 1)
	operandValues := make([]ruleUnit, 1)
	for index := range sites {
		site, siteOK := assembly.admitSite(compositionKeyOf(coldKey(949_700+index)), scope, equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := assembly.admitAt(site)
		operandValue := ruleUnitForSemantic(coldKey(949_710 + index))
		entity, entityOK := operandEntityForContent(operandValue.content)
		operand, operandOK := assembly.admitOperand(occurrence, entity)
		if !siteOK || !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("diagnostics receipt source")
		}
		sites[index], occurrences[index], operands[index], operandValues[index] = site, occurrence, operand, operandValue
	}
	if !assembly.SealSources() {
		t.Fatal("diagnostics receipt source seal")
	}
	declaration := topologyDeclaration{binding: binding, batch: assembly.inner.batch}
	ruleIDs := make([]byte, 1)
	for index := range sites {
		pointID := receiptAssemblySemanticID(byte(70 + index*2))
		ruleID := receiptAssemblySemanticID(byte(71 + index*2))
		declaration.points = append(declaration.points, declaredPointRow{ID: pointID, Site: sites[index]})
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.beginBindingRuleRow(source)
		part, partOK := implementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
			t.Fatal("diagnostics receipt rule row")
		}
		ruleRow, ruleRowOK := assembly.issueRuleRow(draft)
		if !ruleRowOK {
			t.Fatal("diagnostics receipt topology")
		}
		declaration.members = append(declaration.members, declaredMemberRow{Plane: declaredMemberOwner, ID: ruleID, Row: ruleRow.row})
		ruleIDs[index] = byte(71 + index*2)
	}
	declaration.queries = append(declaration.queries, declaredQueryRow{
		ID: receiptAssemblySemanticID(80),
		Row: equation.QueryInstance{
			Family: schema.querySemanticAt(0), Point: equation.PointAt(0),
			Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}},
		},
	})
	constructed, refusal := constructTopology(declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("diagnostics receipt commit stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("diagnostics receipt committed program")
	}
	queryReceipt, queryReceiptOK := program.Query(receiptAssemblySemanticID(80))
	compilation, compilationOK := BeginProgramConstruction(binding, program)
	if !queryReceiptOK || !compilationOK || compilation == nil {
		t.Fatal("diagnostics receipt commit")
	}
	memberOperands := make(map[identity.ContentID]ruleUnit, len(ruleIDs))
	for index, ruleID := range ruleIDs {
		memberOperands[receiptAssemblySemanticID(ruleID)] = operandValues[index]
	}
	if !installMemberOperandResolver(implementation, memberOperands) {
		t.Fatal("diagnostics receipt resolver")
	}
	for _, ruleID := range ruleIDs {
		if _, memberOK := program.RuleMember(receiptAssemblySemanticID(ruleID)); !memberOK {
			t.Fatal("diagnostics receipt member")
		}
		if attached := AttachRuleMember(compilation, implementation, receiptAssemblySemanticID(ruleID)); !attached {
			t.Fatal("diagnostics receipt member attachment")
		}
	}
	if !AttachExactQuery(compilation, queryImplementation, receiptAssemblySemanticID(80)) {
		t.Fatal("diagnostics receipt query attachment")
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil {
		t.Fatal("diagnostics receipt solver")
	}
	return solver, queryReceipt
}

type diagnosticsExternalInterfaceFixture struct {
	solver       *Solver
	sourceGroups [2]int
}

// collectDiagnosticsExternalInterfaceRows establishes both recurrence
// episodes first, then re-evaluates each sealed acyclic external producer
// through epoch.markDirty. The producers publish a strictly greater value on
// that second generation, so each Region observes a real changed external
// interface and records a restart row through the ordinary executor path.
func collectDiagnosticsExternalInterfaceRows(t testing.TB, maxRows int) SolveDiagnostics {
	t.Helper()
	fixture := newDiagnosticsExternalInterfaceFixture(t)
	epoch, epochOK := newRuntimeEpoch(fixture.solver.runtime, fixture.solver.relation, context.Background())
	if !epochOK || epoch == nil {
		t.Fatal("diagnostics external-interface epoch")
	}
	defer epoch.discard()
	epoch.diagnostics = newSolveDiagnosticState(SolveDiagnosticOptions{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticRestart}, Resources: SolveDiagnosticResources{MaxRows: maxRows}})
	if epoch.diagnostics == nil || !epoch.run() {
		t.Fatal("diagnostics external-interface initial epoch")
	}
	// Exercise both independently sealed source revisions. Canonical ordering is
	// deliberately proved below at the collector boundary, rather than inferred
	// from this WTO-governed executor schedule.
	for index := len(fixture.sourceGroups) - 1; index >= 0; index-- {
		if !epoch.markDirty(fixture.sourceGroups[index]) {
			t.Fatal("diagnostics external-interface source revision")
		}
	}
	if !epoch.run() {
		t.Fatal("diagnostics external-interface revision epoch")
	}
	return epoch.diagnostics.snapshot()
}

func newDiagnosticsExternalInterfaceFixture(t testing.TB) diagnosticsExternalInterfaceFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(949_800))
	writeForm, writeOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(949_801), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(949_802)}, Output: factor.Ref(),
	})
	write, ruleWriteOK := SchemaWrite(rule, writeForm)
	firstQuery, firstQueryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(949_803), Freezer: coldKey(949_804)})
	secondQuery, secondQueryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(949_805), Freezer: coldKey(949_806)})
	if !factorOK || !writeOK || !readOK || !ruleOK || !ruleWriteOK || !firstQueryOK || !secondQueryOK || !SchemaQueryRead(firstQuery, readForm) || !SchemaQueryRead(secondQuery, readForm) {
		t.Fatal("diagnostics external-interface schema")
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("diagnostics external-interface schema seal")
	}

	sourceOperands := [2]ruleUnit{ruleUnitForSemantic(coldKey(949_810)), ruleUnitForSemantic(coldKey(949_811))}
	sourceCounts := make(map[ruleUnit]uint64, len(sourceOperands))
	sourceOperand := func(value ruleUnit) bool { return value == sourceOperands[0] || value == sourceOperands[1] }
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(949_802)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			value, live := Operand(access)
			if !live {
				return false
			}
			out := uint64(1)
			if sourceOperand(value) {
				sourceCounts[value]++
				out = sourceCounts[value]
			}
			return Product(access, func(row Row) bool { return StageValue(access, row, out) })
		},
	}
	firstSpec := hotExactQuerySpec()
	firstSpec.Result.Semantic = coldKey(949_804)
	secondSpec := hotExactQuerySpec()
	secondSpec.Result.Semantic = coldKey(949_806)
	// Every loop site of this fixture carries a self environment input, so the
	// sealed program owns WTO Regions and each Region's runtime scope is a Widen
	// scope. A Factor widens only through a declared well-founded measure, and
	// this fixture's values are ascending counters: their complement is the
	// descending rank that witnesses the ascent terminates.
	factorSpec := hotUintFactorSpec()
	factorSpec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	if binding == nil || !BindFactor(binding, factor, factorSpec) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, firstQuery, factor, firstSpec) || !BindExactQuery(binding, secondQuery, factor, secondSpec) || !binding.Seal() {
		t.Fatal("diagnostics external-interface binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	firstImplementation, firstImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, firstQuery)
	secondImplementation, secondImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, secondQuery)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || implementation == nil || !firstImplementationOK || firstImplementation == nil || !secondImplementationOK || secondImplementation == nil || !assemblyOK || assembly == nil {
		t.Fatal("diagnostics external-interface assembly")
	}

	scope := equation.EmptyScope()
	sites := make([]equation.Site, 4)
	for index := range sites {
		site, admitted := assembly.admitSite(compositionKeyOf(coldKey(949_820+index)), scope, equation.TrueExpr(), equation.InitPresent)
		if !admitted {
			t.Fatal("diagnostics external-interface site")
		}
		sites[index] = site
	}
	// source-1, loop-1, external-1, source-2, loop-2, external-2.
	rowSites := []int{0, 1, 1, 2, 3, 3}
	occurrences := make([]equation.Occurrence, len(rowSites))
	operands := make([]equation.Operand, len(rowSites))
	operandValues := make([]ruleUnit, len(rowSites))
	for index, siteIndex := range rowSites {
		occurrence, occurrenceOK := assembly.admitAt(sites[siteIndex])
		value := ruleUnitForSemantic(coldKey(949_830 + index))
		if index == 0 {
			value = sourceOperands[0]
		} else if index == 3 {
			value = sourceOperands[1]
		}
		entity, entityOK := operandEntityForContent(value.content)
		operand, operandOK := assembly.admitOperand(occurrence, entity)
		if !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("diagnostics external-interface operand")
		}
		occurrences[index], operandValues[index], operands[index] = occurrence, value, operand
	}
	if !assembly.SealSources() {
		t.Fatal("diagnostics external-interface source seal")
	}

	declaration := topologyDeclaration{binding: binding, batch: assembly.inner.batch}
	for index, site := range sites {
		declaration.points = append(declaration.points, declaredPointRow{ID: receiptAssemblySemanticID(byte(90 + index)), Site: site})
	}
	proof := implementation.binding.proof
	ruleIDs := make([]byte, len(rowSites))
	for index, siteIndex := range rowSites {
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.beginBindingRuleRow(source)
		part, partOK := implementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
			t.Fatal("diagnostics external-interface rule")
		}
		row, rowOK := assembly.issueRuleRow(draft)
		if !rowOK {
			t.Fatal("diagnostics external-interface group")
		}
		ruleID := byte(100 + index)
		member := declaredMemberRow{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(ruleID), Row: row.row}
		switch index {
		case 1, 4:
			member.EnvironmentInput = equation.BoundaryInput(sites[siteIndex], sites[siteIndex], compositionKeyOf(coldKey(949_840+index)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
		case 2:
			member.EnvironmentInput = equation.BoundaryInput(sites[0], sites[1], compositionKeyOf(coldKey(949_842)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
		case 5:
			member.EnvironmentInput = equation.BoundaryInput(sites[2], sites[3], compositionKeyOf(coldKey(949_845)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
		}
		declaration.members = append(declaration.members, member)
		ruleIDs[index] = ruleID
	}
	declaration.queries = append(declaration.queries,
		declaredQueryRow{ID: receiptAssemblySemanticID(110), Row: equation.QueryInstance{Family: schema.querySemanticAt(0), Point: equation.PointAt(1), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}}}},
		declaredQueryRow{ID: receiptAssemblySemanticID(111), Row: equation.QueryInstance{Family: schema.querySemanticAt(1), Point: equation.PointAt(3), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}}}},
	)
	constructed, refusal := constructTopology(declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("diagnostics external-interface commit stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("diagnostics external-interface committed program")
	}
	_, firstReceiptOK := program.Query(receiptAssemblySemanticID(110))
	_, secondReceiptOK := program.Query(receiptAssemblySemanticID(111))
	compilation, compilationOK := BeginProgramConstruction(binding, program)
	if !firstReceiptOK || !secondReceiptOK || !compilationOK || compilation == nil {
		t.Fatal("diagnostics external-interface compilation")
	}
	memberOperands := make(map[identity.ContentID]ruleUnit, len(ruleIDs))
	for index, ruleID := range ruleIDs {
		memberOperands[receiptAssemblySemanticID(ruleID)] = operandValues[index]
	}
	if !installMemberOperandResolver(implementation, memberOperands) {
		t.Fatal("diagnostics external-interface resolver")
	}
	for _, ruleID := range ruleIDs {
		if _, memberOK := program.RuleMember(receiptAssemblySemanticID(ruleID)); !memberOK {
			t.Fatal("diagnostics external-interface member")
		}
		if attached := AttachRuleMember(compilation, implementation, receiptAssemblySemanticID(ruleID)); !attached {
			t.Fatal("diagnostics external-interface member attachment")
		}
	}
	if !AttachExactQuery(compilation, firstImplementation, receiptAssemblySemanticID(110)) || !AttachExactQuery(compilation, secondImplementation, receiptAssemblySemanticID(111)) {
		t.Fatal("diagnostics external-interface query attachment")
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil || solver.runtime == nil {
		t.Fatal("diagnostics external-interface solver")
	}
	sourceGroups := [2]int{}
	for index, pointID := range []byte{90, 92} {
		point, pointOK := program.lookupPoint(receiptAssemblySemanticID(pointID))
		if !pointOK {
			t.Fatal("diagnostics external-interface source point")
		}
		group, groupOK := solver.runtime.graph.ProducerAt(point, 0)
		groupIndex, indexed := solver.runtime.graph.GroupIndex(group)
		if !groupOK || !indexed {
			t.Fatal("diagnostics external-interface source group")
		}
		sourceGroups[index] = groupIndex
	}
	return diagnosticsExternalInterfaceFixture{solver: solver, sourceGroups: sourceGroups}
}

// TestSolverPublicationStampsFenceCompletedResults proves the two Solver
// fences that share one Generation discipline. The completion serial orders
// published results and never rewinds; the activation-relation stamp binds a
// result to the exact relation that produced it, so publishing a later relation
// invalidates every earlier State without touching the State itself.
func TestSolverPublicationStampsFenceCompletedResults(t *testing.T) {
	solver, _ := newDiagnosticsReceiptSolver(t, false)
	if solver == nil || !solver.relation.Available() {
		t.Fatal("solver publication")
	}
	sealed := solver.relation
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil || state.completion == nil {
		t.Fatalf("solve status=%v state=%t", status, state != nil)
	}
	if !state.completion.serial.Available() || state.completion.serial != solver.completion {
		t.Fatalf("completion serial %d did not name the solver's published completion %d", state.completion.serial, solver.completion)
	}
	if state.completion.relation != solver.relation.Generation() {
		t.Fatal("completed state was not stamped with the live activation relation")
	}
	if !solver.ownsCompletedState(state) {
		t.Fatal("solver rejected its own freshly published state")
	}

	// A later completion serial never resurrects an earlier result, and it never
	// rewinds: atOrBefore is the whole comparison.
	if atOrBefore(state.completion.serial.Next(), solver.completion) {
		t.Fatal("a future completion serial passed the publication fence")
	}
	if !atOrBefore(state.completion.serial, solver.completion.Next()) {
		t.Fatal("a retained completion serial failed the publication fence")
	}
	var unset identity.Generation
	if atOrBefore(unset, solver.completion) || atOrBefore(state.completion.serial, unset) {
		t.Fatal("an unset stamp passed the publication fence")
	}

	// Publishing the next activation relation is the one act that invalidates a
	// retained result. The State is untouched; only the stamp comparison changes.
	published, publishedOK := solver.runtime.topology.Publish(sealed, sealed.Rows())
	if !publishedOK || !sealed.Precedes(published) {
		t.Fatal("solver topology did not advance its publication")
	}
	solver.relation = published
	if solver.ownsCompletedState(state) {
		t.Fatal("a state from a superseded activation relation stayed owned")
	}
	solver.relation = sealed
	if !solver.ownsCompletedState(state) {
		t.Fatal("restoring the publishing relation did not restore the result")
	}
}

// TestExecutionStampCellsAdmitOnlyTheirLiveStamp proves the discipline shared
// by every live-execution fence in the engine: a cell admits exactly one stamp,
// an unavailable stamp is admitted by nothing, a nested token can be claimed
// only while the cell is free, and revoking a stamp that is not live changes
// nothing.
func TestExecutionStampCellsAdmitOnlyTheirLiveStamp(t *testing.T) {
	var sequence generationSequence
	first, issued := sequence.issue()
	second, reissued := sequence.issue()
	if !issued || !reissued || !first.Precedes(second) || second != first.Next() {
		t.Fatalf("sequence did not advance monotonically: first=%d second=%d", first, second)
	}

	var cell generationCell
	if cell.live().Available() || cell.holds(0) || cell.holds(first) {
		t.Fatal("a free cell admitted a stamp")
	}
	if cell.revoke(0) || cell.revoke(first) {
		t.Fatal("a free cell revoked a stamp")
	}
	if !cell.claim(first) || cell.claim(second) || cell.claim(first) {
		t.Fatal("a claimed cell admitted a second holder")
	}
	if !cell.holds(first) || cell.holds(second) || cell.holds(0) {
		t.Fatal("a claimed cell admitted a foreign stamp")
	}
	if cell.revoke(second) || !cell.holds(first) {
		t.Fatal("a foreign stamp revoked a live holder")
	}
	if !cell.revoke(first) || cell.holds(first) || cell.live().Available() {
		t.Fatal("revoking the live stamp did not free the cell")
	}

	cell.open(second)
	if !cell.holds(second) || cell.holds(first) {
		t.Fatal("opening a cell did not install exactly its stamp")
	}
	next, advanced := cell.advance()
	if !advanced || next != second.Next() || !cell.holds(next) || cell.holds(second) {
		t.Fatal("advancing a cell did not supersede the previous stamp")
	}
}

// TestSolveWithReportReceiptCertificateSurvivesSubsequentSolve keeps the
// failure certificate on the receipt-native SolveWithReport route.  A later
// solve is allowed to publish a new terminal attempt, but it must not mutate
// the detached report returned by the earlier attempt.
func TestSolveWithReportReceiptCertificateSurvivesSubsequentSolve(t *testing.T) {
	solver, _ := newDiagnosticsReceiptSolver(t, true)
	state, status, report := solver.SolveWithReport(context.Background())
	if state != nil || status != SolveIncomplete || !report.Available() {
		t.Fatalf("initial receipt report = state:%v status:%v available:%v", state, status, report.Available())
	}
	reason, failure, point, group, member, rule := report.Reason(), report.Failure(), report.Point(), report.Group(), report.Member(), report.Rule()
	if reason == SolveFailureReasonNone || !failure.Available() || !failure.Site.Available() || !point.Available() || !group.Available() || !member.Available() || !rule.Available() {
		t.Fatalf("initial receipt report lost failure coordinates: %#v", report)
	}

	laterState, laterStatus := solver.Solve(context.Background())
	if laterState != nil || laterStatus != SolveIncomplete {
		t.Fatalf("subsequent receipt solve = state:%v status:%v", laterState, laterStatus)
	}
	if report.Reason() != reason || report.Failure() != failure || report.Point() != point || report.Group() != group || report.Member() != member || report.Rule() != rule || !report.Available() {
		t.Fatal("receipt failure certificate changed after subsequent solve")
	}
}

// TestProgramConstructionStagesAreSeparableByTheirSiteDigest is the
// construction localization law. Every stage of the program constructor mints
// one compile-family boundary whose site is unique to it, and the same site is
// the only coordinate needed to recover the stage. A caller therefore localizes
// a construction refusal without the engine publishing a second field.
func TestProgramConstructionStagesAreSeparableByTheirSiteDigest(t *testing.T) {
	seen := make(map[identity.ContentID]ProgramConstructionStage, programConstructionStageCount)
	for stage := ProgramConstructionStageAdmission; stage < programConstructionStageCount; stage++ {
		failure := ProgramConstructionFailure(stage)
		if !failure.Available() || failure.Family != SolveFailureFamilyCompile || !failure.Site.Available() {
			t.Fatalf("stage %d minted no compile-family boundary: %#v", stage, failure)
		}
		if previous, duplicate := seen[failure.Site]; duplicate {
			t.Fatalf("stage %d shares its site digest with stage %d", stage, previous)
		}
		seen[failure.Site] = stage
		recovered, named := ProgramConstructionStageOf(failure)
		if !named || recovered != stage {
			t.Fatalf("stage %d recovered as %d named:%v", stage, recovered, named)
		}
	}
	if len(seen) != int(programConstructionStageCount)-1 {
		t.Fatalf("declared %d stages, minted %d boundaries", int(programConstructionStageCount)-1, len(seen))
	}
}

// TestProgramConstructionStageOfRefusesForeignBoundaries keeps the classifier
// closed over the constructor. A boundary raised by any other authority names
// no construction stage, so routing on it cannot mislocalize a refusal that
// happened somewhere else.
func TestProgramConstructionStageOfRefusesForeignBoundaries(t *testing.T) {
	foreign := []SolveFailure{
		{},
		{Family: SolveFailureFamilyCompile},
		ProgramConstructionFailure(ProgramConstructionStageNone),
		ProgramConstructionFailure(programConstructionStageCount),
		receiptCompilationAttachFailure(1),
		refused(SolveFailureFamilyCompile, "validation").failure(),
		refused(SolveFailureFamilyCompile, "runtime-assembly").failure(),
		receiptFailure(SolveFailureFamilyCompile, "receipt-commit", 1),
		receiptFailure(SolveFailureFamilyObservation, programConstructionAuthority, uint64(ProgramConstructionStageAdmission)),
	}
	for index, failure := range foreign {
		if stage, named := ProgramConstructionStageOf(failure); named || stage != ProgramConstructionStageNone {
			t.Fatalf("foreign boundary %d classified as construction stage %d", index, stage)
		}
	}
}
