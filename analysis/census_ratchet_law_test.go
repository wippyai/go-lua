package analysis

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
// Two lanes apply one judgment. The full lane walks all 911 fixtures and is the
// authoritative mark: only a full run can be compared against a full-corpus
// count. The sample lane walks a deterministic stride of the same enumeration
// against its own separately recorded mark, so a fast gate still ratchets
// instead of skipping. Neither lane is ever skipped, and neither is capped: the
// repository's bounded runner remains the resource authority, so a killed run
// is a failed run.

// censusHighWater is the checked-in mark. Counts are absolute fixture counts of
// the lane they describe, never fractions, so a mark cannot be satisfied by a
// corpus that shrank.
type censusHighWater struct {
	Measured string              `json:"measured"`
	Note     string              `json:"note"`
	Projects int                 `json:"corpus_projects"`
	Full     censusHighWaterLane `json:"full"`
	Sample   censusHighWaterLane `json:"sample"`
}

// censusHighWaterLane is one lane's recorded mark. Stride selects the sample
// lane's fixtures; the full lane leaves it zero and walks the enumeration.
//
// Measured states whether the counts below came from a run of this lane. A mark
// that has never been measured is not a ceiling of zero - it is an absent mark,
// and recording it as zero would turn the lane into a test that passes on any
// corpus at all. An unmeasured lane therefore reports its census and fails,
// naming the measurement it is waiting for.
type censusHighWaterLane struct {
	Measured       bool `json:"measured"`
	Stride         int  `json:"stride,omitempty"`
	Projects       int  `json:"projects"`
	CompleteMin    int  `json:"complete_min"`
	IncompleteMax  int  `json:"incomplete_max"`
	UnsupportedMax int  `json:"unsupported_max"`
	InvalidMax     int  `json:"invalid_max"`
}

const censusHighWaterPath = "analysis/testdata/census_highwater.json"

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
				run, class, err := corpusHarnessExecute(t, projects[index], mode)
				outcomes[index] = corpusHarnessOutcome{project: projects[index].name, status: run.status, class: class, err: err, cost: run.cost}
			}
		}()
	}
	walkers.Wait()
	var counts [4]int
	for _, outcome := range outcomes {
		if outcome.status >= AnalyzeInvalid && int(outcome.status) < len(counts) {
			counts[outcome.status]++
		}
	}
	return counts, outcomes
}

// censusRatchetJudge applies the mark to one lane's census.
func censusRatchetJudge(t *testing.T, lane string, mark censusHighWaterLane, counts [4]int, outcomes []corpusHarnessOutcome) {
	t.Helper()
	if len(outcomes) != mark.Projects {
		t.Fatalf("census %s lane walked %d fixtures, the mark records %d", lane, len(outcomes), mark.Projects)
	}
	t.Log(censusRatchetReceipt(lane, mark, counts, outcomes))
	if !mark.Measured {
		t.Errorf("census %s lane has no measured mark. Record complete=%d incomplete=%d unsupported=%d invalid=%d in %s and set its measured flag, from a run whose full lane meets its own mark.",
			lane, counts[AnalyzeComplete], counts[AnalyzeIncomplete], counts[AnalyzeUnsupported], counts[AnalyzeInvalid], censusHighWaterPath)
		return
	}
	if counts[AnalyzeComplete] < mark.CompleteMin {
		t.Errorf("census %s lane regressed: complete=%d, high-water mark=%d. Fix the regression; lowering %s is not an option.",
			lane, counts[AnalyzeComplete], mark.CompleteMin, censusHighWaterPath)
	}
	if counts[AnalyzeUnsupported] > mark.UnsupportedMax {
		t.Errorf("census %s lane regressed: unsupported=%d, ceiling=%d", lane, counts[AnalyzeUnsupported], mark.UnsupportedMax)
	}
	if counts[AnalyzeIncomplete] > mark.IncompleteMax {
		t.Errorf("census %s lane regressed: incomplete=%d, ceiling=%d", lane, counts[AnalyzeIncomplete], mark.IncompleteMax)
	}
	if counts[AnalyzeInvalid] > mark.InvalidMax {
		t.Errorf("census %s lane regressed: invalid=%d, ceiling=%d. An invalid fixture is a rejected Link, not a partial analysis.",
			lane, counts[AnalyzeInvalid], mark.InvalidMax)
	}
	if counts[AnalyzeComplete] > mark.CompleteMin {
		t.Logf("census %s lane exceeds its mark: complete=%d > %d. Raise the mark in %s and record the run that produced it.",
			lane, counts[AnalyzeComplete], mark.CompleteMin, censusHighWaterPath)
	}
}

// censusRatchetReceipt is the lane's status line, with the family breakdown a
// raised mark is derived from.
func censusRatchetReceipt(lane string, mark censusHighWaterLane, counts [4]int, outcomes []corpusHarnessOutcome) string {
	families := make(map[string][4]int)
	for _, outcome := range outcomes {
		family := outcome.project
		if index := strings.IndexByte(family, '/'); index >= 0 {
			family = family[:index]
		}
		tally := families[family]
		if outcome.status >= AnalyzeInvalid && int(outcome.status) < len(tally) {
			tally[outcome.status]++
		}
		families[family] = tally
	}
	names := make([]string, 0, len(families))
	for family := range families {
		names = append(names, family)
	}
	sort.Strings(names)
	var wall time.Duration
	for _, outcome := range outcomes {
		wall += outcome.cost.total()
	}
	var receipt strings.Builder
	fmt.Fprintf(&receipt, "census %s lane: fixtures=%d complete=%d/%d incomplete=%d/%d unsupported=%d/%d invalid=%d/%d analysis-wall=%s",
		lane, len(outcomes),
		counts[AnalyzeComplete], mark.CompleteMin,
		counts[AnalyzeIncomplete], mark.IncompleteMax,
		counts[AnalyzeUnsupported], mark.UnsupportedMax,
		counts[AnalyzeInvalid], mark.InvalidMax,
		wall.Round(time.Millisecond))
	for _, family := range names {
		tally := families[family]
		fmt.Fprintf(&receipt, "\n  %-22s complete=%d incomplete=%d unsupported=%d invalid=%d", family, tally[AnalyzeComplete], tally[AnalyzeIncomplete], tally[AnalyzeUnsupported], tally[AnalyzeInvalid])
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
	mark := loadCensusHighWater(t)
	counts, outcomes := censusRatchetCensus(t, corpusHarnessProjects(t))
	censusRatchetFullMutex.Lock()
	censusRatchetFullOutcomes = outcomes
	censusRatchetFullMutex.Unlock()
	censusRatchetJudge(t, "full", mark.Full, counts, outcomes)
}

// TestCorpusCensusHighWaterMarkSample is the fast lane. It applies the same
// judgment to a deterministic stride of the same enumeration, against the mark
// recorded for that stride. It is a real ratchet over a smaller set, not a
// weaker judgment over the whole one: only the full lane above is authoritative
// for the full-corpus mark.
func TestCorpusCensusHighWaterMarkSample(t *testing.T) {
	mark := loadCensusHighWater(t)
	projects := corpusHarnessProjects(t)
	counts, outcomes := censusRatchetSampledCensus(t, projects, mark.Sample.Stride)
	censusRatchetJudge(t, "sample", mark.Sample, counts, outcomes)
}

// censusRatchetSampledCensus produces the sample lane's census, reusing a
// completed full walk when one is available.
func censusRatchetSampledCensus(t *testing.T, projects []corpusHarnessProject, stride int) ([4]int, []corpusHarnessOutcome) {
	t.Helper()
	censusRatchetFullMutex.Lock()
	full := censusRatchetFullOutcomes
	censusRatchetFullMutex.Unlock()
	if len(full) != len(projects) {
		return censusRatchetCensus(t, censusRatchetSample(projects, stride))
	}
	outcomes := make([]corpusHarnessOutcome, 0, len(projects)/stride+1)
	var counts [4]int
	for index := 0; index < len(full); index += stride {
		outcome := full[index]
		outcomes = append(outcomes, outcome)
		if outcome.status >= AnalyzeInvalid && int(outcome.status) < len(counts) {
			counts[outcome.status]++
		}
	}
	return counts, outcomes
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
