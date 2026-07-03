package lua

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

// The fixture oracle is the scorecard for the normal type-checking flow. It runs
// every testdata fixture and judges diagnostics against curated expectations
// rather than against an older engine.

// oracleVerdict is the outcome of judging one fixture's diagnostics against its
// curated expectations.
type oracleVerdict struct {
	name   string
	passed bool
	// missing are curated expectations the checker did not satisfy.
	missing []string
	// unexpected are diagnostics the checker emitted that the curated expectations
	// do not account for.
	unexpected []string
}

func oracleFixtureVerdictWithDeadline(t *testing.T, s namedSuite) oracleVerdict {
	t.Helper()
	deadline := fixtureDeadlineForSuite(s)
	done := make(chan oracleVerdict, 1)
	go func() {
		var v oracleVerdict
		defer func() {
			if r := recover(); r != nil {
				v = oracleVerdict{name: s.Name, passed: false, unexpected: []string{fmt.Sprintf("panic: %v", r)}}
			}
			done <- v
		}()
		diags, entry := fixtureDiagnostics(s)
		v = judgeAgainstCuratedExpectations(s, diags, entry)
	}()

	select {
	case v := <-done:
		return v
	case <-time.After(deadline):
		failFixtureDeadline(t, fmt.Sprintf("oracle fixture deadline exceeded: %s did not complete within %s (rerun that fixture directly with FIXTURE_DEADLINE_SECONDS=%d and FIXTURE_TIMEOUT_EXIT=0 if investigating precision/performance)", s.Name, deadline, int(deadline.Seconds())*4))
		return oracleVerdict{name: s.Name, passed: false}
	}
}

// fixtureDiagnostics runs one fixture's full check phase (all dependency modules
// then the entry) and returns the collected diagnostics with the entry file name.
// It mirrors runCheckPhase's module orchestration exactly.
func fixtureDiagnostics(s namedSuite) (diags []diag.Diagnostic, entryFile string) {
	return fixtureDiagnosticsWithOptions(s)
}

func fixtureDiagnosticsWithOptions(s namedSuite, extraOpts ...testutil.Option) (diags []diag.Diagnostic, entryFile string) {
	files := resolveFiles(s)
	stdlib := resolveStdlib(s)

	var baseOpts []testutil.Option
	if stdlib {
		baseOpts = append(baseOpts, testutil.WithStdlib())
	}
	for _, pkg := range s.Suite.Packages {
		if m := resolvePackageManifest(pkg); m != nil {
			baseOpts = append(baseOpts, testutil.WithManifest(pkg, m))
			baseOpts = append(baseOpts, testutil.WithGlobals(pkg))
		} else {
			return nil, ""
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

	return allDiagnostics, entryFile
}

// judgeAgainstCuratedExpectations applies the same curated-truth verification as
// runCheckPhase: inline expect-error/expect-warning annotations verify local
// markers, manifest check.diagnostics is a structured complete-list oracle when
// present, then check.errors count wins; otherwise the fixture is expected clean.
func judgeAgainstCuratedExpectations(s namedSuite, diagnostics []diag.Diagnostic, entryFile string) oracleVerdict {
	v := oracleVerdict{name: s.Name, passed: true}

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
			missing, _ := matchDiagnosticExpectations(s.Suite.Check.Diagnostics, diagnostics, entryFile, false, fixtureDiagnosticRenderOptions(readFixtureSources(s), entryFile))
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
		missing, unexpected := matchDiagnosticExpectations(s.Suite.Check.Diagnostics, diagnostics, entryFile, true, fixtureDiagnosticRenderOptions(readFixtureSources(s), entryFile))
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

// shouldSkipOracleSuite mirrors the fixture harness's suite-level skips.
func shouldSkipOracleSuite(s namedSuite) (skip bool, deadlock bool) {
	if strings.Contains(s.Name, "deadlock") {
		return false, true
	}
	if s.Suite.Skip != "" {
		return true, false
	}
	if s.Suite.Check != nil && s.Suite.Check.Skip != "" {
		return true, false
	}
	return false, false
}

// TestCuratedOracle judges every fixture's diagnostics against curated
// expectations and reports the pass count plus the failing fixtures bucketed by
// cause. It is a measurement, not a hard gate; TestCuratedGate pins the subset
// that must stay green.
func TestCuratedOracle(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	startFixtureMemoryGuard(t)

	var verdicts []oracleVerdict
	pass, fail, skipped, deadlockPass, deadlockFail := 0, 0, 0, 0, 0
	for _, s := range suites {
		s := s
		skip, deadlock := shouldSkipOracleSuite(s)
		if skip {
			skipped++
			continue
		}
		v := oracleFixtureVerdictWithDeadline(t, s)
		verdicts = append(verdicts, v)
		if v.passed {
			pass++
			if deadlock {
				deadlockPass++
			}
		} else {
			fail++
			if deadlock {
				deadlockFail++
			}
		}
	}

	total := pass + fail
	t.Logf("CURATED ORACLE SCORECARD: %d/%d fixtures PASS against curated truth (%d fail, %d skipped); deadlock-* %d pass / %d fail",
		pass, total, fail, skipped, deadlockPass, deadlockFail)

	// Bucket the failures by dominant cause for the worklist.
	codeBuckets := make(map[string]int)
	var failNames []oracleVerdict
	for _, v := range verdicts {
		if v.passed {
			continue
		}
		failNames = append(failNames, v)
		for _, u := range v.unexpected {
			if code := extractCode(u); code != "" {
				codeBuckets[code]++
			}
		}
	}

	sort.Slice(failNames, func(i, j int) bool { return failNames[i].name < failNames[j].name })
	for _, v := range failNames {
		t.Logf("FAIL %s: %d missing, %d unexpected", v.name, len(v.missing), len(v.unexpected))
		for _, m := range v.missing {
			t.Logf("    MISS: %s", m)
		}
		for _, u := range v.unexpected {
			t.Logf("    FALSE+: %s", u)
		}
	}

	var codes []string
	for c := range codeBuckets {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool { return codeBuckets[codes[i]] > codeBuckets[codes[j]] })
	t.Logf("--- FALSE-POSITIVE CODE HISTOGRAM ---")
	for _, c := range codes {
		t.Logf("  %s: %d", c, codeBuckets[c])
	}
}

// TestCuratedGate is the hard regression gate for the curated oracle: it pins the
// set of fixtures that exercise type-name / scope resolution so those wins cannot
// silently regress.
func TestCuratedGate(t *testing.T) {
	// Fixtures whose curated truth is reached only when a module-local named type
	// resolves structurally: union-alias discriminant narrowing and
	// type-name-in-scope resolution.
	mustPass := []string{
		// Union-alias discriminant narrowing: x.kind == "a" refines the named union
		// AB to variant A so the variant-A field access type-checks clean.
		"narrowing/union-discriminated-literal",
		// Discriminant narrowing detecting a wrong-variant method: a.kind == "dog"
		// refines Animal to Dog, so a.meow() is the curated expect-error.
		"narrowing/discriminator-wrong-method",
		// A named non-generic alias used as a function return type resolves
		// structurally rather than to an unresolved Ref.
		"regression/type-alias-function-return",
		// A generic type alias instantiation resolves through the module scope.
		"regression/generic-type-alias-instantiate",
		// Strict-any proof boundary with exact-call entry values: the provider may
		// export `data_func:any`, but the concrete call path with a literal string
		// must remain clean without teaching the driver a page-registry special case.
		"narrowing/dynamic-registry-renderer-guard",
	}

	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	byName := make(map[string]namedSuite, len(suites))
	for _, s := range suites {
		byName[s.Name] = s
	}

	for _, name := range mustPass {
		name := name
		t.Run(name, func(t *testing.T) {
			s, ok := byName[name]
			if !ok {
				t.Fatalf("gate fixture %q not found", name)
			}
			diags, entry := fixtureDiagnostics(s)
			v := judgeAgainstCuratedExpectations(s, diags, entry)
			if !v.passed {
				t.Errorf("%s: fixture fails curated truth (%d missing, %d unexpected)", name, len(v.missing), len(v.unexpected))
				for _, m := range v.missing {
					t.Errorf("    MISS: %s", m)
				}
				for _, u := range v.unexpected {
					t.Errorf("    FALSE+: %s", u)
				}
			}
		})
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
