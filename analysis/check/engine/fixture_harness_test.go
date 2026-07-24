package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/lint"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// fixtureSuite is the original fixture manifest schema. It remains deliberately
// owned by the oracle: fixture data must not be translated into a new schema to
// suit the new engine.
type fixtureSuite struct {
	Description string        `json:"description,omitempty"`
	Files       []string      `json:"files,omitempty"`
	Stdlib      *bool         `json:"stdlib,omitempty"`
	Packages    []string      `json:"packages,omitempty"`
	Serial      bool          `json:"serial,omitempty"`
	Check       *fixtureCheck `json:"check,omitempty"`
	Run         *fixtureRun   `json:"run,omitempty"`
	Bench       *fixtureBench `json:"bench,omitempty"`
	Skip        string        `json:"skip,omitempty"`
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
	RequireComplete         bool           `json:"require_complete,omitempty"`
	MinStack                int            `json:"min_stack,omitempty"`
	MinOwnedHeap            int            `json:"min_owned_heap,omitempty"`
	MinSharedHeap           int            `json:"min_shared_heap,omitempty"`
	MaxStack                *int           `json:"max_stack,omitempty"`
	MaxOwnedHeap            *int           `json:"max_owned_heap,omitempty"`
	MaxSharedHeap           *int           `json:"max_shared_heap,omitempty"`
	MinStackDepth           int            `json:"min_stack_depth,omitempty"`
	MinOwnedHeapDepth       int            `json:"min_owned_heap_depth,omitempty"`
	MinSharedDepth          int            `json:"min_shared_depth,omitempty"`
	MinOwnerIdentity        int            `json:"min_owner_identity,omitempty"`
	MinSealBeforeShare      int            `json:"min_seal_before_share,omitempty"`
	MinAllocationSites      int            `json:"min_allocation_sites,omitempty"`
	MinDecomposable         int            `json:"min_decomposable,omitempty"`
	MinFrameLocal           int            `json:"min_frame_local,omitempty"`
	MaxNoFact               *int           `json:"max_no_fact,omitempty"`
	MaxUnknown              *int           `json:"max_unknown,omitempty"`
	MaxDecomposable         *int           `json:"max_decomposable,omitempty"`
	MaxFrameLocal           *int           `json:"max_frame_local,omitempty"`
	MinDiesBeforeSuspension int            `json:"min_dies_before_suspension,omitempty"`
	MaxDiesBeforeSuspension *int           `json:"max_dies_before_suspension,omitempty"`
	MinHoistableLoads       int            `json:"min_hoistable_loads,omitempty"`
	MaxHoistableLoads       *int           `json:"max_hoistable_loads,omitempty"`
	MinStackKind            map[string]int `json:"min_stack_kind,omitempty"`
	MinOwnedHeapKind        map[string]int `json:"min_owned_heap_kind,omitempty"`
	MinSharedHeapKind       map[string]int `json:"min_shared_heap_kind,omitempty"`
	MaxStackKind            map[string]int `json:"max_stack_kind,omitempty"`
	MaxOwnedHeapKind        map[string]int `json:"max_owned_heap_kind,omitempty"`
	MaxSharedHeapKind       map[string]int `json:"max_shared_heap_kind,omitempty"`
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
	Name  string
	Dir   string
	Suite fixtureSuite
}
type inlineExpectation struct {
	File     string
	Line     int
	Severity string
	Contains string
}

var expectRe = regexp.MustCompile(`--\s*expect-(error|warning)(?::\s*(.+?))?\s*$`)

func discoverFixtures(root string) ([]namedSuite, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var suites []namedSuite
	err = filepath.WalkDir(root, func(path string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !item.IsDir() {
			return nil
		}
		files, _ := filepath.Glob(filepath.Join(path, "*.lua"))
		if len(files) == 0 {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		suite := fixtureSuite{}
		if data, readErr := os.ReadFile(filepath.Join(path, "manifest.json")); readErr == nil {
			if err := json.Unmarshal(data, &suite); err != nil {
				return fmt.Errorf("bad manifest in %s: %w", filepath.ToSlash(rel), err)
			}
		}
		suites = append(suites, namedSuite{Name: filepath.ToSlash(rel), Dir: path, Suite: suite})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(suites, func(i, j int) bool { return suites[i].Name < suites[j].Name })
	return suites, nil
}

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
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		if entry.Name() == "main.lua" {
			hasMain = true
		} else {
			modules = append(modules, entry.Name())
		}
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
func readFixtureFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		panic(fmt.Sprintf("fixture file %s/%s: %v", dir, name, err))
	}
	return string(data)
}
func readFixtureSources(s namedSuite) map[string]string {
	out := make(map[string]string)
	for _, f := range resolveFiles(s) {
		out[f] = readFixtureFile(s.Dir, f)
	}
	return out
}
func parseExpectations(filename, source string) []inlineExpectation {
	var out []inlineExpectation
	for line, text := range strings.Split(source, "\n") {
		if m := expectRe.FindStringSubmatch(text); m != nil {
			out = append(out, inlineExpectation{File: filename, Line: line + 1, Severity: m[1], Contains: strings.TrimSpace(m[2])})
		}
	}
	return out
}

// fixtureDiagnostics is the only execution adaptation. The lint adapter owns
// project ordering, import resolution, engine.Check invocation, and conversion
// from engine facts to source diagnostics. The manifest oracle remains intact.
func fixtureDiagnostics(s namedSuite) ([]diag.Diagnostic, *engine.PlacementPlan, string, error) {
	files := resolveFiles(s)
	entries := make([]lint.Entry, 0, len(files))
	for _, file := range files {
		entries = append(entries, lint.Entry{Path: file, ModulePath: strings.TrimSuffix(file, ".lua"), Source: readFixtureFile(s.Dir, file)})
	}
	input := lint.ProjectInput{Entries: entries, Targets: []string{strings.TrimSuffix(files[len(files)-1], ".lua")}}
	if policy, err := fixtureDiagnosticPolicy(s.Suite.Check); err != nil {
		return nil, nil, files[len(files)-1], fmt.Errorf("diagnostic_rules: %w", err)
	} else {
		input.DiagnosticPolicy = policy
	}
	// The legacy package list supplies host modules. Local fixture modules are
	// resolved through lint's derived manifests, so each require observes the
	// producer's closed static export rather than the former Any placeholder.
	for _, pkg := range s.Suite.Packages {
		m := manifest.New(pkg)
		m.SetExport(typ.Any)
		input.Manifests = append(input.Manifests, m)
	}
	result, err := lint.CheckProject(context.Background(), input)
	if err != nil {
		return nil, nil, files[len(files)-1], err
	}
	return result.Diagnostics, result.Placement, files[len(files)-1], nil
}

func fixtureDiagnosticPolicy(check *fixtureCheck) (diag.Policy, error) {
	policy := diag.Policy{Rules: make(map[diag.Code]diag.Rule)}
	if check == nil {
		return policy, nil
	}
	for index, spec := range check.DiagnosticRules {
		code := diag.Code(strings.TrimSpace(spec.Code))
		if code == "" {
			return diag.Policy{}, fmt.Errorf("rule %d code is required", index+1)
		}
		if spec.Enabled == nil && strings.TrimSpace(spec.Severity) == "" {
			return diag.Policy{}, fmt.Errorf("rule %d must set enabled or severity", index+1)
		}
		var rule diag.Rule
		if spec.Enabled != nil {
			if *spec.Enabled {
				rule = diag.Enable()
			} else {
				rule = diag.Disable()
			}
		}
		if severity := strings.TrimSpace(spec.Severity); severity != "" {
			value, ok := diagnosticSeverity(severity)
			if !ok {
				return diag.Policy{}, fmt.Errorf("rule %d has unknown severity %q", index+1, spec.Severity)
			}
			rule = rule.WithSeverity(value)
		}
		policy.Rules[code] = rule
	}
	return policy, nil
}

type fixtureExpectationVerdict struct {
	name                   string
	passed                 bool
	missing, unexpected    []string
	expected, hits, misses int
}

func fullOracleFixtureVerdict(s namedSuite) (v fixtureExpectationVerdict) {
	v.name, v.passed = s.Name, true
	defer func() {
		if recovered := recover(); recovered != nil {
			v.passed = false
			v.unexpected = append(v.unexpected, fmt.Sprintf("panic: %v", recovered))
		}
	}()
	diagnostics, plan, entryFile, err := fixtureDiagnostics(s)
	if err != nil {
		v.passed = false
		v.unexpected = append(v.unexpected, "checker infrastructure failure: "+err.Error())
		return v
	}
	return judgeAgainstFixtureExpectations(s, diagnostics, plan, entryFile)
}

// judgeAgainstFixtureExpectations preserves the original precedence exactly:
// inline markers first, then manifest diagnostics (missing-only when inline
// markers exist), then error count, otherwise clean check. Check.Skip and
// fixture Skip are intentionally not consulted by the hard oracle.
func judgeAgainstFixtureExpectations(s namedSuite, diagnostics []diag.Diagnostic, plan *engine.PlacementPlan, entryFile string) fixtureExpectationVerdict {
	v := fixtureExpectationVerdict{name: s.Name, passed: true}
	var inline []inlineExpectation
	for _, file := range resolveFiles(s) {
		inline = append(inline, parseExpectations(file, readFixtureFile(s.Dir, file))...)
	}
	if len(inline) > 0 {
		v.expected += len(inline)
		matched := make([]bool, len(diagnostics))
		for _, exp := range inline {
			found := false
			for i, item := range diagnostics {
				if matchesExpectation(exp, item, entryFile) {
					found = true
					matched[i] = true
				}
			}
			if found {
				v.hits++
			} else {
				v.passed = false
				v.misses++
				v.missing = append(v.missing, fmt.Sprintf("expected %s at %s:%d %q not emitted", exp.Severity, exp.File, exp.Line, exp.Contains))
			}
		}
		for i, item := range diagnostics {
			if !matched[i] && item.Severity != diag.SeverityHint {
				v.passed = false
				v.unexpected = append(v.unexpected, diagSummary(item))
			}
		}
		if s.Suite.Check != nil && len(s.Suite.Check.Diagnostics) > 0 {
			m, h := matchDiagnosticExpectations(s.Suite.Check.Diagnostics, diagnostics, entryFile, false, renderOptions(s, entryFile))
			v.expected += len(s.Suite.Check.Diagnostics)
			v.hits += h
			v.misses += len(m)
			if len(m) > 0 {
				v.passed = false
				for _, item := range m {
					v.missing = append(v.missing, "structured diagnostic not emitted: "+item)
				}
			}
		} else if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
			applyErrorCount(&v, *s.Suite.Check.Errors, diagnostics)
		}
	} else if s.Suite.Check != nil && len(s.Suite.Check.Diagnostics) > 0 {
		m, h, u := matchDiagnosticExpectationsComplete(s.Suite.Check.Diagnostics, diagnostics, entryFile, renderOptions(s, entryFile))
		v.expected += len(s.Suite.Check.Diagnostics)
		v.hits += h
		v.misses += len(m)
		if len(m)+len(u) > 0 {
			v.passed = false
			for _, item := range m {
				v.missing = append(v.missing, "structured diagnostic not emitted: "+item)
			}
			v.unexpected = append(v.unexpected, u...)
		}
	} else if s.Suite.Check != nil && s.Suite.Check.Errors != nil {
		applyErrorCount(&v, *s.Suite.Check.Errors, diagnostics)
	} else {
		for _, item := range diagnostics {
			if item.Severity == diag.SeverityError {
				v.passed = false
				v.unexpected = append(v.unexpected, diagSummary(item))
			}
		}
	}
	if s.Suite.Check != nil && s.Suite.Check.Placement != nil {
		for _, missing := range placementExpectationMisses(s.Suite.Check.Placement, plan) {
			v.passed = false
			v.missing = append(v.missing, "placement: "+missing)
		}
	}
	return v
}

func placementExpectationMisses(expect *fixturePlacement, plan *engine.PlacementPlan) []string {
	if plan == nil {
		return []string{"no placement plan (no allocation fact was proven)"}
	}
	allocations := plan.Allocations
	count := func(predicate func(engine.PlacementAllocation) bool) int {
		out := 0
		for _, allocation := range allocations {
			if predicate(allocation) {
				out++
			}
		}
		return out
	}
	byPlacement := func(want placement.Value) int {
		return count(func(item engine.PlacementAllocation) bool { return item.Placement == want })
	}
	maxDepth := func(want placement.Value) int {
		depth := 0
		for _, item := range allocations {
			if item.Placement == want && item.Depth > depth {
				depth = item.Depth
			}
		}
		return depth
	}
	byKind := func(want placement.Value, kinds map[string]int) []string {
		var misses []string
		for kind, minimum := range kinds {
			got := count(func(item engine.PlacementAllocation) bool { return item.Placement == want && item.Kind == kind })
			if got < minimum {
				misses = append(misses, fmt.Sprintf("%s kind %s = %d, want at least %d", want, kind, got, minimum))
			}
		}
		return misses
	}
	var misses []string
	minimum := func(name string, got, want int) {
		if got < want {
			misses = append(misses, fmt.Sprintf("%s = %d, want at least %d", name, got, want))
		}
	}
	maximum := func(name string, got int, want *int) {
		if want != nil && got > *want {
			misses = append(misses, fmt.Sprintf("%s = %d, want at most %d", name, got, *want))
		}
	}
	if expect.RequireComplete && !plan.Complete {
		misses = append(misses, "plan is incomplete")
	}
	minimum("stack", byPlacement(placement.Stack), expect.MinStack)
	minimum("owned_heap", byPlacement(placement.OwnedHeap), expect.MinOwnedHeap)
	minimum("shared_heap", byPlacement(placement.SharedHeap), expect.MinSharedHeap)
	maximum("stack", byPlacement(placement.Stack), expect.MaxStack)
	maximum("owned_heap", byPlacement(placement.OwnedHeap), expect.MaxOwnedHeap)
	maximum("shared_heap", byPlacement(placement.SharedHeap), expect.MaxSharedHeap)
	minimum("allocation_sites", len(allocations), expect.MinAllocationSites)
	minimum("decomposable", count(func(item engine.PlacementAllocation) bool { return item.Decomposable }), expect.MinDecomposable)
	maximum("decomposable", count(func(item engine.PlacementAllocation) bool { return item.Decomposable }), expect.MaxDecomposable)
	minimum("frame_local", count(func(item engine.PlacementAllocation) bool { return item.FrameLocal }), expect.MinFrameLocal)
	maximum("frame_local", count(func(item engine.PlacementAllocation) bool { return item.FrameLocal }), expect.MaxFrameLocal)
	minimum("dies_before_suspension", count(func(item engine.PlacementAllocation) bool { return item.DiesBeforeSuspension }), expect.MinDiesBeforeSuspension)
	maximum("dies_before_suspension", count(func(item engine.PlacementAllocation) bool { return item.DiesBeforeSuspension }), expect.MaxDiesBeforeSuspension)
	minimum("owner_identity", count(func(item engine.PlacementAllocation) bool { return item.OwnerIdentity }), expect.MinOwnerIdentity)
	minimum("seal_before_share", count(func(item engine.PlacementAllocation) bool { return item.SealBeforeShare }), expect.MinSealBeforeShare)
	minimum("hoistable_loads", len(plan.HoistableLoads), expect.MinHoistableLoads)
	maximum("hoistable_loads", len(plan.HoistableLoads), expect.MaxHoistableLoads)
	minimum("stack_depth", maxDepth(placement.Stack), expect.MinStackDepth)
	minimum("owned_heap_depth", maxDepth(placement.OwnedHeap), expect.MinOwnedHeapDepth)
	minimum("shared_depth", maxDepth(placement.SharedHeap), expect.MinSharedDepth)
	maximum("unknown", byPlacement(placement.Unknown), expect.MaxUnknown)
	maximum("no_fact", 0, expect.MaxNoFact)
	misses = append(misses, byKind(placement.Stack, expect.MinStackKind)...)
	misses = append(misses, byKind(placement.OwnedHeap, expect.MinOwnedHeapKind)...)
	misses = append(misses, byKind(placement.SharedHeap, expect.MinSharedHeapKind)...)
	for kind, maximumCount := range expect.MaxStackKind {
		got := count(func(item engine.PlacementAllocation) bool {
			return item.Placement == placement.Stack && item.Kind == kind
		})
		if got > maximumCount {
			misses = append(misses, fmt.Sprintf("stack kind %s = %d, want at most %d", kind, got, maximumCount))
		}
	}
	for kind, maximumCount := range expect.MaxOwnedHeapKind {
		got := count(func(item engine.PlacementAllocation) bool {
			return item.Placement == placement.OwnedHeap && item.Kind == kind
		})
		if got > maximumCount {
			misses = append(misses, fmt.Sprintf("owned_heap kind %s = %d, want at most %d", kind, got, maximumCount))
		}
	}
	for kind, maximumCount := range expect.MaxSharedHeapKind {
		got := count(func(item engine.PlacementAllocation) bool {
			return item.Placement == placement.SharedHeap && item.Kind == kind
		})
		if got > maximumCount {
			misses = append(misses, fmt.Sprintf("shared_heap kind %s = %d, want at most %d", kind, got, maximumCount))
		}
	}
	return misses
}

func applyErrorCount(v *fixtureExpectationVerdict, want int, diagnostics []diag.Diagnostic) {
	got := 0
	for _, item := range diagnostics {
		if item.Severity == diag.SeverityError {
			got++
		}
	}
	if got != want {
		v.passed = false
		v.missing = append(v.missing, fmt.Sprintf("expected %d errors, got %d", want, got))
		if got > want {
			for _, item := range diagnostics {
				if item.Severity == diag.SeverityError {
					v.unexpected = append(v.unexpected, diagSummary(item))
				}
			}
		}
	}
}
func matchesExpectation(exp inlineExpectation, item diag.Diagnostic, entryFile string) bool {
	if !matchesDiagnosticFile(exp.File, item, entryFile) || item.Position.Line != exp.Line {
		return false
	}
	want := diag.SeverityError
	if exp.Severity == "warning" {
		want = diag.SeverityWarning
	}
	return item.Severity == want && (exp.Contains == "" || strings.Contains(item.Message, exp.Contains))
}
func matchesDiagnosticFile(expected string, item diag.Diagnostic, entryFile string) bool {
	actual := item.Position.File
	return expected == "" || actual == expected || actual == strings.TrimSuffix(expected, ".lua") || (actual == "test.lua" && (expected == entryFile || strings.TrimSuffix(expected, ".lua") == strings.TrimSuffix(entryFile, ".lua")))
}
func diagSummary(item diag.Diagnostic) string {
	return fmt.Sprintf("%s:%d:%d [%s] %s", item.Position.File, item.Position.Line, item.Position.Column, item.Code, item.Message)
}

func matchDiagnosticExpectations(expectations []fixtureDiagnosticExpectation, diagnostics []diag.Diagnostic, entryFile string, requireNoUnexpected bool, options diag.RenderOptions) ([]string, int) {
	missing, hits, _ := matchDiagnosticExpectationsWithUnexpected(expectations, diagnostics, entryFile, requireNoUnexpected, options)
	return missing, hits
}
func matchDiagnosticExpectationsComplete(expectations []fixtureDiagnosticExpectation, diagnostics []diag.Diagnostic, entryFile string, options diag.RenderOptions) ([]string, int, []string) {
	return matchDiagnosticExpectationsWithUnexpected(expectations, diagnostics, entryFile, true, options)
}
func matchDiagnosticExpectationsWithUnexpected(expectations []fixtureDiagnosticExpectation, diagnostics []diag.Diagnostic, entryFile string, requireNoUnexpected bool, options diag.RenderOptions) (missing []string, hits int, unexpected []string) {
	matched := make([]bool, len(diagnostics))
	needHints := false
	for _, exp := range expectations {
		if exp.Severity == "hint" {
			needHints = true
		}
	}
	for _, exp := range expectations {
		if err := validateDiagnosticExpectation(exp); err != nil {
			missing = append(missing, fmt.Sprintf("invalid diagnostic expectation: %s (%s)", err, describeDiagnosticExpectation(exp)))
			continue
		}
		found := false
		for i, item := range diagnostics {
			if !matched[i] && matchesDiagnosticExpectation(exp, item, entryFile, options) {
				matched[i], found = true, true
				hits++
				break
			}
		}
		if !found {
			missing = append(missing, describeDiagnosticExpectation(exp))
		}
	}
	if requireNoUnexpected {
		for i, item := range diagnostics {
			if !matched[i] && (item.Severity != diag.SeverityHint || needHints) {
				unexpected = append(unexpected, diagSummary(item))
			}
		}
	}
	return missing, hits, unexpected
}

// legacyCodeMatches is intentionally small. The new engine's only semantic
// type-failure publication is an unproven annotation claim; it is equivalent
// to the old assignment-contract family, but the remaining message, position,
// evidence, label, help, and render assertions must still all match.
func legacyCodeMatches(expected string, actual diag.Code) bool {
	return expected == actual.String() || ((expected == "type.assignment" || expected == "type.assignment.optional_target") && actual == "lint.claim.unproven")
}
func matchesDiagnosticExpectation(exp fixtureDiagnosticExpectation, item diag.Diagnostic, entryFile string, options diag.RenderOptions) bool {
	if !matchesDiagnosticFile(exp.File, item, entryFile) || (exp.Line != 0 && item.Position.Line != exp.Line) || (exp.Column != 0 && item.Position.Column != exp.Column) {
		return false
	}
	severity, ok := diagnosticSeverity(exp.Severity)
	if !ok || item.Severity != severity || !legacyCodeMatches(exp.Code, item.Code) || !containsAll(item.Message, exp.MessageContains) {
		return false
	}
	evidence := item.Explanation.Evidence()
	if exp.MinEvidence > 0 && len(evidence) < exp.MinEvidence {
		return false
	}
	if !exp.AllowEmptyEvidence && (exp.MinEvidence > 0 || len(exp.EvidenceContains) > 0) && len(evidence) == 0 {
		return false
	}
	if !containsAll(item.Explanation.String(), exp.EvidenceContains) || !matchesEvidence(exp.Evidence, item, entryFile) {
		return false
	}
	if len(exp.RenderContains)+len(exp.RenderOrderedContains)+len(exp.RenderNotContains) > 0 {
		rendered := diag.Render(item, options)
		if !containsAll(rendered, exp.RenderContains) || !containsInOrder(rendered, exp.RenderOrderedContains) || containsAny(rendered, exp.RenderNotContains) {
			return false
		}
	}
	return containsAll(item.Help, exp.HelpContains) && (exp.MinLabels == 0 || len(item.Labels) >= exp.MinLabels) && containsAll(formatLabels(item.Labels), exp.LabelContains) && matchesLabels(exp.Labels, item, entryFile)
}
func matchesEvidence(expectations []fixtureDiagnosticEvidenceExpectation, item diag.Diagnostic, entryFile string) bool {
	offset := 0
	evidence := item.Explanation.Evidence()
	for _, exp := range expectations {
		found := false
		for offset < len(evidence) {
			candidate := evidence[offset]
			offset++
			file := candidate.File
			if file == "" {
				file = item.Position.File
			}
			copy := item
			copy.Position.File = file
			if matchesDiagnosticFile(exp.File, copy, entryFile) && (exp.Line == 0 || candidate.Span.StartLine == exp.Line) && (exp.Column == 0 || candidate.Span.StartCol == exp.Column) && (exp.Kind == "" || candidate.Kind.String() == exp.Kind) && (exp.Trust == "" || candidate.Trust.String() == exp.Trust) && (exp.Reason == "" || candidate.Reason.String() == exp.Reason) && containsAll(diag.NewExplanation(candidate).String(), exp.Contains) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func matchesLabels(expectations []fixtureDiagnosticLabelExpectation, item diag.Diagnostic, entryFile string) bool {
	for _, exp := range expectations {
		found := false
		for _, label := range item.Labels {
			file := label.File
			if file == "" {
				file = item.Position.File
			}
			copy := item
			copy.Position.File = file
			if matchesDiagnosticFile(exp.File, copy, entryFile) && (exp.Line == 0 || label.Span.StartLine == exp.Line) && (exp.Column == 0 || label.Span.StartCol == exp.Column) && containsAll(label.Message, exp.Contains) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func diagnosticSeverity(value string) (diag.Severity, bool) {
	switch value {
	case "error":
		return diag.SeverityError, true
	case "warning":
		return diag.SeverityWarning, true
	case "hint":
		return diag.SeverityHint, true
	}
	return diag.SeverityError, false
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
func formatLabels(labels []diag.Label) string {
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		parts = append(parts, label.Message)
	}
	return strings.Join(parts, "\n")
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
	if _, ok := diagnosticSeverity(exp.Severity); !ok {
		return fmt.Errorf("unknown severity %q", exp.Severity)
	}
	if strings.TrimSpace(exp.Code) == "" {
		return fmt.Errorf("code is required")
	}
	for _, pair := range []struct {
		name     string
		values   []string
		required bool
	}{{"message_contains", exp.MessageContains, true}, {"evidence_contains", exp.EvidenceContains, !exp.AllowEmptyEvidence && len(exp.Evidence) == 0}, {"render_contains", exp.RenderContains, true}, {"render_ordered_contains", exp.RenderOrderedContains, false}, {"render_not_contains", exp.RenderNotContains, false}, {"help_contains", exp.HelpContains, true}, {"label_contains", exp.LabelContains, len(exp.Labels) == 0}} {
		if err := validateContains(pair.name, pair.values, pair.required); err != nil {
			return err
		}
	}
	for index, evidence := range exp.Evidence {
		if err := validateDiagnosticEvidenceExpectation(evidence); err != nil {
			return fmt.Errorf("evidence[%d]: %w", index, err)
		}
	}
	for index, label := range exp.Labels {
		if err := validateDiagnosticLabelExpectation(label); err != nil {
			return fmt.Errorf("labels[%d]: %w", index, err)
		}
	}
	if exp.MinEvidence < 0 || exp.MinLabels < 0 {
		if exp.MinEvidence < 0 {
			return fmt.Errorf("min_evidence must be non-negative")
		}
		return fmt.Errorf("min_labels must be non-negative")
	}
	if !exp.AllowEmptyEvidence && exp.MinEvidence <= 0 && len(exp.Evidence) == 0 {
		return fmt.Errorf("min_evidence must be positive unless allow_empty_evidence is true")
	}
	return nil
}

func validateDiagnosticEvidenceExpectation(exp fixtureDiagnosticEvidenceExpectation) error {
	if err := validateContains("contains", exp.Contains, true); err != nil {
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
	if err := validateContains("contains", exp.Contains, true); err != nil {
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

func validDiagnosticEvidenceKind(kind string) bool {
	switch kind {
	case diag.EvidenceAbstractFact.String(), diag.EvidenceUserAssertion.String(), diag.EvidenceMissingProof.String(), diag.EvidencePrecisionBoundary.String():
		return true
	default:
		return false
	}
}

func validDiagnosticEvidenceTrust(trust string) bool {
	switch trust {
	case diag.TrustProven.String(), diag.TrustClaimed.String(), diag.TrustRefuted.String(), diag.TrustUnknown.String():
		return true
	default:
		return false
	}
}

func validDiagnosticEvidenceReason(reason string) bool {
	switch reason {
	case diag.EvidenceReasonUnspecified.String(), diag.EvidenceReasonBoundaryValidationMissing.String(), diag.EvidenceReasonIndexReadValidationMissing.String(), diag.EvidenceReasonExplicitBoundaryValidation.String():
		return true
	default:
		return false
	}
}
func validateContains(name string, values []string, required bool) error {
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
func describeDiagnosticExpectation(exp fixtureDiagnosticExpectation) string {
	return fmt.Sprintf("file=%s, line=%d, column=%d, severity=%s, code=%s, message~%q", exp.File, exp.Line, exp.Column, exp.Severity, exp.Code, exp.MessageContains)
}
func renderOptions(s namedSuite, entryFile string) diag.RenderOptions {
	sources := diag.SourceMap{}
	display := map[string]string{}
	for file, content := range readFixtureSources(s) {
		sources[file] = content
		module := strings.TrimSuffix(file, ".lua")
		sources[module] = content
		display[module] = file
	}
	sources["test.lua"] = sources[entryFile]
	display["test.lua"] = entryFile
	return diag.RenderOptions{Sources: sources, DisplayFiles: display, ShowSourceLabelRows: true, WitnessTrace: s.Suite.Check != nil && s.Suite.Check.RenderOptions.WitnessTrace}
}

type categoryCensus struct{ pass, fail, expected, hits, misses int }
type fixtureOracleReporter struct {
	pass, fail, expected, hits, misses int
	categories                         map[string]*categoryCensus
	started                            time.Time
}

func newFixtureOracleReporter() *fixtureOracleReporter {
	return &fixtureOracleReporter{categories: make(map[string]*categoryCensus), started: time.Now()}
}
func fixtureCategory(name string) string {
	if head, _, found := strings.Cut(name, "/"); found {
		return head
	}
	return name
}
func (r *fixtureOracleReporter) record(v fixtureExpectationVerdict) {
	r.expected += v.expected
	r.hits += v.hits
	r.misses += v.misses
	category := fixtureCategory(v.name)
	bucket := r.categories[category]
	if bucket == nil {
		bucket = &categoryCensus{}
		r.categories[category] = bucket
	}
	bucket.expected += v.expected
	bucket.hits += v.hits
	bucket.misses += v.misses
	if v.passed {
		r.pass++
		bucket.pass++
	} else {
		r.fail++
		bucket.fail++
	}
}
func (r *fixtureOracleReporter) finish(t *testing.T) {
	total := r.pass + r.fail
	t.Logf("FULL ORACLE SCORECARD: %d/%d fixtures PASS against fixture expectations (%d fail)", r.pass, total, r.fail)
	t.Logf("FULL ORACLE DIAGNOSTICS: %d/%d expected diagnostics hit (%d miss)", r.hits, r.expected, r.misses)
	categories := make([]string, 0, len(r.categories))
	for category := range r.categories {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	t.Log("FULL ORACLE BY CATEGORY:")
	for _, category := range categories {
		bucket := r.categories[category]
		t.Logf("  %s: fixtures %d pass / %d fail; diagnostics %d/%d hit (%d miss)", category, bucket.pass, bucket.fail, bucket.hits, bucket.expected, bucket.misses)
	}
	t.Logf("FULL ORACLE WALL TIME: %s", time.Since(r.started))
}

// TestFullOracle is the hard semantic gate. It never honors fixture Skip:
// a fixture's checked-in expectations are the oracle, including expectations
// the new engine does not yet implement.
func TestFullOracle(t *testing.T) {
	suites, err := discoverFixtures(filepath.Join(corpusRepositoryRoot(t), "testdata", "fixtures"))
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("full oracle discovered no fixture suites")
	}
	reporter := newFixtureOracleReporter()
	defer reporter.finish(t)
	for _, suite := range suites {
		suite := suite
		t.Run(suite.Name, func(t *testing.T) {
			verdict := fullOracleFixtureVerdict(suite)
			reporter.record(verdict)
			if !verdict.passed {
				t.Errorf("fixture fails checked-in expectations (%d missing, %d unexpected)", len(verdict.missing), len(verdict.unexpected))
				for _, message := range verdict.missing {
					t.Errorf("    MISS: %s", message)
				}
				for _, message := range verdict.unexpected {
					t.Errorf("    FALSE+: %s", message)
				}
			}
		})
	}
}
