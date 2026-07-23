package lua

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

// The full fixture oracle is the scorecard for the normal type-checking flow. It
// runs every discovered testdata fixture and judges diagnostics against its
// checked-in expectations rather than against an older engine.

// fixtureExpectationVerdict is the outcome of judging one fixture's diagnostics against its
// checked-in expectations.
type fixtureExpectationVerdict struct {
	name   string
	passed bool
	// missing are checked-in expectations the checker did not satisfy.
	missing []string
	// unexpected are diagnostics the checker emitted that the checked-in expectations
	// do not account for.
	unexpected []string
}

func fullOracleFixtureVerdict(s namedSuite) (v fixtureExpectationVerdict) {
	defer func() {
		if r := recover(); r != nil {
			v = fixtureExpectationVerdict{name: s.Name, passed: false, unexpected: []string{fmt.Sprintf("panic: %v", r)}}
		}
	}()
	diags, entry := fixtureDiagnostics(s)
	return fullOracleVerdictFromDiagnostics(s, diags, entry)
}

func fullOracleVerdictFromDiagnostics(s namedSuite, diagnostics []diag.Diagnostic, entryFile string) fixtureExpectationVerdict {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != diag.Code("check") {
			continue
		}
		return fixtureExpectationVerdict{
			name:       s.Name,
			passed:     false,
			unexpected: []string{"checker infrastructure failure: " + diagSummary(diagnostic)},
		}
	}
	return judgeAgainstFixtureExpectations(s, diagnostics, entryFile)
}

// fixtureDiagnostics runs one fixture's full check phase (all dependency modules
// then the entry) and returns the collected diagnostics with the entry file name.
// It mirrors runCheckPhase's module orchestration exactly.
func fixtureDiagnostics(s namedSuite) (diags []diag.Diagnostic, entryFile string) {
	return fixtureDiagnosticsWithOptions(s)
}

func fixtureDiagnosticsWithOptions(s namedSuite, extraOpts ...testutil.Option) (diags []diag.Diagnostic, entryFile string) {
	return fixtureDiagnosticsWithContext(s, nil, extraOpts...)
}

func fixtureDiagnosticsWithContext(s namedSuite, ctx context.Context, extraOpts ...testutil.Option) (diags []diag.Diagnostic, entryFile string) {
	files := resolveFiles(s)
	stdlib := resolveStdlib(s)

	var baseOpts []testutil.Option
	if ctx != nil {
		baseOpts = append(baseOpts, testutil.WithContext(ctx))
	}
	if stdlib {
		baseOpts = append(baseOpts, testutil.WithStdlib())
	}
	for _, pkg := range s.Suite.Packages {
		if m := resolvePackageManifest(pkg); m != nil {
			baseOpts = append(baseOpts, testutil.WithManifest(pkg, m))
			baseOpts = append(baseOpts, testutil.WithGlobals(pkg))
		} else {
			panic(fmt.Sprintf("fixture %q: package manifest %q is unavailable", s.Name, pkg))
		}
	}
	ruleOpts, err := fixtureDiagnosticRuleOptions(s.Suite.Check)
	if err != nil {
		panic(fmt.Sprintf("diagnostic_rules: %v", err))
	}
	baseOpts = append(baseOpts, ruleOpts...)
	baseOpts = append(baseOpts, extraOpts...)

	sources := make(map[string]string)
	for _, f := range files {
		sources[f] = readFixtureFile(s.Dir, f)
	}

	type namedModule struct {
		name string
		mod  *testutil.ModuleResult
	}
	var moduleOrder []namedModule
	var allDiagnostics []diag.Diagnostic
	for _, f := range files[:len(files)-1] {
		modOpts := append([]testutil.Option{}, baseOpts...)
		for _, nm := range moduleOrder {
			modOpts = append(modOpts, testutil.WithModule(nm.name, nm.mod))
		}
		name := strings.TrimSuffix(f, ".lua")
		mod := testutil.CheckFileAndExport(sources[f], name, f, modOpts...)
		moduleOrder = append(moduleOrder, namedModule{name, mod})
		allDiagnostics = append(allDiagnostics, mod.Errors...)
	}

	entryOpts := append([]testutil.Option{}, baseOpts...)
	for _, nm := range moduleOrder {
		entryOpts = append(entryOpts, testutil.WithModule(nm.name, nm.mod))
	}
	entryFile = files[len(files)-1]
	result := testutil.CheckFile(sources[entryFile], entryFile, entryOpts...)
	allDiagnostics = append(allDiagnostics, result.Diagnostics...)
	result.ReleaseTransient()

	return allDiagnostics, entryFile
}

// judgeAgainstFixtureExpectations applies the same checked-in verification as
// runCheckPhase: inline expect-error/expect-warning annotations verify local
// markers, manifest check.diagnostics is a structured complete-list oracle when
// present, then check.errors count wins; otherwise the fixture is expected clean.
func judgeAgainstFixtureExpectations(s namedSuite, diagnostics []diag.Diagnostic, entryFile string) fixtureExpectationVerdict {
	v := fixtureExpectationVerdict{name: s.Name, passed: true}

	files := resolveFiles(s)
	var expectations []inlineExpectation
	for _, f := range files {
		expectations = append(expectations, parseExpectations(f, readFixtureFile(s.Dir, f))...)
	}

	if len(expectations) > 0 {
		matched := make([]bool, len(diagnostics))
		for _, exp := range expectations {
			found := false
			for i, d := range diagnostics {
				if !matchesExpectation(exp, d, entryFile) {
					continue
				}
				found = true
				matched[i] = true
			}
			if !found {
				v.passed = false
				v.missing = append(v.missing, fmt.Sprintf("expected %s at %s:%d %q not emitted", exp.Severity, exp.File, exp.Line, exp.Contains))
			}
		}
		for i, d := range diagnostics {
			if matched[i] || d.Severity == diag.SeverityHint {
				continue
			}
			v.passed = false
			v.unexpected = append(v.unexpected, diagSummary(d))
		}
		if s.Suite.Check != nil && len(s.Suite.Check.Diagnostics) > 0 {
			missing, _ := matchDiagnosticExpectations(s.Suite.Check.Diagnostics, diagnostics, entryFile, false, fixtureDiagnosticRenderOptions(readFixtureSources(s), entryFile, fixtureDiagnosticRenderConfigForCheck(s.Suite.Check)))
			for _, msg := range missing {
				v.passed = false
				v.missing = append(v.missing, "structured diagnostic not emitted: "+msg)
			}
		} else if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
			want := *s.Suite.Check.Errors
			got := countDiagnosticsBySeverity(diagnostics, diag.SeverityError)
			if got != want {
				v.passed = false
				v.missing = append(v.missing, fmt.Sprintf("expected %d errors, got %d", want, got))
			}
		}
		return v
	}

	if s.Suite.Check != nil && len(s.Suite.Check.Diagnostics) > 0 {
		missing, unexpected := matchDiagnosticExpectations(s.Suite.Check.Diagnostics, diagnostics, entryFile, true, fixtureDiagnosticRenderOptions(readFixtureSources(s), entryFile, fixtureDiagnosticRenderConfigForCheck(s.Suite.Check)))
		if len(missing) > 0 || len(unexpected) > 0 {
			v.passed = false
			for _, msg := range missing {
				v.missing = append(v.missing, "structured diagnostic not emitted: "+msg)
			}
			v.unexpected = append(v.unexpected, unexpected...)
		}
		return v
	}

	if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
		want := *s.Suite.Check.Errors
		got := countDiagnosticsBySeverity(diagnostics, diag.SeverityError)
		if got != want {
			v.passed = false
			if got < want {
				v.missing = append(v.missing, fmt.Sprintf("expected %d errors, got %d", want, got))
			}
			for _, d := range diagnostics {
				if d.Severity != diag.SeverityError {
					continue
				}
				v.unexpected = append(v.unexpected, diagSummary(d))
			}
		}
		return v
	}

	// Default: clean (no errors).
	for _, d := range diagnostics {
		if d.Severity == diag.SeverityError {
			v.passed = false
			v.unexpected = append(v.unexpected, diagSummary(d))
		}
	}
	return v
}

func countDiagnosticsBySeverity(diagnostics []diag.Diagnostic, severity diag.Severity) int {
	count := 0
	for _, d := range diagnostics {
		if d.Severity == severity {
			count++
		}
	}
	return count
}

func diagSummary(d diag.Diagnostic) string {
	return fmt.Sprintf("%s:%d:%d [%s] %s", d.Position.File, d.Position.Line, d.Position.Column, d.Code.String(), d.Message)
}

func isDeadlockFixtureSuite(s namedSuite) bool {
	return strings.Contains(s.Name, "deadlock")
}

// TestFullOracle is the hard semantic gate for every discovered fixture. It
// also reports the pass count and buckets failures by cause so a red gate remains
// actionable. A failed verdict must fail this test; the scorecard is not a
// substitute for the gate.
func TestFullOracle(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("full oracle discovered no fixture suites")
	}
	reporter := newFixtureOracleReporter()
	t.Cleanup(func() {
		reporter.finish(t)
	})

	fixtureSlots := make(chan struct{}, fixtureParallelism())
	traceFixtures := os.Getenv("FULL_ORACLE_TRACE") != ""
	for batchNumber, first := 0, 0; first < len(suites); batchNumber, first = batchNumber+1, first+fixtureOracleBatchSize {
		last := first + fixtureOracleBatchSize
		if last > len(suites) {
			last = len(suites)
		}
		batch := suites[first:last]
		t.Run(fmt.Sprintf("batch-%04d", batchNumber), func(t *testing.T) {
			runFixtureSuites(t, batch, fixtureSlots, func(t *testing.T, s namedSuite) {
				started := time.Now()
				if traceFixtures {
					fmt.Fprintf(os.Stderr, "FULL_ORACLE_BEGIN %s\n", s.Name)
				}
				v := fullOracleFixtureVerdict(s)
				if traceFixtures {
					fmt.Fprintf(os.Stderr, "FULL_ORACLE_END %s pass=%t elapsed=%s\n", s.Name, v.passed, time.Since(started))
				}
				reporter.record(v, isDeadlockFixtureSuite(s))
				if !v.passed {
					t.Errorf("fixture fails checked-in expectations (%d missing, %d unexpected)", len(v.missing), len(v.unexpected))
					for _, m := range v.missing {
						t.Errorf("    MISS: %s", m)
					}
					for _, u := range v.unexpected {
						t.Errorf("    FALSE+: %s", u)
					}
				}
			})
		})
		reporter.logBatch(t, batchNumber, len(batch))
	}
}

const fixtureOracleBatchSize = 16

// fixtureOracleReporter deliberately retains only scalar counters and a compact
// code histogram. Full verdicts can contain every diagnostic from a large
// fixture, so retaining them until the final scoreboard needlessly keeps the
// aggregate process above its RSS limit.
type fixtureOracleReporter struct {
	mu                                     sync.Mutex
	pass, fail, deadlockPass, deadlockFail int
	batchPass, batchFail                   int
	codeBuckets                            map[string]int
}

func newFixtureOracleReporter() *fixtureOracleReporter {
	return &fixtureOracleReporter{codeBuckets: make(map[string]int)}
}

func (r *fixtureOracleReporter) record(v fixtureExpectationVerdict, deadlock bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v.passed {
		r.pass++
		r.batchPass++
		if deadlock {
			r.deadlockPass++
		}
		return
	}
	r.fail++
	r.batchFail++
	if deadlock {
		r.deadlockFail++
	}
	for _, unexpected := range v.unexpected {
		if code := extractCode(unexpected); code != "" {
			r.codeBuckets[code]++
		}
	}
}

func (r *fixtureOracleReporter) logBatch(t *testing.T, batchNumber, batchSize int) {
	t.Helper()
	r.mu.Lock()
	pass, fail := r.batchPass, r.batchFail
	r.batchPass, r.batchFail = 0, 0
	r.mu.Unlock()
	t.Logf("FULL ORACLE BATCH %04d: %d/%d fixtures PASS (%d fail)", batchNumber, pass, batchSize, fail)
}

func (r *fixtureOracleReporter) finish(t *testing.T) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	total := r.pass + r.fail
	t.Logf("FULL ORACLE SCORECARD: %d/%d fixtures PASS against fixture expectations (%d fail); deadlock-* %d pass / %d fail",
		r.pass, total, r.fail, r.deadlockPass, r.deadlockFail)
	if len(r.codeBuckets) == 0 {
		return
	}
	codes := make([]string, 0, len(r.codeBuckets))
	for code := range r.codeBuckets {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool {
		if r.codeBuckets[codes[i]] != r.codeBuckets[codes[j]] {
			return r.codeBuckets[codes[i]] > r.codeBuckets[codes[j]]
		}
		return codes[i] < codes[j]
	})
	t.Log("--- FALSE-POSITIVE CODE HISTOGRAM ---")
	for _, code := range codes {
		t.Logf("  %s: %d", code, r.codeBuckets[code])
	}
}

func TestFullOracleRejectsCheckerInfrastructureDiagnostic(t *testing.T) {
	wantErrors := 1
	suite := namedSuite{
		Name:  "infrastructure-failure-must-not-match-error-count",
		Suite: fixtureSuite{Check: &fixtureCheck{Errors: &wantErrors}},
	}
	diagnostic := diag.Diagnostic{
		Position: diag.Position{File: "main.lua"},
		Code:     diag.Code("check"),
		Message:  "canonical solver failed",
		Severity: diag.SeverityError,
	}

	verdict := fullOracleVerdictFromDiagnostics(suite, []diag.Diagnostic{diagnostic}, "main.lua")
	if verdict.passed {
		t.Fatal("checker infrastructure diagnostic satisfied a fixture error-count expectation")
	}
	if len(verdict.unexpected) != 1 || !strings.Contains(verdict.unexpected[0], "checker infrastructure failure") {
		t.Fatalf("unexpected infrastructure verdict: %#v", verdict)
	}
}

func TestFixtureOracleReporterAggregatesCompactly(t *testing.T) {
	reporter := newFixtureOracleReporter()
	reporter.record(fixtureExpectationVerdict{name: "clean", passed: true}, false)
	reporter.record(fixtureExpectationVerdict{
		name:       "deadlock-failure",
		passed:     false,
		unexpected: []string{"main.lua:1:1 [E1000] first", "main.lua:2:1 [E1000] second", "plain failure"},
	}, true)

	if reporter.pass != 1 || reporter.fail != 1 {
		t.Fatalf("reporter totals = %d pass, %d fail, want 1 each", reporter.pass, reporter.fail)
	}
	if reporter.deadlockPass != 0 || reporter.deadlockFail != 1 {
		t.Fatalf("reporter deadlock totals = %d pass, %d fail, want 0/1", reporter.deadlockPass, reporter.deadlockFail)
	}
	if reporter.codeBuckets["E1000"] != 2 || len(reporter.codeBuckets) != 1 {
		t.Fatalf("reporter code histogram = %#v, want E1000:2", reporter.codeBuckets)
	}
	if reporter.batchPass != 1 || reporter.batchFail != 1 {
		t.Fatalf("reporter batch totals = %d pass, %d fail, want 1 each", reporter.batchPass, reporter.batchFail)
	}
}

// extractCode pulls the "[Exxxx]" code token out of a diagnostic summary line.
func extractCode(s string) string {
	open := strings.Index(s, "[")
	close := strings.Index(s, "]")
	if open < 0 || close < 0 || close < open {
		return ""
	}
	return s[open+1 : close]
}
