package lua

import (
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
)

// relationalCase is a program whose soundness of an array read or nil narrowing
// depends on a RELATIONAL fact between two or more paths (cross-variable length
// equality, index arithmetic, transitive ordering, congruence). These are the
// capabilities the domain/constraint solvers (numeric/diff difference logic,
// equality congruence-closure, numeric relations) are built to provide.
//
// narrows reports the engine's CURRENT behavior, not the aspiration: a met case
// reads the element/value without a nil diagnostic; a gap case still reports the
// soundly-optional nil because the relational fact is not proven. Gap cases are
// the pinned aspirational targets — wiring the named solver flips them, and this
// test then fails until the expectation is updated, which is the intended signal.
type relationalCase struct {
	name    string
	solver  string // the domain/constraint package whose wiring would close a gap
	src     string
	narrows bool // current behavior: does the relational read narrow today?
}

var relationalCorpus = []relationalCase{
	{
		name:    "congruence-path-equality",
		solver:  "equality (already wired via path-equality proofs)",
		narrows: true,
		src: `local function f(a: number?, b: number?): number
			if a == b and a ~= nil then local v: number = b return v end
			return 0
		end return f`,
	},
	{
		name:    "transitive-path-equality",
		solver:  "equality (transitive, wired via pathevidence)",
		narrows: true,
		src: `local function f(x: number?, y: number?, z: number?): number
			if x == y and y == z and x ~= nil then local v: number = z return v end
			return 0
		end return f`,
	},
	{
		// Congruence over access: p == q should carry p.f ~= nil to q.f. The
		// existing pathevidence congruence handles roots and transitivity but the
		// value read does not yet consult equivalent paths for a field, so this
		// stays optional. Closing it is a pathevidence/read-model refinement, NOT
		// the standalone equality E-graph (which would duplicate congruence).
		name:    "congruence-over-field-access",
		solver:  "pathevidence congruence (read-model equivalence; not yet)",
		narrows: false,
		src: `local function f(p: { f: number? }, q: { f: number? }): number
			if p == q and p.f ~= nil then local v: number = q.f return v end
			return 0
		end return f`,
	},
	{
		name:    "cross-variable-length-equality",
		solver:  "numeric/diff + equality",
		narrows: true,
		src: `local function f(a: {number}, b: {number}, i: number): number
			if #a == #b and i >= 1 and i <= #a then local v: number = b[i] return v end
			return 0
		end return f`,
	},
	{
		name:    "index-arithmetic-upper-bound",
		solver:  "numeric/diff (difference logic)",
		narrows: true,
		src: `local function f(xs: {number}, i: number): number
			if i >= 1 and i + 1 <= #xs then local v: number = xs[i + 1] return v end
			return 0
		end return f`,
	},
	{
		name:    "transitive-numeric-ordering",
		solver:  "numeric/diff (difference logic)",
		narrows: true,
		src: `local function f(xs: {number}, i: number, j: number): number
			if i >= 1 and i < j and j <= #xs then local v: number = xs[i] return v end
			return 0
		end return f`,
	},
}

// relationalReadNarrows reports whether src checks clean: a clean check means the
// relational fact narrowed the read, errors mean the soundly-optional nil stayed.
func relationalReadNarrows(t *testing.T, src string) bool {
	t.Helper()
	result := testutil.Check(src, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		if strings.Contains(d.Message, "may be nil") {
			return false
		}
	}
	return true
}

func TestRelationalCoverage(t *testing.T) {
	var met, gaps int
	for _, tc := range relationalCorpus {
		got := relationalReadNarrows(t, tc.src)
		if got != tc.narrows {
			if tc.narrows {
				t.Errorf("%s: regression — relational read no longer narrows (was met)", tc.name)
			} else {
				t.Errorf("%s: now narrows — solver %q appears wired; promote this case to narrows:true", tc.name, tc.solver)
			}
			continue
		}
		if tc.narrows {
			met++
		} else {
			gaps++
			t.Logf("ASPIRATIONAL gap %-32s needs: %s", tc.name, tc.solver)
		}
	}
	t.Logf("relational coverage: %d/%d capabilities met; %d pinned aspirational gaps", met, len(relationalCorpus), gaps)
}
