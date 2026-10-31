local json = require("json")
local uuid = require("uuid")
local expr = require("expr")
local consts = require("df_consts")

local compiler = {}

compiler.OP_TYPES = {
    WITH_INPUT = "with_input",
    WITH_DATA = "with_data",
    FUNC = "func",
    AGENT = "agent",
    CYCLE = "cycle",
    PARALLEL = "parallel",
    SIGNAL = "signal",
    STATE = "state",
    USE = "use",
    AS = "as",
    TO = "to",
    ERROR_TO = "error_to",
    WHEN = "when"
}

local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
    return setmetatable({
        operations = table.create(16, 0),
        nodes = table.create(0, 16),
        node_order = table.create(16, 0),
        edges = table.create(0, 16),
        references = table.create(0, 8),
        input_data = nil,
        input_name = nil,
        input_routes = table.create(4, 0),
        static_data_sources = table.create(4, 0),
        last_node_id = nil,
        last_static_id = nil,
        last_node_name = nil,
        last_route_from_static = false,
        pending_routes = table.create(8, 0),
        has_explicit_routing = false,
        session_parent_id = nil,
        forced_success_nodes = table.create(0, 4),
        forced_failure_nodes = table.create(0, 4),
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

function FlowGraph:create_static_data(data)
    local static_id = uuid.v7()

    table.insert(self.static_data_sources, {
        static_id = static_id,
        data = data,
        routes = table.create(4, 0)
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
                local prev_node = (self.nodes[last_template_node_id] :: any)
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
                active_traits = op.config.active_traits,
                active_tools = op.config.active_tools,
                show_tool_calls = op.config.show_tool_calls,
                input_transform = op.config.input_transform
            }

            if last_template_node_id then
                local prev_node = (self.nodes[last_template_node_id] :: any)
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
                local prev_node = (self.nodes[last_template_node_id] :: any)
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
        local last_node = (self.nodes[last_template_node_id] :: any)
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

function FlowGraph:add_reference(name, node_id)
    if self.references[name] then
        return nil, "Duplicate node name: " .. name
    end
    self.references[name] = node_id
    self.last_node_name = name
    return true, nil
end

function FlowGraph:resolve_reference(name)
    local node_id = self.references[name]
    if not node_id then
        return nil, "Undefined node reference: " .. name
    end
    return node_id, nil
end

function FlowGraph:compute_auto_chain()
    for i = 1, #self.node_order - 1 do
        local current_node_id = self.node_order[i]
        local next_node_id = self.node_order[i + 1]
        local current_node = (self.nodes[current_node_id] :: any)
        local next_node = (self.nodes[next_node_id] :: any)

        if not current_node.parent_node_id and not next_node.parent_node_id then
            local current_edges = (self.edges[current_node_id] :: any)

            local has_any_targets = false
            for _, edge in ipairs(current_edges.targets) do
                if edge.target_node_id or edge.is_workflow_terminal then
                    has_any_targets = true
                    break
                end
            end
            for _, edge in ipairs(current_edges.error_targets) do
                if edge.target_node_id or edge.is_workflow_terminal then
                    has_any_targets = true
                    break
                end
            end

            if not has_any_targets then
                self.auto_chained[current_node_id] = next_node_id
                table.insert(current_edges.targets, {
                    target_node_id = next_node_id,
                    discriminator = "default",
                    is_auto_chain = true,
                    metadata = {
                        source_node_id = current_node_id
                    }
                })
            end
        end
    end
end

function FlowGraph:detect_cycles()
    local visited = table.create(0, 32)
    local rec_stack = table.create(0, 32)

    local function dfs(node_id, path)
        if rec_stack[node_id] then
            local cycle_start = nil
            for i, id in ipairs(path) do
                if id == node_id then
                    cycle_start = i
                    break
                end
            end

            if cycle_start then
                local cycle = table.create(#path - cycle_start + 2, 0)
                for i = cycle_start, #path do
                    table.insert(cycle, path[i])
                end
                table.insert(cycle, node_id)
                return true, "Cycle detected: " .. table.concat(cycle, " -> ")
            end
            return true, "Cycle detected at node: " .. node_id
        end

        if visited[node_id] then
            return false, nil
        end

        visited[node_id] = true
        rec_stack[node_id] = true
        table.insert(path, node_id)

        local edges = (self.edges[node_id] :: any)
        if edges then
            for _, edge in ipairs(edges.targets) do
                if edge.target_node_id then
                    local has_cycle, cycle_desc = dfs(edge.target_node_id, path)
                    if has_cycle then
                        return true, cycle_desc
                    end
                end
            end
            for _, edge in ipairs(edges.error_targets) do
                if edge.target_node_id then
                    local has_cycle, cycle_desc = dfs(edge.target_node_id, path)
                    if has_cycle then
                        return true, cycle_desc
                    end
                end
            end
        end

        rec_stack[node_id] = false
        table.remove(path)
        return false, nil
    end

    for node_id, _ in pairs(self.nodes) do
        if not visited[node_id] then
            local has_cycle, cycle_desc = dfs(node_id, table.create(16, 0))
            if has_cycle then
                return true, cycle_desc
            end
        end
    end

    return false, nil
end

function compiler.build_graph(operations, session_context)
    if not operations or #operations == 0 then
        return nil, "No operations provided"
    end

    local graph = FlowGraph.new() :: any

    if session_context and session_context.node_id then
        graph.session_parent_id = session_context.node_id
    end

    for _, op in ipairs(operations) do
        if op.type == compiler.OP_TYPES.WITH_INPUT then
            graph.input_data = op.config.data
        elseif op.type == compiler.OP_TYPES.WITH_DATA then
            local static_id, err = graph:create_static_data(op.config.data)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.FUNC then
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
                active_traits = op.config.active_traits,
                active_tools = op.config.active_tools,
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
                scheduling = op.config.scheduling,
                on_error = op.config.on_error,
                filter = op.config.filter,
                unwrap = op.config.unwrap,
                passthrough_keys = op.config.passthrough_keys,
                inputs = op.config.inputs,
                input_transform = op.config.input_transform
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.parallel:parallel", config,
                op.config.metadata)
            if err then
                return nil, err
            end

            if op.config.template then
                graph:create_template_nodes(op.config.template, node_id)
            end
        elseif op.type == compiler.OP_TYPES.SIGNAL then
            local config = {
                signal_id = op.config.signal_id,
                timeout = op.config.timeout,
                inputs = op.config.inputs,
                input_transform = op.config.input_transform,
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.signal:node", config, op.config.metadata)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.STATE then
            local config = {
                inputs = op.config.inputs,
                input_transform = op.config.input_transform,
                output_mode = op.config.output_mode,
                ignored_keys = op.config.ignored_keys
            }

            local node_id, err = graph:create_node("userspace.dataflow.node.state:state", config, op.config.metadata)
            if err then
                return nil, err
            end
        elseif op.type == compiler.OP_TYPES.USE then
            if op.config.operations then
                for _, template_op in ipairs(op.config.operations) do
                    table.insert(graph.operations, template_op)
                end
            end
        elseif op.type == compiler.OP_TYPES.AS then
            if graph.last_static_id then
                graph:add_reference(op.config.name, graph.last_static_id)
            elseif graph.input_data and not graph.input_name and not graph.last_node_id then
                graph.input_name = op.config.name
                graph:add_reference(op.config.name, "INPUT")
            elseif graph.last_node_id then
                local success, err = graph:add_reference(op.config.name, graph.last_node_id)
                if err then
                    return nil, err
                end
            else
                return nil, "Cannot name node: no previous node, input, or static data to name: " .. op.config.name
            end
        elseif op.type == compiler.OP_TYPES.TO then
            if op.config.target == "@success" or op.config.target == "@fail" or op.config.target == "@end" then
                if not graph.last_node_id then
                    return nil, "Cannot route to terminal: no source node"
                end

                local is_success = op.config.target == "@success" or (op.config.target == "@end")

                if is_success then
                    graph.forced_success_nodes[graph.last_node_id] = true
                else
                    graph.forced_failure_nodes[graph.last_node_id] = true
                end

                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    is_workflow_terminal = true,
                    is_success = is_success,
                    transform = op.config.transform,
                    condition = nil,
                    is_error = false
                })
            elseif graph.input_data and not graph.last_node_id and not graph.last_static_id then
                table.insert(graph.input_routes, {
                    target_name = op.config.target,
                    input_key = op.config.input_key or graph.input_name or "default",
                    transform = op.config.transform
                })
                graph.has_explicit_routing = true
                graph.last_route_from_static = false
            elseif graph.last_static_id then
                for _, static_source in ipairs(graph.static_data_sources) do
                    if (static_source :: any).static_id == graph.last_static_id then
                        table.insert((static_source :: any).routes, {
                            target_name = op.config.target,
                            input_key = op.config.input_key or graph.last_node_name or "default",
                            transform = op.config.transform
                        })
                        break
                    end
                end
                graph.has_explicit_routing = true
                graph.last_route_from_static = true
            elseif graph.last_node_id then
                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                local discriminator = op.config.input_key
                if not discriminator and graph.last_node_name then
                    discriminator = graph.last_node_name
                end
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    target_name = op.config.target,
                    input_key = discriminator,
                    transform = op.config.transform,
                    is_error = false,
                    condition = nil
                })
            else
                return nil, "Cannot add route: no source node, input, or static data"
            end
        elseif op.type == compiler.OP_TYPES.ERROR_TO then
            if op.config.target == "@success" or op.config.target == "@fail" or op.config.target == "@end" then
                if not graph.last_node_id then
                    return nil, "Cannot route to terminal: no source node"
                end

                local is_success = op.config.target == "@success"

                if is_success then
                    graph.forced_success_nodes[graph.last_node_id] = true
                else
                    graph.forced_failure_nodes[graph.last_node_id] = true
                end

                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    is_workflow_terminal = true,
                    is_success = is_success,
                    transform = op.config.transform,
                    is_error = true,
                    condition = nil
                })
            else
                if not graph.last_node_id then
                    return nil, "Cannot add error route: no source node"
                end
                graph.has_explicit_routing = true
                graph.last_route_from_static = false
                local discriminator = op.config.input_key
                if not discriminator and graph.last_node_name then
                    discriminator = graph.last_node_name
                end
                table.insert(graph.pending_routes, {
                    from_node_id = graph.last_node_id,
                    target_name = op.config.target,
                    input_key = discriminator,
                    transform = op.config.transform,
                    is_error = true,
                    condition = nil
                })
            end
        elseif op.type == compiler.OP_TYPES.WHEN then
            if graph.last_route_from_static then
                return nil,
                    "Cannot use :when() with static data routes. Static data is constant and conditions would always evaluate the same way."
            end
            if #graph.pending_routes == 0 then
                return nil, "Cannot add condition: no preceding route from a node"
            end
            (graph.pending_routes[#graph.pending_routes] :: any).condition = op.config.condition
        end

        local success, err = graph:add_operation(op.type, op.config)
        if err then
            return nil, err
        end
    end

    for _, route in ipairs(graph.pending_routes) do
        local route_entry = (route :: any)
        if route_entry.is_workflow_terminal then
            local edges = (graph.edges[route_entry.from_node_id] :: any)
            local edge_list = route_entry.is_error and edges.error_targets or edges.targets
            table.insert(edge_list, {
                target_node_id = nil,
                is_workflow_terminal = true,
                is_success = route_entry.is_success,
                transform = route_entry.transform,
                condition = route_entry.condition
            })
        else
            local target_node_id, resolve_err = graph:resolve_reference(route_entry.target_name)
            if resolve_err then
                return nil, resolve_err
            end
            local edges = (graph.edges[route_entry.from_node_id] :: any)
            local edge_list = route_entry.is_error and edges.error_targets or edges.targets
            table.insert(edge_list, {
                target_node_id = target_node_id,
                transform = route_entry.transform,
                condition = route_entry.condition,
                input_key = route_entry.input_key
            })
        end
    end

    graph:compute_auto_chain()

    local has_cycles, cycle_desc = graph:detect_cycles()
    if has_cycles then
        return nil, "Flow contains cycles: " .. cycle_desc
    end

    return graph, nil
end

function compiler.validate_graph(graph)
    return true, nil
end

function compiler.compile_to_commands(graph, session_context)
    return {}, nil
end

function compiler.compile(operations, session_context)
    if not operations or #operations == 0 then
        return nil, "No operations to compile"
    end

    local graph, graph_err = compiler.build_graph(operations, session_context)
    if graph_err then
        return nil, graph_err
    end

    local valid, validation_err = compiler.validate_graph(graph)
    if not valid then
        return nil, validation_err
    end

    local commands, commands_err = compiler.compile_to_commands(graph, session_context)
    if commands_err then
        return nil, commands_err
    end

    return {
        commands = commands,
        graph = graph
    }, nil
end

return compiler
