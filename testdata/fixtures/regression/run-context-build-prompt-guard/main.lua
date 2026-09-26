local json = require("json")
local data_reader = require("data_reader")
local node_reader = require("node_reader")
local prompt_builder = require("prompt_builder")
local agent_consts = require("agent_consts")
local consts = require("consts")

local M = {}

type Selector = {
    mode: string?,
    last: any?,
    from_id: string?,
    to_id: string?,
    checkpoint_id: string?,
    max_chars: any?,
}

local HISTORY_TYPES = {
    agent_consts.DATA_TYPE.AGENT_ACTION,
    agent_consts.DATA_TYPE.AGENT_OBSERVATION,
    agent_consts.DATA_TYPE.AGENT_MEMORY,
    agent_consts.DATA_TYPE.AGENT_DELEGATION
}

local function host_ids(args)
    local host = type(args) == "table" and args.host or nil
    local dataflow_id = host and host.dataflow_id or (type(args) == "table" and args.dataflow_id)
    local node_id = host and host.node_id or (type(args) == "table" and args.node_id)

    if type(dataflow_id) ~= "string" or dataflow_id == "" then
        return nil, nil, "host.dataflow_id is required"
    end
    if type(node_id) ~= "string" or node_id == "" then
        return nil, nil, "host.node_id is required"
    end

    return dataflow_id, node_id, nil
end

local function row_id(row)
    if type(row) ~= "table" then
        return ""
    end
    return tostring(row.data_id or "")
end

local function sort_rows(rows)
    table.sort(rows, function(a, b)
        return row_id(a) < row_id(b)
    end)
    return rows
end

local function find_latest_checkpoint_marker(rows)
    for i = #rows, 1, -1 do
        local row = rows[i]
        if row.type == agent_consts.DATA_TYPE.AGENT_MEMORY
           and type(row.metadata) == "table"
           and row.metadata.checkpoint_marker == true then
            return row
        end
    end
    return nil
end

local function is_structured_result_row(row)
    if type(row) ~= "table" then
        return false
    end

    local metadata = row.metadata or {}
    if type(metadata.tool_call_id) ~= "string" or metadata.tool_call_id == "" then
        return false
    end
    if type(metadata.tool_name) ~= "string" or metadata.tool_name == "" then
        return false
    end

    return row.type == agent_consts.DATA_TYPE.AGENT_OBSERVATION
        or row.type == agent_consts.DATA_TYPE.AGENT_DELEGATION
end

local function apply_latest_marker(rows)
    if #rows == 0 then
        return rows
    end

    sort_rows(rows)
    local latest_marker = find_latest_checkpoint_marker(rows)
    if not latest_marker then
        return rows
    end

    local cut_before = tostring((latest_marker.metadata or {}).checkpoint_before_data_id or "")
    local filtered = { latest_marker }
    for _, row in ipairs(rows) do
        if row ~= latest_marker then
            local keep = row_id(row) > cut_before or is_structured_result_row(row)
            if keep then
                filtered[#filtered + 1] = row
            end
        end
    end

    return sort_rows(filtered)
end

local function selector(args): Selector
    local raw = type(args) == "table" and args.selector or nil
    if type(raw) ~= "table" then
        return { mode = "since_checkpoint" } :: Selector
    end
    return raw :: Selector
end

local function copy_rows(rows)
    local copied = {}
    for _, row in ipairs(rows or {}) do
        copied[#copied + 1] = row
    end
    return copied
end

local function load_history_rows(dataflow_id, node_id)
    local ok, rows_or_err = pcall(function()
        return data_reader.with_dataflow(dataflow_id)
            :fetch_options({ replace_references = true })
            :with_nodes(node_id)
            :with_data_types(HISTORY_TYPES)
            :all()
    end)
    if not ok then
        return nil, tostring(rows_or_err)
    end
    return sort_rows(rows_or_err or {}), nil
end

local function slice_rows(rows, sel: Selector)
    local mode = sel.mode or "since_checkpoint"
    local source = copy_rows(rows)

    if mode == "since_checkpoint" then
        return apply_latest_marker(source)
    end

    if mode == "checkpoint" then
        if type(sel.checkpoint_id) ~= "string" or sel.checkpoint_id == "" then
            return nil, "selector.checkpoint_id is required for checkpoint"
        end
        mode = "since_id"
        sel = {
            mode = "since_id",
            from_id = sel.checkpoint_id,
            max_chars = sel.max_chars
        } :: Selector
    end

    if mode == "window" then
        local count: number? = 20
        if sel.last ~= nil then
            count = tonumber(sel.last)
        end
        if not count or count <= 0 then
            return nil, "selector.last must be positive for window"
        end
        local count_value = count :: number
        local first = math.max(1, #source - count_value + 1)
        local out = {}
        for i = first, #source do
            out[#out + 1] = source[i]
        end
        return out
    end

    if mode == "since_id" then
        if type(sel.from_id) ~= "string" or sel.from_id == "" then
            return nil, "selector.from_id is required for since_id"
        end
        local out = {}
        for _, row in ipairs(source) do
            if row_id(row) > sel.from_id then
                out[#out + 1] = row
            end
        end
        return out
    end

    if mode == "range" then
        local out = {}
        for _, row in ipairs(source) do
            local id = row_id(row)
            local after_from = type(sel.from_id) ~= "string" or sel.from_id == "" or id > sel.from_id
            if after_from then
                out[#out + 1] = row
            end
            if type(sel.to_id) == "string" and sel.to_id ~= "" and id == sel.to_id then
                break
            end
        end
        return out
    end

    if mode == "all" then
        return source
    end

    return nil, "unknown selector mode: " .. tostring(mode)
end

local function decode_content(row)
    local content = row.content
    if type(content) == "string"
       and (row.content_type == consts.CONTENT_TYPE.JSON or row.content_type == "application/json") then
        local decoded, err = json.decode(content)
        if not err then
            return decoded
        end
    end
    return content
end

local function normalize_event(row)
    return {
        id = row.data_id,
        role = row.type,
        content = decode_content(row),
        raw_content = row.content,
        content_type = row.content_type,
        key = row.key,
        discriminator = row.discriminator,
        metadata = type(row.metadata) == "table" and row.metadata or {},
        created_at = row.created_at
    }
end

local function content_length(event)
    local content = event.raw_content or event.content or ""
    if type(content) ~= "string" then
        local encoded = json.encode(content)
        content = encoded or tostring(content)
    end
    return #content
end

local function clamp_rows(rows, events, max_chars)
    local limit = tonumber(max_chars)
    if not limit or limit <= 0 then
        return rows, events, false
    end

    local kept_rows = {}
    local kept_events = {}
    local total = 0
    for i, event in ipairs(events) do
        local next_total = total + content_length(event)
        if next_total > limit then
            return kept_rows, kept_events, true
        end
        total = next_total
        kept_rows[#kept_rows + 1] = rows[i]
        kept_events[#kept_events + 1] = event
    end
    return kept_rows, kept_events, false
end

local function history_rows(args)
    local dataflow_id, node_id, id_err = host_ids(args)
    if id_err then
        return nil, nil, nil, id_err
    end

    local all_rows, load_err = load_history_rows(dataflow_id, node_id)
    if load_err then
        return nil, nil, nil, load_err
    end

    local sel = selector(args)
    local rows, slice_err = slice_rows(all_rows, sel)
    if slice_err then
        return nil, nil, nil, slice_err
    end

    local events = {}
    for _, row in ipairs(rows or {}) do
        events[#events + 1] = normalize_event(row)
    end

    local truncated
    rows, events, truncated = clamp_rows(rows or {}, events, sel.max_chars)
    return rows, events, sel, nil, truncated
end

local function node_row(dataflow_id, node_id)
    local reader, reader_err = node_reader.with_dataflow(dataflow_id)
    if not reader then
        return nil, reader_err
    end
    local row, row_err = reader:with_nodes(node_id):one()
    if row_err then
        return nil, row_err
    end
    return row, nil
end

local function merge_into(target, source)
    if type(source) ~= "table" then
        return target
    end
    for key, value in pairs(source) do
        if type(key) == "string" then
            target[key] = value
        end
    end
    return target
end

function M.get_context(args)
    local dataflow_id, node_id, id_err = host_ids(args)
    if id_err then
        return nil, id_err
    end

    local row, row_err = node_row(dataflow_id, node_id)
    if row_err then
        return nil, row_err
    end
    if not row then
        return nil, "node not found"
    end

    local config = row.config or {}
    local metadata = row.metadata or {}
    local context = {
        dataflow_id = dataflow_id,
        node_id = node_id
    }
    merge_into(context, ((config.arena or {}).context))
    merge_into(context, metadata.session_context)

    return {
        context = context,
        host = type(args) == "table" and args.host or nil,
        agent = type(args) == "table" and args.agent or nil,
        config = config,
        metadata = metadata
    }, nil
end

function M.get_history(args)
    local rows, events, sel, err, truncated = history_rows(args)
    if err then
        return nil, err
    end
    rows = rows or {}
    events = events or {}

    local first = events[1]
    local last = events[#events]
    return {
        events = events,
        range = {
            from_id = first and first.id or nil,
            to_id = last and last.id or nil,
            checkpoint_id = sel and sel.checkpoint_id or nil
        },
        truncated = truncated == true,
        count = #rows
    }, nil
end

local function prompt_to_text(messages)
    local parts = {}
    for _, msg in ipairs(messages or {}) do
        local role = tostring(msg.role or "message")
        local text = ""
        if type(msg.content) == "table" then
            for _, item in ipairs(msg.content) do
                if type(item) == "table" and item.text then
                    text = text .. tostring(item.text)
                elseif type(item) == "string" then
                    text = text .. item
                end
            end
        elseif msg.content ~= nil then
            text = tostring(msg.content)
        end
        parts[#parts + 1] = string.format("[%s] %s", role, text)
    end
    return table.concat(parts, "\n")
end

function M.get_prompt(args)
    local dataflow_id, node_id, id_err = host_ids(args)
    if id_err then
        return nil, id_err
    end

    local rows, _, sel, hist_err, truncated = history_rows(args)
    if hist_err then
        return nil, hist_err
    end

    local row, row_err = node_row(dataflow_id, node_id)
    if row_err then
        return nil, row_err
    end
    if not row then
        return nil, "node not found"
    end

    local config = row.config or {}
    local arena = config.arena or {}
    local builder, builder_err = prompt_builder.new(dataflow_id, node_id, { node_id })
    if not builder then
        return nil, builder_err or "failed to create prompt builder"
    end

    builder:with_arena_config(arena):with_history(rows or {})
    local built, build_err = builder:build_prompt(args and args.prompt or arena.prompt, args and args.initial_input)
    if build_err then
        return nil, build_err
    end

    local messages = built:get_messages()
    local format = args and args.format or "messages"
    local first = rows and rows[1] or nil
    local last = rows and rows[#rows] or nil
    local result = {
        range = {
            from_id = first and first.data_id or nil,
            to_id = last and last.data_id or nil,
            checkpoint_id = sel and sel.checkpoint_id or nil
        },
        truncated = truncated == true
    }
    if format == "text" or format == "both" then
        result.text = prompt_to_text(messages)
    end
    if format ~= "text" then
        result.messages = messages
    end

    return result, nil
end

return M
