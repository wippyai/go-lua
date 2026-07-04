package pass_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestAssignmentsRefutesConcreteMismatch(t *testing.T) {
	result := testutil.CheckFile(`local n: number = "bad"`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	item := got[0]
	if item.Code != judgment.CodeAssignment {
		t.Fatalf("code = %q, want %q", item.Code, judgment.CodeAssignment)
	}
	if item.Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", item.Verdict)
	}
	if item.Subject.Kind != judgment.SubjectPath || item.Subject.Label != "n" {
		t.Fatalf("subject = %#v, want path subject labelled n", item.Subject)
	}
	if stable := item.Subject.StableKey(); !strings.HasPrefix(stable, "fixture:assignment|path|assignment:") || !strings.Contains(stable, ":sym:") {
		t.Fatalf("stable key = %q", item.Subject.StableKey())
	}
	if len(item.Spans) != 1 || item.Spans[0].File != "test.lua" || item.Spans[0].StartLine != 1 {
		t.Fatalf("spans = %#v, want source span on test.lua line 1", item.Spans)
	}
}

func TestReturnsRefutesConcreteMismatch(t *testing.T) {
	stmts, err := parse.ParseString(strings.TrimSpace(`local function f(): number
    return "bad"
end`), "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: standard.Registry()}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var got []judgment.Judgment
	got = append(got, obligationpass.New(obligationpass.Returns{}).Run(obligationpass.Context{
		FunctionKey: "fixture:return",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(root.FunctionResults()[0]),
	})...)
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	item := got[0]
	if item.Code != judgment.CodeReturn {
		t.Fatalf("code = %q, want %q", item.Code, judgment.CodeReturn)
	}
	if item.Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", item.Verdict)
	}
	if item.Subject.Kind != judgment.SubjectReturnValue || item.Subject.Label != "returned value 1" {
		t.Fatalf("subject = %#v, want return-value subject labelled returned value 1", item.Subject)
	}
	if item.Actual.ProjectedType == nil || item.Expected.Type == nil {
		t.Fatalf("actual/expected = %#v/%#v, want concrete types", item.Actual, item.Expected)
	}
	if len(item.Spans) != 1 || item.Spans[0].File != "test.lua" || item.Spans[0].StartLine != 2 {
		t.Fatalf("spans = %#v, want return expression span on test.lua line 2", item.Spans)
	}
}

func TestReturnsCarriesMissingRequiredFieldEvidenceDetail(t *testing.T) {
	stmts, err := parse.ParseString(strings.TrimSpace(`type Holder = { required: string, optional: number? }
local function f(): Holder
    local self = { optional = 1 }
    return self
end`), "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: standard.Registry()}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	got := obligationpass.New(obligationpass.Returns{}).Run(obligationpass.Context{
		FunctionKey: "fixture:return",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(root.FunctionResults()[0]),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	field, ok := assignmentMissingRequiredFieldDetail(got[0])
	if !ok || field != "required" {
		t.Fatalf("evidence = %#v, want missing required field required", got[0].Evidence)
	}
}

func TestReturnsSkipsPlainUnknownWithoutBoundary(t *testing.T) {
	stmts, err := parse.ParseString(strings.TrimSpace(`local function f(x): string
    return x.id
end`), "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: standard.Registry()}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	got := obligationpass.New(obligationpass.Returns{}).Run(obligationpass.Context{
		FunctionKey: "fixture:return",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(root.FunctionResults()[0]),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want none for plain unknown without boundary", got)
	}
}

func TestAssignmentsSkipsProvenAssignment(t *testing.T) {
	result := testutil.CheckFile(`local n: number = 1`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want none", got)
	}
}

func TestAssignmentsReportsUnknownAnyAsMissingProof(t *testing.T) {
	result := testutil.CheckFile(`local raw: any = 1
local s: string = raw`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictUnknown {
		t.Fatalf("verdict = %v, want unknown for any boundary", got[0].Verdict)
	}
	if got[0].Subject.Label != "s" {
		t.Fatalf("subject label = %q, want target name s", got[0].Subject.Label)
	}
	if got[0].Actual.Label != "raw" {
		t.Fatalf("actual label = %q, want source name raw", got[0].Actual.Label)
	}
	if got[0].Actual.ProjectedType != typ.Any {
		t.Fatalf("actual projected type = %v, want any for untrusted missing proof", got[0].Actual.ProjectedType)
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("evidence = %#v, want precision-boundary evidence", got[0].Evidence)
	}
}

func TestAssignmentsSkipUnprojectedLocalAssignmentSource(t *testing.T) {
	result := testutil.CheckFile(`local function outer(a: number): (number) -> (number) -> number
	return function(b: number): (number) -> number
		return function(c: number): number
			return a + b + c
		end
	end
end
local result: number = outer(1)(2)(3)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want none from synthetic unknown fallback", got)
	}
}

func TestAssignmentsUseBoundaryProofForGuardedVariantField(t *testing.T) {
	checked := testutil.CheckFile(`type TemplatePage = {
    kind: "template",
    id: string,
    data_func: string?,
    template_set: string,
}
type ComponentPage = {
    kind: "component",
    id: string,
    url: string,
}
type Page = TemplatePage | ComponentPage

local function get_page_data(page: Page?)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end
    local name: string = page.data_func
    return {name}, nil
end
`, "test.lua")
	if checked.RootResult() == nil {
		t.Fatal("RootResult nil")
	}
	for _, result := range checked.BodyResults() {
		got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:guarded-variant",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})
		if len(got) != 0 {
			t.Fatalf("function %p judgments = %#v, want guarded variant field assignment accepted", result.Function(), got)
		}
	}
}

func TestAssignmentsUseContextualBoundaryProofForGuardedInferredParamField(t *testing.T) {
	checked := testutil.CheckFile(`type Page = {
    data_func: string?,
}

local function load_page(): (Page?, string?)
    return { data_func = "demo" }, nil
end

local function get_page_data(page)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end
    local name: string = page.data_func
    return {name}, nil
end

local page, err = load_page()
if err then
    return nil, err
end

return get_page_data(page)
`, "test.lua")
	if checked.RootResult() == nil {
		t.Fatal("RootResult nil")
	}
	for _, result := range checked.BodyResults() {
		got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:guarded-inferred-param",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})
		if len(got) != 0 {
			t.Fatalf("function %p judgments = %#v, want contextual guarded field assignment accepted", result.Function(), got)
		}
	}
}

func TestAssignmentsUseExportedTypeIsProofAcrossRequire(t *testing.T) {
	errorsMod := testutil.CheckAndExport(`
type AppError = {
    code: string,
    message: string,
}

local M = {}
M.AppError = AppError
return M
`, "errors")
	if len(errorsMod.Errors) != 0 {
		t.Fatalf("errors diagnostics = %#v", errorsMod.Errors)
	}
	checked := testutil.Check(`
local errors = require("errors")

local raw = { code = "TEST", message = "hello" }
local validated, err = errors.AppError:is(raw)
if err == nil and validated then
    local code: string = validated.code
end
`, testutil.WithModule("errors", errorsMod))
	if checked.RootResult() == nil {
		t.Fatal("RootResult nil")
	}
	for _, result := range checked.BodyResults() {
		got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:exported-type-is",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})
		if len(got) != 0 {
			t.Fatalf("function %p judgments = %#v, want exported Type:is proof accepted", result.Function(), got)
		}
	}
}

func TestAssignmentsReportsObjectLiteralEntryMismatch(t *testing.T) {
	result := testutil.CheckFile(`local arr: {string} = {1, 2}`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Subject.Label != "arr[1]" {
		t.Fatalf("subject label = %q, want arr[1]", got[0].Subject.Label)
	}
	if got[0].Actual.Label != "arr[1]" {
		t.Fatalf("actual label = %q, want object entry source label arr[1]", got[0].Actual.Label)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
}

func TestAssignmentsReportsRecursiveObjectLiteralEntryMismatch(t *testing.T) {
	result := testutil.CheckFile(`
type Tree = { root: TreeNode? }
type TreeNode = { label: string, owner: Tree, children: {TreeNode}, parent: TreeNode? }

local tree: Tree = {root = nil}
local node: TreeNode = {label = 123, owner = tree, children = {}, parent = nil}
`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Subject.Label != "node.label" {
		t.Fatalf("subject label = %q, want node.label", got[0].Subject.Label)
	}
	if got[0].Actual.Label != "node.label" {
		t.Fatalf("actual label = %q, want node.label", got[0].Actual.Label)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
}

func TestAssignmentsCarriesExplicitAnyAssertionEvidence(t *testing.T) {
	result := testutil.CheckFile(`local x: number = "no" as any`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if !userAssertedAnyDetail(got[0], "assigned value") {
		t.Fatalf("evidence = %#v, want explicit-any assertion detail", got[0].Evidence)
	}
}

func userAssertedAnyDetail(item judgment.Judgment, label string) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailUserAssertedAny &&
			evidence.Detail.SubjectLabel == label {
			return true
		}
	}
	return false
}

func TestAssignmentsCarriesMissingRequiredFieldEvidenceDetail(t *testing.T) {
	result := testutil.CheckFile(`type Point = { x: number, y: number }
local p: Point = { x = 1 }`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	field, ok := assignmentMissingRequiredFieldDetail(got[0])
	if !ok || field != "y" {
		t.Fatalf("evidence = %#v, want missing required field y", got[0].Evidence)
	}
	if got[0].Expected.Label != "Point" {
		t.Fatalf("expected label = %q, want declared alias Point", got[0].Expected.Label)
	}
}

func TestAssignmentsReportsIndexedArrayReadAsOptional(t *testing.T) {
	checked := testutil.CheckFile(`local function first_department(departments: {string}): ()
	local first: string = departments[1]
end`, "test.lua")

	var got []judgment.Judgment
	for _, result := range checked.BodyResults() {
		got = append(got, obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:assignment",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Subject.Label != "first" || got[0].Actual.Label != "departments[1]" {
		t.Fatalf("judgment labels = subject %q actual %q, want first/departments[1]", got[0].Subject.Label, got[0].Actual.Label)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted optional index mismatch", got[0].Verdict)
	}
	if !hasMissingProofDetail(got[0], judgment.EvidenceDetailIndexedReadMissingProof) {
		t.Fatalf("evidence = %#v, want indexed-read missing-proof detail", got[0].Evidence)
	}
}

func TestAssignmentsReportsGuardedOptionalArrayFieldElementRead(t *testing.T) {
	checked := testutil.CheckFile(`type Response = {
	result: {
		data: {
			departments: {string}?,
		},
	},
}
local function f(response: Response): ()
	if response.result.data.departments ~= nil then
	local count: number = #response.result.data.departments
	local first: string = response.result.data.departments[1]
	end
end`, "test.lua")

	var got []judgment.Judgment
	for _, result := range checked.BodyResults() {
		got = append(got, obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:assignment",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want one optional element assignment: %#v", len(got), got)
	}
	if got[0].Subject.Label != "first" || got[0].Actual.Label != "response.result.data.departments[1]" {
		t.Fatalf("judgment labels = subject %q actual %q, want first/response...departments[1]", got[0].Subject.Label, got[0].Actual.Label)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted optional element mismatch", got[0].Verdict)
	}
	if !hasMissingProofDetail(got[0], judgment.EvidenceDetailIndexedReadMissingProof) {
		t.Fatalf("evidence = %#v, want indexed-read missing-proof detail", got[0].Evidence)
	}
}

func TestAssignmentsPreservesRuntimeKindAtUntrustedBoundary(t *testing.T) {
	checked := testutil.CheckFile(`local function rows(block: any): ()
	if type(block.items) == "table" then
		local labels: {string} = block.items
	end
end`, "test.lua")

	var got []judgment.Judgment
	for _, result := range checked.BodyResults() {
		got = append(got, obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:assignment",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Actual.ProjectedType == typ.Any {
		t.Fatalf("actual type = any, want preserved table runtime-kind witness")
	}
	if !strings.Contains(got[0].Actual.ProjectedType.String(), "table") {
		t.Fatalf("actual type = %v, want table-like witness", got[0].Actual.ProjectedType)
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("evidence = %#v, want precision-boundary evidence", got[0].Evidence)
	}
}

func TestAssignmentsDisplaysAnyForUntrustedSameShapedBoundary(t *testing.T) {
	checked := testutil.CheckFile(`type Point = {x: number, y: number}
local function validate(data: any): ()
	local _, err = Point:is(data)
	if err == nil then
		local p: Point = data
	end
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Actual.ProjectedType != typ.Any {
		t.Fatalf("actual type = %v, want any for untrusted same-shaped boundary", got[0].Actual.ProjectedType)
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("evidence = %#v, want precision-boundary evidence", got[0].Evidence)
	}
}

func TestAssignmentsReportsFailedTypeIsResultAsUntrusted(t *testing.T) {
	checked := testutil.CheckFile(`type Point = {x: number, y: number}
local function isPoint(x)
	return Point:is(x)
end
function validate(data: any): ()
	local val, err = isPoint(data)
	if err ~= nil then
		local p: Point = val
	end
	end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1 failed type-is assignment: %#v", len(got), got)
	}
	if got[0].Subject.Label != "p" || got[0].Actual.Label != "val" {
		t.Fatalf("labels = subject %q actual %q, want p/val", got[0].Subject.Label, got[0].Actual.Label)
	}
	if got[0].Actual.ProjectedType != typ.Nil {
		t.Fatalf("actual type = %v, want nil for failed type-is result", got[0].Actual.ProjectedType)
	}
}

func TestAssignmentsReportsOptionalCallResultMemberChain(t *testing.T) {
	checked := testutil.CheckFile(`type Record = {last_status: "queued" | "sent" | "retrying"}
function lookup_record(id: string): Record?
	return nil
end
local missing_status: string = lookup_record("msg-1").last_status`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1 optional call-result member assignment: %#v", len(got), got)
	}
	if got[0].Subject.Label != "missing_status" || got[0].Actual.Label != "lookup_record(...).last_status" {
		t.Fatalf("labels = subject %q actual %q, want missing_status/lookup_record(...).last_status", got[0].Subject.Label, got[0].Actual.Label)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted optional call-result member", got[0].Verdict)
	}
	if !hasMayBeNilAccessEvidence(got[0], "lookup_record(...)", ".last_status") {
		t.Fatalf("evidence = %#v, want call-result receiver nilability before field read", got[0].Evidence)
	}
}

func TestAssignmentsAcceptsRecordOfSharedTypedFunctions(t *testing.T) {
	checked := testutil.CheckFile(`type Response = {
	status_code: number,
	body: string?,
	headers: {[string]: string}?,
	stream: {
		read: (self: any) -> (any, any),
	}?,
}
type Client = {
	get: (url: string, options: any?) -> (Response?, string?),
	post: (url: string, options: any?) -> (Response?, string?),
	put: (url: string, options: any?) -> (Response?, string?),
	patch: (url: string, options: any?) -> (Response?, string?),
}
local function ok(url: string, options: any?): (Response?, string?)
	return {status_code = 200, body = "ok"}
end
local client: Client = {
	get = ok,
	post = ok,
	put = ok,
	patch = ok,
}
local holder: {_http_client: Client} = {_http_client = client}
holder._http_client = {
	get = ok,
	post = ok,
	put = ok,
	patch = ok,
}`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want record of shared typed functions accepted", got)
	}
}

func TestAssignmentsReportsOrdinaryMapWriteFromDeclaredAny(t *testing.T) {
	checked := testutil.CheckFile(`local unknown_id: any = nil
local pages = {
	{ id = unknown_id, mount_route = "/ok" },
}
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	local mr = page.mount_route
	accessible[mr] = page.id
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Subject.Label != "accessible[mr]" || got[0].Actual.Label != "page.id" {
		t.Fatalf("judgment labels = subject %q actual %q, want accessible[mr]/page.id", got[0].Subject.Label, got[0].Actual.Label)
	}
	if got[0].Actual.ProjectedType != typ.Any {
		t.Fatalf("actual type = %v, want declared any", got[0].Actual.ProjectedType)
	}
}

func TestAssignmentsAcceptsUnionValueAfterChannelEqualityNarrowing(t *testing.T) {
	checked := testutil.CheckFile(`type Message = {_topic: string}
type Event = {kind: string}
type Timer = {elapsed: number}
type MsgCh = {__tag: "msg"}
type EventCh = {__tag: "event"}
type TimerCh = {__tag: "timer"}
type Result = {channel: MsgCh, value: Message, ok: boolean} |
	{channel: EventCh, value: Event, ok: boolean} |
	{channel: TimerCh, value: Timer, ok: boolean}
function do_select(m: MsgCh, e: EventCh, t: TimerCh): Result
	return {channel = m, value = {_topic = "test"}, ok = true}
end
function f(msg_ch: MsgCh, events_ch: EventCh, timeout: TimerCh): ()
	local result = do_select(msg_ch, events_ch, timeout)
	if result.channel == timeout then
		return
	end
	if result.channel == events_ch then
		local event = result.value
		local k: string = event.kind
	end
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want none after channel equality narrowing", got)
	}
}

func TestAssignmentsUseDeclaredContractForOrdinaryFieldWrites(t *testing.T) {
	checked := testutil.CheckFile(`local policy: {max_attempts: integer, state: string} = {
	max_attempts = 0,
	state = "",
}
policy.max_attempts = 1
policy.state = "ready"`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want declared field writes accepted", got)
	}
}

func TestAssignmentsUseElementTypeForOrdinaryMapWrites(t *testing.T) {
	checked := testutil.CheckFile(`type Projection = {
	id: string,
	output: string?,
	updated_at: number?,
}
type State = {
	projections: {[string]: Projection},
}
local state: State = {
	projections = {},
}
local projection = {
	id = "queued-1",
	output = nil,
	updated_at = 1,
}
state.projections["queued-1"] = projection`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want map write checked against non-optional element type", got)
	}
}

func TestAssignmentsAcceptsObjectWithNilFieldsForOptionalRecordMapWrite(t *testing.T) {
	checked := testutil.CheckFile(`type Projection = {
	id: string,
	status: "queued" | "started" | "completed" | "failed",
	output: string?,
	error_code: string?,
	retryable: boolean?,
	updated_at: number?,
}
type State = {
	projections: {[string]: Projection},
}
local state: State = {
	projections = {},
}
local projection = {
	id = "queued-1",
	status = "queued",
	output = nil,
	error_code = nil,
	retryable = nil,
	updated_at = 1,
}
state.projections["queued-1"] = projection`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want object literal nil fields accepted by optional map element fields", got)
	}
}

func TestAssignmentsAcceptsPresentIntegerMapRead(t *testing.T) {
	checked := testutil.CheckFile(`local counters: {[string]: integer} = {}
local sent_counter = counters["sent"]
if sent_counter then
	local sent_value: integer = sent_counter
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want guarded integer map read accepted", got)
	}
}

func TestAssignmentsAcceptsMutableLiteralFieldWidening(t *testing.T) {
	checked := testutil.CheckFile(`local obj = {
	value = 0,
}
obj.value = obj.value + 1

local item = {route = ""}
item.route = "primary"

local registry = {active = nil}
registry.active = {name = "suite"}
registry.active = nil`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want mutable literal field writes and inferred nil clearing accepted", got)
	}
}

func TestAssignmentsUseVariantFamilyForAliasWriteTarget(t *testing.T) {
	checked := testutil.CheckFile(`type FileSlot = {
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
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("judgments = %#v, want only stale read diagnostic", got)
	}
	if got[0].Subject.Label != "stale_path" {
		t.Fatalf("subject = %q, want stale_path", got[0].Subject.Label)
	}
	if got[0].Actual.ProjectedType == nil ||
		!strings.Contains(got[0].Actual.ProjectedType.String(), "string") ||
		!typevalue.TypeIncludesNil(got[0].Actual.ProjectedType) {
		t.Fatalf("actual type = %v, want precise string|nil for invalidated stale alias read", got[0].Actual.ProjectedType)
	}
}

func TestAssignmentsReportOptionalAssignmentTarget(t *testing.T) {
	checked := testutil.CheckFile(`type Bag = {name: string}
function update(bag: Bag?): ()
	bag.name = "ok"
end

type Slot = {value: string}
type Slots = {[string]: Slot}
function write(slots: Slots?): ()
	slots["active"].value = "ready"
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	var optional []judgment.Judgment
	for _, item := range got {
		if item.Code == judgment.CodeAssignmentTarget {
			optional = append(optional, item)
		}
	}
	if len(optional) != 2 {
		t.Fatalf("optional target judgments = %d, want 2: %#v", len(optional), got)
	}
	if optional[0].Subject.Label != "bag.name" || optional[0].Actual.Label != "bag" {
		t.Fatalf("first labels = subject %q actual %q, want bag.name/bag", optional[0].Subject.Label, optional[0].Actual.Label)
	}
	if optional[1].Subject.Label != `slots["active"].value` || optional[1].Actual.Label != "slots" {
		t.Fatalf("second labels = subject %q actual %q, want slots[...].value/slots", optional[1].Subject.Label, optional[1].Actual.Label)
	}
}

func TestAssignmentsReportFunctionBodyClosedRecordDynamicWrite(t *testing.T) {
	checked := testutil.CheckFile(`type Key = "name" | "count"
type Bag = {name: string, count: number}

local function f(bag: Bag, key: Key): ()
	bag[key] = "bad"
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	var readAssignments []readmodel.Assignment
	for _, result := range checked.BodyResults() {
		readmodel.New(result).ForEachAssignment(func(assignment readmodel.Assignment) bool {
			readAssignments = append(readAssignments, assignment)
			return true
		})
	}
	var assignments []judgment.Judgment
	for _, item := range got {
		if item.Code == judgment.CodeAssignment && item.Subject.Label == "bag[key]" {
			assignments = append(assignments, item)
		}
	}
	if len(assignments) != 1 {
		t.Fatalf("bag[key] assignment judgments = %d, want 1 across %d bodies; read assignments=%#v judgments=%#v", len(assignments), len(checked.BodyResults()), readAssignments, got)
	}
	if assignments[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", assignments[0].Verdict)
	}
	if !strings.Contains(assignments[0].Expected.Type.String(), "number") {
		t.Fatalf("expected = %v, want contract including number slot", assignments[0].Expected.Type)
	}
	if !dynamicAssignmentTargetDetail(assignments[0], "bag[key]") {
		t.Fatalf("evidence = %#v, want dynamic assignment target detail", assignments[0].Evidence)
	}
}

func TestAssignmentsSuppressesCascadeFromPreviouslyRefutedSourcePath(t *testing.T) {
	checked := testutil.CheckFile(`
local M = {}
function M.f(): string
	return "ok"
end

M.f = 42
local g: () -> string = M.f
`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	var assignments []judgment.Judgment
	for _, item := range got {
		if item.Code == judgment.CodeAssignment {
			assignments = append(assignments, item)
		}
	}
	if len(assignments) != 1 {
		t.Fatalf("assignment judgments = %d, want only root M.f write: %#v", len(assignments), assignments)
	}
	if assignments[0].Subject.Label != "M.f" || assignments[0].Actual.Label == "M.f" {
		t.Fatalf("judgment labels = subject %q actual %q, want M.f write cause", assignments[0].Subject.Label, assignments[0].Actual.Label)
	}
}

func TestAssignmentsSuppressesCascadeFromPreviouslyRefutedDynamicTargetRoot(t *testing.T) {
	checked := testutil.CheckFile(`type Key = "name" | "count"
type Bag = {name: string, count: number}

local function f(bag: Bag, key: Key): ()
	bag[key] = "bad"
	local count: number = bag.count
end`, "test.lua")

	got := assignmentJudgmentsForAllBodies(checked)
	var assignments []judgment.Judgment
	for _, item := range got {
		if item.Code == judgment.CodeAssignment {
			assignments = append(assignments, item)
		}
	}
	if len(assignments) != 1 {
		t.Fatalf("assignment judgments = %d, want only dynamic write cause: %#v", len(assignments), assignments)
	}
	if assignments[0].Subject.Label != "bag[key]" || assignments[0].Actual.Label == "bag.count" {
		t.Fatalf("judgment labels = subject %q actual %q, want bag[key] write cause", assignments[0].Subject.Label, assignments[0].Actual.Label)
	}
	if !dynamicAssignmentTargetDetail(assignments[0], "bag[key]") {
		t.Fatalf("evidence = %#v, want dynamic assignment target detail", assignments[0].Evidence)
	}
}

func TestAssignmentsSuppressesCascadeFromLoopDynamicTargetRootBySourceOrder(t *testing.T) {
	stmts, err := parse.ParseString(`
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = tostring(value)
end

local count: number = item.count
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:   standard.Registry(),
			Globals:    []string{"pairs", "tostring"},
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}

	root := checked.RootResult()
	if root == nil {
		t.Fatal("missing root result")
	}
	got := obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:assignment",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(root),
	})
	var assignments []judgment.Judgment
	for _, item := range got {
		if item.Code == judgment.CodeAssignment {
			assignments = append(assignments, item)
		}
	}
	if len(assignments) != 1 {
		t.Fatalf("assignment judgments = %d, want only loop dynamic write cause: %#v", len(assignments), assignments)
	}
	if assignments[0].Subject.Label != "item[key]" || assignments[0].Actual.Label != "tostring(...)" {
		t.Fatalf("judgment labels = subject %q actual %q, want item[key]/tostring(...)", assignments[0].Subject.Label, assignments[0].Actual.Label)
	}
	if !dynamicAssignmentTargetDetail(assignments[0], "item[key]") {
		t.Fatalf("evidence = %#v, want dynamic assignment target detail", assignments[0].Evidence)
	}
}

func assignmentJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.Assignments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:assignment",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}

func assignmentMissingRequiredFieldDetail(item judgment.Judgment) (string, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailMissingRequiredField {
			return evidence.Detail.Field, true
		}
	}
	return "", false
}

func dynamicAssignmentTargetDetail(item judgment.Judgment, label string) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailDynamicAssignmentTarget &&
			evidence.Detail.SubjectLabel == label {
			return true
		}
	}
	return false
}

func hasMissingProofDetail(item judgment.Judgment, kind judgment.EvidenceDetailKind) bool {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof && evidence.Detail.Kind == kind {
			return true
		}
	}
	return false
}

func hasMayBeNilAccessEvidence(item judgment.Judgment, label, access string) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailMayBeNil &&
			evidence.Detail.SubjectLabel == label &&
			evidence.Detail.Field == access {
			return true
		}
	}
	return false
}
