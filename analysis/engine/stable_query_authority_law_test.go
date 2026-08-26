package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

// programQueryMatrixFixture is a sealed-row fixture. Every member, query and
// optional observation is admitted through ConstructProgram and the solver is
// minted by CommittedProgram.Seal; no mutable construction handle survives.
type receiptQueryMatrixFixture struct {
	solver               *Solver
	queries              []ProgramQuery
	expected             []uint64
	schemaID             identity.ContentID
	topologyKey          composition.Key
	transferRuns         *int
	projectRuns          *int
	freezeRuns           *int
	binding              *SchemaBinding
	factor               *FactorSlot[uint64]
	graph                *CommittedProgram
	addressed            []composition.Key
	queryImplementations []*ExactQueryImplementation[uint64, uint64]
	observations         []ProgramObservationAdmission
	observationIDs       []identity.ContentID
}

func programMatrixID(value int) identity.ContentID {
	var id identity.ContentID
	id[0] = byte(value)
	id[1] = byte(value >> 8)
	return id
}

// hotExactQuerySpec is the smallest exact query contract used by the sealed
// program fixtures. It intentionally lives with the current fixture rather
// than in a retired construction helper.
func hotExactQuerySpec() HotExactQuerySpec[uint64, uint64] {
	return HotExactQuerySpec[uint64, uint64]{
		Project: func(cells OrderedCells[uint64]) uint64 { return uint64(cells.Count()) },
		Result: FrozenResult[uint64]{
			Semantic:    coldKey(953_100),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
			Present:     func(uint64) bool { return true },
		},
	}
}

func exactQuerySchemaFixture(t testing.TB) (*Schema, *FactorSlot[uint64], *QuerySlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	read, readOK := factor.ExactRead()
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(948_002), Freezer: coldKey(953_100), Population: queryschema.PopulationKindSelectedPoint})
	if !factorOK || !readOK || !queryOK || !SchemaQueryRead(query, read) {
		t.Fatal("exact query schema")
	}
	schema, sealOK := builder.Seal()
	if !sealOK || schema == nil {
		t.Fatal("exact query schema seal")
	}
	return schema, factor, query
}

// TestReceiptQueryWarmStateAndCancellation keeps the terminal lifecycle laws
// on the sealed program: a warm state is readable without rerunning callbacks,
// while cancellation publishes no row.
func TestCommittedQueryWarmStateAndCancellation(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	first, status, report := fixture.solver.SolveWithReport(context.Background())
	if first == nil || status != SolveComplete {
		t.Fatalf("first solve = state:%v status:%v report=%v", first, status, report)
	}
	for index, query := range fixture.queries {
		key, keyed := query.PublicationKey()
		if !keyed {
			t.Fatalf("query[%d] has no snapshot key", index)
		}
		value, readable := testSnapshotQueryValue[uint64](fixture.solver, first, key)
		if !readable || value != fixture.expected[index] {
			t.Fatalf("query[%d] = %d/%v, want %d/true", index, value, readable, fixture.expected[index])
		}
	}
	runs, projects, freezes := *fixture.transferRuns, *fixture.projectRuns, *fixture.freezeRuns
	second, status := fixture.solver.Solve(context.Background())
	if second == nil || status != SolveComplete || *fixture.transferRuns != runs || *fixture.projectRuns != projects || *fixture.freezeRuns != freezes {
		t.Fatalf("warm solve reran callbacks: state:%v status:%v transfers:%d/%d projects:%d/%d freezes:%d/%d", second, status, *fixture.transferRuns, runs, *fixture.projectRuns, projects, *fixture.freezeRuns, freezes)
	}
	canceled := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state, canceledStatus := canceled.solver.Solve(ctx)
	if state != nil || canceledStatus != SolveCanceled {
		t.Fatalf("canceled solve = state:%v status:%v", state, canceledStatus)
	}
}

func newReceiptQueryMatrixFixture(t testing.TB, count int, _, _ []int) receiptQueryMatrixFixture {
	t.Helper()
	return buildReceiptQueryMatrixFixtureWithOptions(t, count, false, false, false)
}

func newObservedReceiptQueryMatrixFixture(t testing.TB, count int, _, _ []int) receiptQueryMatrixFixture {
	t.Helper()
	return buildReceiptQueryMatrixFixtureWithOptions(t, count, true, false, false)
}

// newIncompleteQueryMatrixFixture keeps the solve-report laws on the
// current sealed-program path.  The rule cell is validly sealed, but its
// owner transfer refuses at execution, giving SolveIncomplete a real first
// failure boundary rather than a fabricated report.
func newIncompleteQueryMatrixFixture(t testing.TB, count int) receiptQueryMatrixFixture {
	t.Helper()
	return buildReceiptQueryMatrixFixtureWithOptions(t, count, false, true, false)
}

// newFoldQueryMatrixFixture is the runtime counterpart of the direct
// exact-fold cell law.  It uses the same committed rows and changes only the
// sealed Query projection from Project to Fold.
func newFoldQueryMatrixFixture(t testing.TB, count int) receiptQueryMatrixFixture {
	t.Helper()
	return buildReceiptQueryMatrixFixtureWithOptions(t, count, false, false, true)
}

// buildReceiptQueryMatrixFixture uses one sealed schema cell repeatedly at
// distinct mounted artifact coordinates. The template is the authority for
// points and stages; the declaration inventory supplies only sealed owners.
func buildReceiptQueryMatrixFixture(t testing.TB, count int, observed bool) receiptQueryMatrixFixture {
	t.Helper()
	return buildReceiptQueryMatrixFixtureWithOptions(t, count, observed, false, false)
}

func buildReceiptQueryMatrixFixtureWithOptions(t testing.TB, count int, observed, failTransfer, fold bool) receiptQueryMatrixFixture {
	t.Helper()
	if count <= 0 || count > 200 {
		t.Fatalf("matrix width %d is outside the fixture budget", count)
	}
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(951_000))
	writeForm, writeOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(952_000), OperandFamily: unitOperandFamily, Inputs: 0,
		Output: factor.Ref(),
	})
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(953_000), Freezer: coldKey(953_100), Population: queryschema.PopulationKindSelectedPoint})
	if queryOK {
		queryOK = SchemaQueryRead(query, readForm)
	}
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeOK || !readOK || !ruleOK || !writeSlotOK || !queryOK || !schemaOK || schema == nil {
		t.Fatal("sealed matrix schema")
	}
	transferRuns, projectRuns, freezeRuns := new(int), new(int), new(int)
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(955_000)), true },
		Fold: func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] {
			*transferRuns++
			if failTransfer {
				return RuleResult[uint64]{}
			}
			return Staged(frame, uint64(1))
		},
	}
	querySpec := hotExactQuerySpec()
	querySpec.Result.Semantic = coldKey(953_100)
	querySpec.Result.Freeze = func(value uint64) uint64 { *freezeRuns++; return value }
	querySpec.Project = func(cells OrderedCells[uint64]) uint64 {
		*projectRuns++
		value, present, valid := cells.At(0)
		if !valid || !present {
			return 0
		}
		return value
	}
	if fold {
		querySpec.Project = nil
		querySpec.Fold = QueryFold[OrderedCells[uint64], uint64]{
			Begin: func() uint64 { return 37 },
			Accumulate: func(result uint64, cells OrderedCells[uint64]) (uint64, bool) {
				value, present, valid := cells.At(0)
				if cells.Count() != 1 || !valid {
					return 0, false
				}
				if !present {
					return result, true
				}
				return result + value, true
			},
		}
	}
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, writeSlot, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, factor, querySpec) {
		t.Fatal("sealed matrix binding")
	}
	capability, capabilityOK := IssueMountedRuleCapability(binding, rule)
	if !capabilityOK || !RegisterRuleSlot(binding, rule, capability) || !binding.Seal() {
		t.Fatal("sealed matrix capability")
	}
	ruleImplementation, ruleImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ruleImplementationOK || !queryImplementationOK || ruleImplementation == nil || queryImplementation == nil {
		t.Fatal("sealed matrix implementations")
	}
	mountID := programMatrixID(1)
	artifactID, programID := programMatrixID(2), programMatrixID(3)
	spec, specOK := rows.NewArtifactScalarSpec(artifactID, programID, identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{
		Roles: 1, Points: count + 1, Regions: 1, Events: count + 3, Rules: count, Bodies: 1,
	})
	role, roleOK := spec.DeclareRole(programMatrixID(4))
	if !specOK || !roleOK {
		t.Fatal("sealed matrix artifact header")
	}
	pointIDs := make([]identity.ContentID, count+1)
	for index := range pointIDs {
		pointIDs[index] = programMatrixID(10 + index)
		if _, ok := spec.AddPoint(rows.ArtifactScalarPoint{ID: pointIDs[index], Initial: index == 0}); !ok {
			t.Fatal("sealed matrix artifact point")
		}
	}
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: programMatrixID(40), Head: pointIDs[0]})
	for _, point := range pointIDs {
		regionOK = regionOK && spec.AddRegionMember(region, point)
	}
	if !regionOK {
		t.Fatal("sealed matrix artifact region")
	}
	if !spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: programMatrixID(40)}) {
		t.Fatal("sealed matrix enter")
	}
	for _, point := range pointIDs {
		if !spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: point}) {
			t.Fatal("sealed matrix event")
		}
	}
	if !spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: programMatrixID(40)}) {
		t.Fatal("sealed matrix exit")
	}
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: programMatrixID(41)})
	if !bodyOK || !spec.AddBodyEntry(body, pointIDs[0]) || !spec.AddBodyExit(body, pointIDs[count]) {
		t.Fatal("sealed matrix body")
	}
	for index := 0; index < count; index++ {
		if !spec.AddRule(rows.ArtifactScalarRule{Role: role, Stage: programissuance.StageCallDispatch, Point: pointIDs[index+1], Inputs: [6]identity.ContentID{pointIDs[0]}, InputCount: 1, ID: programMatrixID(60 + index), Native: true}) {
			t.Fatal("sealed matrix artifact rule")
		}
	}
	installArtifactStageTable(t, spec)
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	mount := MountedProgramArtifact{Template: template, Roles: []MountedProgramRole{{Scalar: role, Capability: capability}}, Module: mountID}
	bootstrap, bootstrapOK := NewProgramBootstrap(programMatrixID(70), programMatrixID(71))
	if !templateOK || !bootstrapOK {
		t.Fatal("sealed matrix artifact seal")
	}
	admission := MountedProgramAdmission{}
	cell, cellOK := ruleImplementation.sealedRuleCell()
	if !cellOK || cell == nil {
		t.Fatal("sealed matrix program rule")
	}
	for index := 0; index < count; index++ {
		admission.Mounted = append(admission.Mounted, MountedRuleAdmission{Capability: capability, Mount: mountID, Point: pointIDs[index+1], Occurrence: programMatrixID(60 + index)})
	}
	contexts := explicitTestContextDirectory(t, programMatrixID(70), []identity.ContentID{mountID}, programMatrixID(72), programMatrixID(73))
	queryAdmissions := make([]ProgramQueryAdmission, 0, count)
	for index := 0; index < count; index++ {
		admitted, admittedOK := NewExactQueryAdmission(queryImplementation, programMatrixID(110+index), mountID, pointIDs[index+1], explicitTestContext(t, contexts, mountID))
		if !admittedOK {
			t.Fatal("sealed matrix query admission")
		}
		queryAdmissions = append(queryAdmissions, admitted)
	}
	admission.Queries = queryAdmissions
	program, refusal, constructed := ConstructProgram(ProgramDeclaration{Binding: binding, Mounts: []MountedProgramArtifact{mount}, Bootstrap: bootstrap, Contexts: contexts, Admission: admission})
	if !constructed || program == nil {
		t.Fatalf("sealed matrix ConstructProgram stage=%v seal=%v commit=%v", refusal.Stage(), refusal.Seal(), refusal.Commit())
	}
	observations := make([]ProgramObservationAdmission, 0, count)
	observationIDs := make([]identity.ContentID, 0, count)
	if observed {
		for index := 0; index < count; index++ {
			id := programMatrixID(160 + index)
			row, rowOK := NewExactObservationAdmission(queryImplementation, id, capability, mountID, pointIDs[index+1], programMatrixID(60+index), explicitTestContext(t, contexts, mountID))
			if !rowOK {
				t.Fatal("sealed matrix observation admission")
			}
			observations = append(observations, row)
			observationIDs = append(observationIDs, id)
		}
	}
	solver, failure, solverOK := program.Seal(observations)
	if !solverOK || solver == nil {
		t.Fatalf("sealed matrix Solver failure=%v", failure)
	}
	queries := make([]ProgramQuery, count)
	for index := range queries {
		var ok bool
		queries[index], ok = program.Query(programMatrixID(110 + index))
		if !ok {
			t.Fatal("sealed matrix query handle")
		}
	}
	addressed, addressedOK := program.publishedQueryKeys()
	if !addressedOK {
		t.Fatal("sealed matrix query table")
	}
	// Every query is admitted at the point whose member staged the write, so
	// what it observes is that member's cell. The matrix rule stages one, so
	// one is the answer each query publishes; a zero here would be the cell no
	// member of that point wrote.
	expected := make([]uint64, count)
	if !failTransfer {
		for index := range expected {
			expected[index] = 1
		}
	}
	implementations := make([]*ExactQueryImplementation[uint64, uint64], count)
	for index := range implementations {
		implementations[index] = queryImplementation
	}
	return receiptQueryMatrixFixture{solver: solver, queries: queries, expected: expected, schemaID: identity.ContentID(schema.ID().Digest()), topologyKey: program.topology.Key(), transferRuns: transferRuns, projectRuns: projectRuns, freezeRuns: freezeRuns, binding: binding, factor: factor, graph: program, addressed: addressed, queryImplementations: implementations, observations: observations, observationIDs: observationIDs}
}

// newBorrowedQueryFixture is the current sealed-program replacement for the
// retired observation fixture. A completed State borrows a scalar query row
// directly from the immutable publication.
func newBorrowedQueryFixture(t testing.TB) (*Solver, ProgramQuery, *State) {
	t.Helper()
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("sealed query fixture solve = state:%t status:%v", state != nil, status)
	}
	return fixture.solver, fixture.queries[0], state
}

// newBorrowedObservationFixture is the observation-lane counterpart. The
// admission itself is immutable and its public ID is the snapshot row key.
func newBorrowedObservationFixture(t testing.TB) (*Solver, ProgramObservationAdmission, *State) {
	t.Helper()
	fixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete || len(fixture.observations) != 1 {
		t.Fatalf("sealed observation fixture solve = state:%t status:%v observations:%d", state != nil, status, len(fixture.observations))
	}
	return fixture.solver, fixture.observations[0], state
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
