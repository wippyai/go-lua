package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/projection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

// These cases are reduced from real Wippy package lint failures. Each test pins
// a general abstract-interpretation law; none is allowed to depend on Wippy
// package names or DSL globals.

func roundTripModuleManifest(t *testing.T, name string, mod *ModuleResult) *manifest.Manifest {
	t.Helper()
	data, err := manifest.Encode(mod.Manifest)
	if err != nil {
		t.Fatalf("Encode %s: %v", name, err)
	}
	decoded, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("Decode %s: %v", name, err)
	}
	return decoded
}

func TestCheckOptionalAssignedOrDefaultNarrowsTarget(t *testing.T) {
	result := Check(`
local function progress_bar(width: number?): number
    width = width or 20
    local n: number = width
    return n
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want assigned logical-or default to refine target non-nil", result.Diagnostics)
	}
}

func TestCheckStringMethodMatchOnAnnotatedReceiver(t *testing.T) {
	result := Check(`
local function dependency_kind(dep_id: string): string
    if not dep_id:match(":") then
        return "bootloader"
    end
    return "service"
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want string method receiver syntax to be callable on annotated string", result.Diagnostics)
	}
}

func TestCheckMethodReturnContractUsesCallContextReceiver(t *testing.T) {
	result := Check(`
local Counter = {count = 0}
function Counter:increment(): ()
    self.count = self.count + 1
end
function Counter:get(): number
    return self.count
end
Counter:increment()
local n: number = Counter:get()
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want method return checked through call-context receiver shape", result.Diagnostics)
	}
}

func TestCheckGenericMapLoopUsesContextualItemType(t *testing.T) {
	result := Check(`
type Metric = { name: string, value: number }
type Event = { name: string }

local function map<T, U>(items: {T}, fn: (T) -> U): {U}
    local out: {U} = {}
    for _, item in ipairs(items) do
        table.insert(out, fn(item))
    end
    return out
end

local function metric(name: string, value: number): Metric
    return { name = name, value = value }
end

local metrics = { metric("latency", 42) }
local events: {Event} = map(metrics, function(metric: Metric): Event
    return { name = metric.name }
end)
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want contextual generic map item to satisfy callback parameter", result.Diagnostics)
	}
}

func TestCheckCrossModuleGenericMapLoopUsesContextualItemType(t *testing.T) {
	protocol := CheckAndExport(`
type Event = { name: string, tags: {[string]: string} }
type Metric = { name: string, value: number, tags: {[string]: string} }
local M = {}
M.Event = Event
M.Metric = Metric
return M
`, "protocol")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}
	builder := CheckAndExport(`
local protocol = require("protocol")
local M = {}
function M.metric(name: string, value: number, tags: {[string]: string}): protocol.Metric
    return { name = name, value = value, tags = tags }
end
function M.event(metric: protocol.Metric): protocol.Event
    return { name = metric.name, tags = metric.tags }
end
return M
`, "builder", WithModule("protocol", protocol))
	if len(builder.Errors) != 0 {
		t.Fatalf("builder diagnostics = %#v, want none", builder.Errors)
	}
	result := Check(`
local builder = require("builder")
local protocol = require("protocol")

local function map<T, U>(items: {T}, fn: (T) -> U): {U}
    local out: {U} = {}
    for _, item in ipairs(items) do
        table.insert(out, fn(item))
    end
    return out
end

local metrics = {
    builder.metric("latency", 42, {source = "api"}),
}
local events: {protocol.Event} = map(metrics, function(metric: protocol.Metric): protocol.Event
    return builder.event(metric)
end)
local wrong_events: {protocol.Metric} = map(metrics, function(metric: protocol.Metric): protocol.Event
    return builder.event(metric)
end)
`, WithStdlib(), WithModule("protocol", protocol), WithModule("builder", builder))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want only the intentionally wrong result assignment", result.Diagnostics)
	}
	if result.Diagnostics[0].Position.Line != 19 {
		t.Fatalf("diagnostic line = %d, want wrong_events assignment only", result.Diagnostics[0].Position.Line)
	}
}

func TestCheckNilInitializedOptionalMethodReceiverReports(t *testing.T) {
	result := Check(`
type Handler = {
    run: fun(self: Handler, id: string): number
}

local maybe_handler: Handler? = nil
maybe_handler:run("job-1")
`)
	requireDiagnosticCode(t, result, diagnostics.CodeOptionalMethodCall)
}

func TestCheckOptionalLiteralUnionOrDefaultPreservesUnion(t *testing.T) {
	result := Check(`
type Level = "debug" | "info"

local function choose(level: Level?): Level
    local selected: Level = level or "info"
    return selected
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want literal-union default to preserve declared union", result.Diagnostics)
	}
}

func TestCheckTypedOptionalFieldOrFallbackPreservesString(t *testing.T) {
	result := Check(`
type Uuid = {
    v7: () -> string,
}
type Options = {
    data_id: string?,
}

local uuid: Uuid = {
    v7 = function(): string
        return ""
    end,
}

local function data_id(options: Options?): string
    options = options or {}
    return options.data_id or uuid.v7()
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed optional field fallback to prove string", result.Diagnostics)
	}
}

func TestCheckAnyOrFallbackDoesNotProveString(t *testing.T) {
	result := Check(`
local function needs_string(value: string): ()
end

local function from_untrusted(raw: any): ()
    local selected = raw or "fallback"
    needs_string(selected)
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"argument 1 (selected)", "comes from any/unknown", "no proof shows it is string"},
	})
}

func TestCheckUnknownConditionAndOrLiteralSelectorProvesResultType(t *testing.T) {
	result := Check(`
type PageResponse = {
    hidden: number,
}

type Page = {
    inline: unknown,
}

local function page_info(page: Page): PageResponse
    return {
        hidden = page.inline and 1 or 0,
    }
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unknown condition in and/or literal selector to produce numeric literal arms only", result.Diagnostics)
	}
}

func TestCheckUnpackNonEmptyStringSequenceSatisfiesVariadicStringParam(t *testing.T) {
	result := Check(`
local function accept(...: string): ()
end

local function build(): ()
    local select_fields = { "id", "name" }
    table.insert(select_fields, "created_at")
    accept(unpack(select_fields))
    accept(table.unpack(select_fields))
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unpack of non-empty string sequence to satisfy variadic string parameter", result.Diagnostics)
	}
}

func TestCheckUnpackUnknownLengthUnknownSequenceDoesNotProveRequiredArgument(t *testing.T) {
	result := Check(`
local function accept(value: string): ()
end

local function build(values: {unknown}): ()
    accept(unpack(values))
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"unpack(...)", "may be nil"},
	})
}

func TestCheckUnpackNonEmptyUnknownSequenceDoesNotSatisfyStringParam(t *testing.T) {
	result := Check(`
local function accept(value: string): ()
end

local function build(raw: unknown): ()
    local values = { raw }
    accept(unpack(values))
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"argument 1 (unpack(...))", "no proof shows it is string"},
	})
}

func TestCheckUnpackUnknownLengthStringSequenceDoesNotProveRequiredArgument(t *testing.T) {
	result := Check(`
local function accept(value: string): ()
end

local function build(values: {string}): ()
    accept(unpack(values))
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"unpack(...)", "may be nil"},
	})
}

func TestCheckUnpackReturnedClauseTablePreservesFirstElementPresence(t *testing.T) {
	result := Check(`
local function expr(sql: string, ...: string): ()
end

local function create_clause(field: string, values: {string})
    if not values or #values == 0 then
        return nil
    end
    local placeholders = {}
    for i = 1, #values do
        table.insert(placeholders, "?")
    end
    return { field .. " IN (" .. table.concat(placeholders, ", ") .. ")", unpack(values) }
end

local function build(values: {string}): ()
    local clause = create_clause("node_id", values)
    if clause then
        expr(unpack(clause))
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unpack of returned clause table to preserve proven first string element", result.Diagnostics)
	}
}

func TestCheckUnpackLocalClauseTableWithOpenTailPreservesFirstElementPresence(t *testing.T) {
	result := Check(`
local function expr(sql: string, ...: string): ()
end

local function build(values: {string}): ()
    local clause = { "node_id IN (?)", unpack(values) }
    expr(unpack(clause))
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unpack of local clause table to preserve proven first string element", result.Diagnostics)
	}
}

func TestRequireCheckInsertedHelperReturnArrayElementShapeSurvivesImport(t *testing.T) {
	pageRegistry := CheckAndExport(`
local pages = {}

local function extract_page_info(entry)
    local meta = entry.meta
    return {
        id = entry.id,
        title = meta.title or "",
        order = meta.order or 9999,
        secure = meta.secure or false,
        announced = meta.announced or meta.public or false,
        inline = meta.inline or false,
    }
end

function pages.find_all()
    local entries = {
        {
            id = "page.home",
            meta = {
                title = "Home",
                order = 1,
                secure = false,
                public = true,
                announced = true,
                inline = true,
            },
        },
    }
    local pages_list = {}
    for _, entry in ipairs(entries) do
        if entry.meta then
            table.insert(pages_list, extract_page_info(entry))
        end
    end
    return pages_list
end

return pages
`, "page_registry", WithStdlib())
	if len(pageRegistry.Errors) != 0 {
		t.Fatalf("page_registry errors = %#v, want none", pageRegistry.Errors)
	}
	sig, ok := pageRegistry.Manifest.FunctionSignatures["page_registry.find_all"]
	if !ok {
		t.Fatalf("page_registry signatures = %#v, want find_all", pageRegistry.Manifest.FunctionSignatures)
	}
	if sig.OperationalEffects == nil {
		t.Fatalf("find_all signature = %#v, want operational return element facts", sig)
	}
	hasReturnElementFact := false
	for _, fact := range sig.OperationalEffects.DynamicIndexFacts {
		if fact.Table.Equal(pathdom.Path{Root: "ret[0]"}) {
			hasReturnElementFact = true
			break
		}
	}
	if !hasReturnElementFact {
		t.Fatalf("find_all operational effects = %#v, want ret[0] array element fact", sig.OperationalEffects)
	}

	result := Check(`
local page_registry = require("page_registry")

type PageResponse = {
    hidden: number,
}

local all_pages = page_registry.find_all()
local pages: {PageResponse} = {}
for _, page in ipairs(all_pages) do
    local page_info: PageResponse = {
        hidden = page.inline and 1 or 0,
    }
    table.insert(pages, page_info)
end
`, WithStdlib(), WithModule("page_registry", pageRegistry))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inserted helper-return array element shape to preserve inline boolean across import", result.Diagnostics)
	}
}

func TestCheckStdlibMathFloorResultCanFlowToIntegerParameter(t *testing.T) {
	result := Check(`
local function progress_bar(current: number, total: number, width: number?): string
    width = width or 20
    local filled = math.floor((current / total) * width)
    return string.rep("x", filled)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want math.floor result to satisfy integer parameter", result.Diagnostics)
	}
}

func TestCheckIntegerSubtractionPreservesIntegerForParameter(t *testing.T) {
	result := Check(`
local function progress_bar(current: number, total: number, width: integer?): string
    width = width or 20
    local filled = math.floor((current / total) * width)
    local empty = width - filled
    return string.rep(".", empty)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want integer subtraction to satisfy integer parameter", result.Diagnostics)
	}
}

func TestCheckNumberWidthDoesNotLaunderIntoIntegerParameter(t *testing.T) {
	result := Check(`
local function progress_bar(width: number?): string
    width = width or 20
    return string.rep(".", width)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"argument 2 (width) is number", "not integer"},
	})
}

func TestCheckModuloDerivedTupleIndexIsPresent(t *testing.T) {
	result := Check(`
local frames = {"a", "b", "c"}

local function spinner(index: integer): string
    local frame = frames[((index - 1) % #frames) + 1]
    return frame
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want modulo-derived tuple index to prove present value", result.Diagnostics)
	}
}

func TestCheckModuloDerivedModuleFieldTupleIndexIsPresent(t *testing.T) {
	result := Check(`
local term = {}
term.spinner_frames = {"a", "b", "c"}

function term.spinner(index: integer): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return frame
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want module-field tuple index to prove present value", result.Diagnostics)
	}
}

func TestCheckModuloDerivedModuleFieldTupleIndexCastCanFlowToModuleFunction(t *testing.T) {
	result := Check(`
local term = {}

function term.cyan(s: string): string
    return s
end

term.spinner_frames = {"a", "b", "c"}

function term.spinner(index: integer): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return term.cyan(frame :: string)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want casted module-field tuple index to satisfy module function", result.Diagnostics)
	}
}

func TestCheckNumberModuloIndexDoesNotProveTupleElementPresent(t *testing.T) {
	result := Check(`
local term = {}

function term.cyan(s: string): string
    return s
end

term.spinner_frames = {"a", "b", "c"}

function term.spinner(index: number): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return term.cyan(frame)
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"cannot pass frame", "may be nil"},
	})
}

func TestCheckUntypedFieldOrFallbackDoesNotLaunderString(t *testing.T) {
	result := Check(`
type Uuid = {
    v7: () -> string,
}

local uuid: Uuid = {
    v7 = function(): string
        return ""
    end,
}

local function take(id: string): ()
end

local options: any = {}
take(options.data_id or uuid.v7())
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"argument 1", "any/unknown", "no proof shows it is string"},
	})
}

func TestCheckImportedAssertionWrapperNarrowsOptionalArgument(t *testing.T) {
	testlib := CheckAndExport(`
local M = {}

function M.not_nil(val: any, msg: string?): any
    if val == nil then
        error(msg or "nil")
    end
    return val
end

return M
`, "testlib", WithStdlib())
	if len(testlib.Errors) != 0 {
		t.Fatalf("testlib diagnostics = %#v, want none", testlib.Errors)
	}

	result := Check(`
local test = require("testlib")

type Impl = {
    up: any?,
}

local function use(impl: Impl?): ()
    test.not_nil(impl)
    local up = impl.up
end
`, WithStdlib(), WithModule("testlib", testlib))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported assertion wrapper to prove argument present", result.Diagnostics)
	}
}

func TestCheckInvalidAnnotatedLocalRecoversDeclaredTypeForDownstreamUse(t *testing.T) {
	result := Check(`
type Response = {
    headers: {[string]: string},
}
type Context = {
    session: { user_id: string }?,
}

local function respond(ctx: Context): Response
    local user_id: string = ctx.session.user_id
    return {
        headers = {
            ["x-user"] = user_id,
        },
    }
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		MessageContains: []string{"cannot assign ctx.session.user_id"},
	})
}

func TestCheckRecursiveRecordLiteralMethodsSatisfyDeclaredAlias(t *testing.T) {
	result := Check(`
type Level = "debug" | "info"

type Logger = {
    level: Level,
    log: (self: Logger, level: Level, msg: string) -> (),
    debug: (self: Logger, msg: string) -> (),
}

local function new_logger(level: Level?): Logger
    local logger: Logger = {
        level = level or "info",
        log = function(self: Logger, level: Level, msg: string)
            print(level .. msg)
        end,
        debug = function(self: Logger, msg: string)
            self:log("debug", msg)
        end,
    }
    return logger
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want recursive record literal methods to satisfy declared alias", result.Diagnostics)
	}
}

func TestCheckPCallSuccessGuardProjectsCallbackReturn(t *testing.T) {
	result := Check(`
local function run_tests(): number
    return 1
end

local function main(): number
    local ok, result = pcall(run_tests)
    if not ok then
        return 1
    end
    local status: number = result
    return status
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want pcall success guard to project callback return", result.Diagnostics)
	}
}

func TestCheckPCallCallbackKeepsGuardedCapturedChannelPresent(t *testing.T) {
	result := Check(`
type Message = { topic: (self: Message) -> string }
type MessageChannel = Channel<Message>

local function wait_for_topic(inbox: MessageChannel): ()
    local case = inbox:case_receive()
end

local function main(inbox: MessageChannel?): boolean
    if inbox == nil then
        error("missing inbox")
    end
    local ok, err = pcall(function()
        wait_for_topic(inbox)
    end)
    return ok
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want pcall callback to keep guarded captured channel present", result.Diagnostics)
	}
}

func TestCheckPCallCallbackSeedsProtectedCallArguments(t *testing.T) {
	result := Check(`
type Message = { topic: (self: Message) -> string }
type MessageChannel = Channel<Message>

local function wait_for_topic(inbox: MessageChannel): ()
    local case = inbox:case_receive()
end

local function main(inbox: MessageChannel?): boolean
    if inbox == nil then
        error("missing inbox")
    end
    local ok, err = pcall(function(ch)
        wait_for_topic(ch)
    end, inbox)
    return ok
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want pcall callback parameters to inherit protected-call argument types", result.Diagnostics)
	}
}

func TestCheckChannelSelectHelperReturnPreservesSelectedPayloadType(t *testing.T) {
	result := Check(`
type Message = { topic: (self: Message) -> string }
type MessageChannel = Channel<Message>

local function payload_data(msg: Message): string
    return msg:topic()
end

local function wait_for_topic(inbox: MessageChannel)
    local result = channel.select {
        inbox:case_receive(),
    }
    if result.channel == inbox then
        local msg = result.value as Message
        return msg, nil
    end
    return nil, "timeout"
end

local function main(inbox: MessageChannel): boolean
    local msg, wait_err = wait_for_topic(inbox)
    if wait_err then
        return false
    end
    if msg == nil then
        error("missing message")
    end
    local topic = payload_data(msg as Message)
    return topic ~= ""
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want selected channel payload type to survive helper return and redundant cast", result.Diagnostics)
	}
}

func TestCheckChannelSelectHelperReturnPreservesManifestAliasCast(t *testing.T) {
	process := manifest.New("process")
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})
	messageChannelType := typ.Instantiate(ambient.ChannelGeneric(), messageType)
	process.DefineType("Message", messageType)
	process.DefineGlobal("process")
	process.SetExport(typetable.NewRecord().
		Field("inbox", typ.Func().Returns(messageChannelType).Build()).
		Build())

	result := Check(`
type Message = process.Message
type MessageChannel = Channel<Message>

local function payload_data(msg: Message): any?
    local p = msg:payload()
    return p and p:data() or nil
end

local function wait_for_topic(inbox: MessageChannel, deadline: unknown)
    while true do
        local result = channel.select {
            inbox:case_receive(),
            deadline:case_receive(),
        }
        if result.channel == deadline then
            return nil, "timeout waiting for message"
        end
        local msg = result.value as Message
        if msg:topic() == "ack" then
            return msg, nil
        end
    end
end

local function main(deadline: unknown): boolean
    local inbox = process.inbox() as MessageChannel
    local msg, wait_err = wait_for_topic(inbox, deadline)
    if wait_err then
        return false
    end
    if msg == nil then
        error("missing message")
    end
    local data = payload_data(msg as Message)
    return data ~= nil
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want manifest type alias cast to survive helper return and nil guard", result.Diagnostics)
	}
}

func TestCheckAnyReturningProcessInboxConcreteCastValidatesChannelHelperArgument(t *testing.T) {
	process := manifest.New("process")
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	process.DefineType("Message", messageType)
	process.DefineGlobal("process")
	process.SetExport(typetable.NewRecord().
		Field("inbox", typ.Func().Returns(typ.Any).Build()).
		Build())

	result := Check(`
type Message = process.Message
type MessageChannel = Channel<Message>

local function wait_for_topic(inbox: MessageChannel): ()
    local case = inbox:case_receive()
end

local function main(): ()
    local inbox = process.inbox() as MessageChannel
    wait_for_topic(inbox)
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete cast to validate process.inbox() result", result.Diagnostics)
	}
}

func TestCheckProcessInboxChannelSelectPreservesMessagePayloadType(t *testing.T) {
	process := processMessageModeManifest(false)

	result := Check(`
local function main(): ()
    local inbox_ch = process.inbox()
    local result = channel.select({
        inbox_ch:case_receive(),
    })

    if result.channel == inbox_ch then
        local msg = result.value
        local from = msg:from()
        process.send(from, "ack", { ok = true })
    end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want channel.select inbox branch to preserve process.Message payload type", result.Diagnostics)
	}
}

func TestCheckProcessInboxEventsSelectPreservesMessagePayloadType(t *testing.T) {
	process := processMessageModeManifest(false)

	result := Check(`
local function main(): ()
    local inbox_ch = process.inbox()
    local events_ch = process.events()
    local pending = {}
    local result = channel.select({
        inbox_ch:case_receive(),
        events_ch:case_receive(),
    })

    if result.channel == inbox_ch then
        local msg = result.value
        local from = msg:from() :: string
        process.send(from, "ack", { ok = true })
    elseif result.channel == events_ch then
        local event = result.value
        if event.kind == process.event.EXIT then
            process.send(event.from, "exit", {})
        end
    end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inbox branch of mixed select to preserve process.Message payload type", result.Diagnostics)
	}
}

func TestCheckProcessInboxPayloadProofSurvivesSpawnMonitoredCall(t *testing.T) {
	process := processMessageModeManifest(false)

	result := Check(`
local function main(): ()
    local inbox_ch = process.inbox()
    local events_ch = process.events()
    local pending = {}
    local result = channel.select({
        inbox_ch:case_receive(),
        events_ch:case_receive(),
    })

    if result.channel == inbox_ch then
        local msg = result.value
        local from = msg:from() :: string
        local payload = msg:payload():data()
        if type(payload.respond_to) ~= "string" then
            return
        end
        local worker_pid, err = process.spawn_monitored("worker", "group", {
            work_data = payload.work_data,
        })
        if err then
            process.send(from, payload.respond_to, { worker = worker_pid })
        else
            pending[worker_pid] = {
                from = from,
                respond_to = payload.respond_to,
                request_id = payload.request_id,
            }
        end
    end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local message sender proof to survive unrelated spawn_monitored call", result.Diagnostics)
	}
}

func TestCheckRuntimeModuleChannelSelectPreservesMessagePayloadType(t *testing.T) {
	process := processMessageModeManifestWithChannelGeneric(runtimeModuleChannelGeneric(), false)

	result := Check(`
local function main(): ()
    local inbox_ch = process.inbox()
    local events_ch = process.events()
    local result = channel.select({
        inbox_ch:case_receive(),
        events_ch:case_receive(),
    })

    if result.channel == inbox_ch then
        local msg = result.value
        local from = msg:from() :: string
        process.send(from, "ack", { ok = true })
    end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want channel.Channel<T> select branch to preserve process.Message payload type", result.Diagnostics)
	}
}

func TestCheckRuntimeModuleChannelSelectInequalityRemovesTimerPayload(t *testing.T) {
	channelGeneric := runtimeModuleChannelGeneric()
	process := processMessageModeManifestWithChannelGeneric(channelGeneric, false)
	timeMod := timeAfterManifest(channelGeneric)

	result := Check(`
local function main(): ()
    local inbox_ch = process.inbox()
    local timeout = time.after("2s")
    local result = channel.select({
        inbox_ch:case_receive(),
        timeout:case_receive(),
    })

    if result.channel ~= timeout then
        local topic = result.value:topic()
        process.send("worker", topic, {})
    end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithManifest("time", timeMod), WithGlobals("channel", "time"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want channel inequality branch to remove timer payload", result.Diagnostics)
	}
}

func TestCheckRuntimeModuleChannelSelectAssertNeqRemovesTimerPayload(t *testing.T) {
	channelGeneric := runtimeModuleChannelGeneric()
	process := processMessageModeManifestWithChannelGeneric(channelGeneric, false)
	timeMod := timeAfterManifest(channelGeneric)

	result := Check(`
local function assert_neq(actual: any, expected: any): ()
    if actual == expected then
        error("equal")
    end
end

local function main(): ()
    local inbox_ch = process.inbox()
    local timeout = time.after("2s")
    local result = channel.select({
        inbox_ch:case_receive(),
        timeout:case_receive(),
    })

    assert_neq(result.channel, timeout)
    local topic = result.value:topic()
    process.send("worker", topic, {})
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithManifest("time", timeMod), WithGlobals("channel", "time"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want assert_neq normal return to remove timer payload", result.Diagnostics)
	}
}

func TestCheckProcessListenMessageModeConditionalReturn(t *testing.T) {
	process := processMessageModeManifest(true)

	result := Check(`
local function main(): ()
    local ch = process.listen("increment", { message = true })
    local msg, ok = ch:receive()
    if ok then
        local reply_to = msg:from()
        process.send(reply_to, "ack", { ok = true })
    end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want message=true listen mode to return Channel<process.Message>", result.Diagnostics)
	}
}

func TestCheckProcessListenDefaultStaysRawPayload(t *testing.T) {
	process := processMessageModeManifest(true)

	result := Check(`
local function main(): ()
    local ch = process.listen("increment")
    local msg, ok = ch:receive()
    if ok then
        process.send(msg:from(), "ack", { ok = true })
	end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		MessageContains: []string{"argument 1", "comes from any/unknown", "no proof shows it is string"},
	})
}

func processMessageModeManifest(enableMessageMode bool) *manifest.Manifest {
	return processMessageModeManifestWithChannelGeneric(ambient.ChannelGeneric(), enableMessageMode)
}

func processMessageModeManifestWithChannelGeneric(channelGeneric *typ.Generic, enableMessageMode bool) *manifest.Manifest {
	process := manifest.New("process")
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})
	eventType := typetable.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		Build()
	rawChannelType := typ.Instantiate(channelGeneric, typ.Any)
	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	eventChannelType := typ.Instantiate(channelGeneric, eventType)

	listenType := typ.Func().
		Param("topic", typ.String).
		OptParam("options", typ.Any).
		Returns(rawChannelType).
		Build()
	listenSig := signature.Function{Type: listenType}
	if enableMessageMode {
		listenSig.Effect = effect.Empty.With(returns.Return{
			ReturnIndex: 0,
			Transform: returns.ConditionalType{
				Source: effect.ParamRef{Index: 1},
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("message"),
				}},
				When: typ.LiteralBool(true),
				Then: messageChannelType,
			},
		})
	}

	process.DefineType("Message", messageType)
	process.DefineGlobal("process")
	process.DefineFunctionSignature("process.listen", listenSig)
	process.DefineFunctionSignature("process.inbox", signature.Function{
		Type: typ.Func().Returns(messageChannelType).Build(),
	})
	process.DefineFunctionSignature("process.events", signature.Function{
		Type: typ.Func().Returns(eventChannelType).Build(),
	})
	process.DefineFunctionSignature("process.send", signature.Function{
		Type: typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			OptParam("payload", typ.Any).
			Build(),
	})
	process.SetExport(typetable.NewRecord().
		Field("listen", listenType).
		Field("inbox", typ.Func().Returns(messageChannelType).Build()).
		Field("events", typ.Func().Returns(eventChannelType).Build()).
		Field("event", typetable.NewRecord().Field("EXIT", typ.String).Build()).
		Field("spawn_monitored", typ.Func().
			Param("module", typ.String).
			Param("func", typ.String).
			Param("options", typ.Any).
			Returns(typ.String, typeexpr.Optional(typ.Any)).
			Build()).
		Field("send", typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			OptParam("payload", typ.Any).
			Build()).
		Build())
	return process
}

func runtimeModuleChannelGeneric() *typ.Generic {
	elem := typ.NewTypeParam("T", nil)
	return typ.NewGeneric("channel.Channel", []*typ.TypeParam{elem}, typ.NewInterface("channel.Channel", []typ.Method{
		{Name: "receive", Type: typ.Func().Param("self", typ.Self).Returns(elem, typ.Boolean).Build()},
		{Name: "case_receive", Type: typ.Func().Param("self", typ.Self).Returns(typ.Unknown).Build()},
		{Name: "send", Type: typ.Func().Param("self", typ.Self).Param("value", elem).Returns(typ.Boolean).Build()},
	}))
}

func timeAfterManifest(channelGeneric *typ.Generic) *manifest.Manifest {
	timeMod := manifest.New("time")
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timerChannelType := typ.Instantiate(channelGeneric, timeType)
	afterType := typ.Func().Param("duration", typ.String).Returns(timerChannelType).Build()
	timeMod.DefineType("Time", timeType)
	timeMod.DefineFunctionSignature("time.after", signature.Function{Type: afterType})
	timeMod.SetExport(typetable.NewRecord().
		Field("after", afterType).
		Build())
	return timeMod
}

func TestCheckManifestLoggerInterfaceMethodsConsumeReceiver(t *testing.T) {
	logger := loggerPrecisionManifest()
	result := Check(`
local logger = require("logger")

logger:error("failed to get message", {error = "boom"})
logger:info("ticker started", {ticks = 3})

local scoped = logger:named("worker")
scoped:warn("slow task", {duration_ms = 250})
scoped:debug("done")
`, WithStdlib(), WithManifest("logger", logger))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want logger module and derived logger instances to consume receiver for method calls", result.Diagnostics)
	}
}

func TestCheckManifestLoggerDotCallDoesNotConsumeReceiver(t *testing.T) {
	logger := loggerPrecisionManifest()
	result := Check(`
local logger = require("logger")

logger.info("ticker started")
`, WithStdlib(), WithManifest("logger", logger))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallTooFewArgs,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		MessageContains: []string{"logger.info expects 2 arguments, got 1"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"call to logger.info passes 1 argument"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"logger.info declares 2 parameters"},
			},
		},
	})
}

func loggerPrecisionManifest() *manifest.Manifest {
	logger := manifest.New("logger")
	loggerType := typ.NewInterface("logger.Logger", []typ.Method{
		{Name: "debug", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).OptParam("context", typ.Any).Build()},
		{Name: "info", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).OptParam("context", typ.Any).Build()},
		{Name: "warn", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).OptParam("context", typ.Any).Build()},
		{Name: "error", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).OptParam("context", typ.Any).Build()},
		{Name: "named", Type: typ.Func().Param("self", typ.Self).Param("name", typ.String).Returns(typ.Self).Build()},
	})
	logger.DefineType("Logger", loggerType)
	logger.SetExport(loggerType)
	return logger
}

func TestCheckDirectCallArgumentCastToManifestAliasValidatesRuntimeType(t *testing.T) {
	process := manifest.New("process")
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	process.DefineType("Message", messageType)

	result := Check(`
type Message = process.Message

local function payload_data(msg: Message): string
    return msg:topic()
end

local function main(raw: any): string
    return payload_data(raw as Message)
end
`, WithManifest("process", process))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want manifest alias cast to validate the runtime type on the normal path", result.Diagnostics)
	}
}

func TestCheckGuardedLocalCastFromAnyReportsProofBoundaryNotSameTypeMismatch(t *testing.T) {
	result := Check(`
local function send(user_id: string): ()
end

local function main(metadata: any): ()
    local user_id = (metadata and metadata.user_id) :: string?
    if not user_id then
        return
    end
    send(user_id)
end
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one missing-proof diagnostic", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if !strings.Contains(diag.Message, "argument 1 (user_id) comes from any/unknown") ||
		!strings.Contains(diag.Message, "no proof shows it is string") {
		t.Fatalf("diagnostic message = %q, want proof-boundary explanation", diag.Message)
	}
	if strings.Contains(diag.Message, "string, not string") {
		t.Fatalf("diagnostic message = %q, want no redundant same-type mismatch", diag.Message)
	}
}

func TestCheckNilGuardedSecondReturnSlotAllowsErrorMethods(t *testing.T) {
	result := Check(`
type AppError = {
    kind: (self: AppError) -> string,
    retryable: (self: AppError) -> boolean,
}

local function decode(): (string?, AppError?)
    return nil, nil
end

local result, err = decode()
if result ~= nil then
    error("expected nil result")
end
if err == nil then
    error("expected error")
end
if err:kind() ~= "invalid" then
    error("expected invalid kind")
end
if err:retryable() ~= false then
    error("expected non-retryable")
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nil guard on second return slot to expose error methods", result.Diagnostics)
	}
}

func TestCheckFalseFirstReturnSlotProvesSecondReturnSlotString(t *testing.T) {
	result := Check(`
local function wait_for_database(ready: boolean): (boolean, string?)
    if ready then
        return true, nil
    end
    return false, "database unavailable"
end

local function boot(ready: boolean): string
    local db_ready, db_err = wait_for_database(ready)
    if not db_ready then
        return "Database unavailable: " .. db_err
    end
    return "ok"
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want false first return slot to prove second slot string", result.Diagnostics)
	}
}

func TestCheckFalseFirstReturnSlotDoesNotInventMissingSecondSlot(t *testing.T) {
	result := Check(`
local function wait_for_database(ready: boolean, missing: boolean): (boolean, string?)
    if ready then
        return true, nil
    end
    if missing then
        return false, nil
    end
    return false, "database unavailable"
end

local function boot(ready: boolean, missing: boolean): string
    local db_ready, db_err = wait_for_database(ready, missing)
    if not db_ready then
        return "Database unavailable: " .. db_err
    end
    return "ok"
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeConcatOperand,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"right operand", "may be nil"},
	})
}

func TestCheckErrorReturnSiblingPresentAfterValueNilGuard(t *testing.T) {
	result := Check(`
type AppError = {
    kind: (self: AppError) -> string,
}
type User = { id: number }

local function new_error(): AppError
    return { kind = function(self: AppError): string return "invalid" end }
end

local function fetch_user(id)
    if id < 0 then
        return nil, new_error()
    end
    return { id = id }, nil
end

local user, err = fetch_user(-1)
if user ~= nil then
    error("expected nil user")
end
local kind: string = err:kind()
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want value nil guard to prove error slot present for unannotated local result function", result.Diagnostics)
	}
}

func TestCheckErrorReturnSiblingPresentAfterImportedValueNilAssert(t *testing.T) {
	assertMod := CheckAndExport(`
local M = {}

function M.is_nil(value: any, msg: string?)
    if value ~= nil then
        error(msg or "expected nil", 2)
    end
end

return M
`, "assert2", WithStdlib())
	if len(assertMod.Errors) != 0 {
		t.Fatalf("assert module diagnostics = %#v, want clean helper export", assertMod.Errors)
	}

	result := Check(`
local assert = require("assert2")

type AppError = {
    kind: (self: AppError) -> string,
}
type User = { id: number }

local function new_error(): AppError
    return { kind = function(self: AppError): string return "invalid" end }
end

local function fetch_user(id)
    if id < 0 then
        return nil, new_error()
    end
    return { id = id }, nil
end

local user, err = fetch_user(-1)
assert.is_nil(user, "no user on error")
local kind: string = err:kind()
`, WithStdlib(), WithModule("assert2", assertMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported value nil assert to prove error slot present for unannotated local result function", result.Diagnostics)
	}
}

func TestCheckImportedTypeAssertRefinesAnyToString(t *testing.T) {
	assertMod := CheckAndExport(`
local M = {}

function M.is_string(value, msg)
    if type(value) ~= "string" then
        error(msg or "expected string", 2)
    end
    return value
end

return M
`, "assert2", WithStdlib())
	if len(assertMod.Errors) != 0 {
		t.Fatalf("assert module diagnostics = %#v, want clean helper export", assertMod.Errors)
	}

	result := Check(`
local assert = require("assert2")

local function check_denied(msg: any): boolean
    assert.is_string(msg, "error must be a string")
    local hit = string.find(msg, "not allowed", 1, true)
    return hit ~= nil
end
`, WithStdlib(), WithModule("assert2", assertMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported type assertion to refine msg to string", result.Diagnostics)
	}
}

func TestCheckImportedHasErrorRefinesErrorSecondSlot(t *testing.T) {
	assertMod := CheckAndExport(`
local M = {}

function M.has_error(value, err, msg)
    if value ~= nil then
        error(msg or "expected nil result", 2)
    end
    if err == nil then
        error(msg or "expected error", 2)
    end
end

return M
`, "assert2", WithStdlib())
	if len(assertMod.Errors) != 0 {
		t.Fatalf("assert module diagnostics = %#v, want clean helper export", assertMod.Errors)
	}

	result := Check(`
local assert = require("assert2")

type AppError = {
    kind: (self: AppError) -> string,
}
type Result = { id: string }

local function make_error(): AppError
    return { kind = function(self: AppError): string return "invalid" end }
end

local function fetch(flag: boolean): (Result?, AppError?)
    if flag then
        return nil, make_error()
    end
    return { id = "ok" }, nil
end

local res, err = fetch(true)
assert.has_error(res, err, "expected failure")
local kind: string = err:kind()
`, WithStdlib(), WithModule("assert2", assertMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported has_error to refine second error slot present", result.Diagnostics)
	}
}

func TestCheckOrGuardNarrowsRightOperandAndContinuation(t *testing.T) {
	result := Check(`
local function contains(str: any, substr: string): string
    if type(str) ~= "string" or not string.find(str, substr, 1, true) then
        error("missing")
    end
    local narrowed: string = str
    return narrowed
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want false side of left or-guard to refine RHS and continuation", result.Diagnostics)
	}
}

func TestCheckTypedOptionalMethodPresenceGuardAllowsCall(t *testing.T) {
	result := Check(`
type Value = { _tostring: (() -> string)? }
local function format_value(val: Value): string
    if val._tostring then
        return val:_tostring()
    end
    return ""
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want optional method presence guard to expose callable member", result.Diagnostics)
	}
}

func TestCheckCompoundAndGuardNarrowsOptionalMethodReceiver(t *testing.T) {
	result := Check(`
type Stream = {
    read: (self: Stream, n: number?) -> (string?, string?),
}
type Response = {
    status_code: number,
    body: string?,
    stream: Stream?,
}

local function read_error_body(response: Response): string?
    if response.status_code >= 300 and response.stream and not response.body then
        local body_data = response.stream:read()
        response.body = body_data
    end
    return response.body
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want middle operand of compound and-guard to prove optional receiver present", result.Diagnostics)
	}
}

func TestCheckFluentColonChainKeepsBuilderReceiverPresent(t *testing.T) {
	result := Check(`
type Builder = {
    with_name: (self: Builder, name: string) -> Builder,
    with_context: (self: Builder, context: {any}) -> Builder,
    call: (self: Builder, input: string) -> (string?, string?),
}

local M = {}
function M.new(): Builder
    local builder: Builder
    builder = {
        with_name = function(self: Builder, name: string): Builder
            return self
        end,
        with_context = function(self: Builder, context: {any}): Builder
            return self
        end,
        call = function(self: Builder, input: string): (string?, string?)
            return input, nil
        end,
    }
    return builder
end

local out, err = M.new()
    :with_name("jobs")
    :with_context({})
    :call("run")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want fluent colon chain receivers to use each method's present return type", result.Diagnostics)
	}
}

func TestCheckTypedMethodResultReassignedToSameLocalStaysPresent(t *testing.T) {
	result := Check(`
type FuncExecutor = {
    with_context: (self: FuncExecutor, context: {[string]: any}) -> FuncExecutor,
    call: (self: FuncExecutor, func_id: string, data: any) -> (any, string?),
}

local funcs = {}

function funcs.new(): FuncExecutor
    local executor: FuncExecutor
    executor = {
        with_context = function(self: FuncExecutor, context: {[string]: any}): FuncExecutor
            return self
        end,
        call = function(self: FuncExecutor, func_id: string, data: any): (any, string?)
            return data, nil
        end,
    }
    return executor
end

local function call_func(context: {[string]: any}?)
    local executor = funcs.new() :: FuncExecutor
    if context ~= nil then
        executor = executor:with_context(context)
    end

    return executor:call("id", {})
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed method-call result reassigned to the same local to stay present", result.Diagnostics)
	}
}

func TestCheckConcreteCastFromUntypedMemberCallKeepsMethodReceiverPresent(t *testing.T) {
	result := Check(`
type FuncExecutor = {
    with_context: (self: FuncExecutor, context: {[string]: any}) -> FuncExecutor,
    call: (self: FuncExecutor, func_id: string, data: any) -> (any, string?),
}

local deps: any = {}

local function call_func(context: {[string]: any}?)
    local executor = deps.funcs.new() :: FuncExecutor
    if context ~= nil then
        executor = executor:with_context(context)
    end

    return executor:call("id", {})
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete cast from untyped member call to prove receiver present", result.Diagnostics)
	}
}

func TestCheckDeclaredColonMethodValueReturnStaysPresentAfterErrorCheck(t *testing.T) {
	result := Check(`
type Payload = {
    tool_calls: {string},
}
type Caller = {
    apply: (self: Caller, payload: Payload?) -> (Payload, string?),
    validate: (self: Caller, tool_calls: {string}?) -> ({string}?, string?),
}

local caller: Caller
caller = {
    apply = function(self: Caller, payload: Payload?): (Payload, string?)
        local current: Payload = payload or {
            tool_calls = {},
        }
        return current, nil
    end,
    validate = function(self: Caller, tool_calls: {string}?): ({string}?, string?)
        if not tool_calls then
            return {}, nil
        end
        local wrapped_payload, wrapper_err = self:apply({
            tool_calls = tool_calls,
        })
        if wrapper_err then
            return nil, wrapper_err
        end
        tool_calls = wrapped_payload.tool_calls or tool_calls
        return tool_calls, nil
    end,
}
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want declared colon-method payload return to stay present after error check", result.Diagnostics)
	}
}

func TestCheckModuleTableColonMethodUsesDeclaredReturnInSiblingMethod(t *testing.T) {
	result := Check(`
type Payload = {
    tool_calls: {string},
}

local caller = {}

function caller:apply(payload: Payload?): (Payload, string?)
    local current: Payload = payload or {
        tool_calls = {},
    }
    return current, nil
end

function caller:validate(tool_calls: {string}?): ({string}?, string?)
    if not tool_calls then
        return {}, nil
    end
    local wrapped_payload, wrapper_err = self:apply({
        tool_calls = tool_calls,
    })
    if wrapper_err then
        return nil, wrapper_err
    end
    tool_calls = wrapped_payload.tool_calls or tool_calls
    return tool_calls, nil
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want sibling colon method call to use the callee's declared return type", result.Diagnostics)
	}
}

func TestCheckFluentBuilderMethodsDoNotNeedAnyReceiverCast(t *testing.T) {
	result := Check(`
type ErrorContract = {
    model: string?,
}

type ErrorBuilder = {
    _kind: string?,
    _message: string?,
    _contract: ErrorContract?,
    kind: (self: ErrorBuilder, k: string) -> ErrorBuilder,
    message: (self: ErrorBuilder, m: string) -> ErrorBuilder,
    with_contract: (self: ErrorBuilder, contract_args: ErrorContract) -> ErrorBuilder,
}

local ErrorBuilder = {}
ErrorBuilder.__index = ErrorBuilder

function ErrorBuilder:kind(k: string): ErrorBuilder
    self._kind = k
    return self
end

function ErrorBuilder:message(m: string): ErrorBuilder
    self._message = m
    return self
end

function ErrorBuilder:with_contract(contract_args: ErrorContract): ErrorBuilder
    self._contract = contract_args
    return self
end

local function new_builder(): ErrorBuilder
    local self: ErrorBuilder = setmetatable({
        _kind = nil,
        _message = nil,
        _contract = nil,
    }, ErrorBuilder) :: ErrorBuilder
    return self:kind("invalid"):message("bad"):with_contract({ model = "m" })
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want fluent builder receiver fields to be mutable without erasing self to any", result.Diagnostics)
	}
}

func TestCheckUnannotatedBuilderSelfSetmetatableReturnCastSatisfiesFactory(t *testing.T) {
	result := Check(`
type ErrorContract = {
    model: string?,
}

type ErrorBuilder = {
    _operation: string,
    _contract: ErrorContract?,
    _kind: string?,
    kind: (self: ErrorBuilder, k: string) -> ErrorBuilder,
    build: (self: ErrorBuilder) -> any,
}
type ErrorBuilderFactory = (arg: ErrorContract?) -> ErrorBuilder

local ErrorBuilder = {}
ErrorBuilder.__index = ErrorBuilder

function ErrorBuilder:kind(k: string): ErrorBuilder
    self._kind = k
    return self
end

function ErrorBuilder:build(): any
    return { kind = self._kind }
end

local function builder_factory(operation: string): ErrorBuilderFactory
    return function(arg: ErrorContract?): ErrorBuilder
        local a = arg or {}
        local self = {
            _operation = operation,
            _contract = a,
            _kind = nil,
        }
        return setmetatable(self, ErrorBuilder) :: ErrorBuilder
    end
end

local factory: ErrorBuilderFactory = builder_factory("generate")
local builder: ErrorBuilder = factory({ model = "m" })
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want return cast on unannotated setmetatable builder to satisfy factory", result.Diagnostics)
	}
}

func TestCheckOptionalRecordOrEmptyDefaultPreservesRecordShape(t *testing.T) {
	result := Check(`
type ErrorContract = {
    model: string?,
    _provider_id: string?,
}

local function normalize(arg: ErrorContract?): ErrorContract
    local a = arg or {}
    local out: ErrorContract = a
    return out
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want optional record or empty table default to preserve record shape", result.Diagnostics)
	}
}

func TestCheckReturnedClosureOptionalRecordOrEmptyDefaultPreservesRecordShape(t *testing.T) {
	result := Check(`
type ErrorContract = {
    model: string?,
    _provider_id: string?,
}
type ErrorBuilderFactory = (arg: ErrorContract?) -> ErrorContract

local function builder_factory(): ErrorBuilderFactory
    return function(arg: ErrorContract?): ErrorContract
        local a = arg or {}
        local out: ErrorContract = a
        return out
    end
end

local factory: ErrorBuilderFactory = builder_factory()
local contract: ErrorContract = factory({ model = "m" })
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want returned closure to preserve optional record default shape", result.Diagnostics)
	}
}

func TestCheckReturnedClosureObjectLiteralKeepsDefaultedRecordMemberShape(t *testing.T) {
	result := Check(`
type ErrorContract = {
    model: string?,
    _provider_id: string?,
}
type Holder = {
    _contract: ErrorContract?,
}
type HolderFactory = (arg: ErrorContract?) -> Holder

local function builder_factory(): HolderFactory
    return function(arg: ErrorContract?): Holder
        local a = arg or {}
        local self = {
            _contract = a,
        }
        return self
    end
end

local factory: HolderFactory = builder_factory()
local holder: Holder = factory({ model = "m" })
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want object literal to keep defaulted record member shape in returned closure", result.Diagnostics)
	}
}

func TestCheckReturnedClosureObjectLiteralStillReportsMissingRequiredReturnField(t *testing.T) {
	result := Check(`
type Holder = {
    required: string,
    optional: number?,
}
type HolderFactory = () -> Holder

local function builder_factory(): HolderFactory
    return function(): Holder
        local self = {
            optional = 1,
        }
        return self
    end
end

local factory: HolderFactory = builder_factory()
local holder: Holder = factory()
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		DiagnosticCount: 1,
		MessageContains: []string{"returned value 1", "required"},
	})
}

func TestCheckReturnedClosureSetmetatableStillReportsMissingRequiredReturnField(t *testing.T) {
	result := Check(`
type Holder = {
    required: string,
    optional: number?,
}
type HolderFactory = () -> Holder

local Holder = {}
Holder.__index = Holder

local function builder_factory(): HolderFactory
    return function(): Holder
        local self = {
            optional = 1,
        }
        return setmetatable(self, Holder)
    end
end

local factory: HolderFactory = builder_factory()
local holder: Holder = factory()
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		DiagnosticCount: 1,
		MessageContains: []string{"returned value 1", "required"},
	})
}

func TestCheckReturnedClosureReadsDefaultedRecordField(t *testing.T) {
	result := Check(`
type ErrorContract = {
    model: string?,
    _provider_id: string?,
}
type Reader = (arg: ErrorContract?) -> string?

local function reader_factory(): Reader
    return function(arg: ErrorContract?): string?
        local a = arg or {}
        return a.model
    end
end

local reader: Reader = reader_factory()
local model: string? = reader({ model = "m" })
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want returned closure to read defaulted record field precisely", result.Diagnostics)
	}
}

func TestCheckUnannotatedFunctionReturnSlotsPreserveAnnotatedLocals(t *testing.T) {
	result := Check(`
local function consume(xs: any[]): ()
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}

    for _, entry in ipairs(entries) do
        table.insert(no_suite, entry)
    end

    return suites, no_suite
end

local suites, no_suite = group_by_suite({})
consume(no_suite)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want annotated returned locals to project into caller return slots", result.Diagnostics)
	}
}

func TestCheckUnannotatedFunctionReturnSlotsSurviveLocalSortMutator(t *testing.T) {
	result := Check(`
local function consume(xs: any[]): ()
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        return true
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}

    for _, entry in ipairs(entries) do
        table.insert(no_suite, entry)
    end

    sort_tests(no_suite)
    return suites, no_suite
end

local suites, no_suite = group_by_suite({})
consume(no_suite)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want annotated returned array local to survive local sort mutator", result.Diagnostics)
	}
}

func TestCheckWippyRunnerGroupedSuitesRemainTypedThroughSortAndKeyLoops(t *testing.T) {
	result := Check(`
local function run_suite(name: string, tests: any[]): ()
end

local function entry_suite(entry): string?
    return entry.suite
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        return true
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}

    for _, entry in ipairs(entries) do
        local suite = entry_suite(entry)
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    for _, tests in pairs(suites) do
        sort_tests(tests)
    end
    sort_tests(no_suite)

    return suites, no_suite
end

local function sorted_keys(t)
    local keys: string[] = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local entries: any[] = {}
local suites, no_suite = group_by_suite(entries)
local suite_names = sorted_keys(suites)

for idx, name in ipairs(suite_names) do
    run_suite(name, suites[name])
end

if #no_suite > 0 then
    run_suite("other", no_suite)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want Wippy runner grouped suite arrays to stay typed", result.Diagnostics)
	}
}

func TestCheckWippyRunnerGroupedSuitesRemainTypedThroughRealComparator(t *testing.T) {
	result := Check(`
local function run_suite(name: string, tests: any[]): ()
end

local function entry_id(entry): string?
    local id = entry.id
    if type(id) == "string" then
        return id
    end
    return nil
end

local function entry_suite(entry): string?
    local meta = entry.meta
    if type(meta) ~= "table" then
        return nil
    end
    local suite = meta.suite
    if type(suite) == "string" then
        return suite
    end
    return nil
end

local function entry_order(entry): number
    local meta = entry.meta
    if type(meta) ~= "table" then
        return 0
    end
    local order = meta.order
    if type(order) == "number" then
        return order
    end
    return 0
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        local order_a = entry_order(a)
        local order_b = entry_order(b)
        if order_a ~= order_b then
            return order_a < order_b
        end
        return (entry_id(a) or "") < (entry_id(b) or "")
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}

    for _, entry in ipairs(entries) do
        local suite = entry_suite(entry)
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    for _, tests in pairs(suites) do
        sort_tests(tests)
    end
    sort_tests(no_suite)

    return suites, no_suite
end

local entries: any[] = {}
local suites, no_suite = group_by_suite(entries)

if #no_suite > 0 then
    run_suite("other", no_suite)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want Wippy runner no-suite array to stay typed through the real comparator", result.Diagnostics)
	}
}

func TestCheckWippyRunnerRegistryFindEntriesRemainTypedThroughNoSuiteReturn(t *testing.T) {
	registry := manifest.New("registry")
	registry.SetExport(typ.Unknown)
	entryType := typetable.NewRecord().
		Field("id", typ.String).
		Field("kind", typ.String).
		Field("meta", typ.NewMap(typ.String, typ.Any)).
		Build()
	registry.DefineFunctionSignature("registry.find", signature.Function{
		Type: typ.Func().
			Param("query", typ.Any).
			Returns(typ.NewArray(entryType), normalize.Optional(typ.Any)).
			Build(),
	})

	result := Check(`
local registry = require("registry")

local function run_suite(name: string, tests: any[]): ()
end

local function entry_id(entry): string?
    local id = entry.id
    if type(id) == "string" then
        return id
    end
    return nil
end

local function entry_suite(entry): string?
    local meta = entry.meta
    if type(meta) ~= "table" then
        return nil
    end
    local suite = meta.suite
    if type(suite) == "string" then
        return suite
    end
    return nil
end

local function entry_order(entry): number
    local meta = entry.meta
    if type(meta) ~= "table" then
        return 0
    end
    local order = meta.order
    if type(order) == "number" then
        return order
    end
    return 0
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        local order_a = entry_order(a)
        local order_b = entry_order(b)
        if order_a ~= order_b then
            return order_a < order_b
        end
        return (entry_id(a) or "") < (entry_id(b) or "")
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}

    for _, entry in ipairs(entries) do
        local suite = entry_suite(entry)
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    for _, tests in pairs(suites) do
        sort_tests(tests)
    end
    sort_tests(no_suite)

    return suites, no_suite
end

local function run_tests()
    local entries, err = registry.find({["meta.type"] = "test"})
    if err then
        return 1
    end

    local suites, no_suite = group_by_suite(entries)

    if #no_suite > 0 then
        run_suite("other", no_suite)
    end
    return 0
end

return run_tests()
`, WithStdlib(), WithManifest("registry", registry))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want registry.find entries to preserve no-suite array return", result.Diagnostics)
	}
}

func TestCheckImportedSortedKeysPreservesKeyMembershipForCallerMap(t *testing.T) {
	discovery := CheckAndExport(`
local discovery = {}

function discovery.sorted_keys(t: readonly {[string]: any}): {string}
    local keys: {string} = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

return discovery
`, "discovery", WithStdlib())
	if len(discovery.Errors) != 0 {
		t.Fatalf("discovery diagnostics = %#v", discovery.Errors)
	}
	sig, ok := discovery.Manifest.FunctionSignatures["discovery.sorted_keys"]
	if !ok {
		t.Fatalf("discovery signatures = %#v, want discovery.sorted_keys", discovery.Manifest.FunctionSignatures)
	}
	if sig.OperationalEffects == nil ||
		len(sig.OperationalEffects.DynamicIndexFacts) == 0 ||
		len(sig.OperationalEffects.DynamicValueKeys) != 1 ||
		!sig.OperationalEffects.DynamicValueKeys[0].Container.Equal(pathdom.Path{Root: "ret[0]"}) ||
		!sig.OperationalEffects.DynamicValueKeys[0].Table.Equal(pathdom.NewPlaceholder(0)) {
		t.Fatalf("discovery.sorted_keys operational effects = %#v, want ret[0] values as keys of $0", sig.OperationalEffects)
	}
	hasReturnDynamicIndex := false
	for _, fact := range sig.OperationalEffects.DynamicIndexFacts {
		if fact.Table.Equal(pathdom.Path{Root: "ret[0]"}) {
			hasReturnDynamicIndex = true
			break
		}
	}
	if !hasReturnDynamicIndex {
		t.Fatalf("discovery.sorted_keys dynamic-index facts = %#v, want ret[0] array write fact", sig.OperationalEffects.DynamicIndexFacts)
	}

	result := Check(`
local discovery = require("discovery")

local function consume(tests: {any}): ()
end

local suites: {[string]: {any}} = {}
suites["alpha"] = {}

local suite_names = discovery.sorted_keys(suites)
	for _, name in ipairs(suite_names) do
		consume(suites[name])
	end
`, WithStdlib(), WithModule("discovery", discovery))
	if result.checked == nil || result.checked.RootResult() == nil {
		t.Fatalf("missing checked result")
	}
	foundOutcome := false
	root := result.checked.RootResult()
	for _, point := range root.Graph().RPO() {
		site, ok := root.CallSite(point)
		if !ok {
			continue
		}
		name, ok := root.CallSignatureName(site)
		if !ok || name != "discovery.sorted_keys" {
			continue
		}
		outcome, ok := root.CallOutcomeAt(point)
		if !ok {
			t.Fatalf("missing call outcome for %s at %d", name, point)
		}
		foundOutcome = true
		if len(outcome.NormalReturnFacts.DynamicValueKeys) != 1 ||
			!outcome.NormalReturnFacts.DynamicValueKeys[0].Container.Equal(pathdom.Path{Root: "ret[0]"}) ||
			!outcome.NormalReturnFacts.DynamicValueKeys[0].Table.Equal(pathdom.NewPlaceholder(0)) {
			t.Fatalf("call outcome dynamic value keys = %#v, want ret[0] values as keys of $0", outcome.NormalReturnFacts.DynamicValueKeys)
		}
		hasOutcomeReturnDynamicIndex := false
		for _, fact := range outcome.NormalReturnFacts.DynamicIndexFacts {
			if fact.Table.Equal(pathdom.Path{Root: "ret[0]"}) {
				hasOutcomeReturnDynamicIndex = true
				break
			}
		}
		if !hasOutcomeReturnDynamicIndex {
			t.Fatalf("call outcome dynamic-index facts = %#v, want ret[0] array write fact", outcome.NormalReturnFacts.DynamicIndexFacts)
		}
	}
	if !foundOutcome {
		t.Fatalf("did not find discovery.sorted_keys call outcome")
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported sorted_keys return values to prove membership in caller map", result.Diagnostics)
	}
}

func TestCheckImportedSortedKeysThroughSupertypeCastPreservesKeyMembershipForCallerMap(t *testing.T) {
	discovery := CheckAndExport(`
local discovery = {}

function discovery.sorted_keys(t: readonly {[string]: any}): {string}
    local keys: {string} = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

return discovery
`, "discovery", WithStdlib())
	if len(discovery.Errors) != 0 {
		t.Fatalf("discovery diagnostics = %#v", discovery.Errors)
	}

	result := Check(`
local discovery = require("discovery")

type SuiteGroup = {
    name: string,
    tests: {any},
}

local function consume(group: SuiteGroup): ()
end

local suites: {[string]: {any}} = {}
suites["alpha"] = {}

local suite_names = discovery.sorted_keys(suites :: {[string]: any})
for _, name in ipairs(suite_names) do
    consume({ name = name, tests = suites[name] })
end
`, WithStdlib(), WithModule("discovery", discovery))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want supertype cast argument to preserve sorted_keys membership proof for caller map", result.Diagnostics)
	}
}

func TestCheckDynamicIndexDefaultWritePreservesDeclaredMapValueType(t *testing.T) {
	result := Check(`
local function consume(tests: any[]): ()
end

local suites: {[string]: any[]} = {}
local suite = "alpha"

suites[suite] = suites[suite] or {}
consume(suites[suite])
	`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want dynamic-index default write to preserve declared map value type", result.Diagnostics)
	}
}

func TestCheckPollutedKeyArrayDoesNotProveGroupedMapMembership(t *testing.T) {
	result := Check(`
local function consume(tests: any[]): ()
end

local suites: {[string]: any[]} = {}
suites["alpha"] = {}

local keys: string[] = {}
for k in pairs(suites) do
    table.insert(keys, k)
end
table.insert(keys, "missing")

for _, name in ipairs(keys) do
    consume(suites[name])
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{"suites[name]"},
	})
}

func TestCheckWrongTableKeyArrayDoesNotProveGroupedMapMembership(t *testing.T) {
	result := Check(`
local function consume(tests: any[]): ()
end

local suites: {[string]: any[]} = {}
suites["alpha"] = {}

local other: {[string]: boolean} = {}
other["alpha"] = true

local keys: string[] = {}
for k in pairs(other) do
    table.insert(keys, k)
end

for _, name in ipairs(keys) do
    consume(suites[name])
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{"suites[name]"},
	})
}

func TestCheckUnannotatedFunctionReturnSlotPreservesEmptyAnnotatedArrayLocal(t *testing.T) {
	result := Check(`
local function consume(xs: {any}): ()
end

local function build()
    local xs: {any} = {}
    return xs
end

local xs = build()
consume(xs)
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want empty annotated array local to project into caller return slot", result.Diagnostics)
	}
}

func TestCheckPrototypeMethodAssignmentKeepsUnboundSelfParameter(t *testing.T) {
	result := Check(`
local QueryBuilder = {}
QueryBuilder.__index = QueryBuilder

type QueryOptions = {limit: integer?}

type QueryBuilderInstance = {
    filter: unknown?,
    with_filter: (self: QueryBuilderInstance, filter: unknown) -> QueryBuilderInstance,
    execute: (self: QueryBuilderInstance, options: QueryOptions) -> ({string}, string?),
    run: (self: QueryBuilderInstance, filter: unknown) -> number,
}

function QueryBuilder:with_filter(filter: unknown): QueryBuilderInstance
    self.filter = filter
    return self
end

function QueryBuilder:execute(options: QueryOptions): ({string}, string?)
    return { "row" }, nil
end

function QueryBuilder:run(filter: unknown): number
    local rows, err = self:with_filter(filter):execute({ limit = 5 })
    if err then
        return 0
    end
    return #rows
end

local function new_builder(): QueryBuilderInstance
    local self: QueryBuilderInstance = {
        filter = nil,
        with_filter = QueryBuilder.with_filter,
        execute = QueryBuilder.execute,
        run = QueryBuilder.run,
    }
    setmetatable(self, QueryBuilder)
    return self
end

local builder = new_builder()
local count: number = builder:run({ kind = "active" })
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want dot-read prototype methods assignable to explicit-self instance fields", result.Diagnostics)
	}
}

func TestCheckLuaClassPatternColonMethodKeepsSelfReceiverPresent(t *testing.T) {
	result := Check(`
type Payload = {
    tool_calls: {string},
}

local caller = {}
caller.__index = caller
caller._dependency = nil

function caller.new(): any
    local self = setmetatable({}, caller)
    self.wrapper_context = {}
    self.tool_wrappers = {}
    return self
end

function caller:set_wrapper_context(context: table?): any
    self.wrapper_context = context or {}
    return self
end

function caller:apply(payload: Payload?): (Payload, string?)
    local current: Payload = payload or {
        tool_calls = {},
    }
    current.tool_calls = current.tool_calls or {}
    return current, nil
end

function caller:validate(tool_calls: {string}?): ({string}?, string?)
    if not tool_calls then
        return {}, nil
    end
    local before_payload: Payload = {
        tool_calls = tool_calls,
    }
    local wrapped_payload, wrapper_err = self:apply(before_payload)
    if wrapper_err then
        return nil, wrapper_err
    end
    tool_calls = wrapped_payload.tool_calls or tool_calls
    return tool_calls, nil
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want Lua class-pattern self receiver to stay present in sibling colon method", result.Diagnostics)
	}
}

func TestCheckLuaClassPatternSiblingMethodSurvivesWideReceiverFields(t *testing.T) {
	result := Check(`
type ToolCall = {
    name: string,
}
type HostRef = {
    kind: string,
}
type AgentRef = {
    id: string?,
}
type WrapperContext = {
    host: HostRef?,
    agent: AgentRef?,
    run_context: any?,
}
type WrapperSpec = {
    phases: {string},
    strict: boolean?,
}
type Payload = {
    phase: string,
    host: HostRef?,
    agent: AgentRef?,
    tool_calls: {ToolCall},
    options: table,
    run_context: any?,
}

local PHASE = "before_execute"
local caller = {}
caller.__index = caller
caller._dependency = nil

local function copy_payload(payload: Payload?): Payload
    local out: Payload = payload or {
        phase = PHASE,
        tool_calls = {},
        options = {},
    }
    return out
end

local function reset_wrapper_diagnostics(self: any): ()
    self.wrapper_errors = {}
end

function caller.new(): any
    local self = setmetatable({}, caller)
    self.tool_wrappers = {}
    self.wrapper_context = {}
    self.wrapper_errors = {}
    return self
end

function caller:apply_tool_wrappers(phase: string, payload: Payload?): (Payload, string?)
    local current_payload: Payload = copy_payload(payload)
    current_payload.phase = phase
    current_payload.host = current_payload.host or self.wrapper_context.host
    current_payload.agent = current_payload.agent or self.wrapper_context.agent or {}
    current_payload.run_context = current_payload.run_context or self.wrapper_context.run_context
    current_payload.tool_calls = current_payload.tool_calls or {}
    current_payload.options = current_payload.options or {}

    local wrappers: {WrapperSpec} = self.tool_wrappers or {}
    if #wrappers == 0 then
        return current_payload, nil
    end

    return current_payload, nil
end

function caller:validate(tool_calls: {ToolCall}?): ({ToolCall}?, string?)
    if not tool_calls then
        return {}, nil
    end

    reset_wrapper_diagnostics(self)

    local before_payload: Payload = {
        phase = PHASE,
        host = self.wrapper_context.host,
        agent = self.wrapper_context.agent,
        tool_calls = tool_calls,
        options = {},
    }
    local wrapped_payload, wrapper_err = self:apply_tool_wrappers(PHASE, before_payload)
    if wrapper_err then
        return nil, wrapper_err
    end

    tool_calls = wrapped_payload.tool_calls or tool_calls
    return tool_calls, nil
end
`, WithStdlib())
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeOptionalMethodCall) {
		t.Fatalf("diagnostics = %#v, want wide Lua class-pattern self receiver to keep sibling colon method present", result.Diagnostics)
	}
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeMissingMember) {
		t.Fatalf("diagnostics = %#v, want wide Lua class-pattern self receiver to keep sibling colon method evidence", result.Diagnostics)
	}
}

func TestCheckInferredSetmetatableFactoryReturnCanCallPrototypeMethods(t *testing.T) {
	result := Check(`
local Worker = {}
Worker.__index = Worker

function Worker:prepare(payload: any): (any, string?)
    return { prepared = payload }, nil
end

function Worker:dispatch(payload: any): (boolean, string?)
    local prepared, err = self:prepare(payload)
    if err then
        return false, err
    end
    return prepared ~= nil, nil
end

function Worker:run(payload: any): boolean
    local ok, err = self:dispatch(payload)
    if err then
        return false
    end
    return ok
end

local function new_worker()
    return setmetatable({}, Worker)
end

local function main(): boolean
    local worker = new_worker()
    return worker:run({ task = "sync" }) == true
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inferred setmetatable factory return to expose prototype colon methods", result.Diagnostics)
	}
}

func TestCheckConditionalAssignmentCorrelatesWithLaterSameGuard(t *testing.T) {
	result := Check(`
type Executor = {
    with_context: (self: Executor, context: table) -> Executor,
}

local function make_executor(): Executor
    return {
        with_context = function(self: Executor, context: table): Executor
            return self
        end,
    }
end

local function run(use_template: boolean): ()
    local executor: Executor? = nil
    if not use_template then
        executor = make_executor()
    end

    if use_template then
        return
    else
        executor = executor:with_context({})
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want assignment guarded by not use_template to prove executor present in later else branch", result.Diagnostics)
	}
}

func TestCheckConditionalAssignmentDoesNotProveOppositeGuard(t *testing.T) {
	result := Check(`
type Executor = {
    with_context: (self: Executor, context: table) -> Executor,
}

local function make_executor(): Executor
    return {
        with_context = function(self: Executor, context: table): Executor
            return self
        end,
    }
end

local function run(use_template: boolean): ()
    local executor: Executor? = nil
    if use_template then
        executor = make_executor()
    end

    if use_template then
        return
    else
        local sure: Executor = executor
        sure = sure:with_context({})
    end
end
`, WithStdlib())
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeAssignmentType) {
		t.Fatalf("diagnostics = %#v, want optional executor to remain when assignment guard does not cover later else branch", result.Diagnostics)
	}
}

func TestCheckConditionalAssignmentDoesNotUseAnyFactoryAsPresenceProof(t *testing.T) {
	result := Check(`
type Executor = {
    with_context: (self: Executor, context: table) -> Executor,
}

local function make_executor(): any
    return nil
end

local function run(use_template: boolean): ()
    local executor: Executor? = nil
    if not use_template then
        executor = make_executor()
    end

    if use_template then
        return
    else
        executor = executor:with_context({})
    end
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeOptionalMethodCall,
		MessageContains: []string{"cannot call method on an optional value"},
	})
}

func TestCheckStringParameterColonMatchIsNotOptionalReceiver(t *testing.T) {
	result := Check(`
local function dependency_kind(dep_id: string): string
    if not dep_id:match(":") then
        return "bootloader"
    end
    return "service"
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want string parameter to be a present string method receiver", result.Diagnostics)
	}
}

func TestCheckNestedOptionalFieldGuardProvesRepeatedNestedReads(t *testing.T) {
	result := Check(`
type ProcessResult = {
    value: any,
    error: any?,
}

type Event = {
    result: ProcessResult?,
}

local function finish(event: Event): string
    local result_data = nil
    if event.result then
        result_data = event.result.value

        if event.result.error then
            return tostring(event.result.error)
        elseif type(result_data) == "table" and result_data.success == false then
            return tostring(result_data.error or "Node returned {success=false}")
        else
            return "ok"
        end
    end
    return "ok"
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want event.result guard to prove repeated nested reads present", result.Diagnostics)
	}
}

func TestCheckPairsCollectedKeysProveLaterMapReads(t *testing.T) {
	result := Check(`
type Template = {
    config: table,
}

type Graph = {
    nodes: {[string]: Template},
}

local function run(graph: Graph): ()
    local template_ids = {}
    for template_id, _ in pairs(graph.nodes) do
        table.insert(template_ids, template_id)
    end

    for _, template_id in ipairs(template_ids) do
        local template = graph.nodes[template_id]
        local config: table = template.config
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want keys collected from pairs(graph.nodes) to prove later graph.nodes[key] reads present when the table is not invalidated", result.Diagnostics)
	}
}

func TestCheckSortedCollectedKeysProveLaterMapReads(t *testing.T) {
	result := Check(`
type Template = {
    config: table,
}

type Graph = {
    nodes: {[string]: Template},
}

local function run(graph: Graph): ()
    local template_ids = {}
    for template_id, _ in pairs(graph.nodes) do
        table.insert(template_ids, template_id)
    end
    table.sort(template_ids)

    for _, template_id in ipairs(template_ids) do
        local template = graph.nodes[template_id]
        local config: table = template.config
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want sorting collected keys to preserve later graph.nodes[key] proof", result.Diagnostics)
	}
}

func TestCheckUntypedCollectedKeysProveLaterMapReadPresence(t *testing.T) {
	result := Check(`
local function remap(config: table): table
    return config
end

local function run(template_graph): ()
    local template_ids = {}
    for template_id, _ in pairs(template_graph.nodes) do
        table.insert(template_ids, template_id)
    end
    table.sort(template_ids)

    for _, template_id in ipairs(template_ids) do
        local template = template_graph.nodes[template_id]
        local config = remap(template.config)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want keys from pairs(template_graph.nodes) to prove later read present even when element shape is unknown", result.Diagnostics)
	}
}

func TestCheckConcreteCastAliasMapValidatesAssignment(t *testing.T) {
	result := Check(`
type PluginConfig = {
    prefix: string,
    process_id: string,
    host: string?,
    auto_start: boolean,
}

type UserState = {
    plugins: {[string]: PluginConfig},
}

local function run(args: any): ()
    local plugins = args.plugins :: {[string]: PluginConfig}
    local state: UserState = {
        plugins = plugins,
    }
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete cast map alias to validate assignment", result.Diagnostics)
	}
}

func TestCheckConstructorAssignedFieldsArePresentOnConstructedReceiver(t *testing.T) {
	result := Check(`
type Channel = {
    send: (self: Channel, value: any) -> (),
    case_receive: (self: Channel) -> any,
}

local channel = {}
function channel.new(): Channel
    return {
        send = function(self: Channel, value: any) end,
        case_receive = function(self: Channel): any return {} end,
    }
end

local Bus = {
    ops_channel = nil :: any,
    stop_signal = nil :: any,
}
Bus.__index = Bus

function Bus.new()
    local self = setmetatable({}, Bus)
    self.ops_channel = channel.new()
    self.stop_signal = channel.new()
    return self
end

function Bus:queue_op(op: any): ()
    self.ops_channel:send(op)
end

function Bus:stop(): ()
    self.stop_signal:send(true)
end

function Bus:run_once(): ()
    local cases = {
        self.stop_signal:case_receive(),
        self.ops_channel:case_receive(),
    }
end

local bus = Bus.new()
bus:queue_op({})
bus:stop()
bus:run_once()
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want constructor-assigned receiver fields to be present on constructed values", result.Diagnostics)
	}
}

func TestCheckConstructorAssignedManifestGlobalFieldsArePresentOnConstructedReceiver(t *testing.T) {
	channelValue := typetable.NewRecord().
		Field("send", typ.Func().Param("self", typ.Self).Param("value", typ.Any).Build()).
		Field("case_receive", typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()).
		Build()
	channelManifest := manifest.New("channel")
	channelManifest.SetExport(typetable.NewRecord().
		Field("new", typ.Func().OptParam("size", typ.Number).Returns(channelValue).Build()).
		Build())

	result := Check(`
local Bus = {
    ops_channel = nil :: any,
    stop_signal = nil :: any,
}
Bus.__index = Bus

function Bus.new()
    local self = setmetatable({}, Bus)
    self.ops_channel = channel.new(256)
    self.stop_signal = channel.new(1)
    return self
end

function Bus:queue_op(op: any): ()
    self.ops_channel:send(op)
end

function Bus:stop(): ()
    self.stop_signal:send(true)
end

function Bus:run_once(): ()
    local cases = {
        self.stop_signal:case_receive(),
        self.ops_channel:case_receive(),
    }
end

local bus = Bus.new()
bus:queue_op({})
bus:stop()
bus:run_once()
`, WithStdlib(), WithManifest("channel", channelManifest), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want constructor-assigned manifest-global fields to be present on constructed values", result.Diagnostics)
	}
}

func TestCheckConstructorBranchOnlyFieldsRemainOptional(t *testing.T) {
	result := Check(`
type Resource = {
    close: (self: Resource) -> (),
}

local function open_resource(): Resource
    return {
        close = function(self: Resource) end,
    }
end

local Holder = {
    resource = nil :: any,
}
Holder.__index = Holder

function Holder.new(flag: boolean)
    local self = setmetatable({}, Holder)
    if flag then
        self.resource = open_resource()
    end
    return self
end

function Holder:close(): ()
    self.resource:close()
end

local holder = Holder.new(false)
holder:close()
	`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeOptionalMethodCall,
		DiagnosticCount: 1,
		MessageContains: []string{"cannot call method on an optional value"},
	})
}

func TestCheckSelfIndexPrototypeWithDataFieldsCanCallSiblingMethods(t *testing.T) {
	channelValue := typetable.NewRecord().
		Field("send", typ.Func().Param("self", typ.Self).Param("value", typ.Any).Build()).
		Field("case_receive", typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()).
		Build()
	selectResult := typetable.NewRecord().
		Field("ok", typ.Boolean).
		Field("channel", channelValue).
		Field("value", typ.Any).
		Build()
	channelManifest := manifest.New("channel")
	channelManifest.SetExport(typetable.NewRecord().
		Field("new", typ.Func().OptParam("size", typ.Integer).Returns(channelValue).Build()).
		Field("select", typ.Func().Param("cases", typ.NewArray(typ.Any)).Returns(selectResult).Build()).
		Build())

	result := Check(`
local Bus = {
    context = nil :: any,
    ops_channel = nil :: any,
    stop_signal = nil :: any,
    stopping = false,
    finishing = false,
    handlers = {} :: {[string]: (any, any) -> (any, string?)},
}
Bus.__index = Bus

function Bus.new(context: any)
    local self = setmetatable({}, Bus)
    self.context = context
    self.ops_channel = channel.new(256)
    self.stop_signal = channel.new(1)
    self.stopping = false
    self.finishing = false
    self.handlers = {} :: {[string]: (any, any) -> (any, string?)}
    return self
end

function Bus:stop(): ()
    self.stopping = true
    self.stop_signal:send(true)
end

function Bus:is_fatal_error(err: string?): boolean
    return err == "fatal"
end

function Bus:process_operation(op: { type: string }): (any, string?)
    local handler = self.handlers[op.type]
    if not handler then
        return nil, "missing"
    end
    return handler(self.context, op)
end

function Bus:run_once(op: { type: string }): (boolean, string?)
    while not self.stopping do
        local result = channel.select({
            self.stop_signal:case_receive(),
            self.ops_channel:case_receive(),
        })
        if not result.ok then
            break
        end
        if result.channel == self.stop_signal then
            self.stopping = true
        elseif result.channel == self.ops_channel then
            local _, err = self:process_operation(op)
            if err and self:is_fatal_error(err) then
                return false, err
            end
            if self.finishing then
                self:stop()
            end
        end
    end
    return true, nil
end
`, WithStdlib(), WithManifest("channel", channelManifest), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want self-index prototype with data fields to expose sibling methods", result.Diagnostics)
	}
}

func TestCheckFunctionDefinitionAssertRefinesOptionalLocal(t *testing.T) {
	result := Check(`
local assert = {}
function assert.not_nil(value: any, msg: string?)
    if value == nil then
        error(msg or "expected value")
    end
end

local value: string? = "ok"
assert.not_nil(value)
local narrowed: string = value
`, WithStdlib())
	assertRootLocalAssignmentSourceType(t, result, "narrowed", typ.String)
}

func assertRootLocalAssignmentSourceType(t *testing.T, result Result, name string, want typ.Type) {
	t.Helper()
	if result.checked != nil && result.checked.RootResult() != nil && result.checked.RootResult().Graph() != nil {
		root := result.checked.RootResult()
		for _, point := range root.Graph().RPO() {
			fact, ok := root.LocalAssignment(point)
			if !ok || fact.Name != name {
				continue
			}
			if value, ok := root.LocalAssignmentSourceValueAtBoundary(point, fact.Source); ok {
				ty, tyOK := typevalue.TypeOf(root.Registry(), value)
				if !tyOK || !typ.TypeEquals(ty, want) {
					t.Fatalf("%s source type = %v/%v, want %v", name, ty, tyOK, want)
				}
				return
			} else {
				t.Fatalf("%s source read missing", name)
			}
		}
	}
	t.Fatalf("local assignment %q not found", name)
}

func TestCheckImportedFunctionDefinitionAssertReturningValueRefinesOptionalLocal(t *testing.T) {
	assertMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected value")
    end
    return value
end

return test
`, "test", WithStdlib())
	if len(assertMod.Errors) != 0 {
		t.Fatalf("assert module diagnostics = %#v, want clean helper export", assertMod.Errors)
	}

	result := Check(`
local test = require("test")

local value: string? = "ok"
test.not_nil(value)
local narrowed: string = value
`, WithStdlib(), WithModule("test", assertMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported returning assert to refine optional local", result.Diagnostics)
	}
}

func TestCheckImportedAssertedAnyReturnStaysPresentForMethodCall(t *testing.T) {
	testMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected value")
    end
    return value
end

return test
	`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want clean helper export", testMod.Errors)
	}
	contractType := typ.NewInterface("contract.Contract", nil)
	openType := typ.Func().
		Param("self", typ.Self).
		Param("id", typ.String).
		Returns(typ.Any, typeexpr.Optional(typ.String)).
		Build()
	contractType.Methods = []typ.Method{
		{
			Name: "with_actor",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("actor", typ.Any).
				Returns(typ.Self).
				Build(),
		},
		{
			Name: "with_scope",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("scope", typ.Any).
				Returns(typ.Self).
				Build(),
		},
		{Name: "open", Type: openType},
	}
	getType := typ.Func().
		Param("id", typ.String).
		Returns(contractType, typeexpr.Optional(typ.String)).
		Build()
	contractMod := manifest.New("contract")
	contractMod.DefineType("Contract", contractType)
	contractMod.SetExport(typetable.NewRecord().
		Field("get", getType).
		Build())
	contractMod.DefineFunctionSignature("get", signature.Function{Type: getType})

	result := Check(`
local test = require("test")
local contract = require("contract")

local function open_binding()
    local def, def_err = contract.get("app.contract")
    test.not_nil(def_err == nil, "contract.get")
    test.not_nil(def, "contract expected")
    local instance, open_err = def
        :with_actor({})
        :with_scope({})
        :open("app.binding")
    test.not_nil(open_err == nil, "contract.open")
    test.not_nil(instance, "binding expected")
    return instance
end

local result, err = open_binding():get_context({ host = { kind = "session" } })
		`, WithStdlib(), WithModule("test", testMod), WithManifest("contract", contractMod))
	assertNestedLocalPresentAtReturn(t, result, "instance")
	assertMethodReceiverCallResultPresent(t, result, "get_context")
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported assertion helper to prove any-returned binding is present", result.Diagnostics)
	}
}

func assertNestedLocalPresentAtReturn(t *testing.T, result Result, name string) {
	t.Helper()
	if result.checked == nil || result.checked.RootResult() == nil {
		t.Fatalf("missing checked root result")
	}
	root := result.checked.RootResult()
	var sawLocal bool
	var lastDetail string
	for _, child := range root.FunctionResults() {
		if child == nil || child.Graph() == nil {
			continue
		}
		var localPath pathdom.Path
		for _, point := range child.Graph().RPO() {
			fact, ok := child.LocalAssignment(point)
			if ok && fact.Name == name && fact.HasSymbol && fact.Symbol != 0 {
				localPath = pathdom.NewPath(fact.Symbol, name)
				break
			}
		}
		if localPath.IsEmpty() {
			continue
		}
		sawLocal = true
		for _, point := range child.ReturnPoints() {
			value, ok := child.PathValueAtBoundary(point, localPath)
			if !ok {
				lastDetail = fmt.Sprintf("%s missing at return boundary", name)
				continue
			}
			if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
				gotType, typeOK := typevalue.TypeOf(child.Registry(), value)
				lastDetail = fmt.Sprintf("%s presence at return = %s type=%v/%v value=%v, want present", name, got, gotType, typeOK, value)
				continue
			}
			return
		}
	}
	if !sawLocal {
		t.Fatalf("nested local %s not found", name)
	}
	t.Fatalf("%s", lastDetail)
}

func assertMethodReceiverCallResultPresent(t *testing.T, result Result, method string) {
	t.Helper()
	if result.checked == nil || result.checked.RootResult() == nil {
		t.Fatalf("missing checked root result")
	}
	root := result.checked.RootResult()
	graph := root.Graph()
	if graph == nil {
		t.Fatalf("missing root graph")
	}
	for _, point := range graph.RPO() {
		fact, ok := root.Call(point)
		if !ok || fact.Call == nil || fact.Call.Method != method {
			continue
		}
		receiverCall, ok := fact.Call.Receiver.(*ast.FuncCallExpr)
		if !ok {
			t.Fatalf("%s receiver = %T, want function call", method, fact.Call.Receiver)
		}
		value, ok := root.CallExprResultValue(receiverCall, 0)
		if !ok {
			t.Fatalf("%s receiver call result missing", method)
		}
		if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
			gotType, typeOK := typevalue.TypeOf(root.Registry(), value)
			t.Fatalf("%s receiver call result presence = %s type=%v/%v value=%v, want present", method, got, gotType, typeOK, value)
		}
		return
	}
	t.Fatalf("method call %s not found", method)
}

func TestCheckAssertedHelperReturnStaysNonNilForImmediateMethodCall(t *testing.T) {
	result := Check(`
type Binding = {
    get_context: (self: Binding, args: table) -> (table?, string?),
}

type ContractDef = {
    with_actor: (self: ContractDef, actor: table) -> ContractDef,
    with_scope: (self: ContractDef, scope: table) -> ContractDef,
    open: (self: ContractDef, id: string) -> (Binding?, string?),
}

local test = {}
function test.not_nil(value: any, msg: string?)
    if value == nil then
        error(msg or "expected value")
    end
end

local contract = {} :: {
    get: (id: string) -> (ContractDef?, string?),
}

local function open_binding()
    local def, def_err = contract.get("app.contract")
    test.not_nil(def, "definition expected")

    local instance, open_err = def
        :with_actor({})
        :with_scope({})
        :open("app.binding")
    test.not_nil(instance, "binding expected")
    return instance
end

local result, err = open_binding():get_context({ host = { kind = "session" } })
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want asserted helper return to stay non-nil for immediate method call", result.Diagnostics)
	}
}

func TestCheckAssertedHelperReturnStaysNonNilInsideCallback(t *testing.T) {
	result := Check(`
type Binding = {
    get_context: (self: Binding, args: table) -> (table?, string?),
}

type ContractDef = {
    with_actor: (self: ContractDef, actor: table) -> ContractDef,
    with_scope: (self: ContractDef, scope: table) -> ContractDef,
    open: (self: ContractDef, id: string) -> (Binding?, string?),
}

local test = {}
function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected value")
    end
    return value
end
function test.it(name: string, body: () -> ())
    body()
end

local contract = {} :: {
    get: (id: string) -> (ContractDef?, string?),
}

local function open_binding()
    local def, def_err = contract.get("app.contract")
    test.not_nil(def_err == nil, "contract.get")
    test.not_nil(def, "definition expected")

    local instance, open_err = def
        :with_actor({})
        :with_scope({})
        :open("app.binding")
    test.not_nil(open_err == nil, "contract.open")
    test.not_nil(instance, "binding expected")
    return instance
end

test.it("uses asserted helper return in callback", function()
    local result, err = open_binding():get_context({ host = { kind = "session" } })
    test.not_nil(result)
end)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want asserted helper return to stay non-nil inside callback", result.Diagnostics)
	}
}

func TestCheckImportedAssertedHelperReturnStaysNonNilForImmediateLocalTypeMethodCall(t *testing.T) {
	testMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected value")
    end
    return value
end

return test
`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want clean helper export", testMod.Errors)
	}

	result := Check(`
local test = require("test")

type Binding = {
    get_context: (self: Binding, args: table) -> (table?, string?),
}

type ContractDef = {
    with_actor: (self: ContractDef, actor: table) -> ContractDef,
    with_scope: (self: ContractDef, scope: table) -> ContractDef,
    open: (self: ContractDef, id: string) -> (Binding?, string?),
}

local contract = {} :: {
    get: (id: string) -> (ContractDef?, string?),
}

local function open_binding()
    local def, def_err = contract.get("app.contract")
    test.not_nil(def_err == nil, "contract.get")
    test.not_nil(def, "definition expected")

    local instance, open_err = def
        :with_actor({})
        :with_scope({})
        :open("app.binding")
    test.not_nil(open_err == nil, "contract.open")
    test.not_nil(instance, "binding expected")
    return instance
end

local result, err = open_binding():get_context({ host = { kind = "session" } })
`, WithStdlib(), WithModule("test", testMod))
	assertNestedLocalPresentAtReturn(t, result, "instance")
	assertMethodReceiverCallResultPresent(t, result, "get_context")
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported asserted helper return to stay non-nil for immediate local-type method call", result.Diagnostics)
	}
}

func TestCheckImportedAssertedHelperReturnStaysNonNilInsideCallback(t *testing.T) {
	testMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected value")
    end
    return value
end

function test.it(name: string, body: () -> ())
    body()
end

return test
`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want clean helper export", testMod.Errors)
	}
	notNilSig, ok := testMod.Manifest.FunctionSignatures["test.not_nil"]
	if !ok || notNilSig.OperationalEffects == nil {
		t.Fatalf("test.not_nil signature = %#v/%v, want operational effects", notNilSig, ok)
	}
	var exportedNonNil bool
	for _, refinement := range notNilSig.OperationalEffects.NormalReturnPresenceRefinements {
		if refinement.Path.Equal(pathdom.NewPlaceholder(0)) && presence.Equal(refinement.Presence, presence.Present()) {
			exportedNonNil = true
		}
	}
	if !exportedNonNil {
		t.Fatalf("test.not_nil operational presence refinements = %#v, want $0 present", notNilSig.OperationalEffects.NormalReturnPresenceRefinements)
	}

	result := Check(`
local test = require("test")

type Binding = {
    get_context: (self: Binding, args: table) -> (table?, string?),
}

type ContractDef = {
    with_actor: (self: ContractDef, actor: table) -> ContractDef,
    with_scope: (self: ContractDef, scope: table) -> ContractDef,
    open: (self: ContractDef, id: string) -> (Binding?, string?),
}

local contract = {} :: {
    get: (id: string) -> (ContractDef?, string?),
}

local function open_binding()
    local def, def_err = contract.get("app.contract")
    test.not_nil(def_err == nil, "contract.get")
    test.not_nil(def, "definition expected")

    local instance, open_err = def
        :with_actor({})
        :with_scope({})
        :open("app.binding")
    test.not_nil(open_err == nil, "contract.open")
    test.not_nil(instance, "binding expected")
    return instance
end

	test.it("uses asserted imported helper return in callback", function()
	    local result, err = open_binding():get_context({ host = { kind = "session" } })
	    test.not_nil(result)
	end)
	`, WithStdlib(), WithModule("test", testMod))
	assertNestedLocalPresentAtReturn(t, result, "instance")
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported asserted helper return to stay non-nil inside callback", result.Diagnostics)
	}
}

func TestCheckImportedSignatureAcceptsCapturedModuleStringConstant(t *testing.T) {
	funcs := manifest.New("funcs")
	funcs.SetExport(typ.Unknown)
	funcs.DefineFunctionSignature("funcs.call", signature.Function{
		Type: typ.Func().
			Param("id", typ.String).
			Returns(typ.Any, normalize.Optional(typ.String)).
			Build(),
	})
	result := Check(`
local funcs = require("funcs")

local base_id = "app.test.registry:base"
local wrapper_id = "app.test.registry:wrapper"
local func_id = "app.test.registry:func"

local function wait_call(expected: string): ()
    local result, call_err = funcs.call(func_id)
    if call_err then
        return
    end
    if result then
        funcs.call(wrapper_id)
        funcs.call(base_id)
    end
end

local function main(): boolean
    wait_call("ok")
    funcs.call(func_id)
    funcs.call(wrapper_id)
    funcs.call(base_id)
    return true
end
`, WithStdlib(), WithManifest("funcs", funcs))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured module string constants to satisfy imported string parameters", result.Diagnostics)
	}
}

func TestCheckUnannotatedHelperAcceptsCapturedModuleStringConstant(t *testing.T) {
	registry := manifest.New("registry")
	registry.SetExport(typ.Unknown)
	registry.DefineFunctionSignature("registry.get", signature.Function{
		Type: typ.Func().
			Param("id", typ.String).
			Returns(typ.Any, normalize.Optional(typ.String)).
			Build(),
	})
	result := Check(`
local registry = require("registry")

local base_id = "app.test.registry:base"
local wrapper_id = "app.test.registry:wrapper"
local func_id = "app.test.registry:func"

local function assert_absent(id)
    local entry, err = registry.get(id)
    if entry ~= nil then
        return false
    end
    if err ~= nil then
        local msg = id .. " should not exist"
        return msg ~= ""
    end
    return false
end

local function main(): boolean
    assert_absent(func_id)
    assert_absent(wrapper_id)
    assert_absent(base_id)
    assert_absent(func_id)
    assert_absent(wrapper_id)
    assert_absent(base_id)
    return true
end
`, WithStdlib(), WithManifest("registry", registry))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unannotated helper calls to keep captured string constants precise", result.Diagnostics)
	}
}

func TestCheckReassignedModuleStringConstantDoesNotUseStaleInitializer(t *testing.T) {
	registry := manifest.New("registry")
	registry.SetExport(typ.Unknown)
	registry.DefineFunctionSignature("registry.get", signature.Function{
		Type: typ.Func().
			Param("id", typ.String).
			Returns(typ.Any, normalize.Optional(typ.String)).
			Build(),
	})
	result := Check(`
local registry = require("registry")

local func_id = "app.test.registry:func"

local function main(): boolean
    func_id = 42
    local entry, err = registry.get(func_id)
    return entry == nil and err == nil
end
`, WithStdlib(), WithManifest("registry", registry))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{"argument 1 (func_id)", "42", "not string"},
	})
}

func TestCheckStdlibSelectCountDoesNotUseChannelSelectShape(t *testing.T) {
	result := Check(`
local function count(...: any): number
    return select("#", ...)
end

local function main(): boolean
    local c: number = count(1, 2, 3)
    return c == 3
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want stdlib select count to type as number without channel-select shape", result.Diagnostics)
	}
}

func TestCheckNestedGenericUnionPayloadAcceptsArrayLiteralElement(t *testing.T) {
	result := Check(`
type Envelope<T> = {
    payload: T,
}
type RenderPayload = {
    kind: "render",
    template: string,
}
type IndexPayload = {
    kind: "index",
    terms: {string},
}
type Payload = RenderPayload | IndexPayload
type DispatchRequest = {
    kind: "dispatch",
    envelope: Envelope<Payload>,
}

local request: DispatchRequest = {
    kind = "dispatch",
    envelope = {
        payload = {
            kind = "index",
            terms = {"lua", "types", "cache"},
        },
    },
}
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nested discriminated payload to accept homogeneous array literal", result.Diagnostics)
	}
}

func TestCheckFreshRecordLiteralAssignableToBuiltinTableTop(t *testing.T) {
	result := Check(`
local command: table = {
    type = "create",
    payload = {
        id = "one",
        tags = {"fast", "safe"},
    },
}
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want record literal to satisfy builtin table top type", result.Diagnostics)
	}
}

func TestCheckInlineCallbackReturnAnnotationKeepsDiscriminatedUnion(t *testing.T) {
	result := Check(`
type AppError = {
    code: string,
    message: string,
}
type Response = {
    status: integer,
    body: string,
}
type ResponseResult = {ok: true, value: Response} | {ok: false, error: AppError}
type Handler = () -> ResponseResult
type Builder = {
    handle: (self: Builder, handler: Handler) -> Builder,
}

local builder: Builder
builder = {
    handle = function(self: Builder, handler: Handler): Builder
        return self
    end,
}

builder:handle(function(): ResponseResult
    if false then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "missing",
            },
        }
    end
    return {
        ok = true,
        value = {
            status = 200,
            body = "ok",
        },
    }
end)
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want callback return annotation to keep both result union arms", result.Diagnostics)
	}
}

func TestCheckImportedCallbackReturnAnnotationKeepsDiscriminatedUnion(t *testing.T) {
	protocol := CheckAndExport(`
type AppError = {
    code: string,
    message: string,
}
type Response = {
    status: integer,
    body: string,
}
type ResponseResult = {ok: true, value: Response} | {ok: false, error: AppError}

local M = {}
M.ResponseResult = ResponseResult
return M
`, "protocol")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v", protocol.Errors)
	}

	result := Check(`
local protocol = require("protocol")

type Handler = () -> protocol.ResponseResult
type Builder = {
    handle: (self: Builder, handler: Handler) -> Builder,
}

local builder: Builder
builder = {
    handle = function(self: Builder, handler: Handler): Builder
        return self
    end,
}

builder:handle(function(): protocol.ResponseResult
    if false then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "missing",
            },
        }
    end
    return {
        ok = true,
        value = {
            status = 200,
            body = "ok",
        },
    }
end)
`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported callback return annotation to keep both result union arms", result.Diagnostics)
	}
}

func TestCheckCallbackUnionReturnAfterOptionalIndexGuard(t *testing.T) {
	protocol := CheckAndExport(`
type AppError = {
    code: string,
    message: string,
}
type RequestContext = {
    params: {[string]: string},
}
type Response = {
    status: integer,
    body: string,
}
type ResponseResult = {ok: true, value: Response} | {ok: false, error: AppError}

local M = {}
M.RequestContext = RequestContext
M.ResponseResult = ResponseResult
return M
`, "protocol")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v", protocol.Errors)
	}

	result := Check(`
local protocol = require("protocol")

local handler = function(ctx: protocol.RequestContext): protocol.ResponseResult
    local room_id = ctx.params["room_id"]
    if not room_id then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "missing",
            },
        }
    end
    return {
        ok = true,
        value = {
            status = 200,
            body = "room:" .. room_id,
        },
    }
end
`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want optional-index guard not to narrow declared return union", result.Diagnostics)
	}
}

func TestCheckImportedFluentCallbackReturnAnnotationKeepsUnion(t *testing.T) {
	resultMod := CheckAndExport(`
type ErrorCode = "not_found" | "invalid"
type AppError = {
    code: ErrorCode,
    message: string,
    retryable: boolean,
}

local M = {}
M.ErrorCode = ErrorCode
M.AppError = AppError
return M
`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result diagnostics = %#v", resultMod.Errors)
	}

	protocol := CheckAndExport(`
local result = require("result")
local time = require("time")

type AppError = result.AppError
type SessionSnapshot = {
    id: string,
    user_id: string,
    scopes: {[string]: boolean},
    last_seen: time.Time?,
    attributes: {[string]: string}?,
}
type RequestMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}
type HttpRequest = {
    kind: "http",
    method: "GET" | "POST",
    path: string,
    headers: {[string]: string},
    params: {[string]: string}?,
    body: string?,
    meta: RequestMeta,
}
type RequestContext = {
    request: HttpRequest,
    params: {[string]: string},
    locals: {[string]: string},
    session: SessionSnapshot?,
}
type Response = {
    status: integer,
    body: string,
    headers: {[string]: string},
}
type ResponseResult = {ok: true, value: Response} | {ok: false, error: AppError}
type RouteHandler = (RequestContext) -> ResponseResult
type BodyDecorator = (string, RequestContext) -> string
type Route = {
    handle: RouteHandler,
}

local M = {}
M.RequestContext = RequestContext
M.ResponseResult = ResponseResult
M.RouteHandler = RouteHandler
M.BodyDecorator = BodyDecorator
M.Route = Route
return M
`, "protocol", WithStdlib(), WithModule("result", resultMod), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v", protocol.Errors)
	}
	sessionType, ok := protocol.Manifest.Types["SessionSnapshot"].(*typ.Record)
	if !ok {
		t.Fatalf("exported SessionSnapshot = %T %[1]v, want record", protocol.Manifest.Types["SessionSnapshot"])
	}
	userIDField := sessionType.GetField("user_id")
	if userIDField == nil || userIDField.Optional || !typ.TypeEquals(userIDField.Type, typ.String) {
		t.Fatalf("exported SessionSnapshot.user_id = %#v, want required string", userIDField)
	}

	direct := Check(`
local protocol = require("protocol")

local handler = function(ctx: protocol.RequestContext): protocol.ResponseResult
    local room_id = ctx.params["room_id"]
    if not room_id then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "missing",
                retryable = false,
            },
        }
    end
    local user_id = "guest"
    if ctx.session then
        user_id = ctx.session.user_id
    end
    return {
        ok = true,
        value = {
            status = 200,
            body = "room:" .. room_id .. ":" .. user_id,
            headers = {["x-user"] = user_id},
        },
    }
end
`, WithStdlib(), WithModule("result", resultMod), WithModule("protocol", protocol), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(direct.Diagnostics) != 0 {
		t.Fatalf("direct diagnostics = %#v, want imported handler optional session guard to keep required sibling field", direct.Diagnostics)
	}

	builder := CheckAndExport(`
local protocol = require("protocol")

type Builder = {
    decorate_body: (self: Builder, decorator: protocol.BodyDecorator) -> Builder,
    handle: (self: Builder, handler: protocol.RouteHandler) -> Builder,
    build: (self: Builder) -> protocol.Route,
    handler: protocol.RouteHandler,
    decorator: protocol.BodyDecorator?,
}

local Builder = {}
function Builder:decorate_body(decorator: protocol.BodyDecorator): Builder
    self.decorator = decorator
    return self
end
function Builder:handle(handler: protocol.RouteHandler): Builder
    self.handler = handler
    return self
end
function Builder:build(): protocol.Route
    local handler = self.handler
    local decorator = self.decorator
    return {
        handle = function(ctx: protocol.RequestContext): protocol.ResponseResult
            local response_result = handler(ctx)
            if not response_result.ok then
                return response_result
            end
            if decorator then
                local response = response_result.value
                return {
                    ok = true,
                    value = {
                        status = response.status,
                        body = decorator(response.body, ctx),
                        headers = response.headers,
                    },
                }
            end
            return response_result
        end,
    }
end

local M = {}
function M.new(): Builder
    local self: Builder = {
        decorate_body = Builder.decorate_body,
        handle = Builder.handle,
        build = Builder.build,
        handler = function(ctx: protocol.RequestContext): protocol.ResponseResult
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = "missing",
                    retryable = false,
                },
            }
        end,
        decorator = nil,
    }
    return self
end
return M
`, "builder", WithStdlib(), WithModule("protocol", protocol), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(builder.Errors) != 0 {
		t.Fatalf("builder diagnostics = %#v", builder.Errors)
	}

	result := Check(`
local protocol = require("protocol")
local builder = require("builder")

builder.new()
    :decorate_body(function(body: string, ctx: protocol.RequestContext): string
        local source = ctx.locals["source"]
        if source then
            return body .. ":" .. source
        end
        return body
    end)
    :handle(function(ctx: protocol.RequestContext): protocol.ResponseResult
        local room_id = ctx.params["room_id"]
        if not room_id then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = "missing",
                    retryable = false,
                },
            }
        end
        local user_id = "guest"
        local freshness = "cold"
        if ctx.session then
            user_id = ctx.session.user_id
            if ctx.session.last_seen then
                freshness = "warm"
            end
        end
        return {
            ok = true,
            value = {
                status = 200,
                body = "room:" .. room_id .. ":" .. user_id .. ":" .. freshness,
                headers = {["x-user"] = user_id},
            },
        }
    end)
    :build()
`, WithStdlib(), WithModule("result", resultMod), WithModule("protocol", protocol), WithModule("builder", builder), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported fluent callback return annotation to keep union", result.Diagnostics)
	}
}

func TestCheckImportedFunctionTypedRecordFieldProjectsCallableType(t *testing.T) {
	resultMod := CheckAndExport(`
type AppError = {
    code: "invalid" | "not_found",
    message: string,
    retryable: boolean,
}
local M = {}
M.AppError = AppError
return M
`, "result", WithStdlib())
	protocol := CheckAndExport(`
local result = require("result")
type RequestContext = { locals: {[string]: string} }
type Response = { status: integer, body: string, headers: {[string]: string} }
type AppError = result.AppError
type ResponseResult = {ok: true, value: Response} | {ok: false, error: AppError}
type RouteHandler = (RequestContext) -> ResponseResult
local M = {}
M.RequestContext = RequestContext
M.Response = Response
M.ResponseResult = ResponseResult
M.RouteHandler = RouteHandler
return M
`, "protocol", WithStdlib(), WithModule("result", resultMod))
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v", protocol.Errors)
	}
	result := Check(`
local protocol = require("protocol")
type Builder = {
    handler: protocol.RouteHandler,
}

local function selected(self: Builder): protocol.RouteHandler
    local handler = self.handler
    return handler
end
`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported function-typed record field to project callable type", result.Diagnostics)
	}
}

func TestCheckOptionalRecordGuardKeepsRequiredSiblingFieldPresent(t *testing.T) {
	result := Check(`
type Time = {
    unix: (self: Time) -> integer,
}
type SessionSnapshot = {
    id: string,
    user_id: string,
    scopes: {[string]: boolean},
    last_seen: Time?,
    attributes: {[string]: string}?,
}
type RequestContext = {
    session: SessionSnapshot?,
}

local function selected_user(ctx: RequestContext): string
    local user_id = "guest"
    if ctx.session then
        user_id = ctx.session.user_id
    end
    local headers: {[string]: string} = {["x-user"] = user_id}
    return user_id
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want optional record guard to keep required sibling field non-nil", result.Diagnostics)
	}
}

func TestCheckObjectLiteralUnionArmAllowsWidthFields(t *testing.T) {
	result := Check(`
type Success = {
    ok: true,
}
type Failure = {
    ok: false,
    error: string,
}
type Result = Failure | Success

local r: Result = {
    ok = true,
    value = "extra payload kept by width subtyping",
}
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want extra object-literal fields not to reject matching union arm", result.Diagnostics)
	}
}

func TestCheckObjectLiteralUnionMismatchPrefersMatchingDiscriminantArm(t *testing.T) {
	result := Check(`
type Success = {
    ok: true,
    value: string,
}
type Failure = {
    ok: false,
    error: string,
}
type Result = Success | Failure

local r: Result = {
    ok = true,
    value = 123,
}
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one value mismatch", result.Diagnostics)
	}
	got := result.Diagnostics[0].Message
	if !strings.Contains(got, "value") || strings.Contains(got, "true, not false") {
		t.Fatalf("message = %q, want matching-arm value mismatch, not contradictory false-arm mismatch", got)
	}
}

func TestCheckNestedObjectLiteralUnionMismatchKeepsChildPath(t *testing.T) {
	result := Check(`
type Response = {
    body: string,
}
type Success = {
    ok: true,
    value: Response,
}
type Failure = {
    ok: false,
    error: string,
}
type Result = Success | Failure

local maybe: string? = nil
local r: Result = {
    ok = true,
    value = {
        body = maybe,
    },
}
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one nested body mismatch", result.Diagnostics)
	}
	got := result.Diagnostics[0].Message
	if !strings.Contains(got, "body") || strings.Contains(got, "to {body: string}") {
		t.Fatalf("message = %q, want nested value.body mismatch rather than whole value table mismatch", got)
	}
}

func TestCheckAnyMethodPresenceGuardDoesNotInventCallableProof(t *testing.T) {
	result := Check(`
local function format_value(val: any): string
    if type(val) == "table" then
        if val._tostring then
            return val:_tostring()
        end
    end
    return ""
end
`, WithStdlib())
	if len(result.Diagnostics) == 0 {
		t.Fatalf("diagnostics = %#v, want untrusted any member call rejected without function proof", result.Diagnostics)
	}
}

func TestCheckForwardGlobalFunctionDeclarationAvailableInsideEarlierLocalFunction(t *testing.T) {
	result := Check(`
local function call_scheduler(): number
    return decide_execution_strategy()
end

function decide_execution_strategy(): number
    return 1
end

local out: number = call_scheduler()
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want same-module forward global function declaration accepted", result.Diagnostics)
	}
}

func TestCheckForwardLocalFunctionAssignmentAvailableInsideEarlierLocalFunction(t *testing.T) {
	result := Check(`
local query
local parse

local function load(): table
    local rows, err = query({ "n1" })
    if err or not rows then
        return {}
    end
    return parse(rows)
end

query = function(ids: {string}): ({table}?, string?)
    return {{}}, nil
end

parse = function(rows: {table}): table
    return rows[1] or {}
end

local out: table = load()
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want forward-declared local function assignments available to earlier local function bodies after module initialization", result.Diagnostics)
	}
}

func TestCheckForwardLocalFunctionAssignmentAvailableThroughExportedMethod(t *testing.T) {
	result := Check(`
local query
local parse

local mod = {}

local function load(): table
    local rows, err = query({ "n1" })
    if err or not rows then
        return {}
    end
    return parse(rows)
end

function mod.run(): table
    return load()
end

query = function(ids: {string}): ({table}?, string?)
    return {{}}, nil
end

parse = function(rows: {table}): table
    return rows[1] or {}
end

return mod
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want forward-declared local function assignments visible through exported method bodies after module initialization", result.Diagnostics)
	}
}

func TestCheckForwardLocalFunctionCallBeforeAssignmentStillErrors(t *testing.T) {
	result := Check(`
local query

local function load(): table
    local rows, err = query({ "n1" })
    if err or not rows then
        return {}
    end
    return rows[1] or {}
end

local early: table = load()

query = function(ids: {string}): ({table}?, string?)
    return {{}}, nil
end

return { run = load }
`, WithStdlib())
	requireDiagnosticCode(t, result, diagnostics.CodeDirectCallNotCallable)
}

func TestCheckBooleanModeAssignmentProvesElseBranchReceiverPresent(t *testing.T) {
	result := Check(`
type Executor = {
    with_context: (self: Executor, ctx: table) -> Executor,
}

local function make_executor(): Executor
    local e = {}
    function e:with_context(ctx: table): Executor
        return self
    end
    return e
end

local function run(use_template: boolean): ()
    local executor = nil
    if not use_template then
        executor = make_executor()
    end

    for i = 1, 2 do
        if use_template then
        else
            executor = executor:with_context({})
        end
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want boolean mode assignment to prove executor present in the matching else branch", result.Diagnostics)
	}
}

func TestCheckGuardedOptionalReceiverCapturedByCallback(t *testing.T) {
	result := Check(`
type Streamer = {
    buffer_content: (self: Streamer, chunk: string) -> (),
    flush: (self: Streamer) -> (),
}

type Callbacks = {
    on_content: (chunk: string) -> (),
    on_done: () -> (),
}

local function make_streamer(enabled: boolean): Streamer?
    if not enabled then
        return nil
    end
    local streamer: Streamer = {
        buffer_content = function(self: Streamer, chunk: string): ()
        end,
        flush = function(self: Streamer): ()
        end,
    }
    return streamer
end

local function process(callbacks: Callbacks): ()
    callbacks.on_content("part")
    callbacks.on_done()
end

local function run(enabled: boolean): string?
    local streamer = make_streamer(enabled)
    if not streamer then
        return nil
    end

    local full_content = ""
    process({
        on_content = function(chunk: string)
            streamer:buffer_content(chunk)
            full_content = full_content .. chunk
        end,
        on_done = function()
            streamer:flush()
        end,
    })

    return full_content
end

local out: string? = run(true)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded optional receiver captured by later callbacks to stay non-nil", result.Diagnostics)
	}
}

func TestCheckForwardGlobalFunctionDeclarationUsesCurrentReassignedValue(t *testing.T) {
	result := Check(`
local function call_scheduler(): number
    return decide_execution_strategy()
end

function decide_execution_strategy(): string
    return "stale"
end

function replacement_strategy(): number
    return 1
end

decide_execution_strategy = replacement_strategy

local ok: number = call_scheduler()
local bad: string = call_scheduler()
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly one mismatch from current replacement function", result.Diagnostics)
	}
	if strings.Contains(result.Diagnostics[0].Message, "unknown value") {
		t.Fatalf("diagnostic = %#v, want current replacement result mismatch, not unresolved forward global", result.Diagnostics[0])
	}
	if !strings.Contains(result.Diagnostics[0].Message, "number") || !strings.Contains(result.Diagnostics[0].Message, "string") {
		t.Fatalf("diagnostic = %#v, want number-to-string mismatch from replacement_strategy", result.Diagnostics[0])
	}
}

func TestCheckForwardGlobalFunctionDeclarationStillReportsPreReassignmentCall(t *testing.T) {
	result := Check(`
local function call_scheduler(): number
    return decide_execution_strategy()
end

function decide_execution_strategy(): string
    return "stale"
end

local before: number = call_scheduler()

function replacement_strategy(): number
    return 1
end

decide_execution_strategy = replacement_strategy

local after: number = call_scheduler()
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want only pre-reassignment wrapper return mismatch", result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeReturnContractType {
		t.Fatalf("diagnostic = %#v, want return contract mismatch before reassignment", result.Diagnostics[0])
	}
	if !strings.Contains(result.Diagnostics[0].Message, "string") || !strings.Contains(result.Diagnostics[0].Message, "number") {
		t.Fatalf("diagnostic = %#v, want string-to-number mismatch before reassignment", result.Diagnostics[0])
	}
}

func TestCheckConstructorErrorGuardPreservesReceiverThroughStateTable(t *testing.T) {
	result := Check(`
type WorkflowState = {
    track_process: (self: WorkflowState, node_id: string, pid: string) -> (),
    persist: (self: WorkflowState) -> (table?, string?),
}

local methods = {}
local mt = { __index = methods }
local workflow_state = {}

type Node = {
    node_id: string,
    status: string,
}

function workflow_state.new(dataflow_id: string): (WorkflowState?, string?)
    if dataflow_id == "" then
        return nil, "missing dataflow"
    end
    local instance = {}
    return setmetatable(instance, mt), nil
end

function methods:track_process(node_id: string, pid: string): ()
end

function methods:persist(): (table?, string?)
    return {}, nil
end

local function run(dataflow_id: string): ()
    local ws, ws_err = workflow_state.new(dataflow_id)
    if ws_err then
        return
    end
    if not ws then
        return
    end

    local state = {
        workflow_state = ws,
    }

    state.workflow_state:track_process("node-1", "pid-1")
    local result, err = state.workflow_state:persist()
    if err then
        return
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded constructor result to stay present through state.workflow_state", result.Diagnostics)
	}
}

func TestCheckUntypedMetatableConstructorCarriesMethodsThroughStateTable(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }
local workflow_state = {}

function workflow_state.new(dataflow_id)
    if not dataflow_id or dataflow_id == "" then
        return nil, "missing dataflow"
    end
    local instance = {
        queued_commands = {},
    }
    return setmetatable(instance, mt), nil
end

function methods:track_process(node_id: string, pid: string): ()
end

function methods:persist()
    return {}, nil
end

local function run(dataflow_id: string): ()
    local ws, ws_err = workflow_state.new(dataflow_id)
    if ws_err then
        return
    end
    if not ws then
        return
    end

    local state = {
        workflow_state = ws,
    }

    state.workflow_state:track_process("node-1", "pid-1")
    local result, err = state.workflow_state:persist()
    if err then
        return
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inferred metatable constructor result to expose methods after nil guards", result.Diagnostics)
	}
}

func TestCheckBroadTableParamKeepsPreciseCallArgumentForNestedMethod(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }
local workflow_state = {}

function workflow_state.new()
    local instance = {
        queued_commands = {},
    }
    return setmetatable(instance, mt), nil
end

function methods:track_process(node_id: string, pid: string): ()
end

local function start_node_process(state: table): ()
    state.workflow_state:track_process("node-1", "pid-1")
end

local function run(): ()
    local ws, ws_err = workflow_state.new()
    if ws_err then
        return
    end
    if not ws then
        return
    end

    local state = {
        workflow_state = ws,
    }

    start_node_process(state)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want precise call argument to refine broad table parameter inside helper", result.Diagnostics)
	}
}

func TestCheckAnyCastAfterNilGuardKeepsReceiverPresentThroughHelper(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }
local workflow_state = {}

function workflow_state.new()
    local instance = {}
    return setmetatable(instance, mt), nil
end

function methods:process_commits(commit_ids)
    return {}, nil
end

local function process_pending_commits(state: table): ()
    local result, err = state.workflow_state:process_commits({})
    if err then
        return
    end
end

local function run(): ()
    local ws, ws_err = workflow_state.new()
    if ws_err then
        return
    end
    if not ws then
        return
    end
    local checked_workflow_state = ws :: any

    local state = {
        workflow_state = checked_workflow_state,
    }

    process_pending_commits(state)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want any cast after nil guard to keep receiver present through helper", result.Diagnostics)
	}
}

func TestCheckDeclaredStringReturnFromLengthGuardedArrayIndexStaysCallable(t *testing.T) {
	result := Check(`
local function extract_tool_name(full_id: string): string
	local parts: {string} = {}
	for part in string.gmatch(full_id, "[^:]+") do
		table.insert(parts, part)
	end
	if #parts >= 2 then
		return parts[#parts]
	end
	return full_id
end

local function resolve(id: string): string
	return extract_tool_name(id):lower()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want declared string return callable after length-guarded array index", result.Diagnostics)
	}
}

func TestCheckMemberCastToStringLocalFeedsTypedCall(t *testing.T) {
	result := Check(`
local function call_func(func_id: string, data: any)
end

local function execute_pipeline_step(step: any, data: any): ()
    if type(step) ~= "table" then
        return
    end
    if type(step.func_id) ~= "string" or step.func_id == "" then
        return
    end

    local func_id = step.func_id :: string
    call_func(func_id, data)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want cast member local to satisfy string parameter", result.Diagnostics)
	}
}

func TestCheckNumericFallbackArithmeticFromBroadTokenTable(t *testing.T) {
	result := Check(`
local function format_token_count(count: number): string
    return tostring(count)
end

local function build_status_message(total_tokens): string
    local completion_total = (total_tokens.completion_tokens or 0) + (total_tokens.thinking_tokens or 0)
    if completion_total > 0 then
        return format_token_count(completion_total)
    end
    return ""
end

local totals = {
    completion_tokens = 0,
    thinking_tokens = 0,
}
local msg: string = build_status_message(totals)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want call-context token record to prove numeric fallback arithmetic", result.Diagnostics)
	}
}

func TestCheckUntypedMetatableMethodCanCallSiblingMethodOnSelf(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }
local workflow_state = {}

function workflow_state.new()
    local instance = {
        nodes = {},
    }
    return setmetatable(instance, mt)
end

function methods:load_state()
    self:_load_existing_data()
    local reset_err = self:_reset_running_nodes()
    if reset_err then
        return nil, reset_err
    end
    return self, nil
end

function methods:_load_existing_data()
end

function methods:_reset_running_nodes()
    return nil
end

local ws = workflow_state.new()
local result, err = ws:load_state()
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want untyped metatable self to expose sibling methods", result.Diagnostics)
	}
}

func TestCheckRawInstanceCanCallMetatableMethodsBeforeConstructorReturn(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }
local workflow_state = {}

function workflow_state.new()
    local instance = {
        nodes = {},
    }

    instance:_set_input_requirements_from_config()
    instance:_load_existing_data()
    local reset_err = instance:_reset_running_nodes()
    if reset_err then
        return nil, reset_err
    end
    instance:_reconstruct_active_yields()

    return setmetatable(instance, mt), nil
end

function methods:_set_input_requirements_from_config(node_id: string)
end

function methods:_load_existing_data()
end

function methods:_reset_running_nodes()
    return nil
end

function methods:_reconstruct_active_yields()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want raw constructor instance to expose known metatable methods before return", result.Diagnostics)
	}
}

func TestCheckMetatableConstructorAssociatesMethodsWithoutLocalCall(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }
local workflow_state = {}

function workflow_state.new()
    local instance = {
        nodes = {},
        loaded = false,
    }
    return setmetatable(instance, mt), nil
end

function methods:load_state()
    if self.loaded then
        return self, nil
    end

    self.nodes = {}
    local node: {node_id: string, status: string} = {node_id = "n1", status = "pending"}
    self.nodes[node.node_id] = {
        status = node.status,
    }
    self:_set_input_requirements_from_config(node.node_id)

    self:_load_existing_data()

    local reset_err = self:_reset_running_nodes()
    if reset_err then
        return nil, reset_err
    end

    self:_reconstruct_active_yields()

    self.loaded = true
    return self, nil
end

function methods:_set_input_requirements_from_config(node_id: string)
end

function methods:_load_existing_data()
end

function methods:_reset_running_nodes()
    return nil
end

function methods:_reconstruct_active_yields()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want constructor/metatable association to type-check method body without a local call", result.Diagnostics)
	}
}

func TestCheckTypedReceiverMethodSignaturePreservesErrorReturnThroughWrapper(t *testing.T) {
	result := Check(`
local contract = require("contract")

local function open_contract()
    local def, err = contract.get("containers")
    if err then
        return nil, err
    end
    return def:open()
end

local function run(): ()
    local c, err = open_contract()
    if err then
        return
    end
    c:list({})
end
`, WithStdlib(), WithManifest("contract", contractWrapperManifest()))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed receiver method error-return effect to prove c present", result.Diagnostics)
	}
}

func TestCheckErrorReturnWrapperPreservesValuePresenceCorrelation(t *testing.T) {
	result := Check(`
type DB = {
    release: (self: DB) -> (),
}

local function raw_get(dsn: string): (DB?, string?)
    if dsn == "" then
        return nil, "missing dsn"
    end
    local db: DB = {
        release = function(self: DB): () end,
    }
    return db, nil
end

local function get_db(dsn: string)
    local db, err = raw_get(dsn)
    if err then
        return nil, "failed: " .. err
    end
    return db
end

local function run(dsn: string): ()
    local db, err = get_db(dsn)
    if err then
        return
    end
    db:release()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want wrapper return relation to prove db present after err guard", result.Diagnostics)
	}
}

func TestCheckErrorReturnWrapperChainPreservesValuePresenceCorrelation(t *testing.T) {
	result := Check(`
type DB = {
    release: (self: DB) -> (),
}

local function raw_get(dsn: string): (DB?, string?)
    if dsn == "" then
        return nil, "missing dsn"
    end
    local db: DB = {
        release = function(self: DB): () end,
    }
    return db, nil
end

local function open_db(dsn: string)
    local db, err = raw_get(dsn)
    if err then
        return nil, "open failed: " .. err
    end
    return db
end

local function get_db(dsn: string)
    local db, err = open_db(dsn)
    if err then
        return nil, "connect failed: " .. err
    end
    return db
end

local function run(dsn: string): ()
    local db, err = get_db(dsn)
    if err then
        return
    end
    db:release()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want chained wrappers to preserve db/error presence correlation", result.Diagnostics)
	}
}

func TestCheckManifestErrorReturnWrapperPreservesValuePresenceCorrelation(t *testing.T) {
	result := Check(`
local sql = require("sql")

local function get_db()
    local db, err = sql.get("main")
    if err then
        return nil, "failed: " .. err
    end
    return db
end

local function run(): ()
    local db, err = get_db()
    if err then
        return
    end
    db:release()
end
`, WithStdlib(), WithManifest("sql", sqlWrapperManifest()))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want manifest error-return wrapper to prove db present after err guard", result.Diagnostics)
	}
}

func TestCheckManifestErrorReturnWrapperKeepsReceiverPresentAfterArgumentUse(t *testing.T) {
	result := Check(`
local sql = require("sql")

local function get_db()
    local db, err = sql.get("main")
    if err then
        return nil, "failed: " .. err
    end
    return db
end

local function run(): ()
    local db, err = get_db()
    if err then
        return
    end
    local query = sql.builder.select("id")
    local executor = query:run_with(db)
    local rows, query_err = executor:query()
    db:release()
    if query_err then
        return
    end
end
`, WithStdlib(), WithManifest("sql", sqlWrapperManifest()))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded db to stay present after being passed to builder/executor calls", result.Diagnostics)
	}
}

func TestCheckDirectErrorReturnPreservesValuePresenceCorrelation(t *testing.T) {
	result := Check(`
type DB = {
    release: (self: DB) -> (),
}

local function raw_get(dsn: string): (DB?, string?)
    if dsn == "" then
        return nil, "missing dsn"
    end
    local db: DB = {
        release = function(self: DB): () end,
    }
    return db, nil
end

local function run(dsn: string): ()
    local db, err = raw_get(dsn)
    if err then
        return
    end
    db:release()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want direct error-return relation to prove db present after err guard", result.Diagnostics)
	}
}

func TestCheckDeclaredStringMapDynamicWriteUsesAnnotationNotLiteralInitializerSlots(t *testing.T) {
	result := Check(`
local function prepare_headers(config: {headers: {[string]: string}?}): {[string]: string}
    local headers: {[string]: string} = {
        ["content-type"] = "application/json",
        ["accept"] = "application/json",
    }

    if config.headers then
        for header_name, header_value in pairs(config.headers) do
            headers[tostring(header_name)] = tostring(header_value)
        end
    end

    return headers
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want declared string-map writes to use the annotation, not literal initializer slots", result.Diagnostics)
	}
}

func TestCheckStdlibTostringDirectArgumentUsesDeclaredReturnType(t *testing.T) {
	result := Check(`
type Response = {
    status_code: number,
    headers: {[string]: string},
}

local function record_message(message: string): ()
end

local function run(response: Response): ()
    record_message(tostring(response))
    record_message(tostring(true))
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want tostring(any) to produce string at direct argument boundary", result.Diagnostics)
	}
}

func TestCheckStdlibStringConversionFeedsTypedCall(t *testing.T) {
	result := Check(`
local function send(pid: string, payload: any): ()
end

local function run(payload: any): ()
    local pid = string(payload.reply_to)
    send(pid, { ok = true })
    send(string(payload.next_pid), { ok = true })
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want string(any) conversion to produce string for typed calls", result.Diagnostics)
	}
}

func TestCheckUnknownDefaultSatisfiesUnknownOptionalParameter(t *testing.T) {
	result := Check(`
local function find_items(options: unknown?): ({unknown}?, string?)
    return {}, nil
end

local function run(options: unknown?): string?
    options = options or {}
    local items, err = find_items(options)
    if err then
        return nil
    end
    return "ok"
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unknown? argument to satisfy unknown? parameter after default", result.Diagnostics)
	}
}

func TestCheckAnnotatedArrayReturnSurvivesLocalFunctionSummary(t *testing.T) {
	result := Check(`
local function run_suite(name: string, tests: {any})
    return 0, {}
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        return true
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}
    for _, entry in ipairs(entries) do
        table.insert(no_suite, entry)
    end
    sort_tests(no_suite)
    return suites, no_suite
end

local function run(entries): ()
    local suites, no_suite = group_by_suite(entries)
    if #no_suite > 0 then
        local _, failures = run_suite("other", no_suite)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want annotated array return to satisfy downstream array parameter", result.Diagnostics)
	}
}

func TestCheckSuiteObjectFieldDefaultBeforeArrayEscapeSatisfiesRequiredField(t *testing.T) {
	result := Check(`
type Entry = {id: string, group: string, name: string, meta: {[string]: any}}
type Suite = {name: string, tests: {Entry}}

local function run_suite(idx: integer, suite: Suite): ()
end

local function group_by_suite(entries: {Entry}): {Suite}
    local suites: {Suite} = {}
    for _, entry in ipairs(entries) do
        local suite = {
            name = entry.group,
        }
        suite.tests = suite.tests or {}
        table.insert(suite.tests, entry)
        table.insert(suites, suite)
    end
    return suites
end

local suites = group_by_suite({})
for idx, suite in ipairs(suites) do
    run_suite(idx, suite)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want required field proof after direct default write to survive array escape", result.Diagnostics)
	}
}

func TestCheckTableInsertInfersArrayShapeThroughNestedField(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "top-level-empty-array",
			src: `
type Entry = {id: string}

local function takes_entries(entries: {Entry}): ()
end

local entry: Entry = {id = "a"}
local entries = {}
table.insert(entries, entry)
takes_entries(entries)
`,
		},
		{
			name: "nested-direct-empty-array",
			src: `
type Entry = {id: string}
type Suite = {name: string, tests: {Entry}}

local function takes_suite(suite: Suite): ()
end

local entry: Entry = {id = "a"}
local suite = {name = "alpha"}
suite.tests = {}
table.insert(suite.tests, entry)
takes_suite(suite)
`,
		},
		{
			name: "nested-logical-default-empty-array",
			src: `
type Entry = {id: string}
type Suite = {name: string, tests: {Entry}}

local function takes_suite(suite: Suite): ()
end

local entry: Entry = {id = "a"}
local suite = {name = "alpha"}
suite.tests = suite.tests or {}
table.insert(suite.tests, entry)
takes_suite(suite)
`,
		},
		{
			name: "loop-local-nested-logical-default-empty-array",
			src: `
type Entry = {id: string, group: string}
type Suite = {name: string, tests: {Entry}}

local function takes_suites(suites: {Suite}): ()
end

local function collect(entries: {Entry}): ()
    local suites: {Suite} = {}
    for _, entry in ipairs(entries) do
        local suite = {name = "alpha"}
        suite.tests = suite.tests or {}
        table.insert(suite.tests, entry)
        table.insert(suites, suite)
    end
    takes_suites(suites)
end
`,
		},
		{
			name: "function-local-nested-logical-default-empty-array",
			src: `
type Entry = {id: string, group: string}
type Suite = {name: string, tests: {Entry}}

local function takes_suite(suite: Suite): ()
end

local function collect(entry: Entry): ()
    local suite = {name = "alpha"}
    suite.tests = suite.tests or {}
    table.insert(suite.tests, entry)
    takes_suite(suite)
end
`,
		},
		{
			name: "loop-local-nested-logical-default-field-derived-name",
			src: `
type Entry = {id: string, group: string}
type Suite = {name: string, tests: {Entry}}

local function takes_suites(suites: {Suite}): ()
end

local function collect(entries: {Entry}): ()
    local suites: {Suite} = {}
    for _, entry in ipairs(entries) do
        local suite = {name = entry.group}
        suite.tests = suite.tests or {}
        table.insert(suite.tests, entry)
        table.insert(suites, suite)
    end
    takes_suites(suites)
end
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(tc.src, WithStdlib())
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v", result.Diagnostics)
			}
		})
	}
}

func TestCheckStringUnpackLiteralIntegerFormatsReturnInteger(t *testing.T) {
	result := Check(`
local function parse(buf: string): integer
    local total_length: integer = string.unpack(">I4", buf, 1) :: integer
    local headers_length: integer = string.unpack(">I2", buf, 5) :: integer
    return total_length + headers_length
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want literal integer unpack formats to return integer", result.Diagnostics)
	}
}

func TestCheckStringUnpackLiteralStringAndFloatFormatsReturnPreciseTypes(t *testing.T) {
	result := Check(`
local function parse(buf: string): string
    local tag: string = string.unpack(">c4", buf, 1) :: string
    local score: number = string.unpack(">d", buf, 5) :: number
    return tag .. tostring(score)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want literal string and float unpack formats to return precise types", result.Diagnostics)
	}
}

func TestCheckStringUnpackNonLiteralFormatStaysAny(t *testing.T) {
	result := Check(`
local function parse(buf: string, fmt: string): integer
    local total_length: integer = string.unpack(fmt, buf, 1)
    return total_length
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		MessageContains: []string{"string.unpack(...)", "any", "not integer"},
	})
}

func TestCheckTonumberWithExplicitBaseReturnsOptionalInteger(t *testing.T) {
	result := Check(`
local function hex_decode(hex_str: string): string?
    if #hex_str % 2 ~= 0 then
        return nil
    end
    local bytes = ""
    for i = 1, #hex_str, 2 do
        local hex_byte = hex_str:sub(i, i + 1)
        local byte_val = tonumber(hex_byte, 16)
        if not byte_val then
            return nil
        end
        bytes = bytes .. string.char(byte_val)
    end
    return bytes
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want tonumber with an explicit base to produce integer? for string.char after nil guard", result.Diagnostics)
	}
}

func TestCheckReverseNumericForLengthOfInferredArrayBindsIntegerArgument(t *testing.T) {
	result := Check(`
local function take_integer(n: integer): ()
end

local shuffled = {}
for i = #shuffled, 2, -1 do
    take_integer(i)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want #array reverse numeric-for variable to satisfy integer arguments", result.Diagnostics)
	}
}

func TestCheckTonumberWithoutBaseStaysOptionalNumber(t *testing.T) {
	result := Check(`
local function decode_decimal(s: string): string?
    local n = tonumber(s)
    if not n then
        return nil
    end
    return string.char(n)
end
	`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{"argument 1 (n) is number", "not integer"},
	})
}

func TestCheckNumericComparisonGuardNarrowsUntypedValueToNumber(t *testing.T) {
	result := Check(`
local function apply_limit(limit: number): ()
end

local function list(limit)
    if limit and limit > 0 then
        apply_limit(limit)
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want numeric comparison guard to prove limit is number on the taken edge", result.Diagnostics)
	}
}

func TestCheckNumericComparisonEarlyReturnGuardNarrowsContinuationToNumber(t *testing.T) {
	result := Check(`
local function apply_limit(limit: number): ()
end

local function list(limit)
    if limit <= 0 then
        return
    end
    apply_limit(limit)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want false edge of numeric comparison guard to prove limit is number after early return", result.Diagnostics)
	}
}

func TestCheckTruthyGuardDoesNotNarrowUntypedValueToNumber(t *testing.T) {
	result := Check(`
local function apply_limit(limit: number): ()
end

local function list(limit)
    if limit then
        apply_limit(limit)
    end
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{"argument 1 (limit) comes from any/unknown", "number"},
	})
}

func TestCheckModuloLengthIndexProvesNonEmptyArrayReadInBounds(t *testing.T) {
	result := Check(`
local frames = {"a", "b", "c"}

local function spinner(index: integer): string
    local frame = frames[((index - 1) % #frames) + 1]
    return frame
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want modulo-by-length index over non-empty array to prove read in bounds", result.Diagnostics)
	}
}

func TestCheckModuloLengthIndexProvesNonEmptyStaticMemberArrayReadInBounds(t *testing.T) {
	result := Check(`
local term = {}
term.spinner_frames = {"a", "b", "c"}

function term.spinner(index: integer): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return frame
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want modulo-by-length index over non-empty static-member array to prove read in bounds", result.Diagnostics)
	}
}

func TestCheckModuloLengthIndexDoesNotTrustPossiblyEmptyArray(t *testing.T) {
	result := Check(`
local function spinner(frames: {string}, index: integer): string
    local frame = frames[((index - 1) % #frames) + 1]
    return frame
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		DiagnosticCount: 1,
		MessageContains: []string{"cannot return frame", "may be nil"},
	})
}

func TestCheckLengthInequalityGuardProvesFixedArraySlotsPresent(t *testing.T) {
	result := Check(`
local function parse_mock_path(path: string): {string}
    local parts: {string} = {}
    for part in string.gmatch(path, "[^.]+") do
        table.insert(parts, part)
    end
    return parts
end

local function get_target_info(path: string): string
    local parts = parse_mock_path(path)
    if #parts ~= 2 then
        error("invalid path")
    end
    local obj_name, field_name = parts[1], parts[2]
    return obj_name .. "." .. field_name
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want #parts ~= 2 guard to prove parts[1] and parts[2] present", result.Diagnostics)
	}
}

func TestCheckLengthGuardProvesUnannotatedInsertedStringArraySlotsPresent(t *testing.T) {
	result := Check(`
local function parse_mock_path(path)
    local parts = {}
    for part in string.gmatch(path, "[^.]+") do
        table.insert(parts, part)
    end
    return parts
end

local function get_target_info(path: string): string
    local parts = parse_mock_path(path)
    if #parts ~= 2 then
        error("invalid path")
    end
    local obj_name, field_name = parts[1], parts[2]
    return "Cannot find object '" .. obj_name .. "." .. field_name .. "'"
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inferred inserted string array slots present after #parts ~= 2 guard", result.Diagnostics)
	}
}

func TestCheckAndExportPublishesNestedLocalConstantTableMember(t *testing.T) {
	lifecycle := CheckAndExport(`
local PHASE = {
    ACTIVATE = "activate",
    DEACTIVATE = "deactivate",
}

local lifecycle_runtime = {
    PHASE = PHASE,
    CONTRACT_ID = "wippy.agent:lifecycle",
}

return lifecycle_runtime
`, "lifecycle_runtime")
	if len(lifecycle.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", lifecycle.Errors)
	}

	result := Check(`
local lifecycle_runtime = require("lifecycle_runtime")

local function apply_lifecycle(phase: string): ()
end

apply_lifecycle(lifecycle_runtime.PHASE.DEACTIVATE)
`, WithStdlib(), WithModule("lifecycle_runtime", lifecycle))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nested exported constant table member to satisfy string parameter", result.Diagnostics)
	}
}

func TestCheckAndExportKeepsNestedConstantMemberAfterReturnedTableMutation(t *testing.T) {
	lifecycle := CheckAndExport(`
local PHASE = {
    ACTIVATE = "activate",
    DEACTIVATE = "deactivate",
}

local lifecycle_runtime = {
    PHASE = PHASE,
    CONTRACT_ID = "wippy.agent:lifecycle",
}

lifecycle_runtime._contract = nil

return lifecycle_runtime
`, "lifecycle_runtime")
	if len(lifecycle.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", lifecycle.Errors)
	}

	result := Check(`
local lifecycle_runtime = require("lifecycle_runtime")

local function apply_lifecycle(phase: string): ()
end

apply_lifecycle(lifecycle_runtime.PHASE.DEACTIVATE)
`, WithStdlib(), WithModule("lifecycle_runtime", lifecycle))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want later member mutation to preserve unrelated nested constant member", result.Diagnostics)
	}
}

func TestCheckOptionalImportedResponseBodyFallbackFeedsDecode(t *testing.T) {
	responseType := typetable.NewRecord().
		Field("status_code", typ.Integer).
		OptField("body", typ.String).
		Build()
	http := manifest.New("http_client")
	http.SetExport(typetable.NewRecord().
		Field("get", typ.Func().
			Param("url", typ.String).
			Returns(normalize.Optional(responseType), normalize.Optional(typ.String)).
			Build()).
		Build())

	result := Check(`
local http_client = require("http_client")

local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function request(): (table?, string?)
    local response, err = http_client.get("https://example.test")
    if err or not response then
        return nil, err or "no response"
    end

    local parsed, parse_err = json.decode(response.body or "")
    if parse_err then
        return nil, parse_err
    end
    return parsed, nil
end
`, WithStdlib(), WithManifest("http_client", http))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want optional response.body fallback to satisfy decode string parameter", result.Diagnostics)
	}
}

func TestCheckImportedResponseBodyFallbackAfterErrorReturnGuardFeedsDecode(t *testing.T) {
	responseType := typetable.NewRecord().
		Field("status_code", typ.Integer).
		OptField("body", typ.String).
		Build()
	http := manifest.New("http_client")
	postType := typ.Func().
		Param("url", typ.String).
		Param("options", typetable.NewRecord().Build()).
		Returns(normalize.Optional(responseType), normalize.Optional(typ.String)).
		Build()
	http.SetExport(typetable.NewRecord().
		Field("post", postType).
		Build())
	http.DefineFunctionSignature("post", errorReturnSignature(postType))

	result := Check(`
local http = require("http_client")

local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function request(): (table?, string?)
    local response, err = http.post("https://example.test", {})
    if err then
        return nil, err
    end

    local parsed, parse_err = json.decode(response.body or "")
    if parse_err then
        return nil, parse_err
    end
    return parsed, nil
end
`, WithStdlib(), WithManifest("http_client", http))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want error-return guard plus body fallback to satisfy decode string parameter", result.Diagnostics)
	}
}

func TestCheckImportedResponseBodyFallbackInDirectReturnFeedsDecode(t *testing.T) {
	responseType := typetable.NewRecord().
		Field("status_code", typ.Integer).
		OptField("body", typ.String).
		Build()
	http := manifest.New("http_client")
	postType := typ.Func().
		Param("url", typ.String).
		Param("options", typetable.NewRecord().Build()).
		Returns(normalize.Optional(responseType), normalize.Optional(typ.String)).
		Build()
	http.SetExport(typetable.NewRecord().
		Field("post", postType).
		Build())
	http.DefineFunctionSignature("post", errorReturnSignature(postType))

	result := Check(`
local http = require("http_client")

local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function request(): (any, string?)
    local response, err = http.post("https://example.test", {})
    if err then
        return nil, err
    end

    return json.decode(response.body or "")
end
`, WithStdlib(), WithManifest("http_client", http))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want direct return call to see optional response.body fallback string", result.Diagnostics)
	}
}

func TestCheckUntrustedAnyBodyFallbackDoesNotLaunderIntoString(t *testing.T) {
	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function request(response: any): (any, string?)
    return json.decode(response.body or "")
end
`, WithStdlib())
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"argument 1", "any/unknown", "no proof shows it is string"},
	})
	if strings.Contains(diag.Message, "may be nil") {
		t.Fatalf("message = %q, want untrusted-any boundary, not nil-only fallback explanation", diag.Message)
	}
}

func TestCheckAndExportIsFunctionPublishesNormalReturnTypeRefinement(t *testing.T) {
	mod := CheckAndExport(`
local test = {}

function test.is_function(value: any, msg: string?)
    if type(value) ~= "function" then
        error(msg or "expected function", 2)
    end
    return value
end

return test
`, "test", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	sig, ok := mod.Manifest.FunctionSignatures["test.is_function"]
	if !ok {
		t.Fatalf("missing test.is_function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if !hasOperationalNormalReturnTypeRefinement(sig, 0, typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()) {
		t.Fatalf("operational effects = %#v, want param 0 normal-return function type refinement", sig.OperationalEffects)
	}
}

func TestCheckImportedIsFunctionRefinesOptionalMemberCallable(t *testing.T) {
	testMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected non-nil", 2)
    end
    return value
end

function test.is_function(value: any, msg: string?): any
    if type(value) ~= "function" then
        error(msg or "expected function", 2)
    end
    return value
end

function test.it(name: string, fn: () -> ()): ()
    fn()
end

function test.describe(name: string, fn: () -> ()): ()
    fn()
end

function test.run_cases(fn: () -> ()): ()
    fn()
end

return test
`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	result := Check(`
local test = require("test")

type Impl = {
    up: any?,
}

local function run(impl: Impl?)
    test.not_nil(impl, "implementation expected")
    test.is_function(impl.up, "up expected")
    impl.up(nil)
end
`, WithStdlib(), WithModule("test", testMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported is_function to refine optional member callable", result.Diagnostics)
	}
}

func TestCheckImportedIsFunctionRefinesDynamicMapLookupMemberCallable(t *testing.T) {
	testMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected non-nil", 2)
    end
    return value
end

function test.is_function(value: any, msg: string?): any
    if type(value) ~= "function" then
        error(msg or "expected function", 2)
    end
    return value
end

return test
`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	result := Check(`
local test = require("test")

type Impl = {
    up: any?,
    down: any?,
    after: any?,
}
type Item = {
    database_implementations: {[string]: Impl},
}

local function run(implementations: {Item})
    local impl = implementations[1].database_implementations["sqlite"]
    test.not_nil(impl, "implementation expected")
    test.is_function(impl.up, "up expected")
    test.is_function(impl.down, "down expected")
    test.is_function(impl.after, "after expected")

    impl.up(nil)
    impl.down(nil)
    impl.after(nil)
end
`, WithStdlib(), WithModule("test", testMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported is_function to refine dynamic map lookup members callable", result.Diagnostics)
	}
}

func TestCheckImportedIsFunctionRefinesImportedDefineResultDynamicMapMembers(t *testing.T) {
	testMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected non-nil", 2)
    end
    return value
end

function test.is_function(value: any, msg: string?): any
    if type(value) ~= "function" then
        error(msg or "expected function", 2)
    end
    return value
end

return test
`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	coreMod := CheckAndExport(`
type DatabaseImpl = {
    type: string?,
    up: any?,
    after: any?,
    down: any?,
}

type MigrationItem = {
    description: string,
    database_implementations: {[string]: DatabaseImpl},
}

local core = {}

local function create_context(): { implementations: {MigrationItem} }
    return {
        implementations = {
            {
                description = "test migration",
                database_implementations = {
                    sqlite = {
                        up = function(db: any) end,
                        down = function(db: any) end,
                        after = function(db: any) end,
                    },
                },
            },
        },
    } :: { implementations: {MigrationItem} }
end

function core.setup_globals(context: { implementations: {MigrationItem} }): ()
    _G.migration = function(description: string, fn: () -> ()): ()
        fn()
    end
    _G.database = function(db_type: string, fn: () -> ()): ()
        fn()
    end
    _G.up = function(fn: any): () end
    _G.down = function(fn: any): () end
    _G.after = function(fn: any): () end
end

function core.define(fn: () -> ()): {MigrationItem}
    local context = create_context()
    core.setup_globals(context)
    local success, err = pcall(fn)
    if not success then
        error("bad migration: " .. tostring(err))
    end
    return context.implementations
end

return core
`, "core", WithStdlib())
	if len(coreMod.Errors) != 0 {
		t.Fatalf("core module errors = %#v, want none", coreMod.Errors)
	}

	result := Check(`
local core = require("core")
local test = require("test")

local implementations = core.define(function() end)
local impl = implementations[1].database_implementations["sqlite"]
test.not_nil(impl, "implementation expected")
test.is_function(impl.up, "up expected")
test.is_function(impl.down, "down expected")
test.is_function(impl.after, "after expected")

impl.up(nil)
impl.down(nil)
impl.after(nil)
`, WithStdlib(), WithModule("core", coreMod), WithModule("test", testMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported define result dynamic map members callable after test assertions", result.Diagnostics)
	}
}

func TestCheckImportedIsFunctionRefinesImportedDefineResultDynamicMapMembersInCallback(t *testing.T) {
	testMod := CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected non-nil", 2)
    end
    return value
end

function test.is_function(value: any, msg: string?): any
    if type(value) ~= "function" then
        error(msg or "expected function", 2)
    end
    return value
end

function test.it(name: string, fn: () -> ()): ()
    fn()
end

return test
`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	coreMod := CheckAndExport(`
type DatabaseImpl = {
    type: string?,
    up: any?,
    after: any?,
    down: any?,
}

type MigrationItem = {
    description: string,
    database_implementations: {[string]: DatabaseImpl},
}

local core = {}

local function create_context(): { implementations: {MigrationItem} }
    return {
        implementations = {
            {
                description = "test migration",
                database_implementations = {
                    sqlite = {
                        up = function(db: any) end,
                        down = function(db: any) end,
                        after = function(db: any) end,
                    },
                },
            },
        },
    } :: { implementations: {MigrationItem} }
end

function core.define(fn: () -> ()): {MigrationItem}
    local context = create_context()
    local success, err = pcall(fn)
    if not success then
        error("bad migration: " .. tostring(err))
    end
    return context.implementations
end

return core
`, "core", WithStdlib())
	if len(coreMod.Errors) != 0 {
		t.Fatalf("core module errors = %#v, want none", coreMod.Errors)
	}

	result := Check(`
local core = require("core")
local test = require("test")

local function define_tests()
    test.describe("define", function()
        test.it("captures database-specific up/down/after", function()
            local implementations = core.define(function()
                migration("test migration", function()
                    database("sqlite", function()
                        up(function(db: any) end)
                        down(function(db: any) end)
                        after(function(db: any) end)
                    end)
                end)
            end)
            local impl = implementations[1].database_implementations["sqlite"]
            test.not_nil(impl)
            test.is_function(impl.up)
            test.is_function(impl.down)
            test.is_function(impl.after)

            impl.up(nil)
            impl.down(nil)
            impl.after(nil)
        end)
    end)
end

return {
    run_tests = function()
        return test.run_cases(define_tests)
    end
}
`, WithStdlib(),
		WithGlobals("migration", "database", "up", "down", "after"),
		WithManifest("core", roundTripModuleManifest(t, "core", coreMod)),
		WithManifest("test", roundTripModuleManifest(t, "test", testMod)))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported assertions to refine dynamic map members inside callback", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadProvesPrimaryMapMemberPresent(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, boolean, string) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any, any, boolean, string) -> any): ()
    local channel_id = tostring(chan)
    registered_channels[channel_id] = { chan = chan, handler = handler }
    channel_to_id[chan] = channel_id
end

local function dispatch(result: { channel: any, value: any, ok: boolean }, state: any): any
    register_channel(result.channel, function(inner_state, value, ok, id) return value end)
    local channel_id = channel_to_id[result.channel]
    if channel_id then
        local channel_info = registered_channels[channel_id]
        return channel_info.handler(state, result.value, result.ok, channel_id)
    end
    return nil
end

return dispatch
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want reverse-map value membership to prove registered channel entry present", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadSurvivesCapturedRebuildHelper(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, any, any) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}
local select_cases = {}

local function rebuild_select_cases(): ()
    select_cases = {}
    for _, channel_info in pairs(registered_channels) do
        table.insert(select_cases, channel_info.chan:case_receive())
    end
end

local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
    if not chan or type(handler) ~= "function" then
        error("Channel and handler function must be provided")
    end

    local channel_id = tostring(chan)
    registered_channels[channel_id] = { chan = chan, handler = handler }
    channel_to_id[chan] = channel_id
    rebuild_select_cases()
    return true
end

local function dispatch(result: { channel: any?, value: any?, ok: boolean }, state: any): any
    register_channel(result.channel, function(inner_state, value, ok, id) return value end)
    local channel_id = channel_to_id[result.channel]
    if channel_id then
        local channel_info = registered_channels[channel_id]
        local value = result.value
        local is_ok = result.ok
        local reply = channel_info.handler(state, value, is_ok, channel_id)
        return reply
    end
    return nil
end

return dispatch
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want reverse-map membership to survive captured rebuild helper", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadUsesClosedWriterInvariant(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, any, any) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
    local channel_id = tostring(chan)
    registered_channels[channel_id] = { chan = chan, handler = handler }
    channel_to_id[chan] = channel_id
    return true
end

local function unregister_channel(chan: any): boolean
    local channel_id = tostring(chan)
    if registered_channels[channel_id] then
        registered_channels[channel_id] = nil
        channel_to_id[chan] = nil
        return true
    end
    return false
end

local function dispatch(result: { channel: any?, value: any?, ok: boolean }, state: any): any
    local channel_id = channel_to_id[result.channel]
    if channel_id then
        local channel_info = registered_channels[channel_id]
        local reply = channel_info.handler(state, result.value, result.ok, channel_id)
        if not result.ok then
            registered_channels[channel_id] = nil
            channel_to_id[result.channel] = nil
        end
        return reply
    end
    return nil
end

return {
    register_channel = register_channel,
    unregister_channel = unregister_channel,
    dispatch = dispatch,
}
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want closed writer invariant to prove reverse-map values are registered keys", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadUsesClosedWriterInvariantWithCapturedRebuild(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, any, any) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}
local select_cases = {}

local function rebuild_select_cases(): ()
    select_cases = {}
    for _, channel_info in pairs(registered_channels) do
        table.insert(select_cases, channel_info.chan:case_receive())
    end
end

local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
    if not chan or type(handler) ~= "function" then
        error("Channel and handler function must be provided")
    end

    local channel_id = tostring(chan)
    registered_channels[channel_id] = { chan = chan, handler = handler }
    channel_to_id[chan] = channel_id
    rebuild_select_cases()
    return true
end

local function unregister_channel(chan: any): boolean
    if not chan then return false end

    local channel_id = tostring(chan)
    if registered_channels[channel_id] then
        registered_channels[channel_id] = nil
        channel_to_id[chan] = nil
        rebuild_select_cases()
        return true
    end
    return false
end

local function dispatch(result: { channel: any?, value: any?, ok: boolean }, state: any): any
    local channel_id = channel_to_id[result.channel]
    if channel_id then
        local channel_info = registered_channels[channel_id]
        local value = result.value
        local is_ok = result.ok

        local reply = channel_info.handler(state, value, is_ok, channel_id)

        if not is_ok then
            registered_channels[channel_id] = nil
            channel_to_id[result.channel] = nil
            rebuild_select_cases()
        end

        return reply
    end
    return nil
end

return {
    register_channel = register_channel,
    unregister_channel = unregister_channel,
    dispatch = dispatch,
}
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want closed writer invariant to survive captured rebuild helper", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadUsesFunctionLocalClosedWriterInvariant(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, any, any) -> any,
}

type SelectResult = {
    ok: boolean,
    channel: any?,
    value: any?,
}

local function make_actor(): any
    local registered_channels: {[string]: ChannelInfo} = {}
    local channel_to_id: {[any]: string} = {}
    local select_cases = {}

    local function rebuild_select_cases(): ()
        select_cases = {}
        for _, channel_info in pairs(registered_channels) do
            table.insert(select_cases, channel_info.chan:case_receive())
        end
    end

    local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
        if not chan or type(handler) ~= "function" then
            error("Channel and handler function must be provided")
        end

        local channel_id = tostring(chan)
        registered_channels[channel_id] = { chan = chan, handler = handler }
        channel_to_id[chan] = channel_id
        rebuild_select_cases()
        return true
    end

    local function unregister_channel(chan: any): boolean
        if not chan then return false end

        local channel_id = tostring(chan)
        if registered_channels[channel_id] then
            registered_channels[channel_id] = nil
            channel_to_id[chan] = nil
            rebuild_select_cases()
            return true
        end
        return false
    end

    local function dispatch(result: { channel: any?, value: any?, ok: boolean }, state: any): any
        local channel_id = channel_to_id[result.channel]
        if channel_id then
            local channel_info = registered_channels[channel_id]
            local value = result.value
            local is_ok = result.ok

            local reply = channel_info.handler(state, value, is_ok, channel_id)

            if not is_ok then
                registered_channels[channel_id] = nil
                channel_to_id[result.channel] = nil
                rebuild_select_cases()
            end

            return reply
        end
        return nil
    end

    return {
        register_channel = register_channel,
        unregister_channel = unregister_channel,
        dispatch = dispatch,
    }
end

return make_actor
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want function-local closed writer invariant to prove reverse-map values are registered keys", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadRestoresClosedInvariantAcrossCleanupLoop(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, any, any) -> any,
}

local function make_actor(): any
    local registered_channels: {[string]: ChannelInfo} = {}
    local channel_to_id: {[any]: string} = {}
    local select_cases = {}

    local function rebuild_select_cases(): ()
        select_cases = {}
        for _, channel_info in pairs(registered_channels) do
            table.insert(select_cases, channel_info.chan:case_receive())
        end
    end

    local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
        local channel_id = tostring(chan)
        registered_channels[channel_id] = { chan = chan, handler = handler }
        channel_to_id[chan] = channel_id
        rebuild_select_cases()
        return true
    end

    local function dispatch(next_result: () -> { channel: any?, value: any?, ok: boolean }, state: any): any
        while true do
            local result = next_result()
            local channel_id = channel_to_id[result.channel]
            if channel_id then
                local channel_info = registered_channels[channel_id]
                local value = result.value
                local is_ok = result.ok
                local reply = channel_info.handler(state, value, is_ok, channel_id)

                if not is_ok then
                    registered_channels[channel_id] = nil
                    channel_to_id[result.channel] = nil
                    rebuild_select_cases()
                end

                if reply then
                    state = reply
                end
            end
        end
    end

    return {
        register_channel = register_channel,
        dispatch = dispatch,
    }
end

return make_actor
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want cleanup loop to restore closed reverse-map invariant", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadRestoresClosedInvariantAcrossBreakableCleanupLoop(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, any, any) -> any,
}

type SelectResult = {
    ok: boolean,
    channel: any?,
    value: any?,
}

local function make_actor(): any
    local registered_channels: {[string]: ChannelInfo} = {}
    local channel_to_id: {[any]: string} = {}
    local select_cases = {}

    local function rebuild_select_cases(): ()
        select_cases = {}
        for _, channel_info in pairs(registered_channels) do
            table.insert(select_cases, channel_info.chan:case_receive())
        end
    end

    local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
        local channel_id = tostring(chan)
        registered_channels[channel_id] = { chan = chan, handler = handler }
        channel_to_id[chan] = channel_id
        rebuild_select_cases()
        return true
    end

    local function unregister_channel(chan: any): boolean
        if not chan then return false end
        local channel_id = tostring(chan)
        if registered_channels[channel_id] then
            registered_channels[channel_id] = nil
            channel_to_id[chan] = nil
            rebuild_select_cases()
            return true
        end
        return false
    end

    local function dispatch(next_result: () -> SelectResult, state: any, handlers: any): any
        state.register_channel = register_channel
        state.unregister_channel = unregister_channel

        if handlers.__init then
            handlers.__init(state)
        end

        while true do
            local result: SelectResult = next_result()
            if not result.ok then
                break
            end

            local channel_id = channel_to_id[result.channel]
            if channel_id then
                local channel_info = registered_channels[channel_id]
                local value = result.value
                local is_ok = result.ok
                local reply = channel_info.handler(state, value, is_ok, channel_id)

                if not is_ok then
                    registered_channels[channel_id] = nil
                    channel_to_id[result.channel] = nil
                    rebuild_select_cases()
                end

                if reply then
                    state = reply
                end
            end
        end
        return state
    end

    return {
        register_channel = register_channel,
        dispatch = dispatch,
    }
end

return make_actor
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want breakable cleanup loop to restore closed reverse-map invariant", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadUsesClosedInvariantInSameFunctionAsFreshTables(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, any, any) -> any,
}

type SelectResult = {
    ok: boolean,
    channel: any?,
    value: any?,
}

local function run_loop(next_result: () -> SelectResult, state: any): any
    local registered_channels: {[string]: ChannelInfo} = {}
    local channel_to_id: {[any]: string} = {}
    local select_cases = {}

    local function rebuild_select_cases(): ()
        select_cases = {}
        for _, channel_info in pairs(registered_channels) do
            table.insert(select_cases, channel_info.chan:case_receive())
        end
    end

    local function register_channel(chan: any, handler: (any, any, any, any) -> any): boolean
        local channel_id = tostring(chan)
        registered_channels[channel_id] = { chan = chan, handler = handler }
        channel_to_id[chan] = channel_id
        rebuild_select_cases()
        return true
    end

    state.register_channel = register_channel

    while true do
        local result: SelectResult = next_result()
        if not result.ok then
            break
        end

        local channel_id = channel_to_id[result.channel]
        if channel_id then
            local channel_info = registered_channels[channel_id]
            local value = result.value
            local is_ok = result.ok
            local reply = channel_info.handler(state, value, is_ok, channel_id)

            if not is_ok then
                registered_channels[channel_id] = nil
                channel_to_id[result.channel] = nil
                rebuild_select_cases()
            end

            if reply then
                state = reply
            end
        end
    end

    return state
end

return run_loop
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want same-function fresh tables to receive closed reverse-map invariant", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadAfterPrimaryDeleteRequiresCleanupProof(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    handler: (any) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any) -> any): ()
    local channel_id = tostring(chan)
    registered_channels[channel_id] = { handler = handler }
    channel_to_id[chan] = channel_id
end

local function dispatch(chan: any): any
    register_channel(chan, function(value) return value end)
    local channel_id = channel_to_id[chan]
    if channel_id then
        registered_channels[channel_id] = nil
        local stale_id = channel_to_id[chan]
        if stale_id then
            local channel_info = registered_channels[stale_id]
            return channel_info.handler(chan)
        end
    end
    return nil
end

return dispatch
`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeOptionalMethodCall)
	requireEvidenceMessage(t, diag, "receiver channel_info is optional at call to channel_info.handler")
	requireEvidenceMessage(t, diag, "no nil check proves receiver channel_info is present")
}

func TestCheckGlobalTableOverrideFeedsGlobalFunctionCall(t *testing.T) {
	result := Check(`
local captured_fn

_G.coroutine = {
    spawn = function(fn: () -> ())
        captured_fn = fn
        return true
    end,
}

coroutine.spawn(function() end)
captured_fn()
`, WithStdlib(), WithGlobals("coroutine"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want _G.coroutine assignment to feed coroutine.spawn call", result.Diagnostics)
	}
}

func TestCheckRootRecordFunctionCallPropagatesClosureSideEffects(t *testing.T) {
	result := Check(`
local captured_fn

coroutine = {
    spawn = function(fn: () -> ())
        captured_fn = fn
        return true
    end,
}

coroutine.spawn(function() end)
captured_fn()
`, WithStdlib(), WithGlobals("coroutine"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want root record function call to propagate closure side effects", result.Diagnostics)
	}
}

func TestCheckRecordMethodCallbackPropagatesCapturedGlobalSpawn(t *testing.T) {
	result := Check(`
local captured_fn

coroutine = {
    spawn = function(fn: () -> ())
        captured_fn = fn
        return true
    end,
}

local state = {}

local function async(fn: () -> ())
    coroutine.spawn(function()
        fn()
    end)
end

state.async = async

local handlers = {
    __init = function(s)
        s.async(function() end)
    end,
}

handlers.__init(state)
captured_fn()
`, WithStdlib(), WithGlobals("coroutine"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured spawn callback through record callback field to be callable", result.Diagnostics)
	}
}

func TestCheckRecordFunctionFieldCallPropagatesNestedCapturedGlobalSpawn(t *testing.T) {
	result := Check(`
local captured_fn

coroutine = {
    spawn = function(fn: () -> ())
        captured_fn = fn
        return true
    end,
}

local state = {}

local function async(fn: () -> ())
    coroutine.spawn(function()
        fn()
    end)
end

state.async = async
state.async(function() end)
captured_fn()
`, WithStdlib(), WithGlobals("coroutine"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want direct record function field call to propagate nested captured spawn", result.Diagnostics)
	}
}

func TestCheckFactoryInstalledRecordFunctionPropagatesCapturedGlobalSpawn(t *testing.T) {
	result := Check(`
local captured_fn

coroutine = {
    spawn = function(fn: () -> ())
        captured_fn = fn
        return true
    end,
}

local actor = {}

function actor.new(initial_state: any, handlers: any): any
    local state = {}

    local function async(fn: () -> ())
        coroutine.spawn(function()
            fn()
        end)
    end

    state.async = async

    if handlers.__init then
        handlers.__init(state)
    end

    return {
        run = function() end,
    }
end

local a = actor.new({}, {
    __init = function(state)
        state.async(function() end)
    end,
})

a.run()
captured_fn()
`, WithStdlib(), WithGlobals("coroutine"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want factory-installed record function to propagate captured spawn callback", result.Diagnostics)
	}
}

func TestCheckLocalAnyReturnKeepsSolvedBodyRecordShape(t *testing.T) {
	result := Check(`
local function make_runner(): any
    return {
        run = function() end,
    }
end

local runner = make_runner()
runner.run()
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local any return to keep solved record shape", result.Diagnostics)
	}
}

func TestCheckDynamicReverseMapReadRequiresPrimaryMapProofForEveryValueSource(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, boolean, string) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any, any, boolean, string) -> any): ()
    local channel_id = tostring(chan)
    registered_channels[channel_id] = { chan = chan, handler = handler }
    channel_to_id[chan] = channel_id
end

local function register_unpaired(chan: any, id: string): ()
    channel_to_id[chan] = id
end

local function dispatch(result: { channel: any, value: any, ok: boolean }, state: any): any
    register_channel(result.channel, function(inner_state, value, ok, id) return value end)
    register_unpaired(result.channel, "stale")
    local channel_id = channel_to_id[result.channel]
    if channel_id then
        local channel_info = registered_channels[channel_id]
        return channel_info.handler(state, result.value, result.ok, channel_id)
    end
    return nil
end

return dispatch
`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeOptionalMethodCall)
	requireEvidenceMessage(t, diag, "receiver channel_info is optional at call to channel_info.handler")
	requireEvidenceMessage(t, diag, "no nil check proves receiver channel_info is present")
}

func TestCheckDynamicReverseMapReadInvalidatesProofAfterDirectOverwrite(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    chan: any,
    handler: (any, any, boolean, string) -> any,
}

local function dispatch(result: { channel: any, value: any, ok: boolean }, state: any): any
    local registered_channels: {[string]: ChannelInfo} = {}
    local channel_to_id: {[any]: string} = {}
    local channel_id = tostring(result.channel)

    registered_channels[channel_id] = { chan = result.channel, handler = function(inner_state, value, ok, id) return value end }
    channel_to_id[result.channel] = channel_id
    channel_to_id[result.channel] = "stale"

    local stored_id = channel_to_id[result.channel]
    if stored_id then
        local channel_info = registered_channels[stored_id]
        return channel_info.handler(state, result.value, result.ok, stored_id)
    end
    return nil
end

	return dispatch
`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeOptionalMethodCall)
	requireEvidenceMessage(t, diag, "receiver channel_info is optional at call to channel_info.handler")
	requireEvidenceMessage(t, diag, "no nil check proves receiver channel_info is present")
}

func TestCheckDynamicMapReadWithoutMembershipProofIsOptional(t *testing.T) {
	result := Check(`
type ChannelInfo = {
    handler: () -> any,
}

local function dispatch(channel_id: string): any
    local registered_channels: {[string]: ChannelInfo} = {}
    local channel_info = registered_channels[channel_id]
    return channel_info.handler()
end

return dispatch
`, WithStdlib())
	diag := requireDiagnosticCode(t, result, diagnostics.CodeOptionalMethodCall)
	requireEvidenceMessage(t, diag, "receiver channel_info is optional at call to channel_info.handler")
	requireEvidenceMessage(t, diag, "no nil check proves receiver channel_info is present")
}

func TestCheckColonMethodOptionalMapReturnContractAcceptsNilableRead(t *testing.T) {
	result := Check(`
type Decision = {kind: "allow"} | {kind: "deny"}
type Store = {
    state: {cached: {[string]: Decision}},
    lookup: (self: Store, key: string) -> Decision?,
}

local Store = {}
function Store:lookup(key: string): Decision?
    return self.state.cached[key]
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want optional method return to accept nilable map read", result.Diagnostics)
	}
}

func TestCheckImportedResponseBodyTostringFeedsDecode(t *testing.T) {
	responseType := typetable.NewRecord().
		Field("status_code", typ.Integer).
		OptField("body", typ.String).
		Build()
	http := manifest.New("http_client")
	getType := typ.Func().
		Param("url", typ.String).
		Returns(normalize.Optional(responseType), normalize.Optional(typ.String)).
		Build()
	http.SetExport(typetable.NewRecord().
		Field("get", getType).
		Build())
	http.DefineFunctionSignature("get", errorReturnSignature(getType))

	result := Check(`
local http = require("http_client")

local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function request(): (table?, string?)
    local response, err = http.get("https://example.test")
    if err then
        return nil, err
    end

    local parsed, parse_err = json.decode(tostring(response.body))
    if parse_err then
        return nil, parse_err
    end
    return parsed, nil
end
`, WithStdlib(), WithManifest("http_client", http))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want tostring(response.body) to satisfy decode string parameter", result.Diagnostics)
	}
}

func TestCheckImportedJsonDecodeAcceptsImportedResponseBodyFallback(t *testing.T) {
	responseType := typetable.NewRecord().
		Field("status_code", typ.Integer).
		OptField("body", typ.String).
		Build()
	http := manifest.New("http_client")
	postType := typ.Func().
		Param("url", typ.String).
		Param("options", typetable.NewRecord().Build()).
		Returns(normalize.Optional(responseType), normalize.Optional(typ.String)).
		Build()
	http.SetExport(typetable.NewRecord().
		Field("post", postType).
		Build())
	http.DefineFunctionSignature("post", errorReturnSignature(postType))
	jsonMod := CheckAndExport(`
local json = {}

function json.decode(src: string): (any, string?)
    return {}, nil
end

return json
`, "json", WithStdlib())
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json diagnostics = %#v", jsonMod.Errors)
	}

	result := Check(`
local http = require("http_client")
local json = require("json")

local function request(): (any, string?)
    local response, err = http.post("https://example.test", {})
    if err then
        return nil, err
    end

    local parsed, parse_err = json.decode(response.body or "")
    if parse_err then
        return nil, parse_err
    end
    return parsed, nil
end
`, WithStdlib(), WithManifest("http_client", http), WithModule("json", jsonMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported decode to accept optional response.body fallback", result.Diagnostics)
	}
}

func TestCheckImportedStringConstantFallbackDoesNotDriftToSiblingImportType(t *testing.T) {
	env := manifest.New("env")
	env.SetExport(typetable.NewRecord().
		Field("get", typ.Func().
			Param("name", typ.String).
			Returns(normalize.Optional(typ.String)).
			Build()).
		Build())
	store := manifest.New("store")
	store.SetExport(typetable.NewRecord().
		Field("get", typ.Func().
			Param("name", typ.String).
			Returns(typ.Any, normalize.Optional(typ.String)).
			Build()).
		Build())
	config := CheckAndExport(`
local env = require("env")
local store = require("store")

local config = {
    _env = env,
    _store = store,
}

config.DEFAULT_CACHE_ID = "app:cache"

function config.get_oauth2_token()
    return config._store.get(config._env.get("APP_CACHE") or config.DEFAULT_CACHE_ID)
end

return config
`, "config", WithStdlib(), WithManifest("env", env), WithManifest("store", store))
	if len(config.Errors) != 0 {
		t.Fatalf("config diagnostics = %#v", config.Errors)
	}

	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "imported constant remains string beside time import",
			src: `
local time = require("time")
local config = require("config")

local function read_constant(): string
    local time_now = time.now()
    local id: string = config.DEFAULT_CACHE_ID
    return id
end
`,
		},
		{
			name: "fallback expression remains string without later time return",
			src: `
local time = require("time")
local env = require("env")
local store = require("store")
local config = require("config")

local function refresh_token()
    local time_now = time.now()
    local store_instance, err = store.get(env.get("APP_CACHE") or config.DEFAULT_CACHE_ID)
    if err then
        return nil, err
    end
    return store_instance
end
`,
		},
		{
			name: "fallback expression remains string with later time return",
			src: `
local time = require("time")
local env = require("env")
local store = require("store")
local config = require("config")

local function refresh_token()
    local time_now = time.now()
    local store_instance, err = store.get(env.get("APP_CACHE") or config.DEFAULT_CACHE_ID)
    if err then
        return nil, err
    end
    return store_instance, time_now
end
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(tc.src, WithStdlib(), WithManifest("time", timeManifestForPrecisionTests()), WithManifest("env", env), WithManifest("store", store), WithManifest("config", config.Manifest))
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want imported string constant fallback to stay string beside time import", result.Diagnostics)
			}
		})
	}
}

func TestCheckPairsOverTypedMapAfterNilDeletionYieldsNonNilValue(t *testing.T) {
	t.Run("direct local map", func(t *testing.T) {
		result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local function terminate(session_id: string, session_info: ActiveSession): ()
end

local active_sessions = {} :: {[string]: ActiveSession}

local session_id = "s1"
active_sessions[session_id] = {
    pid = "pid",
    created_at = 1,
    terminating = false,
}
active_sessions[session_id] = nil

for id, session_info in pairs(active_sessions) do
    terminate(id, session_info)
end
`, WithStdlib())
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want pairs over typed map to yield non-nil values after deletion writes", result.Diagnostics)
		}
	})

	t.Run("nested table field map", func(t *testing.T) {
		result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local function terminate(session_id: string, session_info: ActiveSession): ()
end

local state = {
    active_sessions = {} :: {[string]: ActiveSession},
}

local session_id = "s1"
state.active_sessions[session_id] = {
    pid = "pid",
    created_at = 1,
    terminating = false,
}
state.active_sessions[session_id] = nil

for id, session_info in pairs(state.active_sessions) do
    terminate(id, session_info)
end
`, WithStdlib())
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want pairs over typed map to yield non-nil values after deletion writes", result.Diagnostics)
		}
	})
}

func TestCheckDynamicReadFromNestedTypedMapAfterNilDeletionNarrowsOnGuard(t *testing.T) {
	result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local function terminate(session_id: string, session_info: ActiveSession): ()
end

local state = {
    active_sessions = {} :: {[string]: ActiveSession},
}

local session_id = "s1"
state.active_sessions[session_id] = {
    pid = "pid",
    created_at = 1,
    terminating = false,
}
state.active_sessions[session_id] = nil

local session_info = state.active_sessions[session_id]
if session_info then
    terminate(session_id, session_info)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded dynamic read from nested typed map to recover element type", result.Diagnostics)
	}
}

func TestCheckTableInsertKeyListCarriesPairsKeyTypeIntoIPairs(t *testing.T) {
	t.Run("straight line insert", func(t *testing.T) {
		result := Check(`
local to_remove = {}
local active_id: string = "s1"
table.insert(to_remove, active_id)

for _, removed_id in ipairs(to_remove) do
    local copy: string = removed_id
end
`, WithStdlib())
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want table.insert value type to feed typed ipairs", result.Diagnostics)
		}
	})

	t.Run("loop-collected keys", func(t *testing.T) {
		result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local function terminate(session_id: string, session_info: ActiveSession): ()
end

local state = {
    active_sessions = {} :: {[string]: ActiveSession},
}

local session_id = "s1"
state.active_sessions[session_id] = {
    pid = "pid",
    created_at = 1,
    terminating = false,
}
state.active_sessions[session_id] = nil

local to_remove = {}
for active_id, session_info in pairs(state.active_sessions) do
    table.insert(to_remove, active_id)
end

for _, removed_id in ipairs(to_remove) do
    local session_info = state.active_sessions[removed_id]
    if session_info then
        terminate(removed_id, session_info)
    end
end
`, WithStdlib())
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want table.insert key list to feed typed ipairs and guarded map read", result.Diagnostics)
		}
	})
}

func TestCheckObjectLiteralPreservesUnknownValuedMembers(t *testing.T) {
	result := Check(`
local test = {
	event = {
		CASE_START = "test:case:start",
	},
}

local _default_context = {
	current_test = nil,
	ref_id = nil,
	target_pid = nil,
	message_topic = "test:update",
	send_message = function(kind, payload) end,
}

local _original_process_send: any = function(pid, topic, payload) end

local function update_send_message_function(): ()
	if _default_context.target_pid and _original_process_send then
		_default_context.send_message = function(kind, data)
			if _default_context.ref_id and not data.ref_id then
				data.ref_id = _default_context.ref_id
			end
			_original_process_send(_default_context.target_pid, _default_context.message_topic, {
				type = kind,
				data = data,
			})
		end
	end
end

update_send_message_function()

local function run_test(suite, test_case): ()
	local results = {}
	local result = {
		suite = suite.full_path or suite.name,
		name = test_case.name,
		status = "pending",
	}
	_default_context.current_test = test_case
	_default_context.send_message(test.event.CASE_START, {
		suite = result.suite,
		test = test_case.name,
	})
	result.status = "pass"
	_default_context.send_message(test.event.CASE_START, {
		suite = result.suite,
		test = test_case.name,
	})
	table.insert(results, result)

	for _, test_result in ipairs(results) do
        local current_suite: any = test_result.suite
        local current_name: any = test_result.name
	end
end

local suite = {
	name = "suite",
	full_path = "suite",
	tests = {
		{
			name = "case",
			fn = function() end,
		},
	},
}

run_test(suite, suite.tests[1])
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want object literal to preserve unknown-valued member presence", result.Diagnostics)
	}
}

func TestCheckObjectLiteralUnknownValuedMemberDoesNotSatisfyConcreteContract(t *testing.T) {
	result := Check(`
local function build(raw: any): { suite: string }
	local result = {
		suite = raw,
		status = "pending",
	}
	return result
end
`)
	if len(result.Diagnostics) == 0 {
		t.Fatal("diagnostics = none, want unknown-valued object member to remain unproven for concrete contract")
	}
	diag := result.Diagnostics[0]
	if !strings.Contains(diag.Message, "suite") || !strings.Contains(diag.Message, "string") {
		t.Fatalf("diagnostic = %#v, want suite/string contract failure", diag)
	}
}

func TestCheckObjectLiteralReturnWithConcreteCastedMemberValidates(t *testing.T) {
	result := Check(`
type Session = {
	session_id: string,
	status: string?,
}

type EnsureResult = {
	session: Session,
	created: boolean,
}

local function build(raw: any): (EnsureResult?, string?)
	local created_session = raw :: Session
	created_session.status = "idle"
	return {
		session = created_session,
		created = true,
	} :: EnsureResult, nil
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete casted member to validate returned object", result.Diagnostics)
	}
}

func TestCheckImportedConstantMutationKeepsTypedRecordInReturnObject(t *testing.T) {
	consts := CheckAndExport(`
local consts = {
    STATUS = {
        IDLE = "idle",
        RUNNING = "running",
    },
}

return consts
`, "consts")
	if len(consts.Errors) != 0 {
		t.Fatalf("consts module errors = %#v, want none", consts.Errors)
	}

	clean := Check(`
local consts = require("consts")

type Session = {
    session_id: string,
    user_id: string,
    status: string?,
    primary_context_id: string,
    title: string,
    kind: string,
    meta: {[string]: any},
    config: {[string]: any},
    public_meta: {[string]: any},
    start_date: string?,
    last_message_date: string?,
}

type EnsureSessionResult = {
    session: Session,
    created: boolean,
    recovered: boolean,
    primary_context_id: string,
}

local function create_session(row: Session, context_id: string): (EnsureSessionResult?, string?)
    local created_session = row
    created_session.status = consts.STATUS.IDLE

    return {
        session = created_session,
        created = true,
        recovered = false,
        primary_context_id = context_id,
    } :: EnsureSessionResult, nil
end
`, WithStdlib(), WithModule("consts", consts))
	if len(clean.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported constant mutation to preserve typed Session in returned object", clean.Diagnostics)
	}
}

func TestCheckAnyConcreteCastReturnedNestedObjectValidates(t *testing.T) {
	result := Check(`
type Session = {
    session_id: string,
    user_id: string,
    status: string?,
    primary_context_id: string,
    title: string,
    kind: string,
    meta: {[string]: any},
    config: {[string]: any},
    public_meta: {[string]: any},
    start_date: string?,
    last_message_date: string?,
}

type EnsureSessionResult = {
    session: Session,
    created: boolean,
    recovered: boolean,
    primary_context_id: string,
}

local function create_session(row: any, context_id: string): (EnsureSessionResult?, string?)
    local created_session = row :: Session

    return {
        session = created_session,
        created = true,
        recovered = false,
        primary_context_id = context_id,
    } :: EnsureSessionResult, nil
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nested concrete casts to validate returned object", result.Diagnostics)
	}
}

func TestCheckNestedOptionalArrayLengthGuardProvesFirstElementReceiver(t *testing.T) {
	result := Check(`
type FilePart = {
    name: (self: FilePart) -> string?,
    header: (self: FilePart, key: string) -> string?,
    size: (self: FilePart) -> number?,
}

type MultipartForm = {
    files: { file: {FilePart}? }?,
}

local function handle(form: MultipartForm): ()
    if not form.files or not form.files.file or #form.files.file == 0 then
        return
    end

    local file_part = form.files.file[1]
    local filename = file_part:name() or "unknown"
    local mime_type = file_part:header("Content-Type") or "application/octet-stream"
    local file_size = file_part:size() or 0
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want length guard to prove first element receiver", result.Diagnostics)
	}
}

func TestCheckTableInsertLengthGuardProvesFirstElementConcat(t *testing.T) {
	result := Check(`
local function format_one(name: string?): string
    local names = {}
    if name then
        table.insert(names, name)
    end
    if #names == 1 then
        return "File processed: " .. names[1]
    end
    return "none"
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table.insert plus length guard to prove first element", result.Diagnostics)
	}
}

func TestCheckModuloLengthIndexUsesModuleTableStaticArrayField(t *testing.T) {
	result := Check(`
local term = {}

function term.cyan(s: string): string
    return s
end

term.spinner_frames = {"a", "b", "c"}

function term.spinner(index: integer): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return term.cyan(frame :: string)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want module-table static array field to preserve element type through modulo index", result.Diagnostics)
	}
}

func TestCheckModuloLengthIndexRejectsFractionalNumberIndex(t *testing.T) {
	result := Check(`
local term = {}

function term.cyan(s: string): string
    return s
end

term.spinner_frames = {"a", "b", "c"}

function term.spinner(index: number): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return term.cyan(frame)
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{"cannot pass frame", "may be nil"},
	})
}

func TestCheckDottedFunctionAnnotatedParameterReturnIsPresent(t *testing.T) {
	result := Check(`
local term = {}

function term.cyan(s: string): string
    return s
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want dotted function annotated parameter to be present inside body", result.Diagnostics)
	}
}

func TestCheckCapturedModuleTableStaticArrayFieldLiteralIndex(t *testing.T) {
	result := Check(`
local term = {}

term.spinner_frames = {"a", "b", "c"}

function term.spinner(): string
    local frame = term.spinner_frames[1]
    return frame
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured module-table static array literal index to preserve element type", result.Diagnostics)
	}
}

func TestCheckCapturedModuleTableStaticArrayFieldLength(t *testing.T) {
	result := Check(`
local term = {}

term.spinner_frames = {"a", "b", "c"}

function term.frame_count(): integer
    return #term.spinner_frames
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured module-table static array length to be integer", result.Diagnostics)
	}
}

func TestCheckDeclaredArrayReturnContextualizesEmptyAccumulator(t *testing.T) {
	result := Check(`
local function collect(out: {any}, raw: any): ()
    if raw ~= nil then
        out[#out + 1] = raw
    end
end

local function from_trait(trait_def: any): {any}
    local out = {}
    if type(trait_def) ~= "table" then
        return out
    end

    collect(out, trait_def.bindings)
    collect(out, trait_def.binding)
    return out
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want declared array return to contextualize empty accumulator passed to helper", result.Diagnostics)
	}
}

func TestCheckTypeGuardedAnyFieldCastReturnsMapAlias(t *testing.T) {
	result := Check(`
type Options = {[string]: any}

local function process_options(data: any): Options
    if type(data) ~= "table" or type(data.options) ~= "table" then
        return {}
    end

    return data.options :: Options
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want type-guarded any field cast to satisfy map alias return", result.Diagnostics)
	}
}

func TestCheckShortCircuitRHSMemberReadsUseGuardEnvironment(t *testing.T) {
	result := Check(`
type AgentRef = string | { id: string, name: string? }

local function target_id(agent_identifier: AgentRef): string?
    return type(agent_identifier) == "table"
        and (agent_identifier.id or agent_identifier.name)
        or agent_identifier
end
`)
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeMissingMember) {
		t.Fatalf("diagnostics = %#v, want short-circuit RHS reads checked under the LHS type guard", result.Diagnostics)
	}
}

func TestCheckMathFloorDoesNotLaunderNumberWidthToIntegerRepeatCount(t *testing.T) {
	result := Check(`
local function progress_bar(current: number, total: number, width: number?): string
    width = width or 20
    local filled = math.floor((current / total) * width)
    local empty = width - filled
    return string.rep("x", filled) .. string.rep(".", empty)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"argument 2 (empty)",
			"number",
			"not integer",
		},
	})
}

func TestCheckIntegerProgressBarWidthSatisfiesRepeatCounts(t *testing.T) {
	result := Check(`
local function progress_bar(current: number, total: number, width: integer?): string
    width = width or 20
    local filled = math.floor((current / total) * width)
    local empty = width - filled
    return string.rep("x", filled) .. string.rep(".", empty)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want integer width arithmetic to satisfy string.rep integer counts", result.Diagnostics)
	}
}

func TestCheckLengthOneGuardProvesInsertedStringArrayFirstElement(t *testing.T) {
	result := Check(`
local function build(filename: string?): string
    local file_names = {}
    if filename then
        table.insert(file_names, filename)
    end
    if #file_names == 1 then
        return "File processed: " .. file_names[1]
    end
    return ""
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want #array == 1 to prove inserted first element present for concat", result.Diagnostics)
	}
}

func TestCheckLogicalAndRhsSuppressesConcatNilWarningForTruthyAny(t *testing.T) {
	result := Check(`
local function notify(stage_title: any): string
    return stage_title and ("stage: " .. stage_title) or ""
end
local function notify_untyped(stage_title): string
    return stage_title and ("stage: " .. stage_title) or ""
end
local pipeline_lib = {}
function pipeline_lib.notify_status_change(upload, status, stage_title, error_msg): string
    return stage_title and ("stage: " .. stage_title) or ""
end
function pipeline_lib.notify_with_print(upload, status, stage_title, error_msg)
    if not upload or not upload.user_id or not upload.uuid then
        return
    end
    local user_topic = string.format("user.%s", upload.user_id)
    local upload_topic = string.format("upload:%s", upload.uuid)
    local notification = {
        uuid = upload.uuid,
        status = status,
        timestamp = time.now():format_rfc3339()
    }
    if stage_title then
        notification.stage = stage_title
    end
    if error_msg then
        notification.error = error_msg
    end
    process.send(user_topic, upload_topic, notification)
    print("Sent notification to", user_topic, "about upload", upload.uuid, "status:", status, stage_title and ("stage: " .. stage_title) or "")
end

function pipeline_lib.process_upload(upload, stages)
    pipeline_lib.notify_with_print(upload, "processing")
    pipeline_lib.notify_with_print(upload, "error", nil, "failed")
    for i, stage in ipairs(stages) do
        local stage_title = stage.title or ("Stage " .. i)
        pipeline_lib.notify_with_print(upload, "processing", stage_title)
        pipeline_lib.notify_with_print(upload, "error", stage_title, "failed")
    end
    pipeline_lib.notify_with_print(upload, "completed")
end
`, WithStdlib())
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeConcatOperand {
			t.Fatalf("diagnostics = %#v, want logical-and rhs to prove concat operand non-nil", result.Diagnostics)
		}
	}
}

func TestCheckInsertedObjectLiteralArrayPreservesStringField(t *testing.T) {
	result := Check(`
local function cyan(s: string): string
    return s
end

local function run(entries)
    local failures = {}
    for _, entry in ipairs(entries) do
        local name: string = "test"
        local suite: string = "default"
        local label = suite .. "/" .. name
        table.insert(failures, { label = label, error = entry.error })
    end
    for _, f in ipairs(failures) do
        cyan(f.label)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table.insert object-literal element field type to survive ipairs", result.Diagnostics)
	}
}

func TestCheckReturnedMutatedAccumulatorShapeSurvivesHelperAndCast(t *testing.T) {
	result := Check(`
type Response = {
    result: string,
    tokens: {},
}

local function normalize_response()
    local normalized = {
        tokens = {},
    }
    normalized.result = "ok"
    return normalized
end

local function generate(): Response?
    local normalized = normalize_response()
    if not normalized then
        return nil
    end
    return normalized :: Response
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want helper-returned mutated table shape to satisfy explicit cast", result.Diagnostics)
	}
}

func TestCheckConditionallyMutatedAccumulatorCastValidatesRequiredField(t *testing.T) {
	result := Check(`
type Response = {
    result: any,
    tokens: {},
}

local function normalize_response(raw_result: any)
    if not raw_result then
        return nil
    end

    local normalized = {
        tokens = raw_result.tokens or {},
    }

    if raw_result.result then
        normalized.result = raw_result.result
    elseif raw_result.content then
        normalized.result = raw_result.content
    end

    return normalized
end

local function generate(raw_result: any): Response?
    local normalized = normalize_response(raw_result)
    if not normalized then
        return nil
    end
    return normalized :: Response
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete response cast to validate required field at runtime", result.Diagnostics)
	}
}

func TestCheckTypeGuardElseBranchAliasKeepsExcludedTableVariant(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "direct parameter use",
			src: `
type AgentRef = string | table

local function resolve_agent(id: string): ()
end

local function load_agent(agent_spec_or_id: AgentRef): ()
    if type(agent_spec_or_id) == "table" then
        return
    else
        resolve_agent(agent_spec_or_id)
    end
end
`,
		},
		{
			name: "branch-local alias",
			src: `
type AgentRef = string | table

local function resolve_agent(id: string): ()
end

local function load_agent(agent_spec_or_id: AgentRef): ()
    if type(agent_spec_or_id) == "table" then
        return
    else
        local agent_identifier = agent_spec_or_id
        resolve_agent(agent_identifier)
    end
end
`,
		},
		{
			name: "predeclared alias mirrors wippy load_agent",
			src: `
type AgentRef = string | table

local function resolve_agent(id: string): ()
end

local function load_agent(agent_spec_or_id: AgentRef): ()
    local agent_identifier
    if type(agent_spec_or_id) == "table" then
        agent_identifier = "inline-agent"
    else
        agent_identifier = agent_spec_or_id
        resolve_agent(agent_identifier)
    end
end
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(tc.src)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want else branch of type(x) == \"table\" to narrow alias to string", result.Diagnostics)
			}
		})
	}
}

func TestCheckTypeGuardBranchMergeDropsPredeclaredAliasNilWhenBothBranchesAssign(t *testing.T) {
	result := Check(`
type InlineAgent = { id: string?, name: string? }
type AgentRef = string | InlineAgent

local function load_agent(agent_spec_or_id: AgentRef): string
    local agent_identifier
    if type(agent_spec_or_id) == "table" then
        agent_identifier = agent_spec_or_id.id or agent_spec_or_id.name or "inline-agent"
    else
        agent_identifier = agent_spec_or_id
    end

    return "Failed to load agent '" .. agent_identifier .. "'"
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want both assigned branches to prove agent_identifier is a string after merge", result.Diagnostics)
	}
}

func TestCheckTypeGuardBranchMergeDropsOpenTableFieldNilAfterLiteralFallback(t *testing.T) {
	result := Check(`
type AgentRef = string | table

local function load_agent(agent_spec_or_id: AgentRef): string
    local raw_spec
    local agent_identifier
    if type(agent_spec_or_id) == "table" then
        raw_spec = agent_spec_or_id
        agent_identifier = raw_spec.id or raw_spec.name or "inline-agent"
    else
        agent_identifier = agent_spec_or_id
    end

	return "Failed to load agent '" .. agent_identifier .. "'"
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, assignment value = %s, want literal fallback to remove nil from open-table field chain after branch merge",
			result.Diagnostics,
			localAssignmentSourceDebugAtLine(t, result, 9))
	}
}

func TestCheckTypeGuardedAnyCastValidatesClosedRecordFields(t *testing.T) {
	result := Check(`
type Point = { x: number }

local function process_point(data: any): Point
    if type(data) ~= "table" then
        return { x = 0 }
    end

    return data :: Point
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete record cast to validate return shape", result.Diagnostics)
	}
}

func TestCheckCallbackBetweenAppendAndLastReadKeepsStableLocalArrayElement(t *testing.T) {
	result := Check(`
type MigrationItem = { description: string }
type MigrationContext = {
    current_migration: MigrationItem?,
    implementations: {MigrationItem},
}

local function define(context: MigrationContext, fn: () -> ()): MigrationItem
    local item: MigrationItem = { description = "create users" }
    context.current_migration = item

    local success, err = pcall(fn)
    if not success then
        error("bad migration: " .. tostring(err))
    end

	table.insert(context.implementations, item)
	context.current_migration = nil

    return context.implementations[#context.implementations] :: MigrationItem
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want required array field and stable inserted local to remain precise across opaque callback", result.Diagnostics)
	}
}

func TestCheckTableInsertRejectsOptionalValueForConcreteArrayElement(t *testing.T) {
	result := Check(`
type MigrationItem = { description: string }
type MigrationContext = {
    implementations: {MigrationItem},
}

local function define(context: MigrationContext, maybe_item: MigrationItem?): MigrationItem
    table.insert(context.implementations, maybe_item)
    return context.implementations[#context.implementations] :: MigrationItem
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code: diagnostics.CodeDirectCallArgType,
		MessageContains: []string{
			"argument 2",
			"maybe_item",
			"may be nil",
		},
	})
}

func TestCheckUnprovenLastArrayReadCastValidatesNilAway(t *testing.T) {
	result := Check(`
type MigrationItem = { description: string }
type MigrationContext = {
    implementations: {MigrationItem},
}

local function last(context: MigrationContext): MigrationItem
    return context.implementations[#context.implementations] :: MigrationItem
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete cast to validate optional array read on the normal path", result.Diagnostics)
	}
}

func TestCheckMixedPreciseAndUntrustedObjectReturnBlamesFieldProof(t *testing.T) {
	src := `
type Spec = {
    id: string,
    prompt: string,
    context: {[string]: any},
}

local function build(data: any): Spec
    local spec = {
        id = "agent",
        prompt = data.prompt,
        context = data.context or {},
    }
    return spec :: Spec
end
`
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"returned value 1.prompt",
			"comes from any/unknown",
			"no proof shows it satisfies declared return type string",
		},
		EvidenceContains: []string{
			"returned value 1.prompt",
			"returned value 1.prompt (data.prompt) comes from any/unknown",
			"no proof on this path",
		},
		RenderOrderedContains: []string{
			"prompt = data.prompt",
			"returned value 1.prompt",
			"returned value 1.prompt (data.prompt) comes from any/unknown",
			"no proof on this path",
		},
		RenderNotContains: []string{
			"object literal does not provide field \"prompt\"",
			"missing required field prompt",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
	})
}

func TestCheckDefaultedUntrustedObjectReturnBlamesFieldProof(t *testing.T) {
	src := `
type Spec = {
    id: string,
    name: string,
    prompt: string,
    context: {[string]: any},
}

local function build(data: any, meta: {name: string?}): Spec
    local spec = {
        id = "agent",
        name = meta.name or "",
        prompt = (data and data.prompt) or "",
        context = (data and data.context) or {},
    }
    return spec :: Spec
end
`
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"returned value 1.prompt",
			"comes from any/unknown",
			"no proof shows it satisfies declared return type string",
		},
		EvidenceContains: []string{
			"returned value 1.prompt",
			"comes from any/unknown",
			"no proof on this path",
		},
		RenderOrderedContains: []string{
			"prompt = (data and data.prompt) or \"\"",
			"returned value 1.prompt",
			"comes from any/unknown",
			"no proof on this path",
		},
		RenderNotContains: []string{
			"object literal does not provide field \"prompt\"",
			"missing required field prompt",
			"returned value 1 (spec) is {id:",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
	})
}

func TestCheckReturnedConfigDefaultedStringFieldSurvivesConcatAndCall(t *testing.T) {
	result := Check(`
type HTTP = {
    get: (url: string, options: table) -> ()
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local function resolve_string(key: string, default_env: string?): string?
    if default_env then
        return default_env
    end
    return nil
end

local function resolve_config()
    local ctx_all: {[string]: any} = {}
    local config = {
        api_key = resolve_string("api_key", "API_KEY"),
        base_url = resolve_string("base_url", "BASE_URL") or "https://api.example.test",
        timeout = 600,
        headers = ctx_all.headers,
    }
    return config
end

local function request(endpoint_path: string): ()
    local config = resolve_config()
    local full_url = config.base_url .. endpoint_path
    http.get(full_url, { timeout = config.timeout })
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want defaulted returned config.base_url to stay string through concat and call", result.Diagnostics)
	}
}

func TestCheckCapturedDefaultedConfigStringSurvivesConcatAndCall(t *testing.T) {
	result := Check(`
type HTTP = {
    get: (url: string, options: table) -> ()
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local function resolve_string(key: string, default_env: string?): string?
    if default_env then
        return default_env
    end
    return nil
end

local function resolve_config()
    local ctx_all: {[string]: any} = {}
    local config = {
        api_key = resolve_string("api_key", "API_KEY"),
        base_url = resolve_string("base_url", "BASE_URL") or "https://api.example.test",
        timeout = 600,
        headers = ctx_all.headers,
    }
    return config
end

local function request(endpoint_path: string): ()
    local config = resolve_config()
    local full_url = config.base_url .. endpoint_path
    local opts = { timeout = config.timeout }
    local function send_once()
        return http.get(full_url, opts)
    end
    send_once()
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured defaulted config string to stay string through nested call", result.Diagnostics)
	}
}

func TestCheckDefaultedConfigStringSurvivesUntrustedContextTable(t *testing.T) {
	result := Check(`
type HTTP = {
    get: (url: string, options: table) -> ()
}
type Ctx = {
    all: () -> any,
}
type Env = {
    get: (name: string) -> string?,
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local client = {
    _http_client = http,
    _ctx = ({ all = function(): any return nil end } :: Ctx),
    _env = ({ get = function(name: string): string? return nil end } :: Env),
}

local function normalize_retry(raw): table?
    if type(raw) ~= "table" then return nil end
    return raw
end

local function resolve_config()
    local ctx_all = client._ctx.all() or {}

    local function resolve_string(key: string, default_env: string?): string?
        if ctx_all[key] then
            return tostring(ctx_all[key])
        end
        local env_key = key .. "_env"
        if ctx_all[env_key] then
            local val = client._env.get(tostring(ctx_all[env_key]))
            if val and val ~= "" then return val end
        end
        if default_env then
            local val = client._env.get(default_env)
            if val and val ~= "" then return val end
        end
        return nil
    end

    local config = {
        api_key = resolve_string("api_key", "API_KEY"),
        base_url = resolve_string("base_url", "BASE_URL") or "https://api.example.test",
        timeout = tonumber(resolve_string("timeout", "TIMEOUT")) or 600,
        retry = normalize_retry(ctx_all.retry),
        headers = ctx_all.headers,
    }
    return config
end

local function request(endpoint_path: string): ()
    local config = resolve_config()
    local full_url = config.base_url .. endpoint_path
    local http_options = { timeout = config.timeout }
    local function send_once()
        return client._http_client.get(full_url, http_options)
    end
    send_once()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want defaulted config string to stay string despite unrelated untrusted context fields", result.Diagnostics)
	}
}

func TestCheckBodyUsageParamObligationFromCapturedConcatOperand(t *testing.T) {
	result := Check(`
type HTTP = {
    get: (url: string, options: table) -> ()
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local function request(endpoint_path): ()
    local base_url = "https://api.example.test"
    local full_url = base_url .. endpoint_path
    local function send_once()
        return http.get(full_url, {})
    end
    send_once()
end

request("/models")
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unannotated endpoint_path to be seeded from call-site/body obligation before nested call", result.Diagnostics)
	}
}

func TestCheckBodyUsageParamObligationFromUncalledCapturedConcatOperand(t *testing.T) {
	result := Check(`
type HTTP = {
    get: (url: string, options: table) -> ()
}

local http: HTTP = {
    get = function(url: string, options: table): () end
}

local client = {}

function client.request(endpoint_path): ()
    local base_url = "https://api.example.test"
    local full_url = base_url .. endpoint_path
    local function send_once()
        return http.get(full_url, {})
    end
    send_once()
end

return client
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want uncalled table-field function body to use its own param obligations before nested call", result.Diagnostics)
	}
}

func TestCheckInsertedMessageRowsPreserveContentMemberThroughIPairs(t *testing.T) {
	result := Check(`
local function merge_messages(contract_messages): ()
    local converse_messages = {}
    for _, source_msg in ipairs(contract_messages) do
        if source_msg.role == "function_result" then
            local result_text = type(source_msg.content) == "string" and source_msg.content or ""
            table.insert(converse_messages, {
                role = "user",
                content = {
                    {
                        toolResult = {
                            toolUseId = source_msg.function_call_id or "",
                            content = { { text = result_text } },
                            status = "success",
                        },
                    },
                },
            })
        else
            local content_blocks = {}
            local content = source_msg.content
            if type(content) == "string" then
                table.insert(content_blocks, { text = content })
            elseif type(content) == "table" then
                for _, part in ipairs(content) do
                    if part.type == "text" then
                        table.insert(content_blocks, { text = part.text })
                    end
                end
            end
            if #content_blocks > 0 then
                table.insert(converse_messages, {
                    role = "user",
                    content = content_blocks,
                })
            end
        end
    end

    if #converse_messages > 0 then
        table.insert(converse_messages, {
            role = "user",
            content = { { text = "tail" } },
        })
    end

    local consolidated = {}
    for _, msg in ipairs(converse_messages) do
        if #consolidated > 0 and consolidated[#consolidated].role == msg.role then
            for _, block in ipairs(msg.content) do
                table.insert(consolidated[#consolidated].content, block)
            end
        else
            table.insert(consolidated, msg)
        end
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inserted message rows to preserve content member through ipairs", result.Diagnostics)
	}
}

func TestCheckTableCreateAccumulatorWidensInsertedMessageRoles(t *testing.T) {
	result := Check(`
local prompt = {
    ROLE = {
        SYSTEM = "system",
        CACHE_MARKER = "cache_marker",
    }
}

local function consume(messages: any[]): ()
end

local function build_messages(id: string, conversation_messages: any[]): ()
    local final_message_count = 2 + #conversation_messages
    local final_messages = table.create(final_message_count, 0)
    local system_message = {
        role = prompt.ROLE.SYSTEM,
        content = { "ready" },
    }
    table.insert(final_messages, system_message)
    table.insert(final_messages, { role = prompt.ROLE.CACHE_MARKER, marker_id = id })
    consume(final_messages)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table.create accumulator to widen across inserted message roles", result.Diagnostics)
	}
}

func TestCheckDeclaredAccumulatorStillRejectsWrongInsertedRole(t *testing.T) {
	result := Check(`
type SystemMessage = { role: "system" }

local function build_messages(): ()
    local final_messages: {SystemMessage} = {}
    table.insert(final_messages, { role = "cache_marker" })
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"argument 2.role",
			"cache_marker",
			"not",
			"system",
		},
	})
}

func TestCheckAnnotatedStringReceiverSurvivesRepeatedMatchCalls(t *testing.T) {
	result := Check(`
local function dependency_kind(dep_id: string): string
    if not dep_id:match(":") then
        return "bootloader"
    end

    local namespace = dep_id:match("^([^:]+):")
    if namespace and namespace:match("%.") then
        return "bootloader"
    end
    return "service"
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want annotated string receiver and guarded match result to support repeated match calls", result.Diagnostics)
	}
}

func TestCheckTableFieldFunctionValueSeesOwnOptionalParameter(t *testing.T) {
	result := Check(`
local prompt = {
    ROLE = {
        DEVELOPER = "developer",
    }
}

local builder = {}

builder.add_message = function(self: any, role: string, content_parts: any[], name: string?, metadata: table?): any
    return self
end

builder.add_developer = function(self: any, content: string, meta: table?): any
    if content and #content > 0 then
        return self:add_message(
            prompt.ROLE.DEVELOPER,
            { content },
            nil,
            meta
        )
    end
    return self
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table-field function value body to see its own optional parameter", result.Diagnostics)
	}
}

func TestCheckCustomPredicateRefinesOptionalRegistryEntryAtCaller(t *testing.T) {
	result := Check(`
type RegistryEntry = {
    id: string,
    meta: {
        type: string?,
    }?,
}

local function is_valid_agent(entry: RegistryEntry?): boolean?
    return entry and entry.meta and entry.meta.type == "agent.gen1"
end

local function entry_id(entry: RegistryEntry): string
    return entry.id
end

local function get_id(entry: RegistryEntry?): string?
    if not is_valid_agent(entry) then
        return nil
    end

    return entry_id(entry)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want custom predicate truthy return to refine optional registry entry at caller", result.Diagnostics)
	}
}

func TestCheckCustomPredicateRefinesOptionalRegistryEntryInTruthyBranch(t *testing.T) {
	result := Check(`
type RegistryEntry = {
    id: string,
    meta: {
        type: string?,
    }?,
}

local function is_valid_agent(entry: RegistryEntry?): boolean?
    return entry and entry.meta and entry.meta.type == "agent.gen1"
end

local function entry_id(entry: RegistryEntry): string
    return entry.id
end

local function get_id(entry: RegistryEntry?): string?
    if is_valid_agent(entry) then
        return entry_id(entry)
    end
    return nil
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want custom predicate truthy branch to refine optional registry entry at caller", result.Diagnostics)
	}
}

func TestCheckBedrockMapMessagesRowsAllExposeContentMember(t *testing.T) {
	promptMod := CheckAndExport(`
type ContentPart = {
    type: string,
    text: string?,
    source: any?,
    arguments: any?,
    id: string?,
    name: string?,
}

type FunctionCall = {
    name: string,
    arguments: string,
    id: string?,
}

type Message = {
    role: string,
    content: {ContentPart}?,
    name: string?,
    metadata: table?,
    function_call: FunctionCall?,
    function_call_id: string?,
}

local prompt = {}
prompt.ROLE = {
        SYSTEM = "system",
        DEVELOPER = "developer",
        FUNCTION_RESULT = "function_result",
        FUNCTION_CALL = "function_call",
        ASSISTANT = "assistant",
    }
return prompt
`, "prompt", WithStdlib())
	if len(promptMod.Errors) != 0 {
		t.Fatalf("prompt module errors = %#v, want none", promptMod.Errors)
	}

	result := Check(`
local prompt = require("prompt")

type Message = {
    role: string,
    content: any?,
    metadata: table?,
    function_call: any?,
    function_call_id: string?,
}

local function normalize_tool_arguments(arguments)
    return arguments or {}
end

local function convert_image_to_converse(content_part)
    if content_part.type == "image" then
        return { image = { source = content_part.source } }
    end
    return nil
end

local function convert_document_to_converse(content_part)
    if content_part.type == "document" then
        return { document = { source = content_part.source } }
    end
    return nil
end

local function map_messages(contract_messages)
    if not contract_messages or #contract_messages == 0 then
        return { messages = {}, system = nil }
    end

    local converse_messages = {}
    local system_blocks = {}

    for _, msg in ipairs(contract_messages) do
        if msg.role == prompt.ROLE.SYSTEM then
            if type(msg.content) == "string" then
                table.insert(system_blocks, { text = msg.content })
            elseif type(msg.content) == "table" then
                for _, part in ipairs(msg.content) do
                    if part.type == "text" then
                        table.insert(system_blocks, { text = part.text })
                    end
                end
            end
        elseif msg.role == "cache_marker" then
            if #converse_messages == 0 and #system_blocks > 0 then
                table.insert(system_blocks, { cachePoint = { type = "default" } })
            elseif #converse_messages > 0 then
                local last_msg = converse_messages[#converse_messages]
                if last_msg.content and #last_msg.content > 0 then
                    table.insert(last_msg.content, { cachePoint = { type = "default" } })
                end
            end
        elseif msg.role == prompt.ROLE.DEVELOPER then
            local dev_text = type(msg.content) == "string" and msg.content or
                (type(msg.content) == "table" and msg.content[1] and msg.content[1].text) or ""

            if dev_text ~= "" then
                table.insert(system_blocks, { text = dev_text })
            end
        elseif msg.role == prompt.ROLE.FUNCTION_RESULT then
            local result_text = type(msg.content) == "string" and msg.content or
                (type(msg.content) == "table" and msg.content[1] and msg.content[1].text) or ""

            table.insert(converse_messages, {
                role = "user",
                content = {
                    {
                        toolResult = {
                            toolUseId = msg.function_call_id or "",
                            content = { { text = result_text } },
                            status = "success",
                        },
                    },
                },
            })
        elseif msg.role == prompt.ROLE.FUNCTION_CALL then
            local arguments = normalize_tool_arguments(msg.function_call.arguments)
            local content_blocks = {}

            if msg.metadata and msg.metadata.thinking_blocks then
                for _, thinking_block in ipairs(msg.metadata.thinking_blocks) do
                    if thinking_block.type == "thinking" then
                        table.insert(content_blocks, {
                            reasoningContent = {
                                reasoningText = {
                                    text = thinking_block.thinking or "",
                                    signature = thinking_block.signature or "",
                                },
                            },
                        })
                    end
                end
            end

            table.insert(content_blocks, {
                toolUse = {
                    toolUseId = msg.function_call.id or "",
                    name = msg.function_call.name or "",
                    input = arguments,
                },
            })

            table.insert(converse_messages, {
                role = "assistant",
                content = content_blocks,
            })
        elseif msg.role == prompt.ROLE.ASSISTANT then
            local content_blocks = {}

            if msg.metadata and msg.metadata.thinking_blocks then
                for _, thinking_block in ipairs(msg.metadata.thinking_blocks) do
                    if thinking_block.type == "thinking" then
                        table.insert(content_blocks, {
                            reasoningContent = {
                                reasoningText = {
                                    text = thinking_block.thinking or "",
                                    signature = thinking_block.signature or "",
                                },
                            },
                        })
                    end
                end
            end

            local content = msg.content
            if type(content) == "string" then
                if content ~= "" then
                    table.insert(content_blocks, { text = content })
                end
            elseif type(content) == "table" then
                for _, part in ipairs(content) do
                    if part.type == "text" and part.text and part.text ~= "" then
                        table.insert(content_blocks, { text = part.text })
                    elseif part.type == "function_call" then
                        local args = normalize_tool_arguments(part.arguments)
                        table.insert(content_blocks, {
                            toolUse = {
                                toolUseId = part.id or "",
                                name = part.name or "",
                                input = args,
                            },
                        })
                    elseif part.type == "image" then
                        local img = convert_image_to_converse(part)
                        if img then
                            table.insert(content_blocks, img)
                        end
                    elseif part.type == "document" then
                        local doc = convert_document_to_converse(part)
                        if doc then
                            table.insert(content_blocks, doc)
                        end
                    end
                end
            end

            if #content_blocks > 0 then
                table.insert(converse_messages, {
                    role = "assistant",
                    content = content_blocks,
                })
            end
        else
            local content_blocks = {}
            local content = msg.content
            if type(content) == "string" then
                table.insert(content_blocks, { text = content })
            elseif type(content) == "table" then
                for _, part in ipairs(content) do
                    if part.type == "text" and part.text then
                        table.insert(content_blocks, { text = part.text })
                    elseif part.type == "image" then
                        local img = convert_image_to_converse(part)
                        if img then
                            table.insert(content_blocks, img)
                        end
                    elseif part.type == "document" then
                        local doc = convert_document_to_converse(part)
                        if doc then
                            table.insert(content_blocks, doc)
                        end
                    end
                end
            end

            if #content_blocks > 0 then
                table.insert(converse_messages, {
                    role = "user",
                    content = content_blocks,
                })
            end
        end
    end

    local consolidated = {}
    for _, msg in ipairs(converse_messages) do
        if #consolidated > 0 and consolidated[#consolidated].role == msg.role then
            for _, block in ipairs(msg.content) do
                table.insert(consolidated[#consolidated].content, block)
            end
        else
            table.insert(consolidated, msg)
        end
    end

    return {
        messages = consolidated,
        system = system_blocks,
    }
end
`, WithStdlib(), WithModule("prompt", promptMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want Bedrock mapper inserted rows to expose content member after consolidation", result.Diagnostics)
	}
}

func TestCheckStringMatchResultCastSatisfiesDecodeArgument(t *testing.T) {
	result := Check(`
local json = {}

function json.decode(s: string): (any, string?)
    return {}, nil
end

local function parse_tool_call(text: string): any?
    local unwrapped = text:match("^<tool_call>%s*(.-)%s*</tool_call>$") or text
    local json_str = unwrapped:match("^(%b{})")
    if not json_str then
        return nil
    end
    local parsed, decode_err = json.decode(json_str :: string)
    if decode_err then
        return nil
    end
    return parsed
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want explicit string cast on match result to satisfy decode argument", result.Diagnostics)
	}
}

func TestRequireCheckStringMatchResultCastSatisfiesImportedDecodeArgument(t *testing.T) {
	jsonMod := CheckAndExport(`
local json = {}

function json.decode(s: string): (any, string?)
    return {}, nil
end

return json
`, "json", WithStdlib())
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	result := Check(`
local json = require("json")

local function parse_tool_call(text: string): any?
    local unwrapped = text:match("^<tool_call>%s*(.-)%s*</tool_call>$") or text
    local json_str = unwrapped:match("^(%b{})")
    if not json_str then
        return nil
    end
    local parsed, decode_err = json.decode(json_str :: string)
    if decode_err then
        return nil
    end
    return parsed
end
`, WithStdlib(), WithModule("json", jsonMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want explicit string cast on match result to satisfy imported decode argument", result.Diagnostics)
	}
}

func TestCheckStringMatchFallbackReassignmentSatisfiesDecodeArgument(t *testing.T) {
	result := Check(`
local json = {}

function json.decode(s: string): (any, string?)
    return {}, nil
end

local function parse_tool_call(text: string): any?
    local stripped = text:gsub("^%s+", ""):gsub("%s+$", "")
    local unwrapped = stripped:match("^<tool_call>%s*(.-)%s*</tool_call>$")
        or stripped:match("^<function_call>%s*(.-)%s*</function_call>$")
        or stripped

    local json_str = unwrapped:match("^(%b{})")
    if not json_str then
        local start_idx = unwrapped:find("{", 1, true)
        if start_idx then
            json_str = unwrapped:sub(start_idx):match("^(%b{})")
        end
    end

    if not json_str then
        return nil
    end

    local parsed, decode_err = json.decode(json_str :: string)
    if decode_err then
        return nil
    end
    return parsed
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded fallback match reassignment to satisfy decode argument", result.Diagnostics)
	}
}

func TestCheckStringGsubReplacementCallbackCaptureIsString(t *testing.T) {
	result := Check(`
local function split_string(str: string, sep: string): {string}
    local fields: {string} = {}
    local pattern = string.format("([^%s]+)", sep)
    str:gsub(pattern, function(c) fields[#fields + 1] = c end)
    return fields
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want gsub capture callback parameter typed as string", result.Diagnostics)
	}
}

func TestCheckReturnedFunctionLiteralUsesUniqueCallableUnionReturnArm(t *testing.T) {
	result := Check(`
type Handler = (value: string) -> string
type MaybeHandler = Handler | false

local function make_handler(): MaybeHandler
    return function(value)
        local s: string = value
        return s
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want returned function literal parameter to use unique callable union return arm", result.Diagnostics)
	}
}

func TestCheckLoopAssignedLocalTruthyGuardSuppressesConcatNilWarning(t *testing.T) {
	result := Check(`
local function f(parts: {any}): string?
    local refusal = nil
    for _, part in ipairs(parts) do
        if part.type == "refusal" and part.refusal then
            refusal = part.refusal
        end
    end
    if refusal then
        return "Request was refused: " .. refusal
    end
    return nil
end
`, WithStdlib())
	for _, d := range result.Diagnostics {
		if d.Code == diagnostics.CodeConcatOperand {
			t.Fatalf("diagnostics = %#v, want truthy guard on loop-assigned local to suppress concat nil warning", result.Diagnostics)
		}
	}
}

func TestCheckLoopAssignedLocalTruthyGuardSuppressesConcatNilWarningInChainedCall(t *testing.T) {
	result := Check(`
type Builder = {
    message: (self: Builder, text: string) -> Builder,
    build: (self: Builder) -> string,
}

local err: Builder

local function f(parts: {any}): string?
    local refusal = nil
    for _, part in ipairs(parts) do
        if part.type == "refusal" and part.refusal then
            refusal = part.refusal
        end
    end
    if refusal then
        return err
            :message("Request was refused: " .. refusal)
            :build()
    end
    return nil
end
`, WithStdlib())
	for _, d := range result.Diagnostics {
		if d.Code == diagnostics.CodeConcatOperand {
			t.Fatalf("diagnostics = %#v, want truthy guard on loop-assigned local to suppress concat nil warning inside chained call", result.Diagnostics)
		}
	}
}

func sqlWrapperManifest() *manifest.Manifest {
	luaErrorType := typ.NewInterface("Error", []typ.Method{
		{
			Name: "message",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.String).
				Build(),
		},
	})
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().
				Param("self", typ.Self).
				Build(),
		},
	})
	executorType := typ.NewInterface("sql.Executor", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.NewArray(typetable.NewRecord().Build()), typeexpr.Optional(typ.String)).
				Build(),
		},
	})
	queryType := typ.NewInterface("sql.Query", []typ.Method{
		{
			Name: "run_with",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("db", dbType).
				Returns(executorType).
				Build(),
		},
	})
	builderType := typetable.NewRecord().
		Field("select", typ.Func().
			Variadic(typ.String).
			Returns(queryType).
			Build()).
		Build()
	getType := typ.Func().
		Param("name", typ.String).
		Returns(typeexpr.Optional(dbType), typeexpr.Optional(luaErrorType)).
		Build()
	m := manifest.New("sql")
	m.SetExport(typetable.NewRecord().
		Field("get", getType).
		Field("builder", builderType).
		Build())
	m.DefineFunctionSignature("get", errorReturnSignature(getType))
	return m
}

func contractWrapperManifest() *manifest.Manifest {
	instanceType := typ.NewInterface("contract.Instance", []typ.Method{
		{
			Name: "list",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("filter", typ.Any).
				Returns(typ.Any, typeexpr.Optional(typ.String)).
				Build(),
		},
	})
	openType := typ.Func().
		Param("self", typ.Self).
		Returns(instanceType, typeexpr.Optional(typ.String)).
		Build()
	contractType := typ.NewInterface("contract.Contract", []typ.Method{
		{Name: "open", Type: openType},
	})
	getType := typ.Func().
		Param("name", typ.String).
		Returns(contractType, typeexpr.Optional(typ.String)).
		Build()

	m := manifest.New("contract")
	m.SetExport(typetable.NewRecord().Field("get", getType).Build())
	m.DefineFunctionSignature("contract.get", errorReturnSignature(getType))
	m.DefineFunctionSignature("contract.Contract.open", errorReturnSignature(openType))
	return m
}

func timeManifestForPrecisionTests() *manifest.Manifest {
	m := manifest.New("time")
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("t", typ.Self).Returns(durationType).Build()},
	})
	m.DefineType("Time", timeType)
	m.DefineType("Duration", durationType)
	m.SetExport(typetable.NewRecord().
		Field("now", typ.Func().Returns(timeType).Build()).
		Build())
	return m
}

func hasOperationalNormalReturnTypeRefinement(sig signature.Function, paramIndex int, want typ.Type) bool {
	if sig.OperationalEffects == nil {
		return false
	}
	for _, refinement := range sig.OperationalEffects.NormalReturnTypeRefinements {
		if refinement.Path.PlaceholderIndex() == paramIndex && typ.TypeEquals(refinement.Type, want) {
			return true
		}
	}
	return false
}

func errorReturnSignature(t *typ.Function) signature.Function {
	return signature.Function{
		Type:   t,
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}
}

func TestCheckReturnedMapKeyPreservesValuePresenceForLaterRead(t *testing.T) {
	result := Check(`
type Time = {
    before: (self: Time, other: Time) -> boolean,
}

type ActiveSession = {
    created_at: Time,
    last_activity: Time?,
    terminating: boolean,
}

local function terminate(_id: string, _session: ActiveSession?): ()
end

local function run(): ()
    local state = {
        active_sessions = {} :: {[string]: ActiveSession},
    }

    local function get_oldest_session()
        local oldest_id = nil
        local oldest_time = nil

        for session_id, session_info in pairs(state.active_sessions) do
            if not session_info.terminating then
                local last_activity = session_info.last_activity or session_info.created_at
                if not oldest_time or last_activity:before(oldest_time) then
                    oldest_time = last_activity
                    oldest_id = session_id
                end
            end
        end

        return oldest_id
    end

    local oldest_id = get_oldest_session()
    if not oldest_id then
        return
    end

    terminate(oldest_id, state.active_sessions[oldest_id])
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want returned key selected from map pairs to preserve the later map value type", result.Diagnostics)
	}
}

func TestCheckLengthComparisonOfGuardedDynamicTableReturnsBoolean(t *testing.T) {
	result := Check(`
local function has_checkpoint_bindings(bindings: any): boolean
    if type(bindings) ~= "table" then
        return false
    end
    if type(bindings.checkpoint) == "table" then
        bindings = bindings.checkpoint
    end
    return #bindings > 0
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want length comparison to be typed as boolean after table guards", result.Diagnostics)
	}
}

func TestCheckTypeGuardedFieldAssignmentPreservesTableAcrossMerge(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    if type(bindings.checkpoint) == "table" then
        bindings = bindings.checkpoint
    end
    return bindings
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want type(field) == table guard to prove reassigned binding remains table", result.Diagnostics)
	}
}

func TestCheckTypeGuardedAnyRootReturnsTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    return bindings
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want type(root) ~= table early return to prove continuation root is table", result.Diagnostics)
	}
}

func TestCheckTypeGuardedAnyFieldReturnsTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    if type(bindings.checkpoint) == "table" then
        return bindings.checkpoint
    end
    return {}
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want type(field) == table guard to prove guarded field read is table", result.Diagnostics)
	}
}

func TestCheckTypeGuardedAnyFieldLocalAliasReturnsTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    if type(bindings.checkpoint) == "table" then
        local selected = bindings.checkpoint
        return selected
    end
    return {}
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local alias of guarded field read to remain table", result.Diagnostics)
	}
}

func TestCheckTypeGuardedAnyFieldAliasThenRootAssignmentReturnsTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    if type(bindings.checkpoint) == "table" then
        local selected = bindings.checkpoint
        bindings = selected
    end
    return bindings
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want root assignment from local alias of guarded field to remain table", result.Diagnostics)
	}
}

func TestCheckDescendantTypeGuardDoesNotDegradeGuardedRootTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    if type(bindings.checkpoint) == "table" then
    end
    return bindings
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want descendant type guard not to erase root table proof", result.Diagnostics)
	}
}

func TestCheckRootAnyReassignmentFromKnownTableAliasReturnsTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    local selected = {}
    bindings = selected
    return bindings
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want root any reassignment from known table alias to return table", result.Diagnostics)
	}
}

func TestCheckConditionalRootAnyReassignmentFromKnownTableAliasReturnsTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    if type(bindings.checkpoint) == "table" then
        local selected = {}
        bindings = selected
    end
    return bindings
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want conditional root reassignment from known table alias to preserve table at merge", result.Diagnostics)
	}
}

func TestCheckTypeGuardedAnyFieldRootAssignmentBeforeReturn(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    if type(bindings.checkpoint) == "table" then
        local selected = bindings.checkpoint
        bindings = selected
        return bindings
    end
    return {}
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want root assignment from guarded any-origin field to be table before merge", result.Diagnostics)
	}
}

func TestCheckTypeGuardedAnyFieldAssignmentToDifferentAnyRootReturnsTable(t *testing.T) {
	result := Check(`
local function select_bindings(bindings: any): table
    if type(bindings) ~= "table" then
        return {}
    end
    local out: any = {}
    if type(bindings.checkpoint) == "table" then
        local selected = bindings.checkpoint
        out = selected
        return out
    end
    return {}
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want assignment from guarded any-origin field alias into different any root to be table", result.Diagnostics)
	}
}
