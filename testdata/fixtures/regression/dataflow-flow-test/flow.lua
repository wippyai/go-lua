local json = require("json")
local uuid = require("uuid")
local ctx = require("ctx")
local agent_ref = require("agent_ref")
local compiler = require("df_compiler")
local client = require("df_client")
local consts = require("df_consts")

local DEFAULTS = {
    AGENT_MAX_ITERATIONS = 32,
    AGENT_MIN_ITERATIONS = 1,
    PARALLEL_BATCH_SIZE = 1,
    PARALLEL_SCHEDULING = "batch",
    PARALLEL_ON_ERROR = "continue",
    PARALLEL_FILTER = "all",
    PARALLEL_UNWRAP = false,
    PARALLEL_ITERATION_INPUT_KEY = "default"
}

local flow = {}

local function get_registry_title(id)
    local resolved = agent_ref.lookup(id)
    return resolved and resolved.title or nil
end

local FlowBuilder = {}
local flow_builder_mt = { __index = FlowBuilder }

function FlowBuilder.new()
    return setmetatable({
        operations = {},
        is_template = false,
        title = nil,
        metadata = nil
    }, flow_builder_mt)
end

function FlowBuilder:_mark_as_template()
    self.is_template = true
    return self
end

function FlowBuilder:_add_operation(op_type, config)
    table.insert(self.operations, {
        type = op_type,
        config = config or {}
    })
    return self
end

function FlowBuilder:with_title(title)
    if not title or title == "" then
        error("Workflow title cannot be empty")
    end
    self.title = title
    return self
end

function FlowBuilder:with_metadata(metadata)
    if type(metadata) ~= "table" then
        error("Metadata must be a table")
    end
    self.metadata = metadata
    return self
end

function FlowBuilder:with_input(data)
    return self:_add_operation(compiler.OP_TYPES.WITH_INPUT, { data = data })
end

function FlowBuilder:with_data(data)
    return self:_add_operation(compiler.OP_TYPES.WITH_DATA, { data = data })
end

function FlowBuilder:func(func_id, config)
    if not func_id or func_id == "" then
        error("Function ID is required")
    end

    local func_config = {
        func_id = func_id,
        args = config and config.args,
        inputs = config and config.inputs,
        context = config and config.context,
        metadata = config and config.metadata,
        input_transform = config and config.input_transform
    }

    if not (func_config.metadata and func_config.metadata.title) then
        local registry_title = get_registry_title(func_id)
        if registry_title then
            if not func_config.metadata then
                func_config.metadata = {}
            end
            func_config.metadata.title = registry_title
        end
    end

    return self:_add_operation(compiler.OP_TYPES.FUNC, func_config)
end

function FlowBuilder:agent(agent_id, config)
    agent_id = agent_id or ""
    config = config or {}
    local arena_config = config.arena or {}

    local agent_config = {
        agent_id = agent_id ~= "" and agent_id or nil,
        model = config.model,
        arena = {
            prompt = arena_config.prompt,
            max_iterations = arena_config.max_iterations or DEFAULTS.AGENT_MAX_ITERATIONS,
            min_iterations = arena_config.min_iterations or DEFAULTS.AGENT_MIN_ITERATIONS,
            max_unproductive_steps = arena_config.max_unproductive_steps,
            tool_calling = arena_config.tool_calling,
            exit_schema = arena_config.exit_schema,
            exit_func_id = arena_config.exit_func_id,
            tools = arena_config.tools,
            context = arena_config.context
        },
        inputs = config.inputs,
        active_traits = config.active_traits,
        active_tools = config.active_tools,
        show_tool_calls = config.show_tool_calls,
        metadata = config.metadata,
        input_transform = config.input_transform
    }

    if not (agent_config.metadata and agent_config.metadata.title) then
        if agent_id ~= "" then
            local registry_title = get_registry_title(agent_id)
            if registry_title then
                if not agent_config.metadata then
                    agent_config.metadata = {}
                end
                agent_config.metadata.title = registry_title
            end
        else
            if not agent_config.metadata then
                agent_config.metadata = {}
            end
            agent_config.metadata.title = "Agent (dynamic)"
        end
    end

    return self:_add_operation(compiler.OP_TYPES.AGENT, agent_config)
end

function FlowBuilder:cycle(config)
    if not config then
        error("Cycle configuration is required")
    end

    if not config.func_id and not config.template then
        error("Cycle requires either func_id or template")
    end

    if config.func_id and config.template then
        error("Cycle cannot have both func_id and template")
    end

    local cycle_config = {
        func_id = config.func_id,
        template = config.template,
        args = config.args,
        continue_condition = config.continue_condition,
        max_iterations = config.max_iterations,
        initial_state = config.initial_state,
        inputs = config.inputs,
        context = config.context,
        metadata = config.metadata,
        input_transform = config.input_transform
    }

    if not (cycle_config.metadata and cycle_config.metadata.title) then
        if config.func_id then
            local registry_title = get_registry_title(config.func_id --[[@as string]])
            if registry_title then
                if not cycle_config.metadata then
                    cycle_config.metadata = {}
                end
                cycle_config.metadata.title = registry_title .. " (Cycle)"
            end
        else
            if not cycle_config.metadata then
                cycle_config.metadata = {}
            end
            cycle_config.metadata.title = "Iterate"
        end
    end

    return self:_add_operation(compiler.OP_TYPES.CYCLE, cycle_config)
end

function FlowBuilder:parallel(config)
    if not config then
        error("Parallel configuration is required")
    end

    if not config.source_array_key then
        error("Parallel requires source_array_key")
    end

    if not config.template then
        error("Parallel requires template")
    end

    local parallel_config = {
        source_array_key = config.source_array_key,
        iteration_input_key = config.iteration_input_key or DEFAULTS.PARALLEL_ITERATION_INPUT_KEY,
        batch_size = config.batch_size or DEFAULTS.PARALLEL_BATCH_SIZE,
        scheduling = config.scheduling or DEFAULTS.PARALLEL_SCHEDULING,
        on_error = config.on_error or DEFAULTS.PARALLEL_ON_ERROR,
        filter = config.filter or DEFAULTS.PARALLEL_FILTER,
        unwrap = config.unwrap,
        passthrough_keys = config.passthrough_keys,
        template = config.template,
        inputs = config.inputs,
        metadata = config.metadata,
        input_transform = config.input_transform
    }

    if parallel_config.unwrap == nil then
        parallel_config.unwrap = DEFAULTS.PARALLEL_UNWRAP
    end

    if not (parallel_config.metadata and parallel_config.metadata.title) then
        if not parallel_config.metadata then
            parallel_config.metadata = {}
        end
        parallel_config.metadata.title = "Process Items"
    end

    return self:_add_operation(compiler.OP_TYPES.PARALLEL, parallel_config)
end

function FlowBuilder:signal(config)
    config = config or {}

    local signal_config = {
        signal_id = config.signal_id,
        timeout = config.timeout,
        inputs = config.inputs,
        metadata = config.metadata,
        input_transform = config.input_transform,
    }

    if not (signal_config.metadata and signal_config.metadata.title) then
        if not signal_config.metadata then
            signal_config.metadata = {}
        end
        signal_config.metadata.title = "Signal: " .. (config.signal_id or "auto")
    end

    return self:_add_operation(compiler.OP_TYPES.SIGNAL, signal_config)
end

function FlowBuilder:join(config)
    config = config or {}

    local join_config = {
        inputs = config.inputs,
        metadata = config.metadata,
        input_transform = config.input_transform,
        output_mode = config.output_mode,
        ignored_keys = config.ignored_keys
    }

    if not (join_config.metadata and join_config.metadata.title) then
        if not join_config.metadata then
            join_config.metadata = {}
        end
        join_config.metadata.title = "Join"
    end

    return self:_add_operation(compiler.OP_TYPES.STATE, join_config)
end

function FlowBuilder:use(template)
    if not template then
        error("Template is required for use operation")
    end

    if type(template) == "table" and template.operations then
        return self:_add_operation(compiler.OP_TYPES.USE, { operations = template.operations })
    else
        return self:_add_operation(compiler.OP_TYPES.USE, { template = template })
    end
end

function FlowBuilder:as(name)
    if not name or name == "" then
        error("Node name is required")
    end
    return self:_add_operation(compiler.OP_TYPES.AS, { name = name })
end

function FlowBuilder:to(target, input_key, transform)
    if not target or target == "" then
        error("Route target is required")
    end
    return self:_add_operation(compiler.OP_TYPES.TO, {
        target = target,
        input_key = input_key,
        transform = transform
    })
end

function FlowBuilder:error_to(target, input_key, transform)
    if not target or target == "" then
        error("Error route target is required")
    end
    return self:_add_operation(compiler.OP_TYPES.ERROR_TO, {
        target = target,
        input_key = input_key,
        transform = transform
    })
end

function FlowBuilder:when(condition)
    if not condition or condition == "" then
        error("Route condition is required")
    end
    return self:_add_operation(compiler.OP_TYPES.WHEN, { condition = condition })
end

function FlowBuilder:run()
    if #self.operations == 0 then
        return nil, "No operations to execute"
    end

    local session_context, ctx_err = ctx.all()
    if ctx_err then
        session_context = {}
    end

    local compilation_result, compile_err = compiler.compile(self.operations, session_context)
    if compile_err then
        return nil, "Compilation failed: " .. compile_err
    end

    local commands = compilation_result.commands

    if session_context.dataflow_id then
        return {
            _control = {
                commands = commands
            }
        }, nil
    else
        local c, client_err = client.new()
        if client_err then
            return nil, "Failed to create dataflow client: " .. client_err
        end

        local workflow_metadata = self.metadata or {}
        workflow_metadata.title = self.title or "Flow Builder Workflow"
        workflow_metadata.created_by = "flow_builder"

        local dataflow_id, create_err = (c --[[@as any]]):create_workflow(commands, {
            metadata = workflow_metadata
        })

        if create_err then
            return nil, "Failed to create workflow: " .. create_err
        end

        local outputs, exec_err = (c --[[@as any]]):execute(dataflow_id, {
            init_func_id = "userspace.dataflow.session:artifact"
        })

        if exec_err then
            return nil, "Failed to execute workflow: " .. exec_err
        end

        if outputs.data then
            return outputs.data, nil
        end

        return outputs, nil
    end
end

function FlowBuilder:start()
    if #self.operations == 0 then
        return nil, "No operations to execute"
    end

    local session_context, ctx_err = ctx.all()
    if ctx_err then
        session_context = {}
    end

    if session_context.dataflow_id then
        return nil, "Cannot start async workflow from nested context"
    end

    local compilation_result, compile_err = compiler.compile(self.operations, session_context)
    if compile_err then
        return nil, "Compilation failed: " .. compile_err
    end

    local commands = compilation_result.commands

    local c, client_err = client.new()
    if client_err then
        return nil, "Failed to create dataflow client: " .. client_err
    end

    local workflow_metadata = self.metadata or {}
    workflow_metadata.title = self.title or "Flow Builder Workflow"
    workflow_metadata.created_by = "flow_builder"

    local dataflow_id, create_err = (c --[[@as any]]):create_workflow(commands, {
        metadata = workflow_metadata
    })

    if create_err then
        return nil, "Failed to create workflow: " .. create_err
    end

    local start_result, start_err = (c --[[@as any]]):start(dataflow_id, {
        init_func_id = "userspace.dataflow.session:artifact"
    })

    if start_err then
        return nil, "Failed to start workflow: " .. start_err
    end

    return dataflow_id, nil
end

function flow.create()
    return FlowBuilder.new()
end

function flow.template()
    return (FlowBuilder.new() :: any):_mark_as_template()
end

return flow
