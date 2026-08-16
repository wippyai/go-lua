package analysis

import (
	"fmt"
	"strings"
	"testing"
)

// TestAnchorTypesNarrowingDiagnosticsPlacement is the standing anchor of the
// analyzer. Two corpus fixtures run the full production acceptance path -
// load, seal, compile with diagnostics, solve, judge the source-authored
// manifest - and every clause below is asserted in order and reported
// individually, so the anchor states what "done" means rather than what
// currently passes. A clause that is red today stays asserted and red; it is
// never skipped, capped, or relaxed.
//
// Fixture one anchors discriminated-union narrowing, its one deliberate
// diagnostic with a complete evidence chain, and the placement plane. Fixture
// two anchors recursive type identity, coinductive convergence, and recursive
// narrowing.
func TestAnchorTypesNarrowingDiagnosticsPlacement(t *testing.T) {
	t.Run("multivariant-dispatch", anchorMultivariantDispatch)
	t.Run("recursive-union", anchorRecursiveUnion)
}

// anchorAcceptanceMode is the acceptance mode without its aggregate verdict.
// The spine, the fixture preflight, and the derived diagnostic policy are the
// production acceptance path; the anchor supplies the clause-by-clause
// judgment in place of the single acceptance mismatch list.
func anchorAcceptanceMode() corpusHarnessMode {
	mode := corpusSemanticAcceptanceMode()
	mode.name = "anchor"
	mode.judge = nil
	return mode
}

func anchorRun(t *testing.T, name string) *corpusHarnessRun {
	t.Helper()
	run, class, err := corpusHarnessExecute(t, corpusHarnessFixture(t, name), anchorAcceptanceMode())
	// Clause 1: the fixture compiles and its solve reaches AnalyzeComplete.
	if err != nil {
		t.Fatalf("anchor clause 1 (compile and AnalyzeComplete) failed at %s: %v", class, err)
	}
	if run.report == nil || !run.report.Available() {
		t.Fatalf("anchor clause 1: acceptance policy produced no available DiagnosticReport")
	}
	if failure := run.report.CollectionFailure(); failure != DiagnosticCollectionOK {
		t.Fatalf("anchor clause 1: DiagnosticReport collection failure=%d", failure)
	}
	return run
}

// anchorFinding is one public report row reduced to its judged identity.
type anchorFinding struct {
	finding      Finding
	file         string
	line, column uint32
	severity     FindingSeverity
	code, help   string
	message      string
}

func anchorFindings(t *testing.T, run *corpusHarnessRun) []anchorFinding {
	t.Helper()
	findings := make([]anchorFinding, 0, run.report.FindingCount())
	for index := 0; index < run.report.FindingCount(); index++ {
		finding, findingOK := run.report.FindingAt(index)
		location, locationOK := finding.Location()
		if !findingOK || !locationOK {
			t.Fatalf("report finding %d has no complete public identity", index)
		}
		line, column := location.Start()
		findings = append(findings, anchorFinding{
			finding: finding, file: location.File(), line: line, column: column,
			severity: finding.Severity(), code: finding.Code().String(), help: finding.Help(), message: finding.Message(),
		})
	}
	return findings
}

func anchorRejectFindingsOutside(t *testing.T, clause string, findings []anchorFinding, line uint32) {
	t.Helper()
	for _, finding := range findings {
		if finding.line == line {
			continue
		}
		t.Errorf("anchor clause %s: clean code at %s:%d:%d emitted %s %s: %s", clause, finding.file, finding.line, finding.column, finding.severity, finding.code, finding.message)
	}
}

// anchorMultivariantDispatch anchors the three-variant discriminated union:
// each render arm narrows its shared raw field through the compound
// type()/discriminant guard and type-checks cleanly, and only the deliberate
// misuse is rejected - with the full proven/claimed evidence chain and the
// rendered diagnostic the fixture manifest pins.
func anchorMultivariantDispatch(t *testing.T) {
	run := anchorRun(t, "narrowing/type-eq-multivariant-dispatch")
	expectation := run.project.expectation
	if expectation.manifest == nil || expectation.manifest.Check == nil || len(expectation.manifest.Check.Diagnostics) != 1 {
		t.Fatal("anchor fixture no longer pins exactly one structured diagnostic")
	}
	want := expectation.manifest.Check.Diagnostics[0]
	if want.Code != "type.call.direct.argument_type" || want.Line != 36 || want.Column != 22 || want.Severity != "error" {
		t.Fatalf("anchor fixture manifest no longer pins the misuse diagnostic: %+v", want)
	}
	findings := anchorFindings(t, run)

	// Clause 2: the three clean render arms and the narrowed total loop emit
	// nothing. Every row of the report must be the one pinned misuse.
	anchorRejectFindingsOutside(t, "2 (clean narrowing arms)", findings, uint32(want.Line))

	// Clause 3: exactly the pinned diagnostic fires, with the manifest's
	// message, evidence chain, labels, and ordered render.
	if len(run.policyUnsupported) != 0 {
		t.Errorf("anchor clause 3: acceptance policy cannot express the pinned contract:\n%s", strings.Join(run.policyUnsupported, "\n"))
	}
	matched := make([]anchorFinding, 0, 1)
	for _, finding := range findings {
		if finding.code == want.Code && finding.line == uint32(want.Line) && finding.column == uint32(want.Column) && finding.severity == corpusDiagnosticSeverity(want.Severity) {
			matched = append(matched, finding)
		}
	}
	if len(matched) != 1 {
		t.Errorf("anchor clause 3: %s at %s:%d:%d matched %d report rows, want exactly 1", want.Code, want.File, want.Line, want.Column, len(matched))
	} else {
		got := matched[0]
		for _, part := range want.MessageContains {
			if !strings.Contains(got.message, part) {
				t.Errorf("anchor clause 3: message %q is missing %q", got.message, part)
			}
		}
		for _, part := range want.HelpContains {
			if !strings.Contains(got.help, part) {
				t.Errorf("anchor clause 3: help %q is missing %q", got.help, part)
			}
		}
		// Evidence, labels, and render are judged exactly as the acceptance
		// mode judges them, including its stricter file matching.
		details := corpusDiagnosticFamilyResult{}
		matchCorpusDiagnosticDetails(&details, want, got.finding, got.file, func(file string) (string, bool) {
			return string(corpusHarnessSourceText(t, run.project, corpusDiagnosticProjectSourceFile(expectation, file))), true
		})
		for _, mismatch := range details.Mismatches {
			t.Errorf("anchor clause 3: %s", mismatch)
		}
	}

	// Clause 4: the placement plane. This is the architect's anchor
	// expectation, not an aspirational skip: Result must publish a solved
	// placement projection for this fixture. It is unavailable today, so this
	// clause is a standing red that defines done.
	if run.result.placement == nil || !run.result.placement.valid() {
		t.Error("anchor clause 4: placement Result surface is unavailable; a solved placement projection is required")
	}
}

// anchorRecursiveUnion anchors the recursive Json union: the mu-type recurses
// through both the array and the map constructor, so reaching AnalyzeComplete
// within the bounded budget is itself the coinductive-convergence proof.
func anchorRecursiveUnion(t *testing.T) {
	// Clause 1: compile and solve complete. A diverging coinductive walk
	// cannot pass this line: the bounded runner kills the process instead.
	run := anchorRun(t, "semantic/recursive-json-union-validator")
	expectation := run.project.expectation
	if expectation.manifest != nil && expectation.manifest.Check != nil && len(expectation.manifest.Check.Diagnostics) != 0 {
		t.Fatal("anchor fixture no longer judges its inline expectation alone")
	}
	if len(expectation.inline) != 1 {
		t.Fatalf("anchor fixture pins %d inline expectations, want exactly the recursive-union assignment error", len(expectation.inline))
	}
	inline := expectation.inline[0]
	if inline.Line != 22 || inline.Severity != "error" {
		t.Fatalf("anchor fixture inline expectation moved: %+v", inline)
	}
	findings := anchorFindings(t, run)

	// Clause 2: the guarded narrowing chain is clean. is_scalar plus the
	// type(doc) == "table" gate, the doc.title access, and the nested
	// type(title) == "string" narrowing to a string local emit nothing.
	anchorRejectFindingsOutside(t, "2 (guarded recursive narrowing)", findings, uint32(inline.Line))

	// Clause 3: the inline expectation fires exactly once, judged through the
	// same inline matcher the acceptance mode uses.
	matched := 0
	for _, finding := range findings {
		if corpusSemanticInlineMatch(expectation, inline, corpusSemanticAcceptanceFinding{
			file: finding.file, line: finding.line, column: finding.column, severity: finding.severity, code: finding.code, message: finding.message,
		}) {
			matched++
		}
	}
	if matched != 1 {
		t.Errorf("anchor clause 3: inline %s at %s:%d matched %d report rows, want exactly 1; report=%s", inline.Severity, inline.File, inline.Line, matched, anchorReportSummary(findings))
	}
}

func anchorReportSummary(findings []anchorFinding) string {
	if len(findings) == 0 {
		return "no findings"
	}
	rows := make([]string, 0, len(findings))
	for _, finding := range findings {
		rows = append(rows, fmt.Sprintf("%s %s at %s:%d:%d", finding.severity, finding.code, finding.file, finding.line, finding.column))
	}
	return strings.Join(rows, "; ")
}
