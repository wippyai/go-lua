package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestCheckReportsOptionalConcatOperand(t *testing.T) {
	result := Check(`
local maybe: string? = nil
local label: string = "prefix:" .. maybe
return label
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	if diag.Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning by default", diag.Severity)
	}
	if !strings.Contains(diag.Message, "may be nil") {
		t.Fatalf("message = %q, want nil-risk concat diagnostic", diag.Message)
	}
	requireLabelMessage(t, diag, "value may be nil")
	evidence := diag.Explanation.Evidence()
	if len(evidence) != 2 {
		t.Fatalf("evidence = %#v, want operand fact and missing nil guard", evidence)
	}
	requireEvidenceMessage(t, diag, "right operand `maybe` can be string or nil here")
	if diag.Help != "Guard `maybe` or provide a default string before using `..`." {
		t.Fatalf("help = %q, want actionable concat remediation", diag.Help)
	}
}

func TestCheckReportsLeftOptionalConcatOperand(t *testing.T) {
	result := Check(`
local maybe: string? = nil
local label: string = maybe .. ":suffix"
return label
`)
	diag := requireDiagnosticCodeWithEvidence(t, result, diagnostics.CodeConcatOperand, "left operand `maybe` can be string or nil here")
	if diag.Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning by default", diag.Severity)
	}
	if !strings.Contains(diag.Message, "left operand") || !strings.Contains(diag.Message, "may be nil") {
		t.Fatalf("message = %q, want left-operand nil-risk concat diagnostic", diag.Message)
	}
	requireLabelMessage(t, diag, "value may be nil")
	if diag.Help != "Guard `maybe` or provide a default string before using `..`." {
		t.Fatalf("help = %q, want actionable concat remediation", diag.Help)
	}
}

func TestCheckConcatOperandMessageNamesPathOperand(t *testing.T) {
	result := Check(`
type Part = { text: string? }
local function render(part: Part): string
	return "text:" .. part.text
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	if !strings.Contains(diag.Message, "right operand `part.text`") {
		t.Fatalf("message = %q, want path operand named", diag.Message)
	}
}

func TestCheckOptionalConcatOperandRendersPathEvidence(t *testing.T) {
	src := strings.TrimLeft(`
local maybe: string? = nil
return "prefix:" .. maybe
`, "\n")
	result := Check(src)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := "warning[type.operator.concat_operand]: right operand `maybe` of `..` may be nil\n" +
		" --> test.lua:2:21\n" +
		"  |\n" +
		"2 | return \"prefix:\" .. maybe\n" +
		"  |                     ↑ value may be nil\n" +
		"\n" +
		"because:\n" +
		"  1. proven: right operand `maybe` can be string or nil here\n" +
		"  2. missing proof: no guard on this path proves maybe is non-nil\n" +
		"\n" +
		"help: Guard `maybe` or provide a default string before using `..`."
	assertRenderedEqual(t, rendered, want)
}

func TestCheckConcatOperandPolicyControlsSeverity(t *testing.T) {
	src := `
local maybe: string? = nil
local label: string = "prefix:" .. maybe
return label
`
	disabled := Check(src, WithDiagnosticRule(diagnostics.CodeConcatOperand, diagnostic.Disable()))
	if len(disabled.Diagnostics) != 0 {
		t.Fatalf("disabled diagnostics = %#v, want none", disabled.Diagnostics)
	}
	promoted := Check(src, WithDiagnosticRule(
		diagnostics.CodeConcatOperand,
		diagnostic.OverrideSeverity(diagnostic.SeverityError),
	))
	diag := requireDiagnosticCode(t, promoted, diagnostics.CodeConcatOperand)
	if diag.Severity != diagnostic.SeverityError {
		t.Fatalf("severity = %s, want promoted error", diag.Severity)
	}
}

func TestCheckReportsArrayIndexConcatWithoutRangeProof(t *testing.T) {
	result := Check(`
local function label(arr: {string}, i: number): string
    return "item:" .. arr[i]
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	requireEvidenceMessage(t, diag, "right operand `arr[i]` can be string or nil here")
}

func TestCheckAcceptsNumericForArrayIndexConcatWithRangeProof(t *testing.T) {
	result := Check(`
local function labels(arr: {string})
    for i = 1, #arr do
        local label: string = "item:" .. arr[i]
        print(label)
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for numeric-for in-range concat", result.Diagnostics)
	}
}

func TestCheckAcceptsReverseNumericForArrayIndexConcatWithRangeProof(t *testing.T) {
	result := Check(`
local function labels(arr: {string})
    for i = #arr, 1, -1 do
        local label: string = "item:" .. arr[i]
        print(label)
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for reverse numeric-for in-range concat", result.Diagnostics)
	}
}

func TestCheckReportsNumericForZeroStartArrayIndexConcat(t *testing.T) {
	result := Check(`
local function labels(arr: {string})
    for i = 0, #arr do
        local label: string = "item:" .. arr[i]
        print(label)
    end
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	requireEvidenceMessage(t, diag, "right operand `arr[i]` can be string or nil here")
}

func TestCheckReportsReverseNumericForZeroLimitArrayIndexConcat(t *testing.T) {
	result := Check(`
local function labels(arr: {string})
    for i = #arr, 0, -1 do
        local label: string = "item:" .. arr[i]
        print(label)
    end
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	requireEvidenceMessage(t, diag, "right operand `arr[i]` can be string or nil here")
}

func TestCheckReportsNumericForDifferentLengthArrayIndexConcat(t *testing.T) {
	result := Check(`
local function labels(arr: {string}, bounds: {string})
    for i = 1, #bounds do
        local label: string = "item:" .. arr[i]
        print(label)
    end
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	requireEvidenceMessage(t, diag, "right operand `arr[i]` can be string or nil here")
}

func TestCheckReportsReverseNumericForDifferentLengthArrayIndexConcat(t *testing.T) {
	result := Check(`
local function labels(arr: {string}, bounds: {string})
    for i = #bounds, 1, -1 do
        local label: string = "item:" .. arr[i]
        print(label)
    end
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeConcatOperand)
	requireEvidenceMessage(t, diag, "right operand `arr[i]` can be string or nil here")
}

func TestCheckLoopConcatAssignmentWideningReportsNumberString(t *testing.T) {
	result := Check(`
local function concat(parts: {string})
    local acc = 0
    for i = 1, #parts do
        acc = acc .. parts[i]
    end
    local n: number = acc
    print(n)
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "number | string") || !strings.Contains(diag.Message, "not number") {
		t.Fatalf("message = %q, want number|string to number assignment", diag.Message)
	}
	requireEvidenceMessage(t, diag, "acc has type number | string")
	requireEvidenceMessage(t, diag, "n is declared as number")
}

func TestCheckLoopLocalConcatMissingBranchReportsOptionalAssignment(t *testing.T) {
	result := Check(`
local function labels(arr: {string}, flag: boolean)
    for i = 1, #arr do
        local note: string? = nil
        if flag then
            note = "item:" .. arr[i]
        end
        local label: string = note
        print(label)
    end
end
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "cannot assign note") || !strings.Contains(diag.Message, "may be nil") {
		t.Fatalf("message = %q, want optional string assignment", diag.Message)
	}
	requireEvidenceMessage(t, diag, "note can be string or nil here")
	requireEvidenceMessage(t, diag, "label is declared as string")
}

func TestCheckLoopLocalConcatAssignedInBothBranchesFlowsIntoObject(t *testing.T) {
	result := Check(`
type Step = { note: string }
local function labels(arr: {string})
    for i = #arr, 1, -1 do
        local note: string
        if arr[i] == "" then
            note = "empty:" .. arr[i]
        else
            note = "item:" .. arr[i]
        end
        local step: Step = { note = note }
        print(step.note)
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for definitely assigned loop-local concat object field", result.Diagnostics)
	}
}

func TestCheckLoopLocalDiscriminatedConcatAssignedInBothBranchesFlowsIntoObject(t *testing.T) {
	result := Check(`
type Release = { kind: "release", token: string }
type Refund = { kind: "refund", payment: string }
type Comp = Release | Refund
type Step = { note: string }
local function labels(items: {Comp})
    for i = #items, 1, -1 do
        local item = items[i]
        local note: string
        if item.kind == "release" then
            note = "release:" .. item.token
        else
            note = "refund:" .. item.payment
        end
        local step: Step = { note = note }
        print(step.note)
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for discriminated loop-local concat object field", result.Diagnostics)
	}
}

func TestCheckLoopLocalDiscriminatedConcatFlowsIntoUnionContextObject(t *testing.T) {
	result := Check(`
type Release = { kind: "release", token: string }
type Refund = { kind: "refund", payment: string }
type Comp = Release | Refund
type ActionStep = { kind: "action", note: string }
type CompensationStep = { kind: "compensation", note: string }
type AuditStep = { kind: "audit", note: string, at: number }
type Step = ActionStep | CompensationStep | AuditStep
local function emit(step: Step)
    print(step.note)
end
local function labels(items: {Comp})
    for i = #items, 1, -1 do
        local item = items[i]
        local note: string
        if item.kind == "release" then
            note = "release:" .. item.token
        else
            note = "refund:" .. item.payment
        end
        local step: Step = { kind = "compensation", note = note }
        emit(step)
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for union-context loop-local concat object field", result.Diagnostics)
	}
}

func TestCheckImportedUnionContextObjectKeepsLoopLocalConcatField(t *testing.T) {
	protocol := CheckAndExport(`
type Release = { kind: "release", token: string }
type Refund = { kind: "refund", payment: string }
type Comp = Release | Refund
type Saga = { order_id: string, compensations: {Comp} }
type ActionStep = { kind: "action", note: string }
type CompensationStep = { kind: "compensation", note: string }
type AuditStep = { kind: "audit", note: string, at: number }
type Step = ActionStep | CompensationStep | AuditStep
return {}
`, "protocol")
	result := Check(`
local protocol = require("protocol")
local function emit(step: protocol.Step)
    print(step.note)
end
local function labels(items: {protocol.Comp})
    for i = #items, 1, -1 do
        local item = items[i]
        local note: string
        if item.kind == "release" then
            note = "release:" .. item.token
        else
            note = "refund:" .. item.payment
        end
        local step: protocol.Step = { kind = "compensation", note = note }
        emit(step)
    end
end
`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for imported union-context loop-local concat object field", result.Diagnostics)
	}
}

func TestCheckImportedNestedArrayUnionContextObjectKeepsLoopLocalConcatField(t *testing.T) {
	protocol := CheckAndExport(`
type Release = { kind: "release", token: string }
type Refund = { kind: "refund", payment: string }
type Comp = Release | Refund
type Saga = { order_id: string, compensations: {Comp} }
type ActionStep = { kind: "action", note: string, order_id: string? }
type CompensationStep = { kind: "compensation", note: string, order_id: string? }
type AuditStep = { kind: "audit", note: string, at: number }
type Step = ActionStep | CompensationStep | AuditStep
return {}
`, "protocol")
	result := Check(`
local protocol = require("protocol")
local function emit(step: protocol.Step)
    print(step.note)
end
local function labels(saga: protocol.Saga)
    for i = #saga.compensations, 1, -1 do
        local item = saga.compensations[i]
        local note: string
        if item.kind == "release" then
            note = "release:" .. item.token
        else
            note = "refund:" .. item.payment
        end
        local step: protocol.Step = {
            kind = "compensation",
            note = note,
            order_id = saga.order_id,
        }
        emit(step)
    end
end
`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for imported nested-array union-context loop-local concat object field", result.Diagnostics)
	}
}

func TestCheckGuardedOptionalImportedNestedArrayKeepsLoopLocalConcatField(t *testing.T) {
	protocol := CheckAndExport(`
type Release = { kind: "release", token: string }
type Refund = { kind: "refund", payment: string }
type Comp = Release | Refund
type Saga = { order_id: string, compensations: {Comp} }
type ActionStep = { kind: "action", note: string, order_id: string? }
type CompensationStep = { kind: "compensation", note: string, order_id: string? }
type AuditStep = { kind: "audit", note: string, at: number }
type Step = ActionStep | CompensationStep | AuditStep
return {}
`, "protocol")
	result := Check(`
local protocol = require("protocol")
local function emit(step: protocol.Step)
    print(step.note)
end
local function labels(saga: protocol.Saga?)
    if saga then
        for i = #saga.compensations, 1, -1 do
            local item = saga.compensations[i]
            local note: string
            if item.kind == "release" then
                note = "release:" .. item.token
            else
                note = "refund:" .. item.payment
            end
            local step: protocol.Step = {
                kind = "compensation",
                note = note,
                order_id = saga.order_id,
            }
            emit(step)
        end
    end
end
`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for guarded optional imported nested-array loop-local concat object field", result.Diagnostics)
	}
}

func TestCheckGuardedOptionalImportedMethodReturnKeepsLoopLocalConcatField(t *testing.T) {
	protocol := CheckAndExport(`
type Release = { kind: "release", token: string }
type Refund = { kind: "refund", payment: string }
type Comp = Release | Refund
type Saga = { order_id: string, compensations: {Comp} }
type Store = { lookup: (self: Store, id: string) -> Saga? }
type ActionStep = { kind: "action", note: string, order_id: string? }
type CompensationStep = { kind: "compensation", note: string, order_id: string? }
type AuditStep = { kind: "audit", note: string, at: number }
type Step = ActionStep | CompensationStep | AuditStep
return {}
`, "protocol")
	result := Check(`
local protocol = require("protocol")
local function emit(step: protocol.Step)
    print(step.note)
end
local function labels(store: protocol.Store, id: string)
    local saga = store:lookup(id)
    if saga then
        for i = #saga.compensations, 1, -1 do
            local item = saga.compensations[i]
            local note: string
            if item.kind == "release" then
                note = "release:" .. item.token
            else
                note = "refund:" .. item.payment
            end
            local step: protocol.Step = {
                kind = "compensation",
                note = note,
                order_id = saga.order_id,
            }
            emit(step)
        end
    end
end
`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for guarded optional imported method-return loop-local concat object field", result.Diagnostics)
	}
}

func TestCheckConcatAcceptsPostReturnDiscriminantNarrowedField(t *testing.T) {
	result := Check(`
type BoxPayload = { kind: "box", box: { label: string } }
type TombstonePayload = { kind: "tombstone", tombstone: { reason: string } }
type Payload = BoxPayload | TombstonePayload
local function label(payload: Payload): string
    if payload.kind == "box" then
        return "box:" .. payload.box.label
    end
    return "tombstone:" .. payload.tombstone.reason
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after post-return discriminant narrowing", result.Diagnostics)
	}
}

func TestCheckConcatAcceptsNestedPostReturnDiscriminantNarrowedField(t *testing.T) {
	result := Check(`
type BoxPayload = { kind: "box", box: { label: string }, next: Payload? }
type TombstonePayload = { kind: "tombstone", tombstone: { reason: string } }
type Payload = BoxPayload | TombstonePayload
local function label(payload: Payload): string
    if payload.kind == "box" then
        local next_payload = payload.next
        if next_payload then
            if next_payload.kind == "box" then
                return "nested-box:" .. next_payload.box.label
            end
            return "nested-tombstone:" .. next_payload.tombstone.reason
        end
        return "box:" .. payload.box.label
    end
    return "tombstone:" .. payload.tombstone.reason
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for nested post-return discriminant narrowing", result.Diagnostics)
	}
}

func TestCheckConcatAcceptsLogicalAndGuardedOperands(t *testing.T) {
	result := Check(`
local function label(left: string?, right: string?): string
    if left and right then
        return left .. ":" .. right
    end
    return ""
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for logical-and guarded concat operands", result.Diagnostics)
	}
}

func TestCheckConcatAcceptsOperandGuardedInsideLogicalAndExpression(t *testing.T) {
	result := Check(`
local function cache_key(prefix: string, tool_id: string, tool_name: string?): string
	return prefix .. tool_id .. (tool_name and (":" .. tool_name) or "")
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for concat operand guarded inside logical-and expression", result.Diagnostics)
	}
}

func TestCheckConcatAcceptsDeepExpressionLocalGuards(t *testing.T) {
	cases := map[string]string{
		"nested-and-chain": `
local function label(a: string?, b: string?): string
	return a and b and ("pair:" .. a .. ":" .. b) or ""
end
`,
		"explicit-not-nil-and": `
local function label(value: string?): string
	return value ~= nil and ("value:" .. value) or ""
end
`,
		"nil-check-or-fallback": `
local function label(value: string?)
	return value == nil or ("value:" .. value)
end
`,
		"negated-nil-check-and": `
local function label(value: string?): string
	return not (value == nil) and ("value:" .. value) or ""
end
`,
		"nested-or-with-type-guarded-and": `
local function label(success: boolean, value: any): string
	return success and "ok" or (type(value) == "string" and ("value:" .. value) or "failed")
end
`,
		"nil-initialized-local-assigned-any-field-then-type-guarded": `
local STATUS = { OK = "ok", ERR = "failed: " }
local function label(result: any): string
	local success = true
	local final_error = nil
	if type(result) == "table" then
		success = result.success ~= false
		final_error = result.error
	end
	return success and STATUS.OK or (type(final_error) == "string" and (STATUS.ERR .. final_error) or "failed")
end
`,
	}
	for name, src := range cases {
		if diagnostics := Check(src).Diagnostics; len(diagnostics) != 0 {
			t.Fatalf("%s: diagnostics = %#v, want expression-local guard to prove concat operand present", name, diagnostics)
		}
	}
}

func TestCheckConcatAcceptsIfGuardedValueAssignedInLoop(t *testing.T) {
	result := Check(`
type Part = {
    kind: "text" | "refusal",
    text: string?,
    refusal: string?,
}

local function refusal_message(parts: {Part}): string?
    local refusal = nil
    for _, part in ipairs(parts) do
        if part.kind == "refusal" and part.refusal then
            refusal = part.refusal
        end
    end
    if refusal then
        return "Request was refused: " .. refusal
    end
    return nil
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want if-guarded loop-assigned value to be present for concat", result.Diagnostics)
	}
}

func TestCheckConcatStillReportsOptionalOnUnprovenExpressionFallback(t *testing.T) {
	result := Check(`
local function label(value: string?): string
	return value or ("value:" .. value)
end
`)
	if len(result.Diagnostics) == 0 {
		t.Fatalf("diagnostics = nil, want concat warning because `value or ...` proves value absent in fallback")
	}
}

func TestCheckConcatAcceptsPostReturnNotNilGuardedOperand(t *testing.T) {
	result := Check(`
local function label(value: string?): string
    if not value then
        return ""
    end
    return "value:" .. value
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after post-return not-nil guard", result.Diagnostics)
	}
}

func TestCheckReportsOptionalIndexVariableConcatOperand(t *testing.T) {
	result := Check(`
local function label(items: {string}, i: number?): string
    return "item:" .. items[i]
end
`)
	diag := requireDiagnosticCodeWithEvidence(t, result, diagnostics.CodeConcatOperand, "right operand `items[i]` can be string or nil here")
	if diag.Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning by default", diag.Severity)
	}
}

func TestCheckConcatReportsNonExhaustiveDiscriminantFallthrough(t *testing.T) {
	result := Check(`
type RouteA = { kind: "route_a" }
type RouteB = { kind: "route_b" }
type StreamPayload = { kind: "stream", router: { selected: RouteA | RouteB } }
type TombstonePayload = { kind: "tombstone", tombstone: { reason: string } }
type BoxPayload = { kind: "box", box: { label: string }, node: { next: BoxPayload | StreamPayload | TombstonePayload } }
type Payload = BoxPayload | StreamPayload | TombstonePayload
local function label(payload: Payload): string
    if payload.kind == "box" then
        local next_payload = payload.node.next
        if next_payload.kind == "stream" then
            local selected_route = next_payload.router.selected
            if selected_route.kind == "route_b" then
                return "stream-route-b"
            end
        end
        if next_payload.kind == "box" then
            return "box:" .. next_payload.box.label
        end
        return "nested-tombstone:" .. next_payload.tombstone.reason
    end
    return "tombstone:" .. payload.tombstone.reason
end
`)
	diag := requireDiagnosticCodeWithEvidence(t, result, diagnostics.CodeConcatOperand, "right operand `payload.tombstone.reason` can be string or nil here")
	if diag.Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning by default", diag.Severity)
	}
	requireEvidenceMessage(t, diag, "right operand `payload.tombstone.reason` can be string or nil here")
}

func TestCheckConcatAcceptsLogicalOrFallbackLocalOperand(t *testing.T) {
	result := Check(`
type RenderPayload = { kind: "render", template: string, values: {[string]: string} }
type AuditPayload = { kind: "audit", action: string }
type Payload = RenderPayload | AuditPayload
local function render(payload: Payload): string
    if payload.kind == "render" then
        local subject = payload.values["subject"] or payload.template
        return payload.template .. ":" .. subject
    end
    return payload.action
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for present logical-or fallback concat operand", result.Diagnostics)
	}
}

func TestCheckConcatAcceptsTruthyGuardedLocalOperand(t *testing.T) {
	result := Check(`
type Message = { id: string, payload: { owner: string? } }
local function label(message: Message): string
    local owner = message.payload.owner
    if owner then
        return message.id .. ":" .. owner
    end
    return message.id
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for truthy-guarded local concat operand", result.Diagnostics)
	}
}

func TestCheckConcatReportsTruthyGuardInvalidatedByReassignment(t *testing.T) {
	result := Check(`
local function label(owner: string?): string
    if owner then
        owner = nil
        return "owner:" .. owner
    end
    return "missing"
end
`)
	diag := requireDiagnosticCodeWithEvidence(t, result, diagnostics.CodeConcatOperand, "right operand `owner` has type nil")
	if diag.Severity != diagnostic.SeverityWarning {
		t.Fatalf("severity = %s, want warning by default", diag.Severity)
	}
}

func requireDiagnosticCode(t *testing.T, result Result, code diagnostic.Code) diagnostic.Diagnostic {
	t.Helper()
	for _, diag := range result.Diagnostics {
		if diag.Code == code {
			return diag
		}
	}
	t.Fatalf("diagnostics = %#v, want code %s", result.Diagnostics, code)
	return diagnostic.Diagnostic{}
}

func requireDiagnosticCodeWithEvidence(t *testing.T, result Result, code diagnostic.Code, evidenceContains string) diagnostic.Diagnostic {
	t.Helper()
	for _, diag := range result.Diagnostics {
		if diag.Code != code {
			continue
		}
		for _, evidence := range diag.Explanation.Evidence() {
			if strings.Contains(evidence.Message, evidenceContains) {
				return diag
			}
		}
	}
	t.Fatalf("diagnostics = %#v, want code %s with evidence containing %q", result.Diagnostics, code, evidenceContains)
	return diagnostic.Diagnostic{}
}
