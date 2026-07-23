package lua

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/placementplan"
)

func TestDecomposableFixtureQualificationStats(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures/decomposable")
	if err != nil {
		t.Fatalf("discovering decomposable fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("no decomposable fixture suites found")
	}

	var plans []placementplan.Plan
	for _, suite := range suites {
		plans = append(plans, decomposableFixturePlans(t, suite)...)
	}
	merged := placementplan.Merge(plans...)
	total, decomposable := merged.AllocationStats()
	if total == 0 {
		t.Fatal("decomposable fixture corpus has no allocation sites")
	}
	percent := 100 * float64(decomposable) / float64(total)
	t.Logf("DECOMPOSABLE FIXTURE CORPUS: %d/%d allocation sites qualify (%.1f%%)", decomposable, total, percent)
	if decomposable == 0 {
		t.Fatalf("decomposable fixture corpus has no qualifying allocation sites; entries=%s", formatPlacementEntries(merged))
	}
}

func decomposableFixturePlans(t testing.TB, suite namedSuite) []placementplan.Plan {
	t.Helper()
	files := resolveFiles(suite)
	baseOpts := decomposableFixtureBaseOptions(t, suite)
	sources := readFixtureSources(suite)

	type namedModule struct {
		name string
		mod  *testutil.ModuleResult
	}
	var modules []namedModule
	var plans []placementplan.Plan
	for _, file := range files[:len(files)-1] {
		opts := append([]testutil.Option{}, baseOpts...)
		for _, module := range modules {
			opts = append(opts, testutil.WithModule(module.name, module.mod))
		}
		name := file[:len(file)-len(".lua")]
		mod := testutil.CheckFileAndExport(sources[file], name, file, opts...)
		if len(mod.Errors) != 0 {
			t.Fatalf("%s/%s diagnostics = %d, want clean", suite.Name, file, len(mod.Errors))
		}
		modules = append(modules, namedModule{name: name, mod: mod})
		plans = append(plans, mod.Placement)
	}

	entry := files[len(files)-1]
	opts := append([]testutil.Option{}, baseOpts...)
	for _, module := range modules {
		opts = append(opts, testutil.WithModule(module.name, module.mod))
	}
	result := testutil.CheckFile(sources[entry], entry, opts...)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("%s/%s diagnostics = %d, want clean", suite.Name, entry, len(result.Diagnostics))
	}
	plans = append(plans, result.PlacementPlan())
	return plans
}

func decomposableFixtureBaseOptions(t testing.TB, suite namedSuite) []testutil.Option {
	t.Helper()
	var opts []testutil.Option
	if resolveStdlib(suite) {
		opts = append(opts, testutil.WithStdlib())
	}
	for _, pkg := range suite.Suite.Packages {
		manifest := resolvePackageManifest(pkg)
		if manifest == nil {
			t.Fatalf("unknown system package: %s", pkg)
		}
		opts = append(opts, testutil.WithManifest(pkg, manifest), testutil.WithGlobals(pkg))
	}
	return opts
}
