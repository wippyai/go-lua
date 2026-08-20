package oracle

import (
	"context"
	"fmt"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/result"
	effectpublication "github.com/wippyai/go-lua/domain/effect/publication"
	valuepublication "github.com/wippyai/go-lua/domain/value/publication"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// The corpus harness is this package's single fixture spine. One enumeration
// names every frozen fixture and binds it to its source-authored expectation;
// one path seals, compiles, solves, judges the detached Result, and closes the
// plan. A test supplies only the analyzer entry it exercises and the judgment
// it applies, never its own corpus walk: independent walks are how a census
// stays green while acceptance never runs.
//
// The kit reaches the analyzer through its published surface alone. That is
// what keeps these gates runnable while the package they measure is being
// rebuilt: a law here can only be red because the analyzer is red, never
// because an internal name it borrowed moved.
//
// There is deliberately no timeout, work budget, result cap, or skip here. The
// repository's bounded runner owns process-tree resource enforcement, so a
// killed shard is a failed shard, never a passed one.

const (
	corpusHarnessProjectCount = 912
	corpusHarnessLuaFileCount = testfixture.FrozenLuaFileCount
	// Keep corpus concurrency independent of both fixture count and unusually
	// large host CPU counts. Thirty-two is the measured 912-corpus lane; the
	// repository's bounded runner remains the hard RSS/time authority.
	corpusHarnessMaxWorkers = 32
)

// corpusHarnessExecution selects which public analyzer entry the spine drives.
// Every fixture is loaded, sealed, judged, and closed identically; only the
// entry under test differs.
type corpusHarnessExecution uint8

const (
	// corpusHarnessAnalyzeOnce drives the public one-shot Analyze.
	corpusHarnessAnalyzeOnce corpusHarnessExecution = iota
	// corpusHarnessDiagnosticSolve drives Compile plus SolveWithDiagnostics.
	corpusHarnessDiagnosticSolve
	// corpusHarnessReportSolve drives Compile plus a policy SolveWithReport.
	corpusHarnessReportSolve
	// corpusHarnessCompileOnly stops at the compiled plan, for laws that judge
	// the compile receipt boundary itself.
	corpusHarnessCompileOnly
)

// corpusHarnessMode is one judgment plugged into the spine. policy, preflight,
// and judge are the only mode-owned decisions; loading, sealing, compiling,
// solving, the detached-Result contract, and plan closure are shared.
type corpusHarnessMode struct {
	name      string
	execution corpusHarnessExecution
	options   engine.SolveDiagnosticOptions
	// preflight fences fixture contracts a mode cannot judge, before compile.
	preflight func(*corpusHarnessProject) []string
	// policy derives the per-fixture diagnostic policy of a report execution.
	// Contracts it cannot express are carried into the run and judged after the
	// fixture has passed through the current analyzer, never before.
	policy func(*corpusHarnessProject) (anadiag.DiagnosticPolicy, []string)
	// judge is the mode's verdict on one completed run.
	judge func(*corpusHarnessRun) []string
}

// corpusHarnessProject is one fixture as both an executable Link source and a
// source-authored expectation. Binding them in one enumeration is what keeps
// the census and the acceptance oracle on the same corpus.
type corpusHarnessProject struct {
	name        string
	source      testfixture.CorpusProject
	expectation *corpusDiagnosticProjectExpectations
}

// corpusHarnessCost is per-fixture accounting. Elapsed time is exact per
// fixture; allocation is exact only in a serial walk, where no other fixture
// is running, and is reported as unavailable otherwise. The one-shot Analyze
// entry reports its whole run as solve, because it owns its own compile.
type corpusHarnessCost struct {
	seal, compile, solve time.Duration
	allocated            uint64
	allocationExact      bool
}

func (cost corpusHarnessCost) total() time.Duration {
	return cost.seal + cost.compile + cost.solve
}

type corpusHarnessRun struct {
	project            corpusHarnessProject
	linked             *link.Link
	plan               *analysis.Plan
	result             *result.Result
	report             *anadiag.DiagnosticReport
	status             analysis.AnalyzeStatus
	compileDiagnostics anadiag.AnalyzeDiagnostics
	solveDiagnostics   anadiag.AnalyzeDiagnostics
	policy             anadiag.DiagnosticPolicy
	policyUnsupported  []string
	cost               corpusHarnessCost
}

// corpusHarnessOutcome is one fixture's classified walk verdict.
type corpusHarnessOutcome struct {
	project string
	status  analysis.AnalyzeStatus
	class   string
	err     error
	cost    corpusHarnessCost
}

var (
	corpusHarnessOnce          sync.Once
	corpusHarnessProjectsValue []corpusHarnessProject
	corpusHarnessProjectsErr   error
)

// corpusHarnessProjects is the single fixture enumeration. It cross-checks the
// executable corpus census against the manifest expectation catalog so no mode
// can silently judge a different corpus than another.
func corpusHarnessProjects(t *testing.T) []corpusHarnessProject {
	t.Helper()
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	corpusHarnessOnce.Do(func() {
		corpusHarnessProjectsValue, corpusHarnessProjectsErr = buildCorpusHarnessProjects(architectureBatteryRepositoryRoot(t), catalog)
	})
	if corpusHarnessProjectsErr != nil {
		t.Fatal(corpusHarnessProjectsErr)
	}
	return corpusHarnessProjectsValue
}

func buildCorpusHarnessProjects(repository string, catalog *corpusDiagnosticExpectationCatalog) ([]corpusHarnessProject, error) {
	if catalog == nil {
		return nil, fmt.Errorf("frozen corpus expectation catalog is unavailable")
	}
	corpus, err := testfixture.LoadCorpus(repository)
	if err != nil {
		return nil, fmt.Errorf("load frozen corpus: %w", err)
	}
	sources := corpus.Projects()
	if len(sources) != corpusHarnessProjectCount || len(catalog.projects) != corpusHarnessProjectCount || catalog.inventory.projects != corpusHarnessProjectCount {
		return nil, fmt.Errorf("frozen corpus projects = %d executable, %d expectation, %d inventory, want exactly %d", len(sources), len(catalog.projects), catalog.inventory.projects, corpusHarnessProjectCount)
	}
	projects := make([]corpusHarnessProject, 0, len(sources))
	files := 0
	for index, source := range sources {
		expectation := catalog.projects[index]
		if expectation == nil || expectation.name != source.Name() {
			name := ""
			if expectation != nil {
				name = expectation.name
			}
			return nil, fmt.Errorf("frozen corpus enumerations diverge at %d: executable %q, expectation %q", index, source.Name(), name)
		}
		if len(expectation.files) != source.FileCount() {
			return nil, fmt.Errorf("frozen corpus fixture %q has %d executable Lua files and %d expectation files", source.Name(), source.FileCount(), len(expectation.files))
		}
		files += source.FileCount()
		projects = append(projects, corpusHarnessProject{name: source.Name(), source: source, expectation: expectation})
	}
	if files != corpusHarnessLuaFileCount || catalog.inventory.luaFiles != corpusHarnessLuaFileCount {
		return nil, fmt.Errorf("frozen corpus Lua files = %d executable, %d expectation, want exactly %d", files, catalog.inventory.luaFiles, corpusHarnessLuaFileCount)
	}
	return projects, nil
}

// corpusHarnessShard selects one canonical fixture-path prefix.
func corpusHarnessShard(t *testing.T, prefix string) []corpusHarnessProject {
	t.Helper()
	projects := corpusHarnessProjects(t)
	selected := make([]corpusHarnessProject, 0, len(projects))
	for _, project := range projects {
		if strings.HasPrefix(project.name, prefix) {
			selected = append(selected, project)
		}
	}
	if len(selected) == 0 {
		t.Fatalf("%s frozen-corpus shard is empty", prefix)
	}
	return selected
}

func corpusHarnessFixture(t *testing.T, name string) corpusHarnessProject {
	t.Helper()
	projects := corpusHarnessProjects(t)
	index := sort.Search(len(projects), func(index int) bool { return projects[index].name >= name })
	if index >= len(projects) || projects[index].name != name {
		t.Fatalf("missing fixture project %q", name)
	}
	return projects[index]
}

func corpusHarnessContract(t testing.TB) *contract.Contract {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	return contract
}

// corpusHarnessSolveOptions is the shared fixture solve selection: complete
// engine evidence with a bounded row projection, and no work budget, so a
// non-terminating fixture is caught by the bounded runner rather than passing
// as a cut-off sample.
func corpusHarnessSolveOptions() engine.SolveDiagnosticOptions {
	return engine.SolveDiagnosticOptions{
		Presentation: engine.SolveDiagnosticPresentation{Flags: engine.SolveDiagnosticAll},
		Resources:    engine.SolveDiagnosticResources{MaxRows: 256},
	}
}

// TestCorpusFixtureDeclaredModulesAreSealed is the multi-module fixture input
// law. A manifest declares a module inventory and an entry, and the sealed Link
// must hold that whole contract: every declared file is one mount, every mount
// carries an analysis root, and the entry root's module-cache closure reaches
// every declared module. Mount order is Link-canonical by Program identity, so
// the declared order is carried by the inventory and its ingress rather than by
// a position in the manifest.
func TestCorpusFixtureDeclaredModulesAreSealed(t *testing.T) {
	project := corpusHarnessFixture(t, "realworld/lookup-table-cast")
	expectation := project.expectation
	if expectation == nil || len(expectation.declaredFiles) < 2 || expectation.entryModule == "" {
		t.Fatalf("fixture %q does not declare a multi-module inventory", project.name)
	}
	linked, err := testfixture.SealCorpusProject(corpusHarnessContract(t), project.source)
	if err != nil {
		t.Fatal(err)
	}
	mounts := linked.Project().Mounts()
	roots := linked.Module().Roots()
	if mounts.Count() != len(expectation.declaredFiles) {
		t.Fatalf("sealed mounts=%d, declared modules=%d", mounts.Count(), len(expectation.declaredFiles))
	}
	shards := make(map[string]linkproject.Shard, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			t.Fatalf("mount %d is not addressable", index)
		}
		name, ok := mounts.Name(shard)
		if !ok {
			t.Fatalf("mount %d has no module name", index)
		}
		if roots.ForShardCount(shard) == 0 {
			t.Fatalf("mounted module %q carries no analysis root", name)
		}
		shards[name] = shard
	}
	for _, file := range expectation.declaredFiles {
		if _, mounted := shards[strings.TrimSuffix(file, ".lua")]; !mounted {
			t.Fatalf("declared module %q is not mounted", file)
		}
	}
	entry, mounted := shards[expectation.entryModule]
	if !mounted {
		t.Fatalf("selected entry module %q is not mounted", expectation.entryModule)
	}
	reached := corpusHarnessModuleClosure(t, linked, entry)
	for name := range shards {
		if !reached[name] {
			t.Fatalf("module %q is unreachable from the selected entry %q", name, expectation.entryModule)
		}
	}
}

// corpusHarnessModuleClosure walks the sealed module-cache ingress from one
// mount and names every module it loads, directly or transitively.
func corpusHarnessModuleClosure(t *testing.T, linked *link.Link, entry linkproject.Shard) map[string]bool {
	t.Helper()
	mounts := linked.Project().Mounts()
	roots := linked.Module().Roots()
	cache := linked.Module().Cache()
	loads := make(map[linkproject.Shard][]linkproject.Shard, mounts.Count())
	for index := 0; index < cache.EntryCount(); index++ {
		entryRow, ok := cache.EntryAt(index)
		if !ok {
			t.Fatalf("module cache entry %d is not addressable", index)
		}
		_, from, to, ok := cache.EntryMapping(entryRow)
		if !ok {
			t.Fatalf("module cache entry %d has no root mapping", index)
		}
		fromShard, _, _, fromOK := roots.Mapping(from)
		toShard, _, _, toOK := roots.Mapping(to)
		if !fromOK || !toOK {
			t.Fatalf("module cache entry %d has no mount mapping", index)
		}
		loads[fromShard] = append(loads[fromShard], toShard)
	}
	reached := make(map[string]bool, mounts.Count())
	frontier := []linkproject.Shard{entry}
	for len(frontier) != 0 {
		shard := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		name, ok := mounts.Name(shard)
		if !ok {
			t.Fatal("reached mount has no module name")
		}
		if reached[name] {
			continue
		}
		reached[name] = true
		frontier = append(frontier, loads[shard]...)
	}
	return reached
}

// corpusHarnessSourceText reads one file of a fixture project. The fixture
// directory stays owned by the corpus; no test reconstructs a corpus path.
func corpusHarnessSourceText(t testing.TB, project corpusHarnessProject, file string) []byte {
	t.Helper()
	contents, err := project.source.SourceText(filepath.FromSlash(file))
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

// corpusHarnessFixtureRun runs one named fixture through the spine and fails
// the calling test on any classified failure.
func corpusHarnessFixtureRun(t *testing.T, name string, mode corpusHarnessMode) *corpusHarnessRun {
	t.Helper()
	run, _, err := corpusHarnessExecute(t, corpusHarnessFixture(t, name), mode)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

// corpusHarnessExecute is the spine: seal, compile, solve, judge the detached
// Result, then apply the mode's judgment. The compiled plan is closed when t
// completes, so a walk holds at most one live plan per worker. Failures are
// returned classified instead of raised, so a single-fixture law can fail in
// place and a walk can group the same verdict.
func corpusHarnessExecute(t testing.TB, project corpusHarnessProject, mode corpusHarnessMode) (*corpusHarnessRun, string, error) {
	t.Helper()
	run := &corpusHarnessRun{project: project}
	if project.name == "" || project.expectation == nil {
		return run, "fixture", fmt.Errorf("unavailable fixture project or expectation")
	}
	if mode.preflight != nil {
		if unsupported := mode.preflight(&run.project); len(unsupported) != 0 {
			return run, "fixture-contract", fmt.Errorf("unsupported fixture contract before compile:\n%s", strings.Join(unsupported, "\n"))
		}
	}
	contract := corpusHarnessContract(t)
	started := time.Now()
	linked, err := testfixture.SealCorpusProject(contract, project.source)
	run.cost.seal = time.Since(started)
	if err != nil {
		return run, "link", fmt.Errorf("link: %w", err)
	}
	run.linked = linked
	return corpusHarnessExecuteLink(t, run, mode)
}

// corpusHarnessExecuteLink runs an already sealed Link through the spine. Its
// caller owns how that Link was constructed; everything after it is shared.
func corpusHarnessExecuteLink(t testing.TB, run *corpusHarnessRun, mode corpusHarnessMode) (*corpusHarnessRun, string, error) {
	t.Helper()
	if run == nil || run.linked == nil {
		return run, "link", fmt.Errorf("unavailable sealed Link")
	}
	linked := run.linked
	if mode.execution == corpusHarnessAnalyzeOnce {
		started := time.Now()
		run.result, run.status = analysis.Analyze(context.Background(), linked)
		run.cost.solve = time.Since(started)
		if run.status != analysis.AnalyzeComplete {
			class := corpusHarnessStatusName(run.status)
			return run, class, fmt.Errorf("Analyze status = %s", class)
		}
	} else {
		started := time.Now()
		plan, compileStatus, compileDiagnostics := analysis.CompileWithDiagnostics(linked)
		run.cost.compile = time.Since(started)
		run.compileDiagnostics = compileDiagnostics
		if compileStatus != analysis.CompileComplete || plan == nil {
			return run, "compile", fmt.Errorf("compile=%v plan=%t stage=%v binding=%v axis=%v seal=%v schedule=%d diagnostics=%+v",
				compileStatus, plan != nil, compileDiagnostics.AssembleStage, compileDiagnostics.Binding, compileDiagnostics.Axis,
				compileDiagnostics.AssembleSeal, compileDiagnostics.AssembleScheduleOrdinal, compileDiagnostics)
		}
		run.plan = plan
		// A fixture is a sequential acceptance unit. Close the assembled Link
		// topology on every post-compile path, including policy, solve, report,
		// and matcher failures; immutable Program artifacts remain cache-owned.
		t.Cleanup(func() {
			if !plan.Close() {
				t.Error("close compiled fixture plan")
			}
		})
		if mode.execution == corpusHarnessCompileOnly {
			return run, "", nil
		}
		if class, err := corpusHarnessSolve(run, mode); err != nil {
			return run, class, err
		}
	}
	if err := corpusHarnessResultDefect(run.result, linked.ContentID()); err != nil {
		return run, "detached-result", err
	}
	if mode.judge != nil {
		if mismatches := mode.judge(run); len(mismatches) != 0 {
			class := mode.name
			if class == "" {
				class = "judgment"
			}
			return run, class, fmt.Errorf("%s:\n%s", class, strings.Join(mismatches, "\n"))
		}
	}
	return run, "", nil
}

func corpusHarnessSolve(run *corpusHarnessRun, mode corpusHarnessMode) (string, error) {
	started := time.Now()
	switch mode.execution {
	case corpusHarnessDiagnosticSolve:
		run.result, run.status, run.solveDiagnostics = run.plan.SolveWithDiagnostics(context.Background(), mode.options)
	case corpusHarnessReportSolve:
		if mode.policy != nil {
			run.policy, run.policyUnsupported = mode.policy(&run.project)
		}
		run.result, run.report, run.status, run.solveDiagnostics = run.plan.SolveWithReport(context.Background(), mode.options, run.policy)
	default:
		return "execution", fmt.Errorf("unknown corpus harness execution %d", mode.execution)
	}
	run.cost.solve = time.Since(started)
	if run.status != analysis.AnalyzeComplete || run.result == nil {
		return corpusHarnessStatusName(run.status), fmt.Errorf("AnalyzeComplete required: status=%v result=%t binding=%v stage=%v axis=%v engine=%s diagnostics=%+v",
			run.status, run.result != nil, run.solveDiagnostics.Binding, run.solveDiagnostics.AssembleStage, run.solveDiagnostics.Axis,
			corpusHarnessEngineFailure(run.solveDiagnostics), run.solveDiagnostics)
	}
	return "", nil
}

// corpusHarnessResultDefect is the detached public Result contract every mode
// applies. A Result that cannot name its own source, bodies, roots, or typed
// query publications is not a clean analysis regardless of the mode's own
// verdict.
func corpusHarnessResultDefect(analysisResult *result.Result, sourceID identity.ContentID) error {
	if analysisResult == nil {
		return fmt.Errorf("nil result")
	}
	if !analysisResult.ContentID().Available() || !analysisResult.SourceID().Available() || analysisResult.SourceID() != sourceID {
		return fmt.Errorf("invalid source/content identity")
	}
	if analysisResult.BodyCount() == 0 {
		return fmt.Errorf("empty body projection")
	}
	for bodyIndex := 0; bodyIndex < analysisResult.BodyCount(); bodyIndex++ {
		body, ok := analysisResult.BodyAt(bodyIndex)
		if !ok {
			return fmt.Errorf("body %d is not addressable", bodyIndex)
		}
		if id, ok := body.ID(); !ok || !id.Available() {
			return fmt.Errorf("body %d has no detached identity", bodyIndex)
		}
		for rootIndex := 0; rootIndex < body.RootCount(); rootIndex++ {
			root, ok := body.RootAt(rootIndex)
			if !ok {
				return fmt.Errorf("body %d root %d is not addressable", bodyIndex, rootIndex)
			}
			if id, ok := root.ID(); !ok || !id.Available() {
				return fmt.Errorf("body %d root %d has no detached identity", bodyIndex, rootIndex)
			}
		}
	}

	valueFamily, valueFamilyOK := valuepublication.Open(analysisResult)
	if !valueFamilyOK {
		return fmt.Errorf("value publication family unavailable")
	}
	effectFamily, effectFamilyOK := effectpublication.Open(analysisResult)
	if !effectFamilyOK {
		return fmt.Errorf("effect publication family unavailable")
	}

	validateBodies := func(family string, queryIndex, bodyCount int, bodyAt func(int) (result.Body, bool)) error {
		for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
			body, ok := bodyAt(bodyIndex)
			if !ok {
				return fmt.Errorf("%s query %d body %d is not readable", family, queryIndex, bodyIndex)
			}
			if id, ok := body.ID(); !ok || !id.Available() {
				return fmt.Errorf("%s query %d body %d has no readable identity", family, queryIndex, bodyIndex)
			}
		}
		return nil
	}

	for queryIndex := 0; queryIndex < valueFamily.QueryCount(); queryIndex++ {
		query, queryOK := valueFamily.QueryAt(queryIndex)
		if !queryOK {
			return fmt.Errorf("value query %d is not addressable", queryIndex)
		}
		if id, ok := query.SiteID(); !ok || !id.Available() {
			return fmt.Errorf("value query %d has no site identity", queryIndex)
		}
		if id, ok := query.MountID(); !ok || !id.Available() {
			return fmt.Errorf("value query %d has no mount identity", queryIndex)
		}
		if id, ok := query.PointID(); !ok || !id.Available() {
			return fmt.Errorf("value query %d has no point identity", queryIndex)
		}
		if err := validateBodies("value", queryIndex, query.BodyCount(), query.BodyAt); err != nil {
			return err
		}
		switch query.Status() {
		case result.QueryProvenAbsent:
			// Proven absence is a publication outcome, not a typed payload. It
			// remains valid without asking the domain facade to decode a cell.
		case result.QueryHit:
			summary, summaryOK := query.Summary()
			if !summaryOK || !summary.Available() {
				return fmt.Errorf("value query %d summary payload is unreadable", queryIndex)
			}
			coordinates := summary.Coordinates()
			for coordinateIndex := 0; coordinateIndex < summary.CoordinateCount(); coordinateIndex++ {
				coordinate, coordinateOK := coordinates.Next()
				id := coordinate.ID()
				if !coordinateOK || !coordinate.Available() || !id.Available() {
					return fmt.Errorf("value query %d coordinate %d has no available identity", queryIndex, coordinateIndex)
				}
			}
			if _, trailing := coordinates.Next(); trailing {
				return fmt.Errorf("value query %d summary has trailing coordinates", queryIndex)
			}
		default:
			return fmt.Errorf("value query %d has invalid publication status", queryIndex)
		}
	}

	for queryIndex := 0; queryIndex < effectFamily.QueryCount(); queryIndex++ {
		query, queryOK := effectFamily.QueryAt(queryIndex)
		if !queryOK {
			return fmt.Errorf("effect query %d is not addressable", queryIndex)
		}
		if id, ok := query.SiteID(); !ok || !id.Available() {
			return fmt.Errorf("effect query %d has no site identity", queryIndex)
		}
		if id, ok := query.MountID(); !ok || !id.Available() {
			return fmt.Errorf("effect query %d has no mount identity", queryIndex)
		}
		if id, ok := query.PointID(); !ok || !id.Available() {
			return fmt.Errorf("effect query %d has no point identity", queryIndex)
		}
		if err := validateBodies("effect", queryIndex, query.BodyCount(), query.BodyAt); err != nil {
			return err
		}
		switch query.Status() {
		case result.QueryProvenAbsent:
			// Proven absence is deliberately left undecoded, just as for the
			// value family.
		case result.QueryHit:
			effect, effectOK := query.Effect()
			if !effectOK || !effect.Available() {
				return fmt.Errorf("effect query %d effect payload is unreadable", queryIndex)
			}
			for atomIndex := 0; atomIndex < effect.AtomCount(); atomIndex++ {
				id, atomOK := effect.AtomAt(atomIndex)
				if !atomOK || !id.Available() {
					return fmt.Errorf("effect query %d atom %d has no available identity", queryIndex, atomIndex)
				}
			}
		default:
			return fmt.Errorf("effect query %d has invalid publication status", queryIndex)
		}
	}
	return nil
}

// corpusHarnessWalk runs one mode over the selected fixtures. Every fixture is
// its own named subtest, so a failure names its fixture and the walk holds at
// most one live plan per worker. The returned outcomes carry the same verdict
// the subtests reported, for the caller's shard receipt.
func corpusHarnessWalk(t *testing.T, projects []corpusHarnessProject, mode corpusHarnessMode) []corpusHarnessOutcome {
	outcomes := make([]corpusHarnessOutcome, len(projects))
	workers := corpusHarnessWorkerCount(len(projects))
	if workers == 0 {
		return outcomes
	}
	serial := workers == 1
	var next atomic.Int64
	var walkers sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		walkers.Add(1)
		go func() {
			defer walkers.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(projects) {
					return
				}
				project := projects[index]
				t.Run(project.name, func(t *testing.T) {
					allocated := corpusHarnessAllocated(serial)
					run, class, err := corpusHarnessExecute(t, project, mode)
					run.cost.allocated, run.cost.allocationExact = corpusHarnessAllocated(serial)-allocated, serial
					outcomes[index] = corpusHarnessOutcome{project: project.name, status: run.status, class: class, err: err, cost: run.cost}
					if err != nil {
						t.Fatal(err)
					}
				})
			}
		}()
	}
	walkers.Wait()
	return outcomes
}

// corpusHarnessAllocated reads the cumulative allocation counter. It is only
// attributable to one fixture in a serial walk, so a concurrent walk does not
// pay for the stop-the-world read.
func corpusHarnessAllocated(serial bool) uint64 {
	if !serial {
		return 0
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.TotalAlloc
}

func corpusHarnessWorkerCount(projects int) int {
	if projects <= 0 {
		return 0
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > corpusHarnessMaxWorkers {
		workers = corpusHarnessMaxWorkers
	}
	if workers > projects {
		workers = projects
	}
	return workers
}

// corpusHarnessShardReceipt is the walk's status line: what ran, what the
// public status census was, and what it cost. Failures are reported by their
// own subtests; this receipt exists so a shard's cost stays visible instead of
// becoming an unexplained bounded-runner kill.
func corpusHarnessShardReceipt(shard string, outcomes []corpusHarnessOutcome) string {
	var counts [4]int
	var wall time.Duration
	failures := 0
	for _, outcome := range outcomes {
		if outcome.status >= analysis.AnalyzeInvalid && int(outcome.status) < len(counts) {
			counts[outcome.status]++
		}
		if outcome.err != nil {
			failures++
		}
		wall += outcome.cost.total()
	}
	var receipt strings.Builder
	fmt.Fprintf(&receipt, "corpus %s: fixtures=%d complete=%d incomplete=%d unsupported=%d invalid=%d failed=%d analysis-wall=%s",
		shard, len(outcomes), counts[analysis.AnalyzeComplete], counts[analysis.AnalyzeIncomplete], counts[analysis.AnalyzeUnsupported], counts[analysis.AnalyzeInvalid], failures, wall.Round(time.Millisecond))
	receipt.WriteString(corpusHarnessCostReport(outcomes, 5))
	return receipt.String()
}

// corpusHarnessCostReport names the most expensive fixtures of a walk. Time is
// always exact; allocation is reported only from a serial walk, where it is
// attributable to a single fixture.
func corpusHarnessCostReport(outcomes []corpusHarnessOutcome, rows int) string {
	if rows < 1 || len(outcomes) == 0 {
		return ""
	}
	ranked := make([]corpusHarnessOutcome, 0, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.project != "" {
			ranked = append(ranked, outcome)
		}
	}
	if len(ranked) == 0 {
		return ""
	}
	sort.Slice(ranked, func(left, right int) bool {
		return ranked[left].cost.total() > ranked[right].cost.total()
	})
	if len(ranked) > rows {
		ranked = ranked[:rows]
	}
	var report strings.Builder
	report.WriteString("\nslowest fixtures:")
	for _, outcome := range ranked {
		fmt.Fprintf(&report, "\n  %s seal=%s compile=%s solve=%s", outcome.project, outcome.cost.seal.Round(time.Millisecond), outcome.cost.compile.Round(time.Millisecond), outcome.cost.solve.Round(time.Millisecond))
		if outcome.cost.allocationExact {
			fmt.Fprintf(&report, " allocated=%dMiB", outcome.cost.allocated/(1<<20))
		}
	}
	return report.String()
}

// corpusHarnessFailureReport groups walk failures by class with a bounded
// per-class detail budget, so one systemic regression cannot flood a shard's
// receipt with one row per fixture.
func corpusHarnessFailureReport(outcomes []corpusHarnessOutcome, perClass int) string {
	if perClass < 1 {
		perClass = 1
	}
	grouped := make(map[string][]string)
	for _, outcome := range outcomes {
		if outcome.err == nil {
			continue
		}
		class := outcome.class
		if class == "" {
			class = "unknown"
		}
		grouped[class] = append(grouped[class], fmt.Sprintf("%s (%v)", outcome.project, outcome.err))
	}
	if len(grouped) == 0 {
		return ""
	}
	classes := make([]string, 0, len(grouped))
	for class := range grouped {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	var report strings.Builder
	report.WriteString("canonical corpus failures")
	for _, class := range classes {
		rows := grouped[class]
		fmt.Fprintf(&report, "\n%s: %d", class, len(rows))
		limit := len(rows)
		if limit > perClass {
			limit = perClass
		}
		for _, row := range rows[:limit] {
			report.WriteString("\n  ")
			report.WriteString(row)
		}
		if len(rows) > limit {
			fmt.Fprintf(&report, "\n  ... %d more", len(rows)-limit)
		}
	}
	return report.String()
}

func corpusHarnessStatusName(status analysis.AnalyzeStatus) string {
	switch status {
	case analysis.AnalyzeInvalid:
		return "invalid"
	case analysis.AnalyzeUnsupported:
		return "unsupported"
	case analysis.AnalyzeIncomplete:
		return "incomplete"
	case analysis.AnalyzeComplete:
		return "complete"
	default:
		return fmt.Sprintf("unknown(%d)", status)
	}
}

// corpusHarnessEngineFailure is the compact engine evidence every failing
// solve reports. It replaces the per-test diagnostic dumps that used to make
// one fixture lane readable and the rest opaque.
func corpusHarnessEngineFailure(diagnostics anadiag.AnalyzeDiagnostics) string {
	failure := diagnostics.Engine.Failure
	return fmt.Sprintf("phase=%s reason=%s rule=%s epochs=%d passes=%d evaluates=%d fails=%d folds=%d restarts=%d activations=%d failure={available:%t reason:%d boundary:%s point:%v group:%v member:%v rule:%v}",
		diagnostics.Phase, diagnostics.Reason, diagnostics.Rule,
		diagnostics.Engine.Epochs, diagnostics.Engine.EpochPasses, diagnostics.Engine.Evaluates, diagnostics.Engine.EvaluateFailures,
		diagnostics.Engine.Folds, diagnostics.Engine.Restarts, diagnostics.Engine.Activations,
		failure.Available(), failure.Reason(), failure.Failure(), failure.Point(), failure.Group(), failure.Member(), failure.Rule())
}

// corpusHarnessCensusMode is the honest status census: the public one-shot
// entry plus the detached Result contract. Diagnostics and fixture
// expectations belong to the acceptance mode.
func corpusHarnessCensusMode() corpusHarnessMode {
	return corpusHarnessMode{name: "census", execution: corpusHarnessAnalyzeOnce}
}

// corpusHarnessDiagnosticMode is the root diagnostic integration lane: one
// compile and one diagnostic solve, reporting the exact construction boundary
// on failure.
func corpusHarnessDiagnosticMode() corpusHarnessMode {
	return corpusHarnessMode{name: "diagnostic", execution: corpusHarnessDiagnosticSolve, options: corpusHarnessSolveOptions()}
}

// corpusHarnessCompileMode stops at the compiled plan, for laws that judge the
// compile receipt boundary without depending on a later solve.
func corpusHarnessCompileMode() corpusHarnessMode {
	return corpusHarnessMode{name: "compile", execution: corpusHarnessCompileOnly}
}
