package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestDirectCallSummaryUsesReassignedLocalFunctionValue(t *testing.T) {
	src := strings.TrimLeft(`
local function f(): string
    return "old"
end

local function g(): number
    return 1
end

f = g

local n: number = f()
local s: string = f()
`, "\n")
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallResultAssignment,
		DiagnosticCount: 1,
		Line:            12,
		Column:          19,
		Span: diagnostic.Span{
			StartLine: 12,
			StartCol:  19,
			EndLine:   12,
			EndCol:    19,
		},
		MessageContains: []string{"call result 1", "number", "not string"},
		EvidenceMin:     2,
		EvidenceContains: []string{
			"returns number",
			"assignment target s requires string",
		},
		EvidenceOrdered: []string{
			"returns number",
			"assignment target s requires string",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"f", "returns number"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"assignment target s", "string"},
			},
		},
		LabelMin: 2,
		LabelContains: []string{
			"declared type",
			"call result",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.call.direct.result_assignment]: call result 1 is number, not string",
			"test.lua:12:19",
			"declared type",
			"12 | local s: string = f()",
			"call result",
			"because:",
			"proven: f returns number",
			"claimed: assignment target s requires string",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenInCallee(t *testing.T) {
	result := Check(`
local function invoke(provider, payload)
    provider.send = function(v: string): () end
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after callee overwrites provider.send with string handler", result.Diagnostics)
	}
}

func TestMemberCallObligationIgnoresProviderBracketMemberOverwrittenInCallee(t *testing.T) {
	result := Check(`
local function invoke(provider, payload)
    provider["send"] = function(v: string): () end
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after callee overwrites provider[\"send\"] with string handler", result.Diagnostics)
	}
}

func TestDirectDynamicIndexWriteProjectsStaticMemberCallableType(t *testing.T) {
	result := Check(`
local p = {}
local key = "send"
p[key] = function(v: string): () end
p.send(1)
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one argument mismatch from dynamic-index string key write", result.Diagnostics)
	}
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "argument")
	requireEvidenceMessage(t, diag, "parameter 1 expects string")
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenByEarlierCall(t *testing.T) {
	result := Check(`
local function install(provider)
    provider.send = function(v: string): () end
end

local function invoke(provider, payload)
    install(provider)
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after earlier call overwrites provider.send with string handler", result.Diagnostics)
	}
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenByImportedDynamicIndexWrite(t *testing.T) {
	mod := CheckAndExport(`
local M = {}
type NumberSender = (number) -> ()
type StringSender = (string) -> ()
type Provider = { send: NumberSender | StringSender }

function M.install(provider: Provider, key: string): ()
    provider[key] = function(v: string): () end
end

return M
`, "ops")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
local ops = require("ops")

local function invoke(provider, payload)
    ops.install(provider, "send")
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`, WithStdlib(), WithModule("ops", mod))
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected stale provider member obligation after imported dynamic-index write: %#v", result.Diagnostics)
		}
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after imported dynamic-index write replaces provider.send", result.Diagnostics)
	}
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenByUntypedImportedDynamicIndexWrite(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.install(provider, key)
    provider[key] = function(v: string): () end
end

return M
`, "ops")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	sig, ok := mod.Manifest.FunctionSignatures["ops.install"]
	if !ok {
		t.Fatalf("missing ops.install function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	requireOperationInvalidatesParam(t, sig, "ops.install", 0)

	result := Check(`
local ops = require("ops")

local function invoke(provider, payload)
    ops.install(provider, "send")
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`, WithStdlib(), WithModule("ops", mod))
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected stale provider member obligation after untyped imported dynamic-index write: %#v", result.Diagnostics)
		}
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after untyped imported dynamic-index write replaces provider.send", result.Diagnostics)
	}
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenByRoundTrippedUntypedImportedDynamicIndexWrite(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.install(provider, key)
    provider[key] = function(v: string): () end
end

return M
`, "ops")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	data, err := manifest.Encode(mod.Manifest)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sig, ok := decoded.FunctionSignatures["ops.install"]
	if !ok {
		t.Fatalf("missing round-tripped ops.install function signature: %#v", decoded.FunctionSignatures)
	}
	if sig.Type != nil {
		t.Fatalf("round-tripped ops.install type = %v, want nil operation-only signature", sig.Type)
	}
	requireOperationInvalidatesParam(t, sig, "ops.install", 0)

	result := Check(`
local ops = require("ops")

local function invoke(provider, payload)
    ops.install(provider, "send")
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`, WithStdlib(), WithManifest("ops", decoded))
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected stale provider member obligation after round-tripped operation-only signature: %#v", result.Diagnostics)
		}
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after round-tripped untyped imported dynamic-index write", result.Diagnostics)
	}
}

func TestImportedUntypedDynamicIndexWriteCarriesPositiveProviderMemberType(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.install(provider, key)
    provider[key] = function(v: string): () end
end

return M
`, "ops")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	data, err := manifest.Encode(mod.Manifest)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sig, ok := decoded.FunctionSignatures["ops.install"]
	if !ok {
		t.Fatalf("missing round-tripped ops.install function signature: %#v", decoded.FunctionSignatures)
	}
	requireOperationInvalidatesParam(t, sig, "ops.install", 0)
	requireOperationDynamicIndexParamKey(t, sig, "ops.install", 0, 1)

	result := Check(`
local ops = require("ops")

local p = {}
ops.install(p, "send")
p.send(1)
`, WithStdlib(), WithManifest("ops", decoded))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly one argument mismatch from imported dynamic-index write; dynamic snapshots: %v", result.Diagnostics, dynamicIndexSnapshotDebug(result))
	}
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "argument")
	requireEvidenceMessage(t, diag, "parameter 1 expects string")
}

func dynamicIndexSnapshotDebug(result Result) []string {
	if result.checked.RootResult() == nil || result.checked.RootResult().Graph() == nil {
		return nil
	}
	root := result.checked.RootResult()
	var out []string
	for _, point := range root.Graph().RPO() {
		if call, ok := root.Call(point); ok && call.HasCalleePath {
			out = append(out, fmt.Sprintf("%d:call=%s", point, call.CalleePath.String()))
		}
		st, ok := root.StateAt(point)
		if !ok {
			continue
		}
		snapshot := st.DynamicIndexFactsSnapshot()
		if snapshot.Top || len(snapshot.Facts) == 0 {
			continue
		}
		for key, fact := range snapshot.Facts {
			keyType, keyOK := typevalue.TypeOf(root.Registry(), fact.KeyValue)
			valueType, valueOK := typevalue.TypeOf(root.Registry(), fact.Value)
			out = append(out, fmt.Sprintf("%d:%s/%s key=%v/%v value=%v/%v admission=%v", point, root.KeySpace().Format(key.Table), key.Site, keyType, keyOK, valueType, valueOK, fact.Admission))
		}
	}
	return out
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenByAliasedUntypedImportedDynamicIndexWrite(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.install(provider, key)
    provider[key] = function(v: string): () end
end

return M
`, "ops")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	sig, ok := mod.Manifest.FunctionSignatures["ops.install"]
	if !ok {
		t.Fatalf("missing ops.install function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	requireOperationInvalidatesParam(t, sig, "ops.install", 0)

	result := Check(`
local ops = require("ops")
local install = ops.install

local function invoke(provider, payload)
    install(provider, "send")
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`, WithStdlib(), WithModule("ops", mod))
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected stale provider member obligation after aliased untyped imported dynamic-index write: %#v", result.Diagnostics)
		}
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after aliased untyped imported dynamic-index write replaces provider.send", result.Diagnostics)
	}
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenByForwardedUntypedImportedDynamicIndexWrite(t *testing.T) {
	mod := CheckAndExport(`
local M = {}

function M.install(provider, key)
    provider[key] = function(v: string): () end
end

function M.wrap(provider, key)
    M.install(provider, key)
end

return M
`, "ops")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	sig, ok := mod.Manifest.FunctionSignatures["ops.wrap"]
	if !ok {
		t.Fatalf("missing ops.wrap function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	requireOperationInvalidatesParam(t, sig, "ops.wrap", 0)

	result := Check(`
local ops = require("ops")

local function invoke(provider, payload)
    ops.wrap(provider, "send")
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "ok")
`, WithStdlib(), WithModule("ops", mod))
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected stale provider member obligation after forwarded untyped imported dynamic-index write: %#v", result.Diagnostics)
		}
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after forwarded untyped imported dynamic-index write replaces provider.send", result.Diagnostics)
	}
}

func TestMemberCallObligationIgnoresProviderMemberOverwrittenByCapturedCallback(t *testing.T) {
	result := Check(`
local function invoke(provider, mutate, payload)
    mutate()
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

local function mutate()
    p.send = function(v: string): () end
end

invoke(p, mutate, "ok")
`)
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected stale provider member obligation after callback mutation: %#v", result.Diagnostics)
		}
	}
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            12,
		Column:          14,
		MessageContains: []string{
			"cannot assign",
			"string",
			"fun(number)",
		},
	})
}

func TestMemberCallObligationIgnoresNestedProviderMemberOverwrittenByCapturedCallback(t *testing.T) {
	result := Check(`
local function invoke(provider, mutate, payload)
    mutate()
    provider.client.send(payload)
end

local p: { client: { send: (number) -> () } } = {
    client = {
        send = function(v: number): () end,
    },
}

local function mutate()
    p.client.send = function(v: string): () end
end

invoke(p, mutate, "ok")
`)
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected stale nested provider member obligation after callback mutation: %#v", result.Diagnostics)
		}
	}
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            14,
		Column:          21,
		MessageContains: []string{
			"cannot assign",
			"string",
			"fun(number)",
		},
	})
}

func TestMemberCallObligationKeepsProviderEvidenceWhenCapturedCallbackMutatesDifferentMember(t *testing.T) {
	result := Check(`
local function invoke(provider, mutate, payload)
    mutate()
    provider.send(payload)
end

local p: { send: (number) -> (), close: () -> () } = {
    send = function(v: number): () end,
    close = function(): () end,
}

local function mutate()
    p.close = function(): () end
end

invoke(p, mutate, "ok")
`)
	diag := requireDiagnosticCodeWithEvidence(t, result, diagnostics.CodeDirectCallArgType, "inside invoke, argument 3 is passed to argument 1.send parameter 1, which requires number")
	requireEvidenceMessage(t, diag, "inside invoke, argument 3 is passed to argument 1.send parameter 1, which requires number")
}

func TestMemberCallObligationSuppressesExplicitAnyProviderEvidence(t *testing.T) {
	result := Check(`
local function invoke(provider, payload)
    provider.send(payload)
end

local provider = ({ send = function(v: number): () end } :: any)
invoke(provider, "ok")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none because explicit any provider has no stable structural validation", result.Diagnostics)
	}
}

func TestMemberCallObligationReportsNestedDIProviderMemberMismatch(t *testing.T) {
	result := Check(`
local function invoke(box, payload)
    local client = box.client
    client.send(payload)
end

local p = {}
p.send = function(v: number): () end

local box = { client = p }
invoke(box, "bad")
`)
	diag := requireDiagnosticCodeWithEvidence(t, result, diagnostics.CodeDirectCallArgType, "inside invoke, argument 2 is passed to argument 1.client.send parameter 1, which requires number")
	requireEvidenceMessage(t, diag, "argument")
	requireEvidenceMessage(t, diag, "inside invoke, argument 2 is passed to argument 1.client.send parameter 1, which requires number")
}

func TestMemberCallObligationIgnoresNestedProviderMemberOverwrittenInCallee(t *testing.T) {
	result := Check(`
local function invoke(box, payload)
    box.client.send = function(v: string): () end
    box.client.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke({ client = p }, "ok")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after callee overwrites box.client.send with string handler", result.Diagnostics)
	}
}

func TestMemberCallObligationIgnoresNestedProviderMemberOverwrittenThroughAlias(t *testing.T) {
	result := Check(`
local function invoke(box, payload)
    local client = box.client
    client.send = function(v: string): () end
    client.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke({ client = p }, "ok")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after alias overwrites box.client.send with string handler", result.Diagnostics)
	}
}

func TestDirectMemberCallUsesReassignedMemberFunctionValue(t *testing.T) {
	result := Check(`
local provider = {}
function provider.send(v: number): () end

provider.send = function(v: string): () end

provider.send("ok")
provider.send(1)
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly one error for calling current string handler with number", result.Diagnostics)
	}
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "argument")
	requireEvidenceMessage(t, diag, "parameter 1 expects string")
}

func TestDirectMemberCallUsesReassignedAncestorFunctionValue(t *testing.T) {
	result := Check(`
local root = { api = {} }
function root.api.send(v: number): () end

root.api = {
    send = function(v: string): () end,
}

root.api.send("ok")
root.api.send(1)
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly one error for calling reassigned string handler with number", result.Diagnostics)
	}
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "argument")
	requireEvidenceMessage(t, diag, "parameter 1 expects string")
}

func TestMemberCallObligationReportsStableProviderMemberMismatch(t *testing.T) {
	src := strings.TrimLeft(`
local function invoke(provider, payload)
    provider.send(payload)
end

local p: { send: (number) -> () } = {
    send = function(v: number): () end,
}

invoke(p, "bad")
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            9,
		Column:          11,
		Span: diagnostic.Span{
			StartLine: 9,
			StartCol:  11,
			EndLine:   9,
			EndCol:    15,
		},
		MessageContains: []string{
			"argument 2",
			`"bad"`,
			"not number",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				Span:            diagnostic.Span{StartLine: 9, StartCol: 11, EndLine: 9, EndCol: 15},
				MessageContains: []string{"argument 2", `literal value "bad"`},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				Span:            diagnostic.Span{StartLine: 9, StartCol: 11, EndLine: 9, EndCol: 15},
				MessageContains: []string{"inside invoke", "argument 1.send parameter 1", "requires number"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no proof", "argument 2 is number"},
			},
		},
		LabelContains: []string{"argument value"},
		HelpContains:  []string{"Pass a value for argument 2", "change the callee signature"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.call.direct.argument_type]: argument 2 is "bad", not number`,
			`9 | invoke(p, "bad")`,
			`  |           ↑ argument value`,
			`1. proven: argument 2 has literal value "bad"`,
			`2. proven: inside invoke, argument 2 is passed to argument 1.send parameter 1, which requires number`,
			`3. missing proof: no proof on this path shows argument 2 is number`,
			`help: Pass a value for argument 2 that satisfies the parameter type, or change the callee signature if that argument is valid.`,
		},
		RenderNotContains: []string{
			"want number",
			"expects number",
			"^~",
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.argument_type]: argument 2 is "bad", not number
 --> test.lua:9:11
  |
9 | invoke(p, "bad")
  |           ↑ argument value

because:
  1. proven: argument 2 has literal value "bad"
  2. proven: inside invoke, argument 2 is passed to argument 1.send parameter 1, which requires number
  3. missing proof: no proof on this path shows argument 2 is number

help: Pass a value for argument 2 that satisfies the parameter type, or change the callee signature if that argument is valid.`
	assertRenderedEqual(t, rendered, want)
}

func requireOperationInvalidatesParam(t *testing.T, sig signature.Function, name string, paramIndex int) {
	t.Helper()
	if sig.OperationalEffects == nil {
		t.Fatalf("%s operational effects = nil, want parameter path invalidation evidence", name)
	}
	want := pathdom.NewPlaceholder(paramIndex)
	for _, invalidation := range sig.OperationalEffects.PathInvalidations {
		if invalidation.Path.Equal(want) {
			return
		}
	}
	t.Fatalf("%s path invalidations = %#v, want %s", name, sig.OperationalEffects.PathInvalidations, want.String())
}

func requireOperationDynamicIndexParamKey(t *testing.T, sig signature.Function, name string, tableParam, keyParam int) {
	t.Helper()
	if sig.OperationalEffects == nil {
		t.Fatalf("%s operational effects = nil, want dynamic-index evidence", name)
	}
	wantTable := pathdom.NewPlaceholder(tableParam)
	wantKey := pathdom.NewPlaceholder(keyParam)
	for _, fact := range sig.OperationalEffects.DynamicIndexFacts {
		if fact.Table.Equal(wantTable) && fact.Key.Path.Equal(wantKey) {
			return
		}
	}
	t.Fatalf("%s dynamic-index facts = %#v, want table %s keyed by %s", name, sig.OperationalEffects.DynamicIndexFacts, wantTable.String(), wantKey.String())
}

func TestMemberCallObligationReportsStableProviderStaticIntMemberMismatch(t *testing.T) {
	result := Check(`
local function invoke(provider: any, payload)
    provider[1](payload)
end

local p: { [1]: (number) -> () } = {
    [1] = function(v: number): () end,
}

invoke(p, "bad")
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, `argument 2 has literal value "bad"`)
	requireEvidenceMessage(t, diag, "inside invoke, argument 2 is passed to argument 1[1] parameter 1, which requires number")
}

func TestMemberCallObligationReportsInferredStaticIntProviderMemberMismatch(t *testing.T) {
	result := Check(`
local function invoke(provider: any, payload)
    provider[1](payload)
end

local p = {}
p[1] = function(v: number): () end

invoke(p, "bad")
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, `argument 2 has literal value "bad"`)
	requireEvidenceMessage(t, diag, "inside invoke, argument 2 is passed to argument 1[1] parameter 1, which requires number")
}

func TestMemberCallObligationReportsInferredStaticStringProviderMemberMismatch(t *testing.T) {
	result := Check(`
local function invoke(provider: any, payload)
    provider.send(payload)
end

local p = {}
p["send"] = function(v: number): () end

invoke(p, "bad")
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, `argument 2 has literal value "bad"`)
	requireEvidenceMessage(t, diag, "inside invoke, argument 2 is passed to argument 1.send parameter 1, which requires number")
}

func TestMemberCallObligationUsesLatestInferredProviderMember(t *testing.T) {
	result := Check(`
local function invoke(provider: any, payload)
    provider[1](payload)
end

local p = {}
p[1] = function(v: number): () end
p[1] = function(v: string): () end

invoke(p, 42)
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "argument 2 has literal value 42")
	requireEvidenceMessage(t, diag, "inside invoke, argument 2 is passed to argument 1[1] parameter 1, which requires string")
}

func TestMemberCallObligationDoesNotUseBranchOnlyProviderMember(t *testing.T) {
	result := Check(`
local function invoke(provider: any, payload)
    provider[1](payload)
end

local p = {}
local enabled: boolean = false
if enabled then
    p[1] = function(v: number): () end
end

invoke(p, "bad")
`)
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType {
			t.Fatalf("unexpected direct-call argument diagnostic from branch-only provider member: %#v", diag)
		}
	}
}
