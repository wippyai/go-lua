package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// corpusDiagnosticFamilyStatus is deliberately separate from AnalyzeStatus.
// A fixture expectation can be unsupported by the native report surface; that
// is not a clean analysis and must never be confused with a passed family.
type corpusDiagnosticFamilyStatus uint8

const (
	corpusDiagnosticFamilyInvalid corpusDiagnosticFamilyStatus = iota
	corpusDiagnosticFamilySupported
	corpusDiagnosticFamilyPassed
	corpusDiagnosticFamilyFailed
	corpusDiagnosticFamilyUnsupported
)

func (status corpusDiagnosticFamilyStatus) String() string {
	switch status {
	case corpusDiagnosticFamilySupported:
		return "supported"
	case corpusDiagnosticFamilyPassed:
		return "passed"
	case corpusDiagnosticFamilyFailed:
		return "failed"
	case corpusDiagnosticFamilyUnsupported:
		return "unsupported"
	default:
		return "invalid"
	}
}

type corpusDiagnosticFamilyResult struct {
	Project     string
	Code        string
	Status      corpusDiagnosticFamilyStatus
	CoreMatched bool
	Expected    int
	Actual      int
	Mismatches  []string
	Unsupported string
}

func unsupportedCorpusDiagnosticFamily(code string, expected int, reason string) corpusDiagnosticFamilyResult {
	return corpusDiagnosticFamilyResult{Code: code, Status: corpusDiagnosticFamilyUnsupported, Expected: expected, Unsupported: reason}
}

type corpusDiagnosticNativeFamilyRegistration struct {
	code    DiagnosticCode
	enabled []DiagnosticRule
	cases   []corpusDiagnosticNativeFamilyCase
}

type corpusDiagnosticNativeFamilyCase struct {
	project string
	expect  int
}

// corpusDiagnosticFixtureKey is the test oracle's exact fixture identity.
// Keep project and diagnostic code distinct rather than relying on a display
// separator that future fixture/code grammars could admit.
type corpusDiagnosticFixtureKey struct {
	project string
	code    string
}

func (key corpusDiagnosticFixtureKey) String() string { return key.project + "/" + key.code }

// corpusDiagnosticNativeFamilies is the test runner's closed installation
// fence. A producer is supported only after one registration supplies its
// public code, exact enabled policy, and the focused corpus cases that must
// pass. Fixture text and engine rules cannot widen this set implicitly.
var corpusDiagnosticNativeFamilies = [...]corpusDiagnosticNativeFamilyRegistration{
	{
		code:    DiagnosticCodeAlwaysTrueGuard,
		enabled: []DiagnosticRule{DiagnosticRuleAlwaysTrueGuard},
		cases:   []corpusDiagnosticNativeFamilyCase{{project: "advice/always-true-guard", expect: 1}},
	},
	{
		code:    DiagnosticCodeAlwaysFalseGuard,
		enabled: []DiagnosticRule{DiagnosticRuleAlwaysFalseGuard},
		cases: []corpusDiagnosticNativeFamilyCase{
			{project: "native/truthy-false-literal-is-falsy", expect: 1},
			{project: "native/branch-always-not-taken", expect: 1},
		},
	},
	{
		code:    DiagnosticCodeUnresolvedTypeReference,
		enabled: []DiagnosticRule{DiagnosticRuleUnresolvedTypeReference},
		cases:   []corpusDiagnosticNativeFamilyCase{{project: "semantic/unresolved-reference-diagnostics-evidence-chain", expect: 1}},
	},
	{
		code:    DiagnosticCodeUnresolvedValueReference,
		enabled: []DiagnosticRule{DiagnosticRuleUnresolvedValueReference},
		cases:   []corpusDiagnosticNativeFamilyCase{{project: "semantic/unresolved-reference-diagnostics-evidence-chain", expect: 1}},
	},
}

func corpusDiagnosticNativeFamilyRegistrationFor(code string) (corpusDiagnosticNativeFamilyRegistration, bool) {
	for _, family := range corpusDiagnosticNativeFamilies {
		if family.code.String() == code {
			return family, true
		}
	}
	return corpusDiagnosticNativeFamilyRegistration{}, false
}

func corpusDiagnosticNativeFamily(code string) bool {
	_, ok := corpusDiagnosticNativeFamilyRegistrationFor(code)
	return ok
}

// corpusDiagnosticNativeFixtureCaseRegistrationFor is deliberately stricter
// than code-level producer capability. A native producer may know a code while
// only selected fixture cases have been proven end-to-end. Other rows stay
// explicitly pending instead of becoming false-clean by code association.
func corpusDiagnosticNativeFixtureCaseRegistrationFor(project, code string) (corpusDiagnosticNativeFamilyRegistration, corpusDiagnosticNativeFamilyCase, bool) {
	family, familyOK := corpusDiagnosticNativeFamilyRegistrationFor(code)
	if !familyOK {
		return corpusDiagnosticNativeFamilyRegistration{}, corpusDiagnosticNativeFamilyCase{}, false
	}
	for _, fixtureCase := range family.cases {
		if fixtureCase.project == project {
			return family, fixtureCase, true
		}
	}
	return corpusDiagnosticNativeFamilyRegistration{}, corpusDiagnosticNativeFamilyCase{}, false
}

func corpusDiagnosticFixtureKeyFor(project, code string) corpusDiagnosticFixtureKey {
	return corpusDiagnosticFixtureKey{project: project, code: code}
}

type corpusDiagnosticSupportCounts struct {
	registeredCases, supportedFindings  int
	whollyPendingCodes, pendingFindings int
	inlinePending                       int
	pendingByFixture                    map[corpusDiagnosticFixtureKey]int
}

// corpusDiagnosticFrozenSupportCensus is the intentionally explicit current
// coverage boundary. Installing a family changes its closed registration and
// this one value; all census laws consume the same computed counts.
var corpusDiagnosticFrozenSupportCensus = corpusDiagnosticSupportCounts{
	registeredCases: 5, supportedFindings: 5,
	whollyPendingCodes: 30, pendingFindings: 128, inlinePending: 714,
}

func (counts corpusDiagnosticSupportCounts) matches(want corpusDiagnosticSupportCounts) bool {
	return counts.registeredCases == want.registeredCases &&
		counts.supportedFindings == want.supportedFindings &&
		counts.whollyPendingCodes == want.whollyPendingCodes &&
		counts.pendingFindings == want.pendingFindings &&
		counts.inlinePending == want.inlinePending
}

func corpusDiagnosticSupportCensus(catalog *corpusDiagnosticExpectationCatalog) (corpusDiagnosticSupportCounts, error) {
	counts := corpusDiagnosticSupportCounts{pendingByFixture: make(map[corpusDiagnosticFixtureKey]int)}
	registeredCases := make(map[corpusDiagnosticFixtureKey]struct{})
	for _, family := range corpusDiagnosticNativeFamilies {
		code := family.code.String()
		for _, fixtureCase := range family.cases {
			key := corpusDiagnosticFixtureKeyFor(fixtureCase.project, code)
			if _, duplicate := registeredCases[key]; duplicate {
				return corpusDiagnosticSupportCounts{}, fmt.Errorf("native diagnostic fixture registration duplicates %q", key)
			}
			if got := corpusDiagnosticProjectExpectedCount(catalog.byProject[fixtureCase.project], code); got != fixtureCase.expect {
				return corpusDiagnosticSupportCounts{}, fmt.Errorf("native diagnostic fixture registration %q expects %d rows, catalog has %d", key, fixtureCase.expect, got)
			}
			registeredCases[key] = struct{}{}
		}
	}
	counts.registeredCases = len(registeredCases)
	for code, refs := range catalog.structuredByCode {
		codeSupported := false
		for _, ref := range refs {
			if _, _, registered := corpusDiagnosticNativeFixtureCaseRegistrationFor(ref.project, code); registered {
				counts.supportedFindings++
				codeSupported = true
				continue
			}
			counts.pendingFindings++
			counts.pendingByFixture[corpusDiagnosticFixtureKeyFor(ref.project, code)]++
		}
		if !codeSupported {
			counts.whollyPendingCodes++
		}
	}
	counts.inlinePending = catalog.inventory.inlineErrors + catalog.inventory.inlineWarnings
	if counts.supportedFindings+counts.pendingFindings != catalog.inventory.structuredFindings {
		return corpusDiagnosticSupportCounts{}, fmt.Errorf("native/pending partition lost structured fixture rows: supported=%d pending=%d total=%d", counts.supportedFindings, counts.pendingFindings, catalog.inventory.structuredFindings)
	}
	return counts, nil
}

func corpusDiagnosticProjectExpectedCount(project *corpusDiagnosticProjectExpectations, code string) int {
	if project == nil || project.manifest == nil || project.manifest.Check == nil {
		return 0
	}
	count := 0
	for _, diagnostic := range project.manifest.Check.Diagnostics {
		if diagnostic.Code == code {
			count++
		}
	}
	return count
}

func corpusDiagnosticSeverity(value string) FindingSeverity {
	switch value {
	case "error":
		return FindingSeverityError
	case "warning":
		return FindingSeverityWarning
	case "hint":
		return FindingSeverityHint
	default:
		return FindingSeverityInvalid
	}
}

func corpusDiagnosticOrderedContains(rendered string, parts []string) bool {
	offset := 0
	for _, part := range parts {
		index := strings.Index(rendered[offset:], part)
		if index < 0 {
			return false
		}
		offset += index + len(part)
	}
	return true
}

func corpusManifestDiagnosticEvidenceMatches(project *corpusDiagnosticProjectExpectations, want corpusDiagnosticEvidenceExpectation, findingFile string, evidence DiagnosticEvidence) bool {
	location, ok := evidence.Location()
	if !ok {
		return false
	}
	line, column := location.Start()
	expectedFile := want.File
	if expectedFile == "" {
		expectedFile = findingFile
	}
	if !corpusDiagnosticProjectMatchesFile(project, expectedFile, location.File()) {
		return false
	}
	if want.Line != 0 && uint32(want.Line) != line || want.Column != 0 && uint32(want.Column) != column {
		return false
	}
	if want.Kind != "" && want.Kind != evidence.Kind() || want.Trust != "" && want.Trust != evidence.Trust() || want.Reason != "" && want.Reason != evidence.Reason() {
		return false
	}
	for _, part := range want.Contains {
		if !strings.Contains(evidence.Detail(), part) {
			return false
		}
	}
	return true
}

func corpusDiagnosticLabelMatches(project *corpusDiagnosticProjectExpectations, want corpusDiagnosticLabelExpectation, findingFile string, label DiagnosticLabel) bool {
	location, ok := label.Location()
	if !ok {
		return false
	}
	line, column := location.Start()
	expectedFile := want.File
	if expectedFile == "" {
		expectedFile = findingFile
	}
	if !corpusDiagnosticProjectMatchesFile(project, expectedFile, location.File()) {
		return false
	}
	if want.Line != 0 && uint32(want.Line) != line || want.Column != 0 && uint32(want.Column) != column {
		return false
	}
	for _, part := range want.Contains {
		if !strings.Contains(label.Text(), part) {
			return false
		}
	}
	return true
}

func matchCorpusDiagnosticDetails(result *corpusDiagnosticFamilyResult, want corpusStructuredDiagnosticExpectation, finding Finding, findingFile string, sourceText func(string) (string, bool)) {
	matchCorpusDiagnosticDetailsForProject(result, nil, want, finding, findingFile, sourceText)
}

func matchCorpusDiagnosticDetailsForProject(result *corpusDiagnosticFamilyResult, project *corpusDiagnosticProjectExpectations, want corpusStructuredDiagnosticExpectation, finding Finding, findingFile string, sourceText func(string) (string, bool)) {
	actualEvidence := make([]DiagnosticEvidence, 0)
	for index := 0; ; index++ {
		evidence, ok := finding.EvidenceAt(index)
		if !ok {
			break
		}
		actualEvidence = append(actualEvidence, evidence)
	}
	if len(want.Evidence) != 0 {
		if len(actualEvidence) != len(want.Evidence) {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("evidence at %s has %d rows, want %d", findingFile, len(actualEvidence), len(want.Evidence)))
		}
	} else if len(actualEvidence) < want.MinEvidence {
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("evidence at %s has %d rows, want at least %d", findingFile, len(actualEvidence), want.MinEvidence))
	}
	usedEvidence := make([]bool, len(actualEvidence))
	for expectedIndex, expectedEvidence := range want.Evidence {
		matched := -1
		for index, evidence := range actualEvidence {
			if !usedEvidence[index] && corpusManifestDiagnosticEvidenceMatches(project, expectedEvidence, findingFile, evidence) {
				matched = index
				break
			}
		}
		if matched < 0 {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("missing evidence row %d at %s", expectedIndex, findingFile))
			continue
		}
		usedEvidence[matched] = true
	}
	for _, part := range want.EvidenceContains {
		found := false
		for _, evidence := range actualEvidence {
			if strings.Contains(evidence.Detail(), part) {
				found = true
				break
			}
		}
		if !found {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("evidence at %s missing %q", findingFile, part))
		}
	}
	if len(want.Evidence) != 0 {
		for index := range actualEvidence {
			if !usedEvidence[index] {
				result.Mismatches = append(result.Mismatches, fmt.Sprintf("unexpected evidence row at %s", findingFile))
			}
		}
	}

	actualLabels := make([]DiagnosticLabel, 0)
	for index := 0; ; index++ {
		label, ok := finding.LabelAt(index)
		if !ok {
			break
		}
		actualLabels = append(actualLabels, label)
	}
	if len(want.Labels) != 0 {
		if len(actualLabels) != len(want.Labels) {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("labels at %s has %d rows, want %d", findingFile, len(actualLabels), len(want.Labels)))
		}
	} else if len(actualLabels) < want.MinLabels {
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("labels at %s has %d rows, want at least %d", findingFile, len(actualLabels), want.MinLabels))
	}
	usedLabels := make([]bool, len(actualLabels))
	for expectedIndex, expectedLabel := range want.Labels {
		matched := -1
		for index, label := range actualLabels {
			if !usedLabels[index] && corpusDiagnosticLabelMatches(project, expectedLabel, findingFile, label) {
				matched = index
				break
			}
		}
		if matched < 0 {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("missing label row %d at %s", expectedIndex, findingFile))
			continue
		}
		usedLabels[matched] = true
	}
	for _, part := range want.LabelContains {
		found := false
		for _, label := range actualLabels {
			if strings.Contains(label.Text(), part) {
				found = true
				break
			}
		}
		if !found {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("labels at %s missing %q", findingFile, part))
		}
	}
	if len(want.Labels) != 0 {
		for index := range actualLabels {
			if !usedLabels[index] {
				result.Mismatches = append(result.Mismatches, fmt.Sprintf("unexpected label row at %s", findingFile))
			}
		}
	}

	if len(want.RenderContains) == 0 && len(want.RenderOrderedContains) == 0 && len(want.RenderNotContains) == 0 {
		return
	}
	text, ok := sourceText(findingFile)
	if !ok {
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("source unavailable for render at %s", findingFile))
		return
	}
	rendered, ok := finding.RenderSource(findingFile, text)
	if !ok {
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("source render unavailable at %s", findingFile))
		return
	}
	for _, part := range want.RenderContains {
		if !strings.Contains(rendered, part) {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("render at %s missing %q", findingFile, part))
		}
	}
	if !corpusDiagnosticOrderedContains(rendered, want.RenderOrderedContains) {
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("render at %s failed ordered contains", findingFile))
	}
	for _, expression := range want.RenderNotContains {
		if strings.Contains(rendered, expression) {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("render at %s unexpectedly contains %q", findingFile, expression))
		}
	}
}

// matchCorpusDiagnosticFamily compares only the public detached report. The
// manifest remains an expectation source, never an input to Plan/Link or the
// inference pipeline. Evidence/render/label fields are intentionally not
// guessed here because they are not part of DiagnosticReport's public API.
func matchCorpusDiagnosticFamily(project string, expectation *corpusDiagnosticProjectExpectations, report *DiagnosticReport, code string, sourceText func(string) (string, bool)) corpusDiagnosticFamilyResult {
	result := corpusDiagnosticFamilyResult{Project: project, Code: code}
	if expectation == nil || expectation.manifest == nil || expectation.manifest.Check == nil {
		result.Status = corpusDiagnosticFamilyUnsupported
		result.Unsupported = "fixture has no structured diagnostic expectation"
		return result
	}
	if !corpusDiagnosticNativeFamily(code) {
		result.Status = corpusDiagnosticFamilyUnsupported
		result.Unsupported = "no native DiagnosticReport producer"
		for _, diagnostic := range expectation.manifest.Check.Diagnostics {
			if diagnostic.Code == code {
				result.Expected++
			}
		}
		return result
	}

	expected := make([]corpusStructuredDiagnosticExpectation, 0)
	for _, diagnostic := range expectation.manifest.Check.Diagnostics {
		if diagnostic.Code == code {
			expected = append(expected, diagnostic)
		}
	}
	result.Status = corpusDiagnosticFamilySupported
	result.Expected = len(expected)
	if report == nil || !report.Available() {
		result.Status = corpusDiagnosticFamilyFailed
		result.Mismatches = append(result.Mismatches, "DiagnosticReport unavailable")
		return result
	}
	if failure := report.CollectionFailure(); failure != DiagnosticCollectionOK {
		result.Status = corpusDiagnosticFamilyFailed
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("collection failure %d", failure))
		return result
	}

	type actualRow struct {
		index         int
		finding       Finding
		file          string
		line, column  uint32
		severity      FindingSeverity
		message, help string
	}
	actual := make([]actualRow, 0)
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		if !findingOK || finding.Code().String() != code {
			continue
		}
		location, locationOK := finding.Location()
		line, column := location.Start()
		if !locationOK {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("finding %d has no public location", index))
			continue
		}
		actual = append(actual, actualRow{index: index, finding: finding, file: location.File(), line: line, column: column, severity: finding.Severity(), message: finding.Message(), help: finding.Help()})
	}
	result.Actual = len(actual)
	used := make([]bool, len(actual))
	for expectedIndex, want := range expected {
		matched := -1
		for index, got := range actual {
			if used[index] || !corpusDiagnosticProjectMatchesFile(expectation, want.File, got.file) || got.line != uint32(want.Line) || got.severity != corpusDiagnosticSeverity(want.Severity) {
				continue
			}
			if want.Column != 0 && got.column != uint32(want.Column) {
				continue
			}
			matched = index
			break
		}
		if matched < 0 {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("missing expected row %d at %s:%d:%d", expectedIndex, want.File, want.Line, want.Column))
			continue
		}
		used[matched] = true
		got := actual[matched]
		for _, part := range want.MessageContains {
			if !strings.Contains(got.message, part) {
				result.Mismatches = append(result.Mismatches, fmt.Sprintf("message at %s:%d missing %q", got.file, got.line, part))
			}
		}
		for _, part := range want.HelpContains {
			if !strings.Contains(got.help, part) {
				result.Mismatches = append(result.Mismatches, fmt.Sprintf("help at %s:%d missing %q", got.file, got.line, part))
			}
		}
		matchCorpusDiagnosticDetailsForProject(&result, expectation, want, got.finding, got.file, sourceText)
	}
	for index, got := range actual {
		if !used[index] {
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("unexpected row at %s:%d:%d", got.file, got.line, got.column))
		}
	}
	if len(result.Mismatches) != 0 {
		result.Status = corpusDiagnosticFamilyFailed
		return result
	}
	result.CoreMatched = true
	result.Status = corpusDiagnosticFamilyPassed
	return result
}

func runCorpusDiagnosticFamily(t *testing.T, projectName, code string) corpusDiagnosticFamilyResult {
	t.Helper()
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	project := catalog.byProject[projectName]
	if project == nil || project.manifest == nil || project.manifest.Check == nil {
		return corpusDiagnosticFamilyResult{Project: projectName, Code: code, Status: corpusDiagnosticFamilyUnsupported, Unsupported: "fixture metadata absent"}
	}
	family, fixtureCase, registered := corpusDiagnosticNativeFixtureCaseRegistrationFor(projectName, code)
	if !registered {
		return unsupportedCorpusDiagnosticFamily(code, corpusDiagnosticProjectExpectedCount(project, code), "no officially verified native DiagnosticReport fixture case")
	}
	if expected := corpusDiagnosticProjectExpectedCount(project, code); expected != fixtureCase.expect {
		t.Fatalf("native fixture registration %s/%s expects %d rows, catalog has %d", projectName, code, fixtureCase.expect, expected)
	}
	plan, _, _, _ := testCorpusReceiptLaw(t, projectName)
	_, report, status, diagnostics := plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{Enabled: family.enabled})
	if status != AnalyzeComplete {
		t.Fatalf("manifest runner solve %s = %v diagnostics=%+v", projectName, status, diagnostics)
	}
	return matchCorpusDiagnosticFamily(projectName, project, report, code, func(file string) (string, bool) {
		file = corpusDiagnosticProjectSourceFile(project, file)
		contents, err := os.ReadFile(filepath.Join(project.directory, filepath.FromSlash(file)))
		if err != nil {
			return "", false
		}
		return string(contents), true
	})
}

func TestCorpusDiagnosticManifestRunnerAlwaysTrueFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, DiagnosticCodeAlwaysTrueGuard.String())
}

func TestCorpusDiagnosticManifestRunnerAlwaysFalseFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, DiagnosticCodeAlwaysFalseGuard.String())
}

func TestCorpusDiagnosticManifestRunnerUnresolvedTypeReferenceFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, DiagnosticCodeUnresolvedTypeReference.String())
}

func TestCorpusDiagnosticManifestRunnerUnresolvedValueReferenceFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, DiagnosticCodeUnresolvedValueReference.String())
}

// TestCorpusDiagnosticManifestRunnerNativeFamiliesLaw makes every installed
// producer prove its own narrow corpus cases. New native support is one table
// registration, and may be run alone with -run
// '^TestCorpusDiagnosticManifestRunnerNativeFamiliesLaw/<code>$'.
func TestCorpusDiagnosticManifestRunnerNativeFamiliesLaw(t *testing.T) {
	seenCodes := make(map[string]struct{}, len(corpusDiagnosticNativeFamilies))
	seenRules := make(map[DiagnosticRule]struct{})
	for _, family := range corpusDiagnosticNativeFamilies {
		code := family.code.String()
		if family.code == DiagnosticCodeInvalid || code == "" || len(family.enabled) == 0 || len(family.cases) == 0 {
			t.Fatalf("native family registration is incomplete: %+v", family)
		}
		if _, duplicate := seenCodes[code]; duplicate {
			t.Fatalf("duplicate native diagnostic code %q", code)
		}
		seenCodes[code] = struct{}{}
		for _, rule := range family.enabled {
			if rule == DiagnosticRuleInvalid || rule.Code() != family.code {
				t.Fatalf("native family %q has non-owning policy rule %q", code, rule.Code().String())
			}
			if _, duplicate := seenRules[rule]; duplicate {
				t.Fatalf("native policy rule %q is registered twice", rule.Code().String())
			}
			seenRules[rule] = struct{}{}
		}
		t.Run(code, func(t *testing.T) { runCorpusDiagnosticNativeFamilyLaw(t, code) })
	}
}

func runCorpusDiagnosticNativeFamilyLaw(t *testing.T, code string) {
	t.Helper()
	family, native := corpusDiagnosticNativeFamilyRegistrationFor(code)
	if !native {
		t.Fatalf("native family %q is not registered", code)
	}
	for _, test := range family.cases {
		got := runCorpusDiagnosticFamily(t, test.project, code)
		if got.Status != corpusDiagnosticFamilyPassed || !got.CoreMatched || got.Expected != test.expect || got.Actual != test.expect || len(got.Mismatches) != 0 || got.Unsupported != "" {
			t.Fatalf("manifest family %s = status:%s core:%t expected:%d actual:%d mismatches:%v unsupported:%q, want passed/%d", test.project, got.Status, got.CoreMatched, got.Expected, got.Actual, got.Mismatches, got.Unsupported, test.expect)
		}
	}
}

func TestCorpusDiagnosticManifestRunnerUnregisteredFamilyLaw(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	codes := make([]string, 0, len(catalog.structuredByCode))
	for code := range catalog.structuredByCode {
		if !corpusDiagnosticNativeFamily(code) {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		t.Skip("every structured fixture family has a native report producer")
	}
	sort.Strings(codes)
	code := codes[0]
	refs := catalog.structuredByCode[code]
	got := runCorpusDiagnosticFamily(t, refs[0].project, code)
	if got.Status != corpusDiagnosticFamilyUnsupported || got.CoreMatched || got.Expected != corpusDiagnosticProjectExpectedCount(catalog.byProject[refs[0].project], code) || got.Actual != 0 || got.Unsupported != "no officially verified native DiagnosticReport fixture case" || len(got.Mismatches) != 0 {
		t.Fatalf("unregistered family became false-clean: %+v", got)
	}
}

func TestCorpusDiagnosticManifestRunnerInstalledCodePendingProjectLaw(t *testing.T) {
	const project = "advice/redundant-guard"
	code := DiagnosticCodeAlwaysTrueGuard.String()
	if !corpusDiagnosticNativeFamily(code) {
		t.Fatalf("test requires installed code-level producer %q", code)
	}
	if _, _, registered := corpusDiagnosticNativeFixtureCaseRegistrationFor(project, code); registered {
		t.Fatalf("unverified fixture %s/%s entered native support", project, code)
	}
	got := runCorpusDiagnosticFamily(t, project, code)
	if got.Status != corpusDiagnosticFamilyUnsupported || got.CoreMatched || got.Expected != 2 || got.Actual != 0 || got.Unsupported != "no officially verified native DiagnosticReport fixture case" || len(got.Mismatches) != 0 {
		t.Fatalf("installed code made unverified project false-clean: %+v", got)
	}
}

func corpusDiagnosticManifestRunnerID(seed byte) keyspace.ContentID {
	var id keyspace.ContentID
	id[0] = seed
	return id
}

func TestCorpusDiagnosticManifestRunnerUnexpectedFindingLaw(t *testing.T) {
	location, locationOK := newDiagnosticLocation("main.lua", 2, 1, 2, 3)
	if !locationOK {
		t.Fatal("synthetic finding location unavailable")
	}
	row := diagnosticFinding{id: corpusDiagnosticManifestRunnerID(3), subject: corpusDiagnosticManifestRunnerID(4), code: DiagnosticCodeAlwaysTrueGuard, severity: FindingSeverityHint, location: location}
	report := &DiagnosticReport{source: corpusDiagnosticManifestRunnerID(1), result: corpusDiagnosticManifestRunnerID(2), findings: []diagnosticFinding{row, row}, sealed: true}
	project := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{Check: &corpusDiagnosticManifestCheck{
		Diagnostics: []corpusStructuredDiagnosticExpectation{{File: "main.lua", Line: 2, Column: 1, Severity: "hint", Code: DiagnosticCodeAlwaysTrueGuard.String()}},
	}}}
	got := matchCorpusDiagnosticFamily("synthetic", project, report, DiagnosticCodeAlwaysTrueGuard.String(), func(string) (string, bool) { return "\nif true then end\n", true })
	if got.Status != corpusDiagnosticFamilyFailed || got.CoreMatched || got.Expected != 1 || got.Actual != 2 || len(got.Mismatches) != 1 || !strings.Contains(got.Mismatches[0], "unexpected row") {
		t.Fatalf("extra native finding escaped one-to-one matching: %+v", got)
	}
}

func TestCorpusDiagnosticManifestRunnerUnsupportedFamilyLaw(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := corpusDiagnosticSupportCensus(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for fixture, expected := range counts.pendingByFixture {
		status := unsupportedCorpusDiagnosticFamily(fixture.String(), expected, "no officially verified native DiagnosticReport fixture case")
		if status.Status != corpusDiagnosticFamilyUnsupported {
			t.Fatalf("pending fixture %q reported %s", fixture, status.Status)
		}
	}
	if !counts.matches(corpusDiagnosticFrozenSupportCensus) {
		t.Fatalf("official fixture support census changed: got=%+v want=%+v", counts, corpusDiagnosticFrozenSupportCensus)
	}
	inlineUnsupported := unsupportedCorpusDiagnosticFamily("inline-no-code", counts.inlinePending, "inline markers have no structured DiagnosticReport code")
	if inlineUnsupported.Status != corpusDiagnosticFamilyUnsupported || inlineUnsupported.Expected != corpusDiagnosticFrozenSupportCensus.inlinePending {
		t.Fatalf("inline unsupported census = %+v, want %d explicit unsupported", inlineUnsupported, corpusDiagnosticFrozenSupportCensus.inlinePending)
	}
}

func TestCorpusDiagnosticManifestRunnerCensusPathLaw(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.inventory.projects != 911 || catalog.inventory.structuredFindings != 133 || catalog.inventory.inlineErrors+catalog.inventory.inlineWarnings != 714 {
		t.Fatalf("census = projects:%d structured:%d inline:%d", catalog.inventory.projects, catalog.inventory.structuredFindings, catalog.inventory.inlineErrors+catalog.inventory.inlineWarnings)
	}
	// Keep this path cheap: it must not compile or solve any unsupported family.
	if len(catalog.structuredByCode) != 34 {
		t.Fatalf("census code families=%d, want 34", len(catalog.structuredByCode))
	}
	keys := make([]string, 0, len(catalog.structuredByCode))
	for code := range catalog.structuredByCode {
		keys = append(keys, code)
	}
	sort.Strings(keys)
	if keys[0] == "" || keys[len(keys)-1] == "" {
		t.Fatal("census contains empty diagnostic family")
	}
}
