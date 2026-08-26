package diagnostic

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type diagnosticTestFixture struct {
	compilation  composite.Compilation
	vocabulary   structure.Table
	declarations diagnostic.Table
	collections  composite.DiagnosticCollections
}

func newDiagnosticTestFixture(t testing.TB) diagnosticTestFixture {
	t.Helper()
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	vocabulary, ok := compilation.Structure()
	if !ok {
		t.Fatal("sealed diagnostic vocabulary unavailable")
	}
	declarations, ok := compilation.Diagnostics()
	if !ok {
		t.Fatal("sealed diagnostic declarations unavailable")
	}
	collections, ok := compilation.DiagnosticCollections()
	if !ok {
		t.Fatal("sealed diagnostic collections unavailable")
	}
	return diagnosticTestFixture{
		compilation:  compilation,
		vocabulary:   vocabulary,
		declarations: declarations,
		collections:  collections,
	}
}

func TestGuardPolarityCollectorMixedTruthsProveNeitherLaw(t *testing.T) {
	truePolarity, falsePolarity := ClassifyGuardPolarity([]valuedomain.Truth{valuedomain.TruthTrue, valuedomain.TruthFalse})
	if truePolarity || falsePolarity {
		t.Fatalf("mixed reachable guard truths classified as polarity true=%t false=%t", truePolarity, falsePolarity)
	}
}

func TestGuardPolarityCollectorZeroReachableRowsProveNeitherLaw(t *testing.T) {
	truePolarity, falsePolarity := ClassifyGuardPolarity(nil)
	if truePolarity || falsePolarity {
		t.Fatalf("zero reachable guard rows classified as polarity true=%t false=%t", truePolarity, falsePolarity)
	}
}

func TestDiagnosticPolicyIsSeparateFromInferenceResultIdentity(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	source, content := reportLawID(1), reportLawID(2)
	off := DiagnosticPolicy{}
	if _, enabled := off.EnabledFor(fixture.declarations, DiagnosticCodeAlwaysTrueGuard); enabled {
		t.Fatal("zero policy enabled semantic collection")
	}
	on := DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}, Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeAlwaysTrueGuard: FindingSeverityHint}}
	severity, enabled := on.EnabledFor(fixture.declarations, DiagnosticCodeAlwaysTrueGuard)
	spelling, spellingOK := FindingSeverityName(fixture.vocabulary, severity)
	if !enabled || severity != FindingSeverityHint || DiagnosticCodeAlwaysTrueGuard.String() != "advice.always_true_guard" || !spellingOK || spelling != "hint" {
		t.Fatal("explicit policy did not enable the declared diagnostic")
	}
	report := NewReport(source, content, fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	if !report.Available() || report.SourceID() != source || report.ResultID() != content || report.FindingCount() != 0 {
		t.Fatal("report is not exactly bound and detached")
	}
	if source != reportLawID(1) || content != reportLawID(2) {
		t.Fatal("report construction changed the bound identities")
	}
}

func reportLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func TestDiagnosticPolicyRejectsAmbiguousAuthority(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	invalid := []DiagnosticPolicy{
		{Enabled: []DiagnosticCode{DiagnosticCodeInvalid}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard, DiagnosticCodeAlwaysTrueGuard}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysFalseGuard, DiagnosticCodeAlwaysFalseGuard}},
		{Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeAlwaysTrueGuard: FindingSeverityHint}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}, Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeAlwaysTrueGuard: FindingSeverityInvalid}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}, Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeInvalid: FindingSeverityHint}},
	}
	for index, policy := range invalid {
		if policy.Valid(fixture.declarations) {
			t.Fatalf("invalid policy %d admitted", index)
		}
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}}).Valid(fixture.declarations) {
		t.Fatal("known unique policy rejected")
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysFalseGuard}}).Valid(fixture.declarations) {
		t.Fatal("installed always-false producer rejected by policy")
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeUnresolvedTypeReference}}).Valid(fixture.declarations) {
		t.Fatal("installed unresolved-type producer rejected by policy")
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeUnresolvedValueReference}}).Valid(fixture.declarations) {
		t.Fatal("installed unresolved-value producer rejected by policy")
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeChannelSelectExhaustiveness}}).Valid(fixture.declarations) {
		t.Fatal("installed channel-select exhaustiveness producer rejected by policy")
	}
	for _, code := range []DiagnosticCode{
		DiagnosticCodeRedundantClaim,
		DiagnosticCodeUnusedLocal,
	} {
		if (DiagnosticPolicy{Enabled: []DiagnosticCode{code}}).Valid(fixture.declarations) {
			t.Fatalf("declared code %q without a producer escaped the policy fence", code.String())
		}
	}
}

// TestDiagnosticDeclarationTableIsPolicyAndDispatchAuthority states that the
// sealed declaration table is the sole authority for both halves of a
// diagnostic's installation: a code is policy-admissible exactly when its row
// declares a producing lane, and a static row is dispatchable exactly by the
// observation population it declares.
func TestDiagnosticDeclarationTableIsPolicyAndDispatchAuthority(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	table := fixture.declarations
	staticPopulations := make(map[schema.Key]struct{}, table.Count())
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK || entry.Code() == DiagnosticCodeInvalid {
			t.Fatalf("declaration row %d is unavailable", position)
		}
		policy := DiagnosticPolicy{Enabled: []DiagnosticCode{entry.Code()}}
		if policy.Valid(fixture.declarations) != entry.Collectable() {
			t.Fatalf("policy admission drifted from the declared lane for %q", entry.Code().String())
		}
		// A row is measured over an Engine observation population exactly when
		// its lane reads one. A post-solve Result row is collected from
		// already-composed facts and names no observation; it declares the
		// verdict category it answers under instead.
		observed := entry.Lane() == diagnostic.LaneBranch || entry.Lane() == diagnostic.LaneStatic
		if entry.Observation().Declared() != observed {
			t.Fatalf("row %q declares observation=%t for lane %d", entry.Code().String(), entry.Observation().Declared(), entry.Lane())
		}
		if entry.Lane() == diagnostic.LaneResult && !entry.VerdictCategory().Available() {
			t.Fatalf("result-lane row %q declares no verdict category", entry.Code().String())
		}
		if !entry.Collectable() {
			continue
		}
		if entry.Lane() != diagnostic.LaneStatic {
			continue
		}
		if _, duplicate := staticPopulations[entry.Observation().Key]; duplicate {
			t.Fatalf("duplicate static observation population %q", entry.Observation().Key)
		}
		staticPopulations[entry.Observation().Key] = struct{}{}
	}
	// Every canonical observation kind projects through the sealed structure
	// table to the diagnostic row that declares its population. The collector
	// therefore consumes one schema identity and holds no mapping of its own.
	for _, kind := range []structure.DiagnosticObservationKind{
		structure.DiagnosticObservationTypeReferenceUnresolved,
		structure.DiagnosticObservationValueReferenceUnresolved,
	} {
		dispatched, dispatchedOK := StaticDeclaration(fixture.declarations, fixture.vocabulary, kind)
		if !dispatchedOK {
			t.Fatalf("static observation kind %d dispatches to no declared row", kind)
		}
		if _, declared := staticPopulations[dispatched.Observation().Key]; !declared {
			t.Fatalf("static observation kind %d dispatched to row %q, which declares no static population", kind, dispatched.Code().String())
		}
	}
	if _, dispatched := StaticDeclaration(fixture.declarations, fixture.vocabulary, structure.DiagnosticObservationBranchCondition); dispatched {
		t.Fatal("a branch population dispatched to a static row")
	}
}

func TestDiagnosticStaticCollectorRejectsUnknownRowKind(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	report := NewReport(reportLawID(1), reportLawID(2), fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	location, locationOK := NewLocation("main.lua", 1, 1, 1, 1)
	if !locationOK {
		t.Fatal("synthetic location unavailable")
	}
	subjects := []StaticSubject{{
		ID: reportLawID(3), FindingID: reportLawID(4),
		Location: location, Kind: structure.DiagnosticObservationKind(255), Name: "unknown",
	}}
	if CollectStatic(report, subjects, DiagnosticPolicy{}) {
		t.Fatal("unknown static observation kind was silently ignored")
	}
}

// diagnosticLawSeed is the row's own position in the sealed declaration table.
// The identities below are synthetic, so they are seeded from the one table
// rather than from a second per-code numbering.
func diagnosticLawSeed(t *testing.T, declarations diagnostic.Table, code DiagnosticCode) byte {
	t.Helper()
	for position := 0; position < declarations.Count(); position++ {
		entry, entryOK := declarations.At(position)
		if entryOK && entry.Code() == code {
			return byte(position + 1)
		}
	}
	t.Fatalf("code %q is not declared", code.String())
	return 0
}

func diagnosticTemplateLawFinding(t *testing.T, fixture diagnosticTestFixture, code DiagnosticCode) Finding {
	t.Helper()
	primary, primaryOK := NewLocation("main.lua", 2, 4, 2, 8)
	if !primaryOK {
		t.Fatal("primary diagnostic location unavailable")
	}
	seed := diagnosticLawSeed(t, fixture.declarations, code)
	severity := FindingSeverityHint
	data := EmptyTemplateData()
	switch code {
	case DiagnosticCodeRedundantClaim:
		subject, subjectOK := NewSemanticName("typed")
		target, targetOK := NewTargetType("string")
		proof, proofOK := NewLocation("main.lua", 2, 9, 2, 13)
		if !subjectOK || !targetOK || !proofOK {
			t.Fatal("redundant claim template payload unavailable")
		}
		data = NewTemplateData(subject, target, TypeCastClaim(), proof)
	case DiagnosticCodeUnresolvedTypeReference:
		subject, subjectOK := NewSemanticName("LocalPoint")
		if !subjectOK {
			t.Fatal("unresolved type subject unavailable")
		}
		data = NewTemplateData(subject, EmptyTarget(), 0)
		severity = FindingSeverityError
	case DiagnosticCodeUnresolvedValueReference:
		subject, subjectOK := NewSemanticName("missing_count")
		if !subjectOK {
			t.Fatal("unresolved value subject unavailable")
		}
		data = NewTemplateData(subject, EmptyTarget(), 0)
		severity = FindingSeverityError
	case DiagnosticCodeUnusedLocal:
		subject, subjectOK := NewSemanticName("unused_local")
		if !subjectOK {
			t.Fatal("unused local subject unavailable")
		}
		data = NewTemplateData(subject, EmptyTarget(), 0)
	case DiagnosticCodeAlwaysTrueGuard, DiagnosticCodeAlwaysFalseGuard:
	default:
		t.Fatalf("test has no template payload for code %q", code.String())
	}
	report := NewReport(reportLawID(seed*10+3), reportLawID(seed*10+4), fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	report.AppendFinding(NewFindingRow(reportLawID(seed*10+1), reportLawID(seed*10+2), code, severity, primary, data))
	finding, findingOK := report.FindingAt(0)
	if !findingOK {
		t.Fatalf("template finding %q unavailable", code.String())
	}
	return finding
}

func TestDiagnosticTemplateRegistryClosedReportLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	tests := []struct {
		code          DiagnosticCode
		severity      FindingSeverity
		message, help string
		evidence      []string
		labels        []string
	}{
		{DiagnosticCodeAlwaysTrueGuard, FindingSeverityHint, "condition is proven always true", "Remove the guard or move the guarded code out of the branch.", []string{"condition is proven to be true on every reachable path"}, []string{"constant guard"}},
		{DiagnosticCodeAlwaysFalseGuard, FindingSeverityHint, "condition is proven always false", "Remove the unreachable branch or invert the guard.", []string{"condition is proven to be false on every reachable path"}, []string{"constant guard"}},
		{DiagnosticCodeRedundantClaim, FindingSeverityHint, "type cast call is redundant; value is already string", "Remove the runtime type claim when the proven source type is sufficient.", []string{"typed is proven to be string before the claim", "claim checks string at this site"}, []string{"claim site", "proven value"}},
		{DiagnosticCodeUnresolvedTypeReference, FindingSeverityError, "unknown type LocalPoint", "Declare the type in scope", []string{"no type named LocalPoint is declared in this scope"}, []string{"unknown type"}},
		{DiagnosticCodeUnresolvedValueReference, FindingSeverityError, "unknown value missing_count", "Declare the value", []string{"no value named missing_count is declared, predeclared, imported, or configured global in this scope"}, []string{"unknown value"}},
		{DiagnosticCodeUnusedLocal, FindingSeverityHint, `local "unused_local" is never read`, "Remove it, use it, or rename it with a leading _ when intentionally unused.", []string{`no read of local "unused_local" was found in this scope`}, []string{"unused local"}},
	}
	for _, test := range tests {
		t.Run(test.code.String(), func(t *testing.T) {
			finding := diagnosticTemplateLawFinding(t, fixture, test.code)
			declared, declaredOK := Declaration(fixture.declarations, test.code)
			if !declaredOK || declared.DefaultSeverity() != test.severity {
				t.Fatalf("declared default severity for %q lost", test.code.String())
			}
			if finding.Code() != test.code || finding.Severity() != test.severity || finding.Message() != test.message || finding.Help() != test.help {
				t.Fatalf("closed template contract lost: code=%q severity=%d message=%q help=%q", finding.Code().String(), finding.Severity(), finding.Message(), finding.Help())
			}
			if finding.EvidenceCount() != len(test.evidence) || finding.LabelCount() != len(test.labels) {
				t.Fatalf("template row counts = evidence %d labels %d, want %d/%d", finding.EvidenceCount(), finding.LabelCount(), len(test.evidence), len(test.labels))
			}
			for index, want := range test.evidence {
				evidence, evidenceOK := finding.EvidenceAt(index)
				if !evidenceOK || evidence.Kind() != "abstract fact" || evidence.Trust() != "proven" || evidence.Reason() != "unspecified" || evidence.Detail() != want {
					t.Fatalf("evidence %d = %#v/%t, want fixed descriptor %q", index, evidence, evidenceOK, want)
				}
			}
			for index, want := range test.labels {
				label, labelOK := finding.LabelAt(index)
				if !labelOK || label.Text() != want {
					t.Fatalf("label %d = %q/%t, want %q", index, label.Text(), labelOK, want)
				}
			}
			first, firstOK := finding.RenderSource("main.lua", "local unused_local = 1\n  typed :: string\n")
			second, secondOK := finding.RenderSource("main.lua", "local unused_local = 1\n  typed :: string\n")
			if !firstOK || !secondOK || first != second || !strings.Contains(first, test.message) || !strings.Contains(first, "because:") || !strings.Contains(first, "help: "+test.help) {
				t.Fatalf("template render is not deterministic and complete: %q/%t %q/%t", first, firstOK, second, secondOK)
			}
		})
	}
}

func TestDiagnosticTemplateRowsRejectInvalidAndForeignPayloads(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	location, locationOK := NewLocation("main.lua", 1, 1, 1, 2)
	if !locationOK {
		t.Fatal("synthetic location unavailable")
	}
	empty := EmptyTemplateData()
	rows := map[string]FindingRow{
		"missing typed payload":    NewFindingRow(reportLawID(71), reportLawID(72), DiagnosticCodeUnresolvedTypeReference, FindingSeverityError, location, empty),
		"foreign code":             NewFindingRow(reportLawID(71), reportLawID(72), DiagnosticCode("advice.undeclared_family"), FindingSeverityError, location, empty),
		"malformed semantic name":  NewFindingRow(reportLawID(71), reportLawID(72), DiagnosticCodeUnresolvedValueReference, FindingSeverityError, location, UnsafeTemplateData("bad\nname", "", 0)),
		"missing proof anchor":     NewFindingRow(reportLawID(71), reportLawID(72), DiagnosticCodeRedundantClaim, FindingSeverityHint, location, UnsafeTemplateData("typed", "string", TypeClaim(), DiagnosticLocation{})),
		"unexpected template data": NewFindingRow(reportLawID(71), reportLawID(72), DiagnosticCodeAlwaysTrueGuard, FindingSeverityHint, location, UnsafeTemplateData("not-a-guard-payload", "", 0)),
	}
	for name, row := range rows {
		t.Run(name, func(t *testing.T) {
			report := NewReport(reportLawID(73), reportLawID(74), fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
			report.AppendFinding(row)
			if report.Available() || report.FindingCount() != 0 {
				t.Fatal("invalid or foreign row escaped the report fence")
			}
			if _, ok := report.FindingAt(0); ok {
				t.Fatal("invalid or foreign row exposed a finding")
			}
		})
	}
}

func TestDiagnosticTemplateSemanticNamesRejectHostileProse(t *testing.T) {
	for _, value := range []string{"LocalPoint", "missing_count", "module.Type2", "_private"} {
		name, nameOK := NewSemanticName(value)
		target, targetOK := NewTargetType(value)
		if !nameOK || !targetOK || !name.Valid() || !target.Valid() {
			t.Fatalf("semantic identifier/path %q was rejected", value)
		}
	}
	for _, value := range []string{
		"", "two words", "typed; help: forged", "typed/child", "typed::string", "typed..proof", ".typed", "typed.", "2typed", "typed-name", "typed\nhelp", "typed\x1b[2J", "týped",
	} {
		name, nameOK := NewSemanticName(value)
		target, targetOK := NewTargetType(value)
		if nameOK || targetOK || name.Valid() || target.Valid() {
			t.Fatalf("hostile prose %q escaped the typed name fence: name=%t target=%t", value, nameOK, targetOK)
		}
	}
}

func TestDiagnosticTemplateHostileTypedDataCannotRender(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	primary, primaryOK := NewLocation("main.lua", 1, 1, 1, 2)
	proof, proofOK := NewLocation("main.lua", 1, 4, 1, 8)
	if !primaryOK || !proofOK {
		t.Fatal("synthetic locations unavailable")
	}
	for _, hostile := range []string{"typed; help: forged", "typed\x1b[2J", "typed value", "typed::string"} {
		t.Run(hostile, func(t *testing.T) {
			report := NewReport(reportLawID(83), reportLawID(84), fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
			report.AppendFinding(NewFindingRow(reportLawID(81), reportLawID(82), DiagnosticCodeRedundantClaim, FindingSeverityHint, primary, UnsafeTemplateData("typed", hostile, TypeClaim(), proof)))
			if report.Available() || report.FindingCount() != 0 {
				t.Fatal("hostile target type entered a report")
			}
			if finding, findingOK := report.FindingAt(0); findingOK || finding.Render() != "" {
				t.Fatal("hostile target type rendered diagnostic prose")
			}
		})
	}
}

func lawAlwaysTrueReport(t testing.TB, fixture diagnosticTestFixture, source, result, id, subject byte, location DiagnosticLocation) *DiagnosticReport {
	t.Helper()
	report := NewReport(reportLawID(source), reportLawID(result), fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	report.AppendFinding(NewFindingRow(reportLawID(id), reportLawID(subject), DiagnosticCodeAlwaysTrueGuard, FindingSeverityHint, location, EmptyTemplateData()))
	return report
}

func TestDiagnosticReportFindingOwnerOrdinalFence(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	location, locationOK := NewLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := lawAlwaysTrueReport(t, fixture, 1, 2, 3, 4, location)
	finding, ok := report.FindingAt(0)
	if !ok {
		t.Fatal("FindingAt(0) unavailable")
	}
	id, idOK := finding.ID()
	subject, subjectOK := finding.SubjectID()
	gotLocation, gotLocationOK := finding.Location()
	line, column := gotLocation.Start()
	if !idOK || !subjectOK || !gotLocationOK || id != reportLawID(3) || subject != reportLawID(4) || finding.Code() != DiagnosticCodeAlwaysTrueGuard || finding.Severity() != FindingSeverityHint || gotLocation.File() != "main.lua" || line != 2 || column != 4 || finding.Message() != "condition is proven always true" || finding.Help() != "Remove the guard or move the guarded code out of the branch." {
		t.Fatal("finding lost exact immutable row")
	}
	if _, ok := report.FindingAt(1); ok {
		t.Fatal("out-of-range report finding admitted")
	}
	if ForgedFinding(report, 2).Code() != DiagnosticCodeInvalid {
		t.Fatal("forged ordinal read a finding")
	}
}

func TestDiagnosticReportAlwaysTrueEvidenceLabelsAndRenderLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	location, locationOK := NewLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := lawAlwaysTrueReport(t, fixture, 11, 12, 13, 14, location)
	finding, findingOK := report.FindingAt(0)
	if !findingOK || finding.EvidenceCount() != 1 || finding.LabelCount() != 1 {
		t.Fatal("exact evidence/label rows were not published")
	}
	evidence, evidenceOK := finding.EvidenceAt(0)
	label, labelOK := finding.LabelAt(0)
	if !evidenceOK || !labelOK || evidence.Kind() != "abstract fact" || evidence.Trust() != "proven" || evidence.Reason() != "unspecified" || evidence.Detail() != "condition is proven to be true on every reachable path" || label.Text() != "constant guard" {
		t.Fatal("evidence or label lost its fixed semantic contract")
	}
	want := "hint[advice.always_true_guard]: condition is proven always true\n" +
		"--> main.lua:2:4\n" +
		"because:\n" +
		"1. proven: condition is proven to be true on every reachable path\n" +
		"help: Remove the guard or move the guarded code out of the branch."
	if finding.Render() != want {
		t.Fatalf("deterministic finding render = %q, want %q", finding.Render(), want)
	}
}

func TestDiagnosticReportRenderSourceExactOrderingLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	location, locationOK := NewLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := lawAlwaysTrueReport(t, fixture, 21, 22, 23, 24, location)
	finding, findingOK := report.FindingAt(0)
	if !findingOK {
		t.Fatal("FindingAt(0) unavailable")
	}
	source := "local flag = true\n  if flag then\n    flag = true\n"
	rendered, renderOK := finding.RenderSource("main.lua", source)
	want := "hint[advice.always_true_guard]: condition is proven always true\n" +
		"--> main.lua:2:4\n" +
		"2 | if flag then\n" +
		"because:\n" +
		"1. proven: condition is proven to be true on every reachable path\n" +
		"help: Remove the guard or move the guarded code out of the branch."
	if !renderOK || rendered != want {
		t.Fatalf("source render = %q/%t, want %q/true", rendered, renderOK, want)
	}
	if !(strings.Index(rendered, "--> main.lua:2:4") < strings.Index(rendered, "2 | if flag then") && strings.Index(rendered, "2 | if flag then") < strings.Index(rendered, "because:")) {
		t.Fatal("source render ordering is not deterministic")
	}
}

func TestDiagnosticReportRenderSourceHostileContextLaw(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	location, locationOK := NewLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := lawAlwaysTrueReport(t, fixture, 31, 32, 33, 34, location)
	finding, findingOK := report.FindingAt(0)
	if !findingOK {
		t.Fatal("FindingAt(0) unavailable")
	}
	if _, ok := finding.RenderSource("other.lua", "local flag = true\nif flag then"); ok {
		t.Fatal("foreign source file rendered a finding")
	}
	if _, ok := finding.RenderSource("main.lua", "local flag = true"); ok {
		t.Fatal("missing source line rendered a finding")
	}
	if _, ok := (Finding{}).RenderSource("main.lua", "local flag = true\nif flag then"); ok {
		t.Fatal("invalid finding rendered through source context")
	}
	foreign := lawAlwaysTrueReport(t, fixture, 41, 42, 43, 44, location)
	foreignFinding, foreignOK := foreign.FindingAt(0)
	if !foreignOK {
		t.Fatal("foreign FindingAt(0) unavailable")
	}
	if _, ok := foreignFinding.RenderSource("main.lua", "local flag = true\nif flag then"); !ok {
		t.Fatal("independent valid report was rejected")
	}
	if _, ok := ForgedFinding(report, 2).RenderSource("main.lua", "local flag = true\nif flag then"); ok {
		t.Fatal("forged ordinal rendered a finding")
	}
}
