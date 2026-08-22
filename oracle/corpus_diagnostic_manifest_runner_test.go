package oracle

import (
	"context"
	"fmt"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
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
	code    anadiag.DiagnosticCode
	enabled []anadiag.DiagnosticCode
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
		code:    anadiag.DiagnosticCodeAlwaysTrueGuard,
		enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeAlwaysTrueGuard},
		cases:   []corpusDiagnosticNativeFamilyCase{{project: "advice/always-true-guard", expect: 1}},
	},
	{
		code:    anadiag.DiagnosticCodeAlwaysFalseGuard,
		enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeAlwaysFalseGuard},
		cases: []corpusDiagnosticNativeFamilyCase{
			{project: "native/truthy-false-literal-is-falsy", expect: 1},
			{project: "native/branch-always-not-taken", expect: 1},
		},
	},
	{
		code:    anadiag.DiagnosticCodeUnresolvedTypeReference,
		enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeUnresolvedTypeReference},
		cases:   []corpusDiagnosticNativeFamilyCase{{project: "semantic/unresolved-reference-diagnostics-evidence-chain", expect: 1}},
	},
	{
		code:    anadiag.DiagnosticCodeUnresolvedValueReference,
		enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeUnresolvedValueReference},
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
	whollyPendingCodes: 30, pendingFindings: 135, inlinePending: 731,
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
	for _, row := range project.manifest.Check.Diagnostics {
		if row.Code == code {
			count++
		}
	}
	return count
}

func corpusDiagnosticSeverity(compilation composite.Compilation, value string) anadiag.FindingSeverity {
	vocabulary, vocabularyOK := composite.StructureVocabulary(compilation)
	if !vocabularyOK {
		return anadiag.FindingSeverityInvalid
	}
	ordinal, ok := vocabulary.Spelling(structure.CategoryDiagnosticSeverity, value)
	if !ok {
		return anadiag.FindingSeverityInvalid
	}
	return anadiag.FindingSeverity(ordinal)
}

// corpusDiagnosticSeveritySpelling names one published severity as the sealed
// structure vocabulary spells it. The spelling is read from that vocabulary
// rather than restated here, so a fixture's inline marker and a report row are
// compared through the analyzer's own naming.
func corpusDiagnosticSeveritySpelling(compilation composite.Compilation, severity anadiag.FindingSeverity) (string, bool) {
	vocabulary, vocabularyOK := composite.StructureVocabulary(compilation)
	if !vocabularyOK || !severity.Available() {
		return "", false
	}
	member, memberOK := vocabulary.At(structure.CategoryDiagnosticSeverity, severity.Ordinal())
	if !memberOK {
		return "", false
	}
	return member.Spelling(), true
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

func corpusManifestDiagnosticEvidenceMatches(project *corpusDiagnosticProjectExpectations, want corpusDiagnosticEvidenceExpectation, findingFile string, evidence anadiag.DiagnosticEvidence) bool {
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

func corpusDiagnosticLabelMatches(project *corpusDiagnosticProjectExpectations, want corpusDiagnosticLabelExpectation, findingFile string, label anadiag.DiagnosticLabel) bool {
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

func matchCorpusDiagnosticDetails(result *corpusDiagnosticFamilyResult, want corpusStructuredDiagnosticExpectation, finding anadiag.Finding, findingFile string, sourceText func(string) (string, bool)) {
	matchCorpusDiagnosticDetailsForProject(result, nil, want, finding, findingFile, sourceText)
}

func matchCorpusDiagnosticDetailsForProject(result *corpusDiagnosticFamilyResult, project *corpusDiagnosticProjectExpectations, want corpusStructuredDiagnosticExpectation, finding anadiag.Finding, findingFile string, sourceText func(string) (string, bool)) {
	actualEvidence := make([]anadiag.DiagnosticEvidence, 0)
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

	actualLabels := make([]anadiag.DiagnosticLabel, 0)
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
func matchCorpusDiagnosticFamily(compilation composite.Compilation, project string, expectation *corpusDiagnosticProjectExpectations, report *anadiag.DiagnosticReport, code string, sourceText func(string) (string, bool)) corpusDiagnosticFamilyResult {
	result := corpusDiagnosticFamilyResult{Project: project, Code: code}
	if expectation == nil || expectation.manifest == nil || expectation.manifest.Check == nil {
		result.Status = corpusDiagnosticFamilyUnsupported
		result.Unsupported = "fixture has no structured diagnostic expectation"
		return result
	}
	if !corpusDiagnosticNativeFamily(code) {
		result.Status = corpusDiagnosticFamilyUnsupported
		result.Unsupported = "no native DiagnosticReport producer"
		for _, row := range expectation.manifest.Check.Diagnostics {
			if row.Code == code {
				result.Expected++
			}
		}
		return result
	}

	expected := make([]corpusStructuredDiagnosticExpectation, 0)
	for _, row := range expectation.manifest.Check.Diagnostics {
		if row.Code == code {
			expected = append(expected, row)
		}
	}
	result.Status = corpusDiagnosticFamilySupported
	result.Expected = len(expected)
	if report == nil || !report.Available() {
		result.Status = corpusDiagnosticFamilyFailed
		result.Mismatches = append(result.Mismatches, "DiagnosticReport unavailable")
		return result
	}
	if failure := report.CollectionFailure(); failure != anadiag.DiagnosticCollectionOK {
		result.Status = corpusDiagnosticFamilyFailed
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("collection failure %d", failure))
		return result
	}

	actual := corpusDiagnosticActualRows(&result, report, code)
	return judgeCorpusDiagnosticRows(compilation, result, expectation, expected, actual, sourceText)
}

// corpusDiagnosticActualRow is one published report row of the family under
// judgment, reduced to the identity the matcher selects on.
type corpusDiagnosticActualRow struct {
	index         int
	finding       anadiag.Finding
	file          string
	line, column  uint32
	severity      anadiag.FindingSeverity
	message, help string
}

// corpusDiagnosticActualRows reduces a report to the family's published rows.
// A row that cannot name its own location is recorded as a mismatch rather than
// dropped, so an unlocatable finding cannot make a family look clean.
func corpusDiagnosticActualRows(result *corpusDiagnosticFamilyResult, report *anadiag.DiagnosticReport, code string) []corpusDiagnosticActualRow {
	actual := make([]corpusDiagnosticActualRow, 0)
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
		actual = append(actual, corpusDiagnosticActualRow{index: index, finding: finding, file: location.File(), line: line, column: column, severity: finding.Severity(), message: finding.Message(), help: finding.Help()})
	}
	return actual
}

// judgeCorpusDiagnosticRows is the one-to-one judgment: every expected row
// claims exactly one published row, and every published row a manifest did not
// claim is reported. Matching is stated over reduced rows rather than over a
// report so the judgment itself is provable without a published analysis.
func judgeCorpusDiagnosticRows(compilation composite.Compilation, result corpusDiagnosticFamilyResult, expectation *corpusDiagnosticProjectExpectations, expected []corpusStructuredDiagnosticExpectation, actual []corpusDiagnosticActualRow, sourceText func(string) (string, bool)) corpusDiagnosticFamilyResult {
	result.Actual = len(actual)
	used := make([]bool, len(actual))
	for expectedIndex, want := range expected {
		matched := -1
		for index, got := range actual {
			if used[index] || !corpusDiagnosticProjectMatchesFile(expectation, want.File, got.file) || got.line != uint32(want.Line) || got.severity != corpusDiagnosticSeverity(compilation, want.Severity) {
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
	run, _, err := corpusHarnessExecuteWithPlanCleanup(t, corpusHarnessFixture(t, projectName), corpusHarnessDiagnosticMode(), false)
	if run != nil && run.plan != nil {
		plan := run.plan
		defer func() {
			if !plan.Close() {
				t.Error("close compiled fixture plan")
			}
		}()
	}
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.plan == nil {
		t.Fatal("manifest runner has no compiled plan")
	}
	plan := run.plan
	_, report, status, diagnostics := plan.SolveWithReport(context.Background(), corpusHarnessSolveOptions(), anadiag.DiagnosticPolicy{Enabled: family.enabled})
	if status != analysis.AnalyzeComplete {
		t.Fatalf("manifest runner solve %s = %v diagnostics=%+v", projectName, status, diagnostics)
	}
	return matchCorpusDiagnosticFamily(run.compilation, projectName, project, report, code, func(file string) (string, bool) {
		file = corpusDiagnosticProjectSourceFile(project, file)
		contents, err := os.ReadFile(filepath.Join(project.directory, filepath.FromSlash(file)))
		if err != nil {
			return "", false
		}
		return string(contents), true
	})
}

func TestCorpusDiagnosticManifestRunnerAlwaysTrueFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, anadiag.DiagnosticCodeAlwaysTrueGuard.String())
}

func TestCorpusDiagnosticManifestRunnerAlwaysFalseFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, anadiag.DiagnosticCodeAlwaysFalseGuard.String())
}

func TestCorpusDiagnosticManifestRunnerUnresolvedTypeReferenceFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, anadiag.DiagnosticCodeUnresolvedTypeReference.String())
}

func TestCorpusDiagnosticManifestRunnerUnresolvedValueReferenceFamilyLaw(t *testing.T) {
	runCorpusDiagnosticNativeFamilyLaw(t, anadiag.DiagnosticCodeUnresolvedValueReference.String())
}

// TestCorpusDiagnosticManifestRunnerNativeFamiliesLaw makes every installed
// producer prove its own narrow corpus cases. New native support is one table
// registration, and may be run alone with -run
// '^TestCorpusDiagnosticManifestRunnerNativeFamiliesLaw/<code>$'.
func TestCorpusDiagnosticManifestRunnerNativeFamiliesLaw(t *testing.T) {
	seenCodes := make(map[string]struct{}, len(corpusDiagnosticNativeFamilies))
	for _, family := range corpusDiagnosticNativeFamilies {
		code := family.code.String()
		if family.code == anadiag.DiagnosticCodeInvalid || code == "" || len(family.enabled) == 0 || len(family.cases) == 0 {
			t.Fatalf("native family registration is incomplete: %+v", family)
		}
		if _, duplicate := seenCodes[code]; duplicate {
			t.Fatalf("duplicate native diagnostic code %q", code)
		}
		seenCodes[code] = struct{}{}
		for _, enabled := range family.enabled {
			if enabled != family.code {
				t.Fatalf("native family %q enables the foreign code %q", code, enabled.String())
			}
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
	code := anadiag.DiagnosticCodeAlwaysTrueGuard.String()
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

// TestCorpusDiagnosticManifestRunnerUnexpectedFindingLaw proves the matcher's
// one-to-one core: a second published row at the expected location is claimed
// by nothing, so it is reported as an unexpected row rather than absorbed by
// the expectation the first row already satisfied.
func TestCorpusDiagnosticManifestRunnerUnexpectedFindingLaw(t *testing.T) {
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	code := anadiag.DiagnosticCodeAlwaysTrueGuard.String()
	row := corpusDiagnosticActualRow{file: "main.lua", line: 2, column: 1, severity: anadiag.FindingSeverityHint}
	project := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{Check: &corpusDiagnosticManifestCheck{
		Diagnostics: []corpusStructuredDiagnosticExpectation{{File: "main.lua", Line: 2, Column: 1, Severity: "hint", Code: code}},
	}}}
	expected := project.manifest.Check.Diagnostics
	seed := corpusDiagnosticFamilyResult{Project: "synthetic", Code: code, Status: corpusDiagnosticFamilySupported, Expected: len(expected)}
	got := judgeCorpusDiagnosticRows(compilation, seed, project, expected, []corpusDiagnosticActualRow{row, {index: 1, file: row.file, line: row.line, column: row.column, severity: row.severity}},
		func(string) (string, bool) { return "\nif true then end\n", true })
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
	if catalog.inventory.projects != 914 || catalog.inventory.structuredFindings != 136 || catalog.inventory.inlineErrors+catalog.inventory.inlineWarnings != 714 {
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
