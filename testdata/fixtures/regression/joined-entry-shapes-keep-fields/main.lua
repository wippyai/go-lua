local parallel = {}

type FuncExecutor = {
    with_context: (self: FuncExecutor, context: {[string]: any}) -> FuncExecutor,
    call: (self: FuncExecutor, func_id: string, data: any) -> (any, string?),
}

type ExtractorFn = (parallel_result: any) -> any

parallel._deps = {
    node = require("node"),
    template_graph = require("template_graph"),
    iterator = require("iterator"),
    funcs = require("funcs"),
    json = require("json"),
    uuid = require("uuid"),
    consts = require("consts"),
    node_reader = require("node_reader")
}

parallel.DEFAULTS = {
    BATCH_SIZE = 1,
    ITERATION_INPUT_KEY = "default",
    FAILURE_STRATEGY = "collect_errors",
    ON_ERROR = "continue",
    FILTER = "all",
    UNWRAP = false
}

parallel.ON_ERROR_STRATEGIES = {
    FAIL_FAST = "fail_fast",
    CONTINUE = "continue"
}

parallel.FAILURE_STRATEGIES = {
    FAIL_FAST = parallel.ON_ERROR_STRATEGIES.FAIL_FAST,
    COLLECT_ERRORS = "collect_errors"
}

parallel.FILTER_STRATEGIES = {
    ALL = "all",
    SUCCESSES = "successes",
    FAILURES = "failures"
}

parallel.EXTRACTORS = {
    SUCCESSES = "successes",
    FAILURES = "failures"
}

parallel.ERRORS = {
    MISSING_SOURCE_ARRAY_KEY = "MISSING_SOURCE_ARRAY_KEY",
    NO_INPUT_DATA = "NO_INPUT_DATA",
    NO_TEMPLATES = "NO_TEMPLATES",
    INVALID_INPUT_STRUCTURE = "INVALID_INPUT_STRUCTURE",
    ITERATION_FAILED = "ITERATION_FAILED",
    TEMPLATE_DISCOVERY_FAILED = "TEMPLATE_DISCOVERY_FAILED",
    INVALID_ON_ERROR_STRATEGY = "INVALID_ON_ERROR_STRATEGY",
    INVALID_FILTER_STRATEGY = "INVALID_FILTER_STRATEGY",
    INVALID_BATCH_SIZE = "INVALID_BATCH_SIZE",
    INVALID_PIPELINE_STEP = "INVALID_PIPELINE_STEP",
    INVALID_EXTRACTOR = "INVALID_EXTRACTOR"
}

local function create_array(size)
    return table.create(math.floor(size or 0), 0)
end

parallel.extractors = ({
    [parallel.EXTRACTORS.SUCCESSES] = function(parallel_result)
        local successes = parallel_result.successes or {}
        local results = create_array(#successes)
        for index, entry in ipairs(successes) do
            results[index] = entry.result
        end
        return results
    end,
    [parallel.EXTRACTORS.FAILURES] = function(parallel_result)
        local input_failures = parallel_result.failures or {}
        local failures = create_array(#input_failures)
        for index, entry in ipairs(input_failures) do
            failures[index] = entry
        end
        return failures
    end
}) :: {[string]: ExtractorFn}

local function normalize_on_error_strategy(strategy)
    if strategy == parallel.FAILURE_STRATEGIES.COLLECT_ERRORS then
        return parallel.ON_ERROR_STRATEGIES.CONTINUE
    end

    return strategy
end

local function validate_on_error_strategy(strategy)
    strategy = normalize_on_error_strategy(strategy)
    return strategy == parallel.ON_ERROR_STRATEGIES.FAIL_FAST or
        strategy == parallel.ON_ERROR_STRATEGIES.CONTINUE
end

local function validate_filter_strategy(strategy)
    return strategy == parallel.FILTER_STRATEGIES.ALL or
        strategy == parallel.FILTER_STRATEGIES.SUCCESSES or
        strategy == parallel.FILTER_STRATEGIES.FAILURES
end

local function validate_batch_size(size)
    return type(size) == "number" and size > 0 and size <= 1000
end

local function merge_context(base, additions)
    local merged = {}

    if type(base) == "table" then
        for k, v in pairs(base) do
            merged[k] = v
        end
    end

    if type(additions) == "table" then
        for k, v in pairs(additions) do
            merged[k] = v
        end
    end

    return merged
end

local function count_array_entries(values)
    if type(values) ~= "table" then
        return 0
    end

    local count = 0
    for _ in ipairs(values) do
        count = count + 1
    end
    return count
end

local function call_func(func_id: string, data: any, context: {[string]: any}?)
    local executor = parallel._deps.funcs.new() :: FuncExecutor
    if context ~= nil then
        executor = executor:with_context(context)
    end

    return executor:call(func_id, data)
end

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

local function normalize_input_payload(inputs)
    if not inputs or next(inputs) == nil then
        return nil
    end

    if inputs.default and inputs.default.content ~= nil then
        return inputs.default.content
    end

    if inputs[""] and inputs[""].content ~= nil then
        return inputs[""].content
    end

    local count = 0
    local first_input = nil
    for _, input in pairs(inputs) do
        count = count + 1
        if not first_input then
            first_input = input
        end
    end

    if count == 1 and first_input then
        return first_input.content
    end

    local mapped = {}
    for key, input in pairs(inputs) do
        mapped[key] = input.content
    end

    return mapped
end

function parallel.validate_item_pipeline_step(step)
    if type(step) ~= "table" then
        return false, "item step must be a table"
    end

    if step.context ~= nil and type(step.context) ~= "table" then
        return false, "context must be a table"
    end

    if step.type ~= "map" and step.type ~= "filter" then
        return false, "item step type must be 'map' or 'filter'"
    end

    if type(step.func_id) ~= "string" or step.func_id == "" then
        return false, "item step func_id is required"
    end

    return true, nil
end

function parallel.validate_reduction_pipeline_step(step, _shape)
    if type(step) ~= "table" then
        return false, "reduction step must be a table"
    end

    if step.context ~= nil and type(step.context) ~= "table" then
        return false, "context must be a table"
    end

    if step.type == "map" or step.type == "filter" or step.type == "aggregate" then
        if type(step.func_id) ~= "string" or step.func_id == "" then
            return false, "reduction step func_id is required"
        end
        return true, nil
    end

    if step.type == "group" then
        if type(step.key_func_id) ~= "string" or step.key_func_id == "" then
            return false, "group step key_func_id is required"
        end
        return true, nil
    end

    return false, "invalid reduction step type"
end

function parallel.validate_reduction_pipeline_flow(extractor_name, steps)
    if extractor_name ~= nil and parallel.extractors[extractor_name] == nil then
        return false, "invalid extractor"
    end

    if steps == nil then
        return true, nil
    end

    if type(steps) ~= "table" then
        return false, "reduction_steps must be a table"
    end

    local shape = "array"
    for _, step in ipairs(steps) do
        local valid, err = parallel.validate_reduction_pipeline_step(step, shape)
        if not valid then
            return false, err
        end

        if step.type == "group" then
            shape = "grouped"
        elseif step.type == "aggregate" then
            shape = "scalar"
        end
    end

    return true, nil
end

function parallel.execute_item_pipeline_step(step, data)
    local valid, err = parallel.validate_item_pipeline_step(step)
    if not valid then
        return nil, err
    end

    local func_id = step.func_id :: string
    local step_context = step.context :: {[string]: any}?

    if step.type == "map" then
        return call_func(func_id, data, step_context)
    end

    local keep, call_err = call_func(func_id, data, step_context)
    if call_err then
        return nil, call_err
    end

    return keep == true, nil
end

function parallel.execute_reduction_pipeline_step(step, data)
    local valid, err = parallel.validate_reduction_pipeline_step(step, "array")
    if not valid then
        return nil, err
    end

    if step.type == "aggregate" then
        local func_id = step.func_id :: string
        local step_context = step.context :: {[string]: any}?
        return call_func(func_id, data, step_context)
    end

    if type(data) ~= "table" then
        return nil, "reduction step requires array input"
    end

    if step.type == "map" then
        local func_id = step.func_id :: string
        local mapped = create_array(#data)
        for index, item in ipairs(data) do
            local ctx = nil
            if step.context ~= nil then
                ctx = merge_context(step.context, {
                    current_item = item,
                    item_index = index
                })
            end

            local mapped_item, map_err = call_func(func_id, item, ctx)
            if map_err then
                return nil, map_err
            end
            mapped[index] = mapped_item
        end
        return mapped, nil
    end

    if step.type == "filter" then
        local func_id = step.func_id :: string
        local filtered = create_array(#data)
        local filtered_count = 0
        for index, item in ipairs(data) do
            local ctx = nil
            if step.context ~= nil then
                ctx = merge_context(step.context, {
                    current_item = item,
                    item_index = index
                })
            end

            local keep, filter_err = call_func(func_id, item, ctx)
            if filter_err then
                return nil, filter_err
            end
            if keep == true then
                filtered_count = filtered_count + 1
                filtered[filtered_count] = item
            end
        end
        return filtered, nil
    end

    local grouped = {}
    local key_func_id = step.key_func_id :: string
    for index, item in ipairs(data) do
        local ctx = nil
        if step.context ~= nil then
            ctx = merge_context(step.context, {
                current_item = item,
                item_index = index
            })
        end

        local key, key_err = call_func(key_func_id, item, ctx)
        if key_err then
            return nil, key_err
        end

        local key_str = tostring(key)
        if grouped[key_str] == nil then
            grouped[key_str] = create_array(1)
        end
        table.insert(grouped[key_str], item)
    end

    return grouped, nil
end

local function gather_run_nodes(iterations)
    local total_run_nodes = 0
    for _, iteration in ipairs(iterations) do
        if type(iteration.uuid_mapping) == "table" and next(iteration.uuid_mapping) ~= nil then
            for _ in pairs(iteration.uuid_mapping) do
                total_run_nodes = total_run_nodes + 1
            end
        elseif type(iteration.root_nodes) == "table" then
            total_run_nodes = total_run_nodes + #iteration.root_nodes
        end
    end

    local run_nodes = create_array(total_run_nodes)
    local run_node_count = 0

    for _, iteration in ipairs(iterations) do
        if type(iteration.uuid_mapping) == "table" and next(iteration.uuid_mapping) ~= nil then
            for _, node_id in pairs(iteration.uuid_mapping) do
                run_node_count = run_node_count + 1
                run_nodes[run_node_count] = node_id
            end
        elseif type(iteration.root_nodes) == "table" then
            for _, node_id in ipairs(iteration.root_nodes) do
                run_node_count = run_node_count + 1
                run_nodes[run_node_count] = node_id
            end
        end
    end

    return run_nodes
end

local function fail_iteration(n, error_value)
    local message = error_message(error_value, "Iteration failed")
    if string.find(message, "Iteration failed", 1, true) == nil then
        message = "Iteration failed: " .. message
    end

    if type(error_value) == "table" then
        local normalized_error = {}
        for key, value in pairs(error_value) do
            normalized_error[key] = value
        end
        normalized_error.message = message
        return n:fail(normalized_error, message)
    end

    return n:fail({
        code = parallel.ERRORS.ITERATION_FAILED,
        message = message
    }, message)
end

local function add_failure(parallel_result, iteration: any, err_message)
    parallel_result.failure_count = parallel_result.failure_count + 1
    parallel_result.failures[parallel_result.failure_count] = {
        iteration = iteration.iteration or iteration.iteration_index,
        item = iteration.input_item,
        error = err_message
    }
end

local function add_success(parallel_result, iteration: any, result)
    parallel_result.success_count = parallel_result.success_count + 1
    parallel_result.successes[parallel_result.success_count] = {
        iteration = iteration.iteration or iteration.iteration_index,
        item = iteration.input_item,
        result = result
    }
end

local function create_parallel_result(items)
    return {
        successes = create_array(#items),
        failures = create_array(#items),
        success_count = 0,
        failure_count = 0,
        total_iterations = #items
    }
end

local build_iteration_record

local function maybe_materialize_passthrough_inputs(n, inputs, passthrough_keys)
    if type(passthrough_keys) ~= "table" or #passthrough_keys == 0 then
        return {}, nil
    end

    local passthrough_inputs = {}

    for _, key in ipairs(passthrough_keys) do
        local input = inputs[key]
        if input then
            local data_id = input.data_id

            if not data_id and input.content ~= nil then
                data_id = parallel._deps.uuid.v7()
                n:data(parallel._deps.consts.DATA_TYPE.NODE_INPUT, input.content, {
                    data_id = data_id,
                    discriminator = key,
                    node_id = n.node_id,
                    content_type = type(input.content) == "table" and
                        parallel._deps.consts.CONTENT_TYPE.JSON or
                        parallel._deps.consts.CONTENT_TYPE.TEXT
                })
            end

            if data_id then
                passthrough_inputs[key] = {
                    node_id = n.node_id,
                    data_id = data_id
                }
            end
        end
    end

    if next(passthrough_inputs) ~= nil and type(n.submit) == "function" then
        local submit_ok, submit_err = n:submit()
        if not submit_ok then
            return nil, submit_err or "failed to submit passthrough inputs"
        end
    end

    return passthrough_inputs, nil
end

local function process_reduction_pipeline(config: any, parallel_result, on_error)
    local reduction_extract = config.reduction_extract
    local reduction_steps = config.reduction_steps

    local current = parallel_result

    if reduction_extract ~= nil then
        local extractor = parallel.extractors[reduction_extract]
        if extractor == nil then
            return nil, "invalid extractor"
        end
        current = extractor(parallel_result)
    end

    if reduction_steps and #reduction_steps > 0 then
        for _, step in ipairs(reduction_steps) do
            local next_value, step_err = parallel.execute_reduction_pipeline_step(step, current)
            if step_err then
                if on_error == parallel.ON_ERROR_STRATEGIES.FAIL_FAST then
                    return nil, "Reduction pipeline failed: " .. step_err
                end
                return parallel_result, nil
            end
            current = next_value
        end
    end

    return current, nil
end

local function has_reduction_pipeline(config)
    return config.reduction_extract ~= nil or (config.reduction_steps and #config.reduction_steps > 0)
end

local function has_legacy_output_directive(config)
    return config.filter ~= nil or config.unwrap ~= nil or config.output_shape ~= nil
end

local function should_apply_output_shape(config, started_from_recovery)
    if has_legacy_output_directive(config) then
        return true
    end

    return not started_from_recovery
end

local function resolve_legacy_output_directives(config)
    local filter = config.filter
    local unwrap = config.unwrap
    local output_shape = config.output_shape

    if type(output_shape) == "table" then
        if filter == nil then
            filter = output_shape.filter
        end
        if unwrap == nil then
            unwrap = output_shape.unwrap
        end
    elseif type(output_shape) == "string" then
        local normalized = string.lower(output_shape)

        if filter == nil then
            if string.find(normalized, "failure", 1, true) ~= nil then
                filter = parallel.EXTRACTORS.FAILURES
            elseif string.find(normalized, "success", 1, true) ~= nil then
                filter = parallel.EXTRACTORS.SUCCESSES
            elseif string.find(normalized, "all", 1, true) ~= nil then
                filter = "all"
            end
        end

        if unwrap == nil then
            if string.find(normalized, "unwrap", 1, true) ~= nil then
                unwrap = true
            elseif normalized == "wrapped" then
                unwrap = false
            end
        end
    end

    if filter == nil then
        filter = "all"
    end

    return filter, unwrap == true
end

local function copy_parallel_entries(entries, entry_count)
    local copied = {}
    if type(entries) ~= "table" then
        return copied
    end

    local max_index = type(entry_count) == "number" and entry_count or #entries
    for index = 1, max_index do
        local entry = entries[index]
        if entry ~= nil then
            copied[#copied + 1] = entry
        end
    end

    return copied
end

local function collect_all_parallel_entries(parallel_result)
    local ordered = {}
    local entries_by_iteration = {}

    for index = 1, (parallel_result.success_count or 0) do
        local entry = parallel_result.successes[index]
        if type(entry) == "table" and type(entry.iteration) == "number" then
            entries_by_iteration[entry.iteration] = entry
        end
    end

    for index = 1, (parallel_result.failure_count or 0) do
        local entry = parallel_result.failures[index]
        if type(entry) == "table" and type(entry.iteration) == "number" then
            entries_by_iteration[entry.iteration] = entry
        end
    end

    for iteration = 1, (parallel_result.total_iterations or 0) do
        local entry = entries_by_iteration[iteration]
        if entry ~= nil then
            ordered[#ordered + 1] = entry
        end
    end

    return ordered
end

local function apply_legacy_output_shape(config, parallel_result)
    local filter, unwrap = resolve_legacy_output_directives(config)
    local shaped_output

    if filter == parallel.EXTRACTORS.SUCCESSES then
        shaped_output = copy_parallel_entries(parallel_result.successes, parallel_result.success_count)
    elseif filter == parallel.EXTRACTORS.FAILURES then
        shaped_output = copy_parallel_entries(parallel_result.failures, parallel_result.failure_count)
    else
        shaped_output = collect_all_parallel_entries(parallel_result)
    end

    if not unwrap then
        return shaped_output
    end

    local unwrapped = {}
    for _, entry in ipairs(shaped_output) do
        if type(entry) == "table" then
            if entry.result ~= nil then
                unwrapped[#unwrapped + 1] = entry.result
            else
                unwrapped[#unwrapped + 1] = entry.error
            end
        else
            unwrapped[#unwrapped + 1] = entry
        end
    end

    return unwrapped
end

local PARALLEL_PROGRESS_CURSOR_KEY = "cursor"
local PARALLEL_PROGRESS_OUTCOME = {
    SUCCESS = "success",
    FAILURE = "failure",
    FILTERED = "filtered"
}

local function iteration_progress_key(iteration_index)
    return string.format("iteration.%06d", iteration_index)
end

local function decode_data_content(row)
    local content = row.content
    if row.content_type == parallel._deps.consts.CONTENT_TYPE.JSON and type(content) == "string" then
        local decoded, decode_err = parallel._deps.json.decode(content)
        if not decode_err then
            return decoded
        end
    end
    return content
end

local function sort_data_rows(rows)
    table.sort(rows, function(a, b)
        local a_created = tostring(a.created_at or "")
        local b_created = tostring(b.created_at or "")

        if a_created == b_created then
            return tostring(a.data_id or "") < tostring(b.data_id or "")
        end

        return a_created < b_created
    end)

    return rows
end

local function extract_iteration_index(row, total_iterations)
    local metadata = row.metadata
    local iteration_index = metadata and tonumber(metadata.iteration)

    if not iteration_index then
        local discriminator = tostring(row.discriminator or "")
        local matched_iteration = string.match(discriminator, "^iteration%.(%d+)")
        if matched_iteration then
            iteration_index = tonumber(matched_iteration)
        end
    end

    if iteration_index and iteration_index >= 1 and iteration_index <= total_iterations then
        return iteration_index
    end

    return nil
end

local function extract_error_value(content)
    if type(content) == "table" and content.error ~= nil then
        return content.error
    end

    return content
end

local function build_completion_from_output_row(row, total_iterations)
    local iteration_index = extract_iteration_index(row, total_iterations)
    if iteration_index == nil then
        return nil
    end

    local content = decode_data_content(row)
    local outcome = row.type == parallel._deps.consts.DATA_TYPE.ITERATION_ERROR and
        PARALLEL_PROGRESS_OUTCOME.FAILURE or
        PARALLEL_PROGRESS_OUTCOME.SUCCESS

    return {
        iteration = iteration_index,
        outcome = outcome,
        attempt_id = row.metadata and row.metadata.attempt_id,
        result = outcome == PARALLEL_PROGRESS_OUTCOME.SUCCESS and content or nil,
        error = outcome == PARALLEL_PROGRESS_OUTCOME.FAILURE and extract_error_value(content) or nil
    }
end

local function normalize_submitted_iterations(submitted_iterations, batch_start, batch_end)
    local normalized = create_array(math.max(batch_end - batch_start + 1, 0))
    local seen = {}
    local normalized_count = 0

    if type(submitted_iterations) == "table" and #submitted_iterations > 0 then
        for _, iteration_index in ipairs(submitted_iterations) do
            if type(iteration_index) == "number" and iteration_index >= batch_start and
               iteration_index <= batch_end and iteration_index == math.floor(iteration_index) and
               not seen[iteration_index] then
                seen[iteration_index] = true
                normalized_count = normalized_count + 1
                normalized[normalized_count] = iteration_index
            end
        end
    end

    if normalized_count == 0 then
        for iteration_index = batch_start, batch_end do
            normalized_count = normalized_count + 1
            normalized[normalized_count] = iteration_index
        end
    else
        table.sort(normalized)
    end

    return normalized
end

local function normalize_string_array(values)
    local normalized = {}
    local seen = {}
    for _, value in ipairs(type(values) == "table" and values or {}) do
        if type(value) == "string" and value ~= "" and not seen[value] then
            seen[value] = true
            table.insert(normalized, value)
        end
    end
    return normalized
end

local function normalize_progress_cursor(content, total_iterations)
    local cursor = {
        next_batch_start = 1,
        active_batch = nil
    }

    if type(content) ~= "table" then
        return cursor
    end

    local next_batch_start = tonumber(content.next_batch_start) or 1
    next_batch_start = math.max(1, math.min(next_batch_start, total_iterations + 1))
    cursor.next_batch_start = next_batch_start

    if type(content.active_batch) == "table" then
        local batch_start = tonumber(content.active_batch.batch_start)
        local batch_end = tonumber(content.active_batch.batch_end)
        local attempt_id = content.active_batch.attempt_id

        if batch_start and batch_end and batch_start >= 1 and batch_start <= batch_end and
           batch_end <= total_iterations and type(attempt_id) == "string" and attempt_id ~= "" then
            cursor.active_batch = {
                batch_start = batch_start,
                batch_end = batch_end,
                attempt_id = attempt_id,
                submitted_iterations = normalize_submitted_iterations(
                    content.active_batch.submitted_iterations,
                    batch_start,
                    batch_end
                ),
                run_nodes = normalize_string_array(content.active_batch.run_nodes)
            }
        end
    end

    return cursor
end

local function normalize_iteration_completion(content, iteration_index)
    if type(content) ~= "table" then
        return nil
    end

    local outcome = content.outcome
    if outcome ~= PARALLEL_PROGRESS_OUTCOME.SUCCESS and
       outcome ~= PARALLEL_PROGRESS_OUTCOME.FAILURE and
       outcome ~= PARALLEL_PROGRESS_OUTCOME.FILTERED then
        return nil
    end

    return {
        iteration = iteration_index,
        outcome = outcome,
        attempt_id = content.attempt_id,
        result = content.result,
        error = content.error
    }
end

local function advance_cursor(cursor: any, completed_iterations: any, total_iterations, next_batch_start)
    cursor.next_batch_start = math.max(1, math.min(next_batch_start, total_iterations + 1))

    while cursor.next_batch_start <= total_iterations and completed_iterations[cursor.next_batch_start] ~= nil do
        cursor.next_batch_start = cursor.next_batch_start + 1
    end
end

local function load_parallel_progress(n, total_iterations)
    local progress: any = {
        cursor = {
            next_batch_start = 1,
            active_batch = nil
        },
        completed_iterations = {}
    }

    -- unit tests pass a mock node without :query; treat as fresh progress
    if type(n.query) ~= "function" then
        return progress
    end

    local ok, rows = pcall(function()
        return (n:query() :: any)
            :with_nodes(n.node_id)
            :with_data_types({
                parallel._deps.consts.DATA_TYPE.PARALLEL_PROGRESS,
                parallel._deps.consts.DATA_TYPE.ITERATION_RESULT,
                parallel._deps.consts.DATA_TYPE.ITERATION_ERROR
            })
            :all()
    end)

    if not ok or not rows then
        return progress
    end

    sort_data_rows(rows)

    for _, row in ipairs(rows) do
        if row.type == parallel._deps.consts.DATA_TYPE.PARALLEL_PROGRESS then
            local key = row.key or ""
            local content = decode_data_content(row)

            if key == PARALLEL_PROGRESS_CURSOR_KEY then
                progress.cursor = normalize_progress_cursor(content, total_iterations)
            else
                local matched_iteration = string.match(key, "^iteration%.(%d+)$")
                if matched_iteration then
                    local iteration_index = tonumber(matched_iteration)
                    if iteration_index and iteration_index >= 1 and iteration_index <= total_iterations then
                        local completion = normalize_iteration_completion(content, iteration_index)
                        if completion ~= nil then
                            progress.completed_iterations[iteration_index] = completion
                        end
                    end
                end
            end
        else
            local completion = build_completion_from_output_row(row, total_iterations)
            if completion ~= nil then
                progress.completed_iterations[completion.iteration] = completion
            end
        end
    end

    if progress.cursor.active_batch and progress.cursor.active_batch.batch_end < progress.cursor.next_batch_start then
        progress.cursor.active_batch = nil
    end

    advance_cursor(progress.cursor, progress.completed_iterations :: any, total_iterations, progress.cursor.next_batch_start)

    return progress
end

local function can_persist_parallel_progress(n)
    return type(n.data) == "function" and type(n.submit) == "function"
end

local function queue_parallel_cursor(n, cursor: any)
    if not can_persist_parallel_progress(n) then
        return true, nil
    end

    local _, data_err = n:data(parallel._deps.consts.DATA_TYPE.PARALLEL_PROGRESS, {
        next_batch_start = cursor.next_batch_start,
        active_batch = cursor.active_batch
    }, {
        node_id = n.node_id,
        key = PARALLEL_PROGRESS_CURSOR_KEY
    })

    if data_err then
        return nil, data_err
    end

    return true, nil
end

local function persist_parallel_cursor(n, cursor: any)
    if not can_persist_parallel_progress(n) then
        return true, nil
    end

    local queued, queue_err = queue_parallel_cursor(n, cursor)
    if not queued then
        return nil, queue_err
    end

    return n:submit()
end

local function persist_iteration_completion(n, completion: any)
    if not can_persist_parallel_progress(n) then
        return true, nil
    end

    local _, data_err = n:data(parallel._deps.consts.DATA_TYPE.PARALLEL_PROGRESS, {
        iteration = completion.iteration,
        outcome = completion.outcome,
        attempt_id = completion.attempt_id,
        result = completion.result,
        error = completion.error
    }, {
        node_id = n.node_id,
        key = iteration_progress_key(completion.iteration)
    })

    if data_err then
        return nil, data_err
    end

    return n:submit()
end

local function apply_iteration_completion(parallel_result, iteration: any, completion: any)
    if completion.outcome == PARALLEL_PROGRESS_OUTCOME.SUCCESS then
        add_success(parallel_result, iteration, completion.result)
    elseif completion.outcome == PARALLEL_PROGRESS_OUTCOME.FAILURE then
        add_failure(parallel_result, iteration, completion.error or "Iteration failed")
    end
end

local function build_iteration_completion(iteration: any, outcome, extras)
    extras = extras or {}

    return {
        iteration = iteration.iteration,
        outcome = outcome,
        attempt_id = extras.attempt_id or iteration.attempt_id,
        result = extras.result,
        error = extras.error
    }
end

local function rebuild_parallel_result(items, completed_iterations)
    local parallel_result = create_parallel_result(items)

    for iteration_index = 1, #items do
        local completion = completed_iterations[iteration_index]
        if completion ~= nil then
            apply_iteration_completion(
                parallel_result,
                build_iteration_record(items, iteration_index, completion.attempt_id),
                completion
            )
        end
    end

    return parallel_result
end

local function has_recovery_progress(progress: any)
    if progress.cursor.active_batch ~= nil or progress.cursor.next_batch_start > 1 then
        return true
    end

    return next(progress.completed_iterations) ~= nil
end

local function process_iteration_output(config: any, failure_strategy, parallel_result, iteration: any, iteration_result)
    local processed_result = iteration_result

    if config.item_steps and #config.item_steps > 0 then
        local filtered_out = false

        for _, step in ipairs(config.item_steps) do
            local step_result, step_err = parallel.execute_item_pipeline_step(step, processed_result)
            if step_err then
                local pipeline_err = prefixed_error("Item pipeline failed: ", step_err, "unknown")
                if failure_strategy == parallel.FAILURE_STRATEGIES.FAIL_FAST then
                    return nil, pipeline_err
                end

                local completion = build_iteration_completion(iteration, PARALLEL_PROGRESS_OUTCOME.FAILURE, {
                    error = pipeline_err
                })
                add_failure(parallel_result, iteration, pipeline_err)
                return completion, nil
            end

            if step.type == "filter" then
                if not step_result then
                    filtered_out = true
                    break
                end
            else
                processed_result = step_result
            end
        end

        if filtered_out then
            return build_iteration_completion(iteration, PARALLEL_PROGRESS_OUTCOME.FILTERED, {}), nil
        end
    end

    local completion = build_iteration_completion(iteration, PARALLEL_PROGRESS_OUTCOME.SUCCESS, {
        result = processed_result
    })
    add_success(parallel_result, iteration, processed_result)
    return completion, nil
end

local function collect_iteration_completion(n, config: any, failure_strategy, parallel_result, iteration: any)
    local iteration_result, collect_err = parallel._deps.iterator.collect_results(n, iteration)
    if collect_err then
        if failure_strategy == parallel.FAILURE_STRATEGIES.FAIL_FAST then
            return nil, prefixed_error("Iteration failed: ", collect_err, "unknown")
        end

        local completion = build_iteration_completion(iteration, PARALLEL_PROGRESS_OUTCOME.FAILURE, {
            error = collect_err
        })
        add_failure(parallel_result, iteration, completion.error)
        return completion, nil
    end

    return process_iteration_output(config, failure_strategy, parallel_result, iteration, iteration_result)
end

local function recover_iteration_completion(n, config: any, failure_strategy, parallel_result, iteration: any)
    local iteration_result, collect_err = parallel._deps.iterator.collect_results(n, iteration)
    if collect_err then
        if string.find(error_message(collect_err, ""), "No output data found for iteration", 1, true) ~= nil then
            return nil, nil
        end

        if failure_strategy == parallel.FAILURE_STRATEGIES.FAIL_FAST then
            return nil, prefixed_error("Iteration failed: ", collect_err, "unknown")
        end

        local completion = build_iteration_completion(iteration, PARALLEL_PROGRESS_OUTCOME.FAILURE, {
            error = collect_err
        })
        add_failure(parallel_result, iteration, completion.error)
        return completion, nil
    end

    return process_iteration_output(config, failure_strategy, parallel_result, iteration, iteration_result)
end

build_iteration_record = function(items, iteration_index: number, attempt_id)
    return {
        iteration = iteration_index,
        input_item = items[iteration_index],
        attempt_id = attempt_id,
        uuid_mapping = {}
    }
end

local function collect_pending_iterations(progress: any)
    local active_batch = progress.cursor.active_batch
    if active_batch == nil then
        return create_array(0)
    end

    local pending_iterations = create_array(count_array_entries(active_batch.submitted_iterations))
    local pending_count = 0

    for _, iteration_index in ipairs(active_batch.submitted_iterations or {}) do
        if progress.completed_iterations[iteration_index] == nil then
            pending_count = pending_count + 1
            pending_iterations[pending_count] = iteration_index
        end
    end

    return pending_iterations
end

local function merge_loaded_completions(parallel_result, items, progress: any, loaded_progress: any, iteration_indices)
    for _, iteration_index in ipairs(iteration_indices) do
        if progress.completed_iterations[iteration_index] == nil then
            local completion = loaded_progress.completed_iterations[iteration_index]
            if completion ~= nil then
                progress.completed_iterations[iteration_index] = completion
                apply_iteration_completion(
                    parallel_result,
                    build_iteration_record(items, iteration_index, completion.attempt_id),
                    completion
                )
            end
        end
    end
end

local function discard_queued_commands(n)
    if type(n._queued_commands) == "table" then
        n._queued_commands = table.create(10, 0)
    end
end

local function recover_active_batch(n, config: any, failure_strategy, parallel_result, items, progress: any)
    local active_batch: any = progress.cursor.active_batch
    if active_batch == nil then
        return {}, nil
    end

    local pending_iterations = create_array(count_array_entries(active_batch.submitted_iterations))
    local pending_count = 0

    for _, iteration_index in ipairs(active_batch.submitted_iterations or {}) do
        if progress.completed_iterations[iteration_index] == nil then
            local iteration = build_iteration_record(items, iteration_index :: number, active_batch.attempt_id)
            local completion, recover_err = recover_iteration_completion(
                n,
                config,
                failure_strategy,
                parallel_result,
                iteration
            )

            if recover_err then
                return nil, recover_err
            end

            if completion ~= nil then
                progress.completed_iterations[iteration_index] = completion

                local submit_ok, submit_err = persist_iteration_completion(n, completion)
                if not submit_ok then
                    return nil, prefixed_error("Failed to persist parallel iteration progress: ", submit_err, "unknown")
                end
            else
                pending_count = pending_count + 1
                pending_iterations[pending_count] = iteration_index
            end
        end
    end

    if pending_count > 0 then
        local run_nodes = normalize_string_array(active_batch.run_nodes)
        if #run_nodes == 0 then
            local reader, reader_err = parallel._deps.node_reader.with_dataflow(n.dataflow_id)
            if not reader then return nil, prefixed_error("Recovery node read failed: ", reader_err, "unknown") end
            local rows, rows_err = (reader :: any):with_parent_nodes(n.node_id):all()
            if rows_err then return nil, prefixed_error("Recovery node read failed: ", rows_err, "unknown") end
            local submitted = {}
            for _, iteration_index in ipairs(active_batch.submitted_iterations or {}) do
                submitted[iteration_index] = true
            end
            for _, row in ipairs(rows or {}) do
                local metadata = type(row.metadata) == "table" and row.metadata or {}
                if submitted[tonumber(metadata.iteration)] and
                    (metadata.attempt_id == nil or metadata.attempt_id == active_batch.attempt_id) and
                    row.status ~= parallel._deps.consts.STATUS.TEMPLATE then
                    table.insert(run_nodes, row.node_id)
                end
            end
        end
        if #run_nodes > 0 then
            local _, yield_err = n:yield({ run_nodes = run_nodes })
            if yield_err then return nil, prefixed_error("Recovery yield failed: ", yield_err, "unknown") end
        end

        local loaded_progress = load_parallel_progress(n, #items)
        merge_loaded_completions(parallel_result, items, progress, loaded_progress, pending_iterations)
        pending_iterations = collect_pending_iterations(progress)

        local still_pending = {}
        for _, iteration_index in ipairs(pending_iterations) do
            local iteration = build_iteration_record(items, iteration_index :: number, active_batch.attempt_id)
            local completion, recover_err = recover_iteration_completion(
                n, config, failure_strategy, parallel_result, iteration)
            if recover_err then return nil, recover_err end
            if completion then
                progress.completed_iterations[iteration_index] = completion
                local submit_ok, submit_err = persist_iteration_completion(n, completion)
                if not submit_ok then
                    return nil, prefixed_error("Failed to persist parallel iteration progress: ", submit_err, "unknown")
                end
            else
                table.insert(still_pending, iteration_index)
            end
        end
        pending_iterations = still_pending
    end

    if #pending_iterations == 0 then
        progress.cursor.active_batch = nil
        advance_cursor(progress.cursor, progress.completed_iterations :: any, #items, (active_batch.batch_end :: number) + 1)

        local submit_ok, submit_err = persist_parallel_cursor(n, progress.cursor)
        if not submit_ok then
            return nil, prefixed_error("Failed to persist parallel cursor: ", submit_err, "unknown")
        end
    end

    return pending_iterations, nil
end

local function process_batch(n, template_graph, items, batch_start, batch_end, iteration_input_key,
                             passthrough_inputs, config: any, failure_strategy, parallel_result, progress: any, selected_iterations)
    selected_iterations = normalize_submitted_iterations(selected_iterations, batch_start, batch_end)

    if #selected_iterations == 0 then
        return nil
    end

    local attempt_id = parallel._deps.uuid.v7()

    progress.cursor.active_batch = {
        batch_start = batch_start,
        batch_end = batch_end,
        attempt_id = attempt_id,
        submitted_iterations = selected_iterations
    }
    progress.cursor.next_batch_start = batch_start

    local iterations, create_err = parallel._deps.iterator.create_batch(
        n,
        template_graph,
        items,
        batch_start,
        batch_end,
        iteration_input_key,
        passthrough_inputs,
        {
            attempt_id = attempt_id,
            iteration_indices = selected_iterations
        }
    )
    iterations = iterations :: {any}

    if create_err then
        discard_queued_commands(n)

        if failure_strategy == parallel.FAILURE_STRATEGIES.FAIL_FAST then
            return prefixed_error("Iteration creation failed: ", create_err, "unknown")
        end

        for _, iteration_index in ipairs(selected_iterations) do
            local completion = build_iteration_completion(
                build_iteration_record(items, iteration_index, attempt_id),
                PARALLEL_PROGRESS_OUTCOME.FAILURE,
                { error = prefixed_error("Iteration creation failed: ", create_err, "unknown") }
            )

            progress.completed_iterations[iteration_index] = completion
            apply_iteration_completion(parallel_result, build_iteration_record(items, iteration_index, attempt_id), completion)

            local submit_ok, submit_err = persist_iteration_completion(n, completion)
            if not submit_ok then
                return prefixed_error("Failed to persist parallel iteration progress: ", submit_err, "unknown")
            end
        end

        progress.cursor.active_batch = nil
        advance_cursor(progress.cursor, progress.completed_iterations :: any, #items, batch_end + 1)

        local submit_ok, submit_err = persist_parallel_cursor(n, progress.cursor)
        if not submit_ok then
            return prefixed_error("Failed to persist parallel cursor: ", submit_err, "unknown")
        end

        return nil
    end

    local run_nodes = gather_run_nodes(iterations)
    progress.cursor.active_batch.run_nodes = run_nodes
    local cursor_ok, cursor_err = queue_parallel_cursor(n, progress.cursor)
    if not cursor_ok then
        discard_queued_commands(n)
        return prefixed_error("Failed to persist parallel cursor: ", cursor_err, "unknown")
    end

    if #run_nodes > 0 then
        local _, yield_err = n:yield({ run_nodes = run_nodes })
        if yield_err then
            discard_queued_commands(n)

            if failure_strategy == parallel.FAILURE_STRATEGIES.FAIL_FAST then
                return prefixed_error("Yield failed: ", yield_err, "unknown")
            end

            for _, iteration in ipairs(iterations) do
                local completion = build_iteration_completion(iteration :: any, PARALLEL_PROGRESS_OUTCOME.FAILURE, {
                    error = prefixed_error("Yield failed: ", yield_err, "unknown")
                })

                progress.completed_iterations[iteration.iteration] = completion
                add_failure(parallel_result, iteration :: any, completion.error)

                local submit_ok, submit_err = persist_iteration_completion(n, completion)
                if not submit_ok then
                    return prefixed_error("Failed to persist parallel iteration progress: ", submit_err, "unknown")
                end
            end

            progress.cursor.active_batch = nil
            advance_cursor(progress.cursor, progress.completed_iterations :: any, #items, batch_end + 1)

            local submit_ok, submit_err = persist_parallel_cursor(n, progress.cursor)
            if not submit_ok then
                return prefixed_error("Failed to persist parallel cursor: ", submit_err, "unknown")
            end

            return nil
        end
    else
        local submit_ok, submit_err = n:submit()
        if not submit_ok then
            discard_queued_commands(n)
            return prefixed_error("Failed to submit parallel batch: ", submit_err, "unknown")
        end
    end

    for _, iteration in ipairs(iterations) do
        local completion, iteration_err = collect_iteration_completion(
            n,
            config,
            failure_strategy,
            parallel_result,
            iteration :: any
        )

        if iteration_err then
            return iteration_err
        end

        progress.completed_iterations[iteration.iteration] = completion

        local submit_ok, submit_err = persist_iteration_completion(n, completion)
        if not submit_ok then
            return prefixed_error("Failed to persist parallel iteration progress: ", submit_err, "unknown")
        end
    end

    progress.cursor.active_batch = nil
    advance_cursor(progress.cursor, progress.completed_iterations :: any, #items, batch_end + 1)

    local submit_ok, submit_err = persist_parallel_cursor(n, progress.cursor)
    if not submit_ok then
        return prefixed_error("Failed to persist parallel cursor: ", submit_err, "unknown")
    end

    return nil
end

local function run(args)
    local n, err = parallel._deps.node.new(args)
    if err then
        error(err)
    end

    local config: any = n:config() or {}
    if config.item_steps == nil and config.item_pipeline ~= nil then
        config.item_steps = config.item_pipeline
    end

    local source_array_key = config.source_array_key
    if type(source_array_key) ~= "string" or source_array_key == "" then
        return n:fail({
            code = parallel.ERRORS.MISSING_SOURCE_ARRAY_KEY,
            message = "source_array_key is required in parallel configuration"
        }, "Missing source_array_key in config")
    end

    local raw_on_error = config.failure_strategy or config.on_error or parallel.DEFAULTS.FAILURE_STRATEGY
    local failure_strategy = normalize_on_error_strategy(raw_on_error)
    if not validate_on_error_strategy(failure_strategy) then
        return n:fail({
            code = parallel.ERRORS.INVALID_ON_ERROR_STRATEGY,
            message = "Invalid on_error/failure_strategy: " .. error_message(raw_on_error, "invalid value")
        }, "Invalid on_error strategy")
    end

    if has_legacy_output_directive(config) then
        local filter = resolve_legacy_output_directives(config)
        if not validate_filter_strategy(filter) then
            return n:fail({
                code = parallel.ERRORS.INVALID_FILTER_STRATEGY,
                message = "Invalid filter: " .. error_message(filter, "invalid value")
            }, "Invalid filter strategy")
        end
    end

    local batch_size = config.batch_size or parallel.DEFAULTS.BATCH_SIZE
    if not validate_batch_size(batch_size) then
        return n:fail({
            code = parallel.ERRORS.INVALID_BATCH_SIZE,
            message = "batch_size must be a positive number <= 1000"
        }, "Invalid batch size")
    end

    if config.item_steps ~= nil then
        if type(config.item_steps) ~= "table" then
            return n:fail({
                code = parallel.ERRORS.INVALID_PIPELINE_STEP,
                message = "item_steps must be a table"
            }, "Invalid item pipeline")
        end

        for _, step in ipairs(config.item_steps) do
            local valid, step_err = parallel.validate_item_pipeline_step(step)
            if not valid then
                return n:fail({
                    code = parallel.ERRORS.INVALID_PIPELINE_STEP,
                    message = step_err
                }, "Invalid item pipeline")
            end
        end
    end

    if config.reduction_extract ~= nil and parallel.extractors[config.reduction_extract] == nil then
        return n:fail({
            code = parallel.ERRORS.INVALID_EXTRACTOR,
            message = "Invalid reduction extractor: " .. error_message(config.reduction_extract, "invalid value")
        }, "Invalid reduction extractor")
    end

    local reduction_valid, reduction_err = parallel.validate_reduction_pipeline_flow(
        config.reduction_extract,
        config.reduction_steps
    )
    if not reduction_valid then
        local code = parallel.ERRORS.INVALID_PIPELINE_STEP
        if string.find(error_message(reduction_err, ""), "extractor", 1, true) then
            code = parallel.ERRORS.INVALID_EXTRACTOR
        end

        return n:fail({
            code = code,
            message = reduction_err
        }, "Invalid reduction pipeline")
    end

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

    local input_payload = normalize_input_payload(inputs)
    if input_payload == nil then
        return n:fail({
            code = parallel.ERRORS.NO_INPUT_DATA,
            message = "No input data provided for parallel node"
        }, "Parallel node requires input data")
    end

    if type(input_payload) ~= "table" then
        return n:fail({
            code = parallel.ERRORS.INVALID_INPUT_STRUCTURE,
            message = "Input data must be an object with source array"
        }, "Invalid input structure for parallel")
    end

    local items = input_payload[source_array_key]
    if items == nil then
        return n:fail({
            code = parallel.ERRORS.INVALID_INPUT_STRUCTURE,
            message = "Input data must contain '" .. source_array_key .. "' field with array"
        }, "Invalid input structure for parallel")
    end

    if type(items) ~= "table" or #items == 0 then
        return n:fail({
            code = parallel.ERRORS.INVALID_INPUT_STRUCTURE,
            message = "Field '" .. source_array_key .. "' must be a non-empty array"
        }, "Invalid input structure for parallel")
    end

    local passthrough_keys = create_array(type(config.passthrough_keys) == "table" and #config.passthrough_keys or 0)
    local passthrough_key_count = 0
    if type(config.passthrough_keys) == "table" then
        for _, key in ipairs(config.passthrough_keys) do
            if type(key) == "string" then
                passthrough_key_count = passthrough_key_count + 1
                passthrough_keys[passthrough_key_count] = key
            end
        end
    end

    local passthrough_inputs, passthrough_err = maybe_materialize_passthrough_inputs(
        n,
        inputs,
        passthrough_keys
    )
    if passthrough_err then
        return fail_iteration(n, prefixed_error("Failed to materialize passthrough inputs: ", passthrough_err, "unknown"))
    end

    local template_graph, template_err = parallel._deps.template_graph.build_for_node(n)
    if template_err then
        return n:fail({
            code = parallel.ERRORS.TEMPLATE_DISCOVERY_FAILED,
            message = template_err
        }, "Failed to discover template nodes")
    end

    if template_graph:is_empty() then
        return n:fail({
            code = parallel.ERRORS.NO_TEMPLATES,
            message = "No template nodes found. Parallel requires child nodes with status='template'"
        }, "No template nodes found")
    end

    local progress: any = load_parallel_progress(n, #items)
    local started_from_recovery = has_recovery_progress(progress)

    local parallel_result = rebuild_parallel_result(items, progress.completed_iterations)

    local iteration_input_key = config.iteration_input_key or parallel.DEFAULTS.ITERATION_INPUT_KEY

    local pending_active_iterations, recover_err = recover_active_batch(
        n,
        config,
        failure_strategy,
        parallel_result,
        items,
        progress
    )
    if recover_err then
        return fail_iteration(n, recover_err)
    end

    if progress.cursor.active_batch ~= nil and #pending_active_iterations > 0 then
        local active_batch = progress.cursor.active_batch
        local batch_err = process_batch(
            n,
            template_graph,
            items,
            active_batch.batch_start,
            active_batch.batch_end,
            iteration_input_key,
            passthrough_inputs,
            config,
            failure_strategy,
            parallel_result,
            progress,
            pending_active_iterations
        )

        if batch_err then
            return fail_iteration(n, batch_err)
        end
    end

    while progress.cursor.next_batch_start <= #items do
        local batch_start = progress.cursor.next_batch_start :: number
        local batch_end = math.min(batch_start + batch_size - 1, #items)

        local batch_err = process_batch(
            n,
            template_graph,
            items,
            batch_start,
            batch_end,
            iteration_input_key,
            passthrough_inputs,
            config,
            failure_strategy,
            parallel_result,
            progress,
            nil
        )

        if batch_err then
            return fail_iteration(n, batch_err)
        end
    end

    if started_from_recovery and type(n.query) == "function" then
        parallel_result = rebuild_parallel_result(items, load_parallel_progress(n, #items).completed_iterations)
    end

    local final_output = parallel_result
    if has_reduction_pipeline(config) then
        local reduced_output, reduction_err = process_reduction_pipeline(config, parallel_result, failure_strategy)
        if reduction_err then
            return fail_iteration(n, reduction_err)
        end
        final_output = reduced_output
    elseif should_apply_output_shape(config, started_from_recovery) then
        final_output = apply_legacy_output_shape(config, parallel_result)
    end

    return (n:complete(final_output, "Parallel processing completed successfully"))
end

parallel.run = run
return parallel

