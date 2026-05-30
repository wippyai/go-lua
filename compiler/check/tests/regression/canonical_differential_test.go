package regression

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestCanonicalDifferential_FlowFixtures is the parity gate of the canonical
// cutover (DAG component 11b). For each small flow fixture, it runs BOTH flows
// through the differential harness (separate sessions, separate databases) and
// asserts the canonical flow produces EXACTLY the legacy diagnostics: no
// canonical-only over-report and no legacy-only miss.
//
// The observation bridge routes the converged FunctionState into the per-point /
// per-symbol facts and declared-type inputs the diagnostic passes query, so the
// SAME passes run on canonical-computed types. A non-empty diff is a transfer- or
// bridge-fidelity regression: a CanonicalOnly entry is an over-report (a value the
// canonical flow read as unknown where legacy resolved a concrete type), a
// LegacyOnly entry is a miss (a fact the bridge fails to route).
func TestCanonicalDifferential_FlowFixtures(t *testing.T) {
	// A mix of clean fixtures (legacy emits nothing — canonical-only over-reports
	// surface here) and error-bearing fixtures (legacy emits a diagnostic the
	// canonical flow misses — the legacy-only worklist surfaces here).
	fixtures := []string{
		"if-simple",
		"if-else",
		"break-in-for",
		"return-correct-type",
		"return-multiple-values",
		"closure-captures-type",
		"return-wrong-type",
		"do-block-scope",
	}

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			src := readFlowFixtureSource(t, name)
			diff := testutil.Differential(src, name+".lua", testutil.WithStdlib())

			t.Logf("fixture %q: legacy=%d canonical=%d matched=%d legacy-only=%d canonical-only=%d",
				name, len(diff.LegacyAll), len(diff.CanonicalAll),
				len(diff.Matched), len(diff.LegacyOnly), len(diff.CanonicalOnly))
			for _, e := range diff.Matched {
				t.Logf("  matched:        %s | %s | %s", e.Diagnostic.Position, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
			for _, e := range diff.LegacyOnly {
				t.Errorf("legacy-only (canonical MISS): %s | %s | %s", e.Diagnostic.Position, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
			for _, e := range diff.CanonicalOnly {
				t.Errorf("canonical-only (OVER-REPORT): %s | %s | %s", e.Diagnostic.Position, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
		})
	}
}

// TestCanonicalDifferential_TransferNodeKinds is the parity gate for the transfer
// node kinds wired in fidelity iteration 2: call-return typing (predeclared globals
// visible, recursive and forward function references typed through their signature),
// field/index writes with read-back, container-targeted function definitions
// (function M.f), and table-constructor field typing. Each source exercises one such
// node kind and must reach differential 0/0 — the canonical flow computes the same
// fact the legacy flow does, with no over-report and no miss.
func TestCanonicalDifferential_TransferNodeKinds(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "global-print-visible",
			src:  "local x = 1\nprint(x)\n",
		},
		{
			name: "recursive-local-function-return",
			src: "local function fib(n: number): number\n" +
				"    if n < 2 then return n end\n" +
				"    return fib(n - 1) + fib(n - 2)\n" +
				"end\n" +
				"print(fib(10))\n",
		},
		{
			name: "recursive-global-function-return",
			src: "function factorial(n: number): number\n" +
				"    if n <= 1 then return 1 end\n" +
				"    return n * factorial(n - 1)\n" +
				"end\n",
		},
		{
			name: "field-write-read-back",
			src:  "local t = {}\nt.x = 5\nlocal y: number = t.x\n",
		},
		{
			name: "func-def-field-write",
			src: "local M = {}\n" +
				"function M.add(a: number, b: number): number\n" +
				"    return a + b\n" +
				"end\n" +
				"local result: number = M.add(1, 2)\n",
		},
		{
			name: "table-literal-field-call",
			src:  "local t = { id = function(msg) return msg end }\nt.id(\"hi\")\n",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			diff := testutil.Differential(c.src, c.name+".lua", testutil.WithStdlib())
			for _, e := range diff.LegacyOnly {
				t.Errorf("legacy-only (canonical MISS): %s | %s | %s", e.Diagnostic.Position, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
			for _, e := range diff.CanonicalOnly {
				t.Errorf("canonical-only (OVER-REPORT): %s | %s | %s", e.Diagnostic.Position, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
		})
	}
}

// TestCanonicalDifferential_Narrowing is the parity gate for path-sensitive
// narrowing (DAG component 11b, fidelity iteration 3). Each fixture exercises one
// narrowing kind the canonical flow now applies per branch edge — nil/non-nil
// presence (x ~= nil, x == nil), truthiness (if x), and positive typeof tests
// (type(x) == "string"/"number") — and must reach differential 0/0: the canonical
// flow narrows the guarded value exactly as the legacy flow does, with no
// over-report and no miss.
//
// The narrowing is the per-successor refinement NarrowEdge applies (true edge
// carries the guard, false edge its negation), exposed through the observation
// surface so the assignment-source check observes the narrowed type. The merge
// point's join recovers the unnarrowed value, so the narrowing is local to its
// guard (verified by the else-branch fixture, where the false edge narrows to the
// negation and the join afterward does not retain either branch's refinement).
func TestCanonicalDifferential_Narrowing(t *testing.T) {
	fixtures := []string{
		"nil-check-optional", // x ~= nil narrows x? to x on the true edge
		"nil-check-else",     // x == nil: true edge is nil, false edge is non-nil
		"truthiness-narrows", // if x narrows x? to x
		"typeof-string",      // type(x) == "string" narrows string|number to string
		"typeof-number",      // type(x) == "number" narrows string|number to number
	}

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			src := readNarrowingFixtureSource(t, name)
			diff := testutil.Differential(src, name+".lua", testutil.WithStdlib())
			for _, e := range diff.LegacyOnly {
				t.Errorf("legacy-only (canonical MISS): %s | %s | %s", e.Diagnostic.Position, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
			for _, e := range diff.CanonicalOnly {
				t.Errorf("canonical-only (OVER-REPORT): %s | %s | %s", e.Diagnostic.Position, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
		})
	}
}

// readFlowFixtureSource reads a flow fixture's main.lua from the repository
// testdata tree, relative to this regression test package.
func readFlowFixtureSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "testdata", "fixtures", "flow", name, "main.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read flow fixture %s: %v", name, err)
	}
	return string(data)
}

// readNarrowingFixtureSource reads a narrowing fixture's main.lua from the
// repository testdata tree, relative to this regression test package.
func readNarrowingFixtureSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "testdata", "fixtures", "narrowing", name, "main.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read narrowing fixture %s: %v", name, err)
	}
	return string(data)
}
