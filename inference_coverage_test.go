package lua

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// inferenceCase is an unannotated (or minimally-annotated) program whose returned
// value the engine infers. The module export type is the engine's inferred public
// surface; every unknown/any node in it is an inference fallback, so the corpus
// yields a hard, re-runnable inference-coverage number.
type inferenceCase struct {
	name string
	src  string
}

// inferableCorpus holds programs whose returned value is SOUNDLY fully inferable
// without type ascriptions: literals, operators, records, narrowing, call-site
// parameter inference. A correct checker resolves each export to a concrete type
// with zero unknown/any nodes, so this set is asserted at 100%.
var inferableCorpus = []inferenceCase{
	{"int-literal", `local x = 42 return x`},
	{"arithmetic", `local a = 1 + 2 * 3 return a`},
	{"string-concat", `local s = "a" .. "b" return s`},
	{"comparison-bool", `local b = 1 < 2 return b`},
	{"record-literal", `local t = { name = "x", count = 3, ok = true } return t`},
	{"nested-record", `local t = { inner = { value = 1 }, label = "y" } return t`},
	{"array-literal", `local arr = { 1, 2, 3 } return arr`},
	{"call-result", `local function g() return 5 end local y = g() return y`},
	{"multi-return", `local function p() return 1, "a", true end return p`},
	{"call-site-param", `local function f(x) return x + 1 end return f(5)`},
	{"call-site-concat", `local function f(s) return s .. "!" end return f("a")`},
	{"optional-narrowed", `local function h(v: number?) if v then return v end return 0 end return h`},
	{"and-or-default", `local function d(v: string?) return v or "default" end return d`},
	{"local-reassign", `local x = 1 x = x + 1 return x`},
	{"table-field-read", `local t = { n = 7 } local v = t.n return v`},
	{"len-of-array", `local arr = { 10, 20 } local n = #arr return n`},
	{"index-in-range", `local function first(xs: {number}) if #xs >= 1 then return xs[1] end return 0 end return first`},
	{"boolean-not", `local function neg(b: boolean) return not b end return neg`},
	{"annotated-param", `local function add(a: number, b: number) return a + b end return add`},
	{"guard-index-narrow", `local function at(xs: {number}, i: number) if i >= 1 and i <= #xs then return xs[i] end return 0 end return at`},
	{"string-method", `local s = ("hello"):upper() return s`},
	{"string-method-chain", `local s = ("hi"):upper():lower() return s`},
	{"string-method-len", `local n = ("hello"):len() return n`},
	{"string-method-on-var", `local function up(x: string) return x:upper() end return up`},
	{"string-gsub-pair", `local text, count = ("a b"):gsub("%s+", "-") return { text = text, count = count }`},
	{"string-match-guard", `local m = ("id=42"):match("(%d+)") if m == nil then return "" end return m:upper()`},
	{"string-byte-guard", `local b = ("abc"):byte(1) if b == nil then return 0 end return b + 1`},
	{"string-unpack-literal", `local n, s, pos = string.unpack(">I2c3", "abctail") return { n = n, s = s, pos = pos }`},
}

// frontierCorpus holds programs at the current inference frontier: a bare
// parameter used only in its body (no call-site, no annotation) is left unknown,
// which is the SOUND choice (x in `x + 1` could carry an __add metatable). These
// are measured and reported but not asserted, documenting the honest frontier
// rather than demanding unsound over-narrowing.
var frontierCorpus = []inferenceCase{
	{"bare-param-arith", `local function f(x) return x + 1 end return f`},
	{"bare-param-concat", `local function f(s) return s .. "!" end return f`},
	{"closure-capture", `local base = 10 local function add(x) return x + base end return add`},
}

// countTypeNodes recursively counts type nodes and the subset that are unknown or
// any (inference fallbacks), bounded against recursive type cycles.
func countTypeNodes(t typ.Type, depth int) (total, imprecise int) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return 0, 0
	}
	total = 1
	if typ.IsUnknown(t) || typ.IsAny(t) {
		imprecise = 1
	}
	typ.WalkChildren(t, func(child typ.Type) bool {
		ct, ci := countTypeNodes(child, depth+1)
		total += ct
		imprecise += ci
		return true
	})
	return total, imprecise
}

func exportNodeCounts(t *testing.T, tc inferenceCase) (total, imprecise int) {
	t.Helper()
	result := testutil.CheckAndExport(tc.src, tc.name, testutil.WithStdlib())
	if len(result.Errors) != 0 {
		t.Fatalf("%s: expected a well-typed program, got %d diagnostics: %v", tc.name, len(result.Errors), result.Errors)
	}
	total, imprecise = countTypeNodes(result.Manifest.Export, 0)
	if total == 0 {
		t.Fatalf("%s: empty inferred export type", tc.name)
	}
	return total, imprecise
}

func TestInferenceCoverage(t *testing.T) {
	var totalNodes, impreciseNodes, fullyInferred int
	for _, tc := range inferableCorpus {
		nodes, imprecise := exportNodeCounts(t, tc)
		totalNodes += nodes
		impreciseNodes += imprecise
		if imprecise == 0 {
			fullyInferred++
		} else {
			t.Errorf("%s: %d unknown/any node(s) in a soundly-inferable export", tc.name, imprecise)
		}
	}

	coverage := float64(totalNodes-impreciseNodes) / float64(totalNodes)
	t.Logf("inferable corpus: %.1f%% concrete nodes (%d/%d), %d/%d cases fully inferred",
		coverage*100, totalNodes-impreciseNodes, totalNodes, fullyInferred, len(inferableCorpus))

	// Every soundly-inferable case must stay fully inferred without annotations.
	if fullyInferred != len(inferableCorpus) {
		t.Errorf("inferable coverage regressed: %d/%d cases fully inferred", fullyInferred, len(inferableCorpus))
	}
}

// TestInferenceFrontier reports (without asserting) the coverage on cases where
// unknown is currently the sound result, so the frontier is measured and visible.
func TestInferenceFrontier(t *testing.T) {
	for _, tc := range frontierCorpus {
		nodes, imprecise := exportNodeCounts(t, tc)
		t.Logf("%-22s nodes=%d imprecise=%d", tc.name, nodes, imprecise)
	}
}
