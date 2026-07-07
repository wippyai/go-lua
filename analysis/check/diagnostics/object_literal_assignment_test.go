package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestAnnotationAssignabilityAcceptsBracketStringMapLiteralEntries(t *testing.T) {
	diags := runDiagnostics(t, `
		local routes: {[string]: string} = { ["/ok"] = "page:ok" }
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityBracketStringDoesNotSatisfyRequiredField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local p: Point = {["x"] = 10, y = 20}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, `"x"`) {
		t.Fatalf("diagnostic = %#v, want missing required field x", d)
	}
	if d := diags[0]; !diagnosticEvidenceContains(d.Explanation.Evidence(), `object literal has type {y: 20, ["x"]: 10}`) ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "p is declared as Point") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "required field p.x has type number, but the object literal does not provide it") {
		t.Fatalf("evidence = %#v, want provided shape, declared type, and missing-field path proof", d.Explanation.Evidence())
	}
	if d := diags[0]; !strings.Contains(d.Help, "Add field `x`") {
		t.Fatalf("help = %q, want missing-field repair", d.Help)
	}
	if d := diags[0]; len(d.Labels) < 2 || d.Labels[0].Message != "object literal" ||
		d.Labels[1].Message != "declared type" || d.Labels[0].Span != d.Span {
		t.Fatalf("labels/span = %#v/%#v, want object literal label on diagnostic span plus declared type", d.Labels, d.Span)
	}
}

func TestAnnotationAssignabilityAllowsMissingNilableRecordField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Registry = {
			primary: ((string) -> string)?,
			backup: ((string) -> string)?,
		}
		local registry: Registry = {}
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	src := "type Point = {id: string}\nlocal raw: any = nil\nlocal p: Point = {id = raw}\n"
	requireDiagnosticShape(t, src, runDiagnostics(t, src), diagnosticShapeWant{
		code:    CodeAssignmentType,
		message: "cannot assign raw to p.id because raw is any, not string",
		span:    diagnostic.Span{StartLine: 3, StartCol: 24, EndLine: 3, EndCol: 26},
		labels: []diagnosticLabelWant{
			{message: labelAssignedValue, span: diagnostic.Span{StartLine: 3, StartCol: 24, EndLine: 3, EndCol: 26}},
			{message: labelDeclaredType, span: diagnostic.Span{StartLine: 3, StartCol: 10, EndLine: 3, EndCol: 14}},
		},
		evidence: []diagnosticEvidenceWant{
			{kind: diagnostic.EvidenceAbstractFact, trust: diagnostic.TrustProven, message: "raw has type any", span: diagnostic.Span{StartLine: 3, StartCol: 24, EndLine: 3, EndCol: 26}},
			{kind: diagnostic.EvidenceUserAssertion, trust: diagnostic.TrustClaimed, message: "p.id is declared as string", span: diagnostic.Span{StartLine: 3, StartCol: 10, EndLine: 3, EndCol: 14}},
			{kind: diagnostic.EvidenceAbstractFact, trust: diagnostic.TrustProven, message: "assigned value has type {id: nil}", span: diagnostic.Span{StartLine: 3, StartCol: 18, EndLine: 3, EndCol: 27}},
			{kind: diagnostic.EvidenceUserAssertion, trust: diagnostic.TrustClaimed, message: "p is declared as {id: string}", span: diagnostic.Span{StartLine: 3, StartCol: 10, EndLine: 3, EndCol: 14}},
			{kind: diagnostic.EvidenceUserAssertion, trust: diagnostic.TrustClaimed, reason: diagnostic.EvidenceReasonUserAssertedAny, message: "user asserted any; not abstract-interpreter proof", span: diagnostic.Span{StartLine: 3, StartCol: 24, EndLine: 3, EndCol: 26}},
			{kind: diagnostic.EvidencePrecisionBoundary, trust: diagnostic.TrustUnknown, reason: diagnostic.EvidenceReasonExplicitBoundaryValidation, message: "raw comes from any/unknown", span: diagnostic.Span{StartLine: 3, StartCol: 24, EndLine: 3, EndCol: 26}},
			{kind: diagnostic.EvidenceMissingProof, trust: diagnostic.TrustUnknown, reason: diagnostic.EvidenceReasonBoundaryValidationMissing, message: "no proof on this path shows raw is string", span: diagnostic.Span{StartLine: 3, StartCol: 24, EndLine: 3, EndCol: 26}},
		},
		help: "Use a value compatible with the expected type, or change the target type if `raw` is valid.",
		renderContains: []string{
			"error[type.assignment]: cannot assign raw to p.id because raw is any, not string",
			" --> main.lua:3:24",
			"  |          ↓ declared type",
			"  |                        ↑ assigned value",
			"3 | local p: Point = {id = raw}",
			"1. proven: raw has type any",
			"2. claimed: p.id is declared as string",
			"3. proven: assigned value has type {id: nil}",
			"4. claimed: p is declared as {id: string}",
			"5. claimed: user asserted any; not abstract-interpreter proof",
			"6. unvalidated value: raw comes from any/unknown",
			"7. missing proof: no proof on this path shows raw is string",
			"help: Use a value compatible with the expected type, or change the target type if `raw` is valid.",
		},
	})
}

func TestAnnotationAssignabilityRejectsObjectLiteralTopOriginInsideClosedUnion(t *testing.T) {
	diags := runDiagnostics(t, `
		type A = {kind: "a", id: string}
		type B = {kind: "b", id: number}
		type Item = A | B
		local raw: any = nil
		local item: Item = {kind = "a", id = raw}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
		t.Fatalf("diagnostic = %#v, want union object member mismatch", d)
	}
	if d := diags[0]; !diagnosticEvidenceContains(d.Explanation.Evidence(), "assigned value has type") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "item is declared as") {
		t.Fatalf("evidence = %#v, want source object and declared union evidence", d.Explanation.Evidence())
	}
	if d := diags[0]; len(d.Labels) < 2 || d.Labels[0].Message != "assigned value" ||
		d.Labels[1].Message != "declared type" || d.Labels[0].Span != d.Span {
		t.Fatalf("labels/span = %#v/%#v, want assigned object literal label on diagnostic span plus declared type", d.Labels, d.Span)
	}
}

func TestDirectCallRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	src := "type Point = {id: string}\nfunction take(p: Point)\nend\nlocal raw: any = nil\ntake({id = raw})\n"
	requireDiagnosticShape(t, src, runDiagnostics(t, src), diagnosticShapeWant{
		code:    CodeDirectCallArgType,
		message: "argument 1.id (raw) comes from any/unknown; no proof shows it is string",
		span:    diagnostic.Span{StartLine: 5, StartCol: 12, EndLine: 5, EndCol: 14},
		labels: []diagnosticLabelWant{
			{message: labelArgumentValue, span: diagnostic.Span{StartLine: 5, StartCol: 12, EndLine: 5, EndCol: 14}},
		},
		evidence: []diagnosticEvidenceWant{
			{kind: diagnostic.EvidenceAbstractFact, trust: diagnostic.TrustProven, message: "argument 1.id (raw) has type any", span: diagnostic.Span{StartLine: 5, StartCol: 12, EndLine: 5, EndCol: 14}},
			{kind: diagnostic.EvidenceUserAssertion, trust: diagnostic.TrustClaimed, message: "take parameter 1.id expects string", span: diagnostic.Span{StartLine: 2, StartCol: 18, EndLine: 2, EndCol: 22}},
			{kind: diagnostic.EvidencePrecisionBoundary, trust: diagnostic.TrustUnknown, reason: diagnostic.EvidenceReasonExplicitBoundaryValidation, message: "raw comes from any/unknown", span: diagnostic.Span{StartLine: 5, StartCol: 12, EndLine: 5, EndCol: 14}},
			{kind: diagnostic.EvidenceMissingProof, trust: diagnostic.TrustRefuted, reason: diagnostic.EvidenceReasonBoundaryValidationMissing, message: "no proof on this path shows raw satisfies the parameter type", span: diagnostic.Span{StartLine: 5, StartCol: 12, EndLine: 5, EndCol: 14}},
		},
		help: "Validate or narrow `raw` before passing it; any/unknown values do not prove parameter contracts.",
		renderContains: []string{
			"error[type.call.direct.argument_type]: argument 1.id (raw) comes from any/unknown; no proof shows it is string",
			" --> main.lua:5:12",
			"  |            ↑ argument value",
			"5 | take({id = raw})",
			"1. proven: argument 1.id (raw) has type any",
			"2. claimed: take parameter 1.id expects string",
			" --> main.lua:2:18",
			"2 | function take(p: Point)",
			"  |                  ^",
			"3. unvalidated value: raw comes from any/unknown",
			"4. missing proof: no proof on this path shows raw satisfies the parameter type",
			"help: Validate or narrow `raw` before passing it; any/unknown values do not prove parameter contracts.",
		},
	})
}

func TestJudgmentDirectCallLabelsObjectLiteralExplicitAnyMember(t *testing.T) {
	src := "type Point = {id: string}\nfunction take(p: Point)\nend\nlocal raw: any = nil\ntake({id = raw})\n"
	result := runDiagnosticsResult(t, src)
	diags := ProduceWithConfig(result, Config{})
	d := requireDirectCallDiagnostic(t, diags, CodeDirectCallArgType)
	if !strings.Contains(d.Message, "argument 1.id (raw)") {
		t.Fatalf("message = %q, want refined member source label", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "argument 1.id (raw)") {
		t.Fatalf("evidence = %#v, want refined member source label", d.Explanation.Evidence())
	}
	wantSpan := diagnostic.Span{StartLine: 2, StartCol: 18, EndLine: 2, EndCol: 22}
	for _, evidence := range d.Explanation.Evidence() {
		if evidence.Kind == diagnostic.EvidenceUserAssertion &&
			evidence.Message == "take parameter 1.id expects string" &&
			evidence.Span == wantSpan {
			return
		}
	}
	t.Fatalf("evidence = %#v, want expected-type assertion at %v", d.Explanation.Evidence(), wantSpan)
}

func TestReturnContractRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	src := "type Point = {id: string}\nfunction make(raw: any): Point\n\treturn {id = raw}\nend\n"
	requireDiagnosticShape(t, src, runDiagnostics(t, src), diagnosticShapeWant{
		code:    CodeReturnContractType,
		message: "returned value 1.id (raw) comes from any/unknown; no proof shows it satisfies declared return type string",
		span:    diagnostic.Span{StartLine: 3, StartCol: 15, EndLine: 3, EndCol: 17},
		labels: []diagnosticLabelWant{
			{message: labelReturnedValue, span: diagnostic.Span{StartLine: 3, StartCol: 15, EndLine: 3, EndCol: 17}},
			{message: labelDeclaredReturn, span: diagnostic.Span{StartLine: 2, StartCol: 26, EndLine: 2, EndCol: 30}},
		},
		evidence: []diagnosticEvidenceWant{
			{kind: diagnostic.EvidenceAbstractFact, trust: diagnostic.TrustProven, message: "returned value 1.id (raw) has type any", span: diagnostic.Span{StartLine: 3, StartCol: 15, EndLine: 3, EndCol: 17}},
			{kind: diagnostic.EvidenceUserAssertion, trust: diagnostic.TrustClaimed, message: "returned value 1.id must satisfy declared return type string", span: diagnostic.Span{StartLine: 2, StartCol: 26, EndLine: 2, EndCol: 30}},
			{kind: diagnostic.EvidenceUserAssertion, trust: diagnostic.TrustClaimed, reason: diagnostic.EvidenceReasonUserAssertedAny, message: "user asserted any; not abstract-interpreter proof", span: diagnostic.Span{StartLine: 3, StartCol: 15, EndLine: 3, EndCol: 17}},
			{kind: diagnostic.EvidencePrecisionBoundary, trust: diagnostic.TrustUnknown, reason: diagnostic.EvidenceReasonExplicitBoundaryValidation, message: "returned value 1.id (raw) comes from any/unknown", span: diagnostic.Span{StartLine: 3, StartCol: 15, EndLine: 3, EndCol: 17}},
			{kind: diagnostic.EvidenceMissingProof, trust: diagnostic.TrustUnknown, reason: diagnostic.EvidenceReasonBoundaryValidationMissing, message: "no proof on this path shows returned value 1.id (raw) satisfies the declared return type", span: diagnostic.Span{StartLine: 3, StartCol: 15, EndLine: 3, EndCol: 17}},
		},
		help: "Return a value compatible with the declared return type, or change the return annotation if the returned value is valid.",
		renderContains: []string{
			"error[type.return.contract]: returned value 1.id (raw) comes from any/unknown; no proof shows it satisfies declared return type string",
			" --> main.lua:3:15",
			"  |                  ↑ returned value",
			"3 |     return {id = raw}",
			"1. proven: returned value 1.id (raw) has type any",
			"2. claimed: returned value 1.id must satisfy declared return type string",
			" --> main.lua:2:26",
			"  |                          ↓ declared return type",
			"2 | function make(raw: any): Point",
			"3. claimed: user asserted any; not abstract-interpreter proof",
			"4. unvalidated value: returned value 1.id (raw) comes from any/unknown",
			"5. missing proof: no proof on this path shows returned value 1.id (raw) satisfies the declared return type",
			"help: Return a value compatible with the declared return type, or change the return annotation if the returned value is valid.",
		},
	})
}

func TestReturnContractAcceptsGuardedObjectLiteralAnyMember(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {id: string}
		function make(raw: any): Point?
			if type(raw) ~= "string" then
				return nil
			end
			return {id = raw}
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded return member witness", diags)
	}
}

func TestReturnContractAcceptsGuardedObjectLiteralAnyPathMember(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {id: string}
		function make(raw: any): Point?
			if type(raw.id) ~= "string" then
				return nil
			end
			return {id = raw.id}
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded return path member witness", diags)
	}
}

func TestReturnContractAcceptsGuardedPathMemberThroughNestedUnionReturn(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Task = {kind: "task", id: string}
		type Err = {code: string, message: string}
		type Result = {ok: true, value: Task} | {ok: false, error: Err}
		function make(raw: any): Result
			if type(raw.kind) ~= "string" then
				return {ok = false, error = {code = "kind", message = "bad"}}
			end
			if type(raw.id) ~= "string" then
				return {ok = false, error = {code = "id", message = "bad"}}
			end
			if raw.kind == "task" then
				return {
					ok = true,
					value = {
						kind = "task",
						id = raw.id,
					},
				}
			end
			return {ok = false, error = {code = "unknown", message = raw.kind}}
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded nested-union return members", diags)
	}
}

func TestCallParamObligationRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		type Sink = {send: (p: Point) -> ()}
		function wrap(sink: Sink, payload)
			sink.send(payload)
		end
		local sink: Sink = {send = function(p: Point) end}
		local raw: any = nil
		wrap(sink, {id = raw})
	`)
	for _, d := range diags {
		if d.Code == CodeDirectCallArgType &&
			strings.Contains(d.Message, "argument 2.id (raw) comes from any/unknown") {
			if got := d.Explanation.String(); !strings.Contains(got, "raw comes from any/unknown") ||
				!strings.Contains(got, "no proof on this path shows argument 1 (payload) is string") {
				t.Fatalf("explanation = %q, want any/unknown boundary and missing-proof evidence", got)
			}
			if !diagnosticHasLabel(d, "argument value") {
				t.Fatalf("labels = %#v, want argument value label on offending payload member", d.Labels)
			}
			if len(d.Labels) == 0 || d.Labels[0].Span != d.Explanation.Evidence()[0].Span {
				t.Fatalf("label/evidence spans = %#v/%#v, want label to track offending argument value", d.Labels, d.Explanation.Evidence())
			}
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want call-site obligation member mismatch", diags)
}

func TestCallParamObligationRejectsExplicitAnyStructuralWitness(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		type Sink = {send: (p: Point) -> ()}
		function wrap(sink: Sink, payload)
			sink.send(payload)
		end
		local sink: Sink = {send = function(p: Point) end}
		local raw = ({id = "ok"} :: any)
		wrap(sink, raw)
	`)
	for _, d := range diags {
		if d.Code == CodeDirectCallArgType &&
			strings.Contains(d.Message, "argument 2") &&
			strings.Contains(d.Message, "id") {
			if got := d.Explanation.String(); !strings.Contains(got, "raw comes from any/unknown") ||
				!strings.Contains(got, "user asserted any; not abstract-interpreter proof") ||
				!strings.Contains(got, "inside wrap, argument 1 (payload) is passed to sink.send parameter 1, which requires {id: string}") ||
				!strings.Contains(got, "no proof on this path shows argument 1 (payload) is {id: string}") {
				t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
			}
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want call-site obligation mismatch for explicit-any structural witness", diags)
}

func TestCallParamObligationRejectsAsAnyCastEscape(t *testing.T) {
	diags := runDiagnostics(t, `
		type Sink = {send: (n: number) -> ()}
		function wrap(sink: Sink, payload)
			sink.send(payload)
		end
		local sink: Sink = {send = function(n: number) end}
		wrap(sink, "no" as any)
	`)
	for _, d := range diags {
		if d.Code != CodeDirectCallArgType || !strings.Contains(d.Message, "number") {
			continue
		}
		got := d.Explanation.String()
		if !strings.Contains(got, "argument 2 comes from any/unknown") {
			continue
		}
		if !strings.Contains(got, "argument 2 comes from any/unknown") ||
			!strings.Contains(got, "user asserted any; not abstract-interpreter proof") ||
			!strings.Contains(got, "inside wrap, argument 1 (payload) is passed to sink.send parameter 1, which requires number") ||
			!strings.Contains(got, "no proof on this path shows argument 1 (payload) is number") {
			t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
		}
		if !strings.Contains(d.Help, "Validate or narrow") ||
			!strings.Contains(d.Help, "parameter contracts") {
			t.Fatalf("help = %q, want any/unknown proof repair", d.Help)
		}
		if !diagnosticHasLabel(d, "argument value") {
			t.Fatalf("labels = %#v, want argument value label on as-any argument", d.Labels)
		}
		if len(d.Labels) == 0 || d.Labels[0].Span != d.Explanation.Evidence()[0].Span {
			t.Fatalf("label/evidence spans = %#v/%#v, want label to track offending as-any argument", d.Labels, d.Explanation.Evidence())
		}
		return
	}
	t.Fatalf("diagnostics = %#v, want summary call-param as-any obligation mismatch", diags)
}

func TestReturnContractRejectsExplicitAnyStructuralWitness(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		local function make(): Point
			return ({id = "ok"} :: any)
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType || !strings.Contains(d.Message, "id") {
		t.Fatalf("diagnostic = %#v, want return contract structural explicit-any error", d)
	}
	got := diags[0].Explanation.String()
	if !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "returned value 1 comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path shows returned value 1 satisfies the declared return type") {
		t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
	}
}

func TestOrdinaryAssignmentRejectsObjectLiteralExplicitAnyMember(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {id: string}
		type Box = {p: Point}
		local raw: any = nil
		local box: Box = {p = {id = "ok"}}
		box.p = {id = raw}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "any") || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want any-to-string ordinary assignment member mismatch", d)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "raw comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path shows raw is string") {
		t.Fatalf("explanation = %q, want any/unknown boundary and missing-proof evidence", got)
	}
	if d := diags[0]; len(d.Labels) < 2 || d.Labels[0].Message != "assigned value" ||
		d.Labels[1].Message != "assignment target" || d.Labels[0].Span != d.Span {
		t.Fatalf("labels/span = %#v/%#v, want assigned value label on offending member plus assignment target", d.Labels, d.Span)
	}
}

func TestDirectCallAcceptsGuardedObjectLiteralAnyMember(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {id: string}
		function take(p: Point)
		end
		function validate(raw: any)
			if type(raw) == "string" then
				take({id = raw})
			end
		end
	`, []string{"type"})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after guarded concrete witness", diags)
	}
}

type diagnosticShapeWant struct {
	code           diagnostic.Code
	severity       diagnostic.Severity
	message        string
	span           diagnostic.Span
	labels         []diagnosticLabelWant
	evidence       []diagnosticEvidenceWant
	help           string
	renderContains []string
}

type diagnosticLabelWant struct {
	message string
	span    diagnostic.Span
}

type diagnosticEvidenceWant struct {
	kind    diagnostic.EvidenceKind
	trust   diagnostic.TrustKind
	reason  diagnostic.EvidenceReason
	message string
	span    diagnostic.Span
}

func requireDiagnosticShape(t *testing.T, src string, diags []diagnostic.Diagnostic, want diagnosticShapeWant) diagnostic.Diagnostic {
	t.Helper()
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	wantSeverity := want.severity
	if d.Code != want.code || d.Severity != wantSeverity {
		t.Fatalf("diagnostic code/severity = %s/%s, want %s/%s", d.Code, d.Severity, want.code, wantSeverity)
	}
	if d.Message != want.message {
		t.Fatalf("message = %q, want %q", d.Message, want.message)
	}
	if d.Span != want.span {
		t.Fatalf("span = %#v, want %#v", d.Span, want.span)
	}
	positionSpan := diagnostic.Span{StartLine: d.Position.Line, StartCol: d.Position.Column, EndLine: d.Position.EndLine, EndCol: d.Position.EndColumn}
	if positionSpan != want.span {
		t.Fatalf("position span = %#v, want diagnostic span %#v", positionSpan, want.span)
	}
	if d.Help != want.help {
		t.Fatalf("help = %q, want %q", d.Help, want.help)
	}
	requireDiagnosticLabels(t, d.Labels, want.labels)
	requireDiagnosticEvidence(t, d.Explanation.Evidence(), want.evidence)
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"main.lua": src},
		ShowSourceLabelRows: true,
	})
	requireRenderedContains(t, rendered, want.renderContains...)
	return d
}

func requireDiagnosticLabels(t *testing.T, got []diagnostic.Label, want []diagnosticLabelWant) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("labels = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i].Message != want[i].message || got[i].Span != want[i].span {
			t.Fatalf("label[%d] = %#v, want message %q span %#v", i, got[i], want[i].message, want[i].span)
		}
	}
}

func requireDiagnosticEvidence(t *testing.T, got []diagnostic.Evidence, want []diagnosticEvidenceWant) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("evidence = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i].Kind != want[i].kind ||
			got[i].Trust != want[i].trust ||
			got[i].Reason != want[i].reason ||
			got[i].Message != want[i].message ||
			got[i].Span != want[i].span {
			t.Fatalf("evidence[%d] = %#v, want kind %s trust %s reason %s span %#v message %q",
				i, got[i], want[i].kind, want[i].trust, want[i].reason, want[i].span, want[i].message)
		}
	}
}
