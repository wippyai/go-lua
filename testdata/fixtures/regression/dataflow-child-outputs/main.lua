local func = {}

local ERROR_MISSING_FUNC_ID = "MISSING_FUNC_ID"
local ERROR_NO_INPUT_DATA = "NO_INPUT_DATA"
local ERROR_FUNCTION_CANCELED = "FUNCTION_CANCELED"
local ERROR_FUNCTION_EXECUTION_FAILED = "FUNCTION_EXECUTION_FAILED"

func._deps = {
    node = require("node"),
    funcs = require("funcs"),
    consts = require("consts"),
    child_output = require("child_output")
}

local function error_message(error_value: any, fallback: string)
    if type(error_value) == "string" then
        return error_value
    end
    if type(error_value) == "table" then
        local error_table = error_value :: any
        if type(error_table.message) == "string" then
            return error_table.message
        end
        if type(error_table.status) == "string" then
            return error_table.status
        end
        if error_table.error ~= nil then
            return error_message(error_table.error, fallback)
        end
        if type(error_table.code) == "string" then
            return error_table.code
        end
        return fallback
    end
    if error_value == nil then
        return fallback
    end
    return tostring(error_value)
end

local function prefixed_error(prefix: string, error_value: any, fallback: string)
    if type(error_value) == "table" then
        return error_value
    end
    return prefix .. error_message(error_value, fallback)
end

local function build_execution_context_with_dataflow(base_context, dataflow_id, node_id, path)
    local execution_context = {}

    if base_context then
        for k, v in pairs(base_context) do
            execution_context[k] = v
        end
    end

    execution_context.dataflow_id = dataflow_id
    execution_context.node_id = node_id
    execution_context.path = path or {}
    execution_context.runtime_type = "dataflow_function"
    execution_context.execution_timestamp = os.time()
    execution_context.parent_node_id = node_id

    return execution_context
end

local function merge_inputs_with_base_args(inputs, base_args)
    if not inputs or next(inputs) == nil then
        if not base_args then
            return nil
        end
        return base_args
    end

    if not base_args then
        if inputs["default"] and inputs["default"].content ~= nil then
            return inputs["default"].content
        end

        if inputs[""] and inputs[""].content ~= nil then
            return inputs[""].content
        end

        local input_count = 0
        for _ in pairs(inputs) do
            input_count = input_count + 1
        end

        if input_count == 1 then
            for key, input in pairs(inputs) do
                local discriminator = input.discriminator

                if discriminator == nil or discriminator == "" or discriminator == "default" then
                    return input.content
                end

                return {
                    [discriminator or key] = input.content
                }
            end
        end

        local result = {}
        for key, input in pairs(inputs) do
            result[key] = input.content
        end
        return result
    end

    local result = {}
    for k, v in pairs(base_args) do
        result[k] = v
    end

    for discriminator, input_data in pairs(inputs) do
        if discriminator ~= "" and discriminator ~= "default" then
            result[discriminator] = input_data.content
        end
    end

    return result
end

local function safe_inputs(n)
    local ok, inputs_or_err, inputs_err = pcall(function()
        return n:inputs()
    end)

    if not ok then
        return nil, inputs_or_err
    end

    if inputs_err then
        return nil, inputs_err
    end

    return inputs_or_err, nil
end

local function build_child_output_result(output_data)
    if output_data and #output_data > 0 then
        for _, output in ipairs(output_data) do
            if output.discriminator == "error" then
                local error_content = output.content
                return nil, {
                    code = "CHILD_WORKFLOW_FAILED",
                    message = type(error_content) == "string" and error_content or "Child workflow failed",
                    status = "Child workflow failed"
                }
            end
        end

        if #output_data == 1 then
            return {
                _dataflow_ref = output_data[1].data_id
            }, nil
        else
            local ref_array = {}
            for _, output in ipairs(output_data) do
                table.insert(ref_array, {
                    _dataflow_ref = output.data_id
                })
            end
            return ref_array, nil
        end
    end

    return nil, nil
end

local function collect_child_result(n, child_node_ids, yield_result)
    local output_data, collect_err = func._deps.child_output.collect_outputs(n, child_node_ids, yield_result)
    if collect_err then
        if type(collect_err) == "table" then
            return nil, collect_err
        end
        return nil, prefixed_error("Failed to collect child outputs: ", collect_err, "unknown")
    end

    return build_child_output_result(output_data)
end

local function fail_child_collect(n, collect_err)
    if type(collect_err) == "table" then
        return n:fail(collect_err, error_message(collect_err, "Child workflow failed"))
    end

    return n:fail({
        code = ERROR_FUNCTION_EXECUTION_FAILED,
        message = error_message(collect_err, "Child workflow failed")
    }, error_message(collect_err, "Child workflow failed"))
end

local function complete_from_children(n, child_node_ids, fallback_result)
    local final_output, collect_err = func._deps.child_output.resume_children(n, child_node_ids, collect_child_result)
    if collect_err then
        return fail_child_collect(n, collect_err)
    end
    if final_output ~= nil then
        return (n:complete(final_output, "Function executed successfully"))
    end

    return (n:complete(fallback_result, "Function executed successfully"))
end

-- Checkpoints child ids before yielding so recovery resumes the same children
-- instead of re-running the function and creating duplicate children.
local function finish_with_children(n, created_node_ids, fallback_result)
    n:update_metadata({ func_pending = { child_node_ids = created_node_ids } })

    local yield_result, yield_err = n:yield({ run_nodes = created_node_ids })
    if yield_err then
        if type(yield_err) == "table" then
            return n:fail(yield_err, "Control command execution failed")
        end
        return n:fail({
            code = ERROR_FUNCTION_EXECUTION_FAILED,
            message = prefixed_error("Control command execution failed: ", yield_err, "unknown")
        }, "Control command execution failed")
    end

    local final_output, collect_err = collect_child_result(n, created_node_ids, yield_result)
    if collect_err then
        return fail_child_collect(n, collect_err)
    end
    if final_output ~= nil then
        return (n:complete(final_output, "Function executed successfully"))
    end

    return (n:complete(fallback_result, "Function executed successfully"))
end

local function run(args)
    local n, err = func._deps.node.new(args) :: any
    if err then
        error(err)
    end

    local config = n:config()

    local pending = (n:metadata() or {}).func_pending
    if type(pending) == "table" and type(pending.child_node_ids) == "table" and #pending.child_node_ids > 0 then
        return complete_from_children(n, pending.child_node_ids, {})
    end
    local func_id = config.func_id
    if not func_id or func_id == "" then
        return n:fail({
            code = ERROR_MISSING_FUNC_ID,
            message = "Function ID not specified in node configuration"
        }, "Missing func_id in node config")
    end

    local base_args = config.args

    local inputs, inputs_err = safe_inputs(n)
    if inputs_err then
        if type(inputs_err) == "table" then
            return n:fail(inputs_err, error_message(inputs_err, "Failed to load inputs"))
        end
        return n:fail({
            code = "INPUT_VALIDATION_FAILED",
            message = inputs_err
        }, inputs_err)
    end

    local input_data = merge_inputs_with_base_args(inputs, base_args)

    if input_data == nil then
        return n:fail({
            code = ERROR_NO_INPUT_DATA,
            message = "No input data provided for function node"
        }, "Function node requires input data")
    end

    local executor = func._deps.funcs.new() :: any
    local base_context = config.context
    local execution_context = build_execution_context_with_dataflow(
        base_context,
        n.dataflow_id,
        n.node_id,
        n.path
    )
    executor = executor:with_context(execution_context)

    local command = executor:async(func_id, input_data)
    local response_channel = command:response()
    local events_channel = process.events()

    local result = channel.select({
        response_channel:case_receive(),
        events_channel:case_receive()
    })

    if result.channel == events_channel then
        local event = result.value
        if event.kind == process.event.CANCEL then
            command:cancel()
            return n:fail({
                code = ERROR_FUNCTION_CANCELED,
                message = "Function execution was canceled by system event"
            }, "Function execution was canceled")
        end
    end

    if command:is_canceled() then
        return n:fail({
            code = ERROR_FUNCTION_CANCELED,
            message = "Function execution was canceled"
        }, "Function execution was canceled")
    end

    local payload, result_err = command:result()
    if result_err then
        return n:fail({
            code = ERROR_FUNCTION_EXECUTION_FAILED,
            message = result_err
        }, "Function execution failed: " .. result_err)
    end

    local function_result = payload:data()

    if type(function_result) == "table" and function_result._control then
        local control = function_result._control
        local created_node_ids = {}

        if control.commands and type(control.commands) == "table" then
            for _, cmd in ipairs(control.commands) do
                if cmd.type and cmd.payload then
                    if cmd.type == func._deps.consts.COMMAND_TYPES.CREATE_NODE then
                        if not cmd.payload.parent_node_id then
                            cmd.payload.parent_node_id = n.node_id
                        end

                        if cmd.payload.node_id then
                            table.insert(created_node_ids, cmd.payload.node_id)
                        end
                    end

                    n:command(cmd)
                end
            end
        end

        local cleaned_result = {}
        for k, v in pairs(function_result) do
            if k ~= "_control" then
                cleaned_result[k] = v
            end
        end

        if #created_node_ids > 0 then
            return finish_with_children(n, created_node_ids, cleaned_result)
        end

        return (n:complete(cleaned_result, "Function executed successfully"))
    end

    return (n:complete(function_result, "Function executed successfully"))
end

func.run = run
return func

