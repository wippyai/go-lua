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
	"time"

	"github.com/wippyai/go-lua/analysis"
)

// The census ratchet turns the frozen corpus status census into a monotone
// commitment. The census itself already states the destination - every fixture
// reaches AnalyzeComplete - and is red until it is reached. What it does not
// state is direction: between two red runs, nothing distinguishes progress from
// regression, so a landing that quietly loses fifty complete fixtures reads the
// same as the landing before it.
//
// The ratchet supplies that missing half. A checked-in high-water mark records
// the best census this corpus has produced; the law asserts the current census
// is at least that good. Raising the mark is a deliberate edit of the data
// file, made by whoever measured the better run, which is what makes the number
// evidence rather than a moving average.
//
// Two lanes apply one judgment. The full lane walks all 912 fixtures and is the
// authoritative mark: only a full run can be compared against a full-corpus
// count. The sample lane walks a deterministic stride of the same enumeration
// against its own separately recorded mark, so a fast gate still ratchets
// instead of skipping. Neither lane is ever skipped, and neither is capped: the
// repository's bounded runner remains the resource authority, so a killed run
// is a failed run.

// The census counts one half of what a landing can lose. The other half is
// cost: a corpus that still reaches 683 complete fixtures while spending an
// order of magnitude more analysis on them has regressed just as concretely,
// and the count ratchet reads that run as unchanged. The recorded analysis wall
// closes that half. It is a macro gate, deliberately coarse: a wall is
// hardware-relative and worker-contention-relative, so it is minted with its
// measurement provenance and judged against a recorded multiplier generous
// enough that only pathologies of the 34m-to-4m28s class trip it.

// censusHighWater is the checked-in mark. Counts are absolute fixture counts of
// the lane they describe, never fractions, so a mark cannot be satisfied by a
// corpus that shrank.
//
// WallCeiling is the multiplier applied to a lane's recorded analysis wall. It
// lives in the data file with the walls it governs, because the tolerance a
// measured wall deserves is a property of that measurement, not of the code
// reading it.
type censusHighWater struct {
	Measured    string              `json:"measured"`
	Note        string              `json:"note"`
	Projects    int                 `json:"corpus_projects"`
	WallCeiling float64             `json:"analysis_wall_ceiling"`
	Full        censusHighWaterLane `json:"full"`
	Sample      censusHighWaterLane `json:"sample"`
}

// censusHighWaterLane is one lane's recorded mark. Stride selects the sample
// lane's fixtures; the full lane leaves it zero and walks the enumeration.
//
// Measured states whether the counts below came from a run of this lane. A mark
// that has never been measured is not a ceiling of zero - it is an absent mark,
// and recording it as zero would turn the lane into a test that passes on any
// corpus at all. An unmeasured lane therefore reports its census and fails,
// naming the measurement it is waiting for.
//
// WallSeconds is the same kind of evidence for cost: the analysis wall this
// lane produced on the run that minted its counts, summed over its own
// fixtures. Zero is an absent wall, not a wall of zero, and is treated exactly
// as an unmeasured count mark is.
type censusHighWaterLane struct {
	Measured       bool    `json:"measured"`
	Stride         int     `json:"stride,omitempty"`
	Projects       int     `json:"projects"`
	CompleteMin    int     `json:"complete_min"`
	IncompleteMax  int     `json:"incomplete_max"`
	UnsupportedMax int     `json:"unsupported_max"`
	InvalidMax     int     `json:"invalid_max"`
	WallSeconds    float64 `json:"analysis_wall_seconds"`
}

// wall is the lane's recorded analysis wall as a duration.
func (lane censusHighWaterLane) wall() time.Duration {
	return time.Duration(lane.WallSeconds * float64(time.Second))
}

const censusHighWaterPath = "oracle/testdata/census_highwater.json"

func loadCensusHighWater(t *testing.T) censusHighWater {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(architectureBatteryRepositoryRoot(t), filepath.FromSlash(censusHighWaterPath)))
	if err != nil {
		t.Fatal(err)
	}
	var mark censusHighWater
	if err := json.Unmarshal(contents, &mark); err != nil {
		t.Fatalf("%s: %v", censusHighWaterPath, err)
	}
	if mark.Projects != corpusHarnessProjectCount {
		t.Fatalf("%s records %d corpus projects, the corpus holds %d; a mark measured on another corpus is not comparable", censusHighWaterPath, mark.Projects, corpusHarnessProjectCount)
	}
	if mark.Full.Projects != corpusHarnessProjectCount {
		t.Fatalf("%s full lane records %d fixtures, want the whole corpus of %d", censusHighWaterPath, mark.Full.Projects, corpusHarnessProjectCount)
	}
	if mark.WallCeiling < 1 {
		t.Fatalf("%s records an analysis-wall ceiling of %v; a ceiling below the recorded wall condemns the run that minted it", censusHighWaterPath, mark.WallCeiling)
	}
	if mark.Sample.Stride < 2 {
		t.Fatalf("%s sample lane records stride %d; a sample lane strides at least 2", censusHighWaterPath, mark.Sample.Stride)
	}
	for name, lane := range map[string]censusHighWaterLane{"full": mark.Full, "sample": mark.Sample} {
		if lane.CompleteMin+lane.IncompleteMax+lane.UnsupportedMax+lane.InvalidMax < lane.Projects {
			t.Fatalf("%s %s lane admits at most %d of %d fixtures; the mark is not total over its lane", censusHighWaterPath, name, lane.CompleteMin+lane.IncompleteMax+lane.UnsupportedMax+lane.InvalidMax, lane.Projects)
		}
	}
	return mark
}

// censusRatchetSample selects the sample lane's fixtures. Selection is a stride
// over the canonical enumeration, so it is deterministic, spans every fixture
// family in corpus order, and is reproducible from the data file alone.
func censusRatchetSample(projects []corpusHarnessProject, stride int) []corpusHarnessProject {
	if stride < 2 {
		return projects
	}
	sampled := make([]corpusHarnessProject, 0, len(projects)/stride+1)
	for index := 0; index < len(projects); index += stride {
		sampled = append(sampled, projects[index])
	}
	return sampled
}

// censusRatchetCensus walks one fixture set through the harness census mode and
// tallies its public statuses. It classifies instead of failing per fixture:
// the census law already reports each fixture as its own subtest, and a ratchet
// that failed on every non-complete fixture could never report the count that
// is its whole subject.
func censusRatchetCensus(t *testing.T, projects []corpusHarnessProject) ([4]int, []corpusHarnessOutcome) {
	t.Helper()
	// Seal the canonical target contract on the test's own goroutine. It is the
	// one step of the spine that reports an environment failure fatally, and a
	// walk worker is not a goroutine that may end a test.
	corpusHarnessContract(t)
	outcomes := make([]corpusHarnessOutcome, len(projects))
	workers := corpusHarnessWorkerCount(len(projects))
	mode := corpusHarnessCensusMode()
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
				run, class, err := corpusHarnessExecuteDetached(t, projects[index], mode)
				outcomes[index] = corpusHarnessOutcome{project: projects[index].name, status: run.status, class: class, err: err, cost: run.cost}
			}
		}()
	}
	walkers.Wait()
	var counts [4]int
	for _, outcome := range outcomes {
		if outcome.status >= analysis.AnalyzeInvalid && int(outcome.status) < len(counts) {
			counts[outcome.status]++
		}
	}
	return counts, outcomes
}

// censusRatchetTiming carries one lane's own analysis wall. Measured is false
// when the lane read a completed walk instead of analyzing its own fixtures:
// those durations belong to the run that produced them, so judging them here
// would grade one lane's cost against another lane's contention.
type censusRatchetTiming struct {
	wall     time.Duration
	measured bool
}

// censusRatchetWall sums a lane's per-fixture analysis cost.
func censusRatchetWall(outcomes []corpusHarnessOutcome) time.Duration {
	var wall time.Duration
	for _, outcome := range outcomes {
		wall += outcome.cost.total()
	}
	return wall
}

// censusRatchetJudge applies the mark to one lane's census and, when the lane
// measured its own cost, to that cost.
func censusRatchetJudge(t *testing.T, lane string, mark censusHighWaterLane, ceiling float64, counts [4]int, outcomes []corpusHarnessOutcome, timing censusRatchetTiming) {
	t.Helper()
	if len(outcomes) != mark.Projects {
		t.Fatalf("census %s lane walked %d fixtures, the mark records %d", lane, len(outcomes), mark.Projects)
	}
	t.Log(censusRatchetReceipt(lane, mark, ceiling, counts, outcomes, timing))
	if !mark.Measured {
		t.Errorf("census %s lane has no measured mark. Record complete=%d incomplete=%d unsupported=%d invalid=%d in %s and set its measured flag, from a run whose full lane meets its own mark.",
			lane, counts[analysis.AnalyzeComplete], counts[analysis.AnalyzeIncomplete], counts[analysis.AnalyzeUnsupported], counts[analysis.AnalyzeInvalid], censusHighWaterPath)
		return
	}
	if counts[analysis.AnalyzeComplete] < mark.CompleteMin {
		t.Errorf("census %s lane regressed: complete=%d, high-water mark=%d. Fix the regression; lowering %s is not an option.",
			lane, counts[analysis.AnalyzeComplete], mark.CompleteMin, censusHighWaterPath)
	}
	if counts[analysis.AnalyzeUnsupported] > mark.UnsupportedMax {
		t.Errorf("census %s lane regressed: unsupported=%d, ceiling=%d", lane, counts[analysis.AnalyzeUnsupported], mark.UnsupportedMax)
	}
	if counts[analysis.AnalyzeIncomplete] > mark.IncompleteMax {
		t.Errorf("census %s lane regressed: incomplete=%d, ceiling=%d", lane, counts[analysis.AnalyzeIncomplete], mark.IncompleteMax)
	}
	if counts[analysis.AnalyzeInvalid] > mark.InvalidMax {
		t.Errorf("census %s lane regressed: invalid=%d, ceiling=%d. An invalid fixture is a rejected Link, not a partial analysis.",
			lane, counts[analysis.AnalyzeInvalid], mark.InvalidMax)
	}
	if counts[analysis.AnalyzeComplete] > mark.CompleteMin {
		t.Logf("census %s lane exceeds its mark: complete=%d > %d. Raise the mark in %s and record the run that produced it.",
			lane, counts[analysis.AnalyzeComplete], mark.CompleteMin, censusHighWaterPath)
	}
	censusRatchetJudgeWall(t, lane, mark, ceiling, timing)
}

// censusRatchetWallVerdict is what the recorded wall says about a measured one.
type censusRatchetWallVerdict int

const (
	// censusRatchetWallUnjudged is a lane holding another run's timings.
	censusRatchetWallUnjudged censusRatchetWallVerdict = iota
	censusRatchetWallUnrecorded
	censusRatchetWallAdmitted
	censusRatchetWallRegressed
)

// censusRatchetWallCeiling is the largest wall a lane may measure.
func censusRatchetWallCeiling(mark censusHighWaterLane, ceiling float64) time.Duration {
	return time.Duration(float64(mark.wall()) * ceiling)
}

// censusRatchetWallJudgment decides one lane's cost verdict. It is separated
// from reporting so the verdict table is itself provable, rather than only
// observable through a corpus walk that takes minutes to produce.
func censusRatchetWallJudgment(mark censusHighWaterLane, ceiling float64, timing censusRatchetTiming) censusRatchetWallVerdict {
	if !timing.measured {
		return censusRatchetWallUnjudged
	}
	if mark.WallSeconds <= 0 {
		return censusRatchetWallUnrecorded
	}
	if timing.wall > censusRatchetWallCeiling(mark, ceiling) {
		return censusRatchetWallRegressed
	}
	return censusRatchetWallAdmitted
}

// censusRatchetJudgeWall applies the recorded analysis wall to the wall this
// lane measured. The judgment is deliberately one-sided and coarse: a lane that
// beat its recorded wall is reported, never failed, because the recorded number
// is one machine's measurement and a faster machine is not evidence of a fix.
func censusRatchetJudgeWall(t *testing.T, lane string, mark censusHighWaterLane, ceiling float64, timing censusRatchetTiming) {
	t.Helper()
	switch censusRatchetWallJudgment(mark, ceiling, timing) {
	case censusRatchetWallUnjudged:
		t.Logf("census %s lane read a completed walk; its analysis wall belongs to that walk and is not judged here", lane)
	case censusRatchetWallUnrecorded:
		t.Errorf("census %s lane has no recorded analysis wall. Record analysis_wall_seconds=%.1f in %s, with the machine and worker count that measured it.",
			lane, timing.wall.Seconds(), censusHighWaterPath)
	case censusRatchetWallRegressed:
		t.Errorf("census %s lane cost regressed: analysis-wall=%s, recorded wall=%s, ceiling=%s (recorded x %.2f). Fix the regression; lowering %s is not an option.",
			lane, timing.wall.Round(time.Millisecond), mark.wall().Round(time.Millisecond), censusRatchetWallCeiling(mark, ceiling).Round(time.Millisecond), ceiling, censusHighWaterPath)
	}
}

// censusRatchetReceipt is the lane's status line, with the family breakdown a
// raised mark is derived from.
func censusRatchetReceipt(lane string, mark censusHighWaterLane, ceiling float64, counts [4]int, outcomes []corpusHarnessOutcome, timing censusRatchetTiming) string {
	families := make(map[string][4]int)
	for _, outcome := range outcomes {
		family := outcome.project
		if index := strings.IndexByte(family, '/'); index >= 0 {
			family = family[:index]
		}
		tally := families[family]
		if outcome.status >= analysis.AnalyzeInvalid && int(outcome.status) < len(tally) {
			tally[outcome.status]++
		}
		families[family] = tally
	}
	names := make([]string, 0, len(families))
	for family := range families {
		names = append(names, family)
	}
	sort.Strings(names)
	wallState := "measured"
	if !timing.measured {
		wallState = "reused-walk"
	}
	var receipt strings.Builder
	fmt.Fprintf(&receipt, "census %s lane: fixtures=%d complete=%d/%d incomplete=%d/%d unsupported=%d/%d invalid=%d/%d analysis-wall=%s/%s (%s, recorded=%s x%.2f)",
		lane, len(outcomes),
		counts[analysis.AnalyzeComplete], mark.CompleteMin,
		counts[analysis.AnalyzeIncomplete], mark.IncompleteMax,
		counts[analysis.AnalyzeUnsupported], mark.UnsupportedMax,
		counts[analysis.AnalyzeInvalid], mark.InvalidMax,
		timing.wall.Round(time.Millisecond),
		time.Duration(float64(mark.wall())*ceiling).Round(time.Millisecond),
		wallState,
		mark.wall().Round(time.Millisecond), ceiling)
	for _, family := range names {
		tally := families[family]
		fmt.Fprintf(&receipt, "\n  %-22s complete=%d incomplete=%d unsupported=%d invalid=%d", family, tally[analysis.AnalyzeComplete], tally[analysis.AnalyzeIncomplete], tally[analysis.AnalyzeUnsupported], tally[analysis.AnalyzeInvalid])
	}
	return receipt.String()
}

// censusRatchetFull holds the full lane's walk for the rest of the process. The
// census is deterministic per fixture, so the sample lane may read its own
// fixtures out of a completed full walk instead of analyzing them a second
// time. Running the sample lane alone still walks only its stride, which is
// what makes it the fast lane.
var (
	censusRatchetFullMutex    sync.Mutex
	censusRatchetFullOutcomes []corpusHarnessOutcome
)

// TestCanonicalFrozenCorpusCensusHighWaterMark is the authoritative lane. Its
// name shares the census prefix on purpose: selecting the census by name
// selects its ratchet with it, so the full-corpus mark cannot be left unrun by
// a gate that meant to run the census.
func TestCanonicalFrozenCorpusCensusHighWaterMark(t *testing.T) {
	t.Run("law", func(t *testing.T) {
		mark := loadCensusHighWater(t)
		counts, outcomes := censusRatchetCensus(t, corpusHarnessProjects(t))
		censusRatchetFullMutex.Lock()
		censusRatchetFullOutcomes = outcomes
		censusRatchetFullMutex.Unlock()
		censusRatchetJudge(t, "full", mark.Full, mark.WallCeiling, counts, outcomes, censusRatchetTiming{wall: censusRatchetWall(outcomes), measured: true})
	})
}

// TestCorpusCensusHighWaterMarkSample is the fast lane. It applies the same
// judgment to a deterministic stride of the same enumeration, against the mark
// recorded for that stride. It is a real ratchet over a smaller set, not a
// weaker judgment over the whole one: only the full lane above is authoritative
// for the full-corpus mark.
func TestCorpusCensusHighWaterMarkSample(t *testing.T) {
	t.Run("law", func(t *testing.T) {
		mark := loadCensusHighWater(t)
		projects := corpusHarnessProjects(t)
		counts, outcomes, timing := censusRatchetSampledCensus(t, projects, mark.Sample.Stride)
		censusRatchetJudge(t, "sample", mark.Sample, mark.WallCeiling, counts, outcomes, timing)
	})
}

// censusRatchetSampledCensus produces the sample lane's census, reusing a
// completed full walk when one is available. Reuse is sound for the census,
// which is deterministic per fixture, and unsound for the wall, which is a
// measurement of the run that took it, so the reported timing states which of
// the two the lane holds.
func censusRatchetSampledCensus(t *testing.T, projects []corpusHarnessProject, stride int) ([4]int, []corpusHarnessOutcome, censusRatchetTiming) {
	t.Helper()
	censusRatchetFullMutex.Lock()
	full := censusRatchetFullOutcomes
	censusRatchetFullMutex.Unlock()
	if len(full) != len(projects) {
		counts, outcomes := censusRatchetCensus(t, censusRatchetSample(projects, stride))
		return counts, outcomes, censusRatchetTiming{wall: censusRatchetWall(outcomes), measured: true}
	}
	outcomes := make([]corpusHarnessOutcome, 0, len(projects)/stride+1)
	var counts [4]int
	for index := 0; index < len(full); index += stride {
		outcome := full[index]
		outcomes = append(outcomes, outcome)
		if outcome.status >= analysis.AnalyzeInvalid && int(outcome.status) < len(counts) {
			counts[outcome.status]++
		}
	}
	return counts, outcomes, censusRatchetTiming{wall: censusRatchetWall(outcomes)}
}

// TestCensusRatchetWallGateJudgesOnlyItsOwnMeasurement proves the cost half of
// the ratchet: a wall inside the recorded ceiling is admitted, a wall past it
// regresses, an absent recorded wall demands the measurement instead of passing
// on a ceiling of zero, and a lane that read another walk's timings is not
// judged on them at all.
func TestCensusRatchetWallGateJudgesOnlyItsOwnMeasurement(t *testing.T) {
	mark := loadCensusHighWater(t)
	if mark.Full.WallSeconds <= 0 || mark.Sample.WallSeconds <= 0 {
		t.Fatalf("%s records no analysis wall for both lanes: full=%.1fs sample=%.1fs", censusHighWaterPath, mark.Full.WallSeconds, mark.Sample.WallSeconds)
	}
	if mark.WallCeiling < 1 {
		t.Fatalf("%s records an analysis-wall ceiling of %v", censusHighWaterPath, mark.WallCeiling)
	}
	recorded := mark.Full
	admitted := censusRatchetWallCeiling(recorded, mark.WallCeiling)
	if admitted < recorded.wall() {
		t.Fatalf("recorded ceiling %s is below the recorded wall %s", admitted, recorded.wall())
	}
	cases := []struct {
		name    string
		mark    censusHighWaterLane
		timing  censusRatchetTiming
		verdict censusRatchetWallVerdict
	}{
		{"at the recorded wall", recorded, censusRatchetTiming{wall: recorded.wall(), measured: true}, censusRatchetWallAdmitted},
		{"at the ceiling", recorded, censusRatchetTiming{wall: admitted, measured: true}, censusRatchetWallAdmitted},
		{"one nanosecond past the ceiling", recorded, censusRatchetTiming{wall: admitted + 1, measured: true}, censusRatchetWallRegressed},
		{"an order of magnitude past the ceiling", recorded, censusRatchetTiming{wall: recorded.wall() * 10, measured: true}, censusRatchetWallRegressed},
		{"faster than the recorded wall", recorded, censusRatchetTiming{wall: recorded.wall() / 4, measured: true}, censusRatchetWallAdmitted},
		{"no recorded wall", censusHighWaterLane{Measured: true, Projects: recorded.Projects}, censusRatchetTiming{wall: recorded.wall(), measured: true}, censusRatchetWallUnrecorded},
		{"a reused walk past the ceiling", recorded, censusRatchetTiming{wall: recorded.wall() * 10}, censusRatchetWallUnjudged},
	}
	for _, testCase := range cases {
		if verdict := censusRatchetWallJudgment(testCase.mark, mark.WallCeiling, testCase.timing); verdict != testCase.verdict {
			t.Errorf("%s: verdict=%d, want %d", testCase.name, verdict, testCase.verdict)
		}
	}
}

// TestCensusRatchetSelectsItsSampleDeterministically proves the sample lane's
// selection, so a stride that silently degenerated could not make the fast lane
// pass by walking one fixture.
func TestCensusRatchetSelectsItsSampleDeterministically(t *testing.T) {
	mark := loadCensusHighWater(t)
	projects := corpusHarnessProjects(t)
	first := censusRatchetSample(projects, mark.Sample.Stride)
	second := censusRatchetSample(projects, mark.Sample.Stride)
	if len(first) != mark.Sample.Projects {
		t.Fatalf("stride %d selects %d fixtures, the mark records %d", mark.Sample.Stride, len(first), mark.Sample.Projects)
	}
	for index := range first {
		if first[index].name != second[index].name {
			t.Fatalf("sample selection is not deterministic at %d: %s then %s", index, first[index].name, second[index].name)
		}
	}
	families := make(map[string]struct{})
	for _, project := range first {
		if index := strings.IndexByte(project.name, '/'); index >= 0 {
			families[project.name[:index]] = struct{}{}
		}
	}
	if len(families) < 10 {
		t.Fatalf("sample covers %d fixture families; a stride sample spans the corpus", len(families))
	}
}
