package oracle

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
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
		policy: func(run *corpusHarnessRun) (anadiag.DiagnosticPolicy, []string) {
			return corpusSemanticAcceptancePolicy(run.compilation, run.project.expectation)
		},
		judge: corpusSemanticAcceptanceJudgment,
	}
}

// corpusSemanticAcceptanceVerdict is one fixture's acceptance answer with its
// rows still separated by the contract each one breaks. The judgment below
// renders it; a consumer that counts rows rather than failing on the first one
// reads it. Keeping the rows apart is what lets a count distinguish a fixture
// whose contract this analyzer cannot express yet from one whose expectations
// it can express and does not meet.
type corpusSemanticAcceptanceVerdict struct {
	// unsupported is the fixture contract the current declaration table cannot
	// express. It is decided before any expectation is compared.
	unsupported []string
	// unavailable is the report contract: a policy that selected codes and
	// produced no readable report, or a collector that refused.
	unavailable []string
	diagnostics []string
	native      []string
	placement   []string
}

// judged reports whether the fixture's own expectations were compared at all.
// An unsupported contract or an unavailable report ends the run before that
// comparison, so its expectation rows are unmet rather than met.
func (verdict corpusSemanticAcceptanceVerdict) judged() bool {
	return len(verdict.unsupported) == 0 && len(verdict.unavailable) == 0
}

func (verdict corpusSemanticAcceptanceVerdict) clean() bool {
	return verdict.judged() && len(verdict.diagnostics) == 0 && len(verdict.native) == 0 && len(verdict.placement) == 0
}

// corpusSemanticAcceptanceVerdictOf is the acceptance verdict of one completed
// run. It states the same order the judgment publishes: contract, report,
// diagnostics, native, placement.
func corpusSemanticAcceptanceVerdictOf(run *corpusHarnessRun) corpusSemanticAcceptanceVerdict {
	verdict := corpusSemanticAcceptanceVerdict{}
	if len(run.policyUnsupported) != 0 {
		verdict.unsupported = run.policyUnsupported
		return verdict
	}
	if len(run.policy.Enabled) != 0 {
		if run.report == nil || !run.report.Available() {
			verdict.unavailable = []string{"DiagnosticReport unavailable"}
			return verdict
		}
		if failure := run.report.CollectionFailure(); failure != anadiag.DiagnosticCollectionOK {
			verdict.unavailable = []string{fmt.Sprintf("DiagnosticReport collection failure=%d", failure)}
			return verdict
		}
	} else if run.report != nil {
		verdict.unavailable = []string{"disabled diagnostic policy unexpectedly produced a DiagnosticReport"}
		return verdict
	}
	expectation := run.project.expectation
	verdict.diagnostics = corpusSemanticAcceptanceMismatches(run.compilation, expectation, run.report)
	verdict.native = corpusSemanticNativeMismatches(run.compilation, expectation, run.result)
	verdict.placement = corpusSemanticPlacementMismatches(expectation, run.result, run.placementSchema)
	return verdict
}

func corpusSemanticAcceptanceJudgment(run *corpusHarnessRun) []string {
	verdict := corpusSemanticAcceptanceVerdictOf(run)
	if len(verdict.unsupported) != 0 {
		return []string{"unsupported diagnostic contract:\n" + strings.Join(verdict.unsupported, "\n")}
	}
	if len(verdict.unavailable) != 0 {
		return verdict.unavailable
	}
	if len(verdict.diagnostics) != 0 {
		return []string{"semantic diagnostic mismatch:\n" + strings.Join(verdict.diagnostics, "\n")}
	}
	if len(verdict.native) != 0 {
		return []string{"semantic native mismatch:\n" + strings.Join(verdict.native, "\n")}
	}
	if len(verdict.placement) != 0 {
		return []string{"semantic placement mismatch:\n" + strings.Join(verdict.placement, "\n")}
	}
	return nil
}

// corpusSemanticFixtureInputUnsupported fences contracts the current
// Program-owned fixture adapter cannot preserve. The strict frozen catalog is
// the only source of this classification. In particular, this acceptance gate
// must not quietly drop native or rendering obligations after a successful
// solve.
//
// A declared module inventory is not such a contract. Link canonicalizes mount
// order by Program identity, requires an analysis root for every mount, and
// resolves every require against a mount name, so a fixture's declared file
// order and its selected entry carry no obligation the sealed Link could hold
// differently: the whole declared inventory is mounted, rooted, and wired.
// The canonical fixture Target declares its require-able host module set at
// Target.InitialRootByModulePath, the same table the require-admission gate
// consults; a manifest package name is supported exactly when that table
// declares it, read here through corpusSemanticDeclaredHostModulePaths rather
// than restated as a second, hand-copied list of names. Placement is likewise
// not an input contract: its check is judged after the solve through
// domain/placement/publication, where a missing family is an operational
// mismatch and an unreadable leak dimension is an explicit incomplete
// classification.
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
		declared, err := corpusSemanticDeclaredHostModulePaths()
		if err != nil {
			unsupported = append(unsupported, fmt.Sprintf("canonical fixture Target profile unavailable: %v", err))
		}
		for _, packageName := range expectation.manifest.Packages {
			if _, ok := declared[packageName]; ok {
				continue
			}
			unsupported = append(unsupported, fmt.Sprintf("manifest package host contract %q has no canonical require-able host module surface", packageName))
		}
	}
	if expectation.manifest.Check == nil {
		return unsupported
	}
	if expectation.manifest.Check.RenderOptions != nil {
		unsupported = append(unsupported, "check.render_options requires an unavailable current rendering contract")
	}
	return unsupported
}

// corpusSemanticDeclaredHostModulePaths is the canonical fixture Target's
// require-able module-path surface: every InitialRoot the sealed contract
// names by a module path. It is a live projection of that one declaration
// table, not a second authority a provider addition could drift out of sync
// with.
func corpusSemanticDeclaredHostModulePaths() (map[string]struct{}, error) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		return nil, err
	}
	paths := make(map[string]struct{}, target.InitialRootCount())
	for index := 0; index < target.InitialRootCount(); index++ {
		root, ok := target.InitialRootAt(index)
		if !ok {
			continue
		}
		path, pathOK := target.InitialRootModulePath(root)
		if !pathOK || path == "" {
			continue
		}
		paths[path] = struct{}{}
	}
	return paths, nil
}

// TestCorpusSemanticFixtureInputSupportsDeclaredHostModulePaths pins the
// package fence to the Target's own declaration table rather than a
// hand-copied name list: a manifest naming any module path the Target
// declares must not be fenced, and one naming a path the Target does not
// declare must be, by that name.
func TestCorpusSemanticFixtureInputSupportsDeclaredHostModulePaths(t *testing.T) {
	declared, err := corpusSemanticDeclaredHostModulePaths()
	if err != nil {
		t.Fatalf("declared host module paths: %v", err)
	}
	if _, ok := declared["string"]; !ok {
		t.Fatal("canonical fixture Target does not declare the stdlib \"string\" module path; fixture assumption stale")
	}
	supported := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Packages: []string{"uuid", "channel", "string", "table"},
	}}
	if got := corpusSemanticFixtureInputUnsupported(supported); len(got) != 0 {
		t.Fatalf("declared packages fenced as unsupported: %v", got)
	}
	undeclared := &corpusDiagnosticProjectExpectations{manifest: &corpusDiagnosticManifest{
		Packages: []string{"time"},
	}}
	got := corpusSemanticFixtureInputUnsupported(undeclared)
	if len(got) != 1 || !strings.Contains(got[0], `"time"`) {
		t.Fatalf("undeclared package fence = %v, want exactly one row naming \"time\"", got)
	}
}

type corpusSemanticNativeRow struct {
	row                                      result.NativePublication
	lane, module, family, key, subject, term string
	occurrence, trust                        string
	// columns is the row's published typed content, keyed by the column name a
	// manifest names it under. A column absent from the map is a column the row
	// does not publish, which is a distinct answer from a column whose member
	// happens to be spelled like another.
	columns    map[string]string
	rendered   string
	validity   result.NativePublicationValidity
	validityOK bool
}

// corpusNativeColumnOrder is the canonical order a native row's typed columns
// are read and rendered in.
var corpusNativeColumnOrder = []string{
	"exact", "literal", "representation", "left", "right", "operand",
	"operator", "overflow", "divisor", "truthiness", "partition", "dead_arm", "dead_arm_reachable",
}

// corpusNativeColumns reads one published row's typed columns and resolves each
// vocabulary-valued one to its declared spelling. Nothing here holds a spelling
// of its own: the sealed structural vocabulary is the single declaration, and a
// column the row does not publish is simply absent.
func corpusNativeColumns(compilation composite.Compilation, row result.NativePublication) (map[string]string, bool) {
	vocabulary, vocabularyOK := composite.StructureVocabulary(compilation)
	if !vocabularyOK {
		return nil, false
	}
	complete := true
	columns := make(map[string]string, len(corpusNativeColumnOrder))
	declared := func(name string, column result.NativePublicationColumn) {
		ordinal, published := row.Column(column)
		if !published {
			return
		}
		member, memberOK := vocabulary.At(column.Category(), ordinal)
		if !memberOK {
			complete = false
			return
		}
		columns[name] = member.Spelling()
	}
	if row.Exact() {
		columns["exact"] = "true"
	}
	if literal, literalOK := row.Literal(); literalOK {
		text, textOK := corpusNativeLiteral(literal)
		complete = complete && textOK
		columns["literal"] = text
	}
	if _, scalarOK := row.ScalarRepresentation(); scalarOK {
		declared("representation", result.NativePublicationColumnScalarRepresentation)
	} else {
		declared("representation", result.NativePublicationColumnRepresentation)
	}
	declared("left", result.NativePublicationColumnLeft)
	declared("right", result.NativePublicationColumnRight)
	declared("operand", result.NativePublicationColumnOperand)
	if _, unaryOK := row.UnaryOperator(); unaryOK {
		declared("operator", result.NativePublicationColumnUnaryOperator)
	} else {
		declared("operator", result.NativePublicationColumnBinaryOperator)
	}
	declared("overflow", result.NativePublicationColumnOverflow)
	declared("divisor", result.NativePublicationColumnDivisor)
	declared("truthiness", result.NativePublicationColumnTruthiness)
	declared("partition", result.NativePublicationColumnPartition)
	declared("dead_arm", result.NativePublicationColumnDeadArm)
	if reachable, reachableOK := row.DeadArmReachable(); reachableOK {
		columns["dead_arm_reachable"] = strconv.FormatBool(reachable)
	}
	return columns, complete
}

// corpusNativeLiteral is the canonical text of one published constant. Every
// bit pattern a Lua number holds has a text here, infinities and NaN included.
func corpusNativeLiteral(literal keyspace.LiteralValue) (string, bool) {
	switch literal.Kind {
	case keyspace.LiteralBool:
		return strconv.FormatBool(literal.Bool), true
	case keyspace.LiteralInteger:
		return strconv.FormatInt(literal.Integer, 10), true
	case keyspace.LiteralFloat:
		rendered := strconv.FormatFloat(math.Float64frombits(literal.FloatBits), 'g', -1, 64)
		if !strings.ContainsAny(rendered, ".eEnfIN") {
			rendered += ".0"
		}
		return rendered, true
	case keyspace.LiteralString:
		return strconv.Quote(literal.String), true
	default:
		return "", false
	}
}

// corpusNativeRendering renders one row's typed columns in canonical order. It
// is a consumer's reading of declared spellings, not a form the publication
// carries: the row publishes columns, and this is what they read as.
func corpusNativeRendering(columns map[string]string) string {
	parts := make([]string, 0, len(columns))
	for _, name := range corpusNativeColumnOrder {
		if value, published := columns[name]; published {
			parts = append(parts, name+"="+value)
		}
	}
	return strings.Join(parts, " ")
}

func corpusSemanticNativeMismatches(compilation composite.Compilation, expectation *corpusDiagnosticProjectExpectations, result *result.Result) []string {
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
		validity, validityOK := row.Validity()
		if !rowOK || !idOK || !id.Available() || !row.Lane().Valid() || !row.Kind().Valid() || !row.Trust().Valid() || !row.SemanticID().Available() || row.Family() == "" {
			mismatches = append(mismatches, fmt.Sprintf("native row %d has no complete public identity", index))
			continue
		}
		if row.EvidencePointCount() == 0 {
			mismatches = append(mismatches, fmt.Sprintf("native row %d publishes no evidence set", index))
			continue
		}
		columns, columnsOK := corpusNativeColumns(compilation, row)
		if !columnsOK {
			mismatches = append(mismatches, fmt.Sprintf("native row %d publishes a column its declared vocabulary does not hold", index))
			continue
		}
		rows = append(rows, corpusSemanticNativeRow{
			row: row, lane: row.Lane().String(), module: row.Module(), family: row.Family(), key: row.Key(), subject: row.Subject(), term: row.Term(), occurrence: row.Occurrence(),
			columns: columns, rendered: corpusNativeRendering(columns), trust: row.Trust().String(), validity: validity, validityOK: validityOK,
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
	// The typed columns. An authored column is compared against the member's
	// declared spelling, so a manifest names a member of a sealed vocabulary
	// rather than a fragment of a sentence.
	for name, authored := range selector.Columns {
		published, publishes := row.columns[name]
		switch authored {
		case corpusNativeColumnPresent:
			if !publishes {
				return false
			}
		case corpusNativeColumnAbsent:
			if publishes {
				return false
			}
		default:
			if !publishes || published != authored {
				return false
			}
		}
	}
	// The rendered form remains addressable only for publication families the
	// analyzer does not declare yet; the inventory law rejects it for every
	// family that publishes typed columns today.
	if selector.Value != nil && row.rendered != *selector.Value || selector.ValuePrefix != "" && !strings.HasPrefix(row.rendered, selector.ValuePrefix) {
		return false
	}
	for _, part := range selector.ValueContains {
		if !strings.Contains(row.rendered, part) {
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

func corpusSemanticNativeValidityMatches(event, established, revoked string, validity result.NativePublicationValidity) bool {
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
func corpusSemanticAcceptancePolicy(compilation composite.Compilation, expectation *corpusDiagnosticProjectExpectations) (anadiag.DiagnosticPolicy, []string) {
	table, tableOK := composite.Diagnostics(compilation)
	if !tableOK {
		return anadiag.DiagnosticPolicy{}, []string{"sealed diagnostic declaration table unavailable"}
	}
	selected := make(map[anadiag.DiagnosticCode]bool, table.Count())
	severity := make(map[anadiag.DiagnosticCode]anadiag.FindingSeverity)
	unsupported := make([]string, 0)
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK {
			return anadiag.DiagnosticPolicy{}, []string{"sealed diagnostic declaration row unavailable"}
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
			rule, available := corpusSemanticAcceptanceCode(compilation, configured.Code)
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
			level := corpusDiagnosticSeverity(compilation, configured.Severity)
			if level == anadiag.FindingSeverityInvalid {
				unsupported = append(unsupported, fmt.Sprintf("diagnostic rule %q has invalid severity %q", configured.Code, configured.Severity))
				continue
			}
			if *configured.Enabled {
				severity[rule] = level
			}
		}
		for _, expected := range expectation.manifest.Check.Diagnostics {
			rule, available := corpusSemanticAcceptanceCode(compilation, expected.Code)
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

	rules := make([]anadiag.DiagnosticCode, 0, len(selected))
	for rule := range selected {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i] < rules[j] })
	if len(severity) == 0 {
		severity = nil
	}
	return anadiag.DiagnosticPolicy{Enabled: rules, Severity: severity}, unsupported
}

// corpusSemanticAcceptanceCode is intentionally table-driven. It makes no
// diagnostic-family choice: a code is accepted only when the sealed
// declaration row it names installs a producer. The sealed table is read
// directly, so the kit's notion of an accepted family is the one the analyzer
// composed rather than a list restated here.
func corpusSemanticAcceptanceCode(compilation composite.Compilation, code string) (anadiag.DiagnosticCode, bool) {
	table, tableOK := composite.Diagnostics(compilation)
	if !tableOK {
		return anadiag.DiagnosticCodeInvalid, false
	}
	candidate := anadiag.DiagnosticCode(code)
	entry, entryOK := table.ForCode(candidate)
	if !entryOK || !entry.Collectable() {
		return anadiag.DiagnosticCodeInvalid, false
	}
	return candidate, true
}

type corpusSemanticAcceptanceFinding struct {
	finding       anadiag.Finding
	file, message string
	line, column  uint32
	severity      anadiag.FindingSeverity
	code          string
}

func corpusSemanticAcceptanceFindings(report *anadiag.DiagnosticReport) ([]corpusSemanticAcceptanceFinding, []string) {
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
func corpusSemanticAcceptanceMismatches(compilation composite.Compilation, expectation *corpusDiagnosticProjectExpectations, report *anadiag.DiagnosticReport) []string {
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
			if structuredUsed[index] || !corpusSemanticStructuredMatch(compilation, expectation, want, got) {
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
			if !structuredUsed[index] || inlineUsed[index] || !corpusSemanticInlineMatch(compilation, expectation, want, got) {
				continue
			}
			matched = index
			break
		}
		if matched < 0 {
			for index, got := range actual {
				if structuredUsed[index] || inlineUsed[index] || !corpusSemanticInlineMatch(compilation, expectation, want, got) {
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
	if len(unsupported) != 2 {
		t.Fatalf("unsupported fixture contracts=%v, want the two unpreserved non-input surfaces", unsupported)
	}
	unknownPackage := corpusSemanticFixtureInputUnsupported(&corpusDiagnosticProjectExpectations{
		name:     "unknown-package",
		manifest: &corpusDiagnosticManifest{Packages: []string{"time"}},
	})
	if len(unknownPackage) != 1 || !strings.Contains(unknownPackage[0], `"time"`) {
		t.Fatalf("unknown package preflight = %v, want one fenced package contract", unknownPackage)
	}
}

func TestCorpusSemanticFixtureFileAliases(t *testing.T) {
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	project := &corpusDiagnosticProjectExpectations{
		files:       []string{"main.lua", "module.lua"},
		entryFile:   "main.lua",
		entryModule: "main",
	}
	got := corpusSemanticAcceptanceFinding{code: "x", file: "module", line: 4, severity: anadiag.FindingSeverityError}
	want := corpusStructuredDiagnosticExpectation{Code: "x", File: "module.lua", Line: 4, Severity: "error"}
	if !corpusSemanticStructuredMatch(compilation, project, want, got) {
		t.Fatal("module-name finding did not match its .lua contract")
	}
	got.file = "test.lua"
	want.File = "main.lua"
	if !corpusSemanticStructuredMatch(compilation, project, want, got) {
		t.Fatal("test.lua finding did not match the selected entry contract")
	}
	if corpusSemanticInlineMatch(compilation, project, corpusInlineDiagnosticExpectationRow{File: "other.lua", Line: 4, Severity: "error"}, got) {
		t.Fatal("unrelated file alias matched the selected entry")
	}
}

func corpusSemanticStructuredMatch(compilation composite.Compilation, project *corpusDiagnosticProjectExpectations, want corpusStructuredDiagnosticExpectation, got corpusSemanticAcceptanceFinding) bool {
	if got.code != want.Code || !corpusDiagnosticProjectMatchesFile(project, want.File, got.file) || got.line != uint32(want.Line) || got.severity != corpusDiagnosticSeverity(compilation, want.Severity) {
		return false
	}
	return want.Column == 0 || got.column == uint32(want.Column)
}

func corpusSemanticInlineMatch(compilation composite.Compilation, project *corpusDiagnosticProjectExpectations, want corpusInlineDiagnosticExpectationRow, got corpusSemanticAcceptanceFinding) bool {
	spelling, spellingOK := corpusDiagnosticSeveritySpelling(compilation, got.severity)
	if !corpusDiagnosticProjectMatchesFile(project, want.File, got.file) || got.line != uint32(want.Line) || !spellingOK || spelling != want.Severity {
		return false
	}
	return want.Contains == "" || strings.Contains(got.message, want.Contains)
}

func corpusSemanticErrorCount(actual []corpusSemanticAcceptanceFinding) int {
	count := 0
	for _, finding := range actual {
		if finding.severity == anadiag.FindingSeverityError {
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
