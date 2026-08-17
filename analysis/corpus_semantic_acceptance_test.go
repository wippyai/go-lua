package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
)

// TestCanonicalCorpusSemanticAcceptance is the one complete fixture semantic
// gate.  The fixture source remains the sole expectation authority: this test
// consumes the shared harness enumeration used by the census and the narrow
// producer laws instead of copying any fixture rows into Go.
//
// There is deliberately no local timeout, work budget, result cap, skip, or
// fixture-family branch. The bounded repository runner owns process-tree
// resource enforcement. A runner timeout is therefore a failed acceptance
// invocation, never a successful fixture result.
func TestCanonicalCorpusSemanticAcceptance(t *testing.T) {
	corpusHarnessWalk(t, corpusHarnessProjects(t), corpusSemanticAcceptanceMode())
}

// corpusSemanticAcceptanceMode is the authoritative corpus judgment. The
// harness always passes the project through the current Program -> Link ->
// Plan -> Result path before this mode judges its source-authored contract; an
// unavailable diagnostic family is reported as an acceptance failure after
// that current-analyzer run, never treated as clean.
func corpusSemanticAcceptanceMode() corpusHarnessMode {
	return corpusHarnessMode{
		name:      "acceptance",
		execution: corpusHarnessReportSolve,
		preflight: func(project *corpusHarnessProject) []string {
			return corpusSemanticFixtureInputUnsupported(project.expectation)
		},
		policy: func(project *corpusHarnessProject) (DiagnosticPolicy, []string) {
			return corpusSemanticAcceptancePolicy(project.expectation)
		},
		judge: corpusSemanticAcceptanceJudgment,
	}
}

func corpusSemanticAcceptanceJudgment(run *corpusHarnessRun) []string {
	expectation := run.project.expectation
	if len(run.policyUnsupported) != 0 {
		return []string{"unsupported diagnostic contract:\n" + strings.Join(run.policyUnsupported, "\n")}
	}
	if len(run.policy.Enabled) != 0 {
		if run.report == nil || !run.report.Available() {
			return []string{"DiagnosticReport unavailable"}
		}
		if failure := run.report.CollectionFailure(); failure != DiagnosticCollectionOK {
			return []string{fmt.Sprintf("DiagnosticReport collection failure=%d", failure)}
		}
	} else if run.report != nil {
		return []string{"disabled diagnostic policy unexpectedly produced a DiagnosticReport"}
	}
	if mismatches := corpusSemanticAcceptanceMismatches(expectation, run.report); len(mismatches) != 0 {
		return []string{"semantic diagnostic mismatch:\n" + strings.Join(mismatches, "\n")}
	}
	if mismatches := corpusSemanticNativeMismatches(expectation, run.result); len(mismatches) != 0 {
		return []string{"semantic native mismatch:\n" + strings.Join(mismatches, "\n")}
	}
	return nil
}

// corpusSemanticFixtureInputUnsupported fences contracts the current
// Program-owned fixture adapter cannot preserve. The strict frozen catalog is
// the only source of this classification. In particular, this acceptance gate
// must not run a fixture through the adapter when its manifest declared a host
// surface the adapter drops, nor quietly drop native, placement, or rendering
// obligations after a successful solve.
//
// A declared module inventory is not such a contract. Link canonicalizes mount
// order by Program identity, requires an analysis root for every mount, and
// resolves every require against a mount name, so a fixture's declared file
// order and its selected entry carry no obligation the sealed Link could hold
// differently: the whole declared inventory is mounted, rooted, and wired.
func corpusSemanticFixtureInputUnsupported(expectation *corpusDiagnosticProjectExpectations) []string {
	if expectation == nil {
		return []string{"fixture expectation unavailable"}
	}
	unsupported := make([]string, 0, 6)
	if expectation.manifest == nil {
		return unsupported
	}
	if expectation.manifest.Stdlib != nil {
		unsupported = append(unsupported, "manifest stdlib contract is not preserved by the current fixture input adapter")
	}
	if len(expectation.manifest.Packages) != 0 {
		// Link resolves a require only against a project mount, and Host binds
		// capabilities to initial roots rather than to require-able modules, so a
		// declared system package has no canonical surface to be admitted through
		// at all. The fence names that missing surface instead of running the
		// fixture with its host types silently absent.
		unsupported = append(unsupported, "manifest package host contract has no canonical require-able host module surface")
	}
	if expectation.manifest.Check == nil {
		return unsupported
	}
	if expectation.manifest.Check.Placement != nil {
		unsupported = append(unsupported, "check.placement requires an unavailable receipt-native allocation projection")
	}
	if expectation.manifest.Check.RenderOptions != nil {
		unsupported = append(unsupported, "check.render_options requires an unavailable current rendering contract")
	}
	return unsupported
}

type corpusSemanticNativeRow struct {
	row                                      NativePublication
	lane, module, family, key, subject, term string
	occurrence, value, trust                 string
	valueOK                                  bool
	validity                                 NativePublicationValidity
	validityOK                               bool
}

func corpusSemanticNativeMismatches(expectation *corpusDiagnosticProjectExpectations, result *Result) []string {
	if expectation == nil || result == nil {
		return []string{"native fixture expectation or Result unavailable"}
	}
	var contract *corpusNativeContract
	if expectation.manifest != nil && expectation.manifest.Check != nil {
		contract = expectation.manifest.Check.Native
	}
	if contract == nil {
		return nil
	}
	if !result.NativePublicationAvailable() {
		return []string{"NativePublication unavailable"}
	}
	rows := make([]corpusSemanticNativeRow, 0, result.NativePublicationCount())
	mismatches := make([]string, 0)
	for index := 0; index < result.NativePublicationCount(); index++ {
		row, rowOK := result.NativePublicationAt(index)
		id, idOK := row.ID()
		value, valueOK := row.Value()
		validity, validityOK := row.Validity()
		if !rowOK || !idOK || !id.Available() || !row.Lane().Valid() || !row.Kind().Valid() || !row.Trust().Valid() || !row.SemanticID().Available() || row.Family() == "" {
			mismatches = append(mismatches, fmt.Sprintf("native row %d has no complete public identity", index))
			continue
		}
		rows = append(rows, corpusSemanticNativeRow{
			row: row, lane: row.Lane().String(), module: row.Module(), family: row.Family(), key: row.Key(), subject: row.Subject(), term: row.Term(), occurrence: row.Occurrence(),
			value: value, valueOK: valueOK, trust: row.Trust().String(), validity: validity, validityOK: validityOK,
		})
	}
	if contract.MinFacts != nil && len(rows) < *contract.MinFacts {
		mismatches = append(mismatches, fmt.Sprintf("native fact count=%d, want >=%d", len(rows), *contract.MinFacts))
	}
	if contract.MaxFacts != nil && len(rows) > *contract.MaxFacts {
		mismatches = append(mismatches, fmt.Sprintf("native fact count=%d, want <=%d", len(rows), *contract.MaxFacts))
	}
	for ordinal, fact := range contract.Facts {
		count := 0
		for _, row := range rows {
			if corpusSemanticNativeSelectorMatches(fact.selector(), row) && corpusSemanticNativeRevocationsMatch(fact, row) {
				count++
			}
		}
		minimum, maximum := 0, -1
		if fact.Min != nil {
			minimum = *fact.Min
		}
		if fact.Max != nil {
			maximum = *fact.Max
		}
		if count < minimum || maximum >= 0 && count > maximum {
			name := fact.Name
			if name == "" {
				name = fmt.Sprintf("fact[%d]", ordinal)
			}
			mismatches = append(mismatches, fmt.Sprintf("native %s matched %d row(s), want min=%d max=%d", name, count, minimum, maximum))
		}
	}
	for ordinal, invalidation := range contract.Invalidation {
		count := 0
		for _, row := range rows {
			if corpusSemanticNativeSelectorMatches(invalidation.selector(), row) && corpusSemanticNativeInvalidationMatches(invalidation, row) {
				count++
			}
		}
		minimum, maximum := 0, -1
		if invalidation.Min != nil {
			minimum = *invalidation.Min
		}
		if invalidation.Max != nil {
			maximum = *invalidation.Max
		}
		if count < minimum || maximum >= 0 && count > maximum {
			name := invalidation.Name
			if name == "" {
				name = fmt.Sprintf("invalidation[%d]", ordinal)
			}
			mismatches = append(mismatches, fmt.Sprintf("native %s matched %d row(s), want min=%d max=%d", name, count, minimum, maximum))
		}
	}
	return mismatches
}

func corpusSemanticNativeSelectorMatches(selector corpusNativeSelector, row corpusSemanticNativeRow) bool {
	if selector.Lane != "" && selector.Lane != row.lane || selector.Module != "" && selector.Module != row.module || selector.Family != "" && selector.Family != row.family ||
		selector.Key != "" && selector.Key != row.key || selector.KeyPrefix != "" && !strings.HasPrefix(row.key, selector.KeyPrefix) || selector.KeySuffix != "" && !strings.HasSuffix(row.key, selector.KeySuffix) ||
		selector.Subject != "" && selector.Subject != row.subject || selector.Term != "" && selector.Term != row.term || selector.Occurrence != "" && selector.Occurrence != row.occurrence || selector.Trust != "" && selector.Trust != row.trust {
		return false
	}
	for _, part := range selector.KeyContains {
		if !strings.Contains(row.key, part) {
			return false
		}
	}
	if selector.Value != nil && (!row.valueOK || row.value != *selector.Value) || selector.ValuePrefix != "" && (!row.valueOK || !strings.HasPrefix(row.value, selector.ValuePrefix)) {
		return false
	}
	for _, part := range selector.ValueContains {
		if !row.valueOK || !strings.Contains(row.value, part) {
			return false
		}
	}
	return true
}

func corpusSemanticNativeRevocationsMatch(fact corpusNativeFactContract, row corpusSemanticNativeRow) bool {
	if len(fact.RevokedBy) == 0 {
		return true
	}
	if !row.validityOK {
		return false
	}
	matched := 0
	for _, expected := range fact.RevokedBy {
		if corpusSemanticNativeValidityMatches(expected.Event, expected.Established, expected.Revoked, row.validity) {
			matched++
		}
	}
	return matched == len(fact.RevokedBy) && (!fact.RevokedByExhaustive || matched == 1)
}

func corpusSemanticNativeInvalidationMatches(invalidation corpusNativeInvalidationContract, row corpusSemanticNativeRow) bool {
	return row.validityOK && corpusSemanticNativeValidityMatches(invalidation.Event, invalidation.Established, invalidation.Revoked, row.validity)
}

func corpusSemanticNativeValidityMatches(event, established, revoked string, validity NativePublicationValidity) bool {
	if event != "" && event != validity.EventID().String() || established != "" && established != validity.EstablishedID().String() || revoked != "" && revoked != validity.RevokedID().String() {
		return false
	}
	return true
}

// corpusSemanticAcceptancePolicy derives one complete, closed policy from the
// current collector registry and fixture flags. Every installed collector is
// enabled by default so a clean fixture cannot hide a new false positive;
// manifest diagnostic_rules can disable or refine that default. A requested
// code without a current collector is an explicit unsupported contract.
func corpusSemanticAcceptancePolicy(expectation *corpusDiagnosticProjectExpectations) (DiagnosticPolicy, []string) {
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		return DiagnosticPolicy{}, []string{"sealed diagnostic declaration table unavailable"}
	}
	selected := make(map[DiagnosticCode]bool, table.Count())
	severity := make(map[DiagnosticCode]FindingSeverity)
	unsupported := make([]string, 0)
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK {
			return DiagnosticPolicy{}, []string{"sealed diagnostic declaration row unavailable"}
		}
		if entry.Collectable() {
			selected[entry.Code()] = true
		}
	}

	declared := make(map[string]corpusDiagnosticRuleExpectation)
	if expectation != nil && expectation.manifest != nil && expectation.manifest.Check != nil {
		for _, configured := range expectation.manifest.Check.DiagnosticRules {
			if _, duplicate := declared[configured.Code]; duplicate {
				unsupported = append(unsupported, fmt.Sprintf("duplicate diagnostic rule %q", configured.Code))
				continue
			}
			declared[configured.Code] = configured
			rule, available := corpusSemanticAcceptanceCode(configured.Code)
			if !available {
				unsupported = append(unsupported, fmt.Sprintf("diagnostic rule %q has no current collector", configured.Code))
				continue
			}
			if configured.Enabled == nil {
				unsupported = append(unsupported, fmt.Sprintf("diagnostic rule %q has no enabled setting", configured.Code))
				continue
			}
			if *configured.Enabled {
				selected[rule] = true
			} else {
				delete(selected, rule)
				delete(severity, rule)
			}
			if configured.Severity == "" {
				continue
			}
			level := corpusDiagnosticSeverity(configured.Severity)
			if level == FindingSeverityInvalid {
				unsupported = append(unsupported, fmt.Sprintf("diagnostic rule %q has invalid severity %q", configured.Code, configured.Severity))
				continue
			}
			if *configured.Enabled {
				severity[rule] = level
			}
		}
		for _, expected := range expectation.manifest.Check.Diagnostics {
			rule, available := corpusSemanticAcceptanceCode(expected.Code)
			if !available {
				unsupported = append(unsupported, fmt.Sprintf("expected diagnostic %q has no current collector", expected.Code))
				continue
			}
			if configured, declared := declared[expected.Code]; declared && configured.Enabled != nil && !*configured.Enabled {
				unsupported = append(unsupported, fmt.Sprintf("expected diagnostic %q is disabled by fixture policy", expected.Code))
				continue
			}
			selected[rule] = true
		}
	}

	rules := make([]DiagnosticCode, 0, len(selected))
	for rule := range selected {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i] < rules[j] })
	if len(severity) == 0 {
		severity = nil
	}
	return DiagnosticPolicy{Enabled: rules, Severity: severity}, unsupported
}

// corpusSemanticAcceptanceCode is intentionally table-driven. It makes no
// diagnostic-family choice: a code is accepted only when the sealed
// declaration row it names installs a producer.
func corpusSemanticAcceptanceCode(code string) (DiagnosticCode, bool) {
	candidate := DiagnosticCode(code)
	if !diagnosticCollectable(candidate) {
		return DiagnosticCodeInvalid, false
	}
	return candidate, true
}

type corpusSemanticAcceptanceFinding struct {
	finding       Finding
	file, message string
	line, column  uint32
	severity      FindingSeverity
	code          string
}

func corpusSemanticAcceptanceFindings(report *DiagnosticReport) ([]corpusSemanticAcceptanceFinding, []string) {
	if report == nil {
		return nil, nil
	}
	if !report.Available() {
		return nil, []string{"DiagnosticReport unavailable"}
	}
	findings := make([]corpusSemanticAcceptanceFinding, 0, report.FindingCount())
	mismatches := make([]string, 0)
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		location, locationOK := finding.Location()
		if !findingOK || !locationOK || finding.Code().String() == "" || !finding.Severity().Available() {
			mismatches = append(mismatches, fmt.Sprintf("finding %d has no complete public identity", index))
			continue
		}
		line, column := location.Start()
		findings = append(findings, corpusSemanticAcceptanceFinding{
			finding: finding, file: location.File(), line: line, column: column,
			severity: finding.Severity(), code: finding.Code().String(), message: finding.Message(),
		})
	}
	return findings, mismatches
}

// corpusSemanticAcceptanceMismatches preserves the recovered fixture priority
// while tightening it to a complete public-result oracle: inline markers and
// structured rows may both describe one finding, but each representation is
// individually one-to-one and every actual row must be covered by at least
// one of them. Thus duplicate or extra findings cannot escape through a
// source-marker overlap.
func corpusSemanticAcceptanceMismatches(expectation *corpusDiagnosticProjectExpectations, report *DiagnosticReport) []string {
	if expectation == nil {
		return []string{"fixture expectation unavailable"}
	}
	actual, mismatches := corpusSemanticAcceptanceFindings(report)
	if len(mismatches) != 0 {
		return mismatches
	}

	structuredUsed := make([]bool, len(actual))
	inlineUsed := make([]bool, len(actual))
	var structured []corpusStructuredDiagnosticExpectation
	var errors *int
	if expectation.manifest != nil && expectation.manifest.Check != nil {
		structured = expectation.manifest.Check.Diagnostics
		errors = expectation.manifest.Check.Errors
	}

	for ordinal, want := range structured {
		matched := -1
		for index, got := range actual {
			if structuredUsed[index] || !corpusSemanticStructuredMatch(expectation, want, got) {
				continue
			}
			matched = index
			break
		}
		if matched < 0 {
			mismatches = append(mismatches, fmt.Sprintf("missing structured diagnostic %d %s at %s:%d:%d", ordinal, want.Code, want.File, want.Line, want.Column))
			continue
		}
		structuredUsed[matched] = true
		details := corpusDiagnosticFamilyResult{}
		matchCorpusDiagnosticDetails(&details, want, actual[matched].finding, actual[matched].file, corpusSemanticAcceptanceSource(expectation))
		for _, mismatch := range details.Mismatches {
			mismatches = append(mismatches, fmt.Sprintf("structured diagnostic %d: %s", ordinal, mismatch))
		}
	}

	for _, want := range expectation.inline {
		matched := -1
		// Prefer the exact structured row already selected at this marker.
		// Inline and structured source contracts commonly describe the same
		// public finding; consuming that finding first leaves any duplicate
		// report row uncovered and therefore failing below.
		for index, got := range actual {
			if !structuredUsed[index] || inlineUsed[index] || !corpusSemanticInlineMatch(expectation, want, got) {
				continue
			}
			matched = index
			break
		}
		if matched < 0 {
			for index, got := range actual {
				if structuredUsed[index] || inlineUsed[index] || !corpusSemanticInlineMatch(expectation, want, got) {
					continue
				}
				matched = index
				break
			}
		}
		if matched < 0 {
			mismatches = append(mismatches, fmt.Sprintf("missing inline %s at %s:%d %q", want.Severity, want.File, want.Line, want.Contains))
			continue
		}
		inlineUsed[matched] = true
	}

	// The recovered oracle applies an error-count contract whenever structured
	// diagnostics are absent. Inline markers verify local rows independently;
	// they do not suppress the manifest's aggregate error-count check.
	if corpusSemanticErrorCountApplies(len(structured), errors) {
		got := corpusSemanticErrorCount(actual)
		if got != *errors {
			mismatches = append(mismatches, fmt.Sprintf("error count=%d, want %d", got, *errors))
		}
	}
	if len(expectation.inline) == 0 && len(structured) == 0 && errors == nil {
		if errors := corpusSemanticErrorCount(actual); errors != 0 {
			mismatches = append(mismatches, fmt.Sprintf("clean fixture emitted %d errors", errors))
		}
	}

	for index, got := range actual {
		if structuredUsed[index] || inlineUsed[index] {
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("unexpected diagnostic %s at %s:%d:%d: %s", got.code, got.file, got.line, got.column, got.message))
	}
	return mismatches
}

func corpusSemanticErrorCountApplies(structuredCount int, errors *int) bool {
	return structuredCount == 0 && errors != nil
}

func TestCorpusSemanticErrorCountPrecedence(t *testing.T) {
	count := 1
	if !corpusSemanticErrorCountApplies(0, &count) {
		t.Fatal("error count must apply without structured diagnostics, including alongside inline markers")
	}
	if corpusSemanticErrorCountApplies(1, &count) {
		t.Fatal("structured diagnostics must take precedence over error count")
	}
	if corpusSemanticErrorCountApplies(0, nil) {
		t.Fatal("absent error count must not become a contract")
	}
}

func TestCorpusSemanticFixtureContractPreflight(t *testing.T) {
	clean := &corpusDiagnosticProjectExpectations{name: "clean", files: []string{"main.lua"}, entryFile: "main.lua", entryModule: "main"}
	if unsupported := corpusSemanticFixtureInputUnsupported(clean); len(unsupported) != 0 {
		t.Fatalf("representable fixture input rejected: %v", unsupported)
	}
	declaredSingleton := &corpusDiagnosticProjectExpectations{
		name:          "declared-singleton",
		files:         []string{"main.lua"},
		declaredFiles: []string{"main.lua"},
		entryFile:     "main.lua",
		entryModule:   "main",
	}
	if unsupported := corpusSemanticFixtureInputUnsupported(declaredSingleton); len(unsupported) != 0 {
		t.Fatalf("declared singleton fixture input rejected: %v", unsupported)
	}
	multiModule := &corpusDiagnosticProjectExpectations{
		name:          "multi-module",
		files:         []string{"main.lua", "module.lua"},
		declaredFiles: []string{"module.lua", "main.lua"},
		entryFile:     "main.lua",
		entryModule:   "main",
	}
	if unsupported := corpusSemanticFixtureInputUnsupported(multiModule); len(unsupported) != 0 {
		t.Fatalf("declared multi-module fixture input rejected: %v", unsupported)
	}
	unsupported := corpusSemanticFixtureInputUnsupported(&corpusDiagnosticProjectExpectations{
		name:          "all-contracts",
		files:         []string{"main.lua"},
		declaredFiles: []string{"main.lua"},
		manifest: &corpusDiagnosticManifest{
			Stdlib:   new(bool),
			Packages: []string{"process"},
			Check: &corpusDiagnosticManifestCheck{
				Native:        &corpusNativeContract{},
				Placement:     &corpusPlacementContract{},
				RenderOptions: &corpusDiagnosticRenderOptions{WitnessTrace: true},
			},
		},
	})
	if len(unsupported) != 4 {
		t.Fatalf("unsupported fixture contracts=%v, want all four unpreserved non-input surfaces", unsupported)
	}
}

func TestCorpusSemanticFixtureFileAliases(t *testing.T) {
	project := &corpusDiagnosticProjectExpectations{
		files:       []string{"main.lua", "module.lua"},
		entryFile:   "main.lua",
		entryModule: "main",
	}
	got := corpusSemanticAcceptanceFinding{code: "x", file: "module", line: 4, severity: FindingSeverityError}
	want := corpusStructuredDiagnosticExpectation{Code: "x", File: "module.lua", Line: 4, Severity: "error"}
	if !corpusSemanticStructuredMatch(project, want, got) {
		t.Fatal("module-name finding did not match its .lua contract")
	}
	got.file = "test.lua"
	want.File = "main.lua"
	if !corpusSemanticStructuredMatch(project, want, got) {
		t.Fatal("test.lua finding did not match the selected entry contract")
	}
	if corpusSemanticInlineMatch(project, corpusInlineDiagnosticExpectationRow{File: "other.lua", Line: 4, Severity: "error"}, got) {
		t.Fatal("unrelated file alias matched the selected entry")
	}
}

func corpusSemanticStructuredMatch(project *corpusDiagnosticProjectExpectations, want corpusStructuredDiagnosticExpectation, got corpusSemanticAcceptanceFinding) bool {
	if got.code != want.Code || !corpusDiagnosticProjectMatchesFile(project, want.File, got.file) || got.line != uint32(want.Line) || got.severity != corpusDiagnosticSeverity(want.Severity) {
		return false
	}
	return want.Column == 0 || got.column == uint32(want.Column)
}

func corpusSemanticInlineMatch(project *corpusDiagnosticProjectExpectations, want corpusInlineDiagnosticExpectationRow, got corpusSemanticAcceptanceFinding) bool {
	spelling, spellingOK := findingSeveritySpelling(got.severity)
	if !corpusDiagnosticProjectMatchesFile(project, want.File, got.file) || got.line != uint32(want.Line) || !spellingOK || spelling != want.Severity {
		return false
	}
	return want.Contains == "" || strings.Contains(got.message, want.Contains)
}

func corpusSemanticErrorCount(actual []corpusSemanticAcceptanceFinding) int {
	count := 0
	for _, finding := range actual {
		if finding.severity == FindingSeverityError {
			count++
		}
	}
	return count
}

func corpusSemanticAcceptanceSource(expectation *corpusDiagnosticProjectExpectations) func(string) (string, bool) {
	return func(file string) (string, bool) {
		if expectation == nil || expectation.directory == "" || file == "" {
			return "", false
		}
		contents, err := os.ReadFile(filepath.Join(expectation.directory, filepath.FromSlash(corpusDiagnosticProjectSourceFile(expectation, file))))
		if err != nil {
			return "", false
		}
		return string(contents), true
	}
}
