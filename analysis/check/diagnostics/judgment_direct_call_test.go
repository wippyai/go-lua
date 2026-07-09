package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRenderDirectCallArgumentJudgmentConcreteMismatch(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
need_string(42)`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceReachableCallJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArguments{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want direct-call argument error", d)
	}
	if !strings.Contains(d.Message, "argument 1 is 42, not string") {
		t.Fatalf("message = %q, want concrete mismatch", d.Message)
	}
	if len(d.Explanation.Evidence()) < 3 {
		t.Fatalf("evidence = %#v, want actual/expected/proof chain", d.Explanation.Evidence())
	}
}

func TestDirectCallJudgmentSuppressesCallerOwnedSpecializedParameterDuplicate(t *testing.T) {
	result := runDiagnosticsResult(t, `local function invoke(provider, payload)
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "bad")`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := Produce(result)
	var callArg []diagnostic.Diagnostic
	for _, d := range diags {
		if d.Code == CodeDirectCallArgType {
			callArg = append(callArg, d)
		}
	}
	if len(callArg) != 1 {
		t.Fatalf("direct-call diagnostics = %d, want caller-owned summary only: %#v", len(callArg), diags)
	}
	d := callArg[0]
	if d.Position.Line != 9 || d.Position.Column != 11 {
		t.Fatalf("diagnostic position = %s:%d:%d, want outer call argument", d.Position.File, d.Position.Line, d.Position.Column)
	}
	if !strings.Contains(d.Message, `argument 2 is "bad", not number`) {
		t.Fatalf("message = %q, want caller-facing argument mismatch", d.Message)
	}
}

func TestRenderDirectCallArityJudgmentTooFew(t *testing.T) {
	item := directCallArityJudgmentFixture(judgment.ArityTooFewEvidenceDetail(2, 1), []judgment.SpanRef{
		{File: "main.lua", StartLine: 4, StartCol: 1, EndLine: 4, EndCol: 7},
	})
	d, ok := renderCallArityJudgmentWithPolicy(newJudgmentRenderContext(), item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderCallArityJudgmentWithPolicy returned false")
	}
	if d.Code != CodeDirectCallTooFewArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want too-few-args error", d)
	}
	if d.Message != "add expects 2 arguments, got 1" {
		t.Fatalf("message = %q, want precise arity mismatch", d.Message)
	}
	if !diagnosticHasLabel(d, labelCallExpression) {
		t.Fatalf("labels = %#v, want call expression label", d.Labels)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "call to add passes 1 argument") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "add declares 2 parameters") {
		t.Fatalf("evidence = %#v, want count evidence", d.Explanation.Evidence())
	}
}

func TestProduceDirectCallArityJudgmentDiagnosticsFromResult(t *testing.T) {
	result := runDiagnosticsResult(t, `local function add(a: number, b: number): number
    return a + b
end
add(1)
add(1, 2, 3)`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArity{})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want two arity diagnostics: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeDirectCallTooFewArgs || diags[1].Code != CodeDirectCallTooManyArgs {
		t.Fatalf("codes = %s, %s; want too-few then too-many", diags[0].Code, diags[1].Code)
	}
	if len(diags[1].Labels) != 1 || diags[1].Labels[0].Message != labelExtraArgument {
		t.Fatalf("too-many labels = %#v, want extra argument", diags[1].Labels)
	}
}

func TestProduceDirectCallCalleeJudgmentDiagnosticsFromResult(t *testing.T) {
	result := runDiagnosticsResult(t, `local x: number = 42
x()
local maybe: (() -> string)? = nil
maybe()`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallCallee{})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want two callee diagnostics: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeDirectCallNotCallable || !strings.Contains(diags[0].Message, "x is 42, not callable") {
		t.Fatalf("first diagnostic = %#v, want non-callable x", diags[0])
	}
	if !diagnosticHasLabel(diags[0], labelCallTarget) {
		t.Fatalf("first labels = %#v, want call target", diags[0].Labels)
	}
	if !diagnosticEvidenceContains(diags[0].Explanation.Evidence(), "x has literal value 42") ||
		!diagnosticEvidenceContains(diags[0].Explanation.Evidence(), "no proof on this path shows x is callable") {
		t.Fatalf("first evidence = %#v, want type and callable-proof chain", diags[0].Explanation.Evidence())
	}
	if diags[1].Code != CodeDirectCallNotCallable || !strings.Contains(diags[1].Message, "cannot call maybe because it may be nil") {
		t.Fatalf("second diagnostic = %#v, want maybe nil callable", diags[1])
	}
	if !diagnosticEvidenceContains(diags[1].Explanation.Evidence(), "maybe has a callable type, but may also be nil") ||
		!diagnosticEvidenceContains(diags[1].Explanation.Evidence(), "no guard on this path proves maybe is non-nil before this call") {
		t.Fatalf("second evidence = %#v, want callable nil evidence", diags[1].Explanation.Evidence())
	}
}

func TestProduceDirectCallCalleeJudgmentDiagnosticsForMissingMember(t *testing.T) {
	result := runDiagnosticsResult(t, `type Client = {id: string}
function f(c: Client)
    c.invoke()
end`)
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}

	diags := produceJudgmentsWithPolicy(result.FunctionResults()[0], "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallCallee{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one missing-member diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want missing-member error", d)
	}
	if !strings.Contains(d.Message, `has no member "invoke"`) {
		t.Fatalf("message = %q, want missing member invoke", d.Message)
	}
	if !diagnosticHasLabel(d, labelMemberCall) {
		t.Fatalf("labels = %#v, want member-call label", d.Labels)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "c.invoke has receiver type") {
		t.Fatalf("evidence = %#v, want receiver type evidence", d.Explanation.Evidence())
	}
}

func TestRenderDirectCallCalleeJudgmentUsesLenientPolicySeverity(t *testing.T) {
	item := directCallCalleeJudgmentFixture(judgment.CalleeMayBeNilEvidenceDetail(true), typ.MaterializeOptional(typ.Func().Returns(typ.String).Build()))

	d, ok := renderCallCalleeJudgmentWithPolicy(newJudgmentRenderContext(), item, judgment.DefaultPolicy(), judgment.StrictnessLenient)
	if !ok {
		t.Fatal("renderCallCalleeJudgmentWithPolicy returned false")
	}
	if d.Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning", d.Severity)
	}
}

func TestRenderDirectCallArityJudgmentTooManyUsesExtraArgumentSpan(t *testing.T) {
	item := directCallArityJudgmentFixture(judgment.ArityTooManyEvidenceDetail(2, 3), []judgment.SpanRef{
		{File: "main.lua", StartLine: 4, StartCol: 1, EndLine: 4, EndCol: 13},
		{File: "main.lua", StartLine: 4, StartCol: 11, EndLine: 4, EndCol: 12},
	})
	d, ok := renderCallArityJudgmentWithPolicy(newJudgmentRenderContext(), item, judgment.DefaultPolicy(), judgment.StrictnessDefault)
	if !ok {
		t.Fatal("renderCallArityJudgmentWithPolicy returned false")
	}
	if d.Code != CodeDirectCallTooManyArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want too-many-args error", d)
	}
	if len(d.Labels) != 1 || d.Labels[0].Message != labelExtraArgument || d.Labels[0].Span.StartCol != 11 {
		t.Fatalf("labels = %#v, want exact extra-argument label", d.Labels)
	}
}

func directCallArityJudgmentFixture(detail judgment.EvidenceDetail, spans []judgment.SpanRef) judgment.Judgment {
	return judgment.Judgment{
		Code: judgment.CodeCallArity,
		Subject: judgment.SubjectRef{
			FunctionKey: "fixture",
			Kind:        judgment.SubjectCallExpression,
			Key:         "call:4:arity",
			Label:       "add",
		},
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact, Trust: judgment.EvidenceTrustProven, Detail: detail, Span: spans[0]},
			{Kind: judgment.EvidenceUserAssertion, Trust: judgment.EvidenceTrustClaimed, Detail: detail},
			{Kind: judgment.EvidenceMissingProof, Trust: judgment.EvidenceTrustRefuted, Detail: detail},
		},
		Spans: spans,
	}
}

func directCallCalleeJudgmentFixture(detail judgment.EvidenceDetail, actual typ.Type) judgment.Judgment {
	span := judgment.SpanRef{File: "main.lua", StartLine: 4, StartCol: 1, EndLine: 4, EndCol: 6}
	verdict := judgment.VerdictRefuted
	trust := judgment.EvidenceTrustRefuted
	if detail.Kind == judgment.EvidenceDetailCalleeMayBeNil {
		verdict = judgment.VerdictUnknown
		trust = judgment.EvidenceTrustUnknown
	}
	return judgment.Judgment{
		Code: judgment.CodeCallCallee,
		Subject: judgment.SubjectRef{
			FunctionKey: "fixture",
			Kind:        judgment.SubjectCallExpression,
			Key:         "call:4:callee",
			Label:       "maybe",
		},
		Actual:  judgment.NewValueRef(1, actual),
		Verdict: verdict,
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact, Trust: judgment.EvidenceTrustProven, Detail: detail, Span: span},
			{Kind: judgment.EvidenceUserAssertion, Trust: judgment.EvidenceTrustClaimed, Detail: detail, Span: span},
			{Kind: judgment.EvidenceMissingProof, Trust: trust, Detail: detail, Span: span},
		},
		Spans: []judgment.SpanRef{span},
	}
}

func TestRenderDirectCallArgumentJudgmentUntrustedBoundary(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
local raw: any = nil
need_string(raw)`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceReachableCallJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArguments{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if !strings.Contains(d.Message, "argument 1 (raw)") {
		t.Fatalf("message = %q, want named argument", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, "raw comes from any/unknown") {
		t.Fatalf("evidence = %#v, want precision boundary evidence", evidence)
	}
	if !diagnosticEvidenceContains(evidence, "no proof on this path shows raw satisfies the parameter type") {
		t.Fatalf("evidence = %#v, want source-named missing-proof evidence", evidence)
	}
}

func TestRenderDirectCallArgumentJudgmentBoundaryWordingRequiresEvidence(t *testing.T) {
	item := judgment.Judgment{
		Code: judgment.CodeCallArgType,
		Subject: judgment.SubjectRef{
			FunctionKey: "fixture",
			Kind:        judgment.SubjectCallArgument,
			Key:         "call:1:arg:0",
			Label:       "argument 1 (raw)",
		},
		Expected: judgment.NewTypeRef(typ.String),
		Actual:   judgment.NewValueRef(1, typ.Number),
		Verdict:  judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact, Trust: judgment.EvidenceTrustProven},
			{Kind: judgment.EvidenceUserAssertion, Trust: judgment.EvidenceTrustClaimed},
			{Kind: judgment.EvidenceMissingProof, Trust: judgment.EvidenceTrustRefuted},
		},
		Spans: []judgment.SpanRef{{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}},
	}

	diag, ok := renderDirectCallArgumentJudgment(item)
	if !ok {
		t.Fatal("renderDirectCallArgumentJudgment did not render concrete mismatch")
	}
	if strings.Contains(diag.Message, "comes from any/unknown") {
		t.Fatalf("message = %q, want no boundary wording without precision-boundary evidence", diag.Message)
	}

	item.Evidence = append(item.Evidence, judgment.Evidence{Kind: judgment.EvidencePrecisionBoundary, Trust: judgment.EvidenceTrustUnknown})
	diag, ok = renderDirectCallArgumentJudgment(item)
	if !ok {
		t.Fatal("renderDirectCallArgumentJudgment did not render boundary mismatch")
	}
	if !strings.Contains(diag.Message, "argument 1 (raw) comes from any/unknown") {
		t.Fatalf("message = %q, want evidence-driven boundary wording", diag.Message)
	}
	if !diagnosticEvidenceContains(diag.Explanation.Evidence(), "raw comes from any/unknown") {
		t.Fatalf("evidence = %#v, want boundary evidence", diag.Explanation.Evidence())
	}
}

func TestRenderDirectCallArgumentJudgmentNilabilityPrecedesBoundaryWording(t *testing.T) {
	item := judgment.Judgment{
		Code: judgment.CodeCallArgType,
		Subject: judgment.SubjectRef{
			FunctionKey: "fixture",
			Kind:        judgment.SubjectCallArgument,
			Key:         "call:1:arg:0",
			Label:       "argument 1 (raw)",
		},
		Expected: judgment.NewTypeRef(typ.String),
		Actual:   judgment.NewValueRef(1, typ.Any),
		Verdict:  judgment.VerdictUnknown,
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact, Trust: judgment.EvidenceTrustUnknown},
			{Kind: judgment.EvidenceUserAssertion, Trust: judgment.EvidenceTrustClaimed},
			{Kind: judgment.EvidenceMissingProof, Trust: judgment.EvidenceTrustUnknown, Detail: judgment.EvidenceDetail{Kind: judgment.EvidenceDetailMayBeNil, ExpandedSource: true}},
			{Kind: judgment.EvidencePrecisionBoundary, Trust: judgment.EvidenceTrustUnknown},
		},
		Spans: []judgment.SpanRef{{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}},
	}

	diag, ok := renderDirectCallArgumentJudgment(item)
	if !ok {
		t.Fatal("renderDirectCallArgumentJudgment did not render nilable boundary mismatch")
	}
	if !strings.Contains(diag.Message, "cannot pass raw as argument 1 because it may be nil") {
		t.Fatalf("message = %q, want nilability as primary cause", diag.Message)
	}
	evidence := diag.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, "raw comes from any/unknown") {
		t.Fatalf("evidence = %#v, want boundary evidence preserved", evidence)
	}
	if !diagnosticEvidenceContains(evidence, "no guard on this path proves raw is non-nil") {
		t.Fatalf("evidence = %#v, want nil guard evidence", evidence)
	}
}

func TestRenderDirectCallArgumentJudgmentUsesLenientPolicySeverity(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
local raw: any = nil
need_string(raw)`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceReachableCallJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessLenient, pass.CallArguments{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if diags[0].Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning", diags[0].Severity)
	}
}

func TestRenderDirectCallArgumentJudgmentPolicyCanDisableUnknownBoundary(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
local raw: any = nil
need_string(raw)`)
	if result == nil {
		t.Fatal("RootResult nil")
	}
	policy := judgment.NewPolicy(map[judgment.PolicyKey]judgment.Level{
		{Code: judgment.CodeCallArgType, Verdict: judgment.VerdictUnknown, Mode: judgment.StrictnessDefault}: judgment.LevelDisabled,
	})

	diags := produceReachableCallJudgmentsWithPolicy(result, "main.lua", policy, judgment.StrictnessDefault, pass.CallArguments{})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %d, want 0: %#v", len(diags), diags)
	}
}

func TestRenderDirectCallArgumentJudgmentNilMismatchNamesSourcePath(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
local response: { body: string? } = { body = nil }
need_string(response.body)`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceReachableCallJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArguments{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if !strings.Contains(d.Message, "cannot pass response.body as argument 1 because it may be nil") {
		t.Fatalf("message = %q, want source-path nil mismatch", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, "argument 1 (response.body) has type nil") ||
		!diagnosticEvidenceContains(evidence, "no guard on this path proves response.body is non-nil") {
		t.Fatalf("evidence = %#v, want argument observation and source-path nil proof", evidence)
	}
	if !strings.Contains(d.Help, "Guard `response.body` with a nil check") {
		t.Fatalf("help = %q, want source-path nil repair", d.Help)
	}
}

func TestRenderDirectCallArgumentJudgmentNilWordingRequiresEvidenceDetail(t *testing.T) {
	item := judgment.Judgment{
		Code: judgment.CodeCallArgType,
		Subject: judgment.SubjectRef{
			FunctionKey: "fixture",
			Kind:        judgment.SubjectCallArgument,
			Key:         "call:1:arg:0",
			Label:       "argument 1 (response.body)",
		},
		Expected: judgment.NewTypeRef(typ.String),
		Actual:   judgment.NewValueRef(1, typ.MaterializeOptional(typ.String)),
		Verdict:  judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact, Trust: judgment.EvidenceTrustProven},
			{Kind: judgment.EvidenceUserAssertion, Trust: judgment.EvidenceTrustClaimed},
			{Kind: judgment.EvidenceMissingProof, Trust: judgment.EvidenceTrustRefuted},
		},
		Spans: []judgment.SpanRef{{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 14}},
	}

	diag, ok := renderDirectCallArgumentJudgment(item)
	if !ok {
		t.Fatal("renderDirectCallArgumentJudgment did not render refuted argument")
	}
	if strings.Contains(diag.Message, "may be nil") || strings.Contains(diag.Help, "Guard `response.body`") {
		t.Fatalf("diagnostic = %#v, want no nil wording without may-be-nil evidence detail", diag)
	}

	item.Evidence[2].Detail = judgment.MayBeNilEvidenceDetail()
	diag, ok = renderDirectCallArgumentJudgment(item)
	if !ok {
		t.Fatal("renderDirectCallArgumentJudgment did not render may-be-nil argument")
	}
	if !strings.Contains(diag.Message, "cannot pass response.body as argument 1 because it may be nil") {
		t.Fatalf("message = %q, want evidence-driven nil wording", diag.Message)
	}
	if !strings.Contains(diag.Help, "Guard `response.body` with a nil check") {
		t.Fatalf("help = %q, want evidence-driven nil repair", diag.Help)
	}
}

func TestRenderDirectCallArgumentJudgmentMissingRequiredFieldEvidence(t *testing.T) {
	result := runDiagnosticsResult(t, `type HasId = { id: string }
local function need_id<T: HasId>(x: T): string
    return x.id
end
need_id({ name = "no-id-here" })`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceReachableCallJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArguments{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	evidence := diags[0].Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, `object literal does not provide field "id"`) {
		t.Fatalf("evidence = %#v, want missing required field evidence", evidence)
	}
}

func TestRenderDirectCallArgumentJudgmentGenericConflictEvidence(t *testing.T) {
	result := runDiagnosticsResult(t, `type Channel<T> = { value: T }
type Event = { kind: "event", id: string }
type Timer = { kind: "timer", elapsed: number }
type Options<T> = { channel: Channel<T>, decode: (any) -> T }
local function listen<T>(topic: string, options: Options<T>): Channel<T>
    return options.channel
end
local source: { primary: Channel<Event> } = nil :: any
local function decode_timer(raw: any): Timer
    return { kind = "timer", elapsed = 1 }
end
listen("events", {
    channel = source.primary,
    decode = decode_timer,
})`)
	if result == nil {
		t.Fatal("RootResult nil")
	}

	diags := produceReachableCallJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArguments{})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if !strings.Contains(d.Message, "argument 2") ||
		!strings.Contains(d.Message, "`T` incompatible types") {
		t.Fatalf("message = %q, want generic conflict wording", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, "contributes") ||
		!diagnosticEvidenceContains(evidence, "expects one consistent type for `T`") ||
		!diagnosticEvidenceContains(evidence, "no single type for `T`") {
		t.Fatalf("evidence = %#v, want generic conflict evidence chain", evidence)
	}
	if len(evidence) < 2 ||
		evidence[0].Span.StartLine != 13 || evidence[0].Span.StartCol != 15 ||
		evidence[1].Span.StartLine != 14 || evidence[1].Span.StartCol != 14 {
		t.Fatalf("evidence spans = %#v, want distinct contribution spans", evidence)
	}
	if !strings.Contains(d.Help, "Make each use of `T` in this argument agree on the same type") {
		t.Fatalf("help = %q, want generic conflict repair", d.Help)
	}
}

func TestRenderDirectCallArgumentJudgmentSkipsUnknownWithoutBoundary(t *testing.T) {
	_, ok := renderDirectCallArgumentJudgment(judgment.Judgment{
		Code: judgment.CodeCallArgType,
		Subject: judgment.SubjectRef{
			FunctionKey: "fixture",
			Kind:        judgment.SubjectCallArgument,
			Key:         "call:1:arg:0",
		},
		Expected: judgment.NewTypeRef(typ.String),
		Actual:   judgment.NewValueRef(0, typ.Any),
		Verdict:  judgment.VerdictUnknown,
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact},
			{Kind: judgment.EvidenceUserAssertion},
			{Kind: judgment.EvidenceMissingProof},
		},
		Spans: []judgment.SpanRef{{File: "main.lua", StartLine: 1, StartCol: 1}},
	})
	if ok {
		t.Fatal("renderDirectCallArgumentJudgment rendered unknown non-boundary obligation as an error")
	}
}

func TestProduceDirectCallArgumentsFromJudgmentPath(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
need_string(42)`)

	diags := ProduceWithConfig(result, Config{})
	var argDiags []diagnostic.Diagnostic
	for _, diag := range diags {
		if diag.Code == CodeDirectCallArgType {
			argDiags = append(argDiags, diag)
		}
	}
	if len(argDiags) != 1 {
		t.Fatalf("argument diagnostics = %d, want 1: %#v", len(argDiags), diags)
	}
	if !strings.Contains(argDiags[0].Message, "argument 1 is 42, not string") {
		t.Fatalf("message = %q, want judgment-rendered concrete mismatch", argDiags[0].Message)
	}
}

func TestProduceDirectCallArgumentsUsesRootTypeGuardElseMismatch(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_number(n: number): number
    return n
end

local v: number | string = value
if type(v) == "number" then
    return 0
else
    return need_number(v)
end
`)

	argOnly := produceReachableCallJudgmentsWithPolicy(result, "main.lua", judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArguments{})
	if len(argOnly) != 1 || argOnly[0].Code != CodeDirectCallArgType {
		t.Fatalf("argument-only diagnostics = %#v, want one direct-call argument diagnostic", argOnly)
	}
	if !strings.Contains(argOnly[0].Message, "argument 1 (v) is string, not number") {
		t.Fatalf("argument-only message = %q, want narrowed string mismatch", argOnly[0].Message)
	}

	all := ProduceWithConfig(result, Config{})
	var argDiags []diagnostic.Diagnostic
	for _, diag := range all {
		if diag.Code == CodeDirectCallArgType {
			argDiags = append(argDiags, diag)
		}
	}
	if len(argDiags) != 1 {
		t.Fatalf("all diagnostics = %#v, want one direct-call argument diagnostic", all)
	}
	if !strings.Contains(argDiags[0].Message, "argument 1 (v) is string, not number") {
		t.Fatalf("message = %q, want narrowed string mismatch", argDiags[0].Message)
	}
}

func TestProduceDirectCallArgumentsUseJudgmentPolicy(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
local raw: any = nil
need_string(raw)`)

	diags := ProduceWithConfig(result, Config{
		Judgment: judgment.PolicyConfig{Strictness: judgment.StrictnessLenient},
	})
	var argDiags []diagnostic.Diagnostic
	for _, diag := range diags {
		if diag.Code == CodeDirectCallArgType {
			argDiags = append(argDiags, diag)
		}
	}
	if len(argDiags) != 1 {
		t.Fatalf("argument diagnostics = %d, want 1: %#v", len(argDiags), diags)
	}
	if argDiags[0].Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning", argDiags[0].Severity)
	}
}

func TestProduceDirectCallArgumentsKeepArityDiagnostics(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
need_string()`)

	diags := ProduceWithConfig(result, Config{})
	var foundTooFew bool
	for _, diag := range diags {
		if diag.Code == CodeDirectCallTooFewArgs {
			foundTooFew = true
		}
		if diag.Code == CodeDirectCallArgType {
			t.Fatalf("unexpected argument-type diagnostic for missing argument: %#v", diag)
		}
	}
	if !foundTooFew {
		t.Fatalf("diagnostics = %#v, want too-few-args diagnostic", diags)
	}
}

func TestProduceDirectCallContractFromJudgmentPath(t *testing.T) {
	result := runDiagnosticsResult(t, `local x: number = 42
x()

local function need_string(value: string): ()
end
local function need_pair(left: string, right: number): ()
end
need_string()
need_string(42)
need_pair(42, "wrong")`)

	diags := ProduceWithConfig(result, Config{})
	counts := map[diagnostic.Code]int{}
	for _, diag := range diags {
		counts[diag.Code]++
	}
	if counts[CodeDirectCallNotCallable] != 1 ||
		counts[CodeDirectCallTooFewArgs] != 1 ||
		counts[CodeDirectCallArgType] != 2 {
		t.Fatalf("diagnostic counts = %#v; diagnostics = %#v", counts, diags)
	}
	var pairArgDiags int
	for _, diag := range diags {
		if diag.Code == CodeDirectCallArgType && !strings.Contains(diag.Message, "argument 1 is 42, not string") {
			t.Fatalf("argument message = %q, want judgment-rendered argument type", diag.Message)
		}
		if diag.Code == CodeDirectCallTooFewArgs && !strings.Contains(diag.Message, "need_string expects 1 argument, got 0") {
			t.Fatalf("arity message = %q, want judgment-rendered arity", diag.Message)
		}
		if diag.Code == CodeDirectCallNotCallable && !strings.Contains(diag.Message, "x is 42, not callable") {
			t.Fatalf("callee message = %q, want judgment-rendered callee", diag.Message)
		}
		if diag.Code == CodeDirectCallArgType && diag.Span.StartLine == 10 {
			pairArgDiags++
		}
	}
	if pairArgDiags != 1 {
		t.Fatalf("need_pair diagnostics = %d, want one contract diagnostic for the call: %#v", pairArgDiags, diags)
	}
}

func TestProduceDirectCallContractUsesSolvedReachability(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): ()
end
if false then
	need_string(42)
end`)

	diags := ProduceWithConfig(result, Config{})
	for _, diag := range diags {
		if diag.Code == CodeDirectCallArgType {
			t.Fatalf("diagnostics = %#v, want no direct-call argument diagnostic from unreachable branch", diags)
		}
	}
}

func TestProduceDirectCallContractUsesJudgmentPolicyForUnknownCallee(t *testing.T) {
	result := runDiagnosticsResult(t, `local maybe: (() -> string)? = nil
maybe()`)

	diags := ProduceWithConfig(result, Config{
		Judgment: judgment.PolicyConfig{Strictness: judgment.StrictnessLenient},
	})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeDirectCallNotCallable || diags[0].Severity != diagnostic.SeverityWarning {
		t.Fatalf("diagnostic = %#v, want lenient maybe-nil callee warning", diags[0])
	}
}

func TestProduceDirectCallContractSuppressesInvalidCallableDeclarationCascade(t *testing.T) {
	result := runDiagnosticsResult(t, `local t = {}
local g: fun(): number = t.run
g()`)

	diags := ProduceWithConfig(result, Config{})
	var assignmentCount int
	for _, diag := range diags {
		switch diag.Code {
		case CodeAssignmentType:
			assignmentCount++
		case CodeDirectCallNotCallable:
			t.Fatalf("diagnostics = %#v, want invalid local declaration to own later call cascade", diags)
		}
	}
	if assignmentCount != 1 {
		t.Fatalf("assignment diagnostics = %d, want one invalid local declaration diagnostic: %#v", assignmentCount, diags)
	}
}

func TestProduceDirectCallContractSuppressesAssignmentCascadeByPrecedence(t *testing.T) {
	result := runDiagnosticsResult(t, `local function need_string(value: string): string
    return "ok"
end
local out: number = need_string(42)`)

	diags := ProduceWithConfig(result, Config{})
	var sawCallArg bool
	for _, diag := range diags {
		if diag.Code == CodeAssignmentType {
			t.Fatalf("diagnostics = %#v, want direct-call contract to own the cascade instead of assignment", diags)
		}
		if diag.Code == CodeDirectCallArgType {
			sawCallArg = true
		}
	}
	if !sawCallArg {
		t.Fatalf("diagnostics = %#v, want direct-call argument diagnostic", diags)
	}
}
