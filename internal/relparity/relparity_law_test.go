package relparity

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubSource is one observation binary: it answers the probe contract
// (fixture, verb) with a deterministic dump. Divergence and delay are compiled
// into the binary, so the two sides of a test are two separately built
// programs and never one program steered at run time.
const stubSource = `package main

import (
	"fmt"
	"os"
	"time"
)

const divergentValue = %q
const delay = %d

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "stub: usage: stub <fixture> <verb>")
		os.Exit(2)
	}
	if delay > 0 {
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
	fixture, verb := os.Args[1], os.Args[2]
	fmt.Printf("session.Fixture=%%s\n", fixture)
	fmt.Printf("result.FamilyAt(0).Key=%%s\n", verb)
	fmt.Printf("result.FamilyAt(0).QueryAt(0).Cell.Present=true\n")
	fmt.Printf("result.NativePublicationAt(0).Provenance.PointID=%%s\n", divergentValue)
	fmt.Printf("engine.SomeAccessorNoFacetTableKnows=%%s\n", divergentValue)
	fmt.Printf("report.FindingCount=0\n")
}
`

// buildStub writes and builds one stub observation binary and returns a Side
// bound to it.
func buildStub(t *testing.T, role, value string, delayMilliseconds int) Side {
	t.Helper()
	directory := t.TempDir()
	source := fmt.Sprintf(stubSource, value, delayMilliseconds)
	write(t, filepath.Join(directory, "main.go"), source)
	write(t, filepath.Join(directory, "go.mod"), "module relparitystub\n\ngo 1.21\n")

	binary := filepath.Join(directory, role)
	// The stub is a throwaway program in a temporary directory under no
	// version control, so VCS stamping has nothing to read and is switched off.
	command := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s stub: %v\n%s", role, err, output)
	}
	return Side{Name: role, Ref: role, Commit: role, Binary: binary}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func stubPlan(baseline, replacement Side, timeout time.Duration) Plan {
	return Plan{
		Probe:            Probe{Package: "stub", Verbs: []string{"publish", "why"}, Timeout: timeout},
		Baseline:         baseline,
		Replacement:      replacement,
		WorkingDirectory: os.TempDir(),
		Fixtures:         []string{"basic/arithmetic", "errors/type-mismatch"},
		Shards:           1,
	}
}

// A comparison of one runtime against itself is empty. The harness has to be
// silent where there is nothing to say before any divergence it reports means
// anything.
func TestBaselineAgainstBaselineIsEmpty(t *testing.T) {
	baseline := buildStub(t, "baseline", "point-one", 0)
	replacement := buildStub(t, "replacement", "point-one", 0)

	report := Run(context.Background(), stubPlan(baseline, replacement, 20*time.Second))

	if !report.Identical() {
		t.Fatalf("identical sides diverged: %s", report.Summary())
	}
	if report.DivergenceCount != 0 || len(report.Divergences) != 0 || report.First != nil {
		t.Fatalf("empty comparison carried %d divergences", report.DivergenceCount)
	}
	if report.RowsCompared == 0 {
		t.Fatal("comparison examined no rows")
	}
	if report.Observations != len(report.Fixtures)*len(report.Probe.Verbs) {
		t.Fatalf("observations=%d for %d fixtures x %d verbs",
			report.Observations, len(report.Fixtures), len(report.Probe.Verbs))
	}
}

// A seeded divergence is found and named: the fixture, the verb, the accessor
// it lives at, and both sides' values.
func TestSeededDivergenceIsDetectedAndNamed(t *testing.T) {
	baseline := buildStub(t, "baseline", "point-one", 0)
	replacement := buildStub(t, "replacement", "point-two", 0)

	report := Run(context.Background(), stubPlan(baseline, replacement, 20*time.Second))

	if report.Identical() {
		t.Fatal("seeded divergence went undetected")
	}
	first := report.First
	if first == nil {
		t.Fatal("diverged report named no first row")
	}
	if first.Fixture != "basic/arithmetic" || first.Verb != "publish" {
		t.Fatalf("first divergence is %s %s, not the first fixture and verb", first.Fixture, first.Verb)
	}
	if first.Key != "result.NativePublicationAt(0).Provenance.PointID" {
		t.Fatalf("first divergence names %q", first.Key)
	}
	if first.Kind != KindValue || first.Baseline != "point-one" || first.Replacement != "point-two" {
		t.Fatalf("first divergence reads %+v", *first)
	}
	if first.Dimension != DimensionLineage {
		t.Fatalf("a provenance accessor was labelled %s", first.Dimension)
	}
	if !strings.Contains(report.Summary(), "point-two") {
		t.Fatalf("summary hid the divergent value:\n%s", report.Summary())
	}
}

// Classification is a label, never a filter: an accessor the facet table has
// no rule for still diverges.
func TestUnclassifiedAccessorStillDiverges(t *testing.T) {
	baseline := buildStub(t, "baseline", "point-one", 0)
	replacement := buildStub(t, "replacement", "point-two", 0)

	report := Run(context.Background(), stubPlan(baseline, replacement, 20*time.Second))

	unclassified := 0
	for _, divergence := range report.Divergences {
		if divergence.Key == "engine.SomeAccessorNoFacetTableKnows" {
			unclassified++
			if divergence.Dimension != DimensionUnclassified {
				t.Fatalf("expected the unlabelled accessor to stay unclassified, got %s", divergence.Dimension)
			}
		}
	}
	if unclassified == 0 {
		t.Fatal("an accessor outside the facet table was never compared")
	}
	if report.RowsByDimension[DimensionUnclassified] == 0 {
		t.Fatal("unclassified rows were not counted")
	}
}

// An exhausted bound is a finding, never agreement.
func TestExhaustedBoundIsNamedNotSwallowed(t *testing.T) {
	baseline := buildStub(t, "baseline", "point-one", 0)
	replacement := buildStub(t, "replacement", "point-one", 3000)

	plan := stubPlan(baseline, replacement, 250*time.Millisecond)
	plan.Fixtures = []string{"basic/arithmetic"}
	plan.Probe.Verbs = []string{"publish"}
	report := Run(context.Background(), plan)

	if report.Identical() {
		t.Fatal("a side that never finished was reported as agreeing")
	}
	if len(report.Exhausted) != 1 {
		t.Fatalf("exhausted bounds recorded: %v", report.Exhausted)
	}
	if report.First == nil || report.First.Key != "process.timed-out" {
		t.Fatalf("first divergence is %+v, not the exhausted bound", report.First)
	}
	if report.First.Dimension != DimensionProcess {
		t.Fatalf("exhausted bound labelled %s", report.First.Dimension)
	}
}

// Shards partition the corpus: every fixture is compared by exactly one shard.
func TestShardsPartitionTheCorpus(t *testing.T) {
	fixtures := []string{"a", "b", "c", "d", "e", "f", "g"}
	seen := map[string]int{}
	for index := 0; index < 3; index++ {
		selected, err := Shard(fixtures, index, 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, fixture := range selected {
			seen[fixture]++
		}
	}
	for _, fixture := range fixtures {
		if seen[fixture] != 1 {
			t.Fatalf("fixture %q was compared by %d shards", fixture, seen[fixture])
		}
	}
	if _, err := Shard(fixtures, 3, 3); err == nil {
		t.Fatal("a shard index outside the partition was accepted")
	}
	if _, err := Shard(fixtures, 0, 0); err == nil {
		t.Fatal("a zero shard count was accepted")
	}
}

// The harness links no runtime. It cannot compare two runtimes honestly if it
// is compiled against one of them, so its import list holds no analyzer or
// domain package.
func TestHarnessLinksNoRuntime(t *testing.T) {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"github.com/wippyai/go-lua/analysis",
		"github.com/wippyai/go-lua/domain",
		"github.com/wippyai/go-lua/compiler",
	}
	for _, parsed := range packages {
		for name, file := range parsed.Files {
			for _, imported := range file.Imports {
				path := strings.Trim(imported.Path.Value, `"`)
				for _, prefix := range forbidden {
					if path == prefix || strings.HasPrefix(path, prefix+"/") {
						t.Fatalf("%s imports %s: the harness would link a runtime", name, path)
					}
				}
			}
		}
	}
}

// Rows repeating one accessor are addressed by occurrence, so a divergence
// names the accessor it happened at rather than shifting every later row.
func TestRepeatedAccessorsAreAddressedByOccurrence(t *testing.T) {
	baseline := ParseDump("f", "v", "a.Count=1\na.Count=2\na.Count=3\n")
	replacement := ParseDump("f", "v", "a.Count=1\na.Count=9\na.Count=3\n")

	divergences := CompareRows(baseline, replacement)

	if len(divergences) != 1 {
		t.Fatalf("expected one divergence, got %d: %v", len(divergences), divergences)
	}
	if divergences[0].Occurrence != 1 || divergences[0].Baseline != "2" || divergences[0].Replacement != "9" {
		t.Fatalf("divergence reads %+v", divergences[0])
	}
}

// A row only one side published is named on the side that is missing it.
func TestAbsenceIsNamedOnBothSides(t *testing.T) {
	divergences := CompareRows(
		ParseDump("f", "v", "only.Baseline=1\nshared.Key=k\n"),
		ParseDump("f", "v", "shared.Key=k\nonly.Replacement=2\n"),
	)
	kinds := map[DivergenceKind]string{}
	for _, divergence := range divergences {
		kinds[divergence.Kind] = divergence.Key
	}
	if kinds[KindAbsentReplacement] != "only.Baseline" {
		t.Fatalf("baseline-only row not named: %v", divergences)
	}
	if kinds[KindAbsentBaseline] != "only.Replacement" {
		t.Fatalf("replacement-only row not named: %v", divergences)
	}
}
