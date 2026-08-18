package analysis

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// This file is the observation plane of a real solve. The result projection
// consumes the observation column through publishedObservation, and nothing else
// in this package states what that read must answer.
//
// The law reads the plane exactly as production reads it and holds it against
// two authorities the observation column did not produce: the declared geometry
// topology, which is where an evidence point's subject and row key both come
// from, and this file's own statement of what each authored guard condition is.
// Both are pinned to the source the corpus compiles, so a row published under a
// neighboring subject's key answers the wrong guard instead of agreeing with
// itself.
//
// The value lane of the same solve is deliberately not the comparand. A branch
// observation is materialized at its producer's execution point, and that point
// lies outside the mounted point universe the value-summary family is asked at,
// so the two lanes answer different points and their agreement is no law.

// observationPlaneSource is the corpus body: two complete guards whose
// conditions are two distinct exact literals at two distinct coordinates. Two
// subjects are the minimum a pairing law needs, and authoring both arms keeps
// every mounted point the solve is asked at attributable to a body.
const observationPlaneSource = "local a = 1\nlocal b = 2\nlocal c = 0\nif a then c = 1 else c = 2 end\nif b then c = 3 else c = 4 end\nreturn c\n"

// observationPlaneConditions is what the source states each guard tests, keyed
// by the line the guard is authored on: line 4 tests the integer 1 and line 5
// tests the integer 2.
var observationPlaneConditions = map[uint32]int64{4: 1, 5: 2}

var observationPlaneCorpus = equivalenceLinkFor(observationPlaneSource)

// declaredEvidencePoint is one branch-condition evidence point as the compiled
// geometry declares it: the mounted execution point the condition is observed at,
// the anchor the publication row is filed under, the coordinate the condition
// occupies, the line the guard is authored on, and the observation key the
// declared attachment derives.
type declaredEvidencePoint struct {
	mount      identity.ContentID
	execution  identity.ContentID
	anchor     identity.ContentID
	valueIndex uint32
	line       uint32
	key        identity.ContentID
}

// declaredEvidencePoints re-derives the observation plane from the geometry
// alone. The key is the declared attachment identity of the mounted execution
// point, so no part of the expected pairing is read out of the publication under
// test.
func declaredEvidencePoints(t *testing.T, solve *receiptSolve) []declaredEvidencePoint {
	t.Helper()
	if solve == nil || !solve.geometry.valid() {
		t.Fatal("the solve carries no result geometry")
	}
	declared := make([]declaredEvidencePoint, 0)
	for _, subject := range solve.geometry.branchObservations {
		if subject.kind != structure.DiagnosticObservationBranchCondition || !subject.available() {
			t.Fatal("the geometry declares an unavailable branch observation")
		}
		for _, producer := range subject.producers {
			key, derived := identity.DeriveContentID("analysis/branch-value-observation/v1", subject.mount[:], producer.point[:], []byte("value-summary"))
			if !derived {
				t.Fatal("the declared evidence point derives no observation key")
			}
			declared = append(declared, declaredEvidencePoint{
				mount: subject.mount, execution: producer.point, anchor: producer.anchor,
				valueIndex: subject.valueIndex, line: subject.location.StartLine, key: key,
			})
		}
	}
	return declared
}

// consumedObservationMismatches reads every declared evidence point through the
// production accessor and states what that read answers: the row is filed at the
// subject the geometry declares, it is shaped by the sealed coordinate width, and
// the guard condition it carries is the condition the source authors on that
// guard's own line.
func consumedObservationMismatches(solve *receiptSolve, declared []declaredEvidencePoint, published *snapshot.Snapshot) ([]string, int) {
	findings := make([]string, 0)
	compared := 0
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](published, solve.observationFamily)
	if !opens {
		return append(findings, "the solve publishes no observation column"), 0
	}
	for _, point := range declared {
		filed := 0
		for _, row := range solve.observations {
			if row.key != point.key {
				continue
			}
			filed++
			if row.point.mount != point.mount || row.point.point != point.anchor {
				findings = append(findings, fmt.Sprintf("evidence point %x is filed at a subject the geometry does not declare it at", point.key[:4]))
			}
		}
		if filed != 1 {
			findings = append(findings, fmt.Sprintf("evidence point %x is published by %d rows of the observation plan", point.key[:4], filed))
			continue
		}
		expected, stated := observationPlaneConditions[point.line]
		if !stated {
			findings = append(findings, fmt.Sprintf("evidence point %x is authored on line %d, which this law states no condition for", point.key[:4], point.line))
			continue
		}
		observation, readable := publishedObservation[valuedomain.ValueSummaryObservation](published, plan, point.key)
		if !readable || !observation.Valid {
			findings = append(findings, fmt.Sprintf("evidence point %x is unreadable through the production accessor", point.key[:4]))
			continue
		}
		if len(observation.Values) != len(observation.Present) || len(observation.Values) != solve.schema.CoordinateCount() || observation.Rows > 1 {
			findings = append(findings, fmt.Sprintf("evidence point %x is not shaped by the sealed coordinate width", point.key[:4]))
			continue
		}
		index := int(point.valueIndex)
		if index >= len(observation.Values) {
			findings = append(findings, fmt.Sprintf("evidence point %x names a coordinate outside the sealed width", point.key[:4]))
			continue
		}
		if observation.Rows == 0 || !observation.Present[index] {
			findings = append(findings, fmt.Sprintf("the guard on line %d publishes no condition value, and its condition is authored as a literal", point.line))
			continue
		}
		condition, exact := observedIntegerCondition(solve.schema, observation.Values[index])
		if !exact {
			findings = append(findings, fmt.Sprintf("the guard on line %d publishes no exact integer condition", point.line))
			continue
		}
		if condition != expected {
			findings = append(findings, fmt.Sprintf("the guard on line %d publishes the condition %d, and the source authors %d", point.line, condition, expected))
			continue
		}
		compared++
	}
	for _, row := range solve.observations {
		declaredRow := false
		for _, point := range declared {
			if row.key == point.key {
				declaredRow = true
				break
			}
		}
		if !declaredRow {
			findings = append(findings, fmt.Sprintf("the observation plan publishes row %x, which the geometry declares no evidence point for", row.key[:4]))
		}
	}
	return findings, compared
}

// observedIntegerCondition reads the exact integer a published condition value
// carries. A condition the schema does not prove exact is no operand for a law
// stated in literals.
func observedIntegerCondition(schema *valuedomain.Schema, value valuedomain.Value) (int64, bool) {
	scalar, exact := schema.ExactScalar(value)
	if !exact {
		return 0, false
	}
	literal, held := scalar.Literal()
	if !held || literal.Kind != keyspace.LiteralInteger {
		return 0, false
	}
	return literal.Integer, true
}

// TestConsumedObservationsAnswerTheAuthoredGuardConditions is the
// observation-plane law. On a real solve, every branch-condition evidence point
// the geometry declares is readable through the accessor the result projection
// uses, is filed at the subject the geometry declares for it, and carries the
// guard condition the source authors on that guard's own line.
func TestConsumedObservationsAnswerTheAuthoredGuardConditions(t *testing.T) {
	solve := solveThroughReceipts(t, observationPlaneCorpus(t))
	declared := declaredEvidencePoints(t, solve)
	if len(declared) != len(observationPlaneConditions) {
		t.Fatalf("the corpus declares %d branch evidence points for %d authored guards", len(declared), len(observationPlaneConditions))
	}
	if len(solve.observations) != len(declared) {
		t.Fatalf("the solve published %d observation rows for %d declared evidence points", len(solve.observations), len(declared))
	}
	findings, compared := consumedObservationMismatches(solve, declared, &solve.published)
	for _, finding := range findings {
		t.Error(finding)
	}
	if compared != len(observationPlaneConditions) {
		t.Fatalf("%d of %d authored guard conditions were compared", compared, len(observationPlaneConditions))
	}
}

// TestConsumedObservationsCollectThroughTheProductionReader is the consumption
// half: the guard-polarity collector reads this plane on the publication the
// solve sealed without a collection failure, and it names a withdrawn evidence
// point rather than collecting a report from a plane missing a declared subject.
func TestConsumedObservationsCollectThroughTheProductionReader(t *testing.T) {
	solve := solveThroughReceipts(t, observationPlaneCorpus(t))
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&solve.published, solve.observationFamily)
	if !opens {
		t.Fatal("the solve publishes no observation column")
	}
	report := &DiagnosticReport{findings: make([]diagnosticFinding, 0)}
	if !collectGuardPolarityFindings(report, solve.geometry, solve.observations, solve.schema, &solve.published, plan, FindingSeverityWarning, FindingSeverityWarning) {
		t.Fatal("the guard-polarity reader refused the sealed observation column")
	}
	if report.collectionFailure != DiagnosticCollectionOK {
		t.Fatalf("the guard-polarity reader reported %v on the publication the solve sealed", report.collectionFailure)
	}

	declared := declaredEvidencePoints(t, solve)
	withdrawn := withdrawObservationRow(t, solve, plan, declared[0].key)
	withdrawnReport := &DiagnosticReport{findings: make([]diagnosticFinding, 0)}
	if !collectGuardPolarityFindings(withdrawnReport, solve.geometry, solve.observations, solve.schema, &withdrawn, plan, FindingSeverityWarning, FindingSeverityWarning) {
		t.Fatal("the guard-polarity reader refused a withdrawn observation column outright")
	}
	if withdrawnReport.collectionFailure != DiagnosticCollectionQueryUnreadable {
		t.Fatalf("a withdrawn evidence point collected as %v, want an unreadable query", withdrawnReport.collectionFailure)
	}
	findings, _ := consumedObservationMismatches(solve, declared, &withdrawn)
	expected := fmt.Sprintf("evidence point %x is unreadable through the production accessor", declared[0].key[:4])
	if len(findings) != 1 || findings[0] != expected {
		t.Fatalf("a withdrawn evidence point produced %v, want the single finding %q", findings, expected)
	}
}

func withdrawObservationRow(t *testing.T, solve *receiptSolve, plan snapshot.QueryPlan[identity.ContentID, engine.Answer], key identity.ContentID) snapshot.Snapshot {
	t.Helper()
	delta := snapshot.NewDelta(solve.published, solve.published.Generation().Next())
	if err := snapshot.RemoveRow(&delta, plan.Axis(), key); err != nil {
		t.Fatalf("withdraw evidence point: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil || !sealed.Published() {
		t.Fatalf("seal the withdrawn observation plane: %v", err)
	}
	return sealed
}

// TestCrossPairedObservationPlaneNamesBothEvidencePoints is the red half. The two
// published evidence points are stored under each other's declared key, which is
// the corruption a reader that takes both its subject and its expectation from
// the observation column cannot see. The law must name both guards, and it must
// name them by the condition the source authors.
func TestCrossPairedObservationPlaneNamesBothEvidencePoints(t *testing.T) {
	solve := solveThroughReceipts(t, observationPlaneCorpus(t))
	declared := declaredEvidencePoints(t, solve)
	if len(declared) != 2 {
		t.Fatalf("the corpus declares %d branch evidence points, want 2", len(declared))
	}
	baseFindings, compared := consumedObservationMismatches(solve, declared, &solve.published)
	if len(baseFindings) != 0 || compared != 2 {
		t.Fatalf("the sealed observation plane reports %v with %d comparisons", baseFindings, compared)
	}
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&solve.published, solve.observationFamily)
	if !opens {
		t.Fatal("the solve publishes no observation column")
	}
	left, right := declared[0], declared[1]
	leftAnswer, leftStatus := snapshot.Query(&solve.published, plan, left.key)
	rightAnswer, rightStatus := snapshot.Query(&solve.published, plan, right.key)
	if leftStatus != snapshot.ReadHit || rightStatus != snapshot.ReadHit || leftAnswer.Equal(rightAnswer) {
		t.Fatalf("the two evidence points read back as %v/%v and are indistinguishable: %t", leftStatus, rightStatus, leftAnswer.Equal(rightAnswer))
	}
	delta := snapshot.NewDelta(solve.published, solve.published.Generation().Next())
	if err := snapshot.SetRow(&delta, plan.Axis(), left.key, rightAnswer); err != nil {
		t.Fatalf("cross-pair the left evidence point: %v", err)
	}
	if err := snapshot.SetRow(&delta, plan.Axis(), right.key, leftAnswer); err != nil {
		t.Fatalf("cross-pair the right evidence point: %v", err)
	}
	crossed, err := delta.Seal()
	if err != nil || !crossed.Published() {
		t.Fatalf("seal the cross-paired observation plane: %v", err)
	}
	// The two guards test different coordinates, so the swap is caught at the
	// later guard: its condition is not established at the earlier guard's point,
	// so the row it now holds answers nothing at that coordinate. The other
	// direction reads a coordinate both points agree on - the earlier condition is
	// unchanged at the later guard - so this corpus proves one directed detection
	// and one comparison lost rather than two findings.
	later := left.line
	if right.line > later {
		later = right.line
	}
	findings, crossedComparisons := consumedObservationMismatches(solve, declared, &crossed)
	expected := fmt.Sprintf("the guard on line %d publishes no condition value, and its condition is authored as a literal", later)
	if len(findings) != 1 || findings[0] != expected || crossedComparisons != 1 {
		t.Fatalf("a cross-paired observation plane produced %v with %d comparisons, want the single finding %q", findings, crossedComparisons, expected)
	}
	if unchanged, restored := consumedObservationMismatches(solve, declared, &solve.published); len(unchanged) != 0 || restored != 2 {
		t.Fatalf("the cross-paired derivation edited the sealed publication: %v with %d comparisons", unchanged, restored)
	}
}
