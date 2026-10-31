local json = require("json")

local child_output = {}

child_output._deps = {
    consts = require("consts")
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

function child_output.query_node_outputs(n, node_ids)
    if type(node_ids) ~= "table" or #node_ids == 0 then
        return {}, nil
    end

    local ok, output_data_or_err = pcall(function()
        return (n:query() :: any)
            :with_nodes(node_ids)
            :with_data_types(child_output._deps.consts.DATA_TYPE.NODE_OUTPUT)
            :fetch_options({ replace_references = true })
            :all()
    end)

    if not ok then
        return nil, prefixed_error("Failed to query output data: ", output_data_or_err, "unknown")
    end

    return output_data_or_err, nil
end

local function decode_content(content: any)
    if type(content) ~= "string" then
        return content
    end

    local decoded, decode_err = json.decode(content)
    if not decode_err then
        return decoded
    end
    return content
end

local function child_failure_reason(content)
    if content.error ~= nil then
        return content.error
    end
    return content.message or "Child workflow failed"
end

function child_output.outputs_from_yield_results(n, yield_results)
    if type(yield_results) ~= "table" or next(yield_results) == nil then
        return nil, nil
    end

    local result_ids = {}
    local expected_result_ids = {}
    for _, result_id in pairs(yield_results) do
        if type(result_id) == "string" and result_id ~= "" then
            table.insert(result_ids, result_id)
            expected_result_ids[result_id] = true
        end
    end
    if #result_ids == 0 then
        return nil, nil
    end

    local ok, result_rows_or_err = pcall(function()
        return (n:query() :: any)
            :with_data(result_ids)
            :with_data_types(child_output._deps.consts.DATA_TYPE.NODE_RESULT)
            :fetch_options({ replace_references = true })
            :all()
    end)
    if not ok then
        return nil, prefixed_error("Failed to query child result data: ", result_rows_or_err, "unknown")
    end

    local output_ids = {}
    local fallback_node_ids = {}
    local seen_output_ids = {}
    local seen_fallback_node_ids = {}
    local seen_result_ids = {}
    for _, row in ipairs(result_rows_or_err or {}) do
        if row.data_id then
            seen_result_ids[row.data_id] = true
        end
        local content = decode_content(row.content)
        if type(content) == "table" then
            if content.success == false then
                return nil, child_failure_reason(content)
            end

            if content.data_ids ~= nil and type(content.data_ids) ~= "table" then
                return nil, "Child result data_ids must be an array"
            end

            for _, data_id in ipairs(content.data_ids or {}) do
                if type(data_id) == "string" and data_id ~= "" and not seen_output_ids[data_id] then
                    table.insert(output_ids, data_id)
                    seen_output_ids[data_id] = true
                end
            end
        elseif row.node_id and not seen_fallback_node_ids[row.node_id] then
            table.insert(fallback_node_ids, row.node_id)
            seen_fallback_node_ids[row.node_id] = true
        end
    end

    for _, result_id in ipairs(result_ids) do
        if not seen_result_ids[result_id] and expected_result_ids[result_id] then
            return nil, "Child result data not found: " .. result_id
        end
    end

    if #output_ids == 0 then
        return child_output.query_node_outputs(n, fallback_node_ids)
    end

    local output_ok, output_rows_or_err = pcall(function()
        return (n:query() :: any)
            :with_data(output_ids)
            :with_data_types(child_output._deps.consts.DATA_TYPE.NODE_OUTPUT)
            :fetch_options({ replace_references = true })
            :all()
    end)
    if not output_ok then
        return nil, prefixed_error("Failed to query child output data: ", output_rows_or_err, "unknown")
    end

    return output_rows_or_err, nil
end

function child_output.collect_outputs(n, node_ids, yield_results)
    local has_yield_results = type(yield_results) == "table" and next(yield_results) ~= nil
    local output_data, collect_err = child_output.outputs_from_yield_results(n, yield_results)
    if collect_err then
        return nil, collect_err
    end

    if has_yield_results then
        return output_data or {}, nil
    end

    if output_data and #output_data > 0 then
        return output_data, nil
    end

    return child_output.query_node_outputs(n, node_ids)
end

function child_output.resume_children(n, child_node_ids, collect_children_result)
    if type(child_node_ids) ~= "table" or #child_node_ids == 0 then
        return nil, nil
    end

    local existing, existing_err = collect_children_result(n, child_node_ids)
    if existing_err then
        return nil, existing_err
    end
    if existing ~= nil then
        return existing, nil
    end

    local yield_result, yield_err = n:yield({ run_nodes = child_node_ids })
    if yield_err then
        return nil, prefixed_error("Failed to resume children: ", yield_err, "unknown")
    end

    return collect_children_result(n, child_node_ids, yield_result)
end

return child_output

