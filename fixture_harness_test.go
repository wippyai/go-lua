package lua

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/placementplan"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	typemanifest "github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Suite describes a fixture suite loaded from manifest.json.
type fixtureSuite struct {
	Description     string        `json:"description,omitempty"`
	Files           []string      `json:"files,omitempty"`
	Stdlib          *bool         `json:"stdlib,omitempty"`
	Packages        []string      `json:"packages,omitempty"` // predefined system packages: "channel", "process", "resource", "time", "funcs", "uuid"
	DeadlineSeconds int           `json:"deadline_seconds,omitempty"`
	Serial          bool          `json:"serial,omitempty"`
	Check           *fixtureCheck `json:"check,omitempty"`
	Run             *fixtureRun   `json:"run,omitempty"`
	Bench           *fixtureBench `json:"bench,omitempty"`
	Skip            string        `json:"skip,omitempty"`
}

type fixtureCheck struct {
	Errors          *int                           `json:"errors,omitempty"`
	Diagnostics     []fixtureDiagnosticExpectation `json:"diagnostics,omitempty"`
	DiagnosticRules []fixtureDiagnosticRule        `json:"diagnostic_rules,omitempty"`
	RenderOptions   fixtureDiagnosticRenderConfig  `json:"render_options,omitempty"`
	Placement       *fixturePlacement              `json:"placement,omitempty"`
	Skip            string                         `json:"skip,omitempty"`
}

type fixtureDiagnosticRenderConfig struct {
	WitnessTrace bool `json:"witness_trace,omitempty"`
}

type fixtureDiagnosticRule struct {
	Code     string `json:"code,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type fixtureDiagnosticExpectation struct {
	File                  string                                 `json:"file,omitempty"`
	Line                  int                                    `json:"line,omitempty"`
	Column                int                                    `json:"column,omitempty"`
	Severity              string                                 `json:"severity,omitempty"`
	Code                  string                                 `json:"code,omitempty"`
	MessageContains       []string                               `json:"message_contains,omitempty"`
	EvidenceContains      []string                               `json:"evidence_contains,omitempty"`
	Evidence              []fixtureDiagnosticEvidenceExpectation `json:"evidence,omitempty"`
	RenderContains        []string                               `json:"render_contains,omitempty"`
	RenderOrderedContains []string                               `json:"render_ordered_contains,omitempty"`
	RenderNotContains     []string                               `json:"render_not_contains,omitempty"`
	HelpContains          []string                               `json:"help_contains,omitempty"`
	LabelContains         []string                               `json:"label_contains,omitempty"`
	Labels                []fixtureDiagnosticLabelExpectation    `json:"labels,omitempty"`
	MinEvidence           int                                    `json:"min_evidence,omitempty"`
	MinLabels             int                                    `json:"min_labels,omitempty"`
	AllowEmptyEvidence    bool                                   `json:"allow_empty_evidence,omitempty"`
}

type fixtureDiagnosticEvidenceExpectation struct {
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Trust    string   `json:"trust,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Contains []string `json:"contains,omitempty"`
}

type fixtureDiagnosticLabelExpectation struct {
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	Contains []string `json:"contains,omitempty"`
}

type fixturePlacement struct {
	RequireComplete         bool `json:"require_complete,omitempty"`
	MinStack                int  `json:"min_stack,omitempty"`
	MinOwnedHeap            int  `json:"min_owned_heap,omitempty"`
	MinSharedHeap           int  `json:"min_shared_heap,omitempty"`
	MaxStack                *int `json:"max_stack,omitempty"`
	MaxOwnedHeap            *int `json:"max_owned_heap,omitempty"`
	MaxSharedHeap           *int `json:"max_shared_heap,omitempty"`
	MinStackDepth           int  `json:"min_stack_depth,omitempty"`
	MinOwnedHeapDepth       int  `json:"min_owned_heap_depth,omitempty"`
	MinSharedDepth          int  `json:"min_shared_depth,omitempty"`
	MinOwnerIdentity        int  `json:"min_owner_identity,omitempty"`
	MinSealBeforeShare      int  `json:"min_seal_before_share,omitempty"`
	MinAllocationSites      int  `json:"min_allocation_sites,omitempty"`
	MinDecomposable         int  `json:"min_decomposable,omitempty"`
	MinFrameLocal           int  `json:"min_frame_local,omitempty"`
	MaxNoFact               *int `json:"max_no_fact,omitempty"`
	MaxUnknown              *int `json:"max_unknown,omitempty"`
	MaxDecomposable         *int `json:"max_decomposable,omitempty"`
	MaxFrameLocal           *int `json:"max_frame_local,omitempty"`
	MinDiesBeforeSuspension int  `json:"min_dies_before_suspension,omitempty"`
	MaxDiesBeforeSuspension *int `json:"max_dies_before_suspension,omitempty"`
	MinHoistableLoads       int  `json:"min_hoistable_loads,omitempty"`
	MaxHoistableLoads       *int `json:"max_hoistable_loads,omitempty"`

	MinStackKind      map[string]int `json:"min_stack_kind,omitempty"`
	MinOwnedHeapKind  map[string]int `json:"min_owned_heap_kind,omitempty"`
	MinSharedHeapKind map[string]int `json:"min_shared_heap_kind,omitempty"`
	MaxStackKind      map[string]int `json:"max_stack_kind,omitempty"`
	MaxOwnedHeapKind  map[string]int `json:"max_owned_heap_kind,omitempty"`
	MaxSharedHeapKind map[string]int `json:"max_shared_heap_kind,omitempty"`
}

type fixtureRun struct {
	Golden        string `json:"golden,omitempty"`
	Error         bool   `json:"error,omitempty"`
	ErrorContains string `json:"error_contains,omitempty"`
	Skip          string `json:"skip,omitempty"`
}

type fixtureBench struct {
	Skip string `json:"skip,omitempty"`
}

type namedSuite struct {
	Name  string // path-based name for t.Run (e.g. "narrowing/typeof-guard")
	Dir   string // absolute directory path
	Suite fixtureSuite
}

type inlineExpectation struct {
	File     string
	Line     int
	Severity string // "error" or "warning"
	Contains string
}

var expectRe = regexp.MustCompile(`--\s*expect-(error|warning)(?::\s*(.+?))?\s*$`)

// discoverFixtures recursively walks root and finds directories containing .lua files.
func discoverFixtures(root string) ([]namedSuite, error) {
	var suites []namedSuite
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		luaFiles, _ := filepath.Glob(filepath.Join(path, "*.lua"))
		if len(luaFiles) == 0 {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		name := filepath.ToSlash(rel)

		s := fixtureSuite{}
		manifestPath := filepath.Join(path, "manifest.json")
		if data, err := os.ReadFile(manifestPath); err == nil {
			if err := json.Unmarshal(data, &s); err != nil {
				return fmt.Errorf("bad manifest in %s: %w", name, err)
			}
		}

		suites = append(suites, namedSuite{Name: name, Dir: path, Suite: s})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(suites, func(i, j int) bool { return suites[i].Name < suites[j].Name })
	return suites, nil
}

// resolveFiles returns the ordered file list for the suite.
func resolveFiles(s namedSuite) []string {
	if len(s.Suite.Files) > 0 {
		return s.Suite.Files
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return []string{"main.lua"}
	}
	var modules []string
	hasMain := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		if e.Name() == "main.lua" {
			hasMain = true
			continue
		}
		modules = append(modules, e.Name())
	}
	sort.Strings(modules)
	if hasMain {
		return append(modules, "main.lua")
	}
	if len(modules) > 0 {
		return modules
	}
	return []string{"main.lua"}
}

func resolveStdlib(s namedSuite) bool {
	if s.Suite.Stdlib != nil {
		return *s.Suite.Stdlib
	}
	return true
}

func readFixtureFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		panic(fmt.Sprintf("fixture file %s/%s: %v", dir, name, err))
	}
	return string(data)
}

func readFixtureSources(s namedSuite) map[string]string {
	files := resolveFiles(s)
	sources := make(map[string]string, len(files))
	for _, file := range files {
		sources[file] = readFixtureFile(s.Dir, file)
	}
	return sources
}

// parseExpectations scans source lines for expect-error/expect-warning comments.
func parseExpectations(filename, source string) []inlineExpectation {
	var expectations []inlineExpectation
	for i, line := range strings.Split(source, "\n") {
		m := expectRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		expectations = append(expectations, inlineExpectation{
			File:     filename,
			Line:     i + 1,
			Severity: m[1],
			Contains: strings.TrimSpace(m[2]),
		})
	}
	return expectations
}

// runCheckPhase type-checks the fixture and verifies diagnostics.
func runCheckPhase(t *testing.T, s namedSuite) {
	runCheckPhaseContext(t, s, nil)
}

// runCheckPhaseContext is runCheckPhase with an optional cooperative solve
// context supplied by the deadline harness.
func runCheckPhaseContext(t *testing.T, s namedSuite, ctx context.Context) {
	t.Helper()
	if s.Suite.Check != nil && s.Suite.Check.Skip != "" {
		t.Skip(s.Suite.Check.Skip)
	}

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
			t.Fatalf("unknown system package: %s", pkg)
		}
	}
	ruleOpts, err := fixtureDiagnosticRuleOptions(s.Suite.Check)
	if err != nil {
		t.Fatalf("diagnostic_rules: %v", err)
	}
	baseOpts = append(baseOpts, ruleOpts...)

	// Collect all sources and their expectations
	sources := make(map[string]string)
	var allExpectations []inlineExpectation
	for _, f := range files {
		src := readFixtureFile(s.Dir, f)
		sources[f] = src
		allExpectations = append(allExpectations, parseExpectations(f, src)...)
	}

	// Check and export dependency modules (all except entry), preserving file order
	type namedModule struct {
		name string
		mod  *testutil.ModuleResult
	}
	var moduleOrder []namedModule
	var allDiagnostics []diag.Diagnostic
	var placementPlans []placementplan.Plan
	for _, f := range files[:len(files)-1] {
		modOpts := append([]testutil.Option{}, baseOpts...)
		for _, nm := range moduleOrder {
			modOpts = append(modOpts, testutil.WithModule(nm.name, nm.mod))
		}
		stats := fixtureStats()
		if stats != nil {
			modOpts = append(modOpts, testutil.WithStats(stats))
		}
		name := strings.TrimSuffix(f, ".lua")
		mod := testutil.CheckFileAndExport(sources[f], name, f, modOpts...)
		logFixtureStats(t, s.Name, f, stats)
		moduleOrder = append(moduleOrder, namedModule{name, mod})
		allDiagnostics = append(allDiagnostics, mod.Errors...)
		placementPlans = append(placementPlans, mod.Placement)
	}

	// Check entry point
	entryOpts := append([]testutil.Option{}, baseOpts...)
	for _, nm := range moduleOrder {
		entryOpts = append(entryOpts, testutil.WithModule(nm.name, nm.mod))
	}
	stats := fixtureStats()
	if stats != nil {
		entryOpts = append(entryOpts, testutil.WithStats(stats))
	}
	entryFile := files[len(files)-1]
	result := testutil.CheckFile(sources[entryFile], entryFile, entryOpts...)
	logFixtureStats(t, s.Name, entryFile, stats)
	allDiagnostics = append(allDiagnostics, result.Diagnostics...)
	placementPlans = append(placementPlans, result.PlacementPlan())
	renderOptions := fixtureDiagnosticRenderOptions(sources, entryFile, fixtureDiagnosticRenderConfigForCheck(s.Suite.Check))
	verifyDiagnosticRenderPolicy(t, allDiagnostics, renderOptions)

	// Verify expectations
	if len(allExpectations) > 0 {
		verifyInlineExpectations(t, allExpectations, allDiagnostics, entryFile, renderOptions)
		if s.Suite.Check != nil && len(s.Suite.Check.Diagnostics) > 0 {
			verifyDiagnosticExpectations(t, s.Suite.Check.Diagnostics, allDiagnostics, entryFile, true, renderOptions)
		} else if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
			verifyErrorCount(t, *s.Suite.Check.Errors, allDiagnostics, renderOptions)
		}
	} else if s.Suite.Check != nil && len(s.Suite.Check.Diagnostics) > 0 {
		verifyDiagnosticExpectations(t, s.Suite.Check.Diagnostics, allDiagnostics, entryFile, true, renderOptions)
	} else if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
		verifyErrorCount(t, *s.Suite.Check.Errors, allDiagnostics, renderOptions)
	} else {
		verifyClean(t, allDiagnostics, renderOptions)
	}
	if s.Suite.Check != nil && s.Suite.Check.Placement != nil {
		verifyPlacementExpectations(t, *s.Suite.Check.Placement, placementplan.Merge(placementPlans...))
	}
}

func fixtureStats() *program.Stats {
	if os.Getenv("FIXTURE_STATS") == "" {
		return nil
	}
	return &program.Stats{}
}

func logFixtureStats(t testing.TB, suite, file string, stats *program.Stats) {
	t.Helper()
	if stats == nil {
		return
	}
	t.Logf("fixture stats %s/%s: solves prepass=%d summary=%d materialize=%d max_funcs=%d max_contexts=%d max_context_refs=%d materialized_context_solves=%d materialized_context_new=%d query_bodies=%d query_transfers=%d body_solves=%d body_transfers=%d",
		suite,
		file,
		stats.PrepassBodySolves,
		stats.SummaryBodySolves,
		stats.MaterializeBodySolves,
		stats.MaxFunctionCount,
		stats.MaxContextCount,
		stats.MaxCallContextRefCount,
		stats.MaterializedContextSolves,
		stats.MaterializedContextNewContexts,
		stats.Query.BodyInvocations,
		stats.Query.Solver.TransferCalls,
		stats.Body.BodySolves,
		stats.Body.Transfer.Solver.TransferCalls,
	)
}

func verifyPlacementExpectations(t testing.TB, expect fixturePlacement, plan placementplan.Plan) {
	t.Helper()
	if expect.RequireComplete && plan.Incomplete {
		t.Fatalf("placement plan incomplete: blockers=%v entries=%s", plan.Blockers, formatPlacementEntries(plan))
	}
	counts := placementCounts(plan)
	assertMinPlacementCount(t, "stack", counts.stack, expect.MinStack, plan)
	assertMinPlacementCount(t, "owned-heap", counts.ownedHeap, expect.MinOwnedHeap, plan)
	assertMinPlacementCount(t, "shared-heap", counts.sharedHeap, expect.MinSharedHeap, plan)
	assertMaxPlacementCount(t, "stack", counts.stack, expect.MaxStack, plan)
	assertMaxPlacementCount(t, "owned-heap", counts.ownedHeap, expect.MaxOwnedHeap, plan)
	assertMaxPlacementCount(t, "shared-heap", counts.sharedHeap, expect.MaxSharedHeap, plan)
	assertMinPlacementCount(t, "stack depth", plan.MaxTargetDepth(placementplan.TargetStack), expect.MinStackDepth, plan)
	assertMinPlacementCount(t, "owned-heap depth", plan.MaxTargetDepth(placementplan.TargetOwnedHeap), expect.MinOwnedHeapDepth, plan)
	assertMinPlacementCount(t, "shared-heap depth", plan.MaxTargetDepth(placementplan.TargetSharedHeap), expect.MinSharedDepth, plan)
	assertMinPlacementCount(t, "owner-identity obligations", counts.ownerIdentity, expect.MinOwnerIdentity, plan)
	assertMinPlacementCount(t, "seal-before-share obligations", counts.sealBeforeShare, expect.MinSealBeforeShare, plan)
	assertMinPlacementCount(t, "allocation sites", counts.allocationSites, expect.MinAllocationSites, plan)
	assertMinPlacementCount(t, "decomposable", counts.decomposable, expect.MinDecomposable, plan)
	assertMinPlacementCount(t, "frame-local", counts.frameLocal, expect.MinFrameLocal, plan)
	assertMinPlacementCount(t, "dies-before-suspension", counts.diesBeforeSuspension, expect.MinDiesBeforeSuspension, plan)
	assertMaxPlacementCount(t, "dies-before-suspension", counts.diesBeforeSuspension, expect.MaxDiesBeforeSuspension, plan)
	assertMinPlacementCount(t, "hoistable loads", counts.hoistableLoads, expect.MinHoistableLoads, plan)
	assertMaxPlacementCount(t, "hoistable loads", counts.hoistableLoads, expect.MaxHoistableLoads, plan)
	assertMinPlacementKindCounts(t, "stack", placementKindCounts(plan, placementplan.TargetStack), expect.MinStackKind, plan)
	assertMinPlacementKindCounts(t, "owned-heap", placementKindCounts(plan, placementplan.TargetOwnedHeap), expect.MinOwnedHeapKind, plan)
	assertMinPlacementKindCounts(t, "shared-heap", placementKindCounts(plan, placementplan.TargetSharedHeap), expect.MinSharedHeapKind, plan)
	assertMaxPlacementKindCounts(t, "stack", placementKindCounts(plan, placementplan.TargetStack), expect.MaxStackKind, plan)
	assertMaxPlacementKindCounts(t, "owned-heap", placementKindCounts(plan, placementplan.TargetOwnedHeap), expect.MaxOwnedHeapKind, plan)
	assertMaxPlacementKindCounts(t, "shared-heap", placementKindCounts(plan, placementplan.TargetSharedHeap), expect.MaxSharedHeapKind, plan)
	assertMaxPlacementCount(t, "no-fact", counts.noFact, expect.MaxNoFact, plan)
	assertMaxPlacementCount(t, "unknown", counts.unknown, expect.MaxUnknown, plan)
	assertMaxPlacementCount(t, "decomposable", counts.decomposable, expect.MaxDecomposable, plan)
	assertMaxPlacementCount(t, "frame-local", counts.frameLocal, expect.MaxFrameLocal, plan)
}

type fixturePlacementCounts struct {
	stack                int
	ownedHeap            int
	sharedHeap           int
	noFact               int
	unknown              int
	ownerIdentity        int
	sealBeforeShare      int
	allocationSites      int
	decomposable         int
	frameLocal           int
	diesBeforeSuspension int
	hoistableLoads       int
}

func placementCounts(plan placementplan.Plan) fixturePlacementCounts {
	counts := fixturePlacementCounts{hoistableLoads: len(plan.HoistableLoads)}
	for _, entry := range plan.Entries {
		switch entry.Target {
		case placementplan.TargetFrameLocal, placementplan.TargetStack:
			counts.stack++
		case placementplan.TargetOwnedHeap:
			counts.ownedHeap++
		case placementplan.TargetSharedHeap:
			counts.sharedHeap++
		case placementplan.TargetNoFact:
			counts.noFact++
		case placementplan.TargetUnknown:
			counts.unknown++
		}
		if entry.AllocationSite {
			counts.allocationSites++
		}
		if entry.Decomposable {
			counts.decomposable++
		}
		if entry.FrameLocal {
			counts.frameLocal++
		}
		for _, obligation := range entry.Obligations {
			switch obligation {
			case placementplan.ObligationOwnerIdentity:
				counts.ownerIdentity++
			case placementplan.ObligationSealBeforeShare:
				counts.sealBeforeShare++
			}
		}
		if entry.HasDiesBeforeSuspension && entry.DiesBeforeSuspension {
			counts.diesBeforeSuspension++
		}
	}
	return counts
}

func placementKindCounts(plan placementplan.Plan, target placementplan.Target) map[string]int {
	counts := make(map[string]int)
	for _, entry := range plan.Entries {
		if entry.Target == target || (target == placementplan.TargetStack && entry.Target == placementplan.TargetFrameLocal) {
			counts[entry.ID.Kind]++
		}
	}
	return counts
}

func assertMinPlacementCount(t testing.TB, label string, got, want int, plan placementplan.Plan) {
	t.Helper()
	if got < want {
		t.Fatalf("placement %s count = %d, want >= %d; entries=%s", label, got, want, formatPlacementEntries(plan))
	}
}

func assertMaxPlacementCount(t testing.TB, label string, got int, want *int, plan placementplan.Plan) {
	t.Helper()
	if want != nil && got > *want {
		t.Fatalf("placement %s count = %d, want <= %d; entries=%s", label, got, *want, formatPlacementEntries(plan))
	}
}

func assertMinPlacementKindCounts(t testing.TB, label string, got map[string]int, want map[string]int, plan placementplan.Plan) {
	t.Helper()
	for kind, min := range want {
		if got[kind] < min {
			t.Fatalf("placement %s %q count = %d, want >= %d; all %s kinds=%v; entries=%s", label, kind, got[kind], min, label, got, formatPlacementEntries(plan))
		}
	}
}

func assertMaxPlacementKindCounts(t testing.TB, label string, got map[string]int, want map[string]int, plan placementplan.Plan) {
	t.Helper()
	for kind, max := range want {
		if got[kind] > max {
			t.Fatalf("placement %s %q count = %d, want <= %d; all %s kinds=%v; entries=%s", label, kind, got[kind], max, label, got, formatPlacementEntries(plan))
		}
	}
}

func formatPlacementEntries(plan placementplan.Plan) string {
	var parts []string
	for _, entry := range plan.Entries {
		lifetime := "unset"
		if entry.HasDiesBeforeSuspension {
			lifetime = fmt.Sprintf("%t", entry.DiesBeforeSuspension)
		}
		parts = append(parts, fmt.Sprintf("%s:%s alloc=%t decomposable=%t frame_local=%t frame_local_use_proof=%t dies_before_suspension=%s reasons=%v obligations=%v blockers=%v", entry.ID, entry.Target, entry.AllocationSite, entry.Decomposable, entry.FrameLocal, entry.FrameLocalUseProof, lifetime, entry.Reasons, entry.Obligations, entry.Blockers))
	}
	for _, load := range plan.HoistableLoads {
		parts = append(parts, fmt.Sprintf("hoistable-load:body=%d point=%d path=%s loop_head=%d loop_span=%d:%d-%d:%d", load.BodyID, load.Point, load.ReadPath, load.LoopHead, load.LoopSpan.StartLine, load.LoopSpan.StartCol, load.LoopSpan.EndLine, load.LoopSpan.EndCol))
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func fixtureDiagnosticRenderConfigForCheck(check *fixtureCheck) fixtureDiagnosticRenderConfig {
	if check == nil {
		return fixtureDiagnosticRenderConfig{}
	}
	return check.RenderOptions
}

func fixtureDiagnosticRenderOptions(sources map[string]string, entryFile string, configs ...fixtureDiagnosticRenderConfig) diag.RenderOptions {
	opts := diag.RenderOptions{}
	if len(configs) > 0 {
		opts.WitnessTrace = configs[0].WitnessTrace
	}
	if len(sources) == 0 {
		return opts
	}
	renderSources := make(diag.SourceMap, len(sources)*2+1)
	displayFiles := make(map[string]string, len(sources)+1)
	for file, source := range sources {
		renderSources[file] = source
		if moduleName := strings.TrimSuffix(file, ".lua"); moduleName != file {
			renderSources[moduleName] = source
			displayFiles[moduleName] = file
		}
	}
	if source, ok := sources[entryFile]; ok {
		renderSources["test.lua"] = source
		displayFiles["test.lua"] = entryFile
	}
	opts.Sources = renderSources
	opts.DisplayFiles = displayFiles
	opts.ShowSourceLabelRows = true
	return opts
}

func verifyInlineExpectations(t testing.TB, expectations []inlineExpectation, diagnostics []diag.Diagnostic, entryFile string, renderOptions diag.RenderOptions) {
	t.Helper()
	matched := make([]bool, len(diagnostics))
	failed := false

	for _, exp := range expectations {
		found := false
		// An expect-error annotation absorbs ALL matching diagnostics on that line
		for i, d := range diagnostics {
			if !matchesExpectation(exp, d, entryFile) {
				continue
			}
			found = true
			matched[i] = true
		}
		if !found {
			failed = true
			if exp.Contains != "" {
				t.Errorf("expected %s at %s:%d not found: %q", exp.Severity, exp.File, exp.Line, exp.Contains)
			} else {
				t.Errorf("expected %s at %s:%d not found", exp.Severity, exp.File, exp.Line)
			}
		}
	}

	for i, d := range diagnostics {
		if matched[i] || d.Severity == diag.SeverityHint {
			continue
		}
		failed = true
		t.Errorf("unexpected %s at %s:%d: %s (%s)",
			d.Severity, d.Position.File, d.Position.Line, d.Message, d.Code.String())
	}

	if failed {
		dumpDiagnostics(t, diagnostics, renderOptions)
	}
}

func matchesExpectation(exp inlineExpectation, d diag.Diagnostic, entryFile string) bool {
	expFile := exp.File
	// Match diagnostic file: d.Position.File is set by the checker (e.g. "test.lua" or module name)
	if !matchesDiagnosticFile(expFile, d, entryFile) {
		return false
	}
	if d.Position.Line != exp.Line {
		return false
	}
	wantSeverity := diag.SeverityError
	if exp.Severity == "warning" {
		wantSeverity = diag.SeverityWarning
	}
	if d.Severity != wantSeverity {
		return false
	}
	if exp.Contains != "" && !strings.Contains(d.Message, exp.Contains) {
		return false
	}
	return true
}

func matchesDiagnosticFile(expFile string, d diag.Diagnostic, entryFile string) bool {
	if expFile == "" {
		return true
	}
	actual := d.Position.File
	if actual == expFile {
		return true
	}
	expModule := strings.TrimSuffix(expFile, ".lua")
	if actual == expModule {
		return true
	}
	if actual == "test.lua" && (expFile == entryFile || expModule == strings.TrimSuffix(entryFile, ".lua")) {
		return true
	}
	return false
}

func verifyDiagnosticExpectations(t testing.TB, expectations []fixtureDiagnosticExpectation, diagnostics []diag.Diagnostic, entryFile string, requireNoUnexpected bool, renderOptions diag.RenderOptions) {
	t.Helper()
	missing, unexpected := matchDiagnosticExpectations(expectations, diagnostics, entryFile, requireNoUnexpected, renderOptions)
	for _, msg := range missing {
		t.Errorf("missing diagnostic expectation: %s", msg)
	}
	for _, msg := range unexpected {
		t.Errorf("unexpected diagnostic: %s", msg)
	}
	if len(missing) > 0 || len(unexpected) > 0 {
		dumpDiagnostics(t, diagnostics, renderOptions)
	}
}

func matchDiagnosticExpectations(expectations []fixtureDiagnosticExpectation, diagnostics []diag.Diagnostic, entryFile string, requireNoUnexpected bool, renderOptions diag.RenderOptions) (missing, unexpected []string) {
	matched := make([]bool, len(diagnostics))
	requireUnexpectedHints := diagnosticExpectationsIncludeSeverity(expectations, diag.SeverityHint)
	for _, exp := range expectations {
		if err := validateDiagnosticExpectation(exp); err != nil {
			missing = append(missing, fmt.Sprintf("invalid diagnostic expectation: %s (%s)", err, describeDiagnosticExpectation(exp)))
			continue
		}
		found := false
		for i, d := range diagnostics {
			if matched[i] {
				continue
			}
			if !matchesDiagnosticExpectation(exp, d, entryFile, renderOptions) {
				continue
			}
			found = true
			matched[i] = true
			break
		}
		if !found {
			missing = append(missing, describeDiagnosticExpectation(exp))
		}
	}
	if requireNoUnexpected {
		for i, d := range diagnostics {
			if matched[i] || (d.Severity == diag.SeverityHint && !requireUnexpectedHints) {
				continue
			}
			unexpected = append(unexpected, diagSummary(d))
		}
	}
	return missing, unexpected
}

func diagnosticExpectationsIncludeSeverity(expectations []fixtureDiagnosticExpectation, severity diag.Severity) bool {
	for _, exp := range expectations {
		got, ok := diagnosticSeverity(exp.Severity)
		if ok && got == severity {
			return true
		}
	}
	return false
}

func fixtureDiagnosticRuleOptions(check *fixtureCheck) ([]testutil.Option, error) {
	if check == nil || len(check.DiagnosticRules) == 0 {
		return nil, nil
	}
	opts := make([]testutil.Option, 0, len(check.DiagnosticRules))
	for i, ruleSpec := range check.DiagnosticRules {
		code := strings.TrimSpace(ruleSpec.Code)
		if code == "" {
			return nil, fmt.Errorf("rule %d code is required", i+1)
		}
		if ruleSpec.Enabled == nil && strings.TrimSpace(ruleSpec.Severity) == "" {
			return nil, fmt.Errorf("rule %d must set enabled or severity", i+1)
		}
		var rule diag.Rule
		if ruleSpec.Enabled != nil {
			if *ruleSpec.Enabled {
				rule = diag.Enable()
			} else {
				rule = diag.Disable()
			}
		}
		if ruleSpec.Severity != "" {
			severity, ok := diagnosticSeverity(ruleSpec.Severity)
			if !ok {
				return nil, fmt.Errorf("rule %d has unknown severity %q", i+1, ruleSpec.Severity)
			}
			rule = rule.WithSeverity(severity)
		}
		opts = append(opts, testutil.WithDiagnosticRule(diag.Code(code), rule))
	}
	return opts, nil
}

func validateDiagnosticExpectation(exp fixtureDiagnosticExpectation) error {
	if strings.TrimSpace(exp.File) == "" {
		return fmt.Errorf("file is required")
	}
	if exp.Line <= 0 {
		return fmt.Errorf("line must be positive")
	}
	if exp.Column < 0 {
		return fmt.Errorf("column must be non-negative")
	}
	if exp.Severity == "" {
		return fmt.Errorf("severity is required")
	}
	if _, ok := diagnosticSeverity(exp.Severity); !ok {
		return fmt.Errorf("unknown severity %q", exp.Severity)
	}
	if strings.TrimSpace(exp.Code) == "" {
		return fmt.Errorf("code is required")
	}
	if err := validateContainsList("message_contains", exp.MessageContains, true); err != nil {
		return err
	}
	if err := validateContainsList("evidence_contains", exp.EvidenceContains, !exp.AllowEmptyEvidence && len(exp.Evidence) == 0); err != nil {
		return err
	}
	for i, evidence := range exp.Evidence {
		if err := validateDiagnosticEvidenceExpectation(evidence); err != nil {
			return fmt.Errorf("evidence[%d]: %w", i, err)
		}
	}
	if err := validateContainsList("render_contains", exp.RenderContains, true); err != nil {
		return err
	}
	if err := validateContainsList("render_ordered_contains", exp.RenderOrderedContains, false); err != nil {
		return err
	}
	if err := validateContainsList("render_not_contains", exp.RenderNotContains, false); err != nil {
		return err
	}
	if err := validateContainsList("help_contains", exp.HelpContains, true); err != nil {
		return err
	}
	if err := validateContainsList("label_contains", exp.LabelContains, len(exp.Labels) == 0); err != nil {
		return err
	}
	for i, label := range exp.Labels {
		if err := validateDiagnosticLabelExpectation(label); err != nil {
			return fmt.Errorf("labels[%d]: %w", i, err)
		}
	}
	if exp.MinEvidence < 0 {
		return fmt.Errorf("min_evidence must be non-negative")
	}
	if exp.MinLabels < 0 {
		return fmt.Errorf("min_labels must be non-negative")
	}
	if !exp.AllowEmptyEvidence && exp.MinEvidence <= 0 && len(exp.Evidence) == 0 {
		return fmt.Errorf("min_evidence must be positive unless allow_empty_evidence is true")
	}
	return nil
}

func validateDiagnosticEvidenceExpectation(exp fixtureDiagnosticEvidenceExpectation) error {
	if err := validateContainsList("contains", exp.Contains, true); err != nil {
		return err
	}
	if exp.Line < 0 {
		return fmt.Errorf("line must be non-negative")
	}
	if exp.Column < 0 {
		return fmt.Errorf("column must be non-negative")
	}
	if exp.Kind != "" && !validDiagnosticEvidenceKind(exp.Kind) {
		return fmt.Errorf("unknown kind %q", exp.Kind)
	}
	if exp.Trust != "" && !validDiagnosticEvidenceTrust(exp.Trust) {
		return fmt.Errorf("unknown trust %q", exp.Trust)
	}
	if exp.Reason != "" && !validDiagnosticEvidenceReason(exp.Reason) {
		return fmt.Errorf("unknown reason %q", exp.Reason)
	}
	return nil
}

func validateDiagnosticLabelExpectation(exp fixtureDiagnosticLabelExpectation) error {
	if err := validateContainsList("contains", exp.Contains, true); err != nil {
		return err
	}
	if exp.Line < 0 {
		return fmt.Errorf("line must be non-negative")
	}
	if exp.Column < 0 {
		return fmt.Errorf("column must be non-negative")
	}
	return nil
}

func validateContainsList(name string, values []string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("%s must contain at least one assertion", name)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty assertion", name)
		}
	}
	return nil
}

func matchesDiagnosticExpectation(exp fixtureDiagnosticExpectation, d diag.Diagnostic, entryFile string, renderOptions diag.RenderOptions) bool {
	if !matchesDiagnosticFile(exp.File, d, entryFile) {
		return false
	}
	if exp.Line != 0 && d.Position.Line != exp.Line {
		return false
	}
	if exp.Column != 0 && d.Position.Column != exp.Column {
		return false
	}
	if exp.Severity != "" {
		severity, ok := diagnosticSeverity(exp.Severity)
		if !ok || d.Severity != severity {
			return false
		}
	}
	if exp.Code != "" && d.Code.String() != exp.Code {
		return false
	}
	if !containsAll(d.Message, exp.MessageContains) {
		return false
	}
	evidence := d.Explanation.Evidence()
	if exp.MinEvidence > 0 && len(evidence) < exp.MinEvidence {
		return false
	}
	if !exp.AllowEmptyEvidence && (exp.MinEvidence > 0 || len(exp.EvidenceContains) > 0) && len(evidence) == 0 {
		return false
	}
	if !containsAll(d.Explanation.String(), exp.EvidenceContains) {
		return false
	}
	if !matchesDiagnosticEvidenceExpectations(exp.Evidence, d, entryFile) {
		return false
	}
	if len(exp.RenderContains) > 0 || len(exp.RenderOrderedContains) > 0 || len(exp.RenderNotContains) > 0 {
		rendered := diag.Render(d, renderOptions)
		if !containsAll(rendered, exp.RenderContains) {
			return false
		}
		if !containsInOrder(rendered, exp.RenderOrderedContains) {
			return false
		}
		if containsAny(rendered, exp.RenderNotContains) {
			return false
		}
	}
	if !containsAll(d.Help, exp.HelpContains) {
		return false
	}
	if exp.MinLabels > 0 && len(d.Labels) < exp.MinLabels {
		return false
	}
	if !containsAll(formatDiagnosticLabels(d.Labels), exp.LabelContains) {
		return false
	}
	if !matchesDiagnosticLabelExpectations(exp.Labels, d, entryFile) {
		return false
	}
	return true
}

func matchesDiagnosticEvidenceExpectations(expectations []fixtureDiagnosticEvidenceExpectation, d diag.Diagnostic, entryFile string) bool {
	evidence := d.Explanation.Evidence()
	offset := 0
	for _, exp := range expectations {
		matched := false
		for offset < len(evidence) {
			item := evidence[offset]
			offset++
			if matchesDiagnosticEvidenceExpectation(exp, d, entryFile, item) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func matchesDiagnosticEvidenceExpectation(exp fixtureDiagnosticEvidenceExpectation, d diag.Diagnostic, entryFile string, item diag.Evidence) bool {
	if !matchesDiagnosticEvidenceFile(exp.File, d, entryFile, item) {
		return false
	}
	if exp.Line != 0 && item.Span.StartLine != exp.Line {
		return false
	}
	if exp.Column != 0 && item.Span.StartCol != exp.Column {
		return false
	}
	if exp.Kind != "" && item.Kind.String() != exp.Kind {
		return false
	}
	if exp.Trust != "" && item.Trust.String() != exp.Trust {
		return false
	}
	if exp.Reason != "" && item.Reason.String() != exp.Reason {
		return false
	}
	return containsAll(formatDiagnosticEvidenceItem(item), exp.Contains)
}

func matchesDiagnosticEvidenceFile(expFile string, d diag.Diagnostic, entryFile string, item diag.Evidence) bool {
	if expFile == "" {
		return true
	}
	actual := item.File
	if actual == "" {
		actual = d.Position.File
	}
	copy := d
	copy.Position.File = actual
	return matchesDiagnosticFile(expFile, copy, entryFile)
}

func matchesDiagnosticLabelExpectations(expectations []fixtureDiagnosticLabelExpectation, d diag.Diagnostic, entryFile string) bool {
	for _, exp := range expectations {
		matched := false
		for _, label := range d.Labels {
			if matchesDiagnosticLabelExpectation(exp, d, entryFile, label) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func matchesDiagnosticLabelExpectation(exp fixtureDiagnosticLabelExpectation, d diag.Diagnostic, entryFile string, label diag.Label) bool {
	if !matchesDiagnosticLabelFile(exp.File, d, entryFile, label) {
		return false
	}
	if exp.Line != 0 && label.Span.StartLine != exp.Line {
		return false
	}
	if exp.Column != 0 && label.Span.StartCol != exp.Column {
		return false
	}
	return containsAll(label.Message, exp.Contains)
}

func matchesDiagnosticLabelFile(expFile string, d diag.Diagnostic, entryFile string, label diag.Label) bool {
	if expFile == "" {
		return true
	}
	actual := label.DisplayFile()
	if actual == "" {
		actual = d.Position.File
	}
	copy := d
	copy.Position.File = actual
	return matchesDiagnosticFile(expFile, copy, entryFile)
}

func diagnosticSeverity(s string) (diag.Severity, bool) {
	switch s {
	case "error":
		return diag.SeverityError, true
	case "warning":
		return diag.SeverityWarning, true
	case "hint":
		return diag.SeverityHint, true
	default:
		return diag.SeverityError, false
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if needle == "" || !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func containsInOrder(haystack string, needles []string) bool {
	offset := 0
	for _, needle := range needles {
		if needle == "" {
			return false
		}
		index := strings.Index(haystack[offset:], needle)
		if index < 0 {
			return false
		}
		offset += index + len(needle)
	}
	return true
}

func containsAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func formatDiagnosticLabels(labels []diag.Label) string {
	var parts []string
	for _, label := range labels {
		parts = append(parts, label.Message)
	}
	return strings.Join(parts, "\n")
}

func formatDiagnosticEvidenceItem(item diag.Evidence) string {
	return diag.NewExplanation(item).String()
}

func validDiagnosticEvidenceKind(kind string) bool {
	switch kind {
	case diag.EvidenceAbstractFact.String(),
		diag.EvidenceUserAssertion.String(),
		diag.EvidenceMissingProof.String(),
		diag.EvidencePrecisionBoundary.String():
		return true
	default:
		return false
	}
}

func validDiagnosticEvidenceTrust(trust string) bool {
	switch trust {
	case diag.TrustProven.String(),
		diag.TrustClaimed.String(),
		diag.TrustRefuted.String(),
		diag.TrustUnknown.String():
		return true
	default:
		return false
	}
}

func validDiagnosticEvidenceReason(reason string) bool {
	switch reason {
	case diag.EvidenceReasonUnspecified.String(),
		diag.EvidenceReasonBoundaryValidationMissing.String(),
		diag.EvidenceReasonIndexReadValidationMissing.String(),
		diag.EvidenceReasonExplicitBoundaryValidation.String():
		return true
	default:
		return false
	}
}

func describeDiagnosticExpectation(exp fixtureDiagnosticExpectation) string {
	var parts []string
	if exp.File != "" {
		parts = append(parts, "file="+exp.File)
	}
	if exp.Line != 0 {
		parts = append(parts, fmt.Sprintf("line=%d", exp.Line))
	}
	if exp.Column != 0 {
		parts = append(parts, fmt.Sprintf("column=%d", exp.Column))
	}
	if exp.Severity != "" {
		parts = append(parts, "severity="+exp.Severity)
	}
	if exp.Code != "" {
		parts = append(parts, "code="+exp.Code)
	}
	for _, text := range exp.MessageContains {
		parts = append(parts, fmt.Sprintf("message~%q", text))
	}
	for _, text := range exp.EvidenceContains {
		parts = append(parts, fmt.Sprintf("evidence~%q", text))
	}
	for _, evidence := range exp.Evidence {
		parts = append(parts, describeDiagnosticEvidenceExpectation(evidence))
	}
	for _, text := range exp.RenderContains {
		parts = append(parts, fmt.Sprintf("render~%q", text))
	}
	for _, text := range exp.RenderOrderedContains {
		parts = append(parts, fmt.Sprintf("render_ordered~%q", text))
	}
	for _, text := range exp.RenderNotContains {
		parts = append(parts, fmt.Sprintf("render!~%q", text))
	}
	for _, text := range exp.HelpContains {
		parts = append(parts, fmt.Sprintf("help~%q", text))
	}
	for _, text := range exp.LabelContains {
		parts = append(parts, fmt.Sprintf("label~%q", text))
	}
	for _, label := range exp.Labels {
		parts = append(parts, describeDiagnosticLabelExpectation(label))
	}
	if exp.MinEvidence != 0 {
		parts = append(parts, fmt.Sprintf("min_evidence=%d", exp.MinEvidence))
	}
	if exp.MinLabels != 0 {
		parts = append(parts, fmt.Sprintf("min_labels=%d", exp.MinLabels))
	}
	if len(parts) == 0 {
		return "<empty>"
	}
	return strings.Join(parts, ", ")
}

func describeDiagnosticEvidenceExpectation(exp fixtureDiagnosticEvidenceExpectation) string {
	var parts []string
	if exp.File != "" {
		parts = append(parts, "file="+exp.File)
	}
	if exp.Line != 0 {
		parts = append(parts, fmt.Sprintf("line=%d", exp.Line))
	}
	if exp.Column != 0 {
		parts = append(parts, fmt.Sprintf("column=%d", exp.Column))
	}
	if exp.Kind != "" {
		parts = append(parts, "kind="+exp.Kind)
	}
	if exp.Trust != "" {
		parts = append(parts, "trust="+exp.Trust)
	}
	if exp.Reason != "" {
		parts = append(parts, "reason="+exp.Reason)
	}
	for _, text := range exp.Contains {
		parts = append(parts, fmt.Sprintf("contains~%q", text))
	}
	return "evidence{" + strings.Join(parts, ", ") + "}"
}

func describeDiagnosticLabelExpectation(exp fixtureDiagnosticLabelExpectation) string {
	var parts []string
	if exp.File != "" {
		parts = append(parts, "file="+exp.File)
	}
	if exp.Line != 0 {
		parts = append(parts, fmt.Sprintf("line=%d", exp.Line))
	}
	if exp.Column != 0 {
		parts = append(parts, fmt.Sprintf("column=%d", exp.Column))
	}
	for _, text := range exp.Contains {
		parts = append(parts, fmt.Sprintf("contains~%q", text))
	}
	return "label{" + strings.Join(parts, ", ") + "}"
}

func verifyErrorCount(t testing.TB, want int, diagnostics []diag.Diagnostic, renderOptions diag.RenderOptions) {
	t.Helper()
	var errors []diag.Diagnostic
	for _, d := range diagnostics {
		if d.Severity == diag.SeverityError {
			errors = append(errors, d)
		}
	}
	if len(errors) != want {
		t.Errorf("expected %d errors, got %d", want, len(errors))
		dumpDiagnostics(t, diagnostics, renderOptions)
	}
}

func verifyClean(t testing.TB, diagnostics []diag.Diagnostic, renderOptions diag.RenderOptions) {
	t.Helper()
	var errors []diag.Diagnostic
	for _, d := range diagnostics {
		if d.Severity == diag.SeverityError {
			errors = append(errors, d)
		}
	}
	if len(errors) > 0 {
		t.Errorf("expected clean check, got %d errors", len(errors))
		dumpDiagnostics(t, diagnostics, renderOptions)
	}
}

func dumpDiagnostics(t testing.TB, diagnostics []diag.Diagnostic, renderOptions diag.RenderOptions) {
	t.Helper()
	t.Log("--- all diagnostics ---")
	for _, d := range diagnostics {
		t.Log("\n" + diag.Render(d, renderOptions))
	}
}

func verifyDiagnosticRenderPolicy(t testing.TB, diagnostics []diag.Diagnostic, renderOptions diag.RenderOptions) {
	t.Helper()
	for _, d := range diagnostics {
		rendered := diag.Render(d, renderOptions)
		if violations := renderedDiagnosticFramePolicyViolations(rendered); len(violations) > 0 {
			t.Errorf("diagnostic render policy violation for %s at %s:%d: %s\n%s", d.Code, d.Position.File, d.Position.Line, strings.Join(violations, "; "), rendered)
		}
	}
}

func renderedDiagnosticFramePolicyViolations(rendered string) []string {
	var violations []string
	if strings.Contains(rendered, "^~") {
		violations = append(violations, "source frames must use exact carets, not span underlines")
	}

	var frame framePolicyState
	flush := func() {
		if frame.labeledCaretRows > 0 {
			violations = append(violations, "source-frame labels must use directional arrows, not caret-label rows")
		}
		if frame.plainLabelRows > 0 {
			violations = append(violations, "source-frame labels must include a directional arrow")
		}
		if frame.labeledCaretRows > 0 && frame.unlabeledCaret {
			violations = append(violations, "a source frame must not mix unlabeled carets with labeled rows")
		}
		if frame.labelRowsBeforeSource+frame.labelRowsAfterSource > 0 && frame.unlabeledCaret {
			violations = append(violations, "source-frame labels must not render a second caret layer")
		}
		frame = framePolicyState{}
	}

	for _, line := range strings.Split(rendered, "\n") {
		if !strings.Contains(line, "|") {
			flush()
			continue
		}
		pipe := strings.Index(line, "|")
		if pipe < 0 {
			continue
		}
		if sourceFrameLine(line[:pipe]) {
			frame.sourceSeen = true
			continue
		}
		after := strings.TrimSpace(line[pipe+1:])
		if after == "" {
			continue
		}
		if directionalLabelRowHasMessage(after) {
			if frame.sourceSeen {
				frame.labelRowsAfterSource++
			} else {
				frame.labelRowsBeforeSource++
			}
			continue
		}
		if strings.Contains(after, "^") {
			if caretRowHasMessage(after) {
				frame.labeledCaretRows++
			} else {
				frame.unlabeledCaret = true
			}
			continue
		}
		if frame.sourceSeen {
			frame.plainLabelRows++
		} else {
			frame.plainLabelRows++
		}
	}
	flush()
	return violations
}

type framePolicyState struct {
	sourceSeen            bool
	labelRowsBeforeSource int
	labelRowsAfterSource  int
	labeledCaretRows      int
	unlabeledCaret        bool
	plainLabelRows        int
}

func caretRowHasMessage(afterPipe string) bool {
	withoutCarets := strings.ReplaceAll(afterPipe, "^", "")
	return strings.TrimSpace(withoutCarets) != ""
}

func directionalLabelRowHasMessage(afterPipe string) bool {
	withoutArrows := strings.NewReplacer("↑", "", "↓", "").Replace(afterPipe)
	return withoutArrows != afterPipe && strings.TrimSpace(withoutArrows) != ""
}

func sourceFrameLine(prefix string) bool {
	return strings.TrimSpace(prefix) != ""
}

// runExecPhase executes the fixture and verifies output.
func runExecPhase(t *testing.T, s namedSuite) {
	t.Helper()
	if s.Suite.Run == nil {
		// Auto-enable if output.golden exists
		goldenPath := filepath.Join(s.Dir, "output.golden")
		if _, err := os.Stat(goldenPath); err != nil {
			return
		}
	} else if s.Suite.Run.Skip != "" {
		t.Skip(s.Suite.Run.Skip)
	}

	files := resolveFiles(s)

	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenString(L)
	OpenTable(L)
	OpenMath(L)

	// Build module source map and install require
	moduleSources := make(map[string]string)
	for _, f := range files[:len(files)-1] {
		moduleSources[strings.TrimSuffix(f, ".lua")] = readFixtureFile(s.Dir, f)
	}
	installRequire(L, moduleSources)

	// Capture print output
	var buf bytes.Buffer
	capturePrint(L, &buf)

	// Execute entry point
	entrySrc := readFixtureFile(s.Dir, files[len(files)-1])
	err := L.DoString(entrySrc)

	runCfg := s.Suite.Run
	if runCfg != nil && runCfg.Error {
		if err == nil {
			t.Error("expected runtime error, got none")
		} else if runCfg.ErrorContains != "" && !strings.Contains(err.Error(), runCfg.ErrorContains) {
			t.Errorf("error %q does not contain %q", err.Error(), runCfg.ErrorContains)
		}
		return
	}
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}

	verifyGoldenOutput(t, s, &buf)
}

// resolvePackageManifest returns a predefined manifest for a system package name.
func resolvePackageManifest(name string) *typemanifest.Manifest {
	switch name {
	case "channel":
		return testutil.ChannelManifest()
	case "funcs":
		return testutil.FuncsManifest()
	case "process":
		return testutil.ProcessManifest()
	case "resource":
		return fixtureResourceManifest()
	case "ownership":
		return fixtureOwnershipManifest()
	case "time":
		return fixtureTimeManifest()
	case "uuid":
		return fixtureUuidManifest()
	default:
		return nil
	}
}

func fixtureOwnershipManifest() *typemanifest.Manifest {
	m := typemanifest.New("ownership")
	m.DefineFunctionSignature("ownership.store", signature.Function{
		Type: typ.Func().
			Param("value", typ.Any).
			Param("container", typ.Any).
			Build(),
		Effect: effect.Empty.With(ownership.Store{
			Param: effect.ParamRef{Index: 0},
			Into:  effect.ParamRef{Index: 1},
		}),
	})
	m.SetExport(typ.Unknown)
	return m
}

// fixtureResourceManifest is a manifest-only db-like resource surface used by
// the declared lifecycle fixture. It deliberately uses return-slot acquire
// effects so the fixture exercises manifest transport, call binding, aliasing,
// transitions, obligations, and opaque-call escape end to end.
func fixtureResourceManifest() *typemanifest.Manifest {
	m := typemanifest.New("resource")
	for _, def := range []typestate.Definition{
		{
			Protocol:    "connection",
			States:      []typestate.State{"open", "closed"},
			FinalStates: []typestate.State{"closed"},
			Transitions: []typestate.TransitionDecl{{From: "open", To: "open"}, {From: "open", To: "closed"}},
		},
		{
			Protocol:    "transaction",
			States:      []typestate.State{"active", "committed", "rolledback"},
			FinalStates: []typestate.State{"committed", "rolledback"},
			Transitions: []typestate.TransitionDecl{{From: "active", To: "committed"}, {From: "active", To: "rolledback"}},
		},
	} {
		if err := m.DefineTypestateProtocol(def); err != nil {
			panic(err)
		}
	}
	m.DefineFunctionSignature("resource.connect", signature.Function{
		Type: typ.Func().Returns(typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
			Target:     pathdom.Path{Root: "ret[0]"},
			Kind:       signature.LifecycleAcquire,
			Protocol:   "connection",
			To:         "open",
			Obligation: typestate.Obligation{Final: "closed"},
		}}},
	})
	m.DefineFunctionSignature("resource.begin", signature.Function{
		Type: typ.Func().Param("conn", typ.Any).Returns(typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
			Target:     pathdom.Path{Root: "ret[0]"},
			Kind:       signature.LifecycleAcquire,
			Protocol:   "transaction",
			To:         "active",
			Obligation: typestate.Obligation{Finals: typestate.NewFinalStates("committed", "rolledback")},
		}}},
	})
	operations := []struct {
		name     string
		protocol typestate.Protocol
		from     typestate.State
		to       typestate.State
	}{
		{"close", "connection", "open", "closed"},
		{"commit", "transaction", "active", "committed"},
		{"rollback", "transaction", "active", "rolledback"},
	}
	export := typetable.NewRecord().
		Field("connect", typ.Func().Returns(typ.Any).Build()).
		Field("begin", typ.Func().Param("conn", typ.Any).Returns(typ.Any).Build()).
		Field("close", typ.Func().Param("resource", typ.Any).Build()).
		Field("query", typ.Func().Param("resource", typ.Any).Build()).
		Field("commit", typ.Func().Param("resource", typ.Any).Build()).
		Field("rollback", typ.Func().Param("resource", typ.Any).Build()).
		Build()
	m.SetExport(export)
	for _, operation := range operations {
		m.DefineFunctionSignature("resource."+operation.name, signature.Function{
			Type: typ.Func().Param("resource", typ.Any).Build(),
			OperationalEffects: &signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
				Target:   pathdom.NewPlaceholder(0),
				Kind:     signature.LifecycleTransition,
				Protocol: operation.protocol,
				From:     operation.from,
				To:       operation.to,
			}}},
		})
	}
	m.DefineFunctionSignature("resource.query", signature.Function{
		Type: typ.Func().Param("resource", typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{TypestateRequirements: []signature.TypestateRequirement{{
			Target: pathdom.NewPlaceholder(0), Protocol: "connection", State: "open",
		}}},
	})
	return m
}

func fixtureTimeManifest() *typemanifest.Manifest {
	m := typemanifest.New("time")

	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})

	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("t", typ.Self).Returns(durationType).Build()},
		{Name: "add", Type: typ.Func().Param("self", typ.Self).Param("d", durationType).Returns(typ.Self).Build()},
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	m.DefineType("Time", timeType)
	m.DefineType("Duration", durationType)

	moduleType := typetable.NewRecord().
		Field("now", typ.Func().Returns(timeType).Build()).
		Build()
	m.SetExport(moduleType)

	return m
}

func fixtureUuidManifest() *typemanifest.Manifest {
	m := typemanifest.New("uuid")
	m.SetExport(typetable.NewRecord().
		Field("v7", typ.Func().Returns(typ.String).Build()).
		Build())
	return m
}

// installRequire sets up a require() global that loads modules from the given source map.
// Modules are compiled, executed, cached, and returned — matching standard Lua require semantics.
func installRequire(L *LState, sources map[string]string) {
	loaded := L.NewTable()
	L.SetGlobal("require", L.NewFunction(func(L *LState) int {
		name := L.CheckString(1)
		// Return cached module
		if cached := loaded.RawGetString(name); cached != LNil {
			L.Push(cached)
			return 1
		}
		src, ok := sources[name]
		if !ok {
			L.RaiseError("module '%s' not found", name)
			return 0
		}
		fn, err := L.LoadString(src)
		if err != nil {
			L.RaiseError("module '%s': %s", name, err.Error())
			return 0
		}
		L.Push(fn)
		L.Call(0, 1)
		result := L.Get(-1)
		if result == LNil {
			result = LTrue
		}
		loaded.RawSetString(name, result)
		return 1
	}))
}

func capturePrint(L *LState, buf *bytes.Buffer) {
	L.SetGlobal("print", L.NewFunction(func(L *LState) int {
		top := L.GetTop()
		for i := 1; i <= top; i++ {
			if i > 1 {
				buf.WriteByte('\t')
			}
			buf.WriteString(L.ToStringMeta(L.Get(i)).String())
		}
		buf.WriteByte('\n')
		return 0
	}))
}

func verifyGoldenOutput(t *testing.T, s namedSuite, buf *bytes.Buffer) {
	t.Helper()
	goldenName := "output.golden"
	if s.Suite.Run != nil && s.Suite.Run.Golden != "" {
		goldenName = s.Suite.Run.Golden
	}
	goldenPath := filepath.Join(s.Dir, goldenName)

	if os.Getenv("FIXTURE_UPDATE") != "" {
		got := buf.String()
		if got != "" {
			if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
				t.Fatalf("updating golden file: %v", err)
			}
			t.Logf("updated %s", goldenPath)
		}
		return
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) && buf.Len() == 0 {
			return // no golden file and no output, that's fine
		}
		if os.IsNotExist(err) {
			t.Fatalf("output produced but no golden file at %s (run with FIXTURE_UPDATE=1 to create)", goldenPath)
		}
		t.Fatalf("reading golden file: %v", err)
	}

	got := buf.String()
	// Git may check text fixtures out with CRLF on Windows, while capturePrint
	// deliberately emits Lua's canonical newline. Compare the content rather
	// than the checkout's platform-specific line endings.
	want := string(bytes.ReplaceAll(golden, []byte("\r\n"), []byte("\n")))
	if got != want {
		t.Errorf("output mismatch:\n--- want ---\n%s--- got ---\n%s", want, got)
	}
}

// runBenchPhase benchmarks the fixture.
func runBenchPhase(b *testing.B, s namedSuite) {
	b.Helper()
	if s.Suite.Bench == nil {
		b.Skip("no bench config")
	}
	if s.Suite.Bench.Skip != "" {
		b.Skip(s.Suite.Bench.Skip)
	}

	files := resolveFiles(s)

	L := NewState()
	defer L.Close()
	OpenBase(L)
	OpenString(L)
	OpenTable(L)
	OpenMath(L)

	moduleSources := make(map[string]string)
	for _, f := range files[:len(files)-1] {
		moduleSources[strings.TrimSuffix(f, ".lua")] = readFixtureFile(s.Dir, f)
	}
	installRequire(L, moduleSources)

	// Silence print
	L.SetGlobal("print", L.NewFunction(func(L *LState) int { return 0 }))

	entrySrc := readFixtureFile(s.Dir, files[len(files)-1])
	fn, err := L.LoadString(entrySrc)
	if err != nil {
		b.Fatalf("compile error: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		L.Push(fn)
		if err := L.PCall(0, MultRet, nil); err != nil {
			b.Fatalf("runtime error: %v", err)
		}
		L.SetTop(0)
	}
}
