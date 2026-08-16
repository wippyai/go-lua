package analysis_test

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
	"github.com/wippyai/go-lua/program/target/profile"
	"github.com/wippyai/go-lua/program/testfixture"
)

// TestCanonicalFrozenCorpusNativeCensus is a bounded diagnostic shard of the
// same frozen-corpus contract. It is not a fixture exception: every selected
// project still requires AnalyzeComplete, and the full 911-row law remains
// authoritative above. Keeping the shard named lets architectural failures
// converge without repeatedly spending the full-corpus safety budget.
func TestCanonicalFrozenCorpusNativeCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "native/")
}

func TestCanonicalFrozenCorpusCoreCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "core/")
}

func TestCanonicalFrozenCorpusAdviceCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "advice/")
}

func TestCanonicalFrozenCorpusFunctionsCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "functions/")
}

func TestCanonicalFrozenCorpusSemanticCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "semantic/")
}

func TestCanonicalFrozenCorpusTypesCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "types/")
}

func TestCanonicalFrozenCorpusNarrowingCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "narrowing/")
}

func TestCanonicalFrozenCorpusRegressionCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "regression/")
}

func testCanonicalFrozenCorpusPrefix(t *testing.T, prefix string) {
	t.Helper()
	projects, err := testfixture.FrozenCorpusProjects()
	if err != nil {
		t.Fatalf("load frozen corpus: %v", err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	selected := make([]testfixture.CorpusProject, 0)
	for _, project := range projects {
		if !strings.HasPrefix(project.Name(), prefix) {
			continue
		}
		selected = append(selected, project)
	}
	if len(selected) == 0 {
		t.Fatalf("%s frozen-corpus shard is empty", prefix)
	}
	outcomes := runCanonicalFrozenCorpus(contract, selected)
	if failure := formatCanonicalCorpusFailures(outcomes, 12); failure != "" {
		t.Fatal(failure)
	}
}

const (
	canonicalCorpusProjectCount = 911
	canonicalCorpusLuaFileCount = testfixture.FrozenLuaFileCount
	// Keep corpus concurrency independent of both fixture count and unusually
	// large host CPU counts. Thirty-two is the measured 911-corpus lane; the
	// repository's bounded runner remains the hard RSS/time authority.
	canonicalCorpusMaxWorkers = 32
)

// TestCanonicalFrozenCorpusCensus is the honest status census for the current
// canonical analyzer. It checks only Link admission and the public detached
// Result contract; diagnostics and other fixture expectations belong to a
// later oracle layer.
func TestCanonicalFrozenCorpusCensus(t *testing.T) {
	projects, err := testfixture.FrozenCorpusProjects()
	if err != nil {
		t.Fatalf("load frozen corpus: %v", err)
	}
	if len(projects) != canonicalCorpusProjectCount {
		t.Fatalf("frozen corpus projects = %d, want exactly %d", len(projects), canonicalCorpusProjectCount)
	}
	files := 0
	for _, project := range projects {
		files += project.FileCount()
	}
	if files != canonicalCorpusLuaFileCount {
		t.Fatalf("frozen corpus Lua files = %d, want exactly %d", files, canonicalCorpusLuaFileCount)
	}

	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	outcomes := runCanonicalFrozenCorpus(contract, projects)
	var counts [4]int
	for _, outcome := range outcomes {
		if outcome.status >= analysis.AnalyzeInvalid && int(outcome.status) < len(counts) {
			counts[outcome.status]++
		}
	}
	t.Logf("canonical corpus status: executed=%d/%d corpus-projects=%d files=%d/%d complete=%d unsupported=%d", len(outcomes), canonicalCorpusProjectCount, len(projects), files, canonicalCorpusLuaFileCount, counts[analysis.AnalyzeComplete], counts[analysis.AnalyzeUnsupported])
	if failure := formatCanonicalCorpusFailures(outcomes, 12); failure != "" {
		t.Fatal(failure)
	}
}

// canonicalCorpusOutcome is indexed by the canonical project slice. A fixed
// worker pool bounds live Links/Plans by CPU capacity instead of fixture count,
// and the caller reports failures once in canonical grouped order.
type canonicalCorpusOutcome struct {
	project string
	status  analysis.AnalyzeStatus
	class   string
	err     error
}

func runCanonicalFrozenCorpus(contract *target.Contract, projects []testfixture.CorpusProject) []canonicalCorpusOutcome {
	outcomes := make([]canonicalCorpusOutcome, len(projects))
	workers := canonicalCorpusWorkerCount(len(projects))
	if workers == 0 {
		return outcomes
	}
	var next atomic.Int64
	done := make(chan struct{}, workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(projects) {
					return
				}
				project := projects[index]
				outcome := canonicalCorpusOutcome{project: project.Name(), status: analysis.AnalyzeInvalid}
				linked, err := testfixture.SealCorpusProject(contract, project)
				if err != nil {
					outcome.class, outcome.err = "link", err
					outcomes[index] = outcome
					continue
				}
				result, status := analysis.Analyze(context.Background(), linked)
				outcome.status = status
				if status != analysis.AnalyzeComplete {
					outcome.class = analyzeStatusName(status)
					outcome.err = fmt.Errorf("Analyze status = %s", outcome.class)
				} else if err := validateDetachedResult(result, linked.ContentID()); err != nil {
					outcome.class, outcome.err = "detached-result", err
				}
				outcomes[index] = outcome
			}
		}()
	}
	for worker := 0; worker < workers; worker++ {
		<-done
	}
	return outcomes
}

func canonicalCorpusWorkerCount(projects int) int {
	if projects <= 0 {
		return 0
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > canonicalCorpusMaxWorkers {
		workers = canonicalCorpusMaxWorkers
	}
	if workers > projects {
		workers = projects
	}
	return workers
}

func formatCanonicalCorpusFailures(outcomes []canonicalCorpusOutcome, perClass int) string {
	if perClass < 1 {
		perClass = 1
	}
	grouped := make(map[string][]string)
	for _, outcome := range outcomes {
		if outcome.err == nil {
			continue
		}
		class := outcome.class
		if class == "" {
			class = "unknown"
		}
		grouped[class] = append(grouped[class], fmt.Sprintf("%s (%v)", outcome.project, outcome.err))
	}
	if len(grouped) == 0 {
		return ""
	}
	classes := make([]string, 0, len(grouped))
	for class := range grouped {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	var report strings.Builder
	report.WriteString("canonical corpus failures")
	for _, class := range classes {
		rows := grouped[class]
		fmt.Fprintf(&report, "\n%s: %d", class, len(rows))
		limit := len(rows)
		if limit > perClass {
			limit = perClass
		}
		for _, row := range rows[:limit] {
			report.WriteString("\n  ")
			report.WriteString(row)
		}
		if len(rows) > limit {
			fmt.Fprintf(&report, "\n  ... %d more", len(rows)-limit)
		}
	}
	return report.String()
}

func TestCanonicalCorpusFailureFormattingIsGroupedAndBounded(t *testing.T) {
	if workers := canonicalCorpusWorkerCount(1000000); workers < 1 || workers > canonicalCorpusMaxWorkers {
		t.Fatalf("corpus worker bound changed: %d", workers)
	}
	if canonicalCorpusWorkerCount(0) != 0 || canonicalCorpusWorkerCount(1) != 1 {
		t.Fatal("corpus worker bound lost empty/singleton totality")
	}
	outcomes := []canonicalCorpusOutcome{
		{project: "a", class: "incomplete", err: fmt.Errorf("one")},
		{project: "b", class: "incomplete", err: fmt.Errorf("two")},
		{project: "c", class: "incomplete", err: fmt.Errorf("three")},
		{project: "d", class: "link", err: fmt.Errorf("four")},
	}
	report := formatCanonicalCorpusFailures(outcomes, 2)
	for _, want := range []string{"incomplete: 3", "a (one)", "b (two)", "... 1 more", "link: 1", "d (four)"} {
		if !strings.Contains(report, want) {
			t.Fatalf("grouped report omitted %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "c (three)") {
		t.Fatalf("grouped report exceeded its per-class detail budget:\n%s", report)
	}
}

func validateDetachedResult(result *analysis.Result, sourceID keyspace.ContentID) error {
	if result == nil {
		return fmt.Errorf("nil result")
	}
	if !result.ContentID().Available() || !result.SourceID().Available() || result.SourceID() != sourceID {
		return fmt.Errorf("invalid source/content identity")
	}
	if result.BodyCount() == 0 {
		return fmt.Errorf("empty body projection")
	}
	for bodyIndex := 0; bodyIndex < result.BodyCount(); bodyIndex++ {
		body, ok := result.BodyAt(bodyIndex)
		if !ok {
			return fmt.Errorf("body %d is not addressable", bodyIndex)
		}
		if id, ok := body.ID(); !ok || !id.Available() {
			return fmt.Errorf("body %d has no detached identity", bodyIndex)
		}
		for rootIndex := 0; rootIndex < body.RootCount(); rootIndex++ {
			root, ok := body.RootAt(rootIndex)
			if !ok {
				return fmt.Errorf("body %d root %d is not addressable", bodyIndex, rootIndex)
			}
			if id, ok := root.ID(); !ok || !id.Available() {
				return fmt.Errorf("body %d root %d has no detached identity", bodyIndex, rootIndex)
			}
		}
		for valueIndex := 0; valueIndex < body.ValueCount(); valueIndex++ {
			if id, _, ok := body.ValueAt(valueIndex); !ok || !id.Available() {
				return fmt.Errorf("body %d value %d has no detached identity", bodyIndex, valueIndex)
			}
		}
		_, _, ok = body.EffectDisposition()
		if !ok {
			return fmt.Errorf("body %d effect projection unavailable", bodyIndex)
		}
		for effectIndex := 0; effectIndex < body.EffectCount(); effectIndex++ {
			if id, ok := body.EffectAt(effectIndex); !ok || !id.Available() {
				return fmt.Errorf("body %d effect %d has no detached identity", bodyIndex, effectIndex)
			}
		}
	}
	return nil
}

func analyzeStatusName(status analysis.AnalyzeStatus) string {
	switch status {
	case analysis.AnalyzeInvalid:
		return "invalid"
	case analysis.AnalyzeUnsupported:
		return "unsupported"
	case analysis.AnalyzeIncomplete:
		return "incomplete"
	case analysis.AnalyzeComplete:
		return "complete"
	default:
		return fmt.Sprintf("unknown(%d)", status)
	}
}
