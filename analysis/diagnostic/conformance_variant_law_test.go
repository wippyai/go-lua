package diagnostic

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
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
