package lua

import (
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestCanonicalCorpusDifferential is the full-corpus parity gauge for the
// canonical type-flow cutover (DAG component 11b, fidelity iteration 2). It walks
// every fixture suite, runs the entry file through BOTH flows via the differential
// harness, and aggregates the divergences into a categorized inventory: the precise
// transfer-fidelity worklist.
//
// It does NOT fail on a non-zero diff (the corpus is mid-cutover); it REPORTS the
// inventory. The hard parity gate stays TestCanonicalDifferential_FlowFixtures (the
// 8 closed flow fixtures) plus the per-node-kind gates this iteration adds. This
// test is the measurement instrument those gates are derived from.
//
// Multi-file module fixtures load exactly as runCheckPhase does: dependency modules
// are checked+exported (legacy path — they produce the manifests the entry sees),
// then the entry file runs through the differential under the same options.
// Deadlock fixtures the legacy flow cannot complete are skipped (the legacy half of
// the differential would hang).
func TestCanonicalCorpusDifferential(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}

	inv := newDiffInventory()
	skipped := 0
	measured := 0

	for _, s := range suites {
		if skipCorpusSuite(s) {
			skipped++
			continue
		}
		diff, ok := corpusDifferential(t, s)
		if !ok {
			skipped++
			continue
		}
		measured++
		inv.add(s.Name, diff)
	}

	inv.report(t, measured, skipped, len(suites))
}

// skipCorpusSuite reports suites the differential cannot measure: the two deadlock
// fixtures (legacy cannot terminate), bench-only suites, and explicitly-skipped
// suites. A skipped suite is excluded from the inventory, not silently passed.
func skipCorpusSuite(s namedSuite) bool {
	switch s.Name {
	case "regression/deadlock-dataflow-node", "regression/deadlock-compiler-lua":
		return true
	}
	if s.Suite.Skip != "" {
		return true
	}
	if s.Suite.Check != nil && s.Suite.Check.Skip != "" {
		return true
	}
	return false
}

// corpusDifferential builds the differential for one suite's entry file under the
// suite's options (stdlib, system packages, dependency-module manifests), mirroring
// runCheckPhase's module loading. It returns false when the suite cannot be loaded
// (e.g. a malformed dependency), so the walker skips rather than aborts.
func corpusDifferential(t *testing.T, s namedSuite) (testutil.DifferentialResult, bool) {
	t.Helper()
	files := resolveFiles(s)
	if len(files) == 0 {
		return testutil.DifferentialResult{}, false
	}

	var baseOpts []testutil.Option
	if resolveStdlib(s) {
		baseOpts = append(baseOpts, testutil.WithStdlib())
	}
	for _, pkg := range s.Suite.Packages {
		m := resolvePackageManifest(pkg)
		if m == nil {
			return testutil.DifferentialResult{}, false
		}
		baseOpts = append(baseOpts, testutil.WithManifest(pkg, m))
	}

	// Dependency modules (all but the entry) are checked+exported on the legacy
	// path so the entry's differential sees the same imported manifests both the
	// legacy and canonical flows resolve against.
	sources := make(map[string]string, len(files))
	for _, f := range files {
		sources[f] = readFixtureFile(s.Dir, f)
	}
	entryOpts := append([]testutil.Option{}, baseOpts...)
	for _, f := range files[:len(files)-1] {
		modOpts := append([]testutil.Option{}, entryOpts...)
		name := strings.TrimSuffix(f, ".lua")
		mod := testutil.CheckAndExport(sources[f], name, modOpts...)
		entryOpts = append(entryOpts, testutil.WithModule(name, mod))
	}

	entryFile := files[len(files)-1]
	diff := testutil.Differential(sources[entryFile], entryFile, entryOpts...)
	return diff, true
}

// diffInventory aggregates corpus divergences into the categorized worklist.
type diffInventory struct {
	// canonicalOnly and legacyOnly bucket divergences by diagnostic code name (the
	// node-kind proxy: the code identifies which check the diff is in).
	canonicalOnly map[string]*diffBucket
	legacyOnly    map[string]*diffBucket

	totalCanonicalOnly int
	totalLegacyOnly    int

	cleanSuites    int // suites with 0/0 diff
	divergedSuites int // suites with any diff
}

type diffBucket struct {
	count   int
	suites  map[string]bool
	example string // a representative "fixture: position | message"
}

func newDiffInventory() *diffInventory {
	return &diffInventory{
		canonicalOnly: make(map[string]*diffBucket),
		legacyOnly:    make(map[string]*diffBucket),
	}
}

func (inv *diffInventory) add(suite string, diff testutil.DifferentialResult) {
	if len(diff.CanonicalOnly) == 0 && len(diff.LegacyOnly) == 0 {
		inv.cleanSuites++
		return
	}
	inv.divergedSuites++
	for _, e := range diff.CanonicalOnly {
		inv.record(inv.canonicalOnly, suite, e)
		inv.totalCanonicalOnly++
	}
	for _, e := range diff.LegacyOnly {
		inv.record(inv.legacyOnly, suite, e)
		inv.totalLegacyOnly++
	}
}

func (inv *diffInventory) record(buckets map[string]*diffBucket, suite string, e testutil.DiffEntry) {
	code := e.Diagnostic.Code.Name()
	b := buckets[code]
	if b == nil {
		b = &diffBucket{suites: make(map[string]bool)}
		buckets[code] = b
	}
	b.count++
	b.suites[suite] = true
	if b.example == "" {
		b.example = suite + ": " + e.Diagnostic.Position.String() + " | " + truncateMsg(e.Diagnostic.Message)
	}
}

func truncateMsg(m string) string {
	m = strings.ReplaceAll(m, "\n", " ")
	if len(m) > 120 {
		return m[:120] + "..."
	}
	return m
}

func (inv *diffInventory) report(t *testing.T, measured, skipped, total int) {
	t.Helper()
	t.Logf("=== CANONICAL CORPUS DIFFERENTIAL INVENTORY ===")
	t.Logf("suites: total=%d measured=%d skipped=%d | clean(0/0)=%d diverged=%d",
		total, measured, skipped, inv.cleanSuites, inv.divergedSuites)
	t.Logf("divergences: canonical-only(over-report)=%d legacy-only(miss)=%d",
		inv.totalCanonicalOnly, inv.totalLegacyOnly)

	t.Logf("--- CANONICAL-ONLY (false positives / over-reports), by diagnostic code ---")
	logBuckets(t, inv.canonicalOnly)
	t.Logf("--- LEGACY-ONLY (misses / under-reports), by diagnostic code ---")
	logBuckets(t, inv.legacyOnly)
}

func logBuckets(t *testing.T, buckets map[string]*diffBucket) {
	t.Helper()
	type row struct {
		code string
		b    *diffBucket
	}
	rows := make([]row, 0, len(buckets))
	for code, b := range buckets {
		rows = append(rows, row{code, b})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].b.count != rows[j].b.count {
			return rows[i].b.count > rows[j].b.count
		}
		return rows[i].code < rows[j].code
	})
	if len(rows) == 0 {
		t.Logf("  (none)")
		return
	}
	for _, r := range rows {
		t.Logf("  [%s] count=%d suites=%d", r.code, r.b.count, len(r.b.suites))
		t.Logf("      e.g. %s", r.b.example)
	}
}
