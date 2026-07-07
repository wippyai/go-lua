package pass_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCallArgumentsRefutesConcreteMismatch(t *testing.T) {
	result := checkFunction(t, `function f() need_string(1) end`, stringSignatureManifest())

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeCallArgType {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeCallArgType)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Subject.StableKey() != "fixture:f|call_arg|call:2:arg:0" {
		t.Fatalf("stable key = %q", got[0].Subject.StableKey())
	}
}

func TestCallArgumentsSkipsProvenArgument(t *testing.T) {
	result := checkFunction(t, `function f() need_string("ok") end`, stringSignatureManifest())

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want none", got)
	}
}

func TestCallArgumentsUsesBoundaryProofForGuardedVariantField(t *testing.T) {
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

local function takes_string(name: string): string
    return name
end

local function get_page_data(page: Page?)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end
    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end
`, "test.lua")
	if checked.RootResult() == nil {
		t.Fatal("RootResult nil")
	}
	for _, result := range checked.BodyResults() {
		got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:guarded-variant",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})
		if len(got) != 0 {
			t.Fatalf("function %p judgments = %#v, want guarded variant field call accepted", result.Function(), got)
		}
	}
}

func TestCallArgumentsUsesLocalFunctionContract(t *testing.T) {
	result := testutil.CheckFile(`local function add(a: number, b: number): number
    return a + b
end
add(1, "wrong")`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Spans[0].File != "test.lua" || got[0].Spans[0].StartLine != 4 {
		t.Fatalf("span = %#v, want test.lua line 4", got[0].Spans[0])
	}
}

func TestCallArgumentsRejectsUntrustedOptionalAnyForLocalFunctionContract(t *testing.T) {
	checked := testutil.CheckFile(`local function need(id: string): () end

local function f(raw: any?): ()
    need(raw)
end`, "test.lua")
	var got []judgment.Judgment
	for _, fn := range checked.BodyResults() {
		got = append(got, obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
			FunctionKey: "fixture:f",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(fn),
		})...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictUnknown || !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("judgment = %#v, want unknown precision-boundary local function argument", got[0])
	}
}

func TestCallArgumentsLabelsNamedArgument(t *testing.T) {
	result := testutil.CheckFile(`local function need_string(value: string): () end
local raw: any = 1
need_string(raw)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:label",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Subject.Label != "argument 1 (raw)" {
		t.Fatalf("subject label = %q, want argument name", got[0].Subject.Label)
	}
	if got[0].Subject.StableKey() != "fixture:label|call_arg|call:4:arg:0" {
		t.Fatalf("stable key = %q", got[0].Subject.StableKey())
	}
}

func TestCallArgumentsAcceptsOptionalParameterNil(t *testing.T) {
	result := testutil.CheckFile(`local function maybe(value: string?): () end
local value: string? = nil
maybe(value)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:optional-param",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want optional parameter accepted", got)
	}
}

func TestCallArgumentsInstantiatesGenericLocalContract(t *testing.T) {
	result := testutil.CheckFile(`type Result<T, E> = { ok: true, value: T } | { ok: false, error: E }
local function map<T, U, E>(r: Result<T, E>, f: (T) -> U): Result<U, E>
    if r.ok then
        return { ok = true, value = f(r.value) }
    end
    return { ok = false, error = r.error }
end
local r: Result<number, string> = { ok = true, value = 2 }
local doubled = map(r, function(x: number): number return x * 2 end)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:generic",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want no generic instantiation argument mismatch", got)
	}
}

func TestCallArgumentsReportsGenericConstraintViolation(t *testing.T) {
	result := testutil.CheckFile(`type HasName = { name: string }
local function wrap<T: HasName>(x: T): T
    return x
end
local n: number = wrap(42)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:generic-constraint",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Spans[0].StartLine != 5 {
		t.Fatalf("span = %#v, want argument on line 5", got[0].Spans[0])
	}
}

func TestCallArgumentsCarriesMissingRequiredFieldEvidenceDetail(t *testing.T) {
	result := testutil.CheckFile(`type HasId = { id: string }
local function need_id<T: HasId>(x: T): string
    return x.id
end
need_id({ name = "no-id-here" })`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:missing-field",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if field, ok := missingRequiredFieldEvidenceDetail(got[0]); !ok || field != "id" {
		t.Fatalf("evidence = %#v, want missing required field id", got[0].Evidence)
	}
	if got[0].Expected.Label != "need_id parameter 1.id" {
		t.Fatalf("expected label = %q, want refined missing-field parameter label", got[0].Expected.Label)
	}
}

func TestCallArgumentsCarriesMayBeNilEvidenceDetail(t *testing.T) {
	result := testutil.CheckFile(`local function need_string(value: string): () end
local response: { body: string? } = { body = nil }
need_string(response.body)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:may-be-nil",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if !mayBeNilEvidenceDetail(got[0]) {
		t.Fatalf("evidence = %#v, want may-be-nil missing-proof detail", got[0].Evidence)
	}
}

func TestCallArgumentsReportsGenericInferenceConflictAtContributor(t *testing.T) {
	result := testutil.CheckFile(`type Channel<T> = { value: T }
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
})`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:generic-conflict",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want one generic conflict: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Spans[0].StartLine != 13 || got[0].Spans[0].StartCol != 15 || got[0].Spans[0].EndCol != 28 {
		t.Fatalf("span = %#v, want source.primary contribution on line 13", got[0].Spans[0])
	}
	if len(got[0].Evidence) != 4 {
		t.Fatalf("evidence = %#v, want consistency assertion, two contributions, and conflict proof", got[0].Evidence)
	}
	if param, ok := genericConflictEvidenceDetail(got[0]); !ok || param != "T" {
		t.Fatalf("evidence = %#v, want generic conflict detail for T", got[0].Evidence)
	}
	contributionSpans := genericConflictContributionSpans(got[0])
	if len(contributionSpans) != 2 {
		t.Fatalf("contribution spans = %#v, want two contribution spans", contributionSpans)
	}
	if contributionSpans[0].StartLine != 13 || contributionSpans[0].StartCol != 15 ||
		contributionSpans[1].StartLine != 14 || contributionSpans[1].StartCol != 14 {
		t.Fatalf("contribution spans = %#v, want channel and decode contribution spans", contributionSpans)
	}
	contributionLabels := genericConflictContributionLabels(got[0])
	if len(contributionLabels) != 2 || contributionLabels[0] != "argument 2.channel (source.primary)" ||
		contributionLabels[1] != "argument 2.decode (decode_timer) return 1" {
		t.Fatalf("contribution labels = %#v, want user-facing channel and decode return labels", contributionLabels)
	}
}

func TestCallArgumentsReportsSummaryParamObligation(t *testing.T) {
	result := testutil.CheckFile(`local function scale(tokens)
    return tokens * 4
end
local m: { name: string } = { name = "model" }
scale(m)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:summary-obligation",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.Number).Key {
		t.Fatalf("expected = %q, want number", got[0].Expected.Key)
	}
	if got[0].Spans[0].StartLine != 5 || got[0].Spans[0].StartCol != 7 || got[0].Spans[0].EndCol != 8 {
		t.Fatalf("span = %#v, want m identifier on line 5", got[0].Spans[0])
	}
}

func TestCallArgumentsReportsTableInsertAccumulatorElementObligation(t *testing.T) {
	result := testutil.CheckFile(`type SystemMessage = { role: "system" }

local function build_messages(): ()
    local final_messages: {SystemMessage} = {}
    table.insert(final_messages, { role = "cache_marker" })
end`, "test.lua", testutil.WithStdlib()).RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	if len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %d, want build_messages body", len(result.FunctionResults()))
	}
	fn := result.FunctionResults()[0]
	hasOutcomeObligation := false
	for _, point := range fn.Graph().RPO() {
		outcome, ok := fn.CallOutcomeAt(point)
		if ok && len(outcome.ParamObligations) != 0 {
			hasOutcomeObligation = true
			break
		}
	}
	if !hasOutcomeObligation {
		t.Fatalf("table.insert call outcome has no param obligation")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:table-insert-accumulator",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(fn),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want inserted element mismatch: %#v", len(got), got)
	}
	if got[0].Subject.Kind != judgment.SubjectCallArgument || got[0].Subject.Label != "argument 2.role" {
		t.Fatalf("subject = %#v, want argument 2.role", got[0].Subject)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.LiteralString("system")).Key {
		t.Fatalf("expected = %q, want literal system", got[0].Expected.Key)
	}
}

func TestCallArgumentsPointsAtObjectLiteralMemberMismatch(t *testing.T) {
	result := testutil.CheckFile(`type Job = { id: string, meta: { attempt: number } }
local function send(job: Job): ()
end
send({ id = 1, meta = { attempt = 1 } })`, "main.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:object-literal",
		SourceFile:  "main.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.String).Key {
		t.Fatalf("expected = %q, want nested id string", got[0].Expected.Key)
	}
	if got[0].Expected.Label != "send parameter 1.id" {
		t.Fatalf("expected label = %q, want refined member parameter label", got[0].Expected.Label)
	}
	if got[0].Spans[0].StartLine != 4 || got[0].Spans[0].StartCol != 13 || got[0].Spans[0].EndCol != 13 {
		t.Fatalf("span = %#v, want id value literal on line 4", got[0].Spans[0])
	}
}

func TestCallArgumentsReportsForwardedHelperParamObligation(t *testing.T) {
	clientMod := testutil.CheckAndExport(`local client = {}
function client.invoke(model_id: string, payload: any, options: any): any
    return {}
end
return client`, "bedrock_client")
	if len(clientMod.Errors) != 0 {
		t.Fatalf("client module errors = %#v, want none", clientMod.Errors)
	}

	result := testutil.CheckFile(`local bedrock_client = require("bedrock_client")
local function helper(client, model_id)
    return client.invoke(model_id, {}, {})
end
local contract_args = nil :: any
local model_id = contract_args.model
helper(bedrock_client, model_id)`, "main.lua", testutil.WithStdlib(), testutil.WithModule("bedrock_client", clientMod))

	var got []judgment.Judgment
	p := obligationpass.New(obligationpass.CallArguments{})
	for _, bodyResult := range result.BodyResults() {
		got = append(got, p.Run(contextForBody("fixture:helper", "main.lua", bodyResult))...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.String).Key {
		t.Fatalf("expected = %q, want string", got[0].Expected.Key)
	}
	if got[0].Spans[0].StartLine != 7 || got[0].Spans[0].StartCol != 24 {
		t.Fatalf("span = %#v, want model_id argument on line 7", got[0].Spans[0])
	}
}

func TestCallArgumentsUsesImportedModuleMemberContract(t *testing.T) {
	jsonMod := testutil.CheckAndExport(`local json = {}
function json.decode(src: string): any
    return {}
end
return json`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	result := testutil.CheckFile(`local json = require("json")
local body: string? = nil
json.decode(body)`, "client.lua", testutil.WithStdlib(), testutil.WithModule("json", jsonMod))

	var got []judgment.Judgment
	p := obligationpass.New(obligationpass.CallArguments{})
	for _, bodyResult := range result.BodyResults() {
		got = append(got, p.Run(contextForBody("fixture:client", "client.lua", bodyResult))...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.String).Key {
		t.Fatalf("expected = %q, want string", got[0].Expected.Key)
	}
	if got[0].Spans[0].StartLine != 3 {
		t.Fatalf("span = %#v, want body argument on line 3", got[0].Spans[0])
	}
}

func TestCallArgumentsUsesImportedOptionalFieldFromModuleReturn(t *testing.T) {
	jsonMod := testutil.CheckAndExport(`local json = {}
function json.decode(src: string): any
    return {}
end
return json`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}
	httpMod := testutil.CheckAndExport(`local http = {}
type Response = {
    body: string?,
}
function http.get(): Response?
    return nil
end
return http`, "http")
	if len(httpMod.Errors) != 0 {
		t.Fatalf("http module errors = %#v, want none", httpMod.Errors)
	}

	result := testutil.CheckFile(`local json = require("json")
local http = require("http")
local response = http.get()
if not response then
    return nil
end
json.decode(response.body)`, "client.lua", testutil.WithStdlib(), testutil.WithModule("json", jsonMod), testutil.WithModule("http", httpMod))

	var got []judgment.Judgment
	p := obligationpass.New(obligationpass.CallArguments{})
	for _, bodyResult := range result.BodyResults() {
		got = append(got, p.Run(contextForBody("fixture:client", "client.lua", bodyResult))...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.String).Key {
		t.Fatalf("expected = %q, want string", got[0].Expected.Key)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted optional-field mismatch", got[0].Verdict)
	}
	if got[0].Spans[0].StartLine != 7 {
		t.Fatalf("span = %#v, want response.body argument on line 7", got[0].Spans[0])
	}
}

func TestCallArgumentsUsesJoinedImportedOptionalFieldFromModuleReturn(t *testing.T) {
	jsonMod := testutil.CheckAndExport(`local json = {}
function json.decode(src: string): any
    return {}
end
return json`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}
	httpMod := testutil.CheckAndExport(`local http = {}
type Response = {
    body: string?,
}
function http.get(): (Response?, string?)
    return nil, "missing"
end
function http.post(): (Response?, string?)
    return nil, "missing"
end
return http`, "http")
	if len(httpMod.Errors) != 0 {
		t.Fatalf("http module errors = %#v, want none", httpMod.Errors)
	}

	result := testutil.CheckFile(`local json = require("json")
local http = require("http")
local function request(method)
    local response, err
    if method == "GET" then
        response, err = http.get()
    else
        response, err = http.post()
    end
    if not response then
        return nil
    end
    json.decode(response.body)
end`, "client.lua", testutil.WithStdlib(), testutil.WithModule("json", jsonMod), testutil.WithModule("http", httpMod))

	var got []judgment.Judgment
	p := obligationpass.New(obligationpass.CallArguments{})
	for _, bodyResult := range result.BodyResults() {
		got = append(got, p.Run(contextForBody("fixture:client", "client.lua", bodyResult))...)
	}
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.String).Key {
		t.Fatalf("expected = %q, want string", got[0].Expected.Key)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted optional-field mismatch", got[0].Verdict)
	}
	if got[0].Spans[0].StartLine != 13 {
		t.Fatalf("span = %#v, want response.body argument on line 13", got[0].Spans[0])
	}
}

func TestCallArgumentsBindsColonReceiverSlot(t *testing.T) {
	result := testutil.CheckFile(`type Adder = {
    value: number,
    add: (self: Adder, n: number) -> number
}
local a: Adder = {
    value = 0,
    add = function(self: Adder, n: number): number
        self.value = self.value + n
        return self.value
    end
}
a:add("five")`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:method",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Expected.Key != judgment.NewTypeRef(typ.Number).Key {
		t.Fatalf("expected = %q, want number", got[0].Expected.Key)
	}
	if got[0].Spans[0].StartLine != 12 {
		t.Fatalf("span = %#v, want argument on line 12", got[0].Spans[0])
	}
}

func TestCallArgumentsUseImportedReceiverFieldTypeInMethodBody(t *testing.T) {
	timeMod := manifest.New("time")
	durationType := typ.NewInterface("time.Duration", []typ.Method{})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(durationType).Build()},
	})
	timeMod.DefineType("Time", timeType)
	timeMod.DefineType("Duration", durationType)
	timeMod.SetExport(typetable.NewRecord().
		Field("now", typ.Func().Returns(timeType).Build()).
		Build())

	result := testutil.CheckFile(`
local time = require("time")

type Store = {
    started_at: time.Time,
    run: (self: Store, now: time.Time) -> time.Duration,
}

local Store = {}

function Store:run(now: time.Time): time.Duration
    return now:sub(self.started_at)
end
`, "test.lua", testutil.WithManifest("time", timeMod), testutil.WithGlobals("time"))

	var got []judgment.Judgment
	p := obligationpass.New(obligationpass.CallArguments{})
	for _, bodyResult := range result.BodyResults() {
		got = append(got, p.Run(contextForBody("fixture:store", "test.lua", bodyResult))...)
	}
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want imported receiver field type to satisfy parameter", got)
	}
}

func TestCallArgumentsAcceptsInferredCallbackReturn(t *testing.T) {
	result := testutil.CheckFile(`local function map<T, U>(value: T, fn: (T) -> U): U
    return fn(value)
end
local label = map(1, function(n: number)
    return tostring(n)
end)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:callback-return",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 0 {
		t.Fatalf("judgments = %#v, want inferred callback return accepted", got)
	}
}

func TestCallArgumentsKeepsUntrustedAnyUnknown(t *testing.T) {
	result := checkFunction(t, `function f(raw: any) need_string(raw) end`, stringSignatureManifest())

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictUnknown {
		t.Fatalf("verdict = %v, want unknown", got[0].Verdict)
	}
	if got[0].Evidence[2].Kind != judgment.EvidenceMissingProof {
		t.Fatalf("third evidence = %#v, want missing proof", got[0].Evidence[2])
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("evidence = %#v, want untrusted precision-boundary evidence", got[0].Evidence)
	}
}

func TestCallArgumentsReportsUntrustedOptionalAny(t *testing.T) {
	result := checkFunction(t, `function f(raw: any?) need_string(raw) end`, stringSignatureManifest())

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictUnknown {
		t.Fatalf("verdict = %v, want unknown untrusted boundary evidence", got[0].Verdict)
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("evidence = %#v, want untrusted precision-boundary evidence", got[0].Evidence)
	}
}

func TestCallArgumentsKeepsExplicitAnyAssertionEvidence(t *testing.T) {
	result := checkFunction(t, `function f() local raw = (nil :: any) need_string(raw) end`, stringSignatureManifest())

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("evidence = %#v, want untrusted precision-boundary evidence", got[0].Evidence)
	}
	if !hasEvidenceDetail(got[0], judgment.EvidenceDetailUserAssertedAny) {
		t.Fatalf("evidence = %#v, want explicit-any user assertion evidence", got[0].Evidence)
	}
}

func TestCallArgumentsRejectsExplicitAnyForLocalFunctionContract(t *testing.T) {
	result := testutil.CheckFile(`type Payload = { id: string, count: number }
	local raw: any = { id = "cfg", count = 2 }
	local function consume(payload: Payload): number
		return payload.count + 1
	end
	local count = consume(raw)`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want explicit-any local contract argument: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictUnknown ||
		!hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) ||
		!hasEvidenceDetail(got[0], judgment.EvidenceDetailUserAssertedAny) {
		t.Fatalf("judgment = %#v, want unknown precision-boundary explicit-any argument", got[0])
	}
}

func TestCallArgumentsDisplaysExplicitAnyStructuralCandidate(t *testing.T) {
	m := manifest.New("test")
	m.DefineFunctionSignature("need_string", signature.Function{
		Type: typ.Func().Param("req", typetable.NewRecord().Field("id", typ.String).Build()).Build(),
	})
	result := checkFunction(t, `function f()
	local raw = ({ id = "ok" } :: any)
	need_string(raw)
end`, m)

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Actual.ProjectedType == nil || got[0].Actual.ProjectedType.String() != `{id: "ok"}` {
		t.Fatalf(`actual type = %v, want structural candidate {id: "ok"}`, got[0].Actual.ProjectedType)
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) ||
		!hasEvidenceDetail(got[0], judgment.EvidenceDetailUserAssertedAny) {
		t.Fatalf("evidence = %#v, want explicit-any boundary evidence", got[0].Evidence)
	}
}

func TestCallArgumentsKeepsUntrustedOrDefaultUnknown(t *testing.T) {
	result := checkFunction(t, `function f(args)
	local url = (args and args.url) or "http://localhost:8085/hello"
	need_string(url)
end`, stringSignatureManifest())

	got := obligationpass.New(obligationpass.CallArguments{}).Run(obligationpass.Context{
		FunctionKey: "fixture:f",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Verdict != judgment.VerdictUnknown {
		t.Fatalf("verdict = %v, want unknown", got[0].Verdict)
	}
	if !hasEvidenceKind(got[0], judgment.EvidencePrecisionBoundary) {
		t.Fatalf("evidence = %#v, want untrusted precision-boundary evidence", got[0].Evidence)
	}
}

func contextForBody(functionKey, sourceFile string, result *body.Result) obligationpass.Context {
	return obligationpass.Context{
		FunctionKey: functionKey,
		SourceFile:  sourceFile,
		Reader:      readmodel.New(result),
	}
}

func checkFunction(t *testing.T, src string, m *manifest.Manifest) *body.Result {
	t.Helper()
	var manifests []*manifest.Manifest
	if m != nil {
		manifests = append(manifests, m)
	}
	result, err := body.CheckFunction(parseFunction(t, src), body.Config{
		Registry: standard.Registry(),
		Globals:  []string{"need_string"},
		Signatures: signaturelookup.Source{
			Manifests: manifests,
		},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	return result
}

func stringSignatureManifest() *manifest.Manifest {
	m := manifest.New("test")
	m.DefineFunctionSignature("need_string", signature.Function{
		Type: typ.Func().Param("value", typ.String).Build(),
	})
	return m
}

func parseFunction(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts, err := parse.ParseString(src, "test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want one function definition", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	return def.Func
}

func hasEvidenceKind(item judgment.Judgment, kind judgment.EvidenceKind) bool {
	for _, evidence := range item.Evidence {
		if evidence.Kind == kind {
			return true
		}
	}
	return false
}

func hasEvidenceDetail(item judgment.Judgment, detail judgment.EvidenceDetailKind) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == detail {
			return true
		}
	}
	return false
}

func missingRequiredFieldEvidenceDetail(item judgment.Judgment) (string, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof &&
			evidence.Detail.Kind == judgment.EvidenceDetailMissingRequiredField {
			return evidence.Detail.Field, true
		}
	}
	return "", false
}

func mayBeNilEvidenceDetail(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof &&
			evidence.Detail.Kind == judgment.EvidenceDetailMayBeNil {
			return true
		}
	}
	return false
}

func genericConflictEvidenceDetail(item judgment.Judgment) (string, bool) {
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceMissingProof &&
			evidence.Detail.Kind == judgment.EvidenceDetailGenericConflict {
			return evidence.Detail.Param, true
		}
	}
	return "", false
}

func genericConflictContributionSpans(item judgment.Judgment) []judgment.SpanRef {
	var spans []judgment.SpanRef
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceAbstractFact &&
			evidence.Span.StartLine != 0 {
			spans = append(spans, evidence.Span)
		}
	}
	return spans
}

func genericConflictContributionLabels(item judgment.Judgment) []string {
	var labels []string
	for _, evidence := range item.Evidence {
		if evidence.Kind == judgment.EvidenceAbstractFact &&
			evidence.Detail.SubjectLabel != "" {
			labels = append(labels, evidence.Detail.SubjectLabel)
		}
	}
	return labels
}
