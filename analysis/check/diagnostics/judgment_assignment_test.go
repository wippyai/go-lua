package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRenderAssignmentJudgmentConcreteMismatch(t *testing.T) {
	item := assignmentJudgmentFixture(typ.String, typ.Number, judgment.VerdictRefuted)

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, "cannot assign string to number") {
		t.Fatalf("message = %q, want concrete mismatch", d.Message)
	}
	if len(d.Labels) != 2 || d.Labels[0].Message != labelAssignedValue || d.Labels[1].Message != labelDeclaredType {
		t.Fatalf("labels = %#v, want assigned value and declared type", d.Labels)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "assigned value has type string") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "n is declared as number") {
		t.Fatalf("evidence = %#v, want actual and declared evidence", d.Explanation.Evidence())
	}
}

func TestRenderAssignmentJudgmentMessageUsesDeclaredAliasLabel(t *testing.T) {
	expected := typ.Func().Returns(typetable.NewRecord().Field("answer", typ.String).Build()).Build()
	actual := typ.Func().Returns(typ.Nil).Build()
	item := assignmentJudgmentFixture(actual, expected, judgment.VerdictRefuted)
	item.Subject = item.Subject.WithLabel("f")
	item.Actual = item.Actual.WithLabel("M.run")
	item.Expected = item.Expected.WithLabel("fun() -> Res")

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if !strings.Contains(d.Message, "not fun() -> Res") {
		t.Fatalf("message = %q, want declared alias label", d.Message)
	}
	if strings.Contains(d.Message, "{answer: string}") {
		t.Fatalf("message = %q, should not expand declared alias", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "f is declared as fun() -> Res") {
		t.Fatalf("evidence = %#v, want declared alias evidence", d.Explanation.Evidence())
	}
}

func TestProduceAssignmentJudgmentDiagnosticsFromResult(t *testing.T) {
	result := runDiagnosticsResult(t, `local n: number = "bad"
local ok: number = 1`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceAssignmentJudgmentDiagnostics(result, "main.lua")
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one assignment judgment diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want assignment error", d)
	}
	if !strings.Contains(d.Message, `cannot assign "bad" to number`) {
		t.Fatalf("message = %q, want assignment mismatch", d.Message)
	}
	if d.Position.Line != 1 {
		t.Fatalf("position = %s:%d:%d, want line 1", d.Position.File, d.Position.Line, d.Position.Column)
	}
}

func TestProduceAssignmentJudgmentDiagnosticsKeepsAnyAsMissingProof(t *testing.T) {
	result := runDiagnosticsResult(t, `local raw: any = 1
local s: string = raw`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceAssignmentJudgmentDiagnostics(result, "main.lua")
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one assignment judgment diagnostic: %#v", len(diags), diags)
	}
	if !diagnosticEvidenceContains(diags[0].Explanation.Evidence(), "no proof on this path shows raw satisfies the declared type") {
		t.Fatalf("evidence = %#v, want missing proof", diags[0].Explanation.Evidence())
	}
	if !diagnosticEvidenceContains(diags[0].Explanation.Evidence(), "raw comes from any/unknown") {
		t.Fatalf("evidence = %#v, want precision-boundary proof", diags[0].Explanation.Evidence())
	}
}

func TestProduceUsesAssignmentJudgmentsByDefault(t *testing.T) {
	result := runDiagnosticsResult(t, `local raw: any = 1
local s: string = raw`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := ProduceWithConfig(result, Config{})
	var assignment []diagnostic.Diagnostic
	for _, d := range diags {
		if d.Code == CodeAssignmentType {
			assignment = append(assignment, d)
		}
	}
	if len(assignment) != 1 {
		t.Fatalf("assignment diagnostics = %d, want one: %#v", len(assignment), diags)
	}
	if !diagnosticEvidenceContains(assignment[0].Explanation.Evidence(), "no proof on this path shows raw satisfies the declared type") {
		t.Fatalf("evidence = %#v, want judgment missing proof", assignment[0].Explanation.Evidence())
	}
}

func TestProduceAssignmentJudgmentsIncludeOptionalTargetByDefault(t *testing.T) {
	result := runDiagnosticsResult(t, `type Bag = {name: string}
function update(bag: Bag?): ()
	bag.name = "ok"
end`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := ProduceWithConfig(result, Config{})
	var optional []diagnostic.Diagnostic
	for _, d := range diags {
		if d.Code == CodeOptionalAssignmentTarget {
			optional = append(optional, d)
		}
	}
	if len(optional) != 1 {
		t.Fatalf("optional target diagnostics = %d, want one: %#v", len(optional), diags)
	}
	if !strings.Contains(optional[0].Message, "cannot assign through optional bag without nil check") {
		t.Fatalf("message = %q", optional[0].Message)
	}
	if !diagnosticEvidenceContains(optional[0].Explanation.Evidence(), "writing bag.name requires its container to be non-nil") {
		t.Fatalf("evidence = %#v", optional[0].Explanation.Evidence())
	}
}

func TestProduceDefaultAssignmentJudgmentsReportsDynamicVariantAlias(t *testing.T) {
	result := runDiagnosticsResultFull(t, `type FileSlot = {
	kind: "file",
	path: string,
}
type TimerSlot = {
	kind: "timer",
	seconds: number,
}
type Slot = {
	value: FileSlot | TimerSlot,
}
type Slots = {[string]: Slot}

local slots: Slots = {
	active = {
		value = {kind = "file", path = "/tmp/active"},
	},
}
local active = slots.active
local key = "active"

if active.value.kind == "file" then
	slots[key].value = {kind = "timer", seconds = 5}
	local stale_path: string = active.value.path
end`, nil, signaturelookup.Source{})
	if result == nil {
		t.Fatal("RootResult nil")
	}

	direct := produceAssignmentJudgmentDiagnostics(result, "main.lua")
	if len(direct) != 1 {
		t.Fatalf("direct assignment diagnostics = %d, want one stale alias diagnostic: %#v", len(direct), direct)
	}
	diags := Produce(result)
	var assignment []diagnostic.Diagnostic
	for _, d := range diags {
		if d.Code == CodeAssignmentType {
			assignment = append(assignment, d)
		}
	}
	if len(assignment) != 1 {
		t.Fatalf("assignment diagnostics = %d, want one stale alias diagnostic: %#v", len(assignment), diags)
	}
	if !strings.Contains(assignment[0].Message, "active.value.path") {
		t.Fatalf("message = %q, want stale alias path", assignment[0].Message)
	}
}

func TestRenderAssignmentJudgmentUnknownIncludesMissingProof(t *testing.T) {
	item := assignmentJudgmentFixture(typ.Any, typ.String, judgment.VerdictUnknown)
	item.Actual = item.Actual.WithLabel("raw")

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if !strings.Contains(d.Message, "cannot assign raw because it is any, not string") {
		t.Fatalf("message = %q, want boundary mismatch", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "no proof on this path shows raw satisfies the declared type") {
		t.Fatalf("evidence = %#v, want missing proof", d.Explanation.Evidence())
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "n is declared as string") {
		t.Fatalf("evidence = %#v, want target declaration evidence", d.Explanation.Evidence())
	}
}

func TestRenderAssignmentJudgmentOptionalSourceUsesNilHelp(t *testing.T) {
	item := assignmentJudgmentFixture(typ.MaterializeOptional(typ.String), typ.String, judgment.VerdictUnknown)
	item.Actual = item.Actual.WithLabel("n")
	item.Evidence[2].Detail = judgment.MayBeNilEvidenceDetail()

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if !strings.Contains(d.Message, "cannot assign n because it may be nil") {
		t.Fatalf("message = %q, want nil-specific assignment message", d.Message)
	}
	if !strings.Contains(d.Help, "Guard `n` with a nil check") {
		t.Fatalf("help = %q, want nil-specific help", d.Help)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "no guard on this path proves n is non-nil") {
		t.Fatalf("evidence = %#v, want nil guard evidence", d.Explanation.Evidence())
	}
}

func TestRenderAssignmentJudgmentRefutedOptionalSourceIncludesMissingGuard(t *testing.T) {
	item := assignmentJudgmentFixture(typ.MaterializeOptional(typ.Number), typ.Number, judgment.VerdictRefuted)
	item.Actual = item.Actual.WithLabel("h")
	item.Evidence[2].Trust = judgment.EvidenceTrustRefuted
	item.Evidence[2].Detail = judgment.MayBeNilEvidenceDetail()

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if !strings.Contains(d.Message, "cannot assign h because it may be nil") {
		t.Fatalf("message = %q, want nil-specific assignment message", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "no guard on this path proves h is non-nil") {
		t.Fatalf("evidence = %#v, want nil guard evidence", d.Explanation.Evidence())
	}
}

func TestRenderAssignmentJudgmentPrecisionBoundaryIncludesBoundaryEvidence(t *testing.T) {
	item := assignmentJudgmentFixture(typ.String, typ.String, judgment.VerdictUnknown)
	item.Actual = item.Actual.WithLabel("raw")
	item.Evidence = append(item.Evidence, judgment.Evidence{
		Kind:  judgment.EvidencePrecisionBoundary,
		Trust: judgment.EvidenceTrustUnknown,
	})

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "raw comes from any/unknown") {
		t.Fatalf("evidence = %#v, want precision boundary evidence", d.Explanation.Evidence())
	}
	if !strings.Contains(d.Message, "raw comes from any/unknown; no proof shows it satisfies the declared type") {
		t.Fatalf("message = %q, want same-rendered-type validation proof wording", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "no proof on this path shows raw satisfies the declared type") {
		t.Fatalf("evidence = %#v, want missing proof evidence", d.Explanation.Evidence())
	}
}

func TestRenderAssignmentJudgmentExactNilDoesNotSayMayBeNil(t *testing.T) {
	item := assignmentJudgmentFixture(typ.Nil, typ.String, judgment.VerdictRefuted)
	item.Actual = item.Actual.WithLabel("n")

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want exact nil mismatch", d.Message)
	}
	if !strings.Contains(d.Message, "cannot assign n because it is nil, not string") {
		t.Fatalf("message = %q, want exact nil mismatch", d.Message)
	}
}

func TestRenderAssignmentJudgmentMissingRequiredField(t *testing.T) {
	actual := typetable.NewRecord().Field("x", typ.Number).Build()
	expected := typetable.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	item := assignmentJudgmentFixture(actual, expected, judgment.VerdictRefuted)
	item.Subject = item.Subject.WithLabel("p")
	item.Expected = item.Expected.WithLabel("Point")
	item.Evidence[2].Detail = judgment.MissingRequiredFieldTypeEvidenceDetail("y", typ.Number)

	d, ok := renderAssignmentJudgmentWithPolicy(item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderAssignmentJudgmentWithPolicy returned false")
	}
	if !strings.Contains(d.Message, `object literal is missing required field "y"`) {
		t.Fatalf("message = %q, want missing field", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "required field p.y has type number") {
		t.Fatalf("evidence = %#v, want missing field path evidence", d.Explanation.Evidence())
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "p is declared as Point") {
		t.Fatalf("evidence = %#v, want declared alias evidence", d.Explanation.Evidence())
	}
	if !strings.Contains(d.Help, "Add field `y`") {
		t.Fatalf("help = %q, want missing field help", d.Help)
	}
}

func assignmentJudgmentFixture(actual, expected typ.Type, verdict judgment.Verdict) judgment.Judgment {
	return judgment.Judgment{
		Code: judgment.CodeAssignment,
		Subject: judgment.NewSubjectRef(
			"fixture:assignment",
			judgment.SubjectPath,
			"assignment:1:sym:1",
		).WithLabel("n"),
		Expected: judgment.NewTypeRef(expected).WithLabel("n"),
		Actual:   judgment.NewValueRef(1, actual),
		Verdict:  verdict,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Span:  judgment.SpanRef{File: "main.lua", StartLine: 1, StartCol: 19, EndLine: 1, EndCol: 24},
			},
			{
				Kind:  judgment.EvidenceUserAssertion,
				Trust: judgment.EvidenceTrustClaimed,
				Span:  judgment.SpanRef{File: "main.lua", StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 16},
			},
			{
				Kind:  judgment.EvidenceMissingProof,
				Trust: judgment.EvidenceTrustUnknown,
			},
		},
		Spans: []judgment.SpanRef{{File: "main.lua", StartLine: 1, StartCol: 19, EndLine: 1, EndCol: 24}},
	}
}
