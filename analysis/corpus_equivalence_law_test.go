package analysis

import (
	"fmt"
	"testing"

	"context"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// This file is the corpus-backed equivalence law: on real solves, the answers a
// published composition hands out are the answers the executor materialized.
//
// Each declared row is read from the sealed Snapshot by its stable publication
// key, and the typed answer is checked against the owning domain's shape.
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
	record composite.LinkInputs
	// binding is the publication's write authority: the sealed hot binding
	// whose open phase admitted every column this solve publishes.
	binding     *composite.ProgramBinding
	schema      *valuedomain.Schema
	algebra     *effectfactor.Algebra
	coordinates []compiledValueCoordinate
	summaries   []summaryAnswer
	effects     []effectAnswer
	points      []equivalencePoint
	published   snapshot.Snapshot
	queryFamily identity.ContentID
	// geometry, observations and observationFamily are the observation plane of
	// the same solve: the mount-qualified subjects the compiled geometry declares,
	// the publication rows the solver attached for them, and the family a
	// consumer opens to read them.
	geometry          resultGeometry
	observations      []artifactDiagnosticObservationPublication
	observationFamily identity.ContentID
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
	solver, queryPlan, observations, failure, compiled := state.buildRuntimeSolver(nil)
	if !compiled || solver == nil || queryPlan == nil || failure.Available() {
		t.Fatalf("the plan built no runtime solver: %v", failure)
	}
	engineState, solveStatus, _ := solver.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
	if solveStatus != engine.SolveComplete || engineState == nil {
		t.Fatalf("solve = %v", solveStatus)
	}
	sealed, sealedOK := solver.PublishedSnapshot(engineState)
	published := sealed.Snapshot()
	if !sealedOK || !sealed.QueryFamily().Available() {
		t.Fatal("solve sealed no published query family")
	}
	queryRead, queryReadOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.QueryFamily())
	if !queryReadOK {
		t.Fatal("solve sealed no readable query family")
	}
	coordinates, coordinatesOK := compileValueCoordinates(linked)
	if !coordinatesOK {
		t.Fatal("the Link publishes no value coordinate order")
	}
	solve := &receiptSolve{
		record:      record,
		binding:     binding,
		schema:      binding.ValueSchema(),
		algebra:     record.EffectAlgebra,
		coordinates: coordinates,
		published:   published,
		queryFamily: sealed.QueryFamily(),

		geometry:          mustResultGeometry(t, state),
		observations:      observations,
		observationFamily: sealed.ObservationFamily(),
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
		rowKey, keyed := query.PublicationKey()
		if !keyed {
			t.Fatalf("query row at point %x has no publication key", row.point[:4])
		}
		captured, capturedStatus := snapshot.Query(&published, queryRead, rowKey)
		if capturedStatus != snapshot.ReadHit && capturedStatus != snapshot.ReadProvenAbsent {
			t.Fatalf("the published query at point %x reads as %s", row.point[:4], capturedStatus)
		}
		switch row.role {
		case artifactQueryValueSummary:
			answer := valuedomain.BeginValueSummary(solve.schema)
			if captured.Available() {
				var readable bool
				answer, readable = engine.AnswerValue[valuedomain.ValueSummaryObservation](captured)
				if !readable || !answer.Valid {
					t.Fatalf("the executor answer at point %x is not a value summary", row.point[:4])
				}
			}
			if len(answer.Values) != len(answer.Present) || len(answer.Values) != solve.schema.CoordinateCount() {
				t.Fatalf("the value-summary receipt at point %x is not shaped by the sealed coordinate width", row.point[:4])
			}
			solve.summaries = append(solve.summaries, summaryAnswer{subject: rowKey, point: point, answer: answer})
		case artifactQueryEffectExact:
			answer := effectfactor.BeginEffect(solve.algebra)
			if captured.Available() {
				var readable bool
				answer, readable = engine.AnswerValue[effectfactor.EffectObservation](captured)
				if !readable || !answer.Valid {
					t.Fatalf("the executor answer at point %x is not an effect observation", row.point[:4])
				}
			}
			if answer.Rows == 0 {
				solve.effects = append(solve.effects, effectAnswer{subject: rowKey, point: point, answer: answer})
				continue
			}
			root, rootOK := solve.rootForPoint(state, point)
			if !rootOK {
				t.Fatalf("the effect algebra issues no root for the body of point %x", row.point[:4])
			}
			solve.effects = append(solve.effects, effectAnswer{subject: rowKey, point: point, root: root, answer: answer})
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
// the root of the body the result geometry attaches that point to.
func (solve *receiptSolve) rootForPoint(state *compiledState, point equivalencePoint) (effectfactor.Root, bool) {
	geometry, geometryOK := state.resultGeometry()
	if !geometryOK {
		return effectfactor.Root{}, false
	}
	bodies := geometry.pointBodies[artifactResultPoint{mount: point.mount, point: point.point}]
	if len(bodies) == 0 {
		return effectfactor.Root{}, false
	}
	body := geometry.bodies[bodies[0]]
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

type equivalenceCoverage struct {
	summaryAnswers  int
	exactAnswers    int
	summaryAbsences int
	exactAbsences   int
}

func (coverage equivalenceCoverage) String() string {
	return fmt.Sprintf("value-summary answered=%d absent=%d | effect-exact answered=%d absent=%d", coverage.summaryAnswers, coverage.summaryAbsences, coverage.exactAnswers, coverage.exactAbsences)
}

func committedFamilyMismatches(solve *receiptSolve) ([]string, equivalenceCoverage) {
	var coverage equivalenceCoverage
	findings := make([]string, 0)
	findings = append(findings, summaryFamilyMismatches(solve, &solve.published, &coverage)...)
	findings = append(findings, exactFamilyMismatches(solve, &solve.published, &coverage)...)
	return findings, coverage
}

func summaryFamilyMismatches(solve *receiptSolve, published *snapshot.Snapshot, coverage *equivalenceCoverage) []string {
	findings := make([]string, 0)
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](published, solve.queryFamily)
	if !opens {
		return append(findings, "the committed query family opens no published result column")
	}
	for _, summary := range solve.summaries {
		wrapped, status := snapshot.Query(published, plan, summary.subject)
		answer, readable := engine.AnswerValue[valuedomain.ValueSummaryObservation](wrapped)
		if status == snapshot.ReadHit && !readable {
			findings = append(findings, fmt.Sprintf("value-summary subject %x is not a value-summary answer", summary.subject[:4]))
			continue
		}
		if summary.answer.Rows == 0 {
			if status == snapshot.ReadProvenAbsent {
				coverage.summaryAbsences++
				continue
			}
			findings = append(findings, fmt.Sprintf("value-summary subject %x reads back as %s, and the receipt path folded no row for it", summary.subject[:4], status))
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
	if _, status := snapshot.Query(published, plan, identity.ContentID{0xff}); status != snapshot.ReadMiss {
		findings = append(findings, fmt.Sprintf("a subject the committed query family was never asked at reads back as %s, not a miss", status))
	}
	return findings
}

func exactFamilyMismatches(solve *receiptSolve, published *snapshot.Snapshot, coverage *equivalenceCoverage) []string {
	findings := make([]string, 0)
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](published, solve.queryFamily)
	if !opens {
		return append(findings, "the committed query family opens no published result column")
	}
	for _, effect := range solve.effects {
		wrapped, status := snapshot.Query(published, plan, effect.subject)
		answer, readable := engine.AnswerValue[effectfactor.EffectObservation](wrapped)
		if status == snapshot.ReadHit && !readable {
			findings = append(findings, fmt.Sprintf("effect-exact subject %x is not an effect-exact answer", effect.subject[:4]))
			continue
		}
		if effect.answer.Rows == 0 {
			if status == snapshot.ReadProvenAbsent {
				coverage.exactAbsences++
				continue
			}
			findings = append(findings, fmt.Sprintf("effect-exact subject %x reads back as %s, and the receipt path folded no row for it", effect.subject[:4], status))
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
	if _, status := snapshot.Query(published, plan, identity.ContentID{0xfe}); status != snapshot.ReadMiss {
		findings = append(findings, fmt.Sprintf("a subject the committed query family was never asked at reads back as %s, not a miss", status))
	}
	return findings
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
			findings, coverage := committedFamilyMismatches(solve)
			for _, finding := range findings {
				t.Error(finding)
			}
			t.Log(coverage)
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

// TestEquivalenceReportNamesAWithdrawnValueSummarySubject proves the reporting
// half of the equivalence law rather than the equivalence itself: one committed
// value-summary answer is withdrawn from the publication, and the mismatch
// report must name that exact subject as a single finding rather than pass, name
// another subject, or bury it among others.
func TestEquivalenceReportNamesAWithdrawnValueSummarySubject(t *testing.T) {
	solve := solveThroughReceipts(t, equivalenceCorpus[1].link(t))
	var subject identity.ContentID
	for _, summary := range solve.summaries {
		if summary.answer.Rows != 0 {
			subject = summary.subject
			break
		}
	}
	if !subject.Available() {
		t.Fatal("the fixture holds no answered value-summary subject to withdraw")
	}
	corrupted := withdrawCommittedSubject(t, solve, subject)
	findings, _ := committedFamilyMismatchesAgainst(solve, corrupted)
	expected := fmt.Sprintf("value-summary subject %x reads back as proven-absent, not the answer the receipt path published", subject[:4])
	if len(findings) != 1 || findings[0] != expected {
		t.Fatalf("a withdrawn committed row produced %v, not the single finding %q", findings, expected)
	}
}

// TestEquivalenceReportNamesAWithdrawnEffectExactSubject is the same reporting
// proof on the effect-exact lane: one committed answer is withdrawn and the
// mismatch report must name that exact subject as its single finding.
func TestEquivalenceReportNamesAWithdrawnEffectExactSubject(t *testing.T) {
	solve := solveThroughReceipts(t, equivalenceCorpus[1].link(t))
	var subject identity.ContentID
	for _, effect := range solve.effects {
		if effect.answer.Rows != 0 {
			subject = effect.subject
			break
		}
	}
	if !subject.Available() {
		t.Fatal("the fixture holds no answered effect-exact subject to withdraw")
	}
	corrupted := withdrawCommittedSubject(t, solve, subject)
	findings, _ := committedFamilyMismatchesAgainst(solve, corrupted)
	expected := fmt.Sprintf("effect-exact subject %x reads back as proven-absent, not the answer the receipt path published", subject[:4])
	if len(findings) != 1 || findings[0] != expected {
		t.Fatalf("a withdrawn committed row produced %v, not the single finding %q", findings, expected)
	}
}

func committedFamilyMismatchesAgainst(solve *receiptSolve, published snapshot.Snapshot) ([]string, equivalenceCoverage) {
	clone := *solve
	clone.published = published
	return committedFamilyMismatches(&clone)
}

// TestPublishedCompositionNamesCrossPairedSubjects is MUT-1 on a real solve.
// Two captured answers are published under each other's declaration-derived
// key. The expected operand is the executor capture, so the swap is visible.
func TestPublishedCompositionNamesCrossPairedSubjects(t *testing.T) {
	solve := solveThroughReceipts(t, equivalenceCorpus[1].link(t))
	var left, right summaryAnswer
	found := 0
	for _, summary := range solve.summaries {
		if summary.answer.Rows == 0 {
			continue
		}
		if found == 0 {
			left = summary
			found++
			continue
		}
		if !valuedomain.EqualValueSummary(solve.schema, summary.answer, left.answer) {
			right = summary
			found++
			break
		}
	}
	if found < 2 {
		t.Fatal("the fixture holds no two distinct answered value-summary subjects to cross-pair")
	}
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&solve.published, solve.queryFamily)
	if !opens {
		t.Fatal("the committed query family opens no published result column")
	}
	leftAnswer, leftStatus := snapshot.Query(&solve.published, plan, left.subject)
	rightAnswer, rightStatus := snapshot.Query(&solve.published, plan, right.subject)
	if leftStatus != snapshot.ReadHit || rightStatus != snapshot.ReadHit {
		t.Fatalf("the two subjects read back as %v/%v", leftStatus, rightStatus)
	}
	delta := snapshot.NewDelta(solve.published, solve.published.Generation().Next())
	if err := snapshot.SetRow(&delta, plan.Axis(), left.subject, rightAnswer); err != nil {
		t.Fatalf("cross-pair the left subject: %v", err)
	}
	if err := snapshot.SetRow(&delta, plan.Axis(), right.subject, leftAnswer); err != nil {
		t.Fatalf("cross-pair the right subject: %v", err)
	}
	crossed, err := delta.Seal()
	if err != nil || !crossed.Published() {
		t.Fatalf("seal the cross-paired publication: %v", err)
	}
	findings, _ := committedFamilyMismatchesAgainst(solve, crossed)
	expected := []string{
		fmt.Sprintf("value-summary subject %x publishes an answer the receipt path did not", left.subject[:4]),
		fmt.Sprintf("value-summary subject %x publishes an answer the receipt path did not", right.subject[:4]),
	}
	if len(findings) != 2 {
		t.Fatalf("a cross-paired publication produced %v, want %v", findings, expected)
	}
	seen := map[string]int{}
	for _, finding := range findings {
		seen[finding]++
	}
	for _, want := range expected {
		if seen[want] != 1 {
			t.Fatalf("a cross-paired publication produced %v, missing %q", findings, want)
		}
	}
}

func withdrawCommittedSubject(t *testing.T, solve *receiptSolve, subject identity.ContentID) snapshot.Snapshot {
	t.Helper()
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&solve.published, solve.queryFamily)
	if !opens {
		t.Fatal("the committed query family opens no published result column")
	}
	delta := snapshot.NewDelta(solve.published, solve.published.Generation().Next())
	if err := snapshot.RemoveRow(&delta, plan.Axis(), subject); err != nil {
		t.Fatalf("withdraw committed subject: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil || !sealed.Published() {
		t.Fatalf("seal withdrawn publication: %v", err)
	}
	return sealed
}
