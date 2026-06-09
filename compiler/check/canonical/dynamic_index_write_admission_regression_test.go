package canonical_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalMethodDynamicIndexReadbackFeedsErrorReturnRelation(t *testing.T) {
	src := `
local FlowGraph = {}
local mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        references = table.create(0, 16),
    }, mt)
end

function FlowGraph:add_reference(name: string, node_id: string)
    self.references[name] = node_id
end

function FlowGraph:resolve_reference(name: string)
    local node_id = self.references[name]
    if not node_id then
        return nil, "missing"
    end
    return node_id, nil
end

local graph = FlowGraph.new()
graph:add_reference("next", "node-1")
local target_node_id, resolve_err = graph:resolve_reference("next")
if resolve_err then
    return nil, resolve_err
end
local target: string = target_node_id
return target, nil
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected receiver dynamic index readback to feed error-return narrowing, got diagnostics: %v", msgs)
	}
}

func TestCanonicalReceiverOperationAppendDoesNotEraseGraphEdgeAndRouteFacts(t *testing.T) {
	src := `
local uuid = require("uuid")
local FlowGraph = {}
local mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        operations = table.create(8, 0),
        node_order = table.create(8, 0),
        edges = table.create(0, 8),
        references = table.create(0, 8),
        pending_routes = table.create(8, 0),
        last_node_id = nil,
    }, mt)
end

function FlowGraph:add_operation(kind, config)
    table.insert(self.operations, { type = kind, config = config or {} })
    return self, nil
end

function FlowGraph:create_node()
    local node_id = uuid.v7()
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    self.last_node_id = node_id
    return node_id, nil
end

function FlowGraph:add_reference(name, node_id)
    self.references[name] = node_id
    return true, nil
end

function FlowGraph:resolve_reference(name)
    local node_id = self.references[name]
    if not node_id then
        return nil, "missing"
    end
    return node_id, nil
end

local graph = FlowGraph.new()
local node_id, create_err = graph:create_node()
if create_err then
    return nil, create_err
end
graph:add_reference("next", node_id)
table.insert(graph.pending_routes, {
    from_node_id = graph.last_node_id,
    target_name = "next",
    is_error = false,
})
local _, op_err = graph:add_operation("to", { target = "next" })
if op_err then
    return nil, op_err
end

for _, route_entry in ipairs(graph.pending_routes) do
    local target_node_id, resolve_err = graph:resolve_reference(route_entry.target_name)
    if resolve_err then
        return nil, resolve_err
    end
    local edges = graph.edges[route_entry.from_node_id]
    local edge_list = route_entry.is_error and edges.error_targets or edges.targets
    table.insert(edge_list, { target_node_id = target_node_id })
end

for _, id in ipairs(graph.node_order) do
    local edges = graph.edges[id]
    local data_targets = table.create(#edges.targets, 0)
    local error_targets = table.create(#edges.error_targets, 0)
    return data_targets, error_targets
end
return nil, nil
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected receiver operation append not to erase graph facts, got diagnostics: %v", msgs)
	}
}

func TestCanonicalReturnedGraphPreservesDynamicEdgeAndStaticSourceFacts(t *testing.T) {
	src := `
local uuid = require("uuid")
local FlowGraph = {}
local mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(8, 0),
        edges = table.create(0, 8),
        static_data_sources = table.create(4, 0),
    }, mt)
end

function FlowGraph:create_node()
    local node_id = uuid.v7()
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    return node_id, nil
end

function FlowGraph:create_static_data()
    table.insert(self.static_data_sources, {
        routes = table.create(4, 0),
    })
    return true, nil
end

function FlowGraph:add_static_route(target)
    for _, source in ipairs(self.static_data_sources) do
        table.insert(source.routes, {
            target_name = target,
        })
    end
end

local function build_graph()
    local graph = FlowGraph.new()
    local _, node_err = graph:create_node()
    if node_err then
        return nil, node_err
    end
    local _, data_err = graph:create_static_data()
    if data_err then
        return nil, data_err
    end
    graph:add_static_route("next")
    return graph, nil
end

local function compile_to_commands(graph)
    for _, node_id in ipairs(graph.node_order) do
        local edges = graph.edges[node_id]
        local data_targets = table.create(#edges.targets, 0)
        local error_targets = table.create(#edges.error_targets, 0)
        return data_targets, error_targets
    end
    for _, static_source in ipairs(graph.static_data_sources) do
        local src = static_source
        if #src.routes > 0 then
            local first_route = src.routes[1]
            local name: string = first_route.target_name
            return name, nil
        end
    end
    return nil, nil
end

local graph, graph_err = build_graph()
if graph_err then
    return nil, graph_err
end
return compile_to_commands(graph)
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected returned graph to preserve edge/source facts, got diagnostics: %v", msgs)
	}
}

func TestCanonicalUnannotatedGraphParamNestedIteratorLengthUseInfersShape(t *testing.T) {
	src := `
local function compile_to_commands(graph)
    for _, node_id in ipairs(graph.node_order) do
        local edges = graph.edges[node_id]
        local data_targets = table.create(#edges.targets, 0)
        local error_targets = table.create(#edges.error_targets, 0)
        return data_targets, error_targets
    end
    for _, static_source in ipairs(graph.static_data_sources) do
        local src = static_source
        if #src.routes > 0 then
            local first_route = src.routes[1]
            local name = first_route.target_name
            return name, nil
        end
    end
    return nil, nil
end
return compile_to_commands
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected nested unannotated graph parameter uses to infer shape, got diagnostics: %v", msgs)
	}
}

func TestCanonicalIndexedIteratorOverUnannotatedParamDoesNotCoerceValueToString(t *testing.T) {
	src := `
local function compile(operations)
    for _, op in ipairs(operations) do
        local target: string = op.config.target
    end
end
return compile
`
	res := testutil.Check(src, testutil.WithStdlib())
	msgs := testutil.ErrorMessages(res.Diagnostics)
	if !messagesContain(msgs, "cannot assign") || !messagesContain(msgs, "string") {
		t.Fatalf("expected gradual iterator value not to coerce into string proof, got diagnostics: %v", msgs)
	}
}

func TestCanonicalAppendElementFieldDemandRoutesBackToIteratorParam(t *testing.T) {
	src := `
local function resolve_reference(name)
    return "node:" .. name
end

local function compile(operations)
    local graph = { pending_routes = table.create(8, 0) }
    for _, op in ipairs(operations) do
        table.insert(graph.pending_routes, {
            target_name = op.config.target,
        })
    end
    for _, route_entry in ipairs(graph.pending_routes) do
        resolve_reference(route_entry.target_name)
    end
end
return compile
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected appended route field demand to infer iterator param shape, got diagnostics: %v", msgs)
	}
}

func TestCanonicalAppendedRouteVariantPresenceNarrowingPreservesTargetName(t *testing.T) {
	src := `
local function resolve_reference(name)
    return "node:" .. name
end

local function compile(operations)
    local graph = { pending_routes = table.create(8, 0) }
    for _, op in ipairs(operations) do
        if op.kind == "terminal" then
            table.insert(graph.pending_routes, {
                is_workflow_terminal = true,
                is_success = true,
            })
        else
            table.insert(graph.pending_routes, {
                from_node_id = "from",
                target_name = op.config.target,
                is_error = false,
            })
        end
    end
    for _, route_entry in ipairs(graph.pending_routes) do
        if route_entry.is_workflow_terminal then
            local ok = route_entry.is_success
        else
            resolve_reference(route_entry.target_name)
        end
    end
end
return compile
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected appended route variant narrowing to preserve target_name, got diagnostics: %v", msgs)
	}
}

func TestCanonicalAppendedRouteAliasVariantNarrowingPreservesTargetName(t *testing.T) {
	src := `
local function resolve_reference(name: string | number | false | nil)
    return name
end

local function compile(operations)
    local graph = { pending_routes = table.create(8, 0) }
    for _, op in ipairs(operations) do
        if op.kind == "terminal" then
            table.insert(graph.pending_routes, {
                is_workflow_terminal = true,
                is_success = true,
                is_error = false,
            })
        else
            table.insert(graph.pending_routes, {
                from_node_id = "from",
                target_name = op.config.target,
                is_error = false,
            })
        end
    end
    for _, route in ipairs(graph.pending_routes) do
        local route_entry = route
        if route_entry.is_workflow_terminal then
            local ok = route_entry.is_success
        else
            resolve_reference(route_entry.target_name)
        end
    end
end
return compile
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected aliased appended route variant narrowing to preserve target_name, got diagnostics: %v", msgs)
	}
}

func TestCanonicalLogicalOrPreservesTruthyOptionalMemberType(t *testing.T) {
	src := `
local options: { timeout: string? } = { timeout = "30s" }
local timeout = options.timeout or 30
local bad_timeout: number = timeout
`
	res := testutil.Check(src, testutil.WithStdlib())
	msgs := testutil.ErrorMessages(res.Diagnostics)
	if !messagesContain(msgs, "cannot assign") || !messagesContain(msgs, "string") || !messagesContain(msgs, "number") {
		t.Fatalf("expected logical-or assignment mismatch, got diagnostics: %v", msgs)
	}
}

func TestCanonicalTruthyUnionRecordFieldReadDoesNotDropScalarAlternative(t *testing.T) {
	src := `
local meta: string | { content_type: string } = ""
local artifact = { meta = meta }
if artifact.meta then
    local content_type: string = artifact.meta.content_type
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	msgs := testutil.ErrorMessages(res.Diagnostics)
	if !messagesContain(msgs, "content_type") && !messagesContain(msgs, "cannot assign") {
		t.Fatalf("expected field read on truthy scalar-or-record union to fail, got diagnostics: %v", msgs)
	}
}

func TestCanonicalImportedAnyDoesNotSuppressLocalConcreteErrors(t *testing.T) {
	mod := testutil.CheckAndExport(`
local M = {}
function M.decode(raw: any): any
    return raw
end
return M
`, "events", testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(mod.Errors); len(msgs) != 0 {
		t.Fatalf("events module should export cleanly, got diagnostics: %v", msgs)
	}
	res := testutil.Check(`
local events = require("events")
local raw = events.decode({timeout = "30s"})
local options: { timeout: string? } = { timeout = "30s" }
local timeout = options.timeout or 30
local bad_timeout: number = timeout

local meta: string | { content_type: string } = ""
local artifact = { meta = meta, raw = raw }
if artifact.meta then
    local content_type: string = artifact.meta.content_type
end
`, testutil.WithStdlib(), testutil.WithModule("events", mod))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	if !messagesContain(msgs, "bad_timeout") && !messagesContain(msgs, "cannot assign") {
		t.Fatalf("expected imported-any module not to suppress local assignment error, got diagnostics: %v", msgs)
	}
	if !messagesContain(msgs, "content_type") {
		t.Fatalf("expected imported-any module not to suppress local field-read error, got diagnostics: %v", msgs)
	}
}

func TestCanonicalImportedErrorReturnGuardKeepsSuccessContinuationReachable(t *testing.T) {
	sessions := testutil.CheckAndExport(`
type Err = { message: string }
type User = { id: string, name: string }

local users: {[string]: User} = {
    ["u1"] = { id = "u1", name = "Ada" },
}

local M = {}

function M.find_user(id: string): (User?, Err?)
    local user = users[id]
    if not user then
        return nil, { message = id }
    end
    return user, nil
end

function M.describe(id: string): (string?, Err?)
    local user, err = M.find_user(id)
    if err then
        return nil, err
    end
    return user.name, nil
end

return M
`, "sessions", testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(sessions.Errors); len(msgs) != 0 {
		t.Fatalf("sessions module should export cleanly, got diagnostics: %v", msgs)
	}
	res := testutil.Check(`
local sessions = require("sessions")
local session_text, session_err = sessions.describe("u1")
if session_err then
    return session_err.message
end

local options: { timeout: string? } = { timeout = "30s" }
local timeout = options.timeout or 30
local bad_timeout: number = timeout
`, testutil.WithStdlib(), testutil.WithModule("sessions", sessions))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	if !messagesContain(msgs, "cannot assign") {
		t.Fatalf("expected success continuation after imported error guard to be checked, got diagnostics: %v", msgs)
	}
}

func messagesContain(msgs []string, needle string) bool {
	for _, msg := range msgs {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func TestCanonicalDynamicIndexWriteAdmitsLaterSameKeyRead(t *testing.T) {
	src := `
type Entry = { value: number }
type Store = { entries: {[string]: Entry} }

local function install(self: Store, id: string): number
    if id == "" then
        return 0
    end

    self.entries[id] = { value = 42 }
    local current = self.entries[id]
    return current.value
end

return install
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected dynamic write to admit later same-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsFixtureShapeNestedRead(t *testing.T) {
	src := `
type DataTargets = {[string]: string}
type NodeConfig = { data_targets: DataTargets }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function ensure_node(self: Store, id: string): DataTargets
    if id == "" then
        return {}
    end

    self.nodes[id] = { config = { data_targets = {} } }
    local prev = self.nodes[id]
    local targets = prev.config.data_targets
    targets[id] = "present"
    return targets
end

return ensure_node
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected fixture-shaped dynamic write to admit nested same-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalMethodAppendKeyReplaysSiblingMapReadbackAcrossCall(t *testing.T) {
	src := `
local uuid = require("uuid")
local FlowGraph = {}
local mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
    }, mt)
end

function FlowGraph:create_node()
    local node_id = uuid.v7()
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    return node_id, nil
end

local function compile(graph)
    local commands = table.create(#graph.node_order * 2, 0)
    for _, node_id in ipairs(graph.node_order) do
        local edges = graph.edges[node_id]
        local data_targets = table.create(#edges.targets, 0)
        local error_targets = table.create(#edges.error_targets, 0)
        table.insert(commands, { data_targets = data_targets, error_targets = error_targets })
    end
    return commands
end

local graph = FlowGraph.new()
graph:create_node()
return compile(graph)
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected sibling map readback from appended key to cross method call, got diagnostics: %v", msgs)
	}
}

func TestCanonicalReturnedGraphEdgeArraysStayLengthableAfterIterationFlag(t *testing.T) {
	src := `
local uuid = require("uuid")
local FlowGraph = {}
local mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
    }, mt)
end

function FlowGraph:create_node()
    local node_id = uuid.v7()
    table.insert(self.node_order, node_id)
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    return node_id, nil
end

local function compile(graph)
    for _, node_id in ipairs(graph.node_order) do
        local edges = graph.edges[node_id]
        local has_explicit_edges = false
        for _, edge in ipairs(edges.targets) do
            if edge.target_node_id or edge.is_workflow_terminal then
                has_explicit_edges = true
                break
            end
        end
        for _, edge in ipairs(edges.error_targets) do
            if edge.target_node_id or edge.is_workflow_terminal then
                has_explicit_edges = true
                break
            end
        end
        if has_explicit_edges then
            local data_targets = table.create(#edges.targets, 0)
            local error_targets = table.create(#edges.error_targets, 0)
            return data_targets, error_targets
        end
    end
    return nil, nil
end

local graph = FlowGraph.new()
graph:create_node()
return compile(graph)
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected returned graph edge arrays to stay lengthable after iteration flag, got diagnostics: %v", msgs)
	}
}

func TestCanonicalBranchedTemplateAppendKeysReplaySiblingMapReadback(t *testing.T) {
	src := `
local uuid = require("uuid")
local FlowGraph = {}
local mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
    }, mt)
end

function FlowGraph:create_template_nodes(ops)
    for _, op in ipairs(ops) do
        if op.kind == "func" then
            local id = uuid.v7()
            table.insert(self.node_order, id)
            self.edges[id] = {
                targets = table.create(1, 0),
                error_targets = table.create(1, 0),
            }
        elseif op.kind == "agent" then
            local id = uuid.v7()
            table.insert(self.node_order, id)
            self.edges[id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0),
            }
        else
            local id = uuid.v7()
            table.insert(self.node_order, id)
            self.edges[id] = {
                targets = table.create(3, 0),
                error_targets = table.create(1, 0),
            }
        end
    end
end

local function compile(graph)
    for _, node_id in ipairs(graph.node_order) do
        local edges = graph.edges[node_id]
        local data_targets = table.create(#edges.targets, 0)
        local error_targets = table.create(#edges.error_targets, 0)
        if #data_targets > 0 or #error_targets > 0 then
            return true
        end
    end
    return false
end

local graph = FlowGraph.new()
graph:create_template_nodes({ { kind = "func" }, { kind = "agent" } })
return compile(graph)
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected branched template keys to replay sibling edge map readback, got diagnostics: %v", msgs)
	}
}

func TestCanonicalBranchedAppendedRouteRecordsPreserveTargetName(t *testing.T) {
	src := `
local function resolve(name: string | number | false | nil)
    return name
end

local function compile(ops)
    local pending_routes = table.create(8, 0)
    for _, op in ipairs(ops) do
        if op.kind == "to" then
            table.insert(pending_routes, {
                target_name = op.config.target,
                input_key = op.config.input_key,
            })
        elseif op.kind == "error_to" then
            table.insert(pending_routes, {
                target_name = op.config.target,
                input_key = op.config.input_key,
            })
        else
            table.insert(pending_routes, {
                target_name = op.config.target,
                input_key = nil,
            })
        end
    end

    for _, route in ipairs(pending_routes) do
        resolve(route.target_name)
    end
end

compile({
    { kind = "to", config = { target = "next", input_key = "result" } },
    { kind = "error_to", config = { target = "failed", input_key = "error" } },
})
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected branched appended route records to preserve target_name, got diagnostics: %v", msgs)
	}
}

func TestCanonicalAppendedStaticDataSourcesPreserveRoutesArray(t *testing.T) {
	src := `
local function resolve(name: string | number | false | nil)
    return name
end

local function compile(ops)
    local static_data_sources = table.create(4, 0)
    for _, op in ipairs(ops) do
        if op.kind == "data" then
            table.insert(static_data_sources, {
                data = op.config.data,
                routes = table.create(4, 0),
            })
        elseif op.kind == "route" then
            for _, source in ipairs(static_data_sources) do
                table.insert(source.routes, {
                    target_name = op.config.target,
                    input_key = op.config.input_key,
                })
            end
        end
    end

    for _, static_source in ipairs(static_data_sources) do
        local src = static_source
        if #src.routes > 0 then
            local first_route = src.routes[1]
            resolve(first_route.target_name)
        end
    end
end

compile({
    { kind = "data", config = { data = { value = 1 } } },
    { kind = "route", config = { target = "next", input_key = "result" } },
})
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected static data source routes to keep array shape, got diagnostics: %v", msgs)
	}
}

func TestCanonicalGuardedStaticMemberInstallAdmitsLaterValueUse(t *testing.T) {
	src := `
local function install(id: string)
    local store = { nodes = {} }
    store.nodes[id] = { config = { kind = "first" } }
    local prev = store.nodes[id]
    if not prev.config.data_targets then
        prev.config.data_targets = {}
    end
    table.insert(prev.config.data_targets, "next")
end

return install
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected guarded static member install to admit later value use, got diagnostics: %v", msgs)
	}
}

func TestCanonicalLoopCarriedGuardedStaticMemberInstallAdmitsLaterValueUse(t *testing.T) {
	src := `
type Op = { kind: string, id: string }

local function chain(ops: {Op})
    local store = { nodes = {} }
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = op.id
        if last_id then
            local prev = store.nodes[last_id]
            if not prev.config.data_targets then
                prev.config.data_targets = table.create(1, 0)
            end
            table.insert(prev.config.data_targets, { node_id = node_id })
        end
        if op.kind == "agent" then
            store.nodes[node_id] = { config = { agent = op.id } }
        else
            store.nodes[node_id] = { config = { func_id = op.id } }
        end
        last_id = node_id
    end
end

return chain
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected loop-carried guarded static member install to admit later value use, got diagnostics: %v", msgs)
	}
}

func TestCanonicalUnannotatedMethodLoopCarriesPreviousDynamicKey(t *testing.T) {
	src := `
local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = op.id
        if last_id then
            local prev = self.nodes[last_id]
            if not prev.config.data_targets then
                prev.config.data_targets = table.create(1, 0)
            end
            table.insert(prev.config.data_targets, { node_id = node_id })
        end
        if op.kind == "agent" then
            self.nodes[node_id] = { config = { agent = op.id } }
        else
            self.nodes[node_id] = { config = { func_id = op.id } }
        end
        last_id = node_id
    end
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected unannotated method loop to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalRecursiveMethodLoopCarriesPreviousDynamicKey(t *testing.T) {
	src := `
local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local ids = table.create(0, 0)
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = op.id
        if last_id then
            local prev = self.nodes[last_id]
            if not prev.config.data_targets then
                prev.config.data_targets = table.create(1, 0)
            end
            table.insert(prev.config.data_targets, { node_id = node_id })
        end
        self.nodes[node_id] = { config = { data_targets = {} } }
        table.insert(ids, node_id)
        if op.children then
            local child_ids = self:create_template_nodes(op.children)
            for _, child_id in ipairs(child_ids) do
                table.insert(ids, child_id)
            end
        end
        last_id = node_id
    end
    return ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected recursive method loop to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalRecursiveMethodUnionConfigGuardedInstallAdmitsLaterUse(t *testing.T) {
	src := `
local compiler = {}
compiler.OP_TYPES = { FUNC = "func", AGENT = "agent", CYCLE = "cycle" }

local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local ids = table.create(0, 0)
    local last_id = nil
    for _, op in ipairs(ops) do
        if op.type == compiler.OP_TYPES.FUNC then
            local node_id = op.config.id
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                input_transform = op.config.input_transform
            }
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = config }
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local node_id = op.config.id
            local config = {
                agent = op.config.agent,
                model = op.config.model,
                input_transform = op.config.input_transform
            }
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = config }
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local node_id = op.config.id
            local config = {
                continue_condition = op.config.continue_condition,
                initial_state = op.config.initial_state,
                input_transform = op.config.input_transform
            }
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = config }
            table.insert(ids, node_id)
            if op.config.template then
                local child_ids = self:create_template_nodes(op.config.template)
                for _, child_id in ipairs(child_ids) do
                    table.insert(ids, child_id)
                end
            end
            last_id = node_id
        end
    end
    if last_id then
        local last = self.nodes[last_id]
        if not last.config.error_targets then
            last.config.error_targets = table.create(1, 0)
        end
        table.insert(last.config.error_targets, { kind = "error" })
    end
    return ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected guarded installs on union config to admit later use, got diagnostics: %v", msgs)
	}
}

func TestCanonicalSetMetatableConstructorSeedsPrototypeReceiverLoopCarriedPreviousKey(t *testing.T) {
	src := `
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        nodes = table.create(0, 16)
    }, flow_graph_mt)
end

function FlowGraph:create_template_nodes(ops)
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = op.id
        if last_id then
            local prev = self.nodes[last_id]
            return prev.config
        end
        self.nodes[node_id] = { config = { data_targets = {} } }
        last_id = node_id
    end
    return nil
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected constructor-published prototype receiver to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalSetMetatableConstructorWithSiblingTablesKeepsPreviousDynamicKey(t *testing.T) {
	src := `
local uuid = require("uuid")

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        nodes = table.create(0, 16),
        node_order = table.create(16, 0),
        edges = table.create(0, 16)
    }, flow_graph_mt)
end

function FlowGraph:create_template_nodes(ops)
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = uuid.v7()
        if last_id then
            local prev = self.nodes[last_id]
            return prev.config
        end
        self.nodes[node_id] = { config = { data_targets = {} } }
        self.edges[node_id] = { targets = table.create(2, 0) }
        table.insert(self.node_order, node_id)
        last_id = node_id
    end
    return nil
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected constructor receiver with sibling tables to keep dynamic-key readback, got diagnostics: %v", msgs)
	}
}

func TestCanonicalFixtureTemplateNodesGuardedConfigTargets(t *testing.T) {
	src := `
local uuid = require("uuid")

local consts = {
    DATA_TYPE = { NODE_INPUT = "node_input", NODE_OUTPUT = "node_output" },
    STATUS = { TEMPLATE = "template" }
}

local compiler = {}
compiler.OP_TYPES = {
    FUNC = "func",
    AGENT = "agent",
    CYCLE = "cycle"
}

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        nodes = table.create(0, 16),
        node_order = table.create(16, 0),
        edges = table.create(0, 16)
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
            local template_node_id = uuid.v7()
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }
            if last_template_node_id then
                local prev_node = self.nodes[last_template_node_id]
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    discriminator = "default",
                    metadata = {
                        source_node_id = last_template_node_id
                    }
                })
            end
            local metadata = op.config.metadata or {}
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.func:node",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
            }
            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)
            last_template_node_id = template_node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local template_node_id = uuid.v7()
            local config = {
                agent = op.config.agent_id,
                model = op.config.model,
                arena = op.config.arena,
                inputs = op.config.inputs,
                show_tool_calls = op.config.show_tool_calls,
                input_transform = op.config.input_transform
            }
            if last_template_node_id then
                local prev_node = self.nodes[last_template_node_id]
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    discriminator = "default",
                    metadata = {
                        source_node_id = last_template_node_id
                    }
                })
            end
            local metadata = op.config.metadata or {}
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.agent:node",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
            }
            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)
            last_template_node_id = template_node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local template_node_id = uuid.v7()
            local config = {
                agent = op.config.agent_id,
                model = op.config.model,
                arena = op.config.arena,
                inputs = op.config.inputs,
                show_tool_calls = op.config.show_tool_calls,
                input_transform = op.config.input_transform
            }
            if last_template_node_id then
                local prev_node = self.nodes[last_template_node_id]
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    discriminator = "default",
                    metadata = {
                        source_node_id = last_template_node_id
                    }
                })
            end
            local metadata = op.config.metadata or {}
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.agent:node",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
            }
            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)
            last_template_node_id = template_node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local template_node_id = uuid.v7()
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                continue_condition = op.config.continue_condition,
                max_iterations = op.config.max_iterations,
                initial_state = op.config.initial_state,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }
            if last_template_node_id then
                local prev_node = self.nodes[last_template_node_id]
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    source_node_id = last_template_node_id,
                    discriminator = "default"
                })
            end
            local metadata = op.config.metadata or {}
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.cycle:cycle",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
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
        table.insert(last_node.config.data_targets, {
            data_type = consts.DATA_TYPE.NODE_OUTPUT,
            discriminator = "result",
            metadata = {
                source_node_id = last_template_node_id
            }
        })

        if not last_node.config.error_targets then
            last_node.config.error_targets = table.create(1, 0)
        end
        table.insert(last_node.config.error_targets, {
            data_type = consts.DATA_TYPE.NODE_OUTPUT,
            discriminator = "error",
            metadata = {
                source_node_id = last_template_node_id
            }
        })
    end

    return template_node_ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected fixture-shaped template node guarded config targets to admit later use, got diagnostics: %v", res.Diagnostics)
	}
}

func TestCanonicalFixtureBuildGraphTemplateContextKeepsGuardedConfigTargets(t *testing.T) {
	src := `
local uuid = require("uuid")

local consts = require("df_consts")

local compiler = {}
compiler.OP_TYPES = {
    FUNC = "func",
    AGENT = "agent",
    CYCLE = "cycle",
    PARALLEL = "parallel"
}

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        operations = table.create(16, 0),
        nodes = table.create(0, 16),
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
        input_routes = table.create(4, 0),
        static_data_sources = table.create(4, 0),
        last_node_id = nil,
        last_static_id = nil,
        pending_routes = table.create(8, 0),
        auto_chained = table.create(0, 16)
    }, flow_graph_mt)
end

function FlowGraph:add_operation(op_type, config)
    table.insert(self.operations, {
        type = op_type,
        config = config or {}
    })
    return self, nil
end

function FlowGraph:create_node(node_type, config, metadata)
    local node_id = uuid.v7()

    self.nodes[node_id] = {
        node_id = node_id,
        node_type = node_type,
        config = config or {},
        metadata = metadata or {},
        status = consts.STATUS.PENDING
    }

    table.insert(self.node_order, node_id)

    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0)
    }

    self.last_node_id = node_id
    self.last_static_id = nil
    return node_id, nil
end

function FlowGraph:create_template_nodes(template, parent_node_id)
    if not template or not template.operations then
        return table.create(0, 0)
    end

    local template_node_ids = table.create(#template.operations, 0)
    local last_template_node_id = nil

    for _, op in ipairs(template.operations) do
        if op.type == compiler.OP_TYPES.FUNC then
            local template_node_id = uuid.v7()
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }
            if last_template_node_id then
                local prev_node = self.nodes[last_template_node_id]
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    discriminator = "default",
                    metadata = {
                        source_node_id = last_template_node_id
                    }
                })
            end
            local metadata = op.config.metadata or {}
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.func:node",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
            }
            table.insert(self.node_order, template_node_id)
            table.insert(template_node_ids, template_node_id)
            last_template_node_id = template_node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local template_node_id = uuid.v7()
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                continue_condition = op.config.continue_condition,
                max_iterations = op.config.max_iterations,
                initial_state = op.config.initial_state,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }
            if last_template_node_id then
                local prev_node = self.nodes[last_template_node_id]
                if not prev_node.config.data_targets then
                    prev_node.config.data_targets = table.create(1, 0)
                end
                table.insert(prev_node.config.data_targets, {
                    data_type = consts.DATA_TYPE.NODE_INPUT,
                    node_id = template_node_id,
                    source_node_id = last_template_node_id,
                    discriminator = "default"
                })
            end
            local metadata = op.config.metadata or {}
            self.nodes[template_node_id] = {
                node_id = template_node_id,
                node_type = "userspace.dataflow.node.cycle:cycle",
                config = config,
                metadata = metadata,
                status = consts.STATUS.TEMPLATE,
                parent_node_id = parent_node_id
            }
            self.edges[template_node_id] = {
                targets = table.create(2, 0),
                error_targets = table.create(1, 0)
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
        table.insert(last_node.config.data_targets, {
            data_type = consts.DATA_TYPE.NODE_OUTPUT,
            discriminator = "result",
            metadata = {
                source_node_id = last_template_node_id
            }
        })

        if not last_node.config.error_targets then
            last_node.config.error_targets = table.create(1, 0)
        end
        table.insert(last_node.config.error_targets, {
            data_type = consts.DATA_TYPE.NODE_OUTPUT,
            discriminator = "error",
            metadata = {
                source_node_id = last_template_node_id
            }
        })
    end

    return template_node_ids
end

function compiler.build_graph(operations)
    local graph = FlowGraph.new()

    for _, op in ipairs(operations) do
        if op.type == compiler.OP_TYPES.FUNC then
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }
            local node_id, err = graph:create_node("userspace.dataflow.node.func:node", config, op.config.metadata)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.AGENT then
            local config = {
                agent = op.config.agent_id,
                model = op.config.model,
                arena = op.config.arena,
                inputs = op.config.inputs,
                show_tool_calls = op.config.show_tool_calls,
                input_transform = op.config.input_transform
            }
            local node_id, err = graph:create_node("userspace.dataflow.node.agent:node", config, op.config.metadata)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                continue_condition = op.config.continue_condition,
                max_iterations = op.config.max_iterations,
                initial_state = op.config.initial_state,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }
            local node_id, err = graph:create_node("userspace.dataflow.node.cycle:cycle", config, op.config.metadata)
            if err then
                return nil, err
            end
            if op.config.template then
                graph:create_template_nodes(op.config.template, node_id)
            end
        elseif op.type == compiler.OP_TYPES.PARALLEL then
            local config = {
                source_array_key = op.config.source_array_key,
                iteration_input_key = op.config.iteration_input_key,
                batch_size = op.config.batch_size,
                on_error = op.config.on_error,
                filter = op.config.filter,
                unwrap = op.config.unwrap,
                passthrough_keys = op.config.passthrough_keys,
                inputs = op.config.inputs,
                input_transform = op.config.input_transform
            }
            local node_id, err = graph:create_node("userspace.dataflow.node.parallel:parallel", config, op.config.metadata)
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

return compiler
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected build_graph template call context to keep guarded config targets, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsLoopCarriedPreviousKey(t *testing.T) {
	src := `
type NodeConfig = { data_targets: {string} }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function chain(self: Store, ids: {string}): NodeConfig?
    local last_id = nil
    for _, id in ipairs(ids) do
        if last_id then
            local prev = self.nodes[last_id]
            return prev.config
        end
        self.nodes[id] = { config = { data_targets = {} } }
        last_id = id
    end
    return nil
end

return chain
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected loop-carried dynamic write key to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsBranchedLoopCarriedPreviousKey(t *testing.T) {
	src := `
type Op = { kind: string, id: string }
type NodeConfig = { data_targets: {string} }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function chain(self: Store, ops: {Op}): NodeConfig?
    local last_id = nil
    for _, op in ipairs(ops) do
        if op.kind == "a" then
            local node_id = op.id .. "-a"
            if last_id then
                local prev = self.nodes[last_id]
                return prev.config
            end
            self.nodes[node_id] = { config = { data_targets = {} } }
            last_id = node_id
        elseif op.kind == "b" then
            local node_id = op.id .. "-b"
            if last_id then
                local prev = self.nodes[last_id]
                return prev.config
            end
            self.nodes[node_id] = { config = { data_targets = {} } }
            last_id = node_id
        end
    end
    return nil
end

return chain
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected branched loop-carried dynamic write key to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsOpaqueSamePathKey(t *testing.T) {
	src := `
local uuid = require("uuid")

type NodeConfig = { data_targets: {string} }
type Node = { config: NodeConfig }
type Store = { nodes: {[string]: Node} }

local function install(self: Store): NodeConfig
    local node_id = uuid.v7()
    self.nodes[node_id] = { config = { data_targets = {} } }
    local prev = self.nodes[node_id]
    return prev.config
end

return install
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected path-backed dynamic write key to admit same-path read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsOpaqueLoopCarriedPreviousKey(t *testing.T) {
	src := `
local uuid = require("uuid")

local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = uuid.v7()
        if last_id then
            local prev = self.nodes[last_id]
            return prev.config
        end
        self.nodes[node_id] = { config = { data_targets = {} } }
        last_id = node_id
    end
    return nil
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected opaque loop-carried dynamic write key to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsNestedIteratorLoopCarriedPreviousKey(t *testing.T) {
	src := `
local uuid = require("uuid")

local FlowGraph = {}

function FlowGraph:create_template_nodes(template)
    local last_id = nil
    for _, op in ipairs(template.operations) do
        local node_id = uuid.v7()
        if last_id then
            local prev = self.nodes[last_id]
            return prev.config
        end
        self.nodes[node_id] = { config = { data_targets = {} } }
        last_id = node_id
    end
    return nil
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected nested iterator loop-carried key to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsGuardedNestedIteratorLoopCarriedPreviousKey(t *testing.T) {
	src := `
local uuid = require("uuid")

local FlowGraph = {}

function FlowGraph:create_template_nodes(template)
    if not template or not template.operations then
        return table.create(0, 0)
    end

    local ids = table.create(#template.operations, 0)
    local last_id = nil
    for _, op in ipairs(template.operations) do
        local node_id = uuid.v7()
        if last_id then
            local prev = self.nodes[last_id]
            return prev.config
        end
        self.nodes[node_id] = { config = { data_targets = {} } }
        table.insert(ids, node_id)
        last_id = node_id
    end
    return ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected guarded nested iterator loop-carried key to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsGuardedNestedIteratorBranchedRecords(t *testing.T) {
	src := `
local uuid = require("uuid")

local compiler = {}
compiler.OP_TYPES = { FUNC = "func", AGENT = "agent", CYCLE = "cycle" }

local FlowGraph = {}

function FlowGraph:create_template_nodes(template, parent_node_id)
    if not template or not template.operations then
        return table.create(0, 0)
    end

    local ids = table.create(#template.operations, 0)
    local last_id = nil
    for _, op in ipairs(template.operations) do
        if op.type == compiler.OP_TYPES.FUNC then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = {
                node_id = node_id,
                config = { func_id = op.config.func_id, args = op.config.args },
                parent_node_id = parent_node_id
            }
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = {
                node_id = node_id,
                config = { agent = op.config.agent_id, model = op.config.model },
                parent_node_id = parent_node_id
            }
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = {
                node_id = node_id,
                config = { continue_condition = op.config.continue_condition },
                parent_node_id = parent_node_id
            }
            table.insert(ids, node_id)
            last_id = node_id
        end
    end
    return ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected guarded nested iterator branched records to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsGuardedNestedIteratorLocalConfigRecord(t *testing.T) {
	src := `
local uuid = require("uuid")

local compiler = {}
compiler.OP_TYPES = { FUNC = "func", AGENT = "agent" }

local FlowGraph = {}

function FlowGraph:create_template_nodes(template, parent_node_id)
    if not template or not template.operations then
        return table.create(0, 0)
    end

    local ids = table.create(#template.operations, 0)
    local last_id = nil
    for _, op in ipairs(template.operations) do
        if op.type == compiler.OP_TYPES.FUNC then
            local node_id = uuid.v7()
            local config = {
                func_id = op.config.func_id,
                args = op.config.args,
                inputs = op.config.inputs,
                context = op.config.context,
                input_transform = op.config.input_transform
            }
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            local metadata = op.config.metadata or {}
            self.nodes[node_id] = {
                node_id = node_id,
                node_type = "userspace.dataflow.node.func:node",
                config = config,
                metadata = metadata,
                status = "template",
                parent_node_id = parent_node_id
            }
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local node_id = uuid.v7()
            local config = {
                agent = op.config.agent_id,
                model = op.config.model,
                arena = op.config.arena,
                inputs = op.config.inputs,
                show_tool_calls = op.config.show_tool_calls,
                input_transform = op.config.input_transform
            }
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            local metadata = op.config.metadata or {}
            self.nodes[node_id] = {
                node_id = node_id,
                node_type = "userspace.dataflow.node.agent:node",
                config = config,
                metadata = metadata,
                status = "template",
                parent_node_id = parent_node_id
            }
            table.insert(ids, node_id)
            last_id = node_id
        end
    end
    return ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected local config record writes to preserve receiver dynamic-index proofs, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsGuardedNestedIteratorRecursiveBranch(t *testing.T) {
	src := `
local uuid = require("uuid")

local compiler = {}
compiler.OP_TYPES = { FUNC = "func", AGENT = "agent", CYCLE = "cycle" }

local FlowGraph = {}

function FlowGraph:create_template_nodes(template, parent_node_id)
    if not template or not template.operations then
        return table.create(0, 0)
    end

    local ids = table.create(#template.operations, 0)
    local last_id = nil
    for _, op in ipairs(template.operations) do
        if op.type == compiler.OP_TYPES.FUNC then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { func_id = op.config.func_id }, parent_node_id = parent_node_id }
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { continue_condition = op.config.continue_condition }, parent_node_id = parent_node_id }
            table.insert(ids, node_id)
            if op.config.template then
                local child_ids = self:create_template_nodes(op.config.template, node_id)
                for _, child_id in ipairs(child_ids) do
                    table.insert(ids, child_id)
                end
            end
            last_id = node_id
        end
    end
    if last_id then
        local last = self.nodes[last_id]
        if not last.config.error_targets then
            last.config.error_targets = table.create(1, 0)
        end
        table.insert(last.config.error_targets, { kind = "error" })
    end
    return ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected recursive branch to preserve receiver dynamic-index proofs, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsOpaqueLoopCarriedGuardedNestedInstall(t *testing.T) {
	src := `
local uuid = require("uuid")

local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local last_id = nil
    for _, op in ipairs(ops) do
        local node_id = uuid.v7()
        if last_id then
            local prev = self.nodes[last_id]
            if not prev.config.data_targets then
                prev.config.data_targets = table.create(1, 0)
            end
            table.insert(prev.config.data_targets, { node_id = node_id })
        end
        self.nodes[node_id] = { config = { data_targets = {} } }
        last_id = node_id
    end
    return nil
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected opaque loop-carried nested install to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsOpaqueBranchedLoopCarriedNestedInstall(t *testing.T) {
	src := `
local uuid = require("uuid")

local compiler = {}
compiler.OP_TYPES = { FUNC = "func", AGENT = "agent", CYCLE = "cycle" }

local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local last_id = nil
    for _, op in ipairs(ops) do
        if op.type == compiler.OP_TYPES.FUNC then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { func_id = op.config.func_id, args = op.config.args } }
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { agent = op.config.agent_id, model = op.config.model } }
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { continue_condition = op.config.continue_condition } }
            last_id = node_id
        end
    end
    return nil
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected opaque branched nested install to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalDynamicIndexWriteAdmitsOpaqueBranchedLoopCarriedSiblingWrites(t *testing.T) {
	src := `
local uuid = require("uuid")

local compiler = {}
compiler.OP_TYPES = { FUNC = "func", AGENT = "agent", CYCLE = "cycle" }

local FlowGraph = {}

function FlowGraph:create_template_nodes(ops)
    local ids = table.create(0, 0)
    local last_id = nil
    for _, op in ipairs(ops) do
        if op.type == compiler.OP_TYPES.FUNC then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { func_id = op.config.func_id, args = op.config.args } }
            self.edges[node_id] = { targets = table.create(2, 0), error_targets = table.create(1, 0) }
            table.insert(self.node_order, node_id)
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.AGENT then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { agent = op.config.agent_id, model = op.config.model } }
            self.edges[node_id] = { targets = table.create(2, 0), error_targets = table.create(1, 0) }
            table.insert(self.node_order, node_id)
            table.insert(ids, node_id)
            last_id = node_id
        elseif op.type == compiler.OP_TYPES.CYCLE then
            local node_id = uuid.v7()
            if last_id then
                local prev = self.nodes[last_id]
                if not prev.config.data_targets then
                    prev.config.data_targets = table.create(1, 0)
                end
                table.insert(prev.config.data_targets, { node_id = node_id })
            end
            self.nodes[node_id] = { config = { continue_condition = op.config.continue_condition } }
            self.edges[node_id] = { targets = table.create(2, 0), error_targets = table.create(1, 0) }
            table.insert(self.node_order, node_id)
            table.insert(ids, node_id)
            last_id = node_id
        end
    end
    if last_id then
        local last = self.nodes[last_id]
        if not last.config.error_targets then
            last.config.error_targets = table.create(1, 0)
        end
        table.insert(last.config.error_targets, { kind = "error" })
    end
    return ids
end

return FlowGraph
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected opaque branched sibling writes to admit previous-key read, got diagnostics: %v", msgs)
	}
}

func TestCanonicalPendingRouteFromLastNodeReadsEdgeRecord(t *testing.T) {
	src := `
local uuid = require("uuid")

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        edges = table.create(0, 16),
        pending_routes = table.create(8, 0),
        last_node_id = nil,
    }, flow_graph_mt)
end

function FlowGraph:create_node()
    local node_id = uuid.v7()
    self.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    self.last_node_id = node_id
    return node_id, nil
end

function FlowGraph:add_pending_route()
    if self.last_node_id then
        table.insert(self.pending_routes, {
            from_node_id = self.last_node_id,
            is_workflow_terminal = true,
            is_success = true,
            is_error = false,
            condition = nil,
            transform = nil,
        })
    end
end

local function build()
    local graph = FlowGraph.new()
    local _, err = graph:create_node()
    if err then
        return nil, err
    end
    graph:add_pending_route()

    for _, route_entry in ipairs(graph.pending_routes) do
        if route_entry.is_workflow_terminal then
            local edges = graph.edges[route_entry.from_node_id]
            local edge_list = route_entry.is_error and edges.error_targets or edges.targets
            table.insert(edge_list, {
                target_node_id = nil,
                is_workflow_terminal = true,
                is_success = route_entry.is_success,
                transform = route_entry.transform,
                condition = route_entry.condition,
            })
        end
    end
    return graph, nil
end

return build
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithManifest("uuid", canonicalUUIDManifest()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected pending route from last_node_id to read edge record, got diagnostics: %v", msgs)
	}
}

func canonicalUUIDManifest() *io.Manifest {
	m := io.NewManifest("uuid")
	m.SetExport(typ.NewInterface("uuid", []typ.Method{
		{Name: "v7", Type: typ.Func().Returns(typ.String).Build()},
	}))
	return m
}

func TestCanonicalPendingRouteInlineFromLastNodeReadsEdgeRecord(t *testing.T) {
	src := `
local uuid = require("uuid")

local function build()
    local graph = {
        edges = table.create(0, 16),
        pending_routes = table.create(8, 0),
        last_node_id = nil,
    }
    local node_id = uuid.v7()
    graph.edges[node_id] = {
        targets = table.create(4, 0),
        error_targets = table.create(2, 0),
    }
    graph.last_node_id = node_id
    if graph.last_node_id then
        table.insert(graph.pending_routes, {
            from_node_id = graph.last_node_id,
            is_workflow_terminal = true,
            is_success = true,
            is_error = false,
            condition = nil,
            transform = nil,
        })
    end

    for _, route_entry in ipairs(graph.pending_routes) do
        if route_entry.is_workflow_terminal then
            local edges = graph.edges[route_entry.from_node_id]
            local edge_list = route_entry.is_error and edges.error_targets or edges.targets
            table.insert(edge_list, {
                target_node_id = nil,
                is_workflow_terminal = true,
                is_success = route_entry.is_success,
                transform = route_entry.transform,
                condition = route_entry.condition,
            })
        end
    end
    return graph, nil
end

return build
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected inline pending route from last_node_id to read edge record, got diagnostics: %v", msgs)
	}
}

func TestCanonicalMethodCallPublishesReceiverStaticMemberWrite(t *testing.T) {
	src := `
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        last_node_id = nil,
    }, flow_graph_mt)
end

function FlowGraph:create_node()
    local node_id = "node-1"
    self.last_node_id = node_id
    return node_id, nil
end

local function build()
    local graph = FlowGraph.new()
    local _, err = graph:create_node()
    if err then
        return nil, err
    end
    if graph.last_node_id then
        local id: string = graph.last_node_id
        return id, nil
    end
    return nil, "missing"
end

return build
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected method call to publish receiver static member write, got diagnostics: %v", msgs)
	}
}
