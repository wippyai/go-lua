package diagnostic

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/domain/runtimekind"
	typedomain "github.com/wippyai/go-lua/domain/type"
	"github.com/wippyai/go-lua/domain/type/conformance"
)

// conformanceVariantLocation is the one source span these laws report at.
func conformanceVariantLocation(t *testing.T) DiagnosticLocation {
	t.Helper()
	location, ok := NewLocation("main.lua", 2, 18, 2, 26)
	if !ok {
		t.Fatal("law location rejected")
	}
	return location
}

// conformanceVariantFinding publishes one already-answered conformance row and
// returns it as the public finding a consumer reads.
func conformanceVariantFinding(t *testing.T, fixture diagnosticTestFixture, verdict conformance.Verdict, data diagnosticTemplateData) Finding {
	t.Helper()
	report := NewReport(identity.ContentID{1}, identity.ContentID{2}, fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	entry, declared := Declaration(fixture.declarations, typedomain.Code)
	if !declared {
		t.Fatalf("the sealed table declares no row for %s", typedomain.Code)
	}
	if !data.ValidFor(entry, verdict.Ordinal()) {
		t.Fatalf("payload rejected for verdict %d", verdict)
	}
	report.AppendFinding(NewVerdictFindingRow(
		identity.ContentID{3}, identity.ContentID{4}, typedomain.Code, verdict.Ordinal(),
		FindingSeverityError, conformanceVariantLocation(t), data,
	))
	finding, held := report.FindingAt(0)
	if !held {
		t.Fatal("published finding is not readable")
	}
	return finding
}

// TestConformanceRowRendersOneMessagePerVerdict states the declaration half of
// the assignment row: one published code, one rendering per answer the
// conformance judgment gives, each reading only the payload its own answer
// names. Before this, one code carried one message and every answer rendered
// the same sentence.
func TestConformanceRowRendersOneMessagePerVerdict(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	subject, subjectOK := NewSemanticName("value")
	target, targetOK := NewTargetType("string")
	member, memberOK := NewSemanticName("y")
	actual, actualOK := ObservedFamilies(fixture.vocabulary, runtimekind.Bit(runtimekind.Number))
	if !subjectOK || !targetOK || !memberOK || !actualOK {
		t.Fatal("law payload rejected by construction")
	}
	cases := []struct {
		verdict conformance.Verdict
		data    diagnosticTemplateData
		message string
	}{
		{
			conformance.VerdictViolates,
			NewConformanceTemplateData(subject, target, actual, EmptyName()),
			"cannot assign value because it is number, not string",
		},
		{
			conformance.VerdictMayBeNil,
			NewConformanceTemplateData(subject, target, EmptyObservedSpelling(), EmptyName()),
			"cannot assign value because it may be nil",
		},
		{
			conformance.VerdictMemberAbsent,
			NewConformanceTemplateData(EmptyName(), target, EmptyObservedSpelling(), member),
			`object literal is missing required field "y"`,
		},
		{
			conformance.VerdictUnproven,
			NewConformanceTemplateData(subject, target, EmptyObservedSpelling(), EmptyName()),
			"cannot assign value because it comes from any/unknown; no proof shows it satisfies the declared type",
		},
	}
	rendered := make(map[string]conformance.Verdict, len(cases))
	for _, testCase := range cases {
		finding := conformanceVariantFinding(t, fixture, testCase.verdict, testCase.data)
		if finding.Message() != testCase.message {
			t.Errorf("verdict %d message = %q, want %q", testCase.verdict, finding.Message(), testCase.message)
		}
		if prior, duplicate := rendered[finding.Message()]; duplicate {
			t.Errorf("verdicts %d and %d render one message", prior, testCase.verdict)
		}
		rendered[finding.Message()] = testCase.verdict
		if finding.EvidenceCount() == 0 {
			t.Errorf("verdict %d publishes no proof line", testCase.verdict)
		}
		if finding.Help() == "" {
			t.Errorf("verdict %d publishes no help", testCase.verdict)
		}
	}
}

// TestConformanceLiteralIsNamedByItsOwnSpelling states the observed half of a
// violation: a value proved to be one constant renders as that constant, and a
// value narrowed only to families renders as those families. The spellings are
// the value domain's and the runtime vocabulary's; this layer authors neither.
func TestConformanceLiteralIsNamedByItsOwnSpelling(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	subject, subjectOK := NewSemanticName("value")
	target, targetOK := NewTargetType("string")
	if !subjectOK || !targetOK {
		t.Fatal("law payload rejected by construction")
	}
	families, familiesOK := ObservedFamilies(fixture.vocabulary, runtimekind.Bit(runtimekind.Number)|runtimekind.Bit(runtimekind.Boolean))
	if !familiesOK {
		t.Fatal("family spelling rejected")
	}
	finding := conformanceVariantFinding(t, fixture, conformance.VerdictViolates, NewConformanceTemplateData(subject, target, families, EmptyName()))
	if !strings.Contains(finding.Message(), "boolean or number") {
		t.Fatalf("family spelling = %q", finding.Message())
	}
}

// TestDirectCallViolationRendersOwnerIssuedSemanticRoles states the focused
// direct-call contract. The renderer receives an argument role, a parameter
// role, and an observed literal as typed payloads; it does not infer ordinal,
// callee, or evidence prose from the source/result text.
func TestDirectCallViolationRendersOwnerIssuedSemanticRoles(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	entry, declared := Declaration(fixture.declarations, typedomain.CallArgumentCode)
	if !declared {
		t.Fatal("the sealed table declares no direct-call conformance row")
	}
	argumentSubject, subjectOK := NewSemanticName("x")
	callee, calleeOK := NewSemanticName("takes_string")
	target, targetOK := NewTargetType("string")
	actual, actualOK := ObservedLiteral(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 5})
	argument, argumentOK := NewCallArgument(0, argumentSubject)
	parameter, parameterOK := NewCallParameter(0, callee)
	if !subjectOK || !calleeOK || !targetOK || !actualOK || !argumentOK || !parameterOK {
		t.Fatal("direct-call semantic payload rejected by construction")
	}
	data := NewCallConformanceTemplateData(argument, argumentSubject, parameter, target, actual)
	if !data.ValidFor(entry, conformance.VerdictViolates.Ordinal()) {
		t.Fatal("direct-call semantic payload refused by its declaration")
	}
	emptySubject, _ := NewSemanticName("")
	missingSubject := NewCallConformanceTemplateData(argument, emptySubject, parameter, target, actual)
	if missingSubject.ValidFor(entry, conformance.VerdictViolates.Ordinal()) {
		t.Fatal("direct-call violation invented an argument subject")
	}
	if _, ok := NewCallParameter(0, EmptyName()); ok {
		t.Fatal("direct-call parameter role admitted without an authored callee")
	}
	report := NewReport(identity.ContentID{41}, identity.ContentID{42}, fixture.compilation, fixture.vocabulary, fixture.declarations, fixture.collections)
	location, locationOK := NewLocation("main.lua", 3, 21, 3, 35)
	if !locationOK {
		t.Fatal("direct-call source location unavailable")
	}
	report.AppendFinding(NewVerdictFindingRow(
		identity.ContentID{43}, identity.ContentID{44}, typedomain.CallArgumentCode,
		conformance.VerdictViolates.Ordinal(), FindingSeverityError, location, data,
	))
	finding, held := report.FindingAt(0)
	if !held {
		t.Fatal("direct-call finding was not published")
	}
	if finding.Message() != "argument 1 (x) is 5, not string" {
		t.Fatalf("direct-call message = %q", finding.Message())
	}
	if finding.Help() != "Pass `x` as a value compatible with the parameter type, or change the callee signature if that argument is valid." {
		t.Fatalf("direct-call help = %q", finding.Help())
	}
	wantEvidence := []string{
		"argument 1 (x) has literal value 5",
		"takes_string parameter 1 expects string",
		"no proof on this path shows x satisfies the parameter type",
	}
	if finding.EvidenceCount() != len(wantEvidence) {
		t.Fatalf("direct-call evidence count = %d, want %d", finding.EvidenceCount(), len(wantEvidence))
	}
	for index, want := range wantEvidence {
		evidence, evidenceOK := finding.EvidenceAt(index)
		if !evidenceOK || evidence.Detail() != want {
			t.Fatalf("direct-call evidence %d = %q, want %q", index+1, evidence.Detail(), want)
		}
	}
	rendered, renderedOK := finding.RenderSource("main.lua", "local function takes_string(s: string): string return s end\nlocal x: number = 5\nreturn takes_string(x)\n")
	if !renderedOK {
		t.Fatal("direct-call source render was unavailable")
	}
	ordered := []string{
		"error[type.call.direct.argument_type]: argument 1 (x) is 5, not string",
		"--> main.lua:3:21",
		"return takes_string(x)",
		"because:",
		"1. proven: argument 1 (x) has literal value 5",
		"2. claimed: takes_string parameter 1 expects string",
		"3. missing proof: no proof on this path shows x satisfies the parameter type",
		"help: Pass `x` as a value compatible with the parameter type, or change the callee signature if that argument is valid.",
	}
	last := 0
	for _, part := range ordered {
		index := strings.Index(rendered[last:], part)
		if index < 0 {
			t.Fatalf("direct-call render missing or out of order %q: %s", part, rendered)
		}
		last += index + len(part)
	}
}

// TestConformanceVariantRefusesAnotherAnswersPayload states the payload fence
// between answers. An absent member names a field and no observed value; a
// containment violation names an observed value and no field. Filling in the
// other answer's payload is a producer defect, and the row refuses it rather
// than rendering a sentence nothing established.
func TestConformanceVariantRefusesAnotherAnswersPayload(t *testing.T) {
	fixture := newDiagnosticTestFixture(t)
	entry, declared := Declaration(fixture.declarations, typedomain.Code)
	if !declared {
		t.Fatalf("the sealed table declares no row for %s", typedomain.Code)
	}
	subject, _ := NewSemanticName("value")
	target, _ := NewTargetType("string")
	member, _ := NewSemanticName("y")
	actual, _ := ObservedFamilies(fixture.vocabulary, runtimekind.Bit(runtimekind.Number))
	absentWithValue := NewConformanceTemplateData(subject, target, actual, member)
	if absentWithValue.ValidFor(entry, conformance.VerdictMemberAbsent.Ordinal()) {
		t.Fatal("an absent-member payload carrying an observed value was admitted")
	}
	violatesWithoutValue := NewConformanceTemplateData(subject, target, EmptyObservedSpelling(), EmptyName())
	if violatesWithoutValue.ValidFor(entry, conformance.VerdictViolates.Ordinal()) {
		t.Fatal("a containment violation with no observed value was admitted")
	}
	if absentWithValue.ValidFor(entry, 0) {
		t.Fatal("a row that renders per verdict answered under no verdict at all")
	}
}
