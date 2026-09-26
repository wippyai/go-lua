local prompt_lib = require("prompt")
local data_reader = require("data_reader")
local json = require("json")
local agent_consts = require("agent_consts")

local prompt_builder = {}
local mt = { __index = prompt_builder }

local function row_id(row)
    if type(row) ~= "table" then
        return ""
    end
    return tostring(row.data_id or "")
end

local function sort_history_rows(history_rows)
    table.sort(history_rows, function(a, b)
        return row_id(a) < row_id(b)
    end)
    return history_rows
end

local function find_latest_checkpoint_marker(history_rows)
    for i = #history_rows, 1, -1 do
        local item = history_rows[i]
        if item.type == agent_consts.DATA_TYPE.AGENT_MEMORY
           and item.metadata
           and item.metadata.checkpoint_marker == true then
            return item
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

local function apply_latest_marker(history_rows)
    if not history_rows or #history_rows == 0 then
        return history_rows or {}, nil
    end

    sort_history_rows(history_rows)

    local latest_marker = find_latest_checkpoint_marker(history_rows)
    if not latest_marker then
        return history_rows, nil
    end

    local cut_before = tostring((latest_marker.metadata or {}).checkpoint_before_data_id or "")
    local filtered = { latest_marker }
    for _, item in ipairs(history_rows) do
        if item ~= latest_marker then
            local keep = row_id(item) > cut_before or is_structured_result_row(item)
            if keep then
                filtered[#filtered + 1] = item
            end
        end
    end

    return filtered, latest_marker
end

local function clone_table(tbl)
    if type(tbl) ~= "table" then
        return nil
    end

    local copy = {}
    for key, value in pairs(tbl) do
        copy[key] = value
    end
    return copy
end

local function merge_provider_metadata(base_provider_metadata, override_provider_metadata)
    if type(base_provider_metadata) ~= "table" then
        return clone_table(override_provider_metadata)
    end

    if type(override_provider_metadata) ~= "table" then
        return clone_table(base_provider_metadata)
    end

    local merged_provider_metadata = clone_table(base_provider_metadata)
    for key, value in pairs(override_provider_metadata) do
        merged_provider_metadata[key] = value
    end

    return merged_provider_metadata
end

local function extract_message_meta(metadata, provider_metadata_override)
    if type(metadata) ~= "table" then
        if type(provider_metadata_override) == "table" then
            return {
                provider_metadata = clone_table(provider_metadata_override)
            }
        end
        return nil
    end

    local llm_meta = metadata.llm_meta
    local message_meta = nil
    if type(llm_meta) == "table" then
        message_meta = clone_table(llm_meta)
    end

    local resolved_provider_metadata = nil
    if type(message_meta) == "table" and type(message_meta.provider_metadata) == "table" then
        resolved_provider_metadata = clone_table(message_meta.provider_metadata)
    elseif type(metadata.provider_metadata) == "table" then
        resolved_provider_metadata = clone_table(metadata.provider_metadata)
    end

    if type(provider_metadata_override) == "table" then
        resolved_provider_metadata = merge_provider_metadata(
            resolved_provider_metadata,
            provider_metadata_override
        )
    end

    if type(message_meta) == "table" then
        if type(resolved_provider_metadata) == "table" then
            message_meta.provider_metadata = resolved_provider_metadata
        end
        return message_meta
    end

    if type(resolved_provider_metadata) == "table" then
        return {
            provider_metadata = resolved_provider_metadata
        }
    end

    return nil
end

function prompt_builder.new(dataflow_id, node_id, node_path)
    if not dataflow_id then
        return nil, "dataflow_id is required"
    end
    if not node_id then
        return nil, "node_id is required"
    end
    if not node_path then
        return nil, "node_path is required"
    end

    local self = setmetatable({}, mt)
    self.dataflow_id = dataflow_id
    self.node_id = node_id
    self.node_path = node_path
    self._arena_config = nil
    self._initial_input = nil
    self._pending_history = {}
    self._history_override = nil

    return self, nil
end

function prompt_builder:with_arena_config(arena_config)
    self._arena_config = arena_config
    return self
end

function prompt_builder:with_initial_input(initial_input)
    self._initial_input = initial_input
    return self
end

function prompt_builder:with_pending_history(pending_history)
    self._pending_history = pending_history or {}
    return self
end

function prompt_builder:with_history(history)
    self._history_override = history or {}
    return self
end

function prompt_builder:_parse_json_content(content, content_type)
    if content_type == "application/json" and type(content) == "string" then
        local parsed, err = json.decode(content)
        if err then
            return content, nil
        end
        return parsed, nil
    end
    return content, nil
end

function prompt_builder:_load_conversation_history()
    if self._history_override then
        local history = {}
        local function append_rows(rows)
            for _, row in ipairs(rows or {}) do
                history[#history + 1] = row
            end
        end
        append_rows(self._history_override)
        append_rows(self._pending_history)
        sort_history_rows(history)
        return history, nil
    end

    local reader = data_reader.with_dataflow(self.dataflow_id)
        :fetch_options({ replace_references = true })
        :with_nodes(self.node_id)
        :with_data_types(
            agent_consts.DATA_TYPE.AGENT_ACTION,
            agent_consts.DATA_TYPE.AGENT_OBSERVATION,
            agent_consts.DATA_TYPE.AGENT_MEMORY,
            agent_consts.DATA_TYPE.AGENT_DELEGATION
        )

    local history_items, err = reader:all()
    if err then
        return nil, "Failed to load conversation history: " .. err
    end

    local merged_history = {}
    local seen_data_ids = {}

    local function append_rows(rows)
        for _, row in ipairs(rows or {}) do
            local row_data_id = tostring(row.data_id or "")
            if row_data_id == "" or not seen_data_ids[row_data_id] then
                merged_history[#merged_history + 1] = row
                if row_data_id ~= "" then
                    seen_data_ids[row_data_id] = true
                end
            end
        end
    end

    append_rows(history_items)
    append_rows(self._pending_history)

    if #merged_history == 0 then
        return merged_history, nil
    end

    -- sort by UUID v7 data_id; matches recovery ordering and is monotonic
    -- regardless of created_at resolution ties across backends.
    sort_history_rows(merged_history)

    -- find the latest checkpoint marker (if any). checkpoint writes an
    -- AGENT_MEMORY row with metadata.checkpoint_marker = true and records
    -- the history cut-line as metadata.checkpoint_before_data_id. rows at or
    -- before the cut-line are dropped, except structured function results
    -- which stay visible so the next turn keeps tool-call continuity.
    local filtered = apply_latest_marker(merged_history)
    return filtered, nil
end

function prompt_builder:_format_action(action_item, builder)
    local content, err = self:_parse_json_content(action_item.content, action_item.content_type)
    if err then
        return "Failed to parse action content: " .. err
    end

    local metadata = action_item.metadata or {}
    local text_content = ""
    local tool_calls = {}
    local delegate_calls = {}

    if type(content) == "table" then
        text_content = content.result or ""
        tool_calls = content.tool_calls or {}
        delegate_calls = content.delegate_calls or {}
    else
        text_content = content or ""
    end

    local has_tool_calls = #tool_calls > 0
    local has_delegate_calls = #delegate_calls > 0
    local has_text_content = text_content and text_content ~= ""

    -- Always add assistant message if there are tool calls, delegate calls, or text content
    -- This ensures tool calls have a proper assistant message to attach to
    if has_tool_calls or has_delegate_calls or has_text_content then
        local message_meta = extract_message_meta(metadata)
        builder:add_assistant(text_content or "", message_meta)
    end

    -- Process regular tool calls
    if has_tool_calls then
        for _, tool_call in ipairs(tool_calls) do
            local call_id = tool_call.id
            if not call_id then
                return "Tool call missing ID in action"
            end
            local call_options = extract_message_meta(metadata, tool_call.provider_metadata)
            builder:add_function_call(tool_call.name, tool_call.arguments, call_id, call_options)
        end
    end

    -- Process delegate calls
    if has_delegate_calls then
        for _, delegate_call in ipairs(delegate_calls) do
            local call_id = delegate_call.id
            if not call_id then
                return "Delegate call missing ID in action"
            end
            local call_options = extract_message_meta(metadata, delegate_call.provider_metadata)
            builder:add_function_call(delegate_call.name, delegate_call.arguments, call_id, call_options)
        end
    end

    return nil
end

function prompt_builder:_format_observation(obs_item, builder)
    local content, err = self:_parse_json_content(obs_item.content, obs_item.content_type)
    if err then
        return "Failed to parse observation content: " .. err
    end

    local metadata = obs_item.metadata or {}
    local tool_call_id = metadata.tool_call_id
    local tool_name = metadata.tool_name

    if tool_call_id and tool_name then
        local result_content
        if metadata.is_error then
            result_content = "Error: " .. tostring(content)
        elseif content == nil then
            result_content = "nil"
        elseif type(content) == "table" then
            local json_str, json_err = json.encode(content)
            if json_err then
                result_content = "[Failed to encode JSON result]"
            else
                result_content = json_str
            end
        else
            result_content = tostring(content)
        end

        local result_options = extract_message_meta(metadata)
        builder:add_function_result(tool_name, result_content, tool_call_id, result_options)
    else
        local feedback_content = tostring(content)
        if feedback_content and feedback_content ~= "" then
            local message_meta = extract_message_meta(metadata)
            builder:add_developer(feedback_content, message_meta)
        end
    end

    return nil
end

function prompt_builder:_format_memory(memory_item, builder)
    local content = memory_item.content
    if not content or content == "" then
        return nil
    end

    local metadata = memory_item.metadata or {}
    local message_meta = extract_message_meta(metadata)
    builder:add_developer(content, message_meta)
    return nil
end

function prompt_builder:_format_delegation(delegation_item, builder)
    local content = delegation_item.content
    if not content then
        return nil
    end

    local metadata = delegation_item.metadata or {}
    local tool_call_id = metadata.tool_call_id
    local tool_name = metadata.tool_name

    if tool_call_id and tool_name then
        local result_content
        if type(content) == "table" then
            local json_str, json_err = json.encode(content)
            if json_err then
                result_content = "[Failed to encode delegation result]"
            else
                result_content = json_str
            end
        else
            result_content = tostring(content)
        end

        local result_options = extract_message_meta(metadata)
        builder:add_function_result(tool_name, result_content, tool_call_id, result_options)
    else
        local delegation_text = "Delegation result: " .. tostring(content)
        local message_meta = extract_message_meta(metadata)
        builder:add_developer(delegation_text, message_meta)
    end

    return nil
end

function prompt_builder:build_prompt(system_prompt, initial_input)
    local input_content = initial_input or self._initial_input

    local history_items, err = self:_load_conversation_history()
    if err then
        return nil, err
    end

    local builder = prompt_lib.new()

    if type(system_prompt) == "string" then
        builder:add_system(system_prompt)
        builder:add_cache_marker("system_complete")
    end

    if self._arena_config then
        local tool_calling = self._arena_config.tool_calling

        if tool_calling == "none" then
            builder:add_system("Respond with text only, do not call any tools.")
        elseif tool_calling == "auto" then
            builder:add_system(
            "Use appropriate tools when needed to advance the task. You may respond with text only if no tools are required.")
        elseif tool_calling == "any" then
            builder:add_system(
            "You must use tools to complete tasks. Use the finish tool when you have completed the task.")
        end
    end

    if input_content then
        local input_text
        if type(input_content) == "table" then
            local json_str, json_err = json.encode(input_content)
            if json_err then
                input_text = "[Complex input data]"
            else
                input_text = json_str
            end
        else
            input_text = tostring(input_content)
        end
        builder:add_user(input_text)
    end
    builder:add_cache_marker("user_complete")

    for i, item in ipairs(history_items) do
        local process_err = nil

        if item.type == agent_consts.DATA_TYPE.AGENT_ACTION then
            process_err = self:_format_action(item, builder)
        elseif item.type == agent_consts.DATA_TYPE.AGENT_OBSERVATION then
            process_err = self:_format_observation(item, builder)
        elseif item.type == agent_consts.DATA_TYPE.AGENT_MEMORY then
            process_err = self:_format_memory(item, builder)
        elseif item.type == agent_consts.DATA_TYPE.AGENT_DELEGATION then
            process_err = self:_format_delegation(item, builder)
        end

        if process_err then
            return nil, process_err
        end
    end

    return builder, nil
end

return prompt_builder
