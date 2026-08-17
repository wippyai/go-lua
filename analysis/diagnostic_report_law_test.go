package analysis

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
)

func TestGuardPolarityCollectorMixedTruthsProveNeitherLaw(t *testing.T) {
	if got := classifyDiagnosticGuardPolarity([]valuedomain.Truth{valuedomain.TruthTrue, valuedomain.TruthFalse}); got != diagnosticGuardPolarityInvalid {
		t.Fatalf("mixed reachable guard truths classified as polarity %d", got)
	}
}

func TestGuardPolarityCollectorZeroReachableRowsProveNeitherLaw(t *testing.T) {
	if got := classifyDiagnosticGuardPolarity(nil); got != diagnosticGuardPolarityInvalid {
		t.Fatalf("zero reachable guard rows classified as polarity %d", got)
	}
}

func TestDiagnosticPolicyIsSeparateFromInferenceResultIdentity(t *testing.T) {
	result := &Result{source: artifactResultLawID(1), content: artifactResultLawID(2), bodies: []resultBody{{id: artifactResultLawID(3)}}, sealed: true}
	if !result.valid() {
		t.Fatal("synthetic inference Result unavailable")
	}
	off := DiagnosticPolicy{}
	if _, enabled := off.enabled(DiagnosticCodeAlwaysTrueGuard); enabled {
		t.Fatal("zero policy enabled semantic collection")
	}
	on := DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}, Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeAlwaysTrueGuard: FindingSeverityHint}}
	severity, enabled := on.enabled(DiagnosticCodeAlwaysTrueGuard)
	spelling, spellingOK := findingSeveritySpelling(severity)
	if !enabled || severity != FindingSeverityHint || DiagnosticCodeAlwaysTrueGuard.String() != "advice.always_true_guard" || !spellingOK || spelling != "hint" {
		t.Fatal("explicit policy did not enable the declared diagnostic")
	}
	report := &DiagnosticReport{source: result.SourceID(), result: result.ContentID(), sealed: true}
	if !report.Available() || report.SourceID() != result.SourceID() || report.ResultID() != result.ContentID() || report.FindingCount() != 0 {
		t.Fatal("report is not exactly bound and detached")
	}
	if result.ContentID() != artifactResultLawID(2) || result.BodyCount() != 1 {
		t.Fatal("report policy changed inference Result")
	}
}

func TestDiagnosticPolicyRejectsAmbiguousAuthority(t *testing.T) {
	invalid := []DiagnosticPolicy{
		{Enabled: []DiagnosticCode{DiagnosticCodeInvalid}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard, DiagnosticCodeAlwaysTrueGuard}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysFalseGuard, DiagnosticCodeAlwaysFalseGuard}},
		{Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeAlwaysTrueGuard: FindingSeverityHint}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}, Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeAlwaysTrueGuard: FindingSeverityInvalid}},
		{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}, Severity: map[DiagnosticCode]FindingSeverity{DiagnosticCodeInvalid: FindingSeverityHint}},
	}
	for index, policy := range invalid {
		if policy.Valid() {
			t.Fatalf("invalid policy %d admitted", index)
		}
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysTrueGuard}}).Valid() {
		t.Fatal("known unique policy rejected")
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeAlwaysFalseGuard}}).Valid() {
		t.Fatal("installed always-false producer rejected by policy")
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeUnresolvedTypeReference}}).Valid() {
		t.Fatal("installed unresolved-type producer rejected by policy")
	}
	if !(DiagnosticPolicy{Enabled: []DiagnosticCode{DiagnosticCodeUnresolvedValueReference}}).Valid() {
		t.Fatal("installed unresolved-value producer rejected by policy")
	}
	for _, code := range []DiagnosticCode{
		DiagnosticCodeRedundantClaim,
		DiagnosticCodeUnusedLocal,
	} {
		if (DiagnosticPolicy{Enabled: []DiagnosticCode{code}}).Valid() {
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
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	staticPopulations := make(map[schema.Key]struct{}, table.Count())
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK || entry.Code() == DiagnosticCodeInvalid {
			t.Fatalf("declaration row %d is unavailable", position)
		}
		policy := DiagnosticPolicy{Enabled: []DiagnosticCode{entry.Code()}}
		if policy.Valid() != entry.Collectable() {
			t.Fatalf("policy admission drifted from the declared lane for %q", entry.Code().String())
		}
		if !entry.Collectable() {
			if entry.Observation().Declared() {
				t.Fatalf("row %q without a producer declares an observation population", entry.Code().String())
			}
			continue
		}
		if !entry.Observation().Declared() {
			t.Fatalf("producing row %q declares no observation population", entry.Code().String())
		}
		if entry.Lane() != diagnostic.LaneStatic {
			continue
		}
		if _, duplicate := staticPopulations[entry.Observation().Key]; duplicate {
			t.Fatalf("duplicate static observation population %q", entry.Observation().Key)
		}
		staticPopulations[entry.Observation().Key] = struct{}{}
	}
	// The artifact numbers the same populations the declaration does, so every
	// compiled observation kind dispatches to the row that declares it. This is
	// the adoption seam the pin law holds open: a collector holding an ordinal
	// reaches a declared row without a mapping of its own.
	for _, kind := range []programartifact.DiagnosticObservationKind{
		programartifact.DiagnosticObservationTypeReferenceUnresolved,
		programartifact.DiagnosticObservationValueReferenceUnresolved,
	} {
		dispatched, dispatchedOK := staticDiagnosticDeclaration(kind)
		if !dispatchedOK {
			t.Fatalf("static observation kind %d dispatches to no declared row", kind)
		}
		if _, declared := staticPopulations[dispatched.Observation().Key]; !declared {
			t.Fatalf("static observation kind %d dispatched to row %q, which declares no static population", kind, dispatched.Code().String())
		}
	}
	if _, dispatched := staticDiagnosticDeclaration(programartifact.DiagnosticObservationBranchCondition); dispatched {
		t.Fatal("a branch population dispatched to a static row")
	}
}

func TestDiagnosticStaticCollectorRejectsUnknownRowKind(t *testing.T) {
	report := &DiagnosticReport{}
	receipt := &artifactResultReceipt{staticObservations: []compiledObservation{{kind: programartifact.DiagnosticObservationKind(255)}}}
	if collectStaticDiagnosticFindings(report, receipt, DiagnosticPolicy{}) {
		t.Fatal("unknown static observation kind was silently ignored")
	}
}

// diagnosticLawSeed is the row's own position in the sealed declaration table.
// The identities below are synthetic, so they are seeded from the one table
// rather than from a second per-code numbering.
func diagnosticLawSeed(t *testing.T, code DiagnosticCode) byte {
	t.Helper()
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		t.Fatal("sealed diagnostic table unavailable")
	}
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if entryOK && entry.Code() == code {
			return byte(position + 1)
		}
	}
	t.Fatalf("code %q is not declared", code.String())
	return 0
}

func diagnosticTemplateLawFinding(t *testing.T, code DiagnosticCode) Finding {
	t.Helper()
	primary, primaryOK := newDiagnosticLocation("main.lua", 2, 4, 2, 8)
	if !primaryOK {
		t.Fatal("primary diagnostic location unavailable")
	}
	seed := diagnosticLawSeed(t, code)
	row := diagnosticFinding{id: artifactResultLawID(seed*10 + 1), subject: artifactResultLawID(seed*10 + 2), code: code, severity: FindingSeverityHint, location: primary}
	switch code {
	case DiagnosticCodeRedundantClaim:
		subject, subjectOK := newDiagnosticSemanticName("typed")
		target, targetOK := newDiagnosticTargetType("string")
		proof, proofOK := newDiagnosticLocation("main.lua", 2, 9, 2, 13)
		if !subjectOK || !targetOK || !proofOK {
			t.Fatal("redundant claim template payload unavailable")
		}
		row.data = diagnosticTemplateData{subject: subject, target: target, claim: diagnosticClaimFormTypeCastCall, proof: proof}
	case DiagnosticCodeUnresolvedTypeReference:
		subject, subjectOK := newDiagnosticSemanticName("LocalPoint")
		if !subjectOK {
			t.Fatal("unresolved type subject unavailable")
		}
		row.data = diagnosticTemplateData{subject: subject}
		row.severity = FindingSeverityError
	case DiagnosticCodeUnresolvedValueReference:
		subject, subjectOK := newDiagnosticSemanticName("missing_count")
		if !subjectOK {
			t.Fatal("unresolved value subject unavailable")
		}
		row.data = diagnosticTemplateData{subject: subject}
		row.severity = FindingSeverityError
	case DiagnosticCodeUnusedLocal:
		subject, subjectOK := newDiagnosticSemanticName("unused_local")
		if !subjectOK {
			t.Fatal("unused local subject unavailable")
		}
		row.data = diagnosticTemplateData{subject: subject}
		row.severity = FindingSeverityHint
	case DiagnosticCodeAlwaysTrueGuard, DiagnosticCodeAlwaysFalseGuard:
	default:
		t.Fatalf("test has no template payload for code %q", code.String())
	}
	report := &DiagnosticReport{source: artifactResultLawID(seed*10 + 3), result: artifactResultLawID(seed*10 + 4), findings: []diagnosticFinding{row}, sealed: true}
	finding, findingOK := report.FindingAt(0)
	if !findingOK {
		t.Fatalf("template finding %q unavailable", code.String())
	}
	return finding
}

func TestDiagnosticTemplateRegistryClosedReportLaw(t *testing.T) {
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
			finding := diagnosticTemplateLawFinding(t, test.code)
			declared, declaredOK := diagnosticDeclaration(test.code)
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
	location, locationOK := newDiagnosticLocation("main.lua", 1, 1, 1, 2)
	if !locationOK {
		t.Fatal("synthetic location unavailable")
	}
	base := diagnosticFinding{id: artifactResultLawID(71), subject: artifactResultLawID(72), severity: FindingSeverityError, location: location}
	missingPayload := base
	missingPayload.code = DiagnosticCodeUnresolvedTypeReference
	foreign := base
	foreign.code = DiagnosticCode("advice.undeclared_family")
	malformedName := base
	malformedName.code = DiagnosticCodeUnresolvedValueReference
	malformedName.data.subject = diagnosticSemanticName{value: "bad\nname"}
	missingProof := base
	missingProof.code = DiagnosticCodeRedundantClaim
	missingProof.severity = FindingSeverityHint
	missingProof.data.subject = diagnosticSemanticName{value: "typed"}
	missingProof.data.target = diagnosticTargetType{value: "string"}
	missingProof.data.claim = diagnosticClaimFormTypeClaim
	unexpectedPayload := base
	unexpectedPayload.code = DiagnosticCodeAlwaysTrueGuard
	unexpectedPayload.severity = FindingSeverityHint
	unexpectedPayload.data.subject = diagnosticSemanticName{value: "not-a-guard-payload"}
	for name, row := range map[string]diagnosticFinding{
		"missing typed payload":    missingPayload,
		"foreign code":             foreign,
		"malformed semantic name":  malformedName,
		"missing proof anchor":     missingProof,
		"unexpected template data": unexpectedPayload,
	} {
		t.Run(name, func(t *testing.T) {
			report := &DiagnosticReport{source: artifactResultLawID(73), result: artifactResultLawID(74), findings: []diagnosticFinding{row}, sealed: true}
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
		name, nameOK := newDiagnosticSemanticName(value)
		target, targetOK := newDiagnosticTargetType(value)
		if !nameOK || !targetOK || !name.valid() || !target.valid() {
			t.Fatalf("semantic identifier/path %q was rejected", value)
		}
	}
	for _, value := range []string{
		"", "two words", "typed; help: forged", "typed/child", "typed::string", "typed..proof", ".typed", "typed.", "2typed", "typed-name", "typed\nhelp", "typed\x1b[2J", "týped",
	} {
		name, nameOK := newDiagnosticSemanticName(value)
		target, targetOK := newDiagnosticTargetType(value)
		if nameOK || targetOK || name.valid() || target.valid() {
			t.Fatalf("hostile prose %q escaped the typed name fence: name=%t target=%t", value, nameOK, targetOK)
		}
	}
}

func TestDiagnosticTemplateHostileTypedDataCannotRender(t *testing.T) {
	primary, primaryOK := newDiagnosticLocation("main.lua", 1, 1, 1, 2)
	proof, proofOK := newDiagnosticLocation("main.lua", 1, 4, 1, 8)
	if !primaryOK || !proofOK {
		t.Fatal("synthetic locations unavailable")
	}
	for _, hostile := range []string{"typed; help: forged", "typed\x1b[2J", "typed value", "typed::string"} {
		t.Run(hostile, func(t *testing.T) {
			row := diagnosticFinding{
				id: artifactResultLawID(81), subject: artifactResultLawID(82), code: DiagnosticCodeRedundantClaim, severity: FindingSeverityHint, location: primary,
				data: diagnosticTemplateData{subject: diagnosticSemanticName{value: "typed"}, target: diagnosticTargetType{value: hostile}, claim: diagnosticClaimFormTypeClaim, proof: proof},
			}
			report := &DiagnosticReport{source: artifactResultLawID(83), result: artifactResultLawID(84), findings: []diagnosticFinding{row}, sealed: true}
			if report.Available() || report.FindingCount() != 0 {
				t.Fatal("hostile target type entered a report")
			}
			if finding, findingOK := report.FindingAt(0); findingOK || finding.Render() != "" {
				t.Fatal("hostile target type rendered diagnostic prose")
			}
		})
	}
}

func TestDiagnosticReportFindingOwnerOrdinalFence(t *testing.T) {
	location, locationOK := newDiagnosticLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := &DiagnosticReport{source: artifactResultLawID(1), result: artifactResultLawID(2), findings: []diagnosticFinding{{id: artifactResultLawID(3), subject: artifactResultLawID(4), code: DiagnosticCodeAlwaysTrueGuard, severity: FindingSeverityHint, location: location}}, sealed: true}
	finding, ok := report.FindingAt(0)
	if !ok {
		t.Fatal("FindingAt(0) unavailable")
	}
	id, idOK := finding.ID()
	subject, subjectOK := finding.SubjectID()
	gotLocation, gotLocationOK := finding.Location()
	line, column := gotLocation.Start()
	if !idOK || !subjectOK || !gotLocationOK || id != artifactResultLawID(3) || subject != artifactResultLawID(4) || finding.Code() != DiagnosticCodeAlwaysTrueGuard || finding.Severity() != FindingSeverityHint || gotLocation.File() != "main.lua" || line != 2 || column != 4 || finding.Message() != "condition is proven always true" || finding.Help() != "Remove the guard or move the guarded code out of the branch." {
		t.Fatal("finding lost exact immutable row")
	}
	if _, ok := report.FindingAt(1); ok {
		t.Fatal("out-of-range report finding admitted")
	}
	if (&Finding{owner: report, ordinal: 2}).Code() != DiagnosticCodeInvalid {
		t.Fatal("forged ordinal read a finding")
	}
}

func TestDiagnosticReportAlwaysTrueEvidenceLabelsAndRenderLaw(t *testing.T) {
	location, locationOK := newDiagnosticLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := &DiagnosticReport{
		source: artifactResultLawID(11), result: artifactResultLawID(12), sealed: true,
		findings: []diagnosticFinding{{
			id: artifactResultLawID(13), subject: artifactResultLawID(14), code: DiagnosticCodeAlwaysTrueGuard, severity: FindingSeverityHint, location: location,
		}},
	}
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
	location, locationOK := newDiagnosticLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := &DiagnosticReport{source: artifactResultLawID(21), result: artifactResultLawID(22), sealed: true, findings: []diagnosticFinding{{id: artifactResultLawID(23), subject: artifactResultLawID(24), code: DiagnosticCodeAlwaysTrueGuard, severity: FindingSeverityHint, location: location}}}
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
	location, locationOK := newDiagnosticLocation("main.lua", 2, 4, 2, 8)
	if !locationOK {
		t.Fatal("synthetic diagnostic location unavailable")
	}
	report := &DiagnosticReport{source: artifactResultLawID(31), result: artifactResultLawID(32), sealed: true, findings: []diagnosticFinding{{id: artifactResultLawID(33), subject: artifactResultLawID(34), code: DiagnosticCodeAlwaysTrueGuard, severity: FindingSeverityHint, location: location}}}
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
	foreign := &DiagnosticReport{source: artifactResultLawID(41), result: artifactResultLawID(42), sealed: true, findings: []diagnosticFinding{{id: artifactResultLawID(43), subject: artifactResultLawID(44), code: DiagnosticCodeAlwaysTrueGuard, severity: FindingSeverityHint, location: location}}}
	foreignFinding, foreignOK := foreign.FindingAt(0)
	if !foreignOK {
		t.Fatal("foreign FindingAt(0) unavailable")
	}
	if _, ok := foreignFinding.RenderSource("main.lua", "local flag = true\nif flag then"); !ok {
		t.Fatal("independent valid report was rejected")
	}
	if _, ok := (Finding{owner: report, ordinal: 2}).RenderSource("main.lua", "local flag = true\nif flag then"); ok {
		t.Fatal("forged ordinal rendered a finding")
	}
}
