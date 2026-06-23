package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestCheckAndExportPublishesErrorReturnFromImportedGenericResultField(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local result = {}
		result.Result = Result
		return result
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	repoMod := CheckAndExport(`
		local result = require("result")
		type User = { id: string, email: string }
		local repo = {}
		function repo.find_by_id(id: string): Result<User>
			if id == "" then
				return { ok = false, error = "missing" }
			end
			return { ok = true, value = { id = id, email = "a@test" } }
		end
		return repo
	`, "repo", WithStdlib(), WithModule("result", resultMod))
	if len(repoMod.Errors) != 0 {
		t.Fatalf("repo module errors = %#v, want none", repoMod.Errors)
	}

	serviceMod := CheckAndExport(`
		local repo = require("repo")
		local service = {}
		function service.get_email(id: string): (string?, string?)
			local r = repo.find_by_id(id)
			if r.ok then
				return r.value.email, nil
			end
			return nil, r.error
		end
		return service
	`, "service", WithStdlib(), WithModule("result", resultMod), WithModule("repo", repoMod))
	if len(serviceMod.Errors) != 0 {
		t.Fatalf("service module errors = %#v, want none", serviceMod.Errors)
	}

	sig, ok := serviceMod.Manifest.FunctionSignatures["service.get_email"]
	if !ok {
		t.Fatalf("missing service.get_email function signature: %#v", serviceMod.Manifest.FunctionSignatures)
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("signature type = %v effect = %v, want ErrorReturn(0, 1)", sig.Type, sig.Effect)
	}
}

func TestCheckAndExportPublishesErrorReturnFromLocalGenericResultField(t *testing.T) {
	mod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		type User = { id: string, email: string }
		local service = {}
		function service.get_email(r: Result<User>): (string?, string?)
			if r.ok then
				return r.value.email, nil
			end
			return nil, r.error
		end
		return service
	`, "service")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	sig, ok := mod.Manifest.FunctionSignatures["service.get_email"]
	if !ok {
		t.Fatalf("missing service.get_email function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("signature type = %v effect = %v, want ErrorReturn(0, 1)", sig.Type, sig.Effect)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesReturn(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}
	sig, ok := resultMod.Manifest.FunctionSignatures["result.map"]
	if !ok {
		t.Fatalf("missing result.map function signature: %#v", resultMod.Manifest.FunctionSignatures)
	}
	if sig.Type == nil || len(sig.Type.TypeParams) != 2 {
		t.Fatalf("result.map signature = %v, want two type params", sig.Type)
	}

	checked := Check(`
		local result = require("result")
		type StringResult = { ok: true, value: string } | { ok: false, error: string }
		local decoded: StringResult = result.ok("name")
		local mapped = result.map(decoded, function(value: string)
			return #value
		end)
		if mapped.ok then
			local n: number = mapped.value
		end
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported generic return instantiated", checked.Diagnostics)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesImportedCallbackReturn(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string, retries: number }
		local M = {}
		M.User = User
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local result = require("result")
		type UserResult = { ok: true, value: protocol.User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local mapped = result.map(decoded, function(user: protocol.User)
			return user.id .. ":" .. tostring(user.retries)
		end)
		if mapped.ok then
			local text: string = mapped.value
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported callback return instantiated", checked.Diagnostics)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureSeedsUnannotatedCallbackParam(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	src := `
		local result = require("result")
		type User = { id: string, retries: number }
		type UserResult = { ok: true, value: User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local mapped = result.map(decoded, function(user)
			return user.id
		end)
		if mapped.ok then
			local id: string = mapped.value
			local wrong_id: number = mapped.value
		end
	`
	checked := Check(src, WithStdlib(), WithModule("result", resultMod))
	requireDiagnostic(t, checked, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            11,
		Column:          29,
		Span:            diagnostic.Span{StartLine: 11, StartCol: 29, EndLine: 11, EndCol: 40},
		MessageContains: []string{"mapped.value", `"u1"`, "not number"},
		EvidenceMin:     3,
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"mapped.value", `"u1"`},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"wrong_id", "number"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no proof", "mapped.value", "declared type"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"declared type", "assigned value"},
		HelpContains:  []string{"Use a value compatible", "change the target type", "`mapped.value` is valid"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign mapped.value because it is \"u1\", not number",
			"test.lua:11:29",
			"↓ declared type",
			"11 |             local wrong_id: number = mapped.value",
			"↑ assigned value",
			"because:",
			"proven: mapped.value has literal value \"u1\"",
			"claimed: wrong_id is declared as number",
			"missing proof: no proof on this path shows mapped.value satisfies the declared type",
			"help: Use a value compatible with the expected type",
		},
		RenderNotContains: []string{
			"want number",
			"^~",
		},
	})
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesNestedCallbackResult(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string, retries: number }
		type Audit = { user_id: string, event: string }
		local M = {}
		M.User = User
		M.Audit = Audit
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.and_then<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
			if result.ok then
				return fn(result.value)
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local result = require("result")
		type UserResult = { ok: true, value: protocol.User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local audit = result.and_then(decoded, function(user: protocol.User)
			return result.ok({ user_id = user.id, event = "created" })
		end)
		if audit.ok then
			local event: string = audit.value.event
			local wrong_event: number = audit.value.event
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_event diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureRejectsAnnotatedCallResult(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	src := `
		local result = require("result")
		type StringResult = { ok: true, value: string } | { ok: false, error: string }
		type NumberResult = { ok: true, value: number } | { ok: false, error: string }
		local decoded: StringResult = result.ok("u1")
		local wrong_result: NumberResult = result.map(decoded, function(value: string)
			return value .. ":mapped"
		end)
	`
	checked := Check(src, WithStdlib(), WithModule("result", resultMod))
	requireDiagnostic(t, checked, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            6,
		Column:          38,
		Span:            diagnostic.Span{StartLine: 6, StartCol: 38, EndLine: 8, EndCol: 5},
		MessageContains: []string{
			"result.map(...)",
			"{ok: true, value: string}",
			"{ok: true, value: number}",
		},
		EvidenceMin: 2,
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"result.map(...)", "{ok: true, value: string}"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"wrong_result", "NumberResult"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"declared type", "assigned value"},
		HelpContains:  []string{"Use a value compatible", "change the target type", "`result.map(...)` is valid"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign result.map(...) because it is {ok: true, value: string} | {error: string, ok: false}, not {ok: true, value: number} | {error: string, ok: false}",
			"test.lua:6:38",
			"↓ declared type",
			"6 |         local wrong_result: NumberResult = result.map(decoded, function(value: string)",
			"↑ assigned value",
			"because:",
			"proven: result.map(...) has type {ok: true, value: string} | {error: string, ok: false}",
			"claimed: wrong_result is declared as NumberResult",
			"help: Use a value compatible with the expected type",
		},
		RenderNotContains: []string{
			"want number",
			"^~",
		},
	})
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesObjectLiteralArgument(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local result = require("result")
		local wrapped = result.ok({ user_id = "u1", event = "created" })
		if wrapped.ok then
			local event: string = wrapped.value.event
			local wrong_event: number = wrapped.value.event
		end
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_event diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}
