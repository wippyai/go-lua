package analysis

import (
	"fmt"
	"testing"

	"context"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema"
	denominatorcounts "github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/snapshot"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/composite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// This file is the corpus-backed equivalence law: on real solves, the answers a
// published composition hands out are the answers the receipt path hands out.
//
// The law is stated here because this is the only package the receipt graph is
// reachable from. A solve is driven through the production path - the same
// runtime solver Plan.Solve builds - and every answer it publishes is read back
// twice: once through the detached receipt query the analysis result projection
// consumes, and once through the materialized snapshot a consumer of the
// composition reads. The two readings must agree exactly.
//
// The lanes are built from the receipt answers themselves. That is what makes
// the law a statement about the two paths rather than about two solves: one
// solve produces one set of facts, the composition publishes them through the
// owning domains' own contributors, and the published column and query result
// must reproduce the receipt answer at every coordinate and every subject,
// including where the answer is an absence.
//
// Only the value and effect columns have a receipt counterpart. The heap, pack
// and call axes publish no query family and no receipt observation exists to
// compare their columns against, so their lanes here are honest empties: they
// hold no fact, and the law states nothing about them. The reachability lane is
// derived from the receipts themselves - a point the solve folded a row at was
// reached - and the neutral count column carries an explicit zero per declared
// relation, because this law measures no cardinality.

var (
	equivalenceStore      = identity.StoreID(11)
	equivalenceGeneration = identity.Generation(2)
)

// equivalenceExecutionUniverse is the derivation domain of the reachability
// universe this law publishes. The points are the ones the solve was asked at,
// so the membership authority is derived from that exact set.
const equivalenceExecutionUniverse = "analysis/corpus-equivalence/execution-universe/v1"

// equivalencePoint is one mounted execution point as this law spells it. The
// declaring axis leaves the spelling to the publisher and the reader, so the
// pair enters the publication as this file's own type.
type equivalencePoint struct {
	mount identity.ContentID
	point identity.ContentID
}

type equivalenceFixture struct {
	name string
	link func(testing.TB) *link.Link
}

// equivalenceCorpus is the corpus the law runs over: the compiled-plan laws'
// own parity fixture, a multi-coordinate arithmetic body with a guard, a body
// that calls through a local function, and a multiple-return body. Each one
// solves to a different coordinate width and a different mix of present and
// absent per-coordinate answers.
var equivalenceCorpus = []equivalenceFixture{
	{name: "parity", link: planLawLink},
	{name: "arithmetic-guard", link: equivalenceLinkFor("local a = 1\nlocal b = a + 2\nif b then return b end\nreturn a\n")},
	{name: "local-call", link: equivalenceLinkFor("local t = { a = 1 }\nlocal function f(v) return v end\nlocal r = f(t)\nreturn r\n")},
	{name: "multiple-return", link: equivalenceLinkFor("local a, b = 1, 2\nlocal c = a + b\nlocal d = c .. \"x\"\nreturn d, c, a\n")},
}

func equivalenceLinkFor(source string) func(testing.TB) *link.Link {
	return func(t testing.TB) *link.Link {
		t.Helper()
		return planLawMountedLink(t, []linkproject.Module{{Name: "equivalence", Program: planLawProgram(t, source)}})
	}
}

// summaryAnswer is one value-summary subject as the receipt path answers it:
// the query identity the subject is addressed by, the mounted point it was
// asked at, and the detached observation the completed solve produced.
type summaryAnswer struct {
	subject identity.ContentID
	point   equivalencePoint
	answer  valuedomain.ValueSummaryObservation
}

// effectAnswer is one effect-exact subject as the receipt path answers it. The
// root is the effect root of the body the point belongs to, which is the root
// the family folds its one row at.
type effectAnswer struct {
	subject identity.ContentID
	point   equivalencePoint
	root    effectfactor.Root
	answer  effectfactor.EffectObservation
}

// receiptSolve is one completed solve read entirely through its receipts: the
// mounted record the composition publishes from, the sealed authorities, and
// every answer the two receipt-backed families produced.
type receiptSolve struct {
	record      composite.LinkInputs
	schema      *valuedomain.Schema
	algebra     *effectfactor.Algebra
	coordinates []compiledValueCoordinate
	summaries   []summaryAnswer
	effects     []effectAnswer
	points      []equivalencePoint
}

// solveThroughReceipts drives one Link through the production solve path and
// reads every query row of the artifact query plan back out of the receipt
// graph. It performs no analysis of its own: the answers it collects are the
// exact ones the result projection consumes.
func solveThroughReceipts(t *testing.T, linked *link.Link) *receiptSolve {
	t.Helper()
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v", status)
	}
	t.Cleanup(func() { plan.Close() })
	state := plan.state
	// The mounted record is the mount phase's own output and the assembled
	// Plan is forbidden from retaining it: the record carries the Link, and a
	// published Plan holds no construction owner. The law therefore performs
	// the mount itself, through the plan's own construction path, and installs
	// the binding it produced before any runtime topology exists, so the record
	// it publishes from and the authorities the solve runs on are one seal.
	record, binding, bindingFailure, mountFailure, _ := state.newProgramBinding(linked)
	if bindingFailure != ProgramBindingFailureNone || mountFailure.Available() || binding == nil || binding.SchemaBinding() == nil {
		t.Fatalf("the mount phase refused the Link: binding=%v mount=%v", bindingFailure, mountFailure)
	}
	state.binding = binding
	if _, topologyOK := state.instantiateRuntimeTopology(); !topologyOK {
		t.Fatal("the plan built no runtime topology")
	}
	solver, queryPlan, _, failure, compiled := state.buildRuntimeSolver(nil)
	if !compiled || solver == nil || queryPlan == nil || failure.Available() {
		t.Fatalf("the plan built no runtime solver: %v", failure)
	}
	engineState, solveStatus, _ := solver.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
	if solveStatus != engine.SolveComplete || engineState == nil {
		t.Fatalf("solve = %v", solveStatus)
	}
	coordinates, coordinatesOK := compileValueCoordinates(linked)
	if !coordinatesOK {
		t.Fatal("the Link publishes no value coordinate order")
	}
	solve := &receiptSolve{
		record:      record,
		schema:      binding.ValueSchema(),
		algebra:     record.EffectAlgebra,
		coordinates: coordinates,
	}
	if solve.schema == nil || solve.algebra == nil || solve.record.ValueSchema != solve.schema {
		t.Fatal("the mounted record is not the record the solve bound")
	}
	seenPoints := make(map[equivalencePoint]struct{}, len(queryPlan.rows))
	for _, row := range queryPlan.rows {
		query, queryOK := state.graph.Query(row.id)
		if !queryOK {
			t.Fatalf("the receipt graph holds no query for row %x", row.id[:4])
		}
		point := equivalencePoint{mount: row.mount, point: row.point}
		if _, seen := seenPoints[point]; !seen {
			seenPoints[point] = struct{}{}
			solve.points = append(solve.points, point)
		}
		switch row.role {
		case artifactQueryValueSummary:
			answer, readable := engine.ReceiptQueryResult[valuedomain.ValueSummaryObservation](query, solver, engineState)
			if !readable || !answer.Valid {
				t.Fatalf("the value-summary receipt at point %x is unreadable", row.point[:4])
			}
			if len(answer.Values) != len(answer.Present) || len(answer.Values) != solve.schema.CoordinateCount() {
				t.Fatalf("the value-summary receipt at point %x is not shaped by the sealed coordinate width", row.point[:4])
			}
			solve.summaries = append(solve.summaries, summaryAnswer{subject: row.id, point: point, answer: answer})
		case artifactQueryEffectExact:
			answer, readable := engine.ReceiptQueryResult[effectfactor.EffectObservation](query, solver, engineState)
			if !readable || !answer.Valid {
				t.Fatalf("the effect-exact receipt at point %x is unreadable", row.point[:4])
			}
			root, rootOK := solve.rootForPoint(state, point)
			if !rootOK {
				t.Fatalf("the effect algebra issues no root for the body of point %x", row.point[:4])
			}
			solve.effects = append(solve.effects, effectAnswer{subject: row.id, point: point, root: root, answer: answer})
		default:
			t.Fatalf("the artifact query plan holds a row of no known role at point %x", row.point[:4])
		}
	}
	if len(solve.summaries) == 0 || len(solve.effects) != len(solve.summaries) {
		t.Fatalf("the solve answered %d summary and %d effect subjects", len(solve.summaries), len(solve.effects))
	}
	return solve
}

// rootForPoint resolves the effect root the exact family folds one point at:
// the root of the body the compiled result receipt attaches that point to.
func (solve *receiptSolve) rootForPoint(state *compiledState, point equivalencePoint) (effectfactor.Root, bool) {
	bodies := state.resultReceipt.pointBodies[artifactResultPoint{mount: point.mount, point: point.point}]
	if len(bodies) == 0 {
		return effectfactor.Root{}, false
	}
	body := state.resultReceipt.bodies[bodies[0]]
	for _, mount := range state.artifacts.mounts {
		if mount.moduleKey != body.key.mount {
			continue
		}
		return solve.algebra.RootForMountedBodyID(mount.moduleKey, mount.programID, body.key.body)
	}
	return effectfactor.Root{}, false
}

// summaryLane reads one receipt answer back as a value lane. The lane answers
// at a coordinate exactly what the receipt answered at that coordinate's
// ordinal, which is the read a contributor performs.
func summaryLane(schema *valuedomain.Schema, answer valuedomain.ValueSummaryObservation) func(valuedomain.Coordinate) (valuedomain.Value, bool) {
	return func(coordinate valuedomain.Coordinate) (valuedomain.Value, bool) {
		index, indexed := schema.CoordinateIndex(coordinate)
		if !indexed || int(index) >= len(answer.Present) || !answer.Present[index] {
			return valuedomain.Value{}, false
		}
		return answer.Values[index], true
	}
}

// effectValueOf recovers the algebra value one receipt answer states. An answer
// with no present row holds no fact; Top and the empty set are recovered from
// the algebra itself. An atom-bearing answer cannot be recovered: the detached
// observation publishes portable atom identities and the algebra issues no
// reverse lookup for them, so the law refuses rather than guessing a value.
func effectValueOf(algebra *effectfactor.Algebra, answer effectfactor.EffectObservation) (value effectfactor.Value, held bool, recovered bool) {
	if !answer.Present {
		return effectfactor.Value{}, false, true
	}
	if answer.Top {
		return algebra.Top(), true, true
	}
	if len(answer.Atoms) == 0 {
		return algebra.Bottom(), true, true
	}
	return effectfactor.Value{}, false, false
}

// equivalenceLanes is one completed solve as the composition publishes it. The
// two receipt-backed columns carry the join of what the receipts answered, each
// family is asked at exactly the subjects the receipt path answered, and every
// other lane is an honest empty.
func equivalenceLanes(t *testing.T, solve *receiptSolve) composite.LaneSet[equivalencePoint] {
	t.Helper()
	joined, held, joinedOK := solve.joinedValueColumn()
	effectRows, effectRowsOK := solve.joinedEffectColumn()
	if !joinedOK || !effectRowsOK {
		t.Fatal("the receipt answers of one solve do not join into one contributed column")
	}
	reached := make(map[equivalencePoint]bool, len(solve.summaries))
	for _, summary := range solve.summaries {
		reached[summary.point] = summary.answer.Rows != 0
	}
	summaries := make([]composite.SummarySubject, 0, len(solve.summaries))
	for _, summary := range solve.summaries {
		subject := composite.SummarySubject{Subject: summary.subject}
		// A subject the solve folded no row for is asked without a lane: it is a
		// member of the result column's universe and carries no answer, which is
		// the published form of the receipt path's own zero-row answer.
		if summary.answer.Rows != 0 {
			subject.Lane = summaryLane(solve.schema, summary.answer)
		}
		summaries = append(summaries, subject)
	}
	exacts := make([]composite.ExactSubject, 0, len(solve.effects))
	for _, effect := range solve.effects {
		subject := composite.ExactSubject{Subject: effect.subject, Root: effect.root}
		if effect.answer.Rows != 0 {
			value, valueHeld, recovered := effectValueOf(solve.algebra, effect.answer)
			if !recovered {
				t.Fatalf("the effect receipt at point %x publishes atoms the algebra issues no recovery for", effect.point.point[:4])
			}
			subject.Lane = func(asked effectfactor.Root) (effectfactor.Value, bool) {
				if asked != effect.root || !valueHeld {
					return effectfactor.Value{}, false
				}
				return value, true
			}
		}
		exacts = append(exacts, subject)
	}
	universe, derived := identity.DeriveContentID(equivalenceExecutionUniverse, solve.pointParts()...)
	if !derived {
		t.Fatal("the asked point set seals no membership authority")
	}
	return composite.LaneSet[equivalencePoint]{
		Value: func(coordinate valuedomain.Coordinate) (valuedomain.Value, bool) {
			index, indexed := solve.schema.CoordinateIndex(coordinate)
			if !indexed || !held[index] {
				return valuedomain.Value{}, false
			}
			return joined[index], true
		},
		Effect: func(root effectfactor.Root) (effectfactor.Value, bool) {
			index, indexed := solve.algebra.RootIndex(root)
			if !indexed {
				return effectfactor.Value{}, false
			}
			value, rowHeld := effectRows[index]
			return value, rowHeld
		},
		// No receipt observation exists for the heap, pack or call columns, so
		// these lanes state what this law can prove about them: nothing.
		Pack: func(packdomain.Root) (packdomain.Value, bool) { return packdomain.Value{}, false },
		Heap: func(heapdomain.Key) (heapdomain.Value, bool) { return heapdomain.Value{}, false },
		Call: func(calldomain.Key) (calldomain.Value, bool) { return calldomain.Value{}, false },
		Execution: composite.ExecutionLane[equivalencePoint]{
			Universe: universe,
			Points:   solve.points,
			Reached:  func(point equivalencePoint) bool { return reached[point] },
		},
		Counts:       equivalenceCounts(t),
		ValueSummary: summaries,
		EffectExact:  exacts,
	}
}

// joinedValueColumn is the value column this solve contributes: at every
// coordinate, the join of the receipt answers that hold a fact there, and no
// row at all where none does.
func (solve *receiptSolve) joinedValueColumn() ([]valuedomain.Value, []bool, bool) {
	width := solve.schema.CoordinateCount()
	joined := make([]valuedomain.Value, width)
	held := make([]bool, width)
	for _, summary := range solve.summaries {
		if summary.answer.Rows == 0 {
			continue
		}
		for index := 0; index < width; index++ {
			if !summary.answer.Present[index] {
				continue
			}
			if !held[index] {
				joined[index], held[index] = summary.answer.Values[index], true
				continue
			}
			value, ok := solve.schema.Join(joined[index], summary.answer.Values[index])
			if !ok {
				return nil, nil, false
			}
			joined[index] = value
		}
	}
	return joined, held, true
}

// joinedEffectColumn is the effect column this solve contributes, keyed by the
// dense root position: the join of the recovered receipt answers rooted there.
func (solve *receiptSolve) joinedEffectColumn() (map[int]effectfactor.Value, bool) {
	rows := make(map[int]effectfactor.Value, len(solve.effects))
	for _, effect := range solve.effects {
		if effect.answer.Rows == 0 {
			continue
		}
		value, valueHeld, recovered := effectValueOf(solve.algebra, effect.answer)
		if !recovered {
			return nil, false
		}
		if !valueHeld {
			continue
		}
		index, indexed := solve.algebra.RootIndex(effect.root)
		if !indexed {
			return nil, false
		}
		existing, present := rows[index]
		if !present {
			rows[index] = value
			continue
		}
		merged, ok := solve.algebra.Join(existing, value)
		if !ok {
			return nil, false
		}
		rows[index] = merged
	}
	return rows, true
}

func (solve *receiptSolve) pointParts() [][]byte {
	parts := make([][]byte, 0, 2*len(solve.points))
	for index := range solve.points {
		parts = append(parts, solve.points[index].mount[:], solve.points[index].point[:])
	}
	return parts
}

// equivalenceCounts is one complete set of owner-local relation counts. The
// neutral denominator column is total over the generated relation catalog and
// this law states nothing about relation cardinality, so every declaration
// carries an explicit zero rather than being left out.
func equivalenceCounts(t *testing.T) []denominatorcounts.CountRows {
	t.Helper()
	entries := denominatorcounts.GeneratedRelationEntries()
	rows := make([]denominatorcounts.CountRow, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || !entry.EntryAvailable() {
			t.Fatal("the generated relation catalog holds an unavailable declaration")
		}
		row, ok := denominatorcounts.NewCountRow(entry.ID(), 0)
		if !ok {
			t.Fatalf("relation %q admits no count row", entry.Key())
		}
		rows = append(rows, row)
	}
	set, ok := denominatorcounts.NewCountRows(rows)
	if !ok || !denominatorcounts.GeneratedCountRowsComplete(set) {
		t.Fatal("the law supplies an incomplete relation count set")
	}
	return []denominatorcounts.CountRows{set}
}

// equivalenceCoverage is what one run of the law actually proved. A law that
// reads a published composition holding no row, or one whose universe covers
// nothing, states nothing at all, so the coverage travels with the findings and
// the law holds itself to a non-empty one.
type equivalenceCoverage struct {
	valueHits       int
	valueAbsences   int
	effectHits      int
	effectAbsences  int
	summaryAnswers  int
	summaryAbsences int
	exactAnswers    int
	exactAbsences   int
}

func (coverage equivalenceCoverage) String() string {
	return fmt.Sprintf("value hits=%d absent=%d | effect hits=%d absent=%d | value-summary answered=%d absent=%d | effect-exact answered=%d absent=%d",
		coverage.valueHits, coverage.valueAbsences, coverage.effectHits, coverage.effectAbsences,
		coverage.summaryAnswers, coverage.summaryAbsences, coverage.exactAnswers, coverage.exactAbsences)
}

// equivalenceMismatches is the law itself: every disagreement between what the
// published composition answers and what the receipt path answered, named by
// the exact coordinate, root or subject it was found at. It returns findings
// rather than failing so the red-first law can hold it to naming the one
// coordinate a corrupted lane read changes.
func equivalenceMismatches(solve *receiptSolve, published *snapshot.Snapshot) ([]string, equivalenceCoverage) {
	var coverage equivalenceCoverage
	findings := make([]string, 0)
	joined, held, joinedOK := solve.joinedValueColumn()
	if !joinedOK {
		return append(findings, "the receipt answers do not join into one value column"), coverage
	}
	findings = append(findings, valueColumnMismatches(solve, published, joined, held, &coverage)...)
	findings = append(findings, effectColumnMismatches(solve, published, &coverage)...)
	findings = append(findings, summaryFamilyMismatches(solve, published, &coverage)...)
	findings = append(findings, exactFamilyMismatches(solve, published, &coverage)...)
	return findings, coverage
}

func valueColumnMismatches(solve *receiptSolve, published *snapshot.Snapshot, joined []valuedomain.Value, held []bool, coverage *equivalenceCoverage) []string {
	findings := make([]string, 0)
	column, projected := composite.ProjectAxis[valuedomain.Coordinate, valuedomain.Value]("value/facts")
	if !projected {
		return append(findings, "the value column projects no published address")
	}
	for index := 0; index < solve.schema.CoordinateCount(); index++ {
		coordinate, issued := solve.schema.CoordinateAt(index)
		if !issued {
			findings = append(findings, fmt.Sprintf("value coordinate %d is not issued by the sealed schema", index))
			continue
		}
		fact, status := snapshot.Read(published, column, coordinate)
		if !held[index] {
			if status != snapshot.ReadProvenAbsent {
				findings = append(findings, fmt.Sprintf("value coordinate %d reads back as %s, and no receipt answer holds a fact there", index, status))
				continue
			}
			coverage.valueAbsences++
			continue
		}
		if status != snapshot.ReadHit {
			findings = append(findings, fmt.Sprintf("value coordinate %d reads back as %s, not the fact the receipt answers hold there", index, status))
			continue
		}
		if !solve.schema.Equal(fact, joined[index]) {
			findings = append(findings, fmt.Sprintf("value coordinate %d publishes a fact the receipt answers do not state", index))
			continue
		}
		coverage.valueHits++
	}
	return findings
}

func effectColumnMismatches(solve *receiptSolve, published *snapshot.Snapshot, coverage *equivalenceCoverage) []string {
	findings := make([]string, 0)
	column, projected := composite.ProjectAxis[effectfactor.Root, effectfactor.Value]("effect/facts")
	if !projected {
		return append(findings, "the effect column projects no published address")
	}
	rows, rowsOK := solve.joinedEffectColumn()
	if !rowsOK {
		return append(findings, "the receipt answers do not join into one effect column")
	}
	for index := 0; index < solve.algebra.RootCount(); index++ {
		root, issued := solve.algebra.RootAt(index)
		if !issued {
			findings = append(findings, fmt.Sprintf("effect root %d is not issued by the sealed algebra", index))
			continue
		}
		fact, status := snapshot.Read(published, column, root)
		expected, expectedHeld := rows[index]
		if !expectedHeld {
			if status != snapshot.ReadProvenAbsent {
				findings = append(findings, fmt.Sprintf("effect root %d reads back as %s, and no receipt answer holds a fact there", index, status))
				continue
			}
			coverage.effectAbsences++
			continue
		}
		if status != snapshot.ReadHit {
			findings = append(findings, fmt.Sprintf("effect root %d reads back as %s, not the fact the receipt answers hold there", index, status))
			continue
		}
		if !solve.algebra.Equal(fact, expected) {
			findings = append(findings, fmt.Sprintf("effect root %d publishes a fact the receipt answers do not state", index))
			continue
		}
		coverage.effectHits++
	}
	return findings
}

// summaryFamilyMismatches holds the value-summary result column to the receipt
// answers: an answered subject reads back the receipt's own observation, a
// subject the solve folded no row for reads back a proven absence, and a
// subject this family was never asked at reads back a miss.
func summaryFamilyMismatches(solve *receiptSolve, published *snapshot.Snapshot, coverage *equivalenceCoverage) []string {
	findings := make([]string, 0)
	family, familyOK := equivalenceFamily(composite.QueryFamilyValueSummary)
	if !familyOK {
		return append(findings, "the sealed table declares no value-summary family")
	}
	plan, opens := snapshot.OpenQuery[identity.ContentID, valuedomain.ValueSummaryObservation](published, family)
	if !opens {
		return append(findings, "the value-summary family opens no published result column")
	}
	for _, summary := range solve.summaries {
		answer, status := snapshot.Query(published, plan, summary.subject)
		if summary.answer.Rows == 0 {
			if status != snapshot.ReadProvenAbsent {
				findings = append(findings, fmt.Sprintf("value-summary subject %x reads back as %s, and the receipt path folded no row for it", summary.subject[:4], status))
				continue
			}
			coverage.summaryAbsences++
			continue
		}
		if status != snapshot.ReadHit {
			findings = append(findings, fmt.Sprintf("value-summary subject %x reads back as %s, not the answer the receipt path published", summary.subject[:4], status))
			continue
		}
		if !valuedomain.EqualValueSummary(solve.schema, answer, summary.answer) {
			findings = append(findings, fmt.Sprintf("value-summary subject %x publishes an answer the receipt path did not", summary.subject[:4]))
			continue
		}
		coverage.summaryAnswers++
	}
	if _, status := snapshot.Query(published, plan, solve.effects[0].subject); status != snapshot.ReadMiss {
		findings = append(findings, fmt.Sprintf("a subject the value-summary family was never asked at reads back as %s, not a miss", status))
	}
	return findings
}

func exactFamilyMismatches(solve *receiptSolve, published *snapshot.Snapshot, coverage *equivalenceCoverage) []string {
	findings := make([]string, 0)
	family, familyOK := equivalenceFamily(composite.QueryFamilyEffectExact)
	if !familyOK {
		return append(findings, "the sealed table declares no effect-exact family")
	}
	plan, opens := snapshot.OpenQuery[identity.ContentID, effectfactor.EffectObservation](published, family)
	if !opens {
		return append(findings, "the effect-exact family opens no published result column")
	}
	for _, effect := range solve.effects {
		answer, status := snapshot.Query(published, plan, effect.subject)
		if effect.answer.Rows == 0 {
			if status != snapshot.ReadProvenAbsent {
				findings = append(findings, fmt.Sprintf("effect-exact subject %x reads back as %s, and the receipt path folded no row for it", effect.subject[:4], status))
				continue
			}
			coverage.exactAbsences++
			continue
		}
		if status != snapshot.ReadHit {
			findings = append(findings, fmt.Sprintf("effect-exact subject %x reads back as %s, not the answer the receipt path published", effect.subject[:4], status))
			continue
		}
		if !effectfactor.EqualEffect(answer, effect.answer) {
			findings = append(findings, fmt.Sprintf("effect-exact subject %x publishes an answer the receipt path did not", effect.subject[:4]))
			continue
		}
		coverage.exactAnswers++
	}
	if _, status := snapshot.Query(published, plan, solve.summaries[0].subject); status != snapshot.ReadMiss {
		findings = append(findings, fmt.Sprintf("a subject the effect-exact family was never asked at reads back as %s, not a miss", status))
	}
	return findings
}

func equivalenceFamily(family schema.Key) (identity.ContentID, bool) {
	requests, ok := composite.QueryRequests()
	if !ok {
		return identity.ContentID{}, false
	}
	for _, request := range requests {
		if request.Family == family {
			return request.ID, request.ID.Available()
		}
	}
	return identity.ContentID{}, false
}

func publishEquivalence(t *testing.T, solve *receiptSolve, lanes composite.LaneSet[equivalencePoint]) snapshot.Snapshot {
	t.Helper()
	published, failure := composite.Materialize(composite.Materialization[equivalencePoint]{
		Link:       solve.record,
		Store:      equivalenceStore,
		Generation: equivalenceGeneration,
		Lanes:      lanes,
	})
	if failure.Available() {
		t.Fatalf("the materializer refused the mounted record and this solve's lanes: %v", failure)
	}
	if !published.Published() {
		t.Fatal("the publication did not seal")
	}
	return published
}

// TestPublishedCompositionAnswersTheReceiptPath is the equivalence law. For
// every fixture in the corpus, one real solve is read out through the receipt
// graph and published through the composition, and the two readings must agree
// at every coordinate, every effect root and every subject of both sealed
// families, in presence and in absence alike.
func TestPublishedCompositionAnswersTheReceiptPath(t *testing.T) {
	for _, fixture := range equivalenceCorpus {
		t.Run(fixture.name, func(t *testing.T) {
			solve := solveThroughReceipts(t, fixture.link(t))
			published := publishEquivalence(t, solve, equivalenceLanes(t, solve))
			findings, coverage := equivalenceMismatches(solve, &published)
			for _, finding := range findings {
				t.Error(finding)
			}
			t.Log(coverage)
			// A law that proved nothing is a law that passed for the wrong
			// reason: the published composition must have answered, and been
			// held to a proven absence, on both receipt-backed planes.
			if coverage.valueHits == 0 || coverage.valueAbsences == 0 {
				t.Fatalf("the value column proved no agreement: %s", coverage)
			}
			if coverage.effectHits+coverage.effectAbsences == 0 {
				t.Fatalf("the effect column proved no agreement: %s", coverage)
			}
			if coverage.summaryAnswers == 0 || coverage.exactAnswers == 0 {
				t.Fatalf("neither sealed family answered a subject: %s", coverage)
			}
		})
	}
}

// TestPublishedCompositionOrdinalsAreTheReceiptOrdinals is the alignment the
// equivalence rests on: the ordinal a receipt answer indexes a value by is the
// sealed schema's own coordinate ordinal for the same boundary value, so a lane
// built from a receipt answer reads at the coordinate the answer states.
func TestPublishedCompositionOrdinalsAreTheReceiptOrdinals(t *testing.T) {
	for _, fixture := range equivalenceCorpus {
		t.Run(fixture.name, func(t *testing.T) {
			solve := solveThroughReceipts(t, fixture.link(t))
			if len(solve.coordinates) != solve.schema.CoordinateCount() {
				t.Fatalf("the compiled coordinate order holds %d rows and the sealed schema %d", len(solve.coordinates), solve.schema.CoordinateCount())
			}
			for ordinal, coordinate := range solve.coordinates {
				issued, issuedOK := solve.schema.CoordinateForID(coordinate.id)
				index, indexed := solve.schema.CoordinateIndex(issued)
				if !issuedOK || !indexed {
					t.Fatalf("the sealed schema issues no coordinate for the value at ordinal %d", ordinal)
				}
				if int(index) != ordinal {
					t.Fatalf("the value at ordinal %d is sealed at coordinate %d", ordinal, index)
				}
			}
		})
	}
}

// TestPublishedCompositionNamesADivergentCoordinate is the red-first half. One
// lane read is corrupted - the value column drops the fact it holds at one
// coordinate - and the law must name that exact coordinate rather than pass or
// report a different one.
func TestPublishedCompositionNamesADivergentCoordinate(t *testing.T) {
	solve := solveThroughReceipts(t, equivalenceCorpus[1].link(t))
	lanes := equivalenceLanes(t, solve)
	_, held, joinedOK := solve.joinedValueColumn()
	if !joinedOK {
		t.Fatal("the receipt answers of one solve do not join into one contributed column")
	}
	corrupted := -1
	for index, present := range held {
		if present {
			corrupted = index
			break
		}
	}
	if corrupted < 0 {
		t.Fatal("the fixture holds no published value fact to corrupt")
	}
	honest := lanes.Value
	lanes.Value = func(coordinate valuedomain.Coordinate) (valuedomain.Value, bool) {
		if index, indexed := solve.schema.CoordinateIndex(coordinate); indexed && int(index) == corrupted {
			return valuedomain.Value{}, false
		}
		return honest(coordinate)
	}
	published := publishEquivalence(t, solve, lanes)
	findings, _ := equivalenceMismatches(solve, &published)
	expected := fmt.Sprintf("value coordinate %d reads back as proven-absent, not the fact the receipt answers hold there", corrupted)
	if len(findings) != 1 || findings[0] != expected {
		t.Fatalf("a corrupted lane read produced %v, not the single finding %q", findings, expected)
	}
}

// TestPublishedCompositionNamesADivergentSubject is the red-first half on the
// family plane. One subject's lane is corrupted and the law must name that
// exact subject: a family answer that drifts from the receipt path is a finding
// against the subject it was asked at, not a silent pass.
func TestPublishedCompositionNamesADivergentSubject(t *testing.T) {
	solve := solveThroughReceipts(t, equivalenceCorpus[1].link(t))
	lanes := equivalenceLanes(t, solve)
	corrupted := -1
	for index, subject := range lanes.ValueSummary {
		if subject.Lane != nil {
			corrupted = index
			break
		}
	}
	if corrupted < 0 {
		t.Fatal("the fixture asks the value-summary family with no lane at all")
	}
	honest := lanes.ValueSummary[corrupted].Lane
	lanes.ValueSummary[corrupted].Lane = func(coordinate valuedomain.Coordinate) (valuedomain.Value, bool) {
		fact, held := honest(coordinate)
		if !held {
			return solve.schema.Top(), true
		}
		return fact, held
	}
	published := publishEquivalence(t, solve, lanes)
	findings, _ := equivalenceMismatches(solve, &published)
	expected := fmt.Sprintf("value-summary subject %x publishes an answer the receipt path did not", lanes.ValueSummary[corrupted].Subject[:4])
	if len(findings) != 1 || findings[0] != expected {
		t.Fatalf("a corrupted subject lane produced %v, not the single finding %q", findings, expected)
	}
}
