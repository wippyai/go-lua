package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

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
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected fixture-shaped template node guarded config targets to admit later use, got diagnostics: %v", msgs)
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
	res := testutil.Check(src, testutil.WithStdlib())
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
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected opaque loop-carried dynamic write key to admit previous-key read, got diagnostics: %v", msgs)
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
	res := testutil.Check(src, testutil.WithStdlib())
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
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected opaque branched sibling writes to admit previous-key read, got diagnostics: %v", msgs)
	}
}
