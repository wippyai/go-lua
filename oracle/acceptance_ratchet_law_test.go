package oracle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wippyai/go-lua/analysis"
)

// The acceptance ratchet is the census ratchet's judgment applied to the other
// half of the corpus contract. The census states whether a fixture analyzed;
// acceptance states whether the analysis said what the fixture's source says it
// must. Between two red acceptance runs nothing distinguishes progress from
// regression, so a landing that silently stops emitting forty findings reads
// the same as the landing before it.
//
// A checked-in high-water mark closes that half. It records, per fixture
// family, the best acceptance this corpus has produced; the law asserts the
// current run is at least that good. Lowering a mark is a deliberate edit of
// the data file made by whoever measured the better run, which is what makes
// the number evidence rather than a moving average.
//
// # Why unmet expectations, not failures
//
// A fixture that never reaches the acceptance comparison - its contract is one
// the declaration table cannot express, or its report was unavailable - has met
// none of its expectations. Counting it as zero missing rows would make the
// mark rise every time another lane repaired a compile path, which is the exact
// inversion a ratchet exists to prevent. Such a fixture therefore contributes
// its whole expectation set as missing, and the counts stay monotone under a
// repair anywhere else in the analyzer.
//
// The failure path is retained beside that unmet expectation count. The old
// ratchet collapsed every pre-verdict outcome into one unjudged number, which
// made an unsupported declaration, an invalid source, and an engine refusal
// indistinguishable. Unjudged remains the umbrella used by the checked-in
// high-water mark; explicit, mutually exclusive reason categories now explain
// every unjudged row in the receipt.
//
// Unexpected findings are the one column that cannot be attributed that way: a
// fixture that never reported cannot be known to over-report. A family whose
// unjudged count falls may therefore raise its unexpected count without any
// regression having happened, and the law says so in its failure message: the
// answer is a deliberate re-mint from the measured run, never a silently
// widened ceiling.

// acceptanceHighWater is the checked-in mark. Counts are absolute row counts
// over the family they describe, so a mark cannot be satisfied by a corpus that
// shrank.
type acceptanceHighWater struct {
	Measured string                            `json:"measured"`
	Note     string                            `json:"note"`
	Projects int                               `json:"corpus_projects"`
	Families map[string]acceptanceHighWaterRow `json:"families"`
}

// acceptanceHighWaterRow is one fixture family's recorded mark.
//
// CleanMin is a floor: the fixtures whose every source-authored expectation was
// met. The other four are ceilings. Projects pins the family's size, so a mark
// measured over a family that has since gained or lost fixtures is reported as
// not comparable rather than silently applied.
type acceptanceHighWaterRow struct {
	Projects            int `json:"projects"`
	CleanMin            int `json:"clean_min"`
	UnjudgedMax         int `json:"unjudged_max"`
	MissingInlineMax    int `json:"missing_inline_max"`
	MissingStructureMax int `json:"missing_structured_max"`
	UnexpectedMax       int `json:"unexpected_max"`
}

const acceptanceHighWaterPath = "oracle/testdata/acceptance_highwater.json"

// acceptanceTally is one fixture's or one family's acceptance count.
type acceptanceTally struct {
	projects int
	clean    int
	unjudged int
	// unsupported is the explicit contract-refusal subset retained by the old
	// receipt: preflight and policy refusal. The reason counters below are
	// mutually exclusive children of unjudged and include compile/solve refusals
	// that are not contract support claims.
	unsupported          int
	preflightUnsupported int
	policyUnsupported    int
	compileInvalid       int
	compileUnsupported   int
	solveInvalid         int
	solveUnsupported     int
	solveIncomplete      int
	detachedResult       int
	reportUnavailable    int
	otherPreVerdict      int
	missingInline        int
	missingStructured    int
	unexpected           int
	// other is every acceptance row that is none of the above: a structured
	// row's own detail mismatch, an error-count contract, a native or placement
	// mismatch. It is reported and not gated, because the columns this ratchet
	// gates are the two the corpus is recovered against plus the clean floor.
	other int
}

func (tally *acceptanceTally) add(other acceptanceTally) {
	tally.projects += other.projects
	tally.clean += other.clean
	tally.unjudged += other.unjudged
	tally.unsupported += other.unsupported
	tally.preflightUnsupported += other.preflightUnsupported
	tally.policyUnsupported += other.policyUnsupported
	tally.compileInvalid += other.compileInvalid
	tally.compileUnsupported += other.compileUnsupported
	tally.solveInvalid += other.solveInvalid
	tally.solveUnsupported += other.solveUnsupported
	tally.solveIncomplete += other.solveIncomplete
	tally.detachedResult += other.detachedResult
	tally.reportUnavailable += other.reportUnavailable
	tally.otherPreVerdict += other.otherPreVerdict
	tally.missingInline += other.missingInline
	tally.missingStructured += other.missingStructured
	tally.unexpected += other.unexpected
	tally.other += other.other
}

func (tally acceptanceTally) unjudgedReasonCount() int {
	return tally.preflightUnsupported + tally.policyUnsupported + tally.compileInvalid + tally.compileUnsupported +
		tally.solveInvalid + tally.solveUnsupported + tally.solveIncomplete + tally.detachedResult + tally.reportUnavailable + tally.otherPreVerdict
}

// acceptanceFixtureRun is the detached information the ratchet needs when a
// fixture fails before acceptance can produce a verdict. It deliberately holds
// status/class/error rather than a live Plan or Result; the latter must already
// have been closed by corpusHarnessExecuteDetached.
type acceptanceFixtureRun struct {
	verdict     corpusSemanticAcceptanceVerdict
	reached     bool
	observed    bool
	status      analysis.AnalyzeStatus
	statusKnown bool
	class       string
	failure     string
}

type acceptancePreVerdictCategory uint8

const (
	acceptancePreVerdictUnknown acceptancePreVerdictCategory = iota
	acceptancePreflightUnsupported
	acceptanceCompileInvalid
	acceptanceCompileUnsupported
	acceptanceSolveInvalid
	acceptanceSolveUnsupported
	acceptanceSolveIncomplete
	acceptanceDetachedResult
	acceptanceReportUnavailable
	acceptanceOtherFailure
)

func (category acceptancePreVerdictCategory) String() string {
	switch category {
	case acceptancePreflightUnsupported:
		return "preflight-unsupported"
	case acceptanceCompileInvalid:
		return "compile-invalid"
	case acceptanceCompileUnsupported:
		return "compile-unsupported"
	case acceptanceSolveInvalid:
		return "solve-invalid"
	case acceptanceSolveUnsupported:
		return "solve-unsupported"
	case acceptanceSolveIncomplete:
		return "solve-incomplete"
	case acceptanceDetachedResult:
		return "detached-result"
	case acceptanceReportUnavailable:
		return "report-unavailable"
	case acceptanceOtherFailure:
		return "other-pre-verdict"
	default:
		return "unknown"
	}
}

// acceptancePreVerdictCategoryOf is intentionally based on all three pieces
// of detached harness evidence. Class names identify the phase, public status
// identifies the analyzer's contract, and the error text is retained as a
// fallback when a phase forgot to classify itself. This prevents a new failure
// from silently becoming a reasonless unjudged count.
func acceptancePreVerdictCategoryOf(run acceptanceFixtureRun) acceptancePreVerdictCategory {
	if !run.observed {
		return acceptancePreVerdictUnknown
	}
	switch run.class {
	case "fixture-contract":
		return acceptancePreflightUnsupported
	case "compile":
		if run.statusKnown && run.status == analysis.AnalyzeInvalid {
			return acceptanceCompileInvalid
		}
		return acceptanceCompileUnsupported
	case "invalid":
		return acceptanceSolveInvalid
	case "unsupported":
		return acceptanceSolveUnsupported
	case "incomplete":
		return acceptanceSolveIncomplete
	case "detached-result":
		return acceptanceDetachedResult
	case "plan-close", "workspace-close":
		return acceptanceDetachedResult
	case "report-unavailable":
		return acceptanceReportUnavailable
	}
	// A named phase that is not one of the status-bearing solve phases is
	// already a distinct failure class (link, workspace, composition, etc.).
	// Do not let a stale zero/invalid status overwrite that evidence.
	if run.class != "" {
		return acceptanceOtherFailure
	}
	if run.statusKnown && run.status == analysis.AnalyzeInvalid {
		return acceptanceSolveInvalid
	}
	if run.statusKnown && run.status == analysis.AnalyzeUnsupported {
		return acceptanceSolveUnsupported
	}
	if run.statusKnown && run.status == analysis.AnalyzeIncomplete {
		return acceptanceSolveIncomplete
	}
	if strings.Contains(run.failure, "detached-result") || strings.Contains(run.failure, "invalid source/content identity") {
		return acceptanceDetachedResult
	}
	if strings.Contains(run.failure, "compile=") {
		return acceptanceCompileUnsupported
	}
	if run.failure != "" {
		return acceptanceOtherFailure
	}
	return acceptancePreVerdictUnknown
}

// acceptanceExpectationCounts is one fixture's source-authored expectation set:
// the inline markers and the structured rows it declares. A fixture that never
// reaches the comparison is counted as missing exactly this.
func acceptanceExpectationCounts(project corpusHarnessProject) (inline, structured int) {
	expectation := project.expectation
	if expectation == nil {
		return 0, 0
	}
	inline = len(expectation.inline)
	if expectation.manifest != nil && expectation.manifest.Check != nil {
		structured = len(expectation.manifest.Check.Diagnostics)
	}
	return inline, structured
}

// acceptanceFixtureTally is retained as the small law-facing convenience
// helper. A caller that has detached run evidence should use
// acceptanceFixtureTallyOf so the pre-verdict category is not discarded.
func acceptanceFixtureTally(project corpusHarnessProject, verdict corpusSemanticAcceptanceVerdict, reached bool) acceptanceTally {
	return acceptanceFixtureTallyOf(project, acceptanceFixtureRun{
		verdict:  verdict,
		reached:  reached,
		observed: reached,
	})
}

// acceptanceFixtureTallyOf classifies one fixture's acceptance answer and the
// detached failure that preceded it, if any. Every pre-verdict path contributes
// its expectation set as missing and remains inside the legacy unjudged
// umbrella; the reason counters explain which path produced that row.
func acceptanceFixtureTallyOf(project corpusHarnessProject, run acceptanceFixtureRun) acceptanceTally {
	tally := acceptanceTally{projects: 1}
	if !run.reached {
		inline, structured := acceptanceExpectationCounts(project)
		tally.missingInline = inline
		tally.missingStructured = structured
		category := acceptancePreVerdictCategoryOf(run)
		switch category {
		case acceptancePreflightUnsupported:
			tally.preflightUnsupported = 1
		case acceptanceCompileInvalid:
			tally.compileInvalid = 1
		case acceptanceCompileUnsupported:
			tally.compileUnsupported = 1
		case acceptanceSolveInvalid:
			tally.solveInvalid = 1
		case acceptanceSolveUnsupported:
			tally.solveUnsupported = 1
		case acceptanceSolveIncomplete:
			tally.solveIncomplete = 1
		case acceptanceDetachedResult:
			tally.detachedResult = 1
		case acceptanceReportUnavailable:
			tally.reportUnavailable = 1
		case acceptanceOtherFailure:
			tally.otherPreVerdict = 1
		default:
			// Even a missing detached record gets an explicit accounting
			// reason. It remains unjudged, but cannot disappear into a
			// reasonless umbrella.
			tally.otherPreVerdict = 1
		}
		tally.unjudged = 1
		tally.unsupported = tally.preflightUnsupported
		return tally
	}
	verdict := run.verdict
	if !verdict.judged() {
		inline, structured := acceptanceExpectationCounts(project)
		tally.missingInline = inline
		tally.missingStructured = structured
		if len(verdict.unsupported) != 0 {
			tally.policyUnsupported = 1
		} else if len(verdict.unavailable) != 0 {
			tally.reportUnavailable = 1
		}
		tally.unjudged = 1
		tally.unsupported = tally.policyUnsupported
		return tally
	}
	for _, row := range verdict.diagnostics {
		switch {
		case strings.HasPrefix(row, "missing inline "):
			tally.missingInline++
		case strings.HasPrefix(row, "missing structured diagnostic "):
			tally.missingStructured++
		case strings.HasPrefix(row, "unexpected diagnostic "):
			tally.unexpected++
		default:
			tally.other++
		}
	}
	tally.other += len(verdict.native) + len(verdict.placement)
	if verdict.clean() {
		tally.clean = 1
	}
	return tally
}

// acceptanceFamily is the fixture family one project name belongs to: the first
// path segment of the canonical enumeration name, which is the same grouping
// the census ratchet's receipt reports.
func acceptanceFamily(project string) string {
	if index := strings.IndexByte(project, '/'); index >= 0 {
		return project[:index]
	}
	return project
}

func loadAcceptanceHighWater(t *testing.T) acceptanceHighWater {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(architectureBatteryRepositoryRoot(t), filepath.FromSlash(acceptanceHighWaterPath)))
	if err != nil {
		t.Fatal(err)
	}
	var mark acceptanceHighWater
	if err := json.Unmarshal(contents, &mark); err != nil {
		t.Fatalf("%s: %v", acceptanceHighWaterPath, err)
	}
	if mark.Projects != corpusHarnessProjectCount {
		t.Fatalf("%s records %d corpus projects, the corpus holds %d; a mark measured on another corpus is not comparable", acceptanceHighWaterPath, mark.Projects, corpusHarnessProjectCount)
	}
	// A mark with no families is an unminted mark, not a mark of zero. The
	// judgment reports every family it walked with the row to record and fails,
	// which is how the first measurement is taken; admitting it here would be a
	// law that passes on any corpus at all.
	recorded := 0
	for _, row := range mark.Families {
		recorded += row.Projects
	}
	if len(mark.Families) != 0 && recorded != mark.Projects {
		t.Fatalf("%s families cover %d fixtures, the mark records a corpus of %d", acceptanceHighWaterPath, recorded, mark.Projects)
	}
	return mark
}

// acceptanceRatchetWalk runs the acceptance mode over the corpus and tallies
// each fixture instead of failing on it. The acceptance law itself already
// reports every fixture as its own subtest; a ratchet that failed per fixture
// could never report the count that is its whole subject.
func acceptanceRatchetWalk(t *testing.T, projects []corpusHarnessProject) map[string]acceptanceTally {
	t.Helper()
	// Seal the canonical target contract on the test's own goroutine. It is the
	// one step of the spine that reports an environment failure fatally, and a
	// walk worker is not a goroutine that may end a test.
	corpusHarnessContract(t)
	var recording sync.Mutex
	runs := make(map[string]acceptanceFixtureRun, len(projects))
	mode := corpusSemanticAcceptanceMode()
	// The ratchet counts rows; it does not fail on them. Record the verdict and
	// leave the harness error reserved for the failures that happen before a
	// verdict exists. The worker records that detached failure's status, class,
	// and error instead of collapsing it into an absent verdict.
	mode.judge = func(run *corpusHarnessRun) []string {
		verdict := corpusSemanticAcceptanceVerdictOf(run)
		recording.Lock()
		record := runs[run.project.name]
		record.verdict = verdict
		record.reached = true
		record.observed = true
		record.status = run.status
		record.statusKnown = true
		record.class = "judged"
		runs[run.project.name] = record
		recording.Unlock()
		return nil
	}
	workers := corpusHarnessWorkerCount(len(projects))
	var next atomic.Int64
	var walkers sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		walkers.Add(1)
		go func() {
			defer walkers.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(projects) {
					return
				}
				project := projects[index]
				run, class, err := corpusHarnessExecuteDetached(t, project, mode)
				recording.Lock()
				record := runs[project.name]
				record.observed = true
				record.class = class
				if err != nil {
					record.failure = err.Error()
					// A verdict may have been recorded before detached cleanup
					// failed. It is not a valid acceptance result until the
					// Plan/Workspace has detached, so classify that path too.
					if record.reached {
						record.reached = false
					}
				}
				if run != nil {
					record.status = run.status
					record.statusKnown = true
				}
				runs[project.name] = record
				recording.Unlock()
			}
		}()
	}
	walkers.Wait()
	families := make(map[string]acceptanceTally, 32)
	for _, project := range projects {
		family := acceptanceFamily(project.name)
		tally := families[family]
		record := runs[project.name]
		fixture := acceptanceFixtureTallyOf(project, record)
		tally.add(fixture)
		families[family] = tally
	}
	return families
}

// TestCanonicalCorpusAcceptanceHighWaterMark is the acceptance ratchet. Its
// name shares the acceptance prefix on purpose: a gate that selects the
// acceptance corpus by name selects its mark with it.
func TestCanonicalCorpusAcceptanceHighWaterMark(t *testing.T) {
	t.Run("law", func(t *testing.T) {
		mark := loadAcceptanceHighWater(t)
		families := acceptanceRatchetWalk(t, corpusHarnessProjects(t))
		t.Log(acceptanceRatchetReceipt(mark, families))
		acceptanceRatchetJudge(t, mark, families)
	})
}

// acceptanceRatchetReporter is the judgment's output. It is the testing
// surface the gate uses and nothing more, so the gate's own verdict table is
// provable without walking a corpus that takes minutes to produce.
type acceptanceRatchetReporter interface {
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

// acceptanceRatchetRecorder is a reporter that records instead of failing.
type acceptanceRatchetRecorder struct{ failures []string }

func (recorder *acceptanceRatchetRecorder) Errorf(format string, args ...any) {
	recorder.failures = append(recorder.failures, fmt.Sprintf(format, args...))
}

func (recorder *acceptanceRatchetRecorder) Logf(string, ...any) {}

func (recorder *acceptanceRatchetRecorder) failed() bool { return len(recorder.failures) != 0 }

// acceptanceRatchetJudge applies the mark family by family. A family the mark
// does not record is a corpus that grew a family since the mark was minted, and
// is a failure demanding the measurement rather than a family admitted for
// free.
func acceptanceRatchetJudge(t acceptanceRatchetReporter, mark acceptanceHighWater, families map[string]acceptanceTally) {
	for _, family := range acceptanceRatchetNames(families) {
		tally := families[family]
		row, recorded := mark.Families[family]
		if !recorded {
			t.Errorf("acceptance family %q has no mark. Record projects=%d clean_min=%d unjudged_max=%d missing_inline_max=%d missing_structured_max=%d unexpected_max=%d in %s.",
				family, tally.projects, tally.clean, tally.unjudged, tally.missingInline, tally.missingStructured, tally.unexpected, acceptanceHighWaterPath)
			continue
		}
		if row.Projects != tally.projects {
			t.Errorf("acceptance family %q holds %d fixtures, the mark records %d; a mark measured over another fixture set is not comparable", family, tally.projects, row.Projects)
			continue
		}
		if tally.clean < row.CleanMin {
			t.Errorf("acceptance family %q regressed: clean=%d, high-water mark=%d. Fix the regression; lowering %s is not an option.",
				family, tally.clean, row.CleanMin, acceptanceHighWaterPath)
		}
		if tally.unjudged > row.UnjudgedMax {
			t.Errorf("acceptance family %q regressed: unjudged=%d, ceiling=%d. A fixture that produces no acceptance verdict has met none of its expectations.",
				family, tally.unjudged, row.UnjudgedMax)
		}
		if tally.missingInline > row.MissingInlineMax {
			t.Errorf("acceptance family %q regressed: missing inline errors=%d, ceiling=%d. Fix the regression; lowering %s is not an option.",
				family, tally.missingInline, row.MissingInlineMax, acceptanceHighWaterPath)
		}
		if tally.missingStructured > row.MissingStructureMax {
			t.Errorf("acceptance family %q regressed: missing structured diagnostics=%d, ceiling=%d. Fix the regression; lowering %s is not an option.",
				family, tally.missingStructured, row.MissingStructureMax, acceptanceHighWaterPath)
		}
		if tally.unexpected > row.UnexpectedMax {
			detail := ""
			if tally.unjudged < row.UnjudgedMax {
				detail = fmt.Sprintf(" This family also judges %d fixtures the mark could not, so the ceiling may be measuring a smaller set; re-mint the whole row from this run rather than widening one column.", row.UnjudgedMax-tally.unjudged)
			}
			t.Errorf("acceptance family %q regressed: unexpected diagnostics=%d, ceiling=%d. Fix the regression; lowering %s is not an option.%s",
				family, tally.unexpected, row.UnexpectedMax, acceptanceHighWaterPath, detail)
		}
		if tally.clean > row.CleanMin {
			t.Logf("acceptance family %q exceeds its mark: clean=%d > %d. Raise the mark in %s and record the run that produced it.",
				family, tally.clean, row.CleanMin, acceptanceHighWaterPath)
		}
	}
	for family := range mark.Families {
		if _, walked := families[family]; !walked {
			t.Errorf("acceptance mark records family %q, the corpus walked none of it", family)
		}
	}
}

func acceptanceRatchetNames(families map[string]acceptanceTally) []string {
	names := make([]string, 0, len(families))
	for family := range families {
		names = append(names, family)
	}
	sort.Strings(names)
	return names
}

// acceptanceRatchetReceipt is the run's status line, with the per-family
// breakdown a raised mark is derived from.
func acceptanceRatchetReceipt(mark acceptanceHighWater, families map[string]acceptanceTally) string {
	var total acceptanceTally
	for _, tally := range families {
		total.add(tally)
	}
	var receipt strings.Builder
	fmt.Fprintf(&receipt, "acceptance: fixtures=%d clean=%d unjudged=%d unsupported=%d preflight-unsupported=%d policy-unsupported=%d compile-invalid=%d compile-unsupported=%d solve-invalid=%d solve-unsupported=%d solve-incomplete=%d detached-result=%d report-unavailable=%d other-pre-verdict=%d missing-inline=%d missing-structured=%d unexpected=%d other=%d (mark measured %s)",
		total.projects, total.clean, total.unjudged, total.unsupported,
		total.preflightUnsupported, total.policyUnsupported, total.compileInvalid, total.compileUnsupported,
		total.solveInvalid, total.solveUnsupported, total.solveIncomplete, total.detachedResult, total.reportUnavailable, total.otherPreVerdict,
		total.missingInline, total.missingStructured, total.unexpected, total.other, mark.Measured)
	for _, family := range acceptanceRatchetNames(families) {
		tally := families[family]
		row := mark.Families[family]
		fmt.Fprintf(&receipt, "\n  %-22s fixtures=%d clean=%d/%d unjudged=%d/%d unsupported=%d preflight-unsupported=%d policy-unsupported=%d compile-invalid=%d compile-unsupported=%d solve-invalid=%d solve-unsupported=%d solve-incomplete=%d detached-result=%d report-unavailable=%d other-pre-verdict=%d missing-inline=%d/%d missing-structured=%d/%d unexpected=%d/%d other=%d",
			family, tally.projects,
			tally.clean, row.CleanMin,
			tally.unjudged, row.UnjudgedMax,
			tally.unsupported, tally.preflightUnsupported, tally.policyUnsupported, tally.compileInvalid, tally.compileUnsupported,
			tally.solveInvalid, tally.solveUnsupported, tally.solveIncomplete, tally.detachedResult, tally.reportUnavailable, tally.otherPreVerdict,
			tally.missingInline, row.MissingInlineMax,
			tally.missingStructured, row.MissingStructureMax,
			tally.unexpected, row.UnexpectedMax,
			tally.other)
	}
	return receipt.String()
}

// TestAcceptanceRatchetCountsUnjudgedFixturesAsUnmetExpectations proves the
// accounting the mark rests on: a fixture that produced no acceptance verdict
// contributes its whole expectation set as missing, so repairing it elsewhere
// can only lower the recorded counts. Without this, a mark minted while a
// fixture failed to compile would condemn the landing that made it compile.
func TestAcceptanceRatchetCountsUnjudgedFixturesAsUnmetExpectations(t *testing.T) {
	project := acceptanceSyntheticProject()
	inline, structured := acceptanceExpectationCounts(project)
	if inline+structured == 0 {
		t.Fatalf("fixture %s declares no expectation; it cannot demonstrate the accounting", project.name)
	}
	unreached := acceptanceFixtureTally(project, corpusSemanticAcceptanceVerdict{}, false)
	if unreached.unjudged != 1 || unreached.unjudgedReasonCount() != unreached.unjudged || unreached.otherPreVerdict != 1 || unreached.missingInline != inline || unreached.missingStructured != structured || unreached.clean != 0 {
		t.Fatalf("a fixture that produced no verdict tallied %+v, want unjudged=1 other-pre-verdict=1 missing-inline=%d missing-structured=%d", unreached, inline, structured)
	}
	contractRefused := acceptanceFixtureTally(project, corpusSemanticAcceptanceVerdict{unsupported: []string{"expected diagnostic \"type.assignment\" has no current collector"}}, true)
	if contractRefused.unjudged != 1 || contractRefused.unjudgedReasonCount() != 1 || contractRefused.unsupported != 1 || contractRefused.policyUnsupported != 1 || contractRefused.missingInline != inline || contractRefused.missingStructured != structured {
		t.Fatalf("an unsupported contract lost its category: %+v, want unjudged=1 unsupported=1 policy-unsupported=1 and the same unmet expectations", contractRefused)
	}
	combined := acceptanceFixtureTally(project, corpusSemanticAcceptanceVerdict{
		unsupported: []string{"diagnostic contract unavailable"},
		unavailable: []string{"DiagnosticReport unavailable"},
	}, true)
	if combined.unjudged != 1 || combined.unjudgedReasonCount() != 1 || combined.policyUnsupported != 1 || combined.reportUnavailable != 0 || combined.unsupported != 1 {
		t.Fatalf("combined unsupported/unavailable verdict was assigned more than one reason: %+v", combined)
	}
	met := acceptanceFixtureTally(project, corpusSemanticAcceptanceVerdict{}, true)
	if met.clean != 1 || met.unjudged != 0 || met.missingInline != 0 || met.missingStructured != 0 {
		t.Fatalf("a fixture that met every expectation tallied %+v, want clean=1 and no unmet rows", met)
	}
}

func acceptanceSyntheticProject() corpusHarnessProject {
	return corpusHarnessProject{
		name: "synthetic/acceptance-accounting",
		expectation: &corpusDiagnosticProjectExpectations{
			inline: []corpusInlineDiagnosticExpectationRow{{Severity: "error"}},
			manifest: &corpusDiagnosticManifest{Check: &corpusDiagnosticManifestCheck{
				Diagnostics: []corpusStructuredDiagnosticExpectation{{}},
			}},
		},
	}
}

// TestAcceptanceRatchetRetainsPreVerdictFailureCategories proves that the
// acceptance receipt does not turn distinct failures into one unjudged bucket.
// These are synthetic detached records so the law remains bounded and does not
// run the corpus just to test accounting.
func TestAcceptanceRatchetRetainsPreVerdictFailureCategories(t *testing.T) {
	project := acceptanceSyntheticProject()
	cases := []struct {
		name     string
		status   analysis.AnalyzeStatus
		class    string
		failure  string
		category func(acceptanceTally) int
	}{
		{"preflight contract", analysis.AnalyzeInvalid, "fixture-contract", "unsupported fixture contract", func(t acceptanceTally) int { return t.preflightUnsupported }},
		{"compile invalid", analysis.AnalyzeInvalid, "compile", "compile=invalid", func(t acceptanceTally) int { return t.compileInvalid }},
		{"compile unsupported", analysis.AnalyzeUnsupported, "compile", "compile=unsupported", func(t acceptanceTally) int { return t.compileUnsupported }},
		{"solve invalid", analysis.AnalyzeInvalid, "invalid", "Analyze status = invalid", func(t acceptanceTally) int { return t.solveInvalid }},
		{"solve unsupported", analysis.AnalyzeUnsupported, "unsupported", "Analyze status = unsupported", func(t acceptanceTally) int { return t.solveUnsupported }},
		{"solve incomplete", analysis.AnalyzeIncomplete, "incomplete", "Analyze status = incomplete", func(t acceptanceTally) int { return t.solveIncomplete }},
		{"detached result", analysis.AnalyzeComplete, "detached-result", "invalid source/content identity", func(t acceptanceTally) int { return t.detachedResult }},
		{"other failure", analysis.AnalyzeInvalid, "link", "link: malformed source", func(t acceptanceTally) int { return t.otherPreVerdict }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			tally := acceptanceFixtureTallyOf(project, acceptanceFixtureRun{
				observed:    true,
				status:      testCase.status,
				statusKnown: true,
				class:       testCase.class,
				failure:     testCase.failure,
			})
			reasons := tally.preflightUnsupported + tally.compileInvalid + tally.compileUnsupported + tally.solveInvalid + tally.solveUnsupported + tally.solveIncomplete + tally.detachedResult + tally.reportUnavailable + tally.otherPreVerdict
			if got := testCase.category(tally); got != 1 || reasons != 1 || tally.unjudgedReasonCount() != tally.unjudged || tally.unjudged != 1 || tally.missingInline == 0 && tally.missingStructured == 0 {
				t.Fatalf("pre-verdict accounting = %+v, want one %s, unjudged=1, and unmet expectations", tally, testCase.name)
			}
		})
	}

	var receiptTally acceptanceTally
	receiptTally.add(acceptanceTally{projects: 1, preflightUnsupported: 1, unsupported: 1})
	receiptTally.add(acceptanceTally{projects: 1, compileInvalid: 1})
	receipt := acceptanceRatchetReceipt(acceptanceHighWater{Measured: "law"}, map[string]acceptanceTally{"types": receiptTally})
	for _, column := range []string{"preflight-unsupported=1", "compile-invalid=1", "solve-invalid=0", "detached-result=0", "other-pre-verdict=0"} {
		if !strings.Contains(receipt, column) {
			t.Fatalf("receipt omitted %q: %s", column, receipt)
		}
	}
}

// TestAcceptanceRatchetClassifiesEveryAcceptanceRow proves the row
// classification the counts are built from, so a reworded acceptance message
// cannot silently move rows into the ungated "other" column.
func TestAcceptanceRatchetClassifiesEveryAcceptanceRow(t *testing.T) {
	project := acceptanceSyntheticProject()
	verdict := corpusSemanticAcceptanceVerdict{diagnostics: []string{
		`missing inline error at main.lua:6 ""`,
		`missing structured diagnostic 0 type.assignment at main.lua:6:33`,
		`unexpected diagnostic type.assignment at main.lua:6:33: cannot assign node.label`,
		`structured diagnostic 0: evidence 1 trust=proven, want claimed`,
		`error count=2, want 1`,
	}}
	tally := acceptanceFixtureTally(project, verdict, true)
	if tally.missingInline != 1 || tally.missingStructured != 1 || tally.unexpected != 1 || tally.other != 2 {
		t.Fatalf("classification tallied %+v, want missing-inline=1 missing-structured=1 unexpected=1 other=2", tally)
	}
	if tally.clean != 0 {
		t.Fatal("a fixture with acceptance rows is not clean")
	}
}

// TestAcceptanceRatchetJudgesEveryRecordedColumn proves the gate itself: each
// recorded column fails on the side it is recorded for, and a family the mark
// does not record is a failure rather than a family admitted for free.
func TestAcceptanceRatchetJudgesEveryRecordedColumn(t *testing.T) {
	mark := acceptanceHighWater{
		Projects: corpusHarnessProjectCount,
		Families: map[string]acceptanceHighWaterRow{
			"types": {Projects: 4, CleanMin: 2, UnjudgedMax: 1, MissingInlineMax: 3, MissingStructureMax: 2, UnexpectedMax: 1},
		},
	}
	admitted := acceptanceTally{projects: 4, clean: 2, unjudged: 1, missingInline: 3, missingStructured: 2, unexpected: 1}
	cases := []struct {
		name   string
		tally  acceptanceTally
		family string
		fails  bool
	}{
		{"at the mark", admitted, "types", false},
		{"better than the mark", acceptanceTally{projects: 4, clean: 4}, "types", false},
		{"one clean fixture lost", acceptanceTally{projects: 4, clean: 1, unjudged: 1, missingInline: 3, missingStructured: 2, unexpected: 1}, "types", true},
		{"one more unjudged fixture", acceptanceTally{projects: 4, clean: 2, unjudged: 2, missingInline: 3, missingStructured: 2, unexpected: 1}, "types", true},
		{"one more missing inline error", acceptanceTally{projects: 4, clean: 2, unjudged: 1, missingInline: 4, missingStructured: 2, unexpected: 1}, "types", true},
		{"one more missing structured row", acceptanceTally{projects: 4, clean: 2, unjudged: 1, missingInline: 3, missingStructured: 3, unexpected: 1}, "types", true},
		{"one more unexpected finding", acceptanceTally{projects: 4, clean: 2, unjudged: 1, missingInline: 3, missingStructured: 2, unexpected: 2}, "types", true},
		{"a family the mark does not record", admitted, "flow", true},
		{"a family that changed size", acceptanceTally{projects: 5, clean: 2, unjudged: 1, missingInline: 3, missingStructured: 2, unexpected: 1}, "types", true},
	}
	for _, testCase := range cases {
		probe := &acceptanceRatchetRecorder{}
		acceptanceRatchetJudge(probe, mark, map[string]acceptanceTally{testCase.family: testCase.tally})
		if probe.failed() != testCase.fails {
			t.Errorf("%s: failed=%t (%s), want %t", testCase.name, probe.failed(), strings.Join(probe.failures, "; "), testCase.fails)
		}
	}
}
