package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCheckExportedNestedConstantTableSurvivesRequire(t *testing.T) {
	consts := CheckAndExport(`
local consts = {}

consts.topic = {
    IMAGE_BUILD_LOG = "image.build.log",
    IMAGE_BUILD_STATUS = "image.build.status",
}

return consts
`, "consts", WithStdlib())
	if len(consts.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want none", consts.Errors)
	}

	result := Check(`
local consts = require("consts")

local function notify(topic: string): ()
end

notify(consts.topic.IMAGE_BUILD_STATUS)
notify(consts.topic.IMAGE_BUILD_LOG)
`, WithStdlib(), WithModule("consts", consts))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for exported nested constant table", result.Diagnostics)
	}
}

func TestCheckTypeGuardNarrowsUnknownLocalCopyBeforeJsonDecode(t *testing.T) {
	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function normalize_tool_arguments(raw_arguments: unknown): table
    local arguments = raw_arguments
    if type(arguments) == "string" then
        local parsed, parse_err = json.decode(arguments)
        if not parse_err and type(parsed) == "table" then
            arguments = parsed
        else
            arguments = { value = arguments }
        end
    end
    if not arguments or type(arguments) ~= "table" then
        arguments = { run = true }
    end
    return arguments
end

local function map_message(msg: {function_call: {arguments: unknown}}): table
    return normalize_tool_arguments(msg.function_call.arguments)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for type-guarded local copy", result.Diagnostics)
	}
}

func TestCheckImportedTypeAssertRefinesMemberPath(t *testing.T) {
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

local function check(result: {err: any}): boolean
    assert.is_string(result.err, "error must be string, got " .. type(result.err))
    local hit = string.find(result.err, "not allowed", 1, true)
    return hit ~= nil
end
`, WithStdlib(), WithModule("assert2", assertMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported type assertion to refine result.err to string", result.Diagnostics)
	}
}

func TestCheckImportedTypeAssertSurvivesUnrelatedModuleDiagnostic(t *testing.T) {
	assertMod := CheckAndExport(`
local M = {}

function M.is_string(value, msg)
    if type(value) ~= "string" then
        error(msg or "expected string", 2)
    end
    return value
end

function M.unrelated_bad_helper(err, substr)
    local actual_msg = type(err) == "table" and err.message or tostring(err)
    return string.find(actual_msg, substr, 1, true)
end

return M
`, "assert2", WithStdlib())
	if len(assertMod.Errors) == 0 {
		t.Fatalf("assert module unexpectedly clean; fixture must keep an unrelated dirty helper")
	}

	result := Check(`
local assert = require("assert2")

local function check(result: {err: any}): boolean
    assert.is_string(result.err, "error must be string")
    local hit = string.find(result.err, "not allowed", 1, true)
    return hit ~= nil
end
`, WithStdlib(), WithModule("assert2", assertMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want clean helper effect to survive unrelated diagnostics in same imported module", result.Diagnostics)
	}
}

func TestCheckReboundImportedTypeAssertRefinesMemberPath(t *testing.T) {
	assertMod := CheckAndExport(`
local M = {}

function M.is_string(value, msg)
    if type(value) ~= "string" then
        error(msg or "expected string", 2)
    end
    return value
end

return M
`, "app.lib:assert", WithStdlib())
	if len(assertMod.Errors) != 0 {
		t.Fatalf("assert module diagnostics = %#v, want clean helper export", assertMod.Errors)
	}

	result := Check(`
local assert = require("assert2")

local function check(result: {err: any}): boolean
    assert.is_string(result.err, "error must be string, got " .. type(result.err))
    local hit = string.find(result.err, "not allowed", 1, true)
    return hit ~= nil
end
`, WithStdlib(), WithManifest("assert2", assertMod.Manifest.Rebound("assert2")))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want rebound imported type assertion to refine result.err to string", result.Diagnostics)
	}
}

func TestCheckRuntimeFunctionMemberGuardProvesCallable(t *testing.T) {
	result := Check(`
local function prepare(prompt_input: table): any
    if type(prompt_input.build) == "function" then
        local prompt_result = prompt_input:build()
        return prompt_result
    elseif type(prompt_input.get_messages) == "function" then
        return prompt_input:get_messages()
    end
    return nil
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want runtime function member guard to prove method call is callable", result.Diagnostics)
	}
}

func TestCheckExplicitAnyCastToScalarIsAcceptedAsUserAssertion(t *testing.T) {
	result := Check(`
local function need_string(value: string): boolean
    return string.find(value, "x", 1, true) ~= nil
end

local ok = need_string((123 :: any) :: string)
local nil_ok = need_string((nil :: any) :: string)
local table_ok = need_string(({} :: any) :: string)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar type assertion to satisfy string parameter while retaining untrusted origin", result.Diagnostics)
	}
}

func TestCheckScalarCastOfLiteralIsRuntimeValidation(t *testing.T) {
	result := Check(`
local function need_string(value: string): boolean
    return value ~= ""
end

local nil_ok = need_string(nil :: string)
local number_ok = need_string(123 :: string)
local boolean_ok = need_string(false :: string)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar casts of literals to be accepted as runtime validation", result.Diagnostics)
	}
}

func TestCheckInferredObjectFieldSurvivesSiblingDynamicWrite(t *testing.T) {
	result := Check(`
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(16, 0),
        pending_routes = table.create(8, 0),
    }, flow_graph_mt)
end

local graph = FlowGraph.new()
table.insert(graph.pending_routes, {
    condition = nil,
})
graph.pending_routes[#graph.pending_routes].condition = true
local commands = table.create(#graph.node_order * 2, 0)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inferred node_order length to survive sibling dynamic write", result.Diagnostics)
	}
}

func TestCheckTableCreateInsertKeepsHomogeneousRecordElementShape(t *testing.T) {
	result := Check(`
local leaf_nodes = table.create(8, 0)
table.insert(leaf_nodes, {
    node_id = "node-1",
    has_success_route = false,
    has_error_route = true,
    metadata = {},
})

for _, leaf_info in ipairs(leaf_nodes) do
    local li = leaf_info
    if li.has_error_route and not li.has_success_route then
        local title = li.metadata and li.metadata.title or "unnamed"
        table.insert({}, string.format("%s (%s)", title, li.node_id:sub(1, 12)))
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table.create + table.insert to preserve record element shape", result.Diagnostics)
	}
}

func TestCheckGuardedUnknownMemberLengthProvesInteger(t *testing.T) {
	result := Check(`
local function collect(template)
    if not template or not template.operations then
        return table.create(0, 0)
    end
    local ids = table.create(#template.operations, 0)
    return ids
end

collect({ operations = table.create(2, 0) })
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded member length to satisfy table.create capacity", result.Diagnostics)
	}
}

func TestCheckRecursiveTemplateMethodKeepsGuardedOperationsLength(t *testing.T) {
	result := Check(`
local compiler = {}
compiler.OP_TYPES = {
    FUNC = "func",
    CYCLE = "cycle",
}

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        nodes = table.create(0, 4),
        edges = table.create(0, 4),
        node_order = table.create(4, 0),
    }, flow_graph_mt)
end

function FlowGraph:create_template_nodes(template, parent_node_id)
    if not template or not template.operations then
        return table.create(0, 0)
    end

    local template_node_ids = table.create(#template.operations, 0)
    local last_template_node_id = nil

    for _, op in ipairs(template.operations) do
        if op.type == compiler.OP_TYPES.FUNC then
            local template_node_id = "fn"
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                config = {
                    func_id = op.config.func_id,
                },
                parent_node_id = parent_node_id,
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0),
            }
            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)
            last_template_node_id = template_node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local template_node_id = "cycle"
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                config = {
                    func_id = op.config.func_id,
                },
                parent_node_id = parent_node_id,
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0),
            }
            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)

            if op.config.template then
                local cycle_template_nodes = self:create_template_nodes(op.config.template, template_node_id)
                for _, child_id in ipairs(cycle_template_nodes) do
                    table.insert(template_node_ids, child_id)
                end
            end

            last_template_node_id = template_node_id
        end
    end

    if last_template_node_id then
        local last_node = self.nodes[last_template_node_id]
        if not last_node.config.data_targets then
            last_node.config.data_targets = table.create(1, 0)
        end
    end

    return template_node_ids
end

local graph = FlowGraph.new()
graph:create_template_nodes({
    operations = {
        { type = "cycle", config = { func_id = "f", template = { operations = {} } } },
    },
}, "parent")
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want recursive template method guard to preserve operations length proof", result.Diagnostics)
	}
}

func TestCheckOptionalParamNilGuardReturnNarrowsBeforeCall(t *testing.T) {
	result := Check(`
local process = {}

function process.terminate(pid: string): (boolean, string?)
    return pid ~= "", nil
end

local function terminate_best_effort(pid: string?)
    if pid == nil then
        return
    end
    process.terminate(pid)
end

local function main()
    local started_pid: string? = nil
    terminate_best_effort(started_pid)
	end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want early-return nil guard to narrow optional parameter before call", result.Diagnostics)
	}
}

func TestCheckOptionalParamNilGuardReturnNarrowsBeforeManifestMemberCall(t *testing.T) {
	process := manifest.New("process")
	process.DefineGlobal("process")
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})
	eventType := typetable.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		Build()
	messageChannelType := typ.Instantiate(ambient.ChannelGeneric(), messageType)
	eventChannelType := typ.Instantiate(ambient.ChannelGeneric(), eventType)
	process.DefineType("Message", messageType)
	process.DefineType("Event", eventType)
	process.SetExport(typetable.NewRecord().
		Field("event", typetable.NewRecord().Field("EXIT", typ.String).Build()).
		Field("inbox", typ.Func().Returns(messageChannelType).Build()).
		Field("events", typ.Func().Returns(eventChannelType).Build()).
		Field("terminate", typ.Func().
			Param("pid", typ.String).
			Returns(typ.Boolean, typeexpr.Optional(typ.Any)).
			Build()).
		Build())

	result := Check(`
type Message = process.Message
type MessageChannel = Channel<Message>
type Event = process.Event
type EventChannel = Channel<Event>

local function wait_for_exit(events_ch: EventChannel, pid: string)
    local result = channel.select {
        events_ch:case_receive(),
    }
    local event = result.value as Event
    if event.from == pid and event.kind == process.event.EXIT then
        return event, nil
    end
    return nil, "missing"
end

local function terminate_best_effort(pid: string?)
    if pid == nil then
        return
    end
    process.terminate(pid)
end

local function main()
    local started_pid: string? = nil
    terminate_best_effort(started_pid)
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithManifest("process", process), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want early-return nil guard to narrow optional parameter before manifest member call", result.Diagnostics)
	}
}

func TestCheckTableMemberFunctionKeepsDeclaredParameterContract(t *testing.T) {
	result := Check(`
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

type Context = {
    current_migration: MigrationItem?,
    current_database: DatabaseImpl?,
    implementations: {MigrationItem},
}

local M = {}

function M.create_migration_fn(context: Context): any
    return function()
        context.current_migration = {
            description = "ok",
            database_implementations = {},
        }
        table.insert(context.implementations, context.current_migration)
    end
end

function M.install(context: Context)
    _G.migration = M.create_migration_fn(context)
end

M.install({ current_migration = nil, current_database = nil, implementations = {} })
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table-member function body to trust its declared parameter contract", result.Diagnostics)
	}
}

func TestCheckNestedReturnedRecordPreservesOptionalAnyMemberAndSiblingFields(t *testing.T) {
	result := Check(`
local output = {}

output.TYPE = {
    ERROR = "error",
}

output.ERROR_TYPE = {
    SERVER_ERROR = "server_error",
}

type ErrorInfo = {
    type: string,
    message: string,
    code: any?,
}

type OutputChunk = {
    type: string,
    content: string?,
    error: ErrorInfo?,
    name: string?,
    arguments: string?,
    id: string?,
    meta: table?,
    usage: UsageInfo?,
}

type Streamer = {
    pid: string,
    topic: string,
    send_error: (self: Streamer, type: string, message: string, code: any?) -> boolean,
}

type UsageInfo = {
    prompt_tokens: number,
    completion_tokens: number,
    thinking_tokens: number,
    cache_write_tokens: number,
    cache_read_tokens: number,
    total_tokens: number,
}

function output.error(err_type: string, message: string, code: any?): OutputChunk
    return {
        type = output.TYPE.ERROR,
        error = {
            type = err_type or output.ERROR_TYPE.SERVER_ERROR,
            message = message or "Unknown error",
            code = code,
        },
    }
end

function output.streamer(pid: string, topic: string): Streamer
    local streamer = {
        pid = pid,
        topic = topic,
    }
    local target_pid = tostring(pid)
    local target_topic = tostring(topic)
    streamer.send_error = function(self: Streamer, err_type: string, message: string, code: any?): boolean
        local chunk: OutputChunk = output.error(err_type, message, code)
        return chunk.type == output.TYPE.ERROR and target_pid ~= "" and target_topic ~= ""
    end
    return streamer :: Streamer
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nested returned record to preserve optional any member and sibling fields", result.Diagnostics)
	}
}

func TestCheckFreshPartialTableStaysPresentBeforeReturnCast(t *testing.T) {
	result := Check(`
local output = {}

type Streamer = {
    pid: string,
    topic: string,
    buffer: string,
    buffer_size: number,
    send_content: (self: Streamer, text: string) -> boolean,
}

function output.streamer(pid: string?, topic: string?, buffer_size: number?): (Streamer?, string?)
    if not pid then
        return nil, "PID is required for streamer"
    end

    local streamer = {
        pid = pid,
        topic = topic or "llm_response",
        buffer = "",
        buffer_size = buffer_size or 10,
    }

    local target_pid = tostring(pid)
    local target_topic = tostring(topic or "llm_response")

    streamer.send_content = function(self: Streamer, text: string): boolean
        return target_pid ~= "" and target_topic ~= "" and text ~= ""
    end

    return streamer :: Streamer
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want fresh partial table to stay non-nil while methods are attached before return cast", result.Diagnostics)
	}
}

func TestCheckTypeCallReturnsExactRuntimeKindLiteral(t *testing.T) {
	result := Check(`
local tag: "string" = type("x")
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want type(\"x\") to return exact \"string\" literal", result.Diagnostics)
	}
}

func TestCheckTypeNameComparisonUsesPriorDiscriminantNarrowing(t *testing.T) {
	result := Check(`
type IntCell  = { kind: "number",  raw: number | string | boolean }
type TextCell = { kind: "string",  raw: number | string | boolean }
type FlagCell = { kind: "boolean", raw: number | string | boolean }
type Cell = IntCell | TextCell | FlagCell

local function flip(b: boolean): boolean return not b end

local function render(cell: Cell): string
    if cell.kind == "number" and type(cell.raw) == cell.kind then
        return "n"
    elseif cell.kind == "string" and type(cell.raw) == cell.kind then
        return cell.raw
    elseif cell.kind == "boolean" and type(cell.raw) == cell.kind then
        if flip(cell.raw) then
            return "t"
        end
        return "f"
    end
    return "?"
end

return #render({ kind = "string", raw = "x" })
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want type(value) == discriminant to use the prior literal discriminant narrowing", result.Diagnostics)
	}
}

func TestCheckTypeNameComparisonClosesContradictorySpecializedRecord(t *testing.T) {
	result := Check(`
local function flip(b: boolean): boolean return not b end

local cell: { kind: string, raw: string } = { kind = "string", raw = "x" }
if cell.kind == "boolean" and type(cell.raw) == cell.kind then
    flip(cell.raw)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want contradictory type-name guard to close the branch", result.Diagnostics)
	}
}

func TestCheckSeparateMetatableIndexMethodSurfaceOnImplicitSelf(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }

type NodeInstance = {
    node_id: string,
}

local function new_node()
    local instance: NodeInstance = { node_id = "n1" }
    return setmetatable(instance, mt)
end

function methods:data(data_type: string, content: unknown): (NodeInstance, string?)
    return self, nil
end

function methods:_route_errors(error_content: unknown): ()
    self:data("error", error_content)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want separate __index methods table to surface data on implicit self", result.Diagnostics)
	}
}

func TestCheckMetatableMethodLengthOfTypedSelfArrayStaysInteger(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }

type Target = {
    id: string,
}

type NodeInstance = {
    targets: {Target},
}

local function new_node()
    local instance: NodeInstance = { targets = {} }
    return setmetatable(instance, mt)
end

function methods:complete(): ()
    local out = table.create(#self.targets, 0)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want length of typed implicit-self array to satisfy integer parameter", result.Diagnostics)
	}
}

func TestCheckMetatableMethodLengthOfAndOrFallbackSelfArrayStaysInteger(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }

type Target = {
    id: string,
}

type Config = {
    targets: {Target}?,
}

type NodeDefinition = {
    config: Config?,
}

type NodeArgs = {
    node: NodeDefinition?,
}

type NodeInstance = {
    targets: {Target},
}

local function new_node(args: NodeArgs)
    local instance: NodeInstance = {
        targets = (args.node and args.node.config and args.node.config.targets) or ({} :: {Target}),
    }
    return setmetatable(instance, mt)
end

function methods:complete(): ()
    local out = table.create(#self.targets, 0)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want and/or fallback self array field to satisfy length integer parameter", result.Diagnostics)
	}
}

func TestCheckMetatableMethodLengthOfTableCreateFallbackSelfArrayStaysInteger(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }

type Target = {
    id: string,
}

type Config = {
    targets: {Target}?,
}

type NodeDefinition = {
    config: Config?,
}

type NodeArgs = {
    node: NodeDefinition?,
}

type NodeInstance = {
    node: NodeDefinition,
    targets: {Target},
    metadata: {[string]: unknown},
}

local function new_node(args: NodeArgs)
    local instance: NodeInstance = {
        node = args.node or {},
        targets = (args.node and args.node.config and args.node.config.targets) or (table.create(0, 0) :: {Target}),
        metadata = (args.node and args.node.metadata) or {},
    }
    return setmetatable(instance, mt)
end

function methods:complete(): ()
    local out = table.create(#self.targets, 0)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table.create fallback self array field to satisfy length integer parameter", result.Diagnostics)
	}
}

func TestCheckModuleMetatableMethodLengthOfSelfArrayStaysInteger(t *testing.T) {
	result := Check(`
local node = {}
local methods = {}
local mt = { __index = methods }

type Target = {
    id: string,
}

type Config = {
    targets: {Target}?,
}

type NodeDefinition = {
    config: Config?,
    metadata: {[string]: unknown}?,
}

type NodeArgs = {
    node_id: string,
    node: NodeDefinition?,
}

type NodeInstance = {
    node_id: string,
    node: NodeDefinition,
    targets: {Target},
    metadata: {[string]: unknown},
}

function node.new(args: NodeArgs)
    local instance: NodeInstance = {
        node_id = args.node_id,
        node = args.node or {},
        targets = (args.node and args.node.config and args.node.config.targets) or (table.create(0, 0) :: {Target}),
        metadata = (args.node and args.node.metadata) or {},
    }
    return setmetatable(instance, mt), nil
end

function methods:complete(message, extra_metadata)
    local out = table.create(#self.targets, 0)
    return out
end

return node
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want module metatable method self array length to satisfy integer parameter", result.Diagnostics)
	}
}

func TestCheckMethodCallInvalidationPreservesUnrelatedSelfArrayField(t *testing.T) {
	result := Check(`
local node = {}
local methods = {}
local mt = { __index = methods }

type Target = {
    id: string,
}

type NodeInstance = {
    targets: {Target},
    metadata: {[string]: unknown},
}

function node.new()
    local instance: NodeInstance = {
        targets = {},
        metadata = {},
    }
    return setmetatable(instance, mt)
end

function methods:update_metadata(updates)
    for k, v in pairs(updates) do
        self.metadata[k] = v
    end
    return self, nil
end

function methods:complete(extra_metadata)
    if extra_metadata then
        local _, err = self:update_metadata(extra_metadata)
        if err then
            return nil
        end
    end
    local out = table.create(#self.targets, 0)
    return out
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want method call invalidation to preserve unrelated self.targets array type", result.Diagnostics)
	}
}

func TestCheckMetadataMethodCallPreservesConstructorFallbackArrayField(t *testing.T) {
	result := Check(`
local node = {}
local methods = {}
local mt = { __index = methods }

type RouteTarget = {
    id: string,
}

type NodeConfig = {
    data_targets: {RouteTarget}?,
}

type NodeDefinition = {
    config: NodeConfig?,
    metadata: {[string]: unknown}?,
}

type NodeArgs = {
    node_id: string,
    node: NodeDefinition?,
}

type NodeInstance = {
    node_id: string,
    node: NodeDefinition,
    _metadata: {[string]: unknown},
    _queued_commands: {unknown},
    data_targets: {RouteTarget},
}

local function merge_metadata(base, updates)
    return base
end

function node.new(args: NodeArgs)
    local instance: NodeInstance = {
        node_id = args.node_id,
        node = args.node or {},
        data_targets = (args.node and args.node.config and args.node.config.data_targets) or (table.create(0, 0) :: {RouteTarget}),
        _metadata = (args.node and args.node.metadata) or {},
        _queued_commands = table.create(10, 0),
    }
    return setmetatable(instance, mt), nil
end

function methods:update_metadata(updates)
    if not updates or type(updates) ~= "table" then
        return self, nil
    end
    self._metadata = merge_metadata(self._metadata, updates)
    table.insert(self._queued_commands, { metadata = self._metadata })
    return self, nil
end

function methods:complete(message)
    if message then
        local _, err = self:update_metadata({ status_message = message })
        if err then
            return nil
        end
    end
    local out = table.create(#self.data_targets, 0)
    return out
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want metadata update to preserve constructor fallback data_targets array type", result.Diagnostics)
	}
}

func TestCheckMetadataAndSubmitMethodsPreserveConstructorFallbackArrayFields(t *testing.T) {
	result := Check(`
local node = {}
local methods = {}
local mt = { __index = methods }

type RouteTarget = {
    id: string,
}

type Commit = {
    submit: (dataflow_id: string, op_id: string, commands: {unknown}) -> (boolean, string?),
}

type NodeDeps = {
    commit: Commit,
}

type NodeConfig = {
    data_targets: {RouteTarget}?,
    error_targets: {RouteTarget}?,
}

type NodeDefinition = {
    config: NodeConfig?,
    metadata: {[string]: unknown}?,
}

type NodeArgs = {
    node_id: string,
    dataflow_id: string,
    node: NodeDefinition?,
}

type NodeInstance = {
    node_id: string,
    dataflow_id: string,
    node: NodeDefinition,
    _metadata: {[string]: unknown},
    _queued_commands: {unknown},
    data_targets: {RouteTarget},
    error_targets: {RouteTarget},
    _deps: NodeDeps,
}

local default_deps: NodeDeps = {
    commit = {
        submit = function(_dataflow_id: string, _op_id: string, _commands: {unknown}): (boolean, string?)
            return true, nil
        end,
    },
}

local uuid = { v7 = function(): string return "id" end }

local function merge_metadata(base, updates)
    return base
end

function node.new(args: NodeArgs, deps: NodeDeps?)
    local effective_deps: NodeDeps = deps or default_deps
    local instance: NodeInstance = {
        node_id = args.node_id,
        dataflow_id = args.dataflow_id,
        node = args.node or {},
        data_targets = (args.node and args.node.config and args.node.config.data_targets) or (table.create(0, 0) :: {RouteTarget}),
        error_targets = (args.node and args.node.config and args.node.config.error_targets) or (table.create(0, 0) :: {RouteTarget}),
        _metadata = (args.node and args.node.metadata) or {},
        _queued_commands = table.create(10, 0),
        _deps = effective_deps,
    }
    return setmetatable(instance, mt), nil
end

function methods:update_metadata(updates)
    if not updates or type(updates) ~= "table" then
        return self, nil
    end
    self._metadata = merge_metadata(self._metadata, updates)
    table.insert(self._queued_commands, { metadata = self._metadata })
    return self, nil
end

function methods:_submit_final()
    local result, err = self._deps.commit.submit(self.dataflow_id, uuid.v7(), self._queued_commands)
    self._queued_commands = table.create(10, 0)
    return result ~= nil, err
end

function methods:complete(output_content, message, extra_metadata)
    if extra_metadata then
        local _, meta_err = self:update_metadata(extra_metadata)
        if meta_err then
            return nil
        end
    end
    if message then
        local _, msg_err = self:update_metadata({ status_message = message })
        if msg_err then
            return nil
        end
    end
    local data_ids = table.create(#self.data_targets, 0)
    local success, err = self:_submit_final()
    if not success then
        return nil, err
    end
    return data_ids, nil
end

function methods:fail(error_details, message, extra_metadata)
    if extra_metadata then
        self:update_metadata(extra_metadata)
    end
    local data_ids = table.create(#self.error_targets, 0)
    local success, err = self:_submit_final()
    if not success then
        return nil, err
    end
    return data_ids, nil
end

return node
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want metadata and submit methods to preserve constructor fallback array fields", result.Diagnostics)
	}
}

func TestCheckFieldFunctionCallContextSeesConstructorReturnShape(t *testing.T) {
	result := Check(`
local table = table

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(4, 0),
    }, flow_graph_mt)
end

local compiler = {}

function compiler.build_graph()
    local graph = FlowGraph.new()
    return graph, nil
end

function compiler.compile_to_commands(graph, session_context)
    local commands = table.create(#graph.node_order * 2, 0)
    return commands, nil
end

function compiler.compile()
    local graph, graph_err = compiler.build_graph()
    if graph_err then
        return nil, graph_err
    end
    return compiler.compile_to_commands(graph, nil)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want field-defined call context to see constructor return shape", result.Diagnostics)
	}
}

func TestCheckLoopReceiverMutationReturningOtherValuesPreservesConstructorFields(t *testing.T) {
	result := Check(`
local table = table
local uuid = {}

function uuid.v7(): string
    return "id"
end

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        operations = table.create(16, 0),
        nodes = table.create(0, 4),
        node_order = table.create(4, 0),
        edges = table.create(0, 4),
        references = table.create(0, 4),
        input_data = nil,
        input_name = nil,
        input_routes = table.create(4, 0),
        static_data_sources = table.create(4, 0),
        last_node_id = nil,
        last_static_id = nil,
        last_node_name = nil,
        last_route_from_static = false,
        pending_routes = table.create(4, 0),
        has_explicit_routing = false,
        session_parent_id = nil,
        forced_success_nodes = table.create(0, 4),
        forced_failure_nodes = table.create(0, 4),
        auto_chained = table.create(0, 4),
    }, flow_graph_mt)
end

function FlowGraph:create_node(node_type, config)
    local node_id = uuid.v7()
    self.nodes[node_id] = {
        node_id = node_id,
        node_type = node_type,
        config = config,
    }
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    self.last_node_id = node_id
    self.last_static_id = nil
    return node_id, nil
end

function FlowGraph:create_static_data(data)
    local static_id = uuid.v7()
    table.insert(self.static_data_sources, {
        static_id = static_id,
        data = data,
    })
    self.last_static_id = static_id
    self.last_node_id = nil
    return static_id, nil
end

function FlowGraph:create_template_nodes(template, parent_node_id)
    if not template or not template.operations then
        return table.create(0, 0)
    end
    local template_node_ids = table.create(#template.operations, 0)
    for _, template_op in ipairs(template.operations) do
        local template_node_id, err = self:create_node("template", template_op.config)
        if not err then
            table.insert(template_node_ids, template_node_id)
        end
    end
    return template_node_ids
end

function FlowGraph:add_operation(op_type, config)
    table.insert(self.operations, {
        type = op_type,
        config = config or {},
    })
    return self, nil
end

local compiler = {}

function compiler.build_graph(operations)
    if not operations or #operations == 0 then
        return nil, "No operations provided"
    end

    local graph = FlowGraph.new()
    for _, op in ipairs(operations) do
        if op.type == "input" then
            graph.input_data = op.config.data
            graph.input_name = op.config.name
        elseif op.type == "data" then
            local static_id, err = graph:create_static_data(op.config.data)
            if err then
                return nil, err
            end
        elseif op.type == "node" then
            local node_id, err = graph:create_node("node", op.config)
            if err then
                return nil, err
            end
        elseif op.type == "cycle" then
            local node_id, err = graph:create_node("cycle", op.config)
            if err then
                return nil, err
            end
            if op.config.template then
                graph:create_template_nodes(op.config.template, node_id)
            end
        end
        local success, err = graph:add_operation(op.type, op.config)
        if err then
            return nil, err
        end
    end
    return graph, nil
end

function compiler.compile(operations)
    local graph, err = compiler.build_graph(operations)
    if err then
        return nil, err
    end
    local commands = table.create(#graph.node_order * 2, 0)
    return commands, nil
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want receiver mutations returning other values to preserve constructor fields", result.Diagnostics)
	}
}

func TestCheckNilInitializedConstructorFieldDoesNotBecomeNeverWriteContract(t *testing.T) {
	result := Check(`
local Graph = {}
local mt = { __index = Graph }

function Graph.new()
    return setmetatable({
        session_parent_id = nil,
    }, mt)
end

local function build(session_context)
    local graph = Graph.new()
    if session_context and session_context.node_id then
        graph.session_parent_id = session_context.node_id
    end
    return graph
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nil-initialized unannotated field to stay writable", result.Diagnostics)
	}
}

func TestCheckNilInitializedSingletonFieldCanBeRestoredAfterConcreteWrite(t *testing.T) {
	result := Check(`
local test = {}

local _default_context = {
    current_describe = nil,
    suites_hierarchy = {},
    tests = {},
}

function test.suite(name: string)
    return {
        name = name,
        tests = {},
        children = {},
        parent = nil,
        full_path = name,
    }
end

function test.describe(name: string, fn: fun())
    local old_describe = _default_context.current_describe
    local new_suite = test.suite(name)

    if old_describe then
        new_suite.parent = old_describe
        table.insert(old_describe.children, new_suite)
        new_suite.full_path = old_describe.full_path .. " > " .. name
    else
        table.insert(_default_context.suites_hierarchy, new_suite)
    end

    _default_context.current_describe = new_suite
    fn()
    table.insert(_default_context.tests, new_suite)
    _default_context.current_describe = old_describe
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nil-initialized singleton field to restore saved nil-or-suite value after concrete write", result.Diagnostics)
	}
}

func TestCheckMemberReadNormalContinuationProvesReceiverForAssignment(t *testing.T) {
	result := Check(`
type Suite = {
    tests: {table}?,
    children: {Suite},
    before_all: fun()?,
}

local function clear_suite_references(suite: Suite?)
    if suite.tests then
        for i, test_case in ipairs(suite.tests) do
            suite.tests[i].fn = nil
        end
    end

    suite.before_all = nil
    suite.children = {}
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want successful member read to prove optional receiver non-nil on normal continuation", result.Diagnostics)
	}
}

func TestCheckConditionalMemberReadDoesNotProveReceiverForAssignment(t *testing.T) {
	result := Check(`
type Suite = {
    tests: {table}?,
    before_all: fun()?,
}

local function clear_suite_references(flag: boolean, suite: Suite?)
    if flag and suite.tests then
    end

    suite.before_all = nil
end
`, WithStdlib())
	if len(result.Diagnostics) == 0 {
		t.Fatalf("diagnostics = none, want conditional RHS read not to prove optional receiver non-nil")
	}
}

func TestCheckInvalidatedMemberReadDoesNotProveReceiverForAssignment(t *testing.T) {
	result := Check(`
type Suite = {
    tests: {table}?,
    before_all: fun()?,
}

local function clear_suite_references(suite: Suite?)
    if suite.tests then
    end

    suite = nil
    suite.before_all = nil
end
`, WithStdlib())
	if len(result.Diagnostics) == 0 {
		t.Fatalf("diagnostics = none, want stale member-read receiver proof rejected after reassignment")
	}
}

func TestCheckConstructorFallbackArrayFieldLengthWithoutMetadataCall(t *testing.T) {
	result := Check(`
local node = {}
local methods = {}
local mt = { __index = methods }

type RouteTarget = {
    id: string,
}

type NodeConfig = {
    data_targets: {RouteTarget}?,
}

type NodeDefinition = {
    config: NodeConfig?,
}

type NodeArgs = {
    node_id: string,
    node: NodeDefinition?,
}

type NodeInstance = {
    node_id: string,
    node: NodeDefinition,
    data_targets: {RouteTarget},
}

function node.new(args: NodeArgs)
    local instance: NodeInstance = {
        node_id = args.node_id,
        node = args.node or {},
        data_targets = (args.node and args.node.config and args.node.config.data_targets) or (table.create(0, 0) :: {RouteTarget}),
    }
    return setmetatable(instance, mt), nil
end

function methods:complete()
    local out = table.create(#self.data_targets, 0)
    return out
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want constructor fallback data_targets array type at method entry", result.Diagnostics)
	}
}

func TestCheckMetatableMethodSurfaceSurvivesLoopBackEdge(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }

type NodeInstance = {
    node_id: string,
    items: {unknown},
}

local function new_node()
    local instance: NodeInstance = { node_id = "n1", items = {} }
    return setmetatable(instance, mt)
end

function methods:data(content)
    table.insert(self.items, content)
    return self, nil
end

function methods:route(values: {unknown}): ()
    for _, value in ipairs(values) do
        self:data(value)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want metatable method surface to survive loop-carried receiver mutations", result.Diagnostics)
	}
}

func TestCheckMetatableMethodSurfaceSurvivesLoopAliasThroughEnv(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }

type Expr = {
    eval: (source: string, env: unknown) -> (unknown, string?),
}

type Target = {
    transform: string?,
}

type NodeInstance = {
    node_id: string,
    targets: {Target},
    items: {unknown},
}

local expr: Expr = {
    eval = function(source: string, env: unknown): (unknown, string?)
        return env, nil
    end,
}

local function new_node()
    local instance: NodeInstance = { node_id = "n1", targets = {}, items = {} }
    return setmetatable(instance, mt)
end

function methods:data(content: unknown): (NodeInstance, string?)
    table.insert(self.items, content)
    return self, nil
end

function methods:route(error_content: unknown): ()
    local env = {
        error = error_content,
        node = self,
    }
    for _, target in ipairs(self.targets) do
        local output = error_content
        if target.transform then
            local transformed, transform_err = expr.eval(target.transform, env)
            if not transform_err then
                output = transformed
            end
        end
        self:data(output)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want metatable method surface to survive loop-carried env alias", result.Diagnostics)
	}
}

func TestCheckTerminalProgressAndSpinnerArithmeticStaysPrecise(t *testing.T) {
	result := Check(`
local term = {}

function term.dim(s: string): string
    return s
end

function term.cyan(s: string): string
    return s
end

function term.progress_bar(current: number, total: number, width: integer?): string
    width = width or 20
    if total == 0 then
        return term.dim(string.rep(".", width))
    end
    local filled = math.floor((current / total) * width)
    local empty = width - filled
    return term.cyan(string.rep("#", filled)) .. term.dim(string.rep(".", empty))
end

term.spinner_frames = {"a", "b", "c"}

function term.spinner(index: number): string
    local frame = term.spinner_frames[((index - 1) % #term.spinner_frames) + 1]
    return term.cyan(frame :: string)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want terminal progress/spinner arithmetic to stay precise", result.Diagnostics)
	}
}

func TestCheckBooleanReturnFalseNarrowsSecondOptionalStringSlot(t *testing.T) {
	result := Check(`
type BootloaderResult = { status: string, message: string, details: any? }

local function wait_for_database(ready: boolean): (boolean, string?)
    if ready then
        return true, nil
    end
    return false, "database unavailable"
end

local function run(ready: boolean): BootloaderResult
    local db_ready, db_err = wait_for_database(ready)
    if not db_ready then
        return {
            status = "error",
            message = "Database unavailable: " .. db_err,
        }
    end
    return {
        status = "ok",
        message = "ready",
    }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want false boolean return slot to prove second string slot present", result.Diagnostics)
	}
}

func TestCheckLoopBooleanReturnFalseNarrowsSecondOptionalStringSlot(t *testing.T) {
	result := Check(`
type BootloaderResult = { status: string, message: string, details: any? }

local function wait_for_database(ready: boolean, max_attempts: integer): (boolean, string?)
    for attempt = 1, max_attempts do
        local err: string? = ready and nil or "not ready"
        if not err then
            return true, nil
        end
        if attempt == max_attempts then
            return false, tostring(err)
        end
    end
    return false, "Max retry attempts reached"
end

local function run(ready: boolean): BootloaderResult
    local db_ready, db_err = wait_for_database(ready, 20)
    if not db_ready then
        return {
            status = "error",
            message = "Database unavailable: " .. db_err,
            details = {
                databases_processed = {},
            },
        }
    end
    return {
        status = "ok",
        message = "ready",
    }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want loop false return slot to prove second string slot present", result.Diagnostics)
	}
}

func TestCheckAnyStatusGuardDoesNotInventErrorFieldShape(t *testing.T) {
	result := Check(`
local function boot(result: any): string
    if result.status == "error" then
        return "Migration failed: " .. result.error
    end
    return "ok"
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"returned value 1",
			"comes from any/unknown",
			"string",
		},
	})
}

func TestCheckMigrationResultUnionStatusGuardProvesErrorField(t *testing.T) {
	result := Check(`
type MigrationError = {
    status: "error",
    error: string,
    migrations_applied: number?,
    migrations_failed: number?,
}

type MigrationComplete = {
    status: "complete",
    migrations_applied: number,
    migrations_failed: number,
}

type MigrationResult = MigrationError | MigrationComplete

type BootloaderResult = {
    status: string,
    message: string,
    details: any?,
}

local function boot(result: MigrationResult): BootloaderResult
    if result.status == "error" then
        return {
            status = "error",
            message = "Migration failed: " .. result.error,
        }
    end
    return {
        status = "success",
        message = "Migrations complete",
        details = {
            total_applied = result.migrations_applied,
            total_failed = result.migrations_failed,
        },
    }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want status literal union guard to prove error field string", result.Diagnostics)
	}
}

func TestCheckAnyReceiverMethodReturnDoesNotProveErrorMessageString(t *testing.T) {
	result := Check(`
local function call_provider(provider_instance: any): (table?, string?)
    local raw_result, err = (provider_instance :: any):structured_output({})
    if err then
        return nil, err:message()
    end
    return raw_result :: table, nil
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"returned value 2",
			"err:message(...)",
			"any",
			"string?",
		},
	})
}

func TestCheckTypedProviderErrorMessageReturnIsAccepted(t *testing.T) {
	result := Check(`
type ProviderError = {
    message: (self: ProviderError) -> string,
}

type Provider = {
    structured_output: (self: Provider, args: table) -> (table?, ProviderError?),
}

local function call_provider(provider_instance: Provider): (table?, string?)
    local raw_result, err = provider_instance:structured_output({})
    if err then
        return nil, err:message()
    end
    return raw_result, nil
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed provider error method to satisfy optional string return", result.Diagnostics)
	}
}

func TestCheckReceiverMethodCallReturnInsideObjectLiteralField(t *testing.T) {
	result := Check(`
type Summary = {
    processed: number,
}

type RunResult = {
    ok: true,
    value: Summary,
}

type Store = {
    summarize: (self: Store, now: string) -> Summary,
}

local StoreMethods = {}

function StoreMethods:summarize(now: string): Summary
    return { processed = #now }
end

local function run(store: Store, now: string): RunResult
    return {
        ok = true,
        value = store:summarize(now),
    }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed receiver method result to type object literal field", result.Diagnostics)
	}
}

func TestCheckImportedReceiverMethodCallReturnInsideObjectLiteralField(t *testing.T) {
	storeMod := CheckAndExport(`
type Summary = {
    processed: number,
}

type Store = {
    summarize: (self: Store, now: string) -> Summary,
}

local StoreMethods = {}

function StoreMethods:summarize(now: string): Summary
    return { processed = #now }
end

local M = {}
M.Store = Store
return M
`, "storemod", WithStdlib())
	if len(storeMod.Errors) != 0 {
		t.Fatalf("store module diagnostics = %#v, want clean export", storeMod.Errors)
	}

	result := Check(`
local storemod = require("storemod")

type RunResult = {
    ok: true,
    value: storemod.Summary,
}

local function run(store: storemod.Store, now: string): RunResult
    return {
        ok = true,
        value = store:summarize(now),
    }
end
`, WithStdlib(), WithModule("storemod", storeMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported receiver method result to type object literal field", result.Diagnostics)
	}
}

func TestCheckImportedReceiverMethodCallReturnWithTransitiveImportedType(t *testing.T) {
	protocolMod := CheckAndExport(`
local M = {}

type Summary = {
    processed: number,
}

type RunResult = {
    ok: true,
    value: Summary,
}

M.Summary = Summary
M.RunResult = RunResult
return M
`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want clean export", protocolMod.Errors)
	}

	storeMod := CheckAndExport(`
local protocol = require("protocol")

type Store = {
    summarize: (self: Store, now: string) -> protocol.Summary,
}

local StoreMethods = {}

function StoreMethods:summarize(now: string): protocol.Summary
    return { processed = #now }
end

local M = {}
M.Store = Store
return M
`, "storemod", WithStdlib(), WithModule("protocol", protocolMod))
	if len(storeMod.Errors) != 0 {
		t.Fatalf("store module diagnostics = %#v, want clean export", storeMod.Errors)
	}

	result := Check(`
local protocol = require("protocol")
local storemod = require("storemod")

local function run(store: storemod.Store, now: string): protocol.RunResult
    return {
        ok = true,
        value = store:summarize(now),
    }
end
`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("storemod", storeMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want transitively imported receiver method result to type object literal field", result.Diagnostics)
	}
}

func TestCheckImportedReceiverMethodReturnKeepsDeclaredTypeWhenBodyUsesManifestMethod(t *testing.T) {
	protocolMod := CheckAndExport(`
local time = require("time")

local M = {}

type Summary = {
    elapsed_seconds: time.Duration,
    last_output_kind: string?,
}

type Receipt = {
    plugin: string,
    output: {
        kind: string,
    },
    emitted_at: time.Time,
}

type DispatchResult = {
    ok: true,
    value: Receipt?,
} | {
    ok: false,
    error: string,
}

type RunResult = {
    ok: true,
    value: Summary,
} | {
    ok: false,
    error: string,
}

M.Summary = Summary
M.Receipt = Receipt
M.DispatchResult = DispatchResult
M.RunResult = RunResult
return M
`, "protocol", WithStdlib(), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want clean export", protocolMod.Errors)
	}

	storeMod := CheckAndExport(`
local time = require("time")
local protocol = require("protocol")

type Store = {
    started_at: time.Time,
    summarize: (self: Store, now: time.Time) -> protocol.Summary,
}

local Store = {}

function Store:summarize(now: time.Time): protocol.Summary
    return {
        elapsed_seconds = now:sub(self.started_at),
    }
end

local M = {}
M.Store = Store
return M
`, "storemod", WithStdlib(), WithModule("protocol", protocolMod), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(storeMod.Errors) != 0 {
		t.Fatalf("store module diagnostics = %#v, want clean export", storeMod.Errors)
	}

	result := Check(`
local time = require("time")
local protocol = require("protocol")
local storemod = require("storemod")

local function run(store: storemod.Store, now: time.Time): protocol.RunResult
    return {
        ok = true,
        value = store:summarize(now),
    }
end
`, WithStdlib(), WithManifest("time", timeManifestForPrecisionTests()), WithModule("protocol", protocolMod), WithModule("storemod", storeMod), WithGlobals("time"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want manifest-method body result to retain declared receiver method return", result.Diagnostics)
	}
}

func TestCheckImportedReceiverMethodReturnThroughExportedAlias(t *testing.T) {
	protocolMod := CheckAndExport(`
local time = require("time")

local M = {}

type Summary = {
    elapsed_seconds: time.Duration,
    last_output_kind: string?,
}

type Receipt = {
    output: {
        kind: string,
    },
}

type DispatchResult = {
    ok: true,
    value: Receipt?,
} | {
    ok: false,
    error: string,
}

type RunResult = {
    ok: true,
    value: Summary,
} | {
    ok: false,
    error: string,
}

M.Summary = Summary
M.Receipt = Receipt
M.DispatchResult = DispatchResult
M.RunResult = RunResult
return M
`, "protocol", WithStdlib(), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want clean export", protocolMod.Errors)
	}

	storeMod := CheckAndExport(`
local time = require("time")
local protocol = require("protocol")

type PluginStore = {
    state: {
        started_at: time.Time,
        cached_receipts: {[string]: protocol.Receipt},
    },
    started_at: time.Time,
    cache_receipt: (self: PluginStore, key: string, receipt: protocol.Receipt, at: time.Time) -> (),
    summarize: (self: PluginStore, now: time.Time, last_output_kind: string?) -> protocol.Summary,
}

type Store = PluginStore

local Store = {}
Store.__index = Store

function Store:summarize(now: time.Time, last_output_kind: string?): protocol.Summary
    return {
        elapsed_seconds = now:sub(self.state.started_at),
        last_output_kind = last_output_kind,
    }
end

function Store:cache_receipt(key: string, receipt: protocol.Receipt, at: time.Time)
    self.state.cached_receipts[key] = receipt
end

local M = {}
M.PluginStore = PluginStore

function M.new(now: time.Time): PluginStore
    local self: Store = {
        state = {
            started_at = now,
            cached_receipts = {},
        },
        started_at = now,
        cache_receipt = Store.cache_receipt,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

return M
`, "storemod", WithStdlib(), WithModule("protocol", protocolMod), WithManifest("time", timeManifestForPrecisionTests()), WithGlobals("time"))
	if len(storeMod.Errors) != 0 {
		t.Fatalf("store module diagnostics = %#v, want clean export", storeMod.Errors)
	}

	result := Check(`
local time = require("time")
local protocol = require("protocol")
local storemod = require("storemod")

type Runtime = {
    dispatch: (self: Runtime, store: storemod.PluginStore, request: string, now: time.Time) -> protocol.DispatchResult,
    run: (self: Runtime, store: storemod.PluginStore, requests: {string}, now: time.Time) -> protocol.RunResult,
}

local Runtime = {}
Runtime.__index = Runtime

function Runtime:dispatch(store: storemod.PluginStore, request: string, now: time.Time): protocol.DispatchResult
    return {
        ok = true,
        value = {
            plugin = request,
            output = {
                kind = request,
            },
            emitted_at = now,
        },
    }
end

function Runtime:run(store: storemod.PluginStore, requests: {string}, now: time.Time): protocol.RunResult
    local last_output_kind: string? = nil
    for _, request in ipairs(requests) do
        local dispatch_result = self:dispatch(store, request, now)
        if not dispatch_result.ok then
            return {ok = false, error = dispatch_result.error}
        end

        local receipt = dispatch_result.value
        if receipt then
            store:cache_receipt(request, receipt, now)
            last_output_kind = receipt.output.kind
        end
    end
    return {
        ok = true,
        value = store:summarize(now, last_output_kind),
    }
end

local runtime: Runtime = {
    dispatch = Runtime.dispatch,
    run = Runtime.run,
}
local now = time.now()
local store = storemod.new(now)
local result: protocol.RunResult = runtime:run(store, {"render"}, now)
`, WithStdlib(), WithManifest("time", timeManifestForPrecisionTests()), WithModule("protocol", protocolMod), WithModule("storemod", storeMod), WithGlobals("time"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want exported alias receiver method return to type object literal field", result.Diagnostics)
	}
}

func TestCheckTruthyJsonImageUrlDoesNotProveString(t *testing.T) {
	result := Check(`
local prompt = {}
function prompt.image(url: string): table
    return { type = "image", url = url }
end

local function collect(parsed: any): {table}
    local images = {}
    if type(parsed._images) == "table" then
        for _, img in ipairs(parsed._images) do
            if type(img) == "table" and img.url then
                table.insert(images, prompt.image(img.url))
            end
        end
    end
    return images
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"img.url",
			"comes from any/unknown",
			"string",
		},
	})
}

func TestCheckFlatContentPartTextDiscriminantDoesNotProveTextPresent(t *testing.T) {
	result := Check(`
type ContentPart = {
    type: string,
    text: string?,
}

local prompt = {
    CONTENT_TYPE = {
        TEXT = "text",
    },
}

local function merge(last: {content: {ContentPart}}, part: ContentPart): ()
    if part.type == prompt.CONTENT_TYPE.TEXT and
        last.content[#last.content] and
        last.content[#last.content].type == prompt.CONTENT_TYPE.TEXT then
        last.content[#last.content].text = last.content[#last.content].text .. "\n\n" .. part.text
    end
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeConcatOperand,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 2,
		MessageContains: []string{
			"right operand",
			"may be nil",
		},
	})
}

func TestCheckTableGuardDoesNotValidateBindingSpecFields(t *testing.T) {
	result := Check(`
type BindingSpec = {
    id: string?,
    priority: number?,
}

local function normalize_bindings(bindings: any): {BindingSpec}
    local normalized: {BindingSpec} = {}
    for _, binding in ipairs(bindings or {}) do
        if type(binding) == "table" then
            normalized[#normalized + 1] = binding
        end
    end
    return normalized
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"binding",
			"table",
			"not {id: string?, priority: number?}",
		},
	})
}

func TestCheckRuntimeCastValidatesBindingSpecElement(t *testing.T) {
	result := Check(`
type BindingSpec = {
    id: string?,
    priority: number?,
}

local function normalize_bindings(bindings: any): {BindingSpec}
    local normalized: {BindingSpec} = {}
    for _, binding in ipairs(bindings or {}) do
        if type(binding) == "table" then
            normalized[#normalized + 1] = binding :: BindingSpec
        end
    end
    return normalized
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want element runtime cast to validate binding spec before insertion", result.Diagnostics)
	}
}

func TestCheckGuardedRegistryGetterDoesNotTreatTruthyParamAsString(t *testing.T) {
	registry := manifest.New("registry")
	getType := typ.Func().
		Param("id", typ.String).
		Returns(typ.Any, typeexpr.Optional(typ.String)).
		Build()
	registry.DefineFunctionSignature("registry.get", signature.Function{Type: getType})
	registry.SetExport(typ.NewInterface("registry", []typ.Method{
		{Name: "get", Type: getType},
	}))

	result := Check(`
local registry = require("registry")
local pages = {}

function pages.get(page_id)
    if not page_id then
        return nil, "Page ID is required"
    end

    local entry, err = registry.get(page_id)
    if err or not entry then
        return nil, "Page not found: " .. (err or "unknown error")
    end

    return { id = entry.id }, nil
end

return pages
`, WithStdlib(), WithManifest("registry", registry))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"page_id",
			"comes from any/unknown",
			"string",
		},
	})
}

func TestCheckFallbackObjectDoesNotValidateTruthyAnyAsTable(t *testing.T) {
	result := Check(`
local function need(options: table): ()
end

local function build(options: any): ()
    options = options or {}
    need(options)
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"options",
			"any/unknown",
			"table",
		},
	})
}

func TestCheckAnyRuntimeOptionsDefaultDoesNotValidateOptionalTable(t *testing.T) {
	result := Check(`
local function should_recall_memory(messages: any, options: any, runtime_options: table?): boolean
    return runtime_options == nil or runtime_options.disable_memory_recall ~= true
end

local function perform_memory_recall(prompt_builder: any, runtime_options: any): boolean
    runtime_options = runtime_options or {}
    local messages = prompt_builder:get_messages()
    return should_recall_memory(messages, {}, runtime_options)
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"runtime_options",
			"comes from any/unknown",
			"table?",
		},
	})
}

func TestCheckUntypedMessageMetadataDefaultDoesNotValidateTable(t *testing.T) {
	result := Check(`
local function build(messages: any): ()
    for _, msg in ipairs(messages) do
        local metadata: table = msg.metadata or {}
        if metadata.file_uuids then
            local _ = metadata.file_uuids
        end
    end
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"cannot assign",
			"any",
			"table",
		},
	})
}

func TestCheckTypedOptionalMessageMetadataDefaultSatisfiesTable(t *testing.T) {
	result := Check(`
type Message = {
    metadata: table?,
}

local function build(messages: {Message}): ()
    for _, msg in ipairs(messages) do
        local metadata: table = msg.metadata or {}
        if metadata.file_uuids then
            local _ = metadata.file_uuids
        end
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed optional metadata default to satisfy table", result.Diagnostics)
	}
}

func TestCheckRuntimeCastMessageMetadataDefaultSatisfiesTable(t *testing.T) {
	result := Check(`
local function build(messages: any): ()
    for _, msg in ipairs(messages) do
        local metadata: table = (msg.metadata :: table?) or {}
        if metadata.file_uuids then
            local _ = metadata.file_uuids
        end
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want runtime cast optional metadata default to satisfy table", result.Diagnostics)
	}
}

func TestCheckUntypedPageTemplateSetGuardDoesNotValidateString(t *testing.T) {
	result := Check(`
local templates = {}
function templates.get(template_set: string): table
    return {}
end

local function render(page: any): ()
    local tmpl_id = page.template_set
    if not tmpl_id then
        return
    end
    local template_set: string = tmpl_id
    templates.get(template_set)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"tmpl_id",
			"any",
			"string",
		},
	})
}

func TestCheckTypedPageTemplateSetGuardProvesString(t *testing.T) {
	result := Check(`
local templates = {}
function templates.get(template_set: string): table
    return {}
end

type Page = {
    template_set: string?,
}

local function render(page: Page): ()
    local tmpl_id = page.template_set
    if not tmpl_id then
        return
    end
    local template_set: string = tmpl_id
    templates.get(template_set)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed optional template_set guard to prove string", result.Diagnostics)
	}
}

func TestCheckPageTemplateSetRuntimeCastValidatesString(t *testing.T) {
	result := Check(`
local templates = {}
function templates.get(template_set: string): table
    return {}
end

local function render(page: any): ()
    local tmpl_id = page.template_set
    if not tmpl_id then
        return
    end
    local template_set: string = tmpl_id :: string
    templates.get(template_set)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete string cast to validate untyped template_set", result.Diagnostics)
	}
}

func TestCheckAnyNameFallbackDoesNotValidateStringField(t *testing.T) {
	result := Check(`
type TestEntry = {
    id: string,
    name: string,
}

local function collect(entries: any): {TestEntry}
    local tests: {TestEntry} = {}
    for i, entry in ipairs(entries) do
        local meta = entry.meta or {}
        local display_name = meta.name or ("Unnamed test " .. i)
        table.insert(tests, {
            id = entry.id :: string,
            name = display_name,
        })
    end
    return tests
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"name",
			"any/unknown",
			"no proof shows it is string",
		},
	})
}

func TestCheckTypedNameFallbackSatisfiesStringField(t *testing.T) {
	result := Check(`
type RegistryEntry = {
    id: string,
    meta: { name: string? },
}

type TestEntry = {
    id: string,
    name: string,
}

local function collect(entries: {RegistryEntry}): {TestEntry}
    local tests: {TestEntry} = {}
    for i, entry in ipairs(entries) do
        local meta = entry.meta
        local display_name = meta.name or ("Unnamed test " .. i)
        table.insert(tests, {
            id = entry.id,
            name = display_name,
        })
    end
    return tests
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed optional name fallback to satisfy string field", result.Diagnostics)
	}
}

func TestCheckScalarRuntimeCastSatisfiesOptionalReturnSlot(t *testing.T) {
	result := Check(`
local function f(raw: any, err: string?): (string?, string?)
    return raw :: string, err
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar runtime cast string to satisfy optional string return slot", result.Diagnostics)
	}
}

func TestCheckLiteralArgumentSelectsReturnShape(t *testing.T) {
	result := Check(`
local function map_tool_config(choice, available_tools)
    if not choice or choice == "auto" or choice == "any" or choice == "" then
        return { mode = "AUTO" }, nil
    elseif choice == "none" then
        return { mode = "NONE" }, nil
    elseif type(choice) == "string" then
        for _, tool in ipairs(available_tools or {}) do
            if tool.name == choice then
                return {
                    mode = "ANY",
                    allowedFunctionNames = { choice },
                }, nil
            end
        end
        return nil, "not found"
    end
    return "AUTO", nil
end

local tools = {
    { name = "get_weather" },
    { name = "calculate" },
}

local auto_config, auto_err = map_tool_config("auto", tools)
local auto_mode: string = auto_config.mode

local none_config, none_err = map_tool_config("none", tools)
local none_mode: string = none_config.mode
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want simple literal call arguments to select branch-specific return shape", result.Diagnostics)
	}
}

func TestCheckLiteralArgumentNamedLoopReturnShapeFiniteContainerFrontier(t *testing.T) {
	result := Check(`
local function map_tool_config(choice, available_tools)
    if not choice or choice == "auto" or choice == "any" or choice == "" then
        return { mode = "AUTO" }, nil
    elseif choice == "none" then
        return { mode = "NONE" }, nil
    elseif type(choice) == "string" then
        for _, tool in ipairs(available_tools or {}) do
            if tool.name == choice then
                return {
                    mode = "ANY",
                    allowedFunctionNames = { choice },
                }, nil
            end
        end
        return nil, "not found"
    end
    return "AUTO", nil
end

local tools = {
    { name = "get_weather" },
    { name = "calculate" },
}

local named_config, named_err = map_tool_config("get_weather", tools)
local named_mode: string = named_config.mode
local named_allowed: string = named_config.allowedFunctionNames[1]
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 2,
		Line:            27,
		MessageContains: []string{"named_config.mode", "may be nil"},
		EvidenceContains: []string{
			"named_config may be nil before reading .mode",
		},
	})
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 2,
		Line:            28,
		MessageContains: []string{"named_config.allowedFunctionNames[1]", "may be nil"},
		EvidenceContains: []string{
			"indexed read that can miss or read nil",
		},
	})
}

func TestCheckImportedLiteralArgumentSelectsReturnShape(t *testing.T) {
	mapper := CheckAndExport(`
local M = {}

function M.map_tool_config(choice, available_tools)
    if not choice or choice == "auto" or choice == "any" or choice == "" then
        return { mode = "AUTO" }, nil
    elseif choice == "none" then
        return { mode = "NONE" }, nil
    elseif type(choice) == "string" then
        for _, tool in ipairs(available_tools or {}) do
            if tool.name == choice then
                return {
                    mode = "ANY",
                    allowedFunctionNames = { choice },
                }, nil
            end
        end
        return nil, "not found"
    end
    return "AUTO", nil
end

return M
`, "mapper", WithStdlib())
	if len(mapper.Errors) != 0 {
		t.Fatalf("mapper diagnostics = %#v, want clean export", mapper.Errors)
	}

	result := Check(`
local mapper = require("mapper")

local tools = {
    { name = "get_weather" },
    { name = "calculate" },
}

local auto_config, auto_err = mapper.map_tool_config("auto", tools)
local auto_mode: string = auto_config.mode

local none_config, none_err = mapper.map_tool_config("none", tools)
local none_mode: string = none_config.mode
`, WithStdlib(), WithModule("mapper", mapper))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported simple literal call arguments to select branch-specific return shape", result.Diagnostics)
	}
}

func TestCheckImportedLiteralArgumentNamedLoopReturnShapeFiniteContainerFrontier(t *testing.T) {
	mapper := CheckAndExport(`
local M = {}

function M.map_tool_config(choice, available_tools)
    if not choice or choice == "auto" or choice == "any" or choice == "" then
        return { mode = "AUTO" }, nil
    elseif choice == "none" then
        return { mode = "NONE" }, nil
    elseif type(choice) == "string" then
        for _, tool in ipairs(available_tools or {}) do
            if tool.name == choice then
                return {
                    mode = "ANY",
                    allowedFunctionNames = { choice },
                }, nil
            end
        end
        return nil, "not found"
    end
    return "AUTO", nil
end

return M
`, "mapper", WithStdlib())
	if len(mapper.Errors) != 0 {
		t.Fatalf("mapper diagnostics = %#v, want clean export", mapper.Errors)
	}

	result := Check(`
local mapper = require("mapper")

local tools = {
    { name = "get_weather" },
    { name = "calculate" },
}

local named_config, named_err = mapper.map_tool_config("get_weather", tools)
local named_mode: string = named_config.mode
local named_allowed: string = named_config.allowedFunctionNames[1]
`, WithStdlib(), WithModule("mapper", mapper))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 2,
		Line:            10,
		MessageContains: []string{"named_config.mode", "may be nil"},
		EvidenceContains: []string{
			"named_config may be nil before reading .mode",
		},
	})
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 2,
		Line:            11,
		MessageContains: []string{"named_config.allowedFunctionNames[1]", "may be nil"},
		EvidenceContains: []string{
			"indexed read that can miss or read nil",
		},
	})
}

func TestCheckImportedSufficientOrLiteralArgumentSelectsReturnShape(t *testing.T) {
	picker := CheckAndExport(`
local M = {}

function M.pick(choice)
    if not choice or choice == "auto" or choice == "any" then
        return { mode = "AUTO" }, nil
    end
    return nil, "unsupported"
end

return M
`, "picker", WithStdlib())
	if len(picker.Errors) != 0 {
		t.Fatalf("picker diagnostics = %#v, want clean export", picker.Errors)
	}

	result := Check(`
local picker = require("picker")
local config, err = picker.pick("auto")
local mode: string = config.mode
`, WithStdlib(), WithModule("picker", picker))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported literal argument to select OR return case", result.Diagnostics)
	}
}

func TestCheckInferredParamDoesNotRequireStringForGuardedJsonDecode(t *testing.T) {
	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function normalize_tool_arguments(raw_arguments)
    local arguments = raw_arguments
    if type(arguments) == "string" then
        local parsed, parse_err = json.decode(arguments)
        if not parse_err and type(parsed) == "table" then
            arguments = parsed
        else
            arguments = { value = arguments }
        end
    end
    if not arguments or type(arguments) ~= "table" then
        arguments = { run = true }
    end
    return arguments
end

local function map_message(msg: {function_call: {arguments: unknown}}): table
    return normalize_tool_arguments(msg.function_call.arguments)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none because json.decode is behind a string guard", result.Diagnostics)
	}
}

func TestCheckOptionalStringFallbackProvesStringForJsonDecode(t *testing.T) {
	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function parse_response(response: { body: string? }): ()
    local parsed, parse_err = json.decode(response.body or "")
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want string? fallback to prove string", result.Diagnostics)
	}
}

func TestCheckAnyFallbackDoesNotLaunderIntoStringForJsonDecode(t *testing.T) {
	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function parse_response(response: { body: any }): ()
    local parsed, parse_err = json.decode(response.body or "")
end
`, WithStdlib())
	requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
}

func TestCheckImportedUrlResolverWithoutReturnContractRequiresValidation(t *testing.T) {
	registry := CheckAndExport(`
local env = {}
function env.get(name: string): string?
    return nil
end

local M = {}

function M.resolve_base_url(entry)
    local base_url = entry.url or ""

    local base_path = entry.base_path or ""
    if base_path ~= "" then
        if not base_url:match("/$") then
            base_url = base_url .. "/"
        end
        base_url = base_url .. base_path
    end

    local origin = env.get("PUBLIC_API_URL") or ""
    if origin ~= "" and base_url ~= "" and not base_url:match("^https?://") then
        base_url = origin .. base_url
    end

    if base_url ~= "" and not base_url:match("/$") then
        base_url = base_url .. "/"
    end

    return base_url
end

return M
`, "registry", WithStdlib())
	if len(registry.Errors) != 0 {
		t.Fatalf("registry diagnostics = %#v, want clean helper export", registry.Errors)
	}

	result := Check(`
local registry = require("registry")

local function log_missing_once(entry_id: string?, kind: string, url: string): ()
end

local function render(entry: {id: string?, url: string?, base_path: string?}): ()
    local base_url = registry.resolve_base_url(entry)
    log_missing_once(entry.id, "page", (base_url or "") .. "wippy-meta.json")
end
`, WithStdlib(), WithModule("registry", registry))
	requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
}

func TestCheckImportedUrlResolverRuntimeCastExportsStringReturn(t *testing.T) {
	registry := CheckAndExport(`
local env = {}
function env.get(name: string): string?
    return nil
end

local M = {}

function M.resolve_base_url(entry)
    local base_url = entry.url or ""

    local base_path = entry.base_path or ""
    if base_path ~= "" then
        if not base_url:match("/$") then
            base_url = base_url .. "/"
        end
        base_url = base_url .. base_path
    end

    local origin = env.get("PUBLIC_API_URL") or ""
    if origin ~= "" and base_url ~= "" and not base_url:match("^https?://") then
        base_url = origin .. base_url
    end

    if base_url ~= "" and not base_url:match("/$") then
        base_url = base_url .. "/"
    end

    return base_url :: string
end

return M
`, "registry", WithStdlib())
	if len(registry.Errors) != 0 {
		t.Fatalf("registry diagnostics = %#v, want clean helper export", registry.Errors)
	}

	result := Check(`
local registry = require("registry")

local function log_missing_once(entry_id: string?, kind: string, url: string): ()
end

local function render(entry: {id: string?, url: string?, base_path: string?}): ()
    local base_url = registry.resolve_base_url(entry)
    log_missing_once(entry.id, "page", (base_url or "") .. "wippy-meta.json")
end
`, WithStdlib(), WithModule("registry", registry))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported URL resolver cast to export string return", result.Diagnostics)
	}
}

func TestCheckClassConstructorInitializedDefaultedFieldVisibleInMethodBody(t *testing.T) {
	result := Check(`
type ConfigSummary = {
    cache_enabled: boolean,
}

local context = {}
context.__index = context

function context.new(config: { enable_cache: boolean? }?): any
    config = config or {}
    local self = setmetatable({}, context)
    self.enable_cache = config.enable_cache ~= nil and config.enable_cache or true
    return self
end

function context:get_config(): ConfigSummary
    return {
        cache_enabled = self.enable_cache,
    } :: ConfigSummary
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want constructor-initialized defaulted field visible in method body", result.Diagnostics)
	}
}

func TestCheckExportedLocalFunctionPreservesTypedStringReceiverInImportedBody(t *testing.T) {
	bootloader := CheckAndExport(`
local M = {}

local function dependency_kind(dep_id: string): string
    if not dep_id:match(":") then
        return "bootloader"
    end
    return "service"
end

M._dependency_kind = dependency_kind
return M
`, "bootloader", WithStdlib())
	if len(bootloader.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want none", bootloader.Errors)
	}

	result := Check(`
local bootloader = require("bootloader")

local function run(): string
    return bootloader._dependency_kind("app:db")
end
`, WithStdlib(), WithModule("bootloader", bootloader))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported exported local function body to retain typed string parameter", result.Diagnostics)
	}
}

func TestCheckImportedPrototypeAnyFactoryPreservesMethodReturnErrorString(t *testing.T) {
	runner := CheckAndExport(`
local M = {}

local Runner = {}
Runner.__index = Runner

function M.setup(database_id: string): any
    local self = setmetatable({}, Runner)
    self.database_id = database_id
    return self
end

function Runner:run(): any
    return {
        status = "error",
        error = "failed: " .. tostring(self.database_id),
        migrations_failed = 1,
    }
end

return M
`, "runner", WithStdlib())
	if len(runner.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want none", runner.Errors)
	}

	result := Check(`
local runner = require("runner")

type BootloaderResult = { status: string, message: string, details: any? }

local function run(db_resource: string): BootloaderResult
    local db_runner = runner.setup(db_resource)
    local result = db_runner:run()
    if result.status == "error" then
        return {
            status = "error",
            message = "Migration failed: " .. result.error,
            details = {
                total_failed = result.migrations_failed or 0,
            },
        }
    end
    return { status = "complete", message = "ok" }
end
`, WithStdlib(), WithModule("runner", runner))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported prototype factory/method any return to retain concrete error string", result.Diagnostics)
	}
}

func TestCheckImportedPrototypeAccumulatorReturnPreservesStatusErrorShape(t *testing.T) {
	runner := CheckAndExport(`
local M = {}

local Runner = {}
Runner.__index = Runner

local function execute_migration(): any
    return {
        status = "error",
        error = "failed",
    }
end

function M.setup(database_id: string): any
    local self = setmetatable({}, Runner)
    self.database_id = database_id
    return self
end

function Runner:run(): any
    local results = {
        status = "running",
        migrations_applied = 0,
        migrations_failed = 0,
        migrations = {},
    }

    local result = execute_migration()
    if result and result.status == "error" then
        results.migrations_failed = results.migrations_failed + 1
        table.insert(results.migrations, {
            status = "error",
            error = result.error,
        })
        results.status = "error"
        results.error = result.error
    else
        results.status = "complete"
    end

    return results
end

return M
`, "runner", WithStdlib())
	if len(runner.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want none", runner.Errors)
	}

	result := Check(`
local runner = require("runner")

type BootloaderResult = { status: string, message: string, details: any? }

local function run(db_resource: string): BootloaderResult
    local db_runner = runner.setup(db_resource)
    local result = db_runner:run()
    if result.status == "error" then
        return {
            status = "error",
            message = "Migration failed: " .. result.error,
            details = {
                total_failed = result.migrations_failed or 0,
            },
        }
    end
    return { status = "complete", message = "ok" }
end
`, WithStdlib(), WithModule("runner", runner))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported prototype accumulator return to retain status/error shape", result.Diagnostics)
	}
}

func TestCheckLocalAccumulatorReturnPreservesStatusErrorShape(t *testing.T) {
	result := Check(`
local function execute_migration(): any
    return {
        status = "error",
        error = "failed",
    }
end

local function run_migrations(): any
    local results = {
        status = "running",
        migrations_failed = 0,
    }
    local result = execute_migration()
    if result and result.status == "error" then
        results.migrations_failed = results.migrations_failed + 1
        results.status = "error"
        results.error = result.error
    else
        results.status = "complete"
    end
    return results
end

type BootloaderResult = { status: string, message: string, details: any? }

local function boot(): BootloaderResult
    local result = run_migrations()
    if result.status == "error" then
        return {
            status = "error",
            message = "Migration failed: " .. result.error,
        }
    end
    return { status = "complete", message = "ok" }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local accumulator return to retain status/error shape", result.Diagnostics)
	}
}

func TestCheckAnyDiscriminantDoesNotEmitNilWarningForPresentSibling(t *testing.T) {
	result := Check(`
local function execute_migration(): any
    local external: any = {}
    if external.migrations and #external.migrations > 0 then
        return external.migrations[1]
    end
    return external
end

local function run_migrations(): any
    local results = {
        status = "running",
        migrations_failed = 0,
    }
    local result = execute_migration()
    if result and result.status == "error" then
        results.status = "error"
        results.error = result.error
    else
        results.status = "complete"
    end
    return results
end

type BootloaderResult = { status: string, message: string }

local function boot(): BootloaderResult
    local result = run_migrations()
    if result.status == "error" then
        return {
            status = "error",
            message = "Migration failed: " .. result.error,
        }
    end
    return { status = "complete", message = "ok" }
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: present any is not a nil-risk proof failure", result.Diagnostics)
	}
}

func TestCheckAnyMapHelperParamDoesNotInheritFreshCallerMemberContract(t *testing.T) {
	result := Check(`
type DelegateToolsConfig = {
    enabled: boolean,
    description_suffix: string,
    default_schema: {
        type: string,
        properties: {[string]: any}?,
        required: any?,
    },
}

type AgentContextConfig = {
    delegate_tools: DelegateToolsConfig?,
}

local function build_agent_context_config(base_context: {[string]: any}): AgentContextConfig
    local agent_ctx_config: AgentContextConfig = {
        delegate_tools = base_context.delegate_tools,
    }
    return agent_ctx_config
end

local function make_config(): AgentContextConfig
    local base_context = {
        delegate_tools = {
            enabled = true,
            description_suffix = " (runs specialized agent in parallel)",
            default_schema = {
                type = "object",
                properties = {
                    message = {
                        type = "string",
                        description = "The message to forward to the agent",
                    },
                },
                required = { "message" },
            },
        },
    }
    return build_agent_context_config(base_context)
end
	`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeReturnContractType,
		MessageContains: []string{"base_context.delegate_tools", "comes from any/unknown"},
	})
}

func TestCheckTableRuntimeGuardDoesNotProveStringKeyMap(t *testing.T) {
	result := Check(`
local function need_context(context: {[string]: any}?): ()
end

local function process(inputs: any): ()
    local input_context = nil
    if inputs.context then
        local context_content = inputs.context.content
        if type(context_content) ~= "table" then
            return
        end
        input_context = context_content
    end
    need_context(input_context)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"argument 1 (input_context)", "table?", "not {[string]: any}?"},
	})
}

func TestCheckTableRuntimeGuardSatisfiesTableParamForKeyFilteringHelper(t *testing.T) {
	result := Check(`
local function merge_contexts(input_context: table?): {[string]: any}
    local merged: {[string]: any} = {}
    if input_context then
        for k, v in pairs(input_context) do
            if type(k) == "string" then
                merged[k] = v
            end
        end
    end
    return merged
end

local function process(inputs: any): {[string]: any}
    local input_context = nil
    if inputs.context then
        local context_content = inputs.context.content
        if type(context_content) ~= "table" then
            return {}
        end
        input_context = context_content
    end
    return merge_contexts(input_context)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded table to satisfy table? helper that filters keys", result.Diagnostics)
	}
}

func TestCheckStdlibJsonEncodeReturnSatisfiesStringParam(t *testing.T) {
	result := Check(`
local json = require("json")

local function need_string(s: string): ()
end

local function f(value: table): ()
    need_string(json.encode(value))
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want stdlib json.encode return to be string", result.Diagnostics)
	}
}

func TestCheckStdlibJsonDecodeRequiresStringInput(t *testing.T) {
	result := Check(`
local json = require("json")

local function f(value: number): ()
    json.decode(value)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            5,
		MessageContains: []string{"argument 1 (value)", "number", "not string"},
	})
}

func TestCheckStdlibEnvGetFallbackSatisfiesStringParam(t *testing.T) {
	result := Check(`
local env = require("env")

local function need_string(s: string): ()
end

local function f(): ()
    need_string(env.get("PUBLIC_API_URL") or "")
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want stdlib env.get fallback to be string", result.Diagnostics)
	}
}

func TestCheckStdlibEnvGetRequiresStringName(t *testing.T) {
	result := Check(`
local env = require("env")

local function f(name: number): ()
    env.get(name)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            5,
		MessageContains: []string{"argument 1 (name)", "number", "not string"},
	})
}

func TestCheckValidatedOptionalStringHelperReturnSatisfiesOptionalStringParam(t *testing.T) {
	result := Check(`
local function string_or_nil(value: any): string?
    if type(value) == "string" and value ~= "" then
        return value
    end
    return nil
end

local function configure(binding: string?): ()
end

local function run(config: { run_context_binding: any }): ()
    configure(string_or_nil(config.run_context_binding))
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want explicit helper return contract to satisfy optional string parameter", result.Diagnostics)
	}
}

func TestCheckValidatedOptionalStringHelperReturnSatisfiesOptionalStringRecordField(t *testing.T) {
	result := Check(`
type AgentRef = {
    id: string?,
    model: string?,
}

local function string_or_nil(value: any): string?
    if type(value) == "string" and value ~= "" then
        return value
    end
    return nil
end

local function agent_ref_from(agent_id: any, model_name: any): AgentRef
    return {
        id = string_or_nil(agent_id),
        model = string_or_nil(model_name),
    }
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want explicit helper return contract to satisfy optional string record fields", result.Diagnostics)
	}
}

func TestCheckHttpClientOptionalBodyFallbackProvesStringForJsonDecode(t *testing.T) {
	httpClient := manifest.New("http_client")
	responseType := typetable.NewRecord().
		Field("status_code", typ.Number).
		OptField("body", typ.String).
		Build()
	requestType := typ.Func().
		Param("url", typ.String).
		Param("opts", typ.Any).
		Returns(responseType, typeexpr.Optional(typ.String)).
		Build()
	httpClient.DefineFunctionSignature("get", signature.Function{Type: requestType})
	httpClient.DefineFunctionSignature("post", signature.Function{Type: requestType})
	httpClient.SetExport(typ.NewInterface("http_client", []typ.Method{
		{Name: "get", Type: requestType},
		{Name: "post", Type: requestType},
	}))

	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local http_client = require("http_client")

local client = {
    _http_client = http_client
}

function client.request(method: string, url: string, http_options: table): ()
    local response = nil
    local err = nil
    if method == "GET" then
        response, err = client._http_client.get(url, http_options)
    else
        response, err = client._http_client.post(url, http_options)
    end
    if not response then
        return
    end
    local parsed, parse_err = json.decode(response.body or "")
end
`, WithStdlib(), WithManifest("http_client", httpClient))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported http_client string? body fallback to prove string", result.Diagnostics)
	}
}

func TestCheckHttpClientOptionalBodyFallbackAfterStreamBodyAssignment(t *testing.T) {
	httpClient := manifest.New("http_client")
	streamType := typ.NewInterface("StreamReader", []typ.Method{
		{
			Name: "read",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("n", typ.Number).
				Returns(typeexpr.Optional(typ.String), typeexpr.Optional(typ.String)).
				Build(),
		},
	})
	responseType := typetable.NewRecord().
		Field("status_code", typ.Number).
		OptField("body", typ.String).
		OptField("stream", streamType).
		Build()
	requestType := typ.Func().
		Param("url", typ.String).
		Param("opts", typ.Any).
		Returns(responseType, typeexpr.Optional(typ.String)).
		Build()
	httpClient.DefineFunctionSignature("get", signature.Function{Type: requestType})
	httpClient.DefineFunctionSignature("post", signature.Function{Type: requestType})
	httpClient.SetExport(typ.NewInterface("http_client", []typ.Method{
		{Name: "get", Type: requestType},
		{Name: "post", Type: requestType},
	}))

	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local http_client = require("http_client")

local client = {
    _http_client = http_client
}

local function parse_error_response(response)
    return { status_code = response.status_code }
end

local function handle_stream_response(response, http_options)
    return {}, nil
end

function client.request(method, url, http_options)
    if http_options.stream then
        http_options.headers["Accept"] = "text/event-stream"
    end

    local response = nil
    local err = nil
    if method == "GET" then
        response, err = client._http_client.get(url, http_options)
    else
        response, err = client._http_client.post(url, http_options)
    end

    if not response then
        return nil, { message = tostring(err) }
    end

    if response.status_code < 200 or response.status_code >= 300 then
        if http_options.stream and response.stream and not response.body then
            local body_data = response.stream:read(4096)
            response.body = body_data
        end
        local parsed_error = parse_error_response(response)
        return nil, parsed_error
    end

    if http_options.stream and response.stream then
        return handle_stream_response(response, http_options)
    end

    local parsed, parse_err = json.decode(response.body or "")
    return parsed, parse_err
end
`, WithStdlib(), WithManifest("http_client", httpClient))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want stream body repair branch not to break final string? fallback", result.Diagnostics)
	}
}

func TestCheckOptionalRecordDefaultAllowsFieldWrites(t *testing.T) {
	result := Check(`
type RequestOptions = {
    headers: {[string]: string}?,
    timeout: number?,
}

local function merge_headers(headers: {[string]: string}?): {[string]: string}
    return headers or {}
end

local function send(options: RequestOptions?): RequestOptions
    options = options or {}
    options.headers = merge_headers(options.headers)
    options.timeout = options.timeout or 30
    return options
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want options default to make later field writes non-optional", result.Diagnostics)
	}
}

func TestCheckInferredParamDoesNotRequireStringForGuardedJsonDecodeInRoleLoop(t *testing.T) {
	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local prompt = {
    ROLE = {
        FUNCTION_CALL = "function_call",
        ASSISTANT = "assistant",
    },
}

local function normalize_tool_arguments(raw_arguments)
    local arguments = raw_arguments
    if type(arguments) == "string" then
        local parsed, parse_err = json.decode(arguments)
        if not parse_err and type(parsed) == "table" then
            arguments = parsed
        else
            arguments = { value = arguments }
        end
    end
    if not arguments or type(arguments) ~= "table" then
        arguments = { run = true }
    end
    if next(arguments) == nil then
        arguments = { run = true }
    end
    return arguments
end

type FunctionCallMessage = {
    role: "function_call",
    function_call: {arguments: unknown, id: string?, name: string?},
}

type AssistantMessage = {
    role: "assistant",
    content: string?,
}

local function map_messages(contract_messages: {FunctionCallMessage | AssistantMessage}): {table}
    local out = {}
    for _, msg in ipairs(contract_messages) do
        if msg.role == prompt.ROLE.FUNCTION_CALL then
            local arguments = normalize_tool_arguments(msg.function_call.arguments)
            table.insert(out, arguments)
        end
    end
    return out
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none because guarded decode does not constrain callers to string", result.Diagnostics)
	}
}

func TestCheckNilOrNotTableFallbackProvesTableAfterJoin(t *testing.T) {
	result := Check(`
local function consume(value: table): ()
end

local function normalize(raw: unknown): ()
    local value = raw
    if not value or type(value) ~= "table" then
        value = { run = true }
    end
    consume(value)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nil-or-not-table fallback to prove table", result.Diagnostics)
	}
}

func TestCheckUnannotatedNormalizeReturnInferredAsTable(t *testing.T) {
	result := Check(`
local function consume(value: table): ()
end

local function normalize(raw)
    local value = raw
    if not value or type(value) ~= "table" then
        value = { run = true }
    end
    if next(value) == nil then
        value = { run = true }
    end
    return value
end

local function caller(raw: unknown): ()
    local normalized = normalize(raw)
    consume(normalized)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unannotated normalize return inferred as table", result.Diagnostics)
	}
}

func TestCheckUnannotatedNormalizeReturnInferredAsTableAfterParsedTableGuard(t *testing.T) {
	result := Check(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end

local function consume(value: table): ()
end

local function normalize(raw)
    local value = raw
    if type(value) == "string" then
        local parsed, parse_err = json.decode(value)
        if not parse_err and type(parsed) == "table" then
            value = parsed
        else
            value = { run = true }
        end
    end
    if not value or type(value) ~= "table" then
        value = { run = true }
    end
    if next(value) == nil then
        value = { run = true }
    end
    return value
end

local function caller(raw: unknown): ()
    local normalized = normalize(raw)
    consume(normalized)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want parsed table guard assignment to infer table return", result.Diagnostics)
	}
}

func TestCheckUnannotatedNormalizeReturnInferredAsTableInLoop(t *testing.T) {
	result := Check(`
local function consume(value: table): ()
end

local function normalize(raw)
    local value = raw
    if not value or type(value) ~= "table" then
        value = { run = true }
    end
    if next(value) == nil then
        value = { run = true }
    end
    return value
end

local function caller(items: {unknown}): ()
    for _, item in ipairs(items) do
        local normalized = normalize(item)
        consume(normalized)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want normalize return inferred as table inside loop", result.Diagnostics)
	}
}

func TestCheckTableInsertAcceptsUnannotatedNormalizeTableReturn(t *testing.T) {
	result := Check(`
local function normalize(raw)
    local value = raw
    if not value or type(value) ~= "table" then
        value = { run = true }
    end
    return value
end

local function caller(items: {unknown}): {table}
    local out = {}
    for _, item in ipairs(items) do
        local normalized = normalize(item)
        table.insert(out, normalized)
    end
    return out
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want table.insert to accept inferred table return", result.Diagnostics)
	}
}

func TestCheckUnannotatedNormalizeReturnInferredAsTableAfterUnionNarrow(t *testing.T) {
	result := Check(`
local function consume(value: table): ()
end

local function normalize(raw)
    local value = raw
    if not value or type(value) ~= "table" then
        value = { run = true }
    end
    if next(value) == nil then
        value = { run = true }
    end
    return value
end

type FunctionCallMessage = {
    role: "function_call",
    function_call: {arguments: unknown},
}

type AssistantMessage = {
    role: "assistant",
}

local function caller(msg: FunctionCallMessage | AssistantMessage): ()
    if msg.role == "function_call" then
        local normalized = normalize(msg.function_call.arguments)
        consume(normalized)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want normalize return inferred as table after union narrow", result.Diagnostics)
	}
}

func TestCheckStaticConstantMemberNarrowsDiscriminatedUnion(t *testing.T) {
	result := Check(`
local prompt = {
    ROLE = {
        FUNCTION_CALL = "function_call",
        ASSISTANT = "assistant",
    },
}

type FunctionCallMessage = {
    role: "function_call",
    function_call: {arguments: table},
}

type AssistantMessage = {
    role: "assistant",
}

local function consume(value: table): ()
end

local function caller(msg: FunctionCallMessage | AssistantMessage): ()
    if msg.role == prompt.ROLE.FUNCTION_CALL then
        consume(msg.function_call.arguments)
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want static constant member to narrow discriminated union", result.Diagnostics)
	}
}

func TestCheckUnannotatedMapMessagesDoesNotPushGuardedDecodeObligationToNormalizerCall(t *testing.T) {
	promptMod := CheckAndExport(`
local prompt = {}
prompt.ROLE = {
    FUNCTION_CALL = "function_call",
    ASSISTANT = "assistant",
}
return prompt
`, "prompt", WithStdlib())
	if len(promptMod.Errors) != 0 {
		t.Fatalf("prompt module errors = %#v, want none", promptMod.Errors)
	}

	jsonMod := CheckAndExport(`
local json = {}
function json.decode(src: string): (any, string?)
    return {}, nil
end
return json
`, "json", WithStdlib())
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	result := Check(`
local prompt = require("prompt")
local json = require("json")

local function normalize_tool_arguments(raw_arguments)
    local arguments = raw_arguments
    if type(arguments) == "string" then
        local parsed, parse_err = json.decode(arguments)
        if not parse_err and type(parsed) == "table" then
            arguments = parsed
        else
            arguments = { value = arguments }
        end
    end
    if not arguments or type(arguments) ~= "table" then
        arguments = { run = true }
    end
    if next(arguments) == nil then
        arguments = { run = true }
    end
    return arguments
end

function map_messages(contract_messages)
    local out = {}
    for _, msg in ipairs(contract_messages) do
        if msg.role == prompt.ROLE.FUNCTION_CALL then
            local arguments = normalize_tool_arguments(msg.function_call.arguments)
            table.insert(out, {input = arguments})
        elseif msg.role == prompt.ROLE.ASSISTANT then
            local content = msg.content
            if type(content) == "table" then
                for _, part in ipairs(content) do
                    if part.type == "function_call" then
                        local arguments = normalize_tool_arguments(part.arguments)
                        table.insert(out, {input = arguments})
                    end
                end
            end
        end
    end
    return out
end
`, WithStdlib(), WithModule("prompt", promptMod), WithModule("json", jsonMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded json.decode to stay local to normalize_tool_arguments", result.Diagnostics)
	}
}

func TestCheckHelperReturnedContentUnionNarrowsByDiscriminantInLoop(t *testing.T) {
	result := Check(`
local function normalize_tool_arguments(raw_arguments): table
    if type(raw_arguments) == "table" then
        return raw_arguments
    end
    return {}
end

local function convert_image_content(content_part)
    if content_part.type == "image" and content_part.source then
        if content_part.source.type == "base64" then
            return {
                type = "image",
                source = {
                    type = "base64",
                    media_type = content_part.source.mime_type,
                    data = content_part.source.data,
                },
            }
        elseif content_part.source.type == "url" then
            return {
                type = "image",
                source = {
                    type = "url",
                    url = content_part.source.url,
                },
            }
        end
    elseif content_part.type == "document" and content_part.source then
        if content_part.source.type == "base64" then
            return {
                type = "document",
                source = {
                    type = "base64",
                    media_type = content_part.source.mime_type,
                    data = content_part.source.data,
                },
            }
        end
    end
    return content_part
end

local function process_content_array(content)
    if type(content) == "string" then
        return content
    elseif type(content) == "table" then
        local processed = {}
        for _, part in ipairs(content) do
            table.insert(processed, convert_image_content(part))
        end
        return processed
    end
    return content
end

local function map_content(msg: { content: any, metadata: any }): table
    local content_blocks = {}
    if msg.metadata and msg.metadata.thinking_blocks then
        for _, thinking_block in ipairs(msg.metadata.thinking_blocks) do
            table.insert(content_blocks, thinking_block)
        end
    end
    local regular_content = process_content_array(msg.content)
    if type(regular_content) == "string" and regular_content ~= "" then
        table.insert(content_blocks, {
            type = "text",
            text = regular_content,
        })
    elseif type(regular_content) == "table" then
        for _, part in ipairs(regular_content) do
            if part.type == "function_call" then
                local arguments = normalize_tool_arguments(part.arguments)
                table.insert(content_blocks, {
                    type = "tool_use",
                    id = part.id,
                    name = part.name,
                    input = arguments,
                })
            elseif part.type == "text" and part.text and part.text ~= "" then
                table.insert(content_blocks, part)
            elseif part.type ~= "text" then
                table.insert(content_blocks, part)
            end
        end
    end
    return content_blocks
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want helper-returned content union element to narrow by part.type inside loop", result.Diagnostics)
	}
}

func TestCheckGuardedHelperReturnKeepsPassThroughFallback(t *testing.T) {
	result := Check(`
local function convert(value)
    if value.kind == "image" then
        return {kind = "image"}
    end
    return value
end

local function map(values)
    local out = {}
    for _, value in ipairs(values) do
        table.insert(out, convert(value))
    end
    for _, item in ipairs(out) do
        if item.kind == "event" then
            local id = item.id
        end
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want pass-through return to keep non-image variants reachable", result.Diagnostics)
	}
}

func TestCheckCompoundGuardedHelperReturnKeepsPassThroughFallback(t *testing.T) {
	result := Check(`
local function convert(value)
    if value.kind == "image" and value.source then
        if value.source.kind == "base64" then
            return {kind = "image", source = {kind = "base64"}}
        elseif value.source.kind == "url" then
            return {kind = "image", source = {kind = "url"}}
        end
    end
    return value
end

local function map(values)
    local out = {}
    for _, value in ipairs(values) do
        table.insert(out, convert(value))
    end
    for _, item in ipairs(out) do
        if item.kind == "event" then
            local id = item.id
        end
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want compound-guard fallback return to keep non-image variants reachable", result.Diagnostics)
	}
}

func TestCheckExportedDockerConstsNestedTopicMembersSurviveRequire(t *testing.T) {
	consts := CheckAndExport(`
local consts = {}

consts.status = {
    PENDING = "pending",
    RUNNING = "running",
}

consts.stream = {
    STDOUT = "stdout",
    STDERR = "stderr",
}

consts.topic = {
    CONTAINER_NEW = "container.new",
    CONTAINER_LOG = "container.log",
    IMAGE_BUILD_LOG = "image.build.log",
    IMAGE_BUILD_STATUS = "image.build.status",
}

consts.build_status = {
    BUILDING = "building",
    FAILED = "failed",
}

return consts
`, "consts", WithStdlib())
	if len(consts.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want none", consts.Errors)
	}

	result := Check(`
local consts = require("consts")

local function notify(topic: string): ()
end

notify(consts.topic.IMAGE_BUILD_STATUS)
notify(consts.topic.IMAGE_BUILD_LOG)
`, WithStdlib(), WithModule("consts", consts))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want docker consts topic members exported as strings", result.Diagnostics)
	}
}

func TestCheckStringKeyedRecordMapLookupPreservesValueTypeAfterOptionalIdGuard(t *testing.T) {
	result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local state = {
    active_sessions = {} :: {[string]: ActiveSession},
}

local function oldest_session_id(): string?
    return "session-1"
end

local function terminate(session_id: string, session_info: ActiveSession?): ()
end

local function run(): ()
    local session_id = oldest_session_id()
    if not session_id then
        return
    end
    terminate(session_id, state.active_sessions[session_id])
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want string-keyed record map lookup to preserve ActiveSession? after id guard", result.Diagnostics)
	}
}

func TestCheckCapturedStringKeyedRecordMapLookupPreservesValueTypeAfterOptionalIdGuard(t *testing.T) {
	result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local function run(): ()
    local state = {
        active_sessions = {} :: {[string]: ActiveSession},
    }

    local function oldest_session_id(): string?
        return "session-1"
    end

    local function terminate(session_id: string, session_info: ActiveSession?): ()
    end

    local session_id = oldest_session_id()
    if not session_id then
        return
    end
    terminate(session_id, state.active_sessions[session_id])
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured state map lookup to preserve ActiveSession? after id guard", result.Diagnostics)
	}
}

func TestCheckCapturedStringKeyedRecordMapPairsKeyReturnPreservesLookupValueType(t *testing.T) {
	result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local function run(): ()
    local state = {
        active_sessions = {} :: {[string]: ActiveSession},
    }

    local function oldest_session_id()
        local oldest_id = nil
        for session_id, session_info in pairs(state.active_sessions) do
            if not session_info.terminating then
                oldest_id = session_id
            end
        end
        return oldest_id
    end

    local function terminate(session_id: string, session_info: ActiveSession?): ()
    end

    local session_id = oldest_session_id()
    if not session_id then
        return
    end
    terminate(session_id, state.active_sessions[session_id])
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want pairs key returned from captured map to preserve string and ActiveSession? lookup", result.Diagnostics)
	}
}

func TestCheckCapturedStringKeyedRecordMapLookupWithImportedMemberType(t *testing.T) {
	result := Check(`
local time = require("time")

type ActiveSession = {
    pid: any,
    created_at: time.Time,
    last_activity: time.Time?,
    terminating: boolean,
    terminate_reason: string?,
}

local function run(): ()
    local state = {
        active_sessions = {} :: {[string]: ActiveSession},
    }

    local function oldest_session_id()
        local oldest_id = nil
        local oldest_time = nil
        for session_id, session_info in pairs(state.active_sessions) do
            if not session_info.terminating then
                local last_activity = session_info.last_activity or session_info.created_at
                if not oldest_time or last_activity:sub(oldest_time):seconds() > 0 then
                    oldest_time = last_activity
                    oldest_id = session_id
                end
            end
        end
        return oldest_id
    end

    local function terminate(session_id: string, session_info: ActiveSession?): ()
    end

    local session_id = oldest_session_id()
    if not session_id then
        return
    end
    terminate(session_id, state.active_sessions[session_id])
end
`, WithStdlib(), WithManifest("time", timeManifestForPrecisionTests()))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want captured map lookup with imported member types to preserve ActiveSession?", result.Diagnostics)
	}
}

func TestCheckStringKeyedRecordMapLookupDoesNotLaunderAnyField(t *testing.T) {
	result := Check(`
type ActiveSession = {
    pid: any,
    created_at: number,
    terminating: boolean,
}

local state = {
    active_sessions = {} :: {[string]: ActiveSession},
}

local function send(pid: string): ()
end

local function run(session_id: string): ()
    local session_info = state.active_sessions[session_id]
    if not session_info then
        return
    end
    send(session_info.pid)
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		MessageContains: []string{"session_info.pid", "comes from any/unknown"},
	})
}

func TestCheckStringMatchOnAnnotatedLocalFunctionParamIsNotOptional(t *testing.T) {
	result := Check(`
local function dependency_kind(dep_id: string): string
    if not dep_id:match(":") then
        return "bootloader"
    end
    return "service"
end

local function run(): string
    return dependency_kind("app:db")
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want annotated string parameter to support string:match", result.Diagnostics)
	}
}

func TestCheckStringMatchResultMustBeNilGuardedBeforeMethodCall(t *testing.T) {
	result := Check(`
local function dependency_kind(dep_id: string): string
    local namespace = dep_id:match("^([^:]+):")
    if namespace and namespace:match("%.") then
        return "bootloader"
    end
    return "service"
end

local function run(): string
    return dependency_kind("app:db")
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want truthy guard to prove string:match result non-nil before method call", result.Diagnostics)
	}
}

func TestCheckExportedDockerConstsNestedTopicMembersDischargeWrapperObligation(t *testing.T) {
	consts := CheckAndExport(`
local consts = {}

consts.topic = {
    IMAGE_BUILD_LOG = "image.build.log",
    IMAGE_BUILD_STATUS = "image.build.status",
}

return consts
`, "consts", WithStdlib())
	if len(consts.Errors) != 0 {
		t.Fatalf("consts module diagnostics = %#v, want none", consts.Errors)
	}

	helpers := CheckAndExport(`
local process = {}
function process.send(pid: string, topic: string, data: string): ()
end

local json = {}
function json.encode(data: any): string
    return ""
end

local helpers = {}

function helpers.send_json(pid, topic, data)
    process.send(pid, topic, json.encode(data))
end

return helpers
`, "helpers", WithStdlib())
	if len(helpers.Errors) != 0 {
		t.Fatalf("helpers module diagnostics = %#v, want none", helpers.Errors)
	}

	result := Check(`
local consts = require("consts")
local helpers = require("helpers")

local function notify_root(root_pid, topic, payload)
    if root_pid then
        helpers.send_json(root_pid, topic, payload)
    end
end

notify_root("root", consts.topic.IMAGE_BUILD_STATUS, {})
notify_root("root", consts.topic.IMAGE_BUILD_LOG, {})
`, WithStdlib(), WithModule("consts", consts), WithModule("helpers", helpers))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want exported nested constants to discharge projected wrapper obligations", result.Diagnostics)
	}
}

func TestCheckExportedDockerConstsSurviveNestedFunctionWithLaterDynamicCall(t *testing.T) {
	consts := CheckAndExport(`
local consts = {}

consts.topic = {
    IMAGE_BUILD_LOG = "image.build.log",
    IMAGE_BUILD_STATUS = "image.build.status",
}

consts.build_status = {
    FAILED = "failed",
}

return consts
`, "consts", WithStdlib())
	if len(consts.Errors) != 0 {
		t.Fatalf("consts module diagnostics = %#v, want none", consts.Errors)
	}

	helpers := CheckAndExport(`
local process = {}
function process.send(pid: any, topic: string, data: string): ()
end

local json = {}
function json.encode(data: any): string
    return ""
end

local helpers = {}

function helpers.send_json(pid, topic, data)
    process.send(pid, topic, json.encode(data))
end

return helpers
`, "helpers", WithStdlib())
	if len(helpers.Errors) != 0 {
		t.Fatalf("helpers module diagnostics = %#v, want none", helpers.Errors)
	}

	imagesRepo := manifest.New("images_repo")
	imagesRepo.SetExport(typetable.NewRecord().
		Field("get", typ.Func().
			Param("db", typ.Any).
			Param("id", typ.String).
			Returns(typ.Any).
			Build()).
		Field("update_build", typ.Func().
			Param("db", typ.Any).
			Param("id", typ.String).
			Param("status", typ.String).
			Param("updates", typ.Any).
			Returns().
			Build()).
		Build())

	result := Check(`
local consts = require("consts")
local helpers = require("helpers")
local images_repo = require("images_repo")

local function notify_root(root_pid, topic, payload)
    if root_pid then
        helpers.send_json(root_pid, topic, payload)
    end
end

local function run_build(docker: any, root_pid)
    local image = images_repo.get(nil, "image-1")
    if not image then
        images_repo.update_build(nil, "build-1", consts.build_status.FAILED, {})
        notify_root(root_pid, consts.topic.IMAGE_BUILD_STATUS, {})
        notify_root(root_pid, consts.topic.IMAGE_BUILD_LOG, {})
        return
    end
    local lines, build_err = docker:build_image("", "", "")
end

local image_builder = {}

function image_builder.run(root_pid, topic)
    if topic == consts.topic.IMAGE_BUILD_STATUS then
        coroutine.spawn(function()
            run_build({}, root_pid)
        end)
    end
end
`, WithStdlib(),
		WithGlobals("consts", "helpers", "images_repo"),
		WithModule("consts", consts),
		WithModule("helpers", helpers),
		WithManifest("images_repo", imagesRepo))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want later dynamic call not to erase earlier imported constant precision", result.Diagnostics)
	}
}

func TestCheckColonMethodSummaryKeepsExplicitStoreArgument(t *testing.T) {
	result := Check(`
type Store = {
    id: string,
}

type Step = {
    kind: "audit",
    note: string,
}

type Runtime = {
    emit: (self: Runtime, store: Store, step: Step, at: string) -> (),
    run: (self: Runtime, store: Store, at: string) -> (),
}

local Runtime = {}
Runtime.__index = Runtime

function Runtime:emit(store: Store, step: Step, at: string): ()
end

function Runtime:run(store: Store, at: string): ()
    local step: Step = {kind = "audit", note = "ok"}
    self:emit(store, step, at)
end

local rt: Runtime = {
    emit = Runtime.emit,
    run = Runtime.run,
}
	rt:run({id = "s"}, "now")
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want colon method summary to map explicit store/step/at arguments without shifting", result.Diagnostics)
	}
}

func TestCheckConstructorShapeSurvivesColonMethodMutations(t *testing.T) {
	result := Check(`
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
    }, flow_graph_mt)
end

function FlowGraph:create_node()
    local node_id = "node"
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    return node_id, nil
end

local graph = FlowGraph.new()
local _, err = graph:create_node()
if err then
    error(err)
end
local commands = table.create(#graph.node_order * 2, 0)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want constructor field shape to survive sibling colon-method mutations", result.Diagnostics)
	}
}

func TestCheckReturnedGraphShapeSurvivesIntoConsumerFunction(t *testing.T) {
	result := Check(`
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
    }, flow_graph_mt)
end

function FlowGraph:create_node()
    local node_id = "node"
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    return node_id, nil
end

local function build_graph()
    local graph = FlowGraph.new()
    local _, err = graph:create_node()
    if err then
        return nil, err
    end
    return graph, nil
end

local function compile_to_commands(graph)
    local commands = table.create(#graph.node_order * 2, 0)
    return commands, nil
end

local graph, graph_err = build_graph()
if graph_err then
    error(graph_err)
end
local commands, commands_err = compile_to_commands(graph)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want returned constructor shape to survive into consumer function", result.Diagnostics)
	}
}

func TestCheckGraphShapeSurvivesValidatorBeforeConsumer(t *testing.T) {
	result := Check(`
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
    }, flow_graph_mt)
end

function FlowGraph:create_node()
    local node_id = "node"
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    return node_id, nil
end

local function build_graph()
    local graph = FlowGraph.new()
    local _, err = graph:create_node()
    if err then
        return nil, err
    end
    return graph, nil
end

local function validate_graph(graph)
    local leaf_nodes = table.create(8, 0)
    for node_id, edges in pairs(graph.edges) do
        if #edges.targets == 0 then
            table.insert(leaf_nodes, {
                node_id = node_id,
                has_success_route = false,
                has_error_route = #edges.error_targets > 0,
                metadata = edges.metadata,
            })
        end
    end
    for _, leaf_info in ipairs(leaf_nodes) do
        local li = leaf_info
        if li.has_error_route and not li.has_success_route then
            local title = li.metadata and li.metadata.title or "unnamed"
            local message = title .. li.node_id
        end
    end
    return true, nil
end

local function compile_to_commands(graph)
    local commands = table.create(#graph.node_order * 2, 0)
    return commands, nil
end

local graph, graph_err = build_graph()
if graph_err then
    error(graph_err)
end
local valid, validation_err = validate_graph(graph)
if not valid then
    error(validation_err)
end
local commands, commands_err = compile_to_commands(graph)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want validation pass not to erase graph shape or leaf record shape", result.Diagnostics)
	}
}

func TestCheckSeparateLeafNodeTablesDoNotPolluteRecordAccumulator(t *testing.T) {
	result := Check(`
local compiler = {}

function compiler.find_leaf_nodes(graph)
    local leaves = table.create(8, 0)
    for node_id, edges in pairs(graph.edges) do
        if #edges.targets == 0 then
            table.insert(leaves, node_id)
        end
    end
    return leaves, nil
end

function compiler.validate_graph(graph)
    local leaf_nodes = table.create(8, 0)
    for node_id, edges in pairs(graph.edges) do
        if #edges.targets == 0 then
            table.insert(leaf_nodes, {
                node_id = node_id,
                has_success_route = false,
                has_error_route = #edges.error_targets > 0,
                metadata = {},
            })
        end
    end
    for _, leaf_info in ipairs(leaf_nodes) do
        local li = leaf_info
        if li.has_error_route and not li.has_success_route then
            local message = li.node_id
        end
    end
    return true, nil
end

function compiler.compile_to_commands(graph)
    local leaf_nodes, leaf_err = compiler.find_leaf_nodes(graph)
    if leaf_err then
        return nil, leaf_err
    end
    for _, node_id in ipairs(graph.node_order) do
        local is_leaf = false
        for _, leaf_id in ipairs(leaf_nodes) do
            if leaf_id == node_id then
                is_leaf = true
                break
            end
        end
    end
    return {}, nil
end

local graph = {
    node_order = { "a" },
    edges = {
        a = {
            targets = table.create(0, 0),
            error_targets = table.create(1, 0),
        },
    },
}
compiler.find_leaf_nodes(graph)
compiler.validate_graph(graph)
compiler.compile_to_commands(graph)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want string leaf-list result not to pollute record accumulator", result.Diagnostics)
	}
}

func TestCheckLengthSeededStringAccumulatorDoesNotPolluteSourceRecordArray(t *testing.T) {
	result := Check(`
local leaf_nodes = table.create(8, 0)
table.insert(leaf_nodes, {
    node_id = "n1",
    has_success_route = false,
    has_error_route = true,
    metadata = {},
})

local problematic_nodes = table.create(#leaf_nodes, 0)
for _, leaf_info in ipairs(leaf_nodes) do
    local li = leaf_info
    if li.has_error_route and not li.has_success_route then
        table.insert(problematic_nodes, li.node_id)
    end
end

for _, leaf_info in ipairs(leaf_nodes) do
    local li = leaf_info
    if li.has_error_route and not li.has_success_route then
        local message = li.node_id
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want length-seeded string accumulator not to pollute source record array", result.Diagnostics)
	}
}

func TestCheckValidateGraphLeafRecordShapeWithNodeMetadata(t *testing.T) {
	result := Check(`
local consts = { STATUS = { TEMPLATE = "template" } }
local graph = {
    nodes = {
        n1 = {
            status = "ready",
            parent_node_id = nil,
            metadata = { title = "Node" },
        },
    },
    edges = {
        n1 = {
            targets = table.create(0, 0),
            error_targets = table.create(1, 0),
        },
    },
}

local leaf_nodes = table.create(8, 0)
for node_id, edges in pairs(graph.edges) do
    local edge_set = edges
    local node_def = graph.nodes[node_id]
    if node_def and node_def.status ~= consts.STATUS.TEMPLATE then
        local nd = node_def
        local has_node_targets = false
        for _, edge in ipairs(edge_set.targets) do
            if edge.target_node_id then
                has_node_targets = true
                break
            end
        end
        if not has_node_targets and not nd.parent_node_id then
            table.insert(leaf_nodes, {
                node_id = node_id,
                has_success_route = #edge_set.targets > 0,
                has_error_route = #edge_set.error_targets > 0,
                metadata = {},
            })
        end
    end
end

local problematic_nodes = table.create(#leaf_nodes, 0)
for _, leaf_info in ipairs(leaf_nodes) do
    local li = leaf_info
    if li.has_error_route and not li.has_success_route then
        local title = li.metadata and li.metadata.title or "unnamed"
        table.insert(problematic_nodes, string.format("%s (%s)", title, li.node_id:sub(1, 12)))
    end
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want validate_graph leaf record shape to survive metadata and route checks", result.Diagnostics)
	}
}
