package oracle

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The known-reds manifest is the grounding kit's honesty device. A refactor of
// this size always carries failures that are understood and not yet paid; the
// danger is not that they exist, it is that a new failure hides among them and
// a paid one keeps being counted as debt.
//
// The law is therefore two-sided and total over the sets it names. Every listed
// red must still be red: a red that has turned green is a manifest edit, and
// the law says so by name rather than letting the entry rot. Every red in a
// covered set must be listed: an unlisted failure fails loudly, with its test
// name, so it cannot be absorbed into an aggregate count.
//
// Coverage is stated as executable package sets, not as prose. A test outside
// the covered sets is outside what this law claims - the manifest records that
// boundary so the claim can be read exactly.
//
// One entry is listed but never executed. A test that segfaults takes its whole
// package binary down with it, so running it would erase the verdict of every
// other test in that package. It is excluded from execution by name, its
// exclusion is itself part of the manifest, and removing it from the exclusion
// list is how the fix is recorded.

// knownRedManifest is the checked-in expected-failure set.
type knownRedManifest struct {
	Measured    string          `json:"measured"`
	Note        string          `json:"note"`
	CoveredSets []knownRedSet   `json:"covered_sets"`
	Reds        []knownRedEntry `json:"reds"`
	Excluded    []knownRedEntry `json:"excluded"`
}

// knownRedSet is one executable coverage claim: the packages the law runs and
// the granularity at which it judges their verdicts.
type knownRedSet struct {
	Name     string   `json:"name"`
	Reason   string   `json:"reason"`
	Packages []string `json:"packages"`
	// Filter is the -run selection applied to every package of the set. An
	// empty filter runs the whole package.
	Filter string `json:"filter"`
	// Subtests judges at subtest granularity. A set that judges only top-level
	// tests reports a failing parent once, whatever its subtree did.
	Subtests bool `json:"subtests"`
}

// knownRedEntry is one expected failure with the reason it is expected and the
// component that owns the fix.
type knownRedEntry struct {
	Set     string `json:"set"`
	Package string `json:"package"`
	Test    string `json:"test"`
	Reason  string `json:"reason"`
	Owner   string `json:"owner"`
	// Excluded marks an entry that is never executed, with the reason it
	// cannot be. Only entries in the manifest's excluded list carry it.
	Excluded bool `json:"excluded,omitempty"`
}

func (entry knownRedEntry) key() string {
	return entry.Package + "\t" + entry.Test
}

// knownRedManifestPath is the checked-in manifest.
const knownRedManifestPath = "oracle/testdata/known_reds.json"

// knownRedSkippedSuffix marks an entry that ran and skipped rather than failed.
const knownRedSkippedSuffix = " (skipped)"

func loadKnownRedManifest(t *testing.T) knownRedManifest {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(architectureBatteryRepositoryRoot(t), filepath.FromSlash(knownRedManifestPath)))
	if err != nil {
		t.Fatal(err)
	}
	var manifest knownRedManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatalf("%s: %v", knownRedManifestPath, err)
	}
	if len(manifest.CoveredSets) == 0 {
		t.Fatalf("%s claims no covered set; a manifest that covers nothing states nothing", knownRedManifestPath)
	}
	names := make(map[string]struct{}, len(manifest.CoveredSets))
	for _, set := range manifest.CoveredSets {
		if set.Name == "" || len(set.Packages) == 0 {
			t.Fatalf("%s: covered set %q names no packages", knownRedManifestPath, set.Name)
		}
		names[set.Name] = struct{}{}
	}
	for _, entry := range append(append([]knownRedEntry{}, manifest.Reds...), manifest.Excluded...) {
		if _, covered := names[entry.Set]; !covered {
			t.Fatalf("%s: entry %s/%s names set %q which is not covered", knownRedManifestPath, entry.Package, entry.Test, entry.Set)
		}
		if entry.Reason == "" || entry.Owner == "" {
			t.Fatalf("%s: entry %s/%s carries no reason or owner", knownRedManifestPath, entry.Package, entry.Test)
		}
	}
	return manifest
}

// TestKnownRedsManifestIsExactOverItsCoveredSets is the law. It runs each
// covered set once and compares the observed failure set to the manifest in
// both directions.
//
// The two-directional comparison reads observed and expected as whole sets
// merged across every covered set, so no covered set states its half of the
// property alone: the whole run is one subtest, and a pattern that does not
// name it runs none of the covered sets below.
func TestKnownRedsManifestIsExactOverItsCoveredSets(t *testing.T) {
	t.Run("law", func(t *testing.T) {
		manifest := loadKnownRedManifest(t)
		repository := architectureBatteryRepositoryRoot(t)
		excluded := make(map[string]knownRedEntry)
		for _, entry := range manifest.Excluded {
			excluded[entry.key()] = entry
		}
		expected := make(map[string]knownRedEntry)
		for _, entry := range manifest.Reds {
			if _, isExcluded := excluded[entry.key()]; isExcluded {
				t.Fatalf("%s lists %s/%s as both an expected red and an excluded test", knownRedManifestPath, entry.Package, entry.Test)
			}
			expected[entry.key()] = entry
		}
		observed := make(map[string]struct{})
		for _, set := range manifest.CoveredSets {
			for _, failure := range knownRedRunSet(t, repository, set, excluded) {
				observed[failure] = struct{}{}
			}
		}
		for _, key := range knownRedSortedKeys(observed) {
			if _, listed := expected[key]; !listed {
				t.Errorf("unlisted red: %s failed and is not in %s; a new failure is a regression, not a list entry", strings.ReplaceAll(key, "\t", " "), knownRedManifestPath)
			}
		}
		for key, entry := range expected {
			if _, still := observed[key]; !still {
				t.Errorf("%s/%s is green: remove from manifest %s (recorded reason: %s; owner: %s)", entry.Package, entry.Test, knownRedManifestPath, entry.Reason, entry.Owner)
			}
		}
		t.Logf("known reds: %d listed, %d observed, %d excluded from execution", len(expected), len(observed), len(excluded))
	})
}

// knownRedRunSet executes one covered set and returns its failure keys. The
// set's patterns are resolved to import paths first, so every key this law
// produces and every key the manifest records is a full import path; a set
// stated as a pattern and a manifest entry stated as a package then name the
// same thing.
//
// Packages carrying an excluded test run one at a time, because their selection
// is the complement of that exclusion. Everything else runs in one invocation,
// so the set costs one compile-and-run pass rather than one per package.
func knownRedRunSet(t *testing.T, repository string, set knownRedSet, excluded map[string]knownRedEntry) []string {
	t.Helper()
	packages := knownRedResolvePackages(t, repository, set.Packages)
	batched := make([]string, 0, len(packages))
	failures := make([]string, 0, 16)
	for _, pkg := range packages {
		names := knownRedExcludedNames(excluded, pkg)
		if len(names) == 0 {
			batched = append(batched, pkg)
			continue
		}
		filter := knownRedFilterExcluding(t, repository, pkg, set.Filter, names)
		failures = append(failures, knownRedParse(t, knownRedGo(t, repository, knownRedTestArguments(filter, []string{pkg})), []string{pkg}, set)...)
	}
	if len(batched) != 0 {
		failures = append(failures, knownRedParse(t, knownRedGo(t, repository, knownRedTestArguments(set.Filter, batched)), batched, set)...)
	}
	sort.Strings(failures)
	return failures
}

// knownRedTestArguments builds one go test invocation.
func knownRedTestArguments(filter string, packages []string) []string {
	arguments := []string{"test", "-count=1", "-timeout=0", "-json"}
	if filter != "" {
		arguments = append(arguments, "-run", filter)
	}
	return append(arguments, packages...)
}

// knownRedResolvePackages expands the set's patterns to import paths. A pattern
// that resolves to nothing fails: a covered set that names no package covers
// nothing, and would let the law pass by claiming an empty scope.
func knownRedResolvePackages(t *testing.T, repository string, patterns []string) []string {
	t.Helper()
	// A package without test files has no verdict to give, so it is not part of
	// a covered set; including it would report every such package as a failure
	// that never ran.
	listing := knownRedGo(t, repository, append([]string{"list", "-f", "{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}"}, patterns...))
	packages := make([]string, 0, 32)
	for _, line := range strings.Split(listing, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			packages = append(packages, line)
		}
	}
	if len(packages) == 0 {
		t.Fatalf("covered set patterns %s resolve to no package", strings.Join(patterns, " "))
	}
	sort.Strings(packages)
	return packages
}

// knownRedGo runs one go command from the repository root and returns its
// combined output. A non-zero status is expected here: a covered set with known
// reds always fails, and the verdict is read from the event stream, not the
// exit code.
func knownRedGo(t *testing.T, repository string, arguments []string) string {
	t.Helper()
	command := exec.Command("go", arguments...)
	command.Dir = repository
	command.Env = os.Environ()
	output, err := command.Output()
	if err != nil {
		if exitErr, isExit := err.(*exec.ExitError); isExit {
			if len(output) != 0 {
				return string(output)
			}
			t.Fatalf("go %s produced no event stream: %v\n%s", strings.Join(arguments, " "), err, exitErr.Stderr)
		}
		t.Fatalf("go %s: %v", strings.Join(arguments, " "), err)
	}
	return string(output)
}

// knownRedEvent is the subset of the go test event stream this law reads. A
// build failure is reported against an ImportPath rather than a Package and
// carries no package verdict at all, so both fields are read.
type knownRedEvent struct {
	Action     string `json:"Action"`
	Package    string `json:"Package"`
	ImportPath string `json:"ImportPath"`
	Test       string `json:"Test"`
	Output     string `json:"Output"`
}

// knownRedBuildPackage reduces a build ImportPath to the package it belongs to.
// The toolchain qualifies a test build as "path [path.test]".
func knownRedBuildPackage(importPath string) string {
	if index := strings.Index(importPath, " ["); index >= 0 {
		return importPath[:index]
	}
	return importPath
}

// knownRedParse reduces one event stream to failure keys. Every requested
// package must produce a verdict and must run at least one test: a package that
// did not build, or whose selection matched nothing, has no verdict to compare
// against the manifest, and is reported as its own unlisted failure key so the
// law fails loudly instead of reading silence as green.
func knownRedParse(t *testing.T, stream string, packages []string, set knownRedSet) []string {
	t.Helper()
	type packageState struct {
		verdict bool
		tests   int
	}
	states := make(map[string]*packageState, len(packages))
	for _, pkg := range packages {
		states[pkg] = &packageState{}
	}
	failures := make(map[string]struct{})
	var build strings.Builder
	for _, line := range strings.Split(stream, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var event knownRedEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("go test event stream is not JSON: %v\n%s", err, line)
		}
		if event.Action == "build-output" {
			build.WriteString(event.Output)
			continue
		}
		if event.Action == "build-fail" {
			pkg := knownRedBuildPackage(event.ImportPath)
			if state := states[pkg]; state != nil {
				state.verdict, state.tests = true, 1
			}
			failures[pkg+"\t<build failed>"] = struct{}{}
			continue
		}
		state := states[event.Package]
		if state == nil {
			state = &packageState{}
			states[event.Package] = state
		}
		switch event.Action {
		case "pass", "fail", "skip":
			if event.Test == "" {
				state.verdict = true
			} else {
				state.tests++
			}
		}
		if event.Test == "" || (event.Action != "fail" && event.Action != "skip") {
			continue
		}
		name := event.Test
		if !set.Subtests {
			if index := strings.IndexByte(name, '/'); index >= 0 {
				name = name[:index]
			}
		}
		// A skipped test is a covered test that produced no verdict, which is
		// the same defect the manifest exists to keep visible. It is recorded
		// under its own suffix so the manifest states which of its entries are
		// failing and which are not running at all, and so a skip that becomes
		// a failure is a manifest edit rather than a silent substitution.
		if event.Action == "skip" {
			name += knownRedSkippedSuffix
		}
		failures[event.Package+"\t"+name] = struct{}{}
	}
	if build.Len() != 0 {
		t.Logf("covered set %s did not build:\n%s", set.Name, build.String())
	}
	for pkg, state := range states {
		switch {
		case !state.verdict:
			failures[pkg+"\t<no verdict>"] = struct{}{}
		case state.tests == 0:
			failures[pkg+"\t<no test ran>"] = struct{}{}
		}
	}
	return knownRedSortedKeys(failures)
}

// knownRedExcludedNames lists the excluded test names of one package.
func knownRedExcludedNames(excluded map[string]knownRedEntry, pkg string) []string {
	names := make([]string, 0, 1)
	for _, entry := range excluded {
		if entry.Package == pkg {
			names = append(names, entry.Test)
		}
	}
	sort.Strings(names)
	return names
}

// knownRedFilterExcluding builds a -run selection naming every test of a
// package except the excluded ones. Go's selection has no negation, so the
// complement is enumerated from the package's own test list; a name that has
// disappeared from the package fails the law, because an exclusion for a test
// that no longer exists is stale debt.
func knownRedFilterExcluding(t *testing.T, repository, pkg, filter string, names []string) string {
	t.Helper()
	arguments := []string{"test", "-count=1", "-list", "."}
	if filter != "" {
		arguments = []string{"test", "-count=1", "-list", filter}
	}
	listing := knownRedGo(t, repository, append(arguments, pkg))
	excluded := make(map[string]struct{}, len(names))
	for _, name := range names {
		excluded[name] = struct{}{}
	}
	kept := make([]string, 0, 256)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(listing, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Test") {
			continue
		}
		seen[line] = struct{}{}
		if _, skip := excluded[line]; skip {
			continue
		}
		kept = append(kept, line)
	}
	for _, name := range names {
		if _, present := seen[name]; !present {
			t.Errorf("%s excludes %s from %s, but that test no longer exists; remove the exclusion", knownRedManifestPath, name, pkg)
		}
	}
	if len(kept) == 0 {
		t.Fatalf("%s: excluding %s left no test to run", pkg, strings.Join(names, ", "))
	}
	return "^(" + strings.Join(kept, "|") + ")$"
}

func knownRedSortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestKnownRedsManifestParsesItsOwnEventVocabulary proves the reduction the law
// depends on, so a manifest cannot pass because the stream reader stopped
// recognizing failures.
func TestKnownRedsManifestParsesItsOwnEventVocabulary(t *testing.T) {
	stream := strings.Join([]string{
		`{"Action":"run","Package":"p","Test":"TestA"}`,
		`{"Action":"fail","Package":"p","Test":"TestA/case-one"}`,
		`{"Action":"fail","Package":"p","Test":"TestA"}`,
		`{"Action":"pass","Package":"p","Test":"TestB"}`,
		`{"Action":"fail","Package":"p"}`,
	}, "\n")
	rolled := knownRedParse(t, stream, []string{"p"}, knownRedSet{Name: "probe"})
	if len(rolled) != 1 || rolled[0] != "p\tTestA" {
		t.Fatalf("top-level reduction = %v, want one p/TestA", rolled)
	}
	detailed := knownRedParse(t, stream, []string{"p"}, knownRedSet{Name: "probe", Subtests: true})
	if len(detailed) != 2 || detailed[0] != "p\tTestA" || detailed[1] != "p\tTestA/case-one" {
		t.Fatalf("subtest reduction = %v, want p/TestA and p/TestA/case-one", detailed)
	}
	empty := knownRedParse(t, `{"Action":"pass","Package":"p"}`, []string{"p"}, knownRedSet{Name: "probe"})
	if len(empty) != 1 || empty[0] != "p\t<no test ran>" {
		t.Fatalf("empty selection reduction = %v, want a package-level failure", empty)
	}
	build := knownRedParse(t, strings.Join([]string{
		`{"Action":"build-output","ImportPath":"p [p.test]","Output":"undefined: X\n"}`,
		`{"Action":"build-fail","ImportPath":"p [p.test]"}`,
	}, "\n"), []string{"p"}, knownRedSet{Name: "probe"})
	if len(build) != 1 || build[0] != "p\t<build failed>" {
		t.Fatalf("build failure reduction = %v, want a build failure", build)
	}
	silent := knownRedParse(t, `{"Action":"output","Package":"p","Output":"panic"}`, []string{"p"}, knownRedSet{Name: "probe"})
	if len(silent) != 1 || silent[0] != "p\t<no verdict>" {
		t.Fatalf("missing verdict reduction = %v, want a no-verdict failure", silent)
	}
	skipped := knownRedParse(t, strings.Join([]string{
		`{"Action":"skip","Package":"p","Test":"TestC"}`,
		`{"Action":"pass","Package":"p"}`,
	}, "\n"), []string{"p"}, knownRedSet{Name: "probe"})
	if len(skipped) != 1 || skipped[0] != "p\tTestC"+knownRedSkippedSuffix {
		t.Fatalf("skip reduction = %v, want a listed skip entry", skipped)
	}
}

// TestKnownRedsManifestIsWellFormed reads the manifest without running anything,
// so a malformed or unattributed entry is reported even when the covered sets
// are too expensive to have been reached.
func TestKnownRedsManifestIsWellFormed(t *testing.T) {
	manifest := loadKnownRedManifest(t)
	if manifest.Measured == "" {
		t.Errorf("%s records no measurement date", knownRedManifestPath)
	}
	seen := make(map[string]struct{}, len(manifest.Reds))
	for _, entry := range manifest.Reds {
		if _, duplicate := seen[entry.key()]; duplicate {
			t.Errorf("%s lists %s/%s twice", knownRedManifestPath, entry.Package, entry.Test)
		}
		seen[entry.key()] = struct{}{}
	}
	for _, entry := range manifest.Excluded {
		if !entry.Excluded {
			t.Errorf("%s: excluded entry %s/%s is not marked excluded", knownRedManifestPath, entry.Package, entry.Test)
		}
	}
	t.Logf("manifest %s measured %s: %d reds, %d excluded, %d covered sets", knownRedManifestPath, manifest.Measured, len(manifest.Reds), len(manifest.Excluded), len(manifest.CoveredSets))
}
