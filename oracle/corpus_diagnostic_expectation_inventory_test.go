package oracle

import (
	"bytes"
	"encoding/json"
	"fmt"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

var corpusInlineDiagnosticExpectation = regexp.MustCompile(`--\s*expect-(error|warning)(?::\s*(.+?))?\s*$`)

var (
	corpusDiagnosticCatalogOnce  sync.Once
	corpusDiagnosticCatalogValue *corpusDiagnosticExpectationCatalog
	corpusDiagnosticCatalogErr   error
)

type corpusDiagnosticManifest struct {
	Description string                         `json:"description,omitempty"`
	Name        string                         `json:"name,omitempty"`
	Files       []string                       `json:"files,omitempty"`
	Stdlib      *bool                          `json:"stdlib,omitempty"`
	Packages    []string                       `json:"packages,omitempty"`
	Serial      bool                           `json:"serial,omitempty"`
	Check       *corpusDiagnosticManifestCheck `json:"check,omitempty"`
	Run         *corpusFixtureRun              `json:"run,omitempty"`
	Bench       *corpusFixtureBench            `json:"bench,omitempty"`
	Skip        string                         `json:"skip,omitempty"`
}

// corpusDiagnosticManifestCheck deliberately models every present check
// contract. Diagnostics, native publication, and Placement summary checks are
// consumed through their domain-owned public facades. Keeping every modeled
// contract visible prevents an absent consumer from being mistaken for a clean
// fixture.
type corpusDiagnosticManifestCheck struct {
	Errors          *int                                    `json:"errors,omitempty"`
	Diagnostics     []corpusStructuredDiagnosticExpectation `json:"diagnostics,omitempty"`
	DiagnosticRules []corpusDiagnosticRuleExpectation       `json:"diagnostic_rules,omitempty"`
	RenderOptions   *corpusDiagnosticRenderOptions          `json:"render_options,omitempty"`
	Native          *corpusNativeContract                   `json:"native,omitempty"`
	Placement       *corpusPlacementContract                `json:"placement,omitempty"`
	Skip            string                                  `json:"skip,omitempty"`
}

type corpusDiagnosticRenderOptions struct {
	WitnessTrace bool `json:"witness_trace,omitempty"`
}

type corpusNativeContract struct {
	MinFacts     *int                               `json:"min_facts,omitempty"`
	MaxFacts     *int                               `json:"max_facts,omitempty"`
	Facts        []corpusNativeFactContract         `json:"facts,omitempty"`
	Invalidation []corpusNativeInvalidationContract `json:"invalidation,omitempty"`
}

// corpusNativeFactContract is a test-only transport of the public
// publication/dataflow assertion. It does not create an Engine fact or grant
// a producer any semantic authority.
type corpusNativeFactContract struct {
	Name                string                   `json:"name,omitempty"`
	Lane                string                   `json:"lane,omitempty"`
	Module              string                   `json:"module,omitempty"`
	Family              string                   `json:"family,omitempty"`
	Key                 string                   `json:"key,omitempty"`
	KeyPrefix           string                   `json:"key_prefix,omitempty"`
	KeySuffix           string                   `json:"key_suffix,omitempty"`
	KeyContains         []string                 `json:"key_contains,omitempty"`
	Subject             string                   `json:"subject,omitempty"`
	Term                string                   `json:"term,omitempty"`
	Occurrence          string                   `json:"occurrence,omitempty"`
	Trust               string                   `json:"trust,omitempty"`
	Exact               string                   `json:"exact,omitempty"`
	Literal             string                   `json:"literal,omitempty"`
	Representation      string                   `json:"representation,omitempty"`
	Left                string                   `json:"left,omitempty"`
	Right               string                   `json:"right,omitempty"`
	Operand             string                   `json:"operand,omitempty"`
	Operator            string                   `json:"operator,omitempty"`
	Overflow            string                   `json:"overflow,omitempty"`
	Divisor             string                   `json:"divisor,omitempty"`
	Truthiness          string                   `json:"truthiness,omitempty"`
	Partition           string                   `json:"partition,omitempty"`
	DeadArm             string                   `json:"dead_arm,omitempty"`
	DeadArmReachable    string                   `json:"dead_arm_reachable,omitempty"`
	Value               *string                  `json:"value,omitempty"`
	ValuePrefix         string                   `json:"value_prefix,omitempty"`
	ValueContains       []string                 `json:"value_contains,omitempty"`
	Min                 *int                     `json:"min,omitempty"`
	Max                 *int                     `json:"max,omitempty"`
	RevokedBy           []corpusNativeRevocation `json:"revoked_by,omitempty"`
	RevokedByExhaustive bool                     `json:"revoked_by_exhaustive,omitempty"`
}

type corpusNativeRevocation struct {
	Event       string `json:"event,omitempty"`
	Established string `json:"established,omitempty"`
	Revoked     string `json:"revoked,omitempty"`
}

type corpusNativeInvalidationContract struct {
	Name             string   `json:"name,omitempty"`
	Lane             string   `json:"lane,omitempty"`
	Module           string   `json:"module,omitempty"`
	Family           string   `json:"family,omitempty"`
	Key              string   `json:"key,omitempty"`
	KeyPrefix        string   `json:"key_prefix,omitempty"`
	KeySuffix        string   `json:"key_suffix,omitempty"`
	KeyContains      []string `json:"key_contains,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	Term             string   `json:"term,omitempty"`
	Occurrence       string   `json:"occurrence,omitempty"`
	Exact            string   `json:"exact,omitempty"`
	Literal          string   `json:"literal,omitempty"`
	Representation   string   `json:"representation,omitempty"`
	Left             string   `json:"left,omitempty"`
	Right            string   `json:"right,omitempty"`
	Operand          string   `json:"operand,omitempty"`
	Operator         string   `json:"operator,omitempty"`
	Overflow         string   `json:"overflow,omitempty"`
	Divisor          string   `json:"divisor,omitempty"`
	Truthiness       string   `json:"truthiness,omitempty"`
	Partition        string   `json:"partition,omitempty"`
	DeadArm          string   `json:"dead_arm,omitempty"`
	DeadArmReachable string   `json:"dead_arm_reachable,omitempty"`

	Value         *string  `json:"value,omitempty"`
	ValuePrefix   string   `json:"value_prefix,omitempty"`
	ValueContains []string `json:"value_contains,omitempty"`
	Trust         string   `json:"trust,omitempty"`
	Event         string   `json:"event,omitempty"`
	Established   string   `json:"established,omitempty"`
	Revoked       string   `json:"revoked,omitempty"`
	Min           *int     `json:"min,omitempty"`
	Max           *int     `json:"max,omitempty"`
}

type corpusPlacementContract struct {
	RequireComplete    bool `json:"require_complete,omitempty"`
	MinStack           int  `json:"min_stack,omitempty"`
	MinOwnedHeap       int  `json:"min_owned_heap,omitempty"`
	MinSharedHeap      int  `json:"min_shared_heap,omitempty"`
	MaxStack           *int `json:"max_stack,omitempty"`
	MaxOwnedHeap       *int `json:"max_owned_heap,omitempty"`
	MaxSharedHeap      *int `json:"max_shared_heap,omitempty"`
	MinStackDepth      int  `json:"min_stack_depth,omitempty"`
	MinOwnedHeapDepth  int  `json:"min_owned_heap_depth,omitempty"`
	MinSharedDepth     int  `json:"min_shared_depth,omitempty"`
	MinOwnerIdentity   int  `json:"min_owner_identity,omitempty"`
	MinAllocationSites int  `json:"min_allocation_sites,omitempty"`
	MinFrameLocal      int  `json:"min_frame_local,omitempty"`
	MaxNoFact          *int `json:"max_no_fact,omitempty"`
	MaxUnknown         *int `json:"max_unknown,omitempty"`
	MaxFrameLocal      *int `json:"max_frame_local,omitempty"`
	// RetainEscape is an optional proof bound over authenticated present
	// position Facts. Omission is intentional for legacy manifests: the
	// acceptance oracle must never infer retention from Class or from an
	// aggregate join across temporal positions.
	MinRetainProvenPositions int            `json:"min_retain_proven_positions,omitempty"`
	MaxRetainProvenPositions *int           `json:"max_retain_proven_positions,omitempty"`
	MinDiesBeforeSuspension  int            `json:"min_dies_before_suspension,omitempty"`
	MaxDiesBeforeSuspension  *int           `json:"max_dies_before_suspension,omitempty"`
	MinDeepFrozen            int            `json:"min_deep_frozen,omitempty"`
	MaxDeepFrozen            *int           `json:"max_deep_frozen,omitempty"`
	MinStackKind             map[string]int `json:"min_stack_kind,omitempty"`
	MinOwnedHeapKind         map[string]int `json:"min_owned_heap_kind,omitempty"`
	MinSharedHeapKind        map[string]int `json:"min_shared_heap_kind,omitempty"`
	MaxStackKind             map[string]int `json:"max_stack_kind,omitempty"`
	MaxOwnedHeapKind         map[string]int `json:"max_owned_heap_kind,omitempty"`
	MaxSharedHeapKind        map[string]int `json:"max_shared_heap_kind,omitempty"`
}

type corpusFixtureRun struct {
	Golden        string `json:"golden,omitempty"`
	Error         bool   `json:"error,omitempty"`
	ErrorContains string `json:"error_contains,omitempty"`
	Skip          string `json:"skip,omitempty"`
}

type corpusFixtureBench struct {
	Skip string `json:"skip,omitempty"`
}

type corpusStructuredDiagnosticExpectation struct {
	File                  string                                `json:"file,omitempty"`
	Line                  int                                   `json:"line,omitempty"`
	Column                int                                   `json:"column,omitempty"`
	Severity              string                                `json:"severity,omitempty"`
	Code                  string                                `json:"code,omitempty"`
	MessageContains       []string                              `json:"message_contains,omitempty"`
	EvidenceContains      []string                              `json:"evidence_contains,omitempty"`
	Evidence              []corpusDiagnosticEvidenceExpectation `json:"evidence,omitempty"`
	RenderContains        []string                              `json:"render_contains,omitempty"`
	RenderOrderedContains []string                              `json:"render_ordered_contains,omitempty"`
	RenderNotContains     []string                              `json:"render_not_contains,omitempty"`
	HelpContains          []string                              `json:"help_contains,omitempty"`
	LabelContains         []string                              `json:"label_contains,omitempty"`
	Labels                []corpusDiagnosticLabelExpectation    `json:"labels,omitempty"`
	MinEvidence           int                                   `json:"min_evidence,omitempty"`
	MinLabels             int                                   `json:"min_labels,omitempty"`
	AllowEmptyEvidence    bool                                  `json:"allow_empty_evidence,omitempty"`
}

type corpusDiagnosticEvidenceExpectation struct {
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Trust    string   `json:"trust,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Contains []string `json:"contains,omitempty"`
}

type corpusDiagnosticLabelExpectation struct {
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	Contains []string `json:"contains,omitempty"`
}

type corpusDiagnosticRuleExpectation struct {
	Code     string `json:"code,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type corpusDiagnosticExpectationInventory struct {
	projects, luaFiles, manifests, annotatedFiles     int
	inlineErrors, inlineWarnings                      int
	structuredManifests, structuredFindings           int
	structuredErrors, structuredWarnings              int
	structuredHints                                   int
	structuredCodes                                   map[string]int
	ruleCount, enabledRuleCount, disabledRuleCount    int
	ruleCodes                                         map[string]int
	errorCountManifests                               int
	errorCounts                                       map[int]int
	declaredFileManifests, declaredFiles              int
	packageManifests, packageDeclarations             int
	stdlibManifests, renderOptionManifests            int
	nativeManifests, nativeFacts, nativeInvalidations int
	placementManifests                                int
}

type corpusInlineDiagnosticExpectationRow struct {
	File, Severity, Contains string
	Line                     int
}

type corpusDiagnosticProjectExpectations struct {
	name          string
	directory     string
	files         []string // all discovered local Lua files, canonicalized
	declaredFiles []string // manifest order, never substituted from discovery
	entryFile     string   // manifest last file, else legacy canonical selection
	entryModule   string   // selected entry identity without the .lua suffix
	inline        []corpusInlineDiagnosticExpectationRow
	manifest      *corpusDiagnosticManifest
}

type corpusDiagnosticLocationKey struct {
	project, file, severity string
	line                    int
}

type corpusStructuredDiagnosticRef struct {
	project string
	ordinal int
}

type corpusNativeContractRef struct {
	project string
	ordinal int
}

type corpusStructuredDiagnosticLocationKey struct {
	project, code, file, severity string
	line                          int
}

// corpusDiagnosticExpectationCatalog is the single test-only parse shared by
// narrow family laws and the complete oracle. Its indexes keep matching
// proportional to expectations plus findings, never their Cartesian product.
type corpusDiagnosticExpectationCatalog struct {
	inventory        corpusDiagnosticExpectationInventory
	projects         []*corpusDiagnosticProjectExpectations
	byProject        map[string]*corpusDiagnosticProjectExpectations
	inlineByLocation map[corpusDiagnosticLocationKey][]corpusInlineDiagnosticExpectationRow
	structuredByCode map[string][]corpusStructuredDiagnosticRef
	// structuredByLocation is the final oracle's exact O(1) anchor. A
	// collision is rejected by the inventory law instead of introducing a
	// corpus-wide matching search.
	structuredByLocation   map[corpusStructuredDiagnosticLocationKey][]corpusStructuredDiagnosticRef
	nativeProjects         map[string]*corpusNativeContract
	placementProjects      map[string]*corpusPlacementContract
	nativeFactRows         []corpusNativeContractRef
	nativeInvalidationRows []corpusNativeContractRef
}

// TestFrozenCorpusDiagnosticExpectationInventory is the denominator contract
// for the new analyzer's test-only oracle. It intentionally parses source and
// manifest expectations outside program/testfixture: fixture expectations may
// judge public findings, but can never feed semantic facts into Program, Link,
// artifact compilation, or inference.
func TestFrozenCorpusDiagnosticExpectationInventory(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	inventory := catalog.inventory

	if inventory.projects != 944 || inventory.luaFiles != testfixture.FrozenLuaFileCount || inventory.manifests != 563 || inventory.annotatedFiles != 188 {
		t.Fatalf("fixture denominator changed: projects=%d lua=%d manifests=%d annotated=%d", inventory.projects, inventory.luaFiles, inventory.manifests, inventory.annotatedFiles)
	}
	if inventory.inlineErrors != 727 || inventory.inlineWarnings != 4 {
		t.Fatalf("inline diagnostic denominator changed: errors=%d warnings=%d", inventory.inlineErrors, inventory.inlineWarnings)
	}
	if inventory.structuredManifests != 75 || inventory.structuredFindings != 142 || inventory.structuredErrors != 98 || inventory.structuredWarnings != 18 || inventory.structuredHints != 26 {
		t.Fatalf("structured diagnostic denominator changed: manifests=%d findings=%d errors=%d warnings=%d hints=%d", inventory.structuredManifests, inventory.structuredFindings, inventory.structuredErrors, inventory.structuredWarnings, inventory.structuredHints)
	}
	if inventory.errorCountManifests != 252 || !reflect.DeepEqual(inventory.errorCounts, map[int]int{0: 240, 1: 7, 2: 3, 3: 1, 4: 1}) {
		t.Fatalf("legacy error-count denominator changed: manifests=%d counts=%v", inventory.errorCountManifests, inventory.errorCounts)
	}
	if inventory.ruleCount != 21 || inventory.enabledRuleCount != 20 || inventory.disabledRuleCount != 1 {
		t.Fatalf("diagnostic policy denominator changed: rules=%d enabled=%d disabled=%d", inventory.ruleCount, inventory.enabledRuleCount, inventory.disabledRuleCount)
	}

	wantCodes := map[string]int{
		"advice.always_false_guard":          2,
		"advice.always_true_guard":           3,
		"advice.invariant_loop_read":         1,
		"advice.redundant_claim":             2,
		"advice.shape.polymorphic":           1,
		"advice.split_birth_discriminant":    1,
		"channel.close.closed":               1,
		"channel.select.exhaustiveness":      1,
		"channel.send.closed":                2,
		"effect.freeze.mutation":             5,
		"effect.lifecycle.unreleased":        5,
		"lint.claim.unproven":                2,
		"lint.condition.redundant":           4,
		"lint.dead.assignment":               2,
		"lint.union.exhaustiveness":          2,
		"lint.unused.local":                  1,
		"send.isolation":                     9,
		"type.assignment":                    47,
		"type.assignment.optional_target":    1,
		"type.call.direct.argument_type":     19,
		"type.call.direct.not_callable":      4,
		"type.call.direct.result_assignment": 1,
		"type.call.direct.too_few_args":      1,
		"type.call.optional_receiver":        1,
		"type.for.numeric_operand":           1,
		"type.member.missing":                6,
		"type.nil.unsafe_use":                5,
		"type.operator.concat_operand":       4,
		"type.reference.unresolved":          1,
		"type.return.contract":               2,
		"typestate.invalid_requirement":      2,
		"typestate.invalid_transition":       1,
		"typestate.unproven_requirement":     1,
		"value.reference.unresolved":         1,
	}
	if !reflect.DeepEqual(inventory.structuredCodes, wantCodes) {
		t.Fatalf("structured diagnostic code inventory changed:\n got %s\nwant %s", formatDiagnosticCounts(inventory.structuredCodes), formatDiagnosticCounts(wantCodes))
	}
	wantRuleCodes := map[string]int{
		"advice.always_false_guard":       2,
		"advice.always_true_guard":        2,
		"advice.invariant_loop_read":      1,
		"advice.redundant_claim":          1,
		"advice.shape.polymorphic":        1,
		"advice.split_birth_discriminant": 1,
		"effect.freeze.mutation":          1,
		"lint.condition.redundant":        3,
		"lint.dead.assignment":            1,
		"lint.union.exhaustiveness":       3,
		"lint.unused.local":               1,
		"send.isolation":                  3,
		"type.operator.concat_operand":    1,
	}
	if !reflect.DeepEqual(inventory.ruleCodes, wantRuleCodes) {
		t.Fatalf("diagnostic rule inventory changed:\n got %s\nwant %s", formatDiagnosticCounts(inventory.ruleCodes), formatDiagnosticCounts(wantRuleCodes))
	}
	indexedInline := 0
	for _, rows := range catalog.inlineByLocation {
		indexedInline += len(rows)
	}
	indexedStructured := 0
	for code, refs := range catalog.structuredByCode {
		indexedStructured += len(refs)
		for _, ref := range refs {
			project := catalog.byProject[ref.project]
			if project == nil || project.manifest == nil || project.manifest.Check == nil || ref.ordinal < 0 || ref.ordinal >= len(project.manifest.Check.Diagnostics) || project.manifest.Check.Diagnostics[ref.ordinal].Code != code {
				t.Fatalf("structured diagnostic index lost exact owner: code=%q project=%q ordinal=%d", code, ref.project, ref.ordinal)
			}
		}
	}
	indexedStructuredLocations := 0
	for key, refs := range catalog.structuredByLocation {
		indexedStructuredLocations += len(refs)
		if len(refs) != 1 {
			t.Fatalf("structured diagnostic anchor is ambiguous: project=%q code=%q file=%q line=%d severity=%q rows=%d", key.project, key.code, key.file, key.line, key.severity, len(refs))
		}
	}
	if len(catalog.projects) != 944 || indexedInline != 731 || len(catalog.structuredByCode) != 34 || indexedStructured != 142 || len(catalog.structuredByLocation) != 142 || indexedStructuredLocations != 142 {
		t.Fatalf("scalable diagnostic indexes changed: projects=%d inline=%d codes=%d structured=%d structured-anchors=%d/%d", len(catalog.projects), indexedInline, len(catalog.structuredByCode), indexedStructured, len(catalog.structuredByLocation), indexedStructuredLocations)
	}
}

// TestFrozenCorpusUnsupportedDiagnosticFamiliesRemainExplicit freezes the
// current policy vocabulary without pretending it is producer coverage. The
// final oracle must surface every row outside this set as unsupported/missing;
// it may never count an absent native producer as a clean fixture.
func TestFrozenCorpusUnsupportedDiagnosticFamiliesRemainExplicit(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := corpusDiagnosticRegistrationCensus(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !counts.matches(corpusDiagnosticFrozenRegistrationCensus) {
		t.Fatalf("official fixture registration census changed: got=%+v want=%+v", counts, corpusDiagnosticFrozenRegistrationCensus)
	}
}

// TestFrozenCorpusFullManifestContractInventory freezes every fixture contract
// that the complete oracle must eventually judge. A count here is not support:
// native and placement rows remain explicit requirements until current public
// projections exist and their owning laws pass.
func TestFrozenCorpusFullManifestContractInventory(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	got := catalog.inventory
	want := struct {
		declaredFileManifests, declaredFiles              int
		packageManifests, packageDeclarations             int
		stdlibManifests, renderOptionManifests            int
		nativeManifests, nativeFacts, nativeInvalidations int
		placementManifests                                int
	}{364, 630, 123, 141, 1, 8, 177, 764, 30, 51}
	if got.declaredFileManifests != want.declaredFileManifests || got.declaredFiles != want.declaredFiles ||
		got.packageManifests != want.packageManifests || got.packageDeclarations != want.packageDeclarations ||
		got.stdlibManifests != want.stdlibManifests || got.renderOptionManifests != want.renderOptionManifests ||
		got.nativeManifests != want.nativeManifests || got.nativeFacts != want.nativeFacts || got.nativeInvalidations != want.nativeInvalidations ||
		got.placementManifests != want.placementManifests {
		t.Fatalf("full manifest contract census changed: got=%+v want=%+v", got, want)
	}
	if len(catalog.nativeProjects) != want.nativeManifests || len(catalog.placementProjects) != want.placementManifests {
		t.Fatalf("manifest contract indexes changed: native=%d/%d placement=%d/%d", len(catalog.nativeProjects), want.nativeManifests, len(catalog.placementProjects), want.placementManifests)
	}
	if len(catalog.nativeFactRows) != want.nativeFacts || len(catalog.nativeInvalidationRows) != want.nativeInvalidations {
		t.Fatalf("native row indexes changed: facts=%d/%d invalidations=%d/%d", len(catalog.nativeFactRows), want.nativeFacts, len(catalog.nativeInvalidationRows), want.nativeInvalidations)
	}
	for _, ref := range catalog.nativeFactRows {
		contract := catalog.nativeProjects[ref.project]
		if contract == nil || ref.ordinal < 0 || ref.ordinal >= len(contract.Facts) {
			t.Fatalf("native fact index lost exact owner: %+v", ref)
		}
	}
	for _, ref := range catalog.nativeInvalidationRows {
		contract := catalog.nativeProjects[ref.project]
		if contract == nil || ref.ordinal < 0 || ref.ordinal >= len(contract.Invalidation) {
			t.Fatalf("native invalidation index lost exact owner: %+v", ref)
		}
	}
}

func TestFrozenCorpusManifestCatalogCanonicality(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	previous := ""
	for _, project := range catalog.projects {
		if project.name == "" || project.name <= previous || project.entryFile == "" || project.entryModule == "" {
			t.Fatalf("non-canonical project ordering or entry: previous=%q project=%+v", previous, project)
		}
		previous = project.name
		if !corpusDiagnosticProjectMatchesFile(project, project.entryFile, project.entryModule) || !corpusDiagnosticProjectMatchesFile(project, project.entryFile, "test.lua") {
			t.Fatalf("fixture %q lost entry aliases: file=%q module=%q", project.name, project.entryFile, project.entryModule)
		}
		if len(project.declaredFiles) != 0 {
			if project.entryFile != project.declaredFiles[len(project.declaredFiles)-1] {
				t.Fatalf("fixture %q lost declared entry order: entry=%q files=%v", project.name, project.entryFile, project.declaredFiles)
			}
			continue
		}
		if project.entryFile != "main.lua" && project.entryFile != project.files[len(project.files)-1] {
			t.Fatalf("fixture %q has noncanonical implicit entry %q from %v", project.name, project.entryFile, project.files)
		}
	}
}

func TestCorpusDiagnosticManifestSchemaRejectsUnknownOrDuplicateContracts(t *testing.T) {
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	tests := []struct {
		name string
		json string
	}{
		{"unknown top-level", `{"mystery":true}`},
		{"null root", `null`},
		{"unknown nested check", `{"check":{"mystery":true}}`},
		{"unknown native fact", `{"check":{"native":{"facts":[{"name":"x","min":1,"mystery":true}]}}}`},
		{"unknown placement", `{"check":{"placement":{"mystery":1}}}`},
		{"negative placement deep-frozen minimum", `{"check":{"placement":{"min_deep_frozen":-1}}}`},
		{"duplicate JSON key", `{"stdlib":true,"stdlib":false}`},
		{"duplicate file", `{"files":["main.lua","main.lua"]}`},
		{"duplicate package", `{"packages":["channel","channel"]}`},
		{"empty native block", `{"check":{"native":{}}}`},
		{"zero-only native block", `{"check":{"native":{"min_facts":0}}}`},
		{"negative native max facts", `{"check":{"native":{"max_facts":-1}}}`},
		{"native total bounds reversed", `{"check":{"native":{"min_facts":2,"max_facts":1}}}`},
		{"missing native bound", `{"check":{"native":{"facts":[{"name":"x"}]}}}`},
		{"missing native selector", `{"check":{"native":{"facts":[{"name":"x","max":0}]}}}`},
		{"positive native row without content", `{"check":{"native":{"facts":[{"family":"value","min":1}]}}}`},
		{"unknown native lane", `{"check":{"native":{"facts":[{"lane":"mystery","max":0}]}}}`},
		{"unknown native trust", `{"check":{"native":{"facts":[{"trust":"mystery","max":0}]}}}`},
		{"empty native contains", `{"check":{"native":{"facts":[{"key_contains":[],"max":0}]}}}`},
		{"empty native revocation", `{"check":{"native":{"facts":[{"value":"x","min":1,"revoked_by":[{}]}]}}}`},
		{"native revocation without positive min", `{"check":{"native":{"facts":[{"value":"x","max":1,"revoked_by":[{"event":"escape"}]}]}}}`},
		{"native exhaustive without revocation", `{"check":{"native":{"facts":[{"value":"x","max":1,"revoked_by_exhaustive":true}]}}}`},
		{"diagnostic missing file", `{"check":{"diagnostics":[{"line":1,"severity":"error","code":"x"}]}}`},
		{"diagnostic missing line", `{"check":{"diagnostics":[{"file":"main.lua","severity":"error","code":"x"}]}}`},
		{"diagnostic empty contains", `{"check":{"diagnostics":[{"file":"main.lua","line":1,"severity":"error","code":"x","message_contains":[],"render_contains":["x"],"help_contains":["x"],"label_contains":["x"],"allow_empty_evidence":true}]}}`},
		{"diagnostic invalid evidence kind", `{"check":{"diagnostics":[{"file":"main.lua","line":1,"severity":"error","code":"x","message_contains":["x"],"render_contains":["x"],"help_contains":["x"],"label_contains":["x"],"evidence":[{"kind":"mystery","contains":["x"]}]}]}}`},
		{"diagnostic invalid evidence trust", `{"check":{"diagnostics":[{"file":"main.lua","line":1,"severity":"error","code":"x","message_contains":["x"],"render_contains":["x"],"help_contains":["x"],"label_contains":["x"],"evidence":[{"trust":"mystery","contains":["x"]}]}]}}`},
		{"diagnostic invalid evidence reason", `{"check":{"diagnostics":[{"file":"main.lua","line":1,"severity":"error","code":"x","message_contains":["x"],"render_contains":["x"],"help_contains":["x"],"label_contains":["x"],"evidence":[{"reason":"mystery","contains":["x"]}]}]}}`},
		{"diagnostic missing positive evidence minimum", `{"check":{"diagnostics":[{"file":"main.lua","line":1,"severity":"error","code":"x","message_contains":["x"],"render_contains":["x"],"help_contains":["x"],"label_contains":["x"],"evidence_contains":["x"]}]}}`},
		{"negative error count", `{"check":{"errors":-1}}`},
		{"trailing JSON", `{} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := decodeCorpusDiagnosticManifest([]byte(test.json))
			if err == nil {
				err = validateCorpusDiagnosticManifest(compilation, manifest)
			}
			if err == nil {
				t.Fatalf("malformed manifest was admitted: %s", test.json)
			}
		})
	}
}

func TestCorpusDiagnosticManifestSchemaPreservesHistoricNativeGrammar(t *testing.T) {
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	manifest, err := decodeCorpusDiagnosticManifest([]byte(`{
		"files":["module.lua","main.lua"],
		"packages":["channel"],
		"stdlib":false,
		"check":{
			"diagnostic_rules":[{"code":"x.rule","severity":"warning"}],
			"render_options":{"witness_trace":true},
			"native":{
				"min_facts":0,"max_facts":0,
				"facts":[{"module":"module","term":"term-0","value":"","max":0}],
				"invalidation":[{"module":"module","term":"term-0","event":"escape","max":0}]
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCorpusDiagnosticManifest(compilation, manifest); err != nil {
		t.Fatal(err)
	}
	native := manifest.Check.Native
	if native == nil || native.MaxFacts == nil || *native.MaxFacts != 0 || len(native.Facts) != 1 || len(native.Invalidation) != 1 {
		t.Fatalf("native grammar was not preserved: %+v", native)
	}
	fact := native.Facts[0]
	if fact.Module != "module" || fact.Term != "term-0" || fact.Value == nil || *fact.Value != "" {
		t.Fatalf("native selector lost module/term/exact-empty value: %+v", fact)
	}
	rule := manifest.Check.DiagnosticRules[0]
	if rule.Enabled != nil || rule.Severity != "warning" {
		t.Fatalf("severity-only diagnostic rule changed: %+v", rule)
	}
}

// frozenCorpusDiagnosticExpectationInventory is process-cached test support.
// Producer-family laws and the eventual complete oracle share this one census;
// they must not rescan all 1,176 files for every diagnostic rule.
func frozenCorpusDiagnosticExpectationInventory(t *testing.T) (corpusDiagnosticExpectationInventory, error) {
	t.Helper()
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		return corpusDiagnosticExpectationInventory{}, err
	}
	return catalog.inventory, nil
}

func frozenCorpusDiagnosticExpectationCatalog(t *testing.T) (*corpusDiagnosticExpectationCatalog, error) {
	t.Helper()
	corpusDiagnosticCatalogOnce.Do(func() {
		compilation, compilationOK := composite.Build()
		if !compilationOK {
			corpusDiagnosticCatalogErr = fmt.Errorf("sealed composition unavailable")
			return
		}
		corpusDiagnosticCatalogValue, corpusDiagnosticCatalogErr = readCorpusDiagnosticExpectationCatalog(corpusDiagnosticFixtureRoot(t), compilation)
	})
	return corpusDiagnosticCatalogValue, corpusDiagnosticCatalogErr
}

func corpusDiagnosticFixtureRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve diagnostic inventory source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "testdata", "fixtures"))
}

func readCorpusDiagnosticExpectationCatalog(root string, compilation composite.Compilation) (*corpusDiagnosticExpectationCatalog, error) {
	catalog := &corpusDiagnosticExpectationCatalog{
		byProject:            make(map[string]*corpusDiagnosticProjectExpectations),
		inlineByLocation:     make(map[corpusDiagnosticLocationKey][]corpusInlineDiagnosticExpectationRow),
		structuredByCode:     make(map[string][]corpusStructuredDiagnosticRef),
		structuredByLocation: make(map[corpusStructuredDiagnosticLocationKey][]corpusStructuredDiagnosticRef),
		nativeProjects:       make(map[string]*corpusNativeContract),
		placementProjects:    make(map[string]*corpusPlacementContract),
	}
	inventory := &catalog.inventory
	*inventory = corpusDiagnosticExpectationInventory{
		structuredCodes: make(map[string]int),
		ruleCodes:       make(map[string]int),
		errorCounts:     make(map[int]int),
	}
	projectFor := func(directory string) (*corpusDiagnosticProjectExpectations, error) {
		relative, err := filepath.Rel(root, directory)
		if err != nil {
			return nil, err
		}
		name := filepath.ToSlash(relative)
		project := catalog.byProject[name]
		if project == nil {
			project = &corpusDiagnosticProjectExpectations{name: name, directory: directory}
			catalog.byProject[name] = project
		}
		return project, nil
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		switch entry.Name() {
		case "manifest.json":
			inventory.manifests++
			contents, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			manifest, err := decodeCorpusDiagnosticManifest(contents)
			if err != nil {
				return fmt.Errorf("decode %s: %w", path, err)
			}
			project, err := projectFor(filepath.Dir(path))
			if err != nil {
				return err
			}
			if project.manifest != nil {
				return fmt.Errorf("%s: duplicate fixture manifest", path)
			}
			project.manifest = manifest
			project.declaredFiles = append([]string(nil), manifest.Files...)
			if err := validateCorpusDiagnosticManifest(compilation, manifest); err != nil {
				return fmt.Errorf("validate %s: %w", path, err)
			}
			if manifest.Files != nil {
				inventory.declaredFileManifests++
				inventory.declaredFiles += len(manifest.Files)
			}
			if manifest.Packages != nil {
				inventory.packageManifests++
				inventory.packageDeclarations += len(manifest.Packages)
			}
			if manifest.Stdlib != nil {
				inventory.stdlibManifests++
			}
			if manifest.Check == nil {
				return nil
			}
			if manifest.Check.RenderOptions != nil {
				inventory.renderOptionManifests++
			}
			if manifest.Check.Native != nil {
				inventory.nativeManifests++
				inventory.nativeFacts += len(manifest.Check.Native.Facts)
				inventory.nativeInvalidations += len(manifest.Check.Native.Invalidation)
				catalog.nativeProjects[project.name] = manifest.Check.Native
				for ordinal := range manifest.Check.Native.Facts {
					catalog.nativeFactRows = append(catalog.nativeFactRows, corpusNativeContractRef{project: project.name, ordinal: ordinal})
				}
				for ordinal := range manifest.Check.Native.Invalidation {
					catalog.nativeInvalidationRows = append(catalog.nativeInvalidationRows, corpusNativeContractRef{project: project.name, ordinal: ordinal})
				}
			}
			if manifest.Check.Placement != nil {
				inventory.placementManifests++
				catalog.placementProjects[project.name] = manifest.Check.Placement
			}
			if manifest.Check.Errors != nil {
				inventory.errorCountManifests++
				inventory.errorCounts[*manifest.Check.Errors]++
			}
			if len(manifest.Check.Diagnostics) != 0 {
				inventory.structuredManifests++
			}
			for ordinal, row := range manifest.Check.Diagnostics {
				if row.Code == "" {
					return fmt.Errorf("%s: structured diagnostic has no code", path)
				}
				inventory.structuredFindings++
				inventory.structuredCodes[row.Code]++
				ref := corpusStructuredDiagnosticRef{project: project.name, ordinal: ordinal}
				catalog.structuredByCode[row.Code] = append(catalog.structuredByCode[row.Code], ref)
				key := corpusStructuredDiagnosticLocationKey{project: project.name, code: row.Code, file: row.File, line: row.Line, severity: row.Severity}
				catalog.structuredByLocation[key] = append(catalog.structuredByLocation[key], ref)
				vocabulary, vocabularyOK := composite.StructureVocabulary(compilation)
				ordinal, ordinalOK := uint16(0), false
				if vocabularyOK {
					ordinal, ordinalOK = vocabulary.Spelling(structure.CategoryDiagnosticSeverity, row.Severity)
				}
				if !ordinalOK {
					return fmt.Errorf("%s: structured diagnostic %q has invalid severity %q", path, row.Code, row.Severity)
				}
				switch anadiag.FindingSeverity(ordinal) {
				case anadiag.FindingSeverityError:
					inventory.structuredErrors++
				case anadiag.FindingSeverityWarning:
					inventory.structuredWarnings++
				case anadiag.FindingSeverityHint:
					inventory.structuredHints++
				default:
					return fmt.Errorf("%s: structured diagnostic %q has invalid severity %q", path, row.Code, row.Severity)
				}
			}
			for _, rule := range manifest.Check.DiagnosticRules {
				inventory.ruleCount++
				inventory.ruleCodes[rule.Code]++
				if rule.Enabled == nil {
					continue
				}
				if *rule.Enabled {
					inventory.enabledRuleCount++
				} else {
					inventory.disabledRuleCount++
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".lua" {
			return nil
		}
		inventory.luaFiles++
		project, err := projectFor(filepath.Dir(path))
		if err != nil {
			return err
		}
		project.files = append(project.files, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		annotated := false
		for lineIndex, line := range strings.Split(string(contents), "\n") {
			match := corpusInlineDiagnosticExpectation.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			annotated = true
			row := corpusInlineDiagnosticExpectationRow{File: entry.Name(), Line: lineIndex + 1, Severity: match[1], Contains: strings.TrimSpace(match[2])}
			project.inline = append(project.inline, row)
			key := corpusDiagnosticLocationKey{project: project.name, file: row.File, line: row.Line, severity: row.Severity}
			catalog.inlineByLocation[key] = append(catalog.inlineByLocation[key], row)
			inlineVocabulary, inlineVocabularyOK := composite.StructureVocabulary(compilation)
			inlineOrdinal, inlineOrdinalOK := uint16(0), false
			if inlineVocabularyOK {
				inlineOrdinal, inlineOrdinalOK = inlineVocabulary.Spelling(structure.CategoryDiagnosticSeverity, match[1])
			}
			if !inlineOrdinalOK {
				return fmt.Errorf("%s: invalid inline diagnostic severity %q", path, match[1])
			}
			switch anadiag.FindingSeverity(inlineOrdinal) {
			case anadiag.FindingSeverityError:
				inventory.inlineErrors++
			case anadiag.FindingSeverityWarning:
				inventory.inlineWarnings++
			default:
				return fmt.Errorf("%s: invalid inline diagnostic severity %q", path, match[1])
			}
		}
		if annotated {
			inventory.annotatedFiles++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk diagnostic corpus: %w", err)
	}
	for _, project := range catalog.byProject {
		if len(project.files) == 0 {
			continue
		}
		sort.Strings(project.files)
		if err := sealCorpusDiagnosticProjectManifest(project); err != nil {
			return nil, err
		}
		sort.Slice(project.inline, func(i, j int) bool {
			left, right := project.inline[i], project.inline[j]
			if left.File != right.File {
				return left.File < right.File
			}
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			return left.Severity < right.Severity
		})
		catalog.projects = append(catalog.projects, project)
	}
	for key, refs := range catalog.structuredByLocation {
		if len(refs) != 1 {
			return nil, fmt.Errorf("duplicate structured diagnostic anchor: project=%q code=%q file=%q line=%d severity=%q", key.project, key.code, key.file, key.line, key.severity)
		}
	}
	sort.Slice(catalog.projects, func(i, j int) bool { return catalog.projects[i].name < catalog.projects[j].name })
	inventory.projects = len(catalog.projects)
	return catalog, nil
}

// decodeCorpusDiagnosticManifest is intentionally strict. This catalog is the
// one frozen source of fixture contracts, so a typo must fail at admission
// rather than silently turn a semantic requirement into a clean fixture.
func decodeCorpusDiagnosticManifest(contents []byte) (*corpusDiagnosticManifest, error) {
	if err := rejectCorpusManifestDuplicateJSONKeys(contents); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var manifest corpusDiagnosticManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return &manifest, nil
}

func rejectCorpusManifestDuplicateJSONKeys(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return fmt.Errorf("manifest root must be a JSON object")
	}
	if err := validateCorpusManifestJSONValue(decoder, token); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values beginning with %v", token)
		}
		return err
	}
	return nil
}

func validateCorpusManifestJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[key] = struct{}{}
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := validateCorpusManifestJSONValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := validateCorpusManifestJSONValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func validateCorpusDiagnosticManifest(compilation composite.Compilation, manifest *corpusDiagnosticManifest) error {
	if manifest == nil {
		return fmt.Errorf("nil manifest")
	}
	if err := validateCorpusManifestStrings("files", manifest.Files, true); err != nil {
		return err
	}
	if err := validateCorpusManifestStrings("packages", manifest.Packages, false); err != nil {
		return err
	}
	if manifest.Check == nil {
		return nil
	}
	if manifest.Check.Errors != nil && *manifest.Check.Errors < 0 {
		return fmt.Errorf("check.errors must be non-negative")
	}
	seenRules := make(map[string]struct{}, len(manifest.Check.DiagnosticRules))
	for index, rule := range manifest.Check.DiagnosticRules {
		if strings.TrimSpace(rule.Code) == "" || rule.Enabled == nil && rule.Severity == "" {
			return fmt.Errorf("diagnostic_rules[%d] must declare code and enabled or severity", index)
		}
		if _, duplicate := seenRules[rule.Code]; duplicate {
			return fmt.Errorf("diagnostic_rules duplicates code %q", rule.Code)
		}
		seenRules[rule.Code] = struct{}{}
		if rule.Severity != "" && corpusDiagnosticSeverity(compilation, rule.Severity) == anadiag.FindingSeverityInvalid {
			return fmt.Errorf("diagnostic_rules[%d] has invalid severity %q", index, rule.Severity)
		}
	}
	for index, row := range manifest.Check.Diagnostics {
		if strings.TrimSpace(row.File) == "" || row.Line <= 0 {
			return fmt.Errorf("diagnostics[%d] must declare file and positive line", index)
		}
		if strings.TrimSpace(row.Code) == "" || corpusDiagnosticSeverity(compilation, row.Severity) == anadiag.FindingSeverityInvalid {
			return fmt.Errorf("diagnostics[%d] must declare code and valid severity", index)
		}
		if row.Column < 0 || row.MinEvidence < 0 || row.MinLabels < 0 {
			return fmt.Errorf("diagnostics[%d] has a negative location or minimum", index)
		}
		contains := []struct {
			name     string
			values   []string
			required bool
		}{
			{"message_contains", row.MessageContains, true},
			{"evidence_contains", row.EvidenceContains, !row.AllowEmptyEvidence && len(row.Evidence) == 0},
			{"render_contains", row.RenderContains, true},
			{"render_ordered_contains", row.RenderOrderedContains, false},
			{"render_not_contains", row.RenderNotContains, false},
			{"help_contains", row.HelpContains, true},
			{"label_contains", row.LabelContains, len(row.Labels) == 0},
		}
		for _, field := range contains {
			if err := validateCorpusContainsRequired(field.name, field.values, field.required); err != nil {
				return fmt.Errorf("diagnostics[%d]: %w", index, err)
			}
		}
		if row.Evidence != nil && len(row.Evidence) == 0 || row.Labels != nil && len(row.Labels) == 0 {
			return fmt.Errorf("diagnostics[%d] declares an empty evidence or labels list", index)
		}
		for evidenceIndex, evidence := range row.Evidence {
			if err := validateCorpusDiagnosticEvidence(evidence); err != nil {
				return fmt.Errorf("diagnostics[%d].evidence[%d]: %w", index, evidenceIndex, err)
			}
		}
		for labelIndex, label := range row.Labels {
			if err := validateCorpusDiagnosticLabel(label); err != nil {
				return fmt.Errorf("diagnostics[%d].labels[%d]: %w", index, labelIndex, err)
			}
		}
		if !row.AllowEmptyEvidence && row.MinEvidence <= 0 && len(row.Evidence) == 0 {
			return fmt.Errorf("diagnostics[%d].min_evidence must be positive unless allow_empty_evidence is true", index)
		}
	}
	if err := validateCorpusNativeContract(manifest.Check.Native); err != nil {
		return err
	}
	return validateCorpusPlacementContract(manifest.Check.Placement)
}

func validateCorpusDiagnosticEvidence(evidence corpusDiagnosticEvidenceExpectation) error {
	if err := validateCorpusContainsRequired("contains", evidence.Contains, true); err != nil {
		return err
	}
	if evidence.Line < 0 || evidence.Column < 0 {
		return fmt.Errorf("location must be non-negative")
	}
	if evidence.Kind != "" && evidence.Kind != "abstract fact" && evidence.Kind != "user assertion" && evidence.Kind != "missing proof" && evidence.Kind != "precision boundary" && evidence.Kind != "unvalidated value" {
		return fmt.Errorf("unknown kind %q", evidence.Kind)
	}
	if evidence.Trust != "" && evidence.Trust != "proven" && evidence.Trust != "claimed" && evidence.Trust != "refuted" && evidence.Trust != "unknown" {
		return fmt.Errorf("unknown trust %q", evidence.Trust)
	}
	if evidence.Reason != "" && evidence.Reason != "unspecified" && evidence.Reason != "boundary validation missing" && evidence.Reason != "index read validation missing" && evidence.Reason != "explicit boundary validation" {
		return fmt.Errorf("unknown reason %q", evidence.Reason)
	}
	return nil
}

func validateCorpusDiagnosticLabel(label corpusDiagnosticLabelExpectation) error {
	if err := validateCorpusContainsRequired("contains", label.Contains, true); err != nil {
		return err
	}
	if label.Line < 0 || label.Column < 0 {
		return fmt.Errorf("location must be non-negative")
	}
	return nil
}

func validateCorpusManifestStrings(field string, values []string, luaFiles bool) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value == "" {
			return fmt.Errorf("%s[%d] is empty", field, index)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s duplicates %q", field, value)
		}
		seen[value] = struct{}{}
		if luaFiles && (filepath.Base(value) != value || filepath.Ext(value) != ".lua") {
			return fmt.Errorf("files[%d] must name one local .lua file, got %q", index, value)
		}
	}
	return nil
}

func validateCorpusNativeContract(contract *corpusNativeContract) error {
	if contract == nil {
		return nil
	}
	minimumFacts := 0
	if contract.MinFacts != nil {
		minimumFacts = *contract.MinFacts
	}
	if len(contract.Facts) == 0 && len(contract.Invalidation) == 0 && minimumFacts == 0 && contract.MaxFacts == nil {
		return fmt.Errorf("native block asserts nothing")
	}
	if contract.MinFacts != nil && *contract.MinFacts < 0 {
		return fmt.Errorf("native.min_facts must be non-negative")
	}
	if contract.MaxFacts != nil && *contract.MaxFacts < 0 {
		return fmt.Errorf("native.max_facts must be non-negative")
	}
	if contract.MinFacts != nil && contract.MaxFacts != nil && *contract.MaxFacts < *contract.MinFacts {
		return fmt.Errorf("native.max_facts is below min_facts")
	}
	for index, fact := range contract.Facts {
		selector := fact.selector()
		if err := validateCorpusNativeSelector(selector); err != nil {
			return fmt.Errorf("native.facts[%d]: %w", index, err)
		}
		if err := validateCorpusCountRange(fmt.Sprintf("native.facts[%d]", index), fact.Min, fact.Max); err != nil {
			return err
		}
		minimum := 0
		if fact.Min != nil {
			minimum = *fact.Min
		}
		if minimum > 0 && !selector.assertsContent() {
			return fmt.Errorf("native.facts[%d]: positive min requires an exact key or value/trust assertion", index)
		}
		for revocationIndex, revocation := range fact.RevokedBy {
			if err := validateCorpusNativeRevocation(revocation); err != nil {
				return fmt.Errorf("native.facts[%d].revoked_by[%d]: %w", index, revocationIndex, err)
			}
		}
		if len(fact.RevokedBy) > 0 && minimum == 0 {
			return fmt.Errorf("native.facts[%d]: revoked_by requires positive min", index)
		}
		if fact.RevokedByExhaustive && len(fact.RevokedBy) == 0 {
			return fmt.Errorf("native.facts[%d]: revoked_by_exhaustive requires revoked_by", index)
		}
	}
	for index, invalidation := range contract.Invalidation {
		if err := validateCorpusNativeSelector(invalidation.selector()); err != nil {
			return fmt.Errorf("native.invalidation[%d]: %w", index, err)
		}
		if invalidation.Established != "" && invalidation.Revoked != "" && invalidation.Established == invalidation.Revoked {
			return fmt.Errorf("native.invalidation[%d]: revoked cannot equal established", index)
		}
		if err := validateCorpusCountRange(fmt.Sprintf("native.invalidation[%d]", index), invalidation.Min, invalidation.Max); err != nil {
			return err
		}
	}
	return nil
}

type corpusNativeSelector struct {
	Lane, Module, Family, Key, KeyPrefix, KeySuffix string
	KeyContains                                     []string
	Subject, Term, Occurrence                       string
	// Columns is the authored typed content of the selector, keyed by the
	// published column name. A value is the declared spelling of the member the
	// column must publish, or one of the two presence tokens below.
	Columns       map[string]string
	Value         *string
	ValuePrefix   string
	ValueContains []string
	Trust         string
}

// The two presence tokens a column selector may author instead of a member
// spelling. No declared vocabulary spells a member either way, so a token can
// never collide with a member.
const (
	corpusNativeColumnPresent = "present"
	corpusNativeColumnAbsent  = "absent"
)

// corpusNativePublishedFamilies are the publication families the analyzer
// declares typed columns for today. A contract selecting one of them states
// its content as typed columns; the rendered form is not addressable there.
var corpusNativePublishedFamilies = map[string]bool{
	"constant_value": true, "representation": true, "truthiness_class": true,
	"branch_partition": true, "scalar_operator": true, "divisor_property": true,
}

// corpusNativeColumnSelectors collects the authored typed columns of one
// contract. The map keys are the published column names, which are also the
// manifest field names, so a reader sees one vocabulary rather than two.
func corpusNativeColumnSelectors(columns map[string]string) map[string]string {
	authored := make(map[string]string, len(columns))
	for name, value := range columns {
		if value != "" {
			authored[name] = value
		}
	}
	if len(authored) == 0 {
		return nil
	}
	return authored
}

func (contract corpusNativeFactContract) selector() corpusNativeSelector {
	return corpusNativeSelector{
		Lane: contract.Lane, Module: contract.Module, Family: contract.Family,
		Key: contract.Key, KeyPrefix: contract.KeyPrefix, KeySuffix: contract.KeySuffix, KeyContains: contract.KeyContains,
		Subject: contract.Subject, Term: contract.Term, Occurrence: contract.Occurrence,
		Columns: corpusNativeColumnSelectors(map[string]string{
			"exact": contract.Exact, "literal": contract.Literal, "representation": contract.Representation,
			"left": contract.Left, "right": contract.Right, "operand": contract.Operand,
			"operator": contract.Operator, "overflow": contract.Overflow, "divisor": contract.Divisor,
			"truthiness": contract.Truthiness, "partition": contract.Partition,
			"dead_arm": contract.DeadArm, "dead_arm_reachable": contract.DeadArmReachable,
		}),
		Value: contract.Value, ValuePrefix: contract.ValuePrefix, ValueContains: contract.ValueContains, Trust: contract.Trust,
	}
}

func (contract corpusNativeInvalidationContract) selector() corpusNativeSelector {
	return corpusNativeSelector{
		Lane: contract.Lane, Module: contract.Module, Family: contract.Family,
		Key: contract.Key, KeyPrefix: contract.KeyPrefix, KeySuffix: contract.KeySuffix, KeyContains: contract.KeyContains,
		Subject: contract.Subject, Term: contract.Term, Occurrence: contract.Occurrence,
		Columns: corpusNativeColumnSelectors(map[string]string{
			"exact": contract.Exact, "literal": contract.Literal, "representation": contract.Representation,
			"left": contract.Left, "right": contract.Right, "operand": contract.Operand,
			"operator": contract.Operator, "overflow": contract.Overflow, "divisor": contract.Divisor,
			"truthiness": contract.Truthiness, "partition": contract.Partition,
			"dead_arm": contract.DeadArm, "dead_arm_reachable": contract.DeadArmReachable,
		}),
		Value: contract.Value, ValuePrefix: contract.ValuePrefix, ValueContains: contract.ValueContains, Trust: contract.Trust,
	}
}

func (selector corpusNativeSelector) selects() bool {
	return selector.Lane != "" || selector.Module != "" || selector.Family != "" || selector.Key != "" || selector.KeyPrefix != "" || selector.KeySuffix != "" ||
		len(selector.KeyContains) != 0 || selector.Subject != "" || selector.Term != "" || selector.Occurrence != "" || len(selector.Columns) != 0 ||
		selector.Value != nil || selector.ValuePrefix != "" || len(selector.ValueContains) != 0 || selector.Trust != ""
}

func (selector corpusNativeSelector) assertsContent() bool {
	return selector.Key != "" || len(selector.Columns) != 0 || selector.Value != nil || selector.ValuePrefix != "" || len(selector.ValueContains) != 0 || selector.Trust != ""
}

func validateCorpusNativeSelector(selector corpusNativeSelector) error {
	if !selector.selects() {
		return fmt.Errorf("at least one selector is required")
	}
	if selector.Lane != "" && selector.Lane != "values" && selector.Lane != "outcomes" && selector.Lane != "diagnostics" {
		return fmt.Errorf("unknown lane %q", selector.Lane)
	}
	if selector.Trust != "" && selector.Trust != "proven" && selector.Trust != "claimed" && selector.Trust != "unknown" {
		return fmt.Errorf("unknown trust %q", selector.Trust)
	}
	if err := validateCorpusContains("key_contains", selector.KeyContains); err != nil {
		return err
	}
	if err := validateCorpusNativeColumns(selector); err != nil {
		return err
	}
	return validateCorpusContains("value_contains", selector.ValueContains)
}

// validateCorpusNativeColumns states the manifest law for native content: a
// column is authored under its published name, and a family that publishes
// typed columns is asserted through them. The rendered form remains authorable
// only for a family the analyzer does not declare yet, so an expectation
// written against a published family can never depend on a spelling
// arrangement.
func validateCorpusNativeColumns(selector corpusNativeSelector) error {
	for name, value := range selector.Columns {
		if !corpusNativeColumnName(name) {
			return fmt.Errorf("unknown native column %q", name)
		}
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("native column %q is padded", name)
		}
	}
	if !corpusNativePublishedFamilies[selector.Family] {
		return nil
	}
	if selector.Value != nil || selector.ValuePrefix != "" || len(selector.ValueContains) != 0 {
		return fmt.Errorf("family %q publishes typed columns; assert them rather than the rendered form", selector.Family)
	}
	return nil
}

func corpusNativeColumnName(name string) bool {
	for _, declared := range corpusNativeColumnOrder {
		if declared == name {
			return true
		}
	}
	return false
}

func validateCorpusContains(field string, values []string) error {
	return validateCorpusContainsRequired(field, values, false)
}

func validateCorpusContainsRequired(field string, values []string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("%s must contain at least one assertion", field)
	}
	if values != nil && len(values) == 0 {
		return fmt.Errorf("%s declares an empty assertion list", field)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty assertion", field)
		}
	}
	return nil
}

func validateCorpusNativeRevocation(revocation corpusNativeRevocation) error {
	if revocation.Established == "" && revocation.Revoked == "" && revocation.Event == "" {
		return fmt.Errorf("at least one of established, revoked, or event is required")
	}
	if revocation.Established != "" && revocation.Revoked != "" && revocation.Established == revocation.Revoked {
		return fmt.Errorf("revoked cannot equal established")
	}
	return nil
}

func validateCorpusCountRange(field string, minimum, maximum *int) error {
	if minimum == nil && maximum == nil {
		return fmt.Errorf("%s must declare min or max", field)
	}
	if minimum != nil && *minimum < 0 || maximum != nil && *maximum < 0 {
		return fmt.Errorf("%s has a negative bound", field)
	}
	if minimum != nil && maximum != nil && *maximum < *minimum {
		return fmt.Errorf("%s has max below min", field)
	}
	return nil
}

func validateCorpusPlacementContract(contract *corpusPlacementContract) error {
	if contract == nil {
		return nil
	}
	minimums := []int{contract.MinStack, contract.MinOwnedHeap, contract.MinSharedHeap, contract.MinStackDepth, contract.MinOwnedHeapDepth, contract.MinSharedDepth, contract.MinOwnerIdentity, contract.MinAllocationSites, contract.MinFrameLocal, contract.MinRetainProvenPositions, contract.MinDiesBeforeSuspension, contract.MinDeepFrozen}
	for _, value := range minimums {
		if value < 0 {
			return fmt.Errorf("placement minimum is negative")
		}
	}
	maximums := []*int{contract.MaxStack, contract.MaxOwnedHeap, contract.MaxSharedHeap, contract.MaxNoFact, contract.MaxUnknown, contract.MaxFrameLocal, contract.MaxRetainProvenPositions, contract.MaxDiesBeforeSuspension, contract.MaxDeepFrozen}
	for _, value := range maximums {
		if value != nil && *value < 0 {
			return fmt.Errorf("placement maximum is negative")
		}
	}
	for label, counts := range map[string]map[string]int{
		"min_stack_kind": contract.MinStackKind, "min_owned_heap_kind": contract.MinOwnedHeapKind, "min_shared_heap_kind": contract.MinSharedHeapKind,
		"max_stack_kind": contract.MaxStackKind, "max_owned_heap_kind": contract.MaxOwnedHeapKind, "max_shared_heap_kind": contract.MaxSharedHeapKind,
	} {
		for kind, count := range counts {
			if kind == "" || count < 0 {
				return fmt.Errorf("placement.%s has invalid %q=%d", label, kind, count)
			}
		}
	}
	return nil
}

// sealCorpusDiagnosticProjectManifest records the exact input ordering the
// historic checker used while retaining the current catalog's single source
// walk. The last selected module is the entry; when files are not declared,
// legacy selection was sorted modules followed by main.lua when present.
func sealCorpusDiagnosticProjectManifest(project *corpusDiagnosticProjectExpectations) error {
	if project == nil || len(project.files) == 0 {
		return fmt.Errorf("fixture project has no Lua files")
	}
	if len(project.declaredFiles) != 0 {
		available := make(map[string]struct{}, len(project.files))
		for _, file := range project.files {
			available[file] = struct{}{}
		}
		for _, file := range project.declaredFiles {
			if _, found := available[file]; !found {
				return fmt.Errorf("fixture %q declares missing source file %q", project.name, file)
			}
		}
		project.entryFile = project.declaredFiles[len(project.declaredFiles)-1]
		project.entryModule = strings.TrimSuffix(project.entryFile, ".lua")
		return nil
	}
	project.entryFile = project.files[len(project.files)-1]
	for _, file := range project.files {
		if file == "main.lua" {
			project.entryFile = file
			break
		}
	}
	project.entryModule = strings.TrimSuffix(project.entryFile, ".lua")
	return nil
}

// corpusDiagnosticProjectMatchesFile preserves the historic checker aliases:
// a module diagnostic may name `module` for manifest file `module.lua`, while
// the selected entry may also be reported as `test.lua`.
func corpusDiagnosticProjectMatchesFile(project *corpusDiagnosticProjectExpectations, expected, actual string) bool {
	if expected == "" || expected == actual {
		return true
	}
	if project != nil && corpusDiagnosticProjectSourceFile(project, expected) == corpusDiagnosticProjectSourceFile(project, actual) {
		return true
	}
	expectedModule := strings.TrimSuffix(expected, ".lua")
	if actual == expectedModule {
		return true
	}
	return project != nil && actual == "test.lua" && (expected == project.entryFile || expectedModule == project.entryModule)
}

func corpusDiagnosticProjectSourceFile(project *corpusDiagnosticProjectExpectations, name string) string {
	if project == nil {
		return name
	}
	if name == "test.lua" {
		return project.entryFile
	}
	for _, file := range project.files {
		if name == strings.TrimSuffix(file, ".lua") {
			return file
		}
	}
	return name
}

func formatDiagnosticCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}
