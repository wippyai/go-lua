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
	projects          int
	clean             int
	unjudged          int
	missingInline     int
	missingStructured int
	unexpected        int
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
	tally.missingInline += other.missingInline
	tally.missingStructured += other.missingStructured
	tally.unexpected += other.unexpected
	tally.other += other.other
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

// acceptanceFixtureTally classifies one fixture's acceptance answer. A fixture
// that failed before the acceptance verdict was reached at all - a link,
// compile, solve, or detached-result defect - is unjudged for the same reason
// an unsupported contract is, and is counted the same way.
func acceptanceFixtureTally(project corpusHarnessProject, verdict corpusSemanticAcceptanceVerdict, reached bool) acceptanceTally {
	tally := acceptanceTally{projects: 1}
	if !reached || !verdict.judged() {
		inline, structured := acceptanceExpectationCounts(project)
		tally.unjudged = 1
		tally.missingInline = inline
		tally.missingStructured = structured
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
	verdicts := make(map[string]corpusSemanticAcceptanceVerdict, len(projects))
	mode := corpusSemanticAcceptanceMode()
	// The ratchet counts rows; it does not fail on them. Recording the verdict
	// and returning none leaves the harness error reserved for the failures
	// that happen before a verdict exists at all, which is exactly the set the
	// tally counts as unjudged.
	mode.judge = func(run *corpusHarnessRun) []string {
		verdict := corpusSemanticAcceptanceVerdictOf(run)
		recording.Lock()
		verdicts[run.project.name] = verdict
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
				corpusHarnessExecuteDetached(t, projects[index], mode)
			}
		}()
	}
	walkers.Wait()
	families := make(map[string]acceptanceTally, 32)
	for _, project := range projects {
		verdict, reached := verdicts[project.name]
		family := acceptanceFamily(project.name)
		tally := families[family]
		tally.add(acceptanceFixtureTally(project, verdict, reached))
		families[family] = tally
	}
	return families
}

// TestCanonicalCorpusAcceptanceHighWaterMark is the acceptance ratchet. Its
// name shares the acceptance prefix on purpose: a gate that selects the
// acceptance corpus by name selects its mark with it.
func TestCanonicalCorpusAcceptanceHighWaterMark(t *testing.T) {
	mark := loadAcceptanceHighWater(t)
	families := acceptanceRatchetWalk(t, corpusHarnessProjects(t))
	t.Log(acceptanceRatchetReceipt(mark, families))
	acceptanceRatchetJudge(t, mark, families)
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
	fmt.Fprintf(&receipt, "acceptance: fixtures=%d clean=%d unjudged=%d missing-inline=%d missing-structured=%d unexpected=%d other=%d (mark measured %s)",
		total.projects, total.clean, total.unjudged, total.missingInline, total.missingStructured, total.unexpected, total.other, mark.Measured)
	for _, family := range acceptanceRatchetNames(families) {
		tally := families[family]
		row := mark.Families[family]
		fmt.Fprintf(&receipt, "\n  %-22s fixtures=%d clean=%d/%d unjudged=%d/%d missing-inline=%d/%d missing-structured=%d/%d unexpected=%d/%d other=%d",
			family, tally.projects,
			tally.clean, row.CleanMin,
			tally.unjudged, row.UnjudgedMax,
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
	project := corpusHarnessFixture(t, "types/recursive-mismatch-rejected")
	inline, structured := acceptanceExpectationCounts(project)
	if inline+structured == 0 {
		t.Fatalf("fixture %s declares no expectation; it cannot demonstrate the accounting", project.name)
	}
	unreached := acceptanceFixtureTally(project, corpusSemanticAcceptanceVerdict{}, false)
	if unreached.unjudged != 1 || unreached.missingInline != inline || unreached.missingStructured != structured || unreached.clean != 0 {
		t.Fatalf("a fixture that produced no verdict tallied %+v, want unjudged=1 missing-inline=%d missing-structured=%d", unreached, inline, structured)
	}
	contractRefused := acceptanceFixtureTally(project, corpusSemanticAcceptanceVerdict{unsupported: []string{"expected diagnostic \"type.assignment\" has no current collector"}}, true)
	if contractRefused != unreached {
		t.Fatalf("an unsupported contract tallied %+v, want the same unmet expectations as a fixture that never ran: %+v", contractRefused, unreached)
	}
	met := acceptanceFixtureTally(project, corpusSemanticAcceptanceVerdict{}, true)
	if met.clean != 1 || met.unjudged != 0 || met.missingInline != 0 || met.missingStructured != 0 {
		t.Fatalf("a fixture that met every expectation tallied %+v, want clean=1 and no unmet rows", met)
	}
}

// TestAcceptanceRatchetClassifiesEveryAcceptanceRow proves the row
// classification the counts are built from, so a reworded acceptance message
// cannot silently move rows into the ungated "other" column.
func TestAcceptanceRatchetClassifiesEveryAcceptanceRow(t *testing.T) {
	project := corpusHarnessFixture(t, "types/recursive-mismatch-rejected")
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
