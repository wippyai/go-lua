package corpus

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// stubSource is one observation binary. It answers the corpus probe contract
// for the fixtures compiled into it and nothing else. Divergence, delay and
// failure are compiled in, so a test's two sides are one program built once
// and never a program steered at run time.
const stubSource = `package main

import (
	"fmt"
	"os"
	"time"
)

var answers = map[string]string{
%s}

const delayMilliseconds = %d
const failing = %t
const compileDelayMilliseconds = %d
const solveReady = %q
const solvePermit = %q

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "stub: usage: stub <fixture>")
		os.Exit(2)
	}
	if compileDelayMilliseconds > 0 {
		time.Sleep(time.Duration(compileDelayMilliseconds) * time.Millisecond)
	}
	if solveReady != "" {
		fmt.Println(solveReady)
		var permit string
		if _, err := fmt.Fscan(os.Stdin, &permit); err != nil || permit != solvePermit {
			fmt.Fprintln(os.Stderr, "stub: solve phase was not permitted")
			os.Exit(1)
		}
	}
	if delayMilliseconds > 0 {
		// The observation budget begins at the solve marker. This delay models
		// the solver and must therefore remain after the phase boundary.
		time.Sleep(time.Duration(delayMilliseconds) * time.Millisecond)
	}
	if failing {
		fmt.Fprintln(os.Stderr, "stub: the observation refused")
		os.Exit(1)
	}
	text, ok := answers[os.Args[1]]
	if !ok {
		fmt.Fprintln(os.Stderr, "stub: unknown fixture")
		os.Exit(1)
	}
	fmt.Print(text)
}
`

// buildStub writes and builds one stub observation binary answering the given
// envelopes.
func buildStub(t *testing.T, envelopes map[string]Envelope, delayMilliseconds int, failing bool) string {
	return buildStubWithPhase(t, envelopes, delayMilliseconds, failing, 0, SolveReady)
}

// buildStubWithCompileDelay adds a delay before SolveReady, modelling a cold
// compiler. It is used to prove that compilation is outside the solve budget.
func buildStubWithCompileDelay(t *testing.T, envelopes map[string]Envelope, delayMilliseconds int, failing bool, compileDelayMilliseconds int) string {
	return buildStubWithPhase(t, envelopes, delayMilliseconds, failing, compileDelayMilliseconds, SolveReady)
}

func buildStubWithPhase(t *testing.T, envelopes map[string]Envelope, delayMilliseconds int, failing bool, compileDelayMilliseconds int, solveReady string) string {
	t.Helper()
	var table strings.Builder
	names := make([]string, 0, len(envelopes))
	for name := range envelopes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		text, err := envelopes[name].MarshalText()
		if err != nil {
			t.Fatalf("render envelope for %s: %v", name, err)
		}
		fmt.Fprintf(&table, "\t%q: %q,\n", name, string(text))
	}

	directory := t.TempDir()
	write(t, filepath.Join(directory, "main.go"),
		fmt.Sprintf(stubSource, table.String(), delayMilliseconds, failing, compileDelayMilliseconds, solveReady, SolvePermit))
	write(t, filepath.Join(directory, "go.mod"), "module corpusstub\n\ngo 1.21\n")

	binary := filepath.Join(directory, "corpusstub")
	command := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, output)
	}
	return binary
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func rowsOf(value string) []Row {
	return []Row{
		{Family: "call/site", Site: "site/one", Value: value, Outcome: "QueryHit", Lineage: "point/a"},
		{Family: "heap/root", Site: "site/two", Value: "shared", Outcome: "QueryHit", Lineage: "point/b"},
	}
}

func solvedEnvelope(t *testing.T, fixture, oldValue, newValue string) Envelope {
	t.Helper()
	envelope, err := Seal(fixture, []Answer{
		{Side: SideOld, Status: StatusSolved, Rows: rowsOf(oldValue)},
		{Side: SideNew, Status: StatusSolved, Rows: rowsOf(newValue)},
	})
	if err != nil {
		t.Fatalf("seal %s: %v", fixture, err)
	}
	return envelope
}

func stubPlan(binary string, fixtures []string, timeout time.Duration) Plan {
	return Plan{
		Probe:    Probe{Binary: binary, WorkingDirectory: os.TempDir(), Timeout: timeout},
		Fixtures: fixtures,
		Shards:   1,
		Workers:  2,
	}
}

// A corpus walk that enumerates nothing is refused. A driver reporting parity
// over zero fixtures reports nothing while reading as success, which is the
// one outcome a differential measurement must never produce.
func TestWalkRefusesAnEmptyEnumeration(t *testing.T) {
	empty := t.TempDir()
	if err := os.MkdirAll(filepath.Join(empty, "testdata", "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Enumerate(empty); err == nil {
		t.Fatal("an empty corpus enumerated as a successful walk")
	}
	if _, err := Select(nil, 0, 1); err == nil {
		t.Fatal("an empty shard selected as a successful walk")
	}
	if _, err := Run(context.Background(), Plan{Probe: Probe{Binary: "stub"}}); err == nil {
		t.Fatal("a walk over no fixture ran")
	}
}

// The frozen corpus enumerates. The walk is worthless if it cannot find the
// tree it is supposed to range over.
func TestWalkEnumeratesTheFrozenCorpus(t *testing.T) {
	fixtures, err := Enumerate(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("the frozen corpus enumerated no fixture")
	}
	for _, fixture := range fixtures {
		if strings.TrimSpace(fixture) == "" || strings.HasPrefix(fixture, "/") {
			t.Fatalf("fixture name %q is not a corpus-relative path", fixture)
		}
	}
	if Digest(fixtures) == "" {
		t.Fatal("the walked corpus carries no identity")
	}
}

// repositoryRoot walks up from this package to the module root.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the law's own package")
	}
	directory := filepath.Dir(self)
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("no module root above the law's package")
		}
		directory = parent
	}
}

// Two engines answering identically are at parity, and a walk over such a
// corpus is silent. The driver has to be silent where there is nothing to say
// before any divergence it names means anything.
func TestIdenticalAnswersAreParity(t *testing.T) {
	envelopes := map[string]Envelope{
		"basic/arithmetic":     solvedEnvelope(t, "basic/arithmetic", "3", "3"),
		"errors/type-mismatch": solvedEnvelope(t, "errors/type-mismatch", "7", "7"),
	}
	binary := buildStub(t, envelopes, 0, false)

	report, err := Run(context.Background(), stubPlan(binary, []string{"basic/arithmetic", "errors/type-mismatch"}, 20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Identical() {
		t.Fatalf("identical engines diverged: %s", report.Summary())
	}
	if report.FixturesAtParity != 2 || report.FixturesDiverged != 0 {
		t.Fatalf("parity accounting: at-parity=%d diverged=%d", report.FixturesAtParity, report.FixturesDiverged)
	}
	if report.RowsCompared == 0 {
		t.Fatal("the walk compared no row")
	}
}

// A refusal is an answer. Two engines refusing for the same stated reason
// agree, and the walk says so instead of manufacturing a divergence out of
// two silences.
func TestIdenticalRefusalsAreParityAndDifferingOnesAreNot(t *testing.T) {
	agreeing, err := Seal("errors/unsupported", []Answer{
		{Side: SideOld, Status: StatusRefused, Detail: "compile: CompileUnsupported"},
		{Side: SideNew, Status: StatusRefused, Detail: "compile: CompileUnsupported"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if divergences := Compare(agreeing); len(divergences) != 0 {
		t.Fatalf("identical refusals diverged: %v", divergences)
	}

	parting, err := Seal("errors/unsupported", []Answer{
		{Side: SideOld, Status: StatusRefused, Detail: "compile: CompileUnsupported"},
		{Side: SideNew, Status: StatusRefused, Detail: "solve: relation runtime refused the mounted execution"},
	})
	if err != nil {
		t.Fatal(err)
	}
	divergences := Compare(parting)
	if len(divergences) != 1 || divergences[0].Class != ClassStatus {
		t.Fatalf("refusals that part were catalogued as %v", divergences)
	}
}

// A seeded divergence anywhere in the corpus is caught and named: the fixture
// it happened in, the family and query site it lives at, and both engines'
// answers.
func TestSeededCorpusDivergenceIsCaughtAndNamed(t *testing.T) {
	envelopes := map[string]Envelope{
		"basic/arithmetic":     solvedEnvelope(t, "basic/arithmetic", "3", "3"),
		"errors/type-mismatch": solvedEnvelope(t, "errors/type-mismatch", "7", "9"),
	}
	binary := buildStub(t, envelopes, 0, false)

	report, err := Run(context.Background(), stubPlan(binary, []string{"basic/arithmetic", "errors/type-mismatch"}, 20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if report.Identical() {
		t.Fatal("a seeded corpus divergence went undetected")
	}
	if report.FixturesAtParity != 1 || report.FixturesDiverged != 1 {
		t.Fatalf("parity accounting: at-parity=%d diverged=%d", report.FixturesAtParity, report.FixturesDiverged)
	}
	first := report.Divergences[0]
	if first.Fixture != "errors/type-mismatch" {
		t.Fatalf("divergence named fixture %q", first.Fixture)
	}
	if first.Family != "call/site" || first.Site != "site/one" {
		t.Fatalf("divergence addressed %s/%s", first.Family, first.Site)
	}
	if first.Class != ClassValue || first.Old != "7" || first.New != "9" {
		t.Fatalf("divergence reads %+v", first)
	}
	if !strings.Contains(report.Summary(), "errors/type-mismatch") {
		t.Fatalf("the summary hid the diverged fixture:\n%s", report.Summary())
	}
}

// A fixture the new engine cannot yet be asked about is catalogued under its
// own class. It is never dropped from the walk and never counted as agreement.
func TestConstructorUnavailableIsCataloguedNotSkipped(t *testing.T) {
	envelope, err := Seal("basic/arithmetic", []Answer{
		{Side: SideOld, Status: StatusSolved, Rows: rowsOf("3")},
		{Side: SideNew, Status: StatusUnconstructed, Detail: "relation constructor unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	binary := buildStub(t, map[string]Envelope{"basic/arithmetic": envelope}, 0, false)

	report, err := Run(context.Background(), stubPlan(binary, []string{"basic/arithmetic"}, 20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if report.Identical() {
		t.Fatal("an unaskable fixture was reported as agreeing")
	}
	if report.Classes[ClassUnconstructed] != 1 {
		t.Fatalf("class histogram: %v", report.Classes)
	}
	if report.FixtureClasses[ClassUnconstructed] != 1 {
		t.Fatalf("fixture leading-class histogram: %v", report.FixtureClasses)
	}
	if !strings.Contains(report.Divergences[0].New, "constructor unavailable") {
		t.Fatalf("the catalogue hid why the fixture could not be asked: %+v", report.Divergences[0])
	}
}

// An exhausted bound is a recorded divergence and the walk does not wait on
// it. Both halves matter: a walk that hangs measures nothing, and a walk that
// forgets the fixture it killed reports a corpus it did not compare.
func TestExhaustedBoundIsRecordedAndNotWaitedOn(t *testing.T) {
	envelopes := map[string]Envelope{"basic/arithmetic": solvedEnvelope(t, "basic/arithmetic", "3", "3")}
	binary := buildStub(t, envelopes, 5000, false)

	started := time.Now()
	report, err := Run(context.Background(), stubPlan(binary, []string{"basic/arithmetic"}, 250*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if elapsed > 3*time.Second {
		t.Fatalf("the walk waited %s on a fixture bounded at 250ms", elapsed)
	}
	if report.Identical() {
		t.Fatal("a fixture that never answered was reported as agreeing")
	}
	if report.Classes[ClassTimeout] != 1 {
		t.Fatalf("class histogram: %v", report.Classes)
	}
}

// Compilation is before SolveReady, so a cold compiler may take longer than
// the analysis budget without being misreported as a solver timeout.
func TestCompilationIsOutsideSolveBound(t *testing.T) {
	envelopes := map[string]Envelope{"basic/arithmetic": solvedEnvelope(t, "basic/arithmetic", "3", "3")}
	binary := buildStubWithCompileDelay(t, envelopes, 0, false, 300)
	plan := stubPlan(binary, []string{"basic/arithmetic"}, 50*time.Millisecond)
	plan.Probe.ProcessTimeout = 2 * time.Second
	report, err := Run(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Identical() {
		t.Fatalf("compile delay was charged to solve budget: %s", report.Summary())
	}
	if got := report.SlowestFixtures[0].Seconds; got >= plan.Probe.Timeout.Seconds() {
		t.Fatalf("solve timing includes compilation: %.3fs", got)
	}
}

// The process watchdog remains independent: a probe stuck compiling before
// it can announce SolveReady is terminated as a process timeout, not a solve
// timeout.
func TestCompilationWatchdogIsDistinctFromSolveBound(t *testing.T) {
	envelopes := map[string]Envelope{"basic/arithmetic": solvedEnvelope(t, "basic/arithmetic", "3", "3")}
	binary := buildStubWithCompileDelay(t, envelopes, 0, false, 5000)
	plan := stubPlan(binary, []string{"basic/arithmetic"}, 20*time.Millisecond)
	plan.Probe.ProcessTimeout = 100 * time.Millisecond
	started := time.Now()
	report, err := Run(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("process watchdog did not terminate compile hang: %s", elapsed)
	}
	if report.Classes[ClassProcessTimeout] != 1 {
		t.Fatalf("compile watchdog was classified as %v", report.Classes)
	}
}

// A probe that emits a solved envelope without the phase boundary is not
// accepted: without the marker the driver cannot prove that its solve budget
// started after compilation.
func TestSolvedEnvelopeRequiresSolvePhaseMarker(t *testing.T) {
	envelopes := map[string]Envelope{"basic/arithmetic": solvedEnvelope(t, "basic/arithmetic", "3", "3")}
	binary := buildStubWithPhase(t, envelopes, 0, false, 0, "")
	plan := stubPlan(binary, []string{"basic/arithmetic"}, time.Second)
	report, err := Run(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if report.Classes[ClassProtocol] != 1 {
		t.Fatalf("unphased envelope was accepted: %v", report.Classes)
	}
}

// An observation process that fails leaves a named row in the catalogue rather
// than a hole in it.
func TestProbeFailureIsCatalogued(t *testing.T) {
	binary := buildStub(t, map[string]Envelope{}, 0, true)

	report, err := Run(context.Background(), stubPlan(binary, []string{"basic/arithmetic"}, 20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if report.Classes[ClassProbeFailure] != 1 {
		t.Fatalf("class histogram: %v", report.Classes)
	}
	if !strings.Contains(report.Divergences[0].New, "the observation refused") {
		t.Fatalf("the catalogue hid the process failure: %+v", report.Divergences[0])
	}
}

// Concurrency is bounded and the catalogue is not a function of it: a walk run
// with more workers than the ceiling is clamped, and its report is identical
// to the same walk run serially.
func TestConcurrencyIsBoundedAndTheCatalogueIsDeterministic(t *testing.T) {
	envelopes := map[string]Envelope{
		"a/one":   solvedEnvelope(t, "a/one", "1", "2"),
		"b/two":   solvedEnvelope(t, "b/two", "3", "3"),
		"c/three": solvedEnvelope(t, "c/three", "5", "6"),
	}
	binary := buildStub(t, envelopes, 0, false)
	fixtures := []string{"c/three", "a/one", "b/two"}

	plan := stubPlan(binary, fixtures, 20*time.Second)
	plan.Workers = 99
	concurrent, err := Run(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if concurrent.Workers != MaximumWorkers {
		t.Fatalf("worker ceiling: got=%d want=%d", concurrent.Workers, MaximumWorkers)
	}

	plan.Workers = 1
	serial, err := Run(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(concurrent.Divergences, serial.Divergences) {
		t.Fatalf("the catalogue depended on concurrency:\n%v\n%v", concurrent.Divergences, serial.Divergences)
	}
	if concurrent.Divergences[0].Fixture != "a/one" {
		t.Fatalf("the catalogue is not in canonical fixture order: %+v", concurrent.Divergences)
	}
}

// The catalogue round-trips. A report other lanes act on has to survive being
// written and read back unchanged.
func TestReportRoundTrips(t *testing.T) {
	envelopes := map[string]Envelope{
		"basic/arithmetic":     solvedEnvelope(t, "basic/arithmetic", "3", "4"),
		"errors/type-mismatch": solvedEnvelope(t, "errors/type-mismatch", "7", "7"),
	}
	binary := buildStub(t, envelopes, 0, false)

	report, err := Run(context.Background(), stubPlan(binary, []string{"basic/arithmetic", "errors/type-mismatch"}, 20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	text, err := report.JSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseReport(text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, report) {
		t.Fatalf("the catalogue changed across a round trip:\ngot  %+v\nwant %+v", decoded, report)
	}
}

// The envelope round-trips and refuses everything near it.
func TestEnvelopeRoundTripsAndRefusesMalformedInput(t *testing.T) {
	envelope := solvedEnvelope(t, "basic/arithmetic", "3", "4")
	text, err := envelope.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseText(string(text))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, envelope) {
		t.Fatalf("round trip changed the envelope:\ngot  %+v\nwant %+v", decoded, envelope)
	}

	cases := []struct {
		name string
		edit func(string) string
	}{
		{"wrong protocol", func(value string) string {
			return strings.Replace(value, "corpus.Protocol="+Protocol, "corpus.Protocol=w5-corpus/v0", 1)
		}},
		{"unknown field", func(value string) string { return value + "corpus.Unknown=x\n" }},
		{"wrong digest", func(value string) string {
			return strings.Replace(value, "corpus.Digest="+envelope.Digest, "corpus.Digest="+strings.Repeat("0", 64), 1)
		}},
		{"bad encoding", func(value string) string {
			return strings.Replace(value, "corpus.SideAt(0).side=", "corpus.SideAt(0).side=!!!", 1)
		}},
		{"missing side", func(value string) string {
			return strings.Replace(value, "corpus.SideCount=2", "corpus.SideCount=1", 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseText(test.edit(string(text))); err == nil {
				t.Fatal("a malformed envelope was accepted")
			}
		})
	}

	if _, err := Seal("basic/arithmetic", []Answer{
		{Side: SideNew, Status: StatusSolved, Rows: rowsOf("1")},
		{Side: SideOld, Status: StatusSolved, Rows: rowsOf("1")},
	}); err == nil {
		t.Fatal("a transposed envelope was sealed")
	}
	if _, err := Seal("basic/arithmetic", []Answer{
		{Side: SideOld, Status: StatusRefused, Rows: rowsOf("1")},
		{Side: SideNew, Status: StatusRefused},
	}); err == nil {
		t.Fatal("a refusal carrying rows was sealed")
	}
}

// A site only one engine published is named on the engine that is missing it,
// and each published column parts under its own class.
func TestAbsenceAndColumnsAreNamedSeparately(t *testing.T) {
	envelope, err := Seal("basic/arithmetic", []Answer{
		{Side: SideOld, Status: StatusSolved, Rows: []Row{
			{Family: "call/site", Site: "site/one", Value: "3", Outcome: "QueryHit", Lineage: "point/a"},
			{Family: "only/old", Site: "site/x", Value: "1", Outcome: "QueryHit"},
		}},
		{Side: SideNew, Status: StatusSolved, Rows: []Row{
			{Family: "call/site", Site: "site/one", Value: "3", Outcome: "QueryProvenAbsent", Lineage: "point/b"},
			{Family: "only/new", Site: "site/y", Value: "2", Outcome: "QueryHit"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	classes := map[Class]string{}
	for _, divergence := range Compare(envelope) {
		classes[divergence.Class] = divergence.Family
	}
	for _, expected := range []struct {
		class  Class
		family string
	}{
		{ClassOutcome, "call/site"},
		{ClassLineage, "call/site"},
		{ClassAbsentNew, "only/old"},
		{ClassAbsentOld, "only/new"},
	} {
		if classes[expected.class] != expected.family {
			t.Fatalf("class %s named family %q, want %q", expected.class, classes[expected.class], expected.family)
		}
	}
	if _, valued := classes[ClassValue]; valued {
		t.Fatal("an agreeing value column was catalogued as a divergence")
	}
}

// The driver links no engine. It cannot compare two engines honestly if it is
// compiled against one of them, so its own import list holds no analyzer,
// domain or compiler package.
func TestDriverLinksNoRuntime(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("the law cannot locate its own package directory")
	}
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, filepath.Dir(self), nil, parser.ImportsOnly)
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
						t.Fatalf("%s imports %s: the driver would link an engine", name, path)
					}
				}
			}
		}
	}
}

// Every process environment bound the walk imposes is stated, so a walk cannot
// be the thing that exhausts the host.
func TestObservationProcessesAreBounded(t *testing.T) {
	for _, required := range []string{"GOMEMLIMIT=2GiB", "GOMAXPROCS=2"} {
		found := false
		for _, bound := range ProcessEnvironment {
			if bound == required {
				found = true
			}
		}
		if !found {
			t.Fatalf("the walk does not impose %s: %v", required, ProcessEnvironment)
		}
	}
	if MaximumWorkers != 4 {
		t.Fatalf("the concurrency ceiling is %d", MaximumWorkers)
	}
	if DefaultFixtureTimeout != 5*time.Second {
		t.Fatalf("the per-fixture bound is %s", DefaultFixtureTimeout)
	}
	if DefaultProcessTimeout <= DefaultFixtureTimeout {
		t.Fatalf("the process watchdog %s is not distinct from solve bound %s", DefaultProcessTimeout, DefaultFixtureTimeout)
	}
}

// A fixture neither engine was asked about is not evidence of agreement.
// Counting an uncompiled fixture as parity would let a corpus that stopped
// compiling read as a corpus the two engines agree on, which is the most
// expensive way a differential measurement can lie.
func TestUnreachedFixturesAreNotCountedAsParity(t *testing.T) {
	unreached, err := Seal("errors/broken", []Answer{
		{Side: SideOld, Status: StatusUncompiled, Detail: "compile: CompileInvalid"},
		{Side: SideNew, Status: StatusUncompiled, Detail: "compile: CompileInvalid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if divergences := Compare(unreached); len(divergences) != 0 {
		t.Fatalf("an unreached fixture was catalogued as a divergence: %v", divergences)
	}

	envelopes := map[string]Envelope{
		"basic/arithmetic": solvedEnvelope(t, "basic/arithmetic", "3", "3"),
		"errors/broken":    unreached,
	}
	binary := buildStub(t, envelopes, 0, false)

	report, err := Run(context.Background(), stubPlan(binary, []string{"basic/arithmetic", "errors/broken"}, 20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if report.FixturesAtParity != 1 {
		t.Fatalf("fixtures at parity = %d, want only the fixture both engines answered", report.FixturesAtParity)
	}
	if report.FixturesUnreached != 1 {
		t.Fatalf("fixtures unreached = %d", report.FixturesUnreached)
	}
	if report.FixtureClasses[ClassUnreached] != 1 {
		t.Fatalf("fixture leading-class histogram: %v", report.FixtureClasses)
	}
	if !strings.Contains(report.Summary(), "never reached either engine") {
		t.Fatalf("the summary hid the unreached fixtures:\n%s", report.Summary())
	}
}
