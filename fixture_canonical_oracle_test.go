package lua

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// The canonical curated-expectation oracle (DAG component 11b).
//
// This is the real correctness scorecard for the canonical type-flow engine and
// the eventual cutover gate. It runs every testdata fixture through the canonical
// flow (check.WithCanonicalFlow, opt-in) and judges each fixture against its
// hand-CURATED expected diagnostics — the same ground truth the legacy harness
// (runCheckPhase) verifies against — NOT against the legacy flow's output.
//
// The acceptance criterion is correctness, not legacy parity (Forge journal
// #383): a canonical diagnostic legacy misses may be canonical being MORE sound (a
// win to keep), and a legacy diagnostic canonical avoids may be a legacy false
// positive (also a win). The scorecard reports how many fixtures canonical passes
// against curated truth, and buckets the failures by cause.

// oracleVerdict is the outcome of judging one fixture's canonical diagnostics
// against its curated expectations.
type oracleVerdict struct {
	name   string
	passed bool
	// missing are curated expectations the canonical flow did not satisfy (a
	// canonical MISS: an expected diagnostic that was not emitted, or an expected
	// error count that was undershot).
	missing []string
	// unexpected are diagnostics the canonical flow emitted that the curated
	// expectations do not account for (a canonical FALSE POSITIVE / over-report).
	unexpected []string
}

// canonicalFixtureDiagnostics runs one fixture's full check phase (all dependency
// modules then the entry) through the canonical flow and returns the collected
// diagnostics together with the entry file name. It mirrors runCheckPhase's
// module orchestration exactly, threading check.WithCanonicalFlow into every
// checker (module exports and the entry) so the whole fixture is analyzed by the
// canonical engine.
func canonicalFixtureDiagnostics(s namedSuite) (diags []diag.Diagnostic, entryFile string) {
	files := resolveFiles(s)
	stdlib := resolveStdlib(s)

	baseOpts := []testutil.Option{testutil.WithCheckOption(check.WithCanonicalFlow())}
	if stdlib {
		baseOpts = append(baseOpts, testutil.WithStdlib())
	}
	for _, pkg := range s.Suite.Packages {
		if m := resolvePackageManifest(pkg); m != nil {
			baseOpts = append(baseOpts, testutil.WithManifest(pkg, m))
		} else {
			return nil, ""
		}
	}

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
		mod := testutil.CheckAndExport(sources[f], name, modOpts...)
		moduleOrder = append(moduleOrder, namedModule{name, mod})
		allDiagnostics = append(allDiagnostics, mod.Errors...)
	}

	entryOpts := append([]testutil.Option{}, baseOpts...)
	for _, nm := range moduleOrder {
		entryOpts = append(entryOpts, testutil.WithModule(nm.name, nm.mod))
	}
	entryFile = files[len(files)-1]
	result := testutil.Check(sources[entryFile], entryOpts...)
	allDiagnostics = append(allDiagnostics, result.Diagnostics...)

	return allDiagnostics, entryFile
}

// judgeAgainstCuratedExpectations applies the SAME curated-truth verification the
// legacy harness applies (inline expect-error/expect-warning annotations win;
// otherwise manifest check.errors count; otherwise clean) and returns a verdict
// listing the canonical misses and false positives. It is a pure function so the
// scorecard and the gate both judge identically to runCheckPhase.
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
		return v
	}

	if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
		want := *s.Suite.Check.Errors
		var errs []diag.Diagnostic
		for _, d := range diagnostics {
			if d.Severity == diag.SeverityError {
				errs = append(errs, d)
			}
		}
		if len(errs) != want {
			v.passed = false
			if len(errs) < want {
				v.missing = append(v.missing, fmt.Sprintf("expected %d errors, got %d", want, len(errs)))
			}
			for _, d := range errs {
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

func diagSummary(d diag.Diagnostic) string {
	return fmt.Sprintf("%s:%d:%d [%s] %s", d.Position.File, d.Position.Line, d.Position.Column, d.Code.Name(), d.Message)
}

// shouldSkipOracleSuite mirrors the legacy harness's suite-level skips so the
// oracle judges only the fixtures the legacy harness also judges, plus the
// deadlock fixtures the legacy flow cannot run (which the canonical flow must now
// pass — they are tracked as wins, not skipped).
func shouldSkipOracleSuite(s namedSuite) (skip bool, deadlock bool) {
	// A deadlock fixture is one the legacy flow cannot terminate on (the harness
	// catches it via runWithDeadline). The canonical flow terminates, so it is a
	// "canonical must now pass" entry: never skipped, always tracked as deadlock.
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

// TestCanonicalCuratedOracle is the canonical correctness scorecard: it judges
// every fixture's canonical diagnostics against the fixture's curated
// expectations and reports the pass count plus the failing fixtures bucketed by
// cause (canonical miss vs canonical false positive). It is a measurement, not a
// hard gate (it does not fail the build on a non-pass); TestCanonicalCuratedGate
// pins the subset that must stay green.
func TestCanonicalCuratedOracle(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}

	var verdicts []oracleVerdict
	pass, fail, skipped, deadlockPass, deadlockFail := 0, 0, 0, 0, 0
	for _, s := range suites {
		s := s
		skip, deadlock := shouldSkipOracleSuite(s)
		if skip {
			skipped++
			continue
		}
		var v oracleVerdict
		func() {
			defer func() {
				if r := recover(); r != nil {
					v = oracleVerdict{name: s.Name, passed: false, unexpected: []string{fmt.Sprintf("panic: %v", r)}}
				}
			}()
			diags, entry := canonicalFixtureDiagnostics(s)
			v = judgeAgainstCuratedExpectations(s, diags, entry)
		}()
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
	t.Logf("CANONICAL CURATED ORACLE SCORECARD: %d/%d fixtures PASS against curated truth (%d fail, %d skipped); deadlock-* %d pass / %d fail",
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
	t.Logf("--- FALSE-POSITIVE CODE HISTOGRAM (canonical over-reports) ---")
	for _, c := range codes {
		t.Logf("  %s: %d", c, codeBuckets[c])
	}
}

// TestCanonicalCuratedGate is the hard regression gate for the canonical curated
// oracle: it pins the set of fixtures that exercise type-name / scope resolution
// (DAG component 11b, fidelity iteration 4) so the named-type-resolution win
// cannot silently regress. Each listed fixture must PASS against its curated
// expectations under the canonical flow — the named annotation resolves
// structurally, the field-on-named-type check succeeds, and discriminant narrowing
// fires where the fixture relies on it.
func TestCanonicalCuratedGate(t *testing.T) {
	// Fixtures whose curated truth is reached only when a module-local named type
	// resolves structurally in the canonical flow: union-alias discriminant
	// narrowing and type-name-in-scope resolution.
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
			diags, entry := canonicalFixtureDiagnostics(s)
			v := judgeAgainstCuratedExpectations(s, diags, entry)
			if !v.passed {
				t.Errorf("%s: canonical fails curated truth (%d missing, %d unexpected)", name, len(v.missing), len(v.unexpected))
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
