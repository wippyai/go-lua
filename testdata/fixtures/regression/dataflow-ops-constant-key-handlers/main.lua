local sql = require("sql")
local time = require("time")
local uuid = require("uuid")
local json = require("json")
local consts = require("dataflow_consts")
local activation_repo = require("activation_repo")
local encoding = require("encoding")

-- Use shared constants from consts
local constants = {
    COMMAND_TYPES = consts.COMMAND_TYPES,
    META_KEYS = consts.META_KEYS,
    STATUS = consts.STATUS,
}

-- Module to export
local ops = {}

-- Export constants for external use
ops.COMMAND_TYPES = constants.COMMAND_TYPES
ops.META_KEYS = constants.META_KEYS
ops.STATUS = constants.STATUS

local function is_unique_violation(err)
    local message = string.lower(tostring(err or ""))
    return string.find(message, "unique", 1, true) ~= nil or
        string.find(message, "duplicate key", 1, true) ~= nil
end

local function is_workflow_output_unique_slot(payload)
    return payload and payload.data_type == consts.DATA_TYPE.WORKFLOW_OUTPUT
end

local function is_success_result_unique_slot(payload)
    return payload and payload.data_type == consts.DATA_TYPE.NODE_RESULT and
        payload.discriminator == "result.success" and payload.node_id ~= nil
end

local function is_iteration_terminal_unique_slot(payload)
    return payload and
        (payload.data_type == consts.DATA_TYPE.ITERATION_RESULT or
        payload.data_type == consts.DATA_TYPE.ITERATION_ERROR) and
        payload.node_id ~= nil and type(payload.discriminator) == "string" and
        payload.discriminator ~= "" and type(payload.key) == "string" and
        payload.key ~= "" and type(payload.metadata) == "table" and
        payload.metadata.terminal_emission_key_version == 1
end

local function is_rolling_yield_result(payload)
    return payload and payload.data_type == consts.DATA_TYPE.NODE_YIELD_RESULT and
        type(payload.data_id) == "string" and payload.data_id ~= "" and
        tostring(payload.data_id) == tostring(payload.key)
end

local function is_idempotent_explicit_replay(payload)
    return payload and payload.data_id ~= nil and
        (payload.data_type == consts.DATA_TYPE.NODE_YIELD or
        is_rolling_yield_result(payload) or
        payload.data_type == consts.DATA_TYPE.PARALLEL_PROGRESS)
end

local function find_existing_unique_data_row(tx, dataflow_id, payload)
    if not is_workflow_output_unique_slot(payload) and not is_success_result_unique_slot(payload) and
       not is_iteration_terminal_unique_slot(payload) then
        return nil, nil
    end

    local query = sql.builder.select("data_id")
        :from("dataflow_data")
        :where("dataflow_id = ?", dataflow_id)

    if not is_iteration_terminal_unique_slot(payload) then
        query = query:where("type = ?", payload.data_type)
    end

    if is_workflow_output_unique_slot(payload) then
        query = query
            :where(sql.builder.expr("COALESCE(key, '') = COALESCE(?, '')", payload.key))
            :where(sql.builder.expr("COALESCE(discriminator, '') = COALESCE(?, '')", payload.discriminator))
    elseif is_success_result_unique_slot(payload) then
        query = query
            :where("node_id = ?", payload.node_id)
            :where("discriminator = ?", payload.discriminator)
    else
        local function find_iteration_type(data_type)
            local typed_query = sql.builder.select("data_id")
                :from("dataflow_data")
                :where("dataflow_id = ?", dataflow_id)
                :where("node_id = ?", payload.node_id)
                :where("discriminator = ?", payload.discriminator)
                :where(sql.builder.expr("COALESCE(key, '') = ?", payload.key))
                :where("type = ?", data_type)

            if tx:db_type() == "postgres" then
                typed_query = typed_query:where(sql.builder.expr(
                    "metadata->>'terminal_emission_key_version' = '1'"
                ))
            else
                typed_query = typed_query
                    :where(sql.builder.expr("json_valid(metadata)"))
                    :where(sql.builder.expr(
                        "json_extract(metadata, '$.terminal_emission_key_version') = 1"
                    ))
            end

            local typed_rows, typed_err = typed_query
                :order_by("created_at DESC")
                :limit(1)
                :run_with(tx)
                :query()
            if typed_err then
                return nil, "Failed to query existing iteration terminal row: " .. typed_err
            end
            return typed_rows and typed_rows[1] or nil, nil
        end

        local result_row, result_err = find_iteration_type(consts.DATA_TYPE.ITERATION_RESULT)
        if result_err then return nil, result_err end
        if result_row then return result_row, nil end
        return find_iteration_type(consts.DATA_TYPE.ITERATION_ERROR)
    end

    query = query:order_by("created_at DESC"):limit(1)

    local rows, err = query:run_with(tx):query()
    if err then
        return nil, "Failed to query existing unique data row: " .. err
    end

    if not rows or #rows == 0 then
        return nil, nil
    end

    return rows[1], nil
end

local function replace_iteration_terminal(tx, dataflow_id, op_id, payload, data_id,
                                          content_value, content_type, metadata)
    local result, err = sql.builder.update("dataflow_data")
        :where("dataflow_id = ?", dataflow_id)
        :where("data_id = ?", data_id)
        :set("type", payload.data_type)
        :set("discriminator", payload.discriminator)
        :set("key", payload.key)
        :set("content", encoding.ensure_storable(content_value, content_type))
        :set("content_type", content_type)
        :set("metadata", encoding.ensure_utf8(metadata))
        :run_with(tx)
        :exec()
    if err then return nil, "Failed to replace iteration terminal data: " .. err end
    return {
        data_id = data_id,
        changes_made = result.rows_affected > 0,
        op_id = op_id,
        deduplicated = true,
        replaced = true,
    }, nil
end

local function savepoint_name()
    return "sp_" .. string.gsub(uuid.v7(), "-", "")
end

local function create_savepoint(tx, name)
    local _, err = tx:execute("SAVEPOINT " .. name)
    if err then
        return nil, "Failed to create savepoint: " .. err
    end
    return true, nil
end

local function rollback_savepoint(tx, name)
    local _, err = tx:execute("ROLLBACK TO SAVEPOINT " .. name)
    if err then
        return nil, "Failed to rollback savepoint: " .. err
    end
    return true, nil
end

local function release_savepoint(tx, name)
    local _, err = tx:execute("RELEASE SAVEPOINT " .. name)
    if err then
        return nil, "Failed to release savepoint: " .. err
    end
    return true, nil
end

local function nullable_equal(left, right)
    if left == nil and right == nil then return true end
    if left == nil or right == nil then return false end
    return tostring(left) == tostring(right)
end

local function find_data_by_id(tx, data_id)
    local rows, err = sql.builder.select(
            "dataflow_id", "node_id", "type", "discriminator", "key",
            "content", "content_type", "metadata"
        )
        :from("dataflow_data")
        :where("data_id = ?", data_id)
        :limit(1)
        :run_with(tx)
        :query()
    if err then return nil, "Failed to query existing data ID: " .. err end
    return rows and rows[1] or nil, nil
end

local function deep_equal(left, right)
    if type(left) ~= type(right) then return false end
    if type(left) ~= "table" then return left == right end
    for key, value in pairs(left) do
        if not deep_equal(value, right[key]) then return false end
    end
    for key in pairs(right) do
        if left[key] == nil then return false end
    end
    return true
end

local function json_equal(left, right)
    if left == nil or right == nil then return nullable_equal(left, right) end
    local decoded_left, left_err = json.decode(tostring(left))
    local decoded_right, right_err = json.decode(tostring(right))
    if left_err or right_err then return nullable_equal(left, right) end
    return deep_equal(decoded_left, decoded_right)
end

local function matches_explicit_data_replay(row, payload, content, content_type, metadata)
    return nullable_equal(row.node_id, payload.node_id) and
        nullable_equal(row.type, payload.data_type) and
        nullable_equal(row.discriminator, payload.discriminator) and
        nullable_equal(row.key, payload.key) and
        json_equal(row.content, content) and
        nullable_equal(row.content_type, content_type) and
        json_equal(row.metadata, metadata)
end

local function resolve_idempotent_explicit_replay(tx, dataflow_id, op_id, data_id,
                                                  payload, content, content_type, metadata)
    local existing, existing_err = find_data_by_id(tx, data_id)
    if existing_err then return nil, existing_err end
    -- A concurrent consumer may remove an ephemeral row after winning the
    -- insert. The collision still proves this create was already observed;
    -- treating it as consumed avoids resurrecting a stale yield.
    if existing == nil then
        return {
            data_id = data_id,
            changes_made = false,
            op_id = op_id,
            deduplicated = true,
            consumed = true,
        }
    end
    if tostring(existing.dataflow_id) ~= tostring(dataflow_id) then
        return nil, "Data ID belongs to another workflow"
    end
    if not matches_explicit_data_replay(
            existing, payload, content, content_type, metadata) then
        return nil, "Data ID already exists with conflicting payload"
    end
    return {
        data_id = data_id,
        changes_made = false,
        op_id = op_id,
        deduplicated = true,
    }
end

-- ============================================================================
-- PRIVATE HANDLERS - IMPLEMENTATION DETAILS
-- ============================================================================

-- Define handlers for command types (private to this module)
local handlers = {}

-- Node Operations
handlers[constants.COMMAND_TYPES.CREATE_NODE] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}

    if not payload.node_type then
        return nil, "Node type is required"
    end

    local node_id = payload.node_id or uuid.v7()
    local parent_node_id = payload.parent_node_id or sql.as.null()

    local metadata = payload.metadata or "{}"
    if type(metadata) == "table" then
        local encoded, err_encode = json.encode(metadata)
        if err_encode then
            return nil, "Failed to encode metadata: " .. err_encode
        end
        metadata = encoded
    end
    metadata = encoding.ensure_utf8(metadata)

    local config = payload.config or "{}"
    if type(config) == "table" then
        local encoded, err_encode = json.encode(config)
        if err_encode then
            return nil, "Failed to encode config: " .. err_encode
        end
        config = encoded
    end

    local status = payload.status or constants.STATUS.PENDING
    local now_ts = time.now():format(time.RFC3339NANO)

    local insert_query = sql.builder.insert("dataflow_nodes")
        :set_map({
            node_id = node_id,
            dataflow_id = dataflow_id,
            parent_node_id = parent_node_id,
            type = payload.node_type,
            status = status,
            config = encoding.ensure_utf8(config),
            metadata = metadata,
            created_at = now_ts,
            updated_at = now_ts
        })

    local executor = insert_query:run_with(tx)
    local result, err = executor:exec()

    if err then
        return nil, "Failed to create node: " .. err
    end

    return {
        node_id = node_id,
        changes_made = true,
        op_id = op_id
    }
end

handlers[constants.COMMAND_TYPES.UPDATE_NODE] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}

    if not payload.node_id then
        return nil, "Node ID is required"
    end

    -- Metadata merge configuration - default is merge=true for consistency with UPDATE_WORKFLOW
    local merge_metadata = payload.merge_metadata
    if merge_metadata == nil then
        merge_metadata = true -- Default to merge
    end

    local update_query = sql.builder.update("dataflow_nodes")
        :where("node_id = ?", payload.node_id)
        :where("dataflow_id = ?", dataflow_id)

    local has_update = false

    if payload.node_type then
        update_query = update_query:set("type", payload.node_type)
        has_update = true
    end

    if payload.status then
        update_query = update_query:set("status", payload.status)
        has_update = true
    end

    if payload.config then
        local config = payload.config
        if type(config) == "table" then
            local encoded, err_encode = json.encode(config)
            if err_encode then
                return nil, "Failed to encode config: " .. err_encode
            end
            config = encoded
        end
        update_query = update_query:set("config", encoding.ensure_utf8(config))
        has_update = true
    end

    if payload.metadata ~= nil then
        local meta_val_for_db

        if merge_metadata and payload.metadata then
            -- Read existing metadata first for merging
            local existing_query = sql.builder.select("metadata")
                :from("dataflow_nodes")
                :where("node_id = ?", payload.node_id)
                :where("dataflow_id = ?", dataflow_id)

            local existing_executor = existing_query:run_with(tx)
            local existing_result, existing_err = existing_executor:query()

            if existing_err then
                return nil, "Failed to read existing metadata for merge: " .. existing_err
            end

            -- Parse existing metadata
            local existing_metadata = {}
            if #existing_result > 0 and existing_result[1].metadata then
                local existing_meta_str = existing_result[1].metadata :: string
                if existing_meta_str and existing_meta_str ~= "" and existing_meta_str ~= "{}" then
                    local decoded, decode_err = json.decode(existing_meta_str)
                    if not decode_err and type(decoded) == "table" then
                        existing_metadata = decoded
                    end
                end
            end

            -- Parse new metadata
            local new_metadata = payload.metadata
            if type(new_metadata) == "string" then
                local decoded, decode_err = json.decode(new_metadata)
                if not decode_err and type(decoded) == "table" then
                    new_metadata = decoded
                elseif decode_err then
                    return nil, "Failed to decode new metadata JSON: " .. decode_err
                end
            end

            -- Merge metadata: existing + new (new overwrites existing keys)
            local merged_metadata = {}

            -- Copy existing metadata
            if type(existing_metadata) == "table" then
                for k, v in pairs(existing_metadata) do
                    merged_metadata[k] = v
                end
            end

            -- Overlay new metadata
            if type(new_metadata) == "table" then
                for k, v in pairs(new_metadata) do
                    merged_metadata[k] = v
                end
            end

            -- Encode merged result
            local encoded, err_json = json.encode(merged_metadata)
            if err_json then
                return nil, "Failed to encode merged metadata: " .. err_json
            end
            meta_val_for_db = encoded

        else
            -- Replacement mode (original behavior)
            if payload.metadata == nil then
                meta_val_for_db = sql.as.null()
            elseif type(payload.metadata) == "table" then
                local encoded, err_json = json.encode(payload.metadata)
                if err_json then
                    return nil, "Failed to encode metadata for update: " .. err_json
                end
                meta_val_for_db = encoded
            elseif type(payload.metadata) == "string" then
                meta_val_for_db = payload.metadata
            else
                return nil, "Invalid metadata type for update: must be a table, JSON string, or nil (for SQL NULL)"
            end
        end

        update_query = update_query:set("metadata", encoding.ensure_utf8(meta_val_for_db))
        has_update = true
    end

    if not has_update then
        return {
            node_id = payload.node_id,
            changes_made = false,
            op_id = op_id,
            message = "No fields provided for update"
        }
    end

    local now_ts = time.now():format(time.RFC3339NANO)
    update_query = update_query:set("updated_at", now_ts)

    local executor = update_query:run_with(tx)
    local result, err = executor:exec()

    if err then
        return nil, "Failed to update node: " .. err
    end

    return {
        node_id = payload.node_id,
        changes_made = result.rows_affected > 0,
        op_id = op_id,
        rows_affected = result.rows_affected,
        metadata_merged = merge_metadata
    }
end

handlers[constants.COMMAND_TYPES.DELETE_NODE] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}

    if not payload.node_id then
        return nil, "Node ID is required"
    end

    local delete_query = sql.builder.delete("dataflow_nodes")
        :where("node_id = ?", payload.node_id)
        :where("dataflow_id = ?", dataflow_id)

    local executor = delete_query:run_with(tx)
    local result, err = executor:exec()

    if err then
        return nil, "Failed to delete node: " .. err
    end

    return {
        node_id = payload.node_id,
        changes_made = result.rows_affected > 0,
        op_id = op_id,
        rows_affected = result.rows_affected
    }
end

-- Data Operations
handlers[constants.COMMAND_TYPES.CREATE_DATA] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}

    if not payload.data_type then
        return nil, "Data type is required"
    end

    if not payload.content then
        return nil, "Data content is required"
    end

    local data_id = payload.data_id or uuid.v7()
    local raw_content = payload.content
    local content_value = raw_content
    local wake_index_changed = false

    if type(content_value) == "table" then
        local encoded, err_encode = json.encode(content_value)
        if err_encode then
            return nil, "Failed to encode content: " .. err_encode
        end
        content_value = encoded
    end
    local content_type = payload.content_type or "application/json"
    -- Storage boundary: external content can carry arbitrary bytes; every
    -- textual write is valid UTF-8, declared binary passes byte-identical.
    content_value = encoding.ensure_storable(content_value, content_type)
    local node_id = payload.node_id or sql.as.null()
    local metadata = payload.metadata or "{}"

    if type(metadata) == "table" then
        local encoded, err_encode = json.encode(metadata)
        if err_encode then
            return nil, "Failed to encode metadata: " .. err_encode
        end
        metadata = encoded
    end
    metadata = encoding.ensure_utf8(metadata)

    if is_iteration_terminal_unique_slot(payload) then
        local existing_row, existing_err = find_existing_unique_data_row(tx, dataflow_id, payload)
        if existing_err then return nil, existing_err end
        if existing_row and existing_row.data_id then
            return replace_iteration_terminal(
                tx, dataflow_id, op_id, payload, existing_row.data_id,
                content_value, content_type, metadata
            )
        end
    end

    local now_ts = time.now():format(time.RFC3339NANO)

    local insert_query = sql.builder.insert("dataflow_data")
        :set_map({
            data_id = data_id,
            dataflow_id = dataflow_id,
            node_id = node_id,
            type = payload.data_type,
            discriminator = payload.discriminator,
            key = payload.key,
            content = content_value,
            content_type = content_type,
            metadata = metadata,
            created_at = now_ts
        })

    local executor = insert_query:run_with(tx)
    local uses_unique_slot = is_workflow_output_unique_slot(payload) or
        is_success_result_unique_slot(payload) or is_iteration_terminal_unique_slot(payload)
    local insert_savepoint = (uses_unique_slot or is_idempotent_explicit_replay(payload)) and
        savepoint_name() or nil

    if insert_savepoint then
        local _, savepoint_err = create_savepoint(tx, insert_savepoint)
        if savepoint_err then
            return nil, savepoint_err
        end
    end

    local result, err = executor:exec()

    if err then
        if insert_savepoint then
            local _, rollback_err = rollback_savepoint(tx, insert_savepoint)
            if rollback_err then
                return nil, rollback_err
            end
            local _, release_err = release_savepoint(tx, insert_savepoint)
            if release_err then
                return nil, release_err
            end
        end

        if is_unique_violation(err) then
            if is_idempotent_explicit_replay(payload) then
                return resolve_idempotent_explicit_replay(
                    tx, dataflow_id, op_id, data_id, payload,
                    content_value, content_type, metadata)
            end
            local existing_row, existing_err = find_existing_unique_data_row(tx, dataflow_id, payload)
            if existing_err then
                return nil, existing_err
            end

            if existing_row and existing_row.data_id then
                if is_iteration_terminal_unique_slot(payload) then
                    return replace_iteration_terminal(
                        tx, dataflow_id, op_id, payload, existing_row.data_id,
                        content_value, content_type, metadata
                    )
                end
                return {
                    data_id = existing_row.data_id,
                    changes_made = false,
                    op_id = op_id,
                    deduplicated = true
                }
            end
        end

        return nil, "Failed to create data record [type=" ..
            tostring(payload.data_type) .. ", data_id=" .. tostring(data_id) .. "]: " .. err
    end

    if insert_savepoint then
        local _, release_err = release_savepoint(tx, insert_savepoint)
        if release_err then
            return nil, release_err
        end
    end

    -- A timed signal yield and its wake must commit atomically. This is a
    -- targeted projection of the persisted deadline, never a scan of yield JSON.
    if payload.data_type == consts.DATA_TYPE.NODE_YIELD and type(raw_content) == "table" then
        local yield_context = type(raw_content.yield_context) == "table" and raw_content.yield_context or {}
        local wake_at = yield_context.timeout_deadline
        if type(wake_at) == "string" and wake_at ~= "" then
            local yield_id = raw_content.yield_id or payload.key
            local wake_result, wake_err = activation_repo.register_yield_wake_tx(
                tx, dataflow_id, tostring(yield_id), wake_at)
            if wake_err then return nil, "Failed to register dataflow wake: " .. tostring(wake_err) end
            wake_index_changed = wake_result.changed == true
        end
    end

    -- Consuming a wait result removes exactly that wait's timer in the same
    -- transaction. Other branches and later wait episodes remain untouched.
    if payload.data_type == consts.DATA_TYPE.NODE_YIELD_RESULT then
        local consume_wake_keys = payload.consume_wake_keys
        if type(consume_wake_keys) ~= "table" then
            consume_wake_keys = type(payload.key) == "string" and { "yield:" .. payload.key } or {}
        end
        for _, wake_key in ipairs(consume_wake_keys) do
            if type(wake_key) == "string" and wake_key ~= "" then
                local wake_result, wake_err = sql.builder.delete("dataflow_wakes")
                    :where("dataflow_id = ?", dataflow_id)
                    :where("wake_key = ?", wake_key)
                    :run_with(tx)
                    :exec()
                if wake_err then return nil, "Failed to consume yield wake: " .. tostring(wake_err) end
                wake_index_changed = wake_index_changed or (wake_result.rows_affected or 0) > 0
            end
        end
    end

    return {
        data_id = data_id,
        changes_made = true,
        op_id = op_id,
        wake_index_changed = wake_index_changed,
    }
end

handlers[constants.COMMAND_TYPES.UPDATE_DATA] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}

    if not payload.data_id then
        return nil, "Data ID is required"
    end

    local update_query = sql.builder.update("dataflow_data")
        :where("data_id = ?", payload.data_id)
        :where("dataflow_id = ?", dataflow_id)

    local has_update = false

    if payload.content ~= nil then
        local content_value = payload.content
        if type(content_value) == "table" then
            local encoded, err_encode = json.encode(content_value)
            if err_encode then
                return nil, "Failed to encode content: " .. err_encode
            end
            content_value = encoded
        end

        update_query = update_query:set("content", encoding.ensure_storable(content_value, payload.content_type))
        has_update = true
    end

    if payload.content_type ~= nil then
        update_query = update_query:set("content_type", payload.content_type)
        has_update = true
    end

    if payload.metadata ~= nil then
        local metadata = payload.metadata
        if type(metadata) == "table" then
            local encoded, err_encode = json.encode(metadata)
            if err_encode then
                return nil, "Failed to encode metadata: " .. err_encode
            end
            metadata = encoded
        end

        update_query = update_query:set("metadata", encoding.ensure_utf8(metadata))
        has_update = true
    end

    if payload.data_type ~= nil then
        update_query = update_query:set("type", payload.data_type)
        has_update = true
    end

    if payload.discriminator ~= nil then
        update_query = update_query:set("discriminator", payload.discriminator)
        has_update = true
    end

    if payload.key ~= nil then
        update_query = update_query:set("key", payload.key)
        has_update = true
    end

    if not has_update then
        return {
            data_id = payload.data_id,
            changes_made = false,
            op_id = op_id,
            message = "No fields to update"
        }
    end

    local executor = update_query:run_with(tx)
    local result, err = executor:exec()

    if err then
        return nil, "Failed to update data record: " .. err
    end

    if result.rows_affected == 0 and payload.create_if_missing == true then
        -- A zero-row UPDATE is not enough to prove absence: another transaction
        -- may be committing the initial cursor row. Insert-if-absent converges that
        -- race without raising a uniqueness error, then the retry applies our newer
        -- value. The normal rolling-update path remains a single write.
        if payload.data_type == nil or payload.content == nil then
            return nil, "Mutable data slot creation requires data_type and content"
        end
        local content_value = payload.content
        if type(content_value) == "table" then
            local encoded, encode_err = json.encode(content_value)
            if encode_err then return nil, "Failed to encode content: " .. encode_err end
            content_value = encoded
        end
        content_value = encoding.ensure_utf8(content_value)
        local metadata = payload.metadata or "{}"
        if type(metadata) == "table" then
            local encoded, encode_err = json.encode(metadata)
            if encode_err then return nil, "Failed to encode metadata: " .. encode_err end
            metadata = encoded
        end
        metadata = encoding.ensure_utf8(metadata)
        local inserted, insert_err = sql.builder.insert("dataflow_data")
            :set_map({
                data_id = payload.data_id,
                dataflow_id = dataflow_id,
                node_id = payload.node_id or sql.as.null(),
                type = payload.data_type,
                discriminator = payload.discriminator,
                key = payload.key,
                content = content_value,
                content_type = payload.content_type or "application/json",
                metadata = metadata,
                created_at = time.now():format(time.RFC3339NANO),
            })
            :suffix("ON CONFLICT(data_id) DO NOTHING")
            :run_with(tx)
            :exec()
        if insert_err then return nil, "Failed to create mutable data slot: " .. insert_err end

        local rows, query_err = sql.builder.select("dataflow_id")
            :from("dataflow_data")
            :where("data_id = ?", payload.data_id)
            :limit(1)
            :run_with(tx)
            :query()
        if query_err then
            return nil, "Failed to verify mutable data slot: " .. query_err
        end
        if not rows or not rows[1] or tostring(rows[1].dataflow_id) ~= tostring(dataflow_id) then
            return nil, "Mutable data slot belongs to another workflow or could not be created"
        end

        local retry_payload = {}
        for key, value in pairs(payload) do retry_payload[key] = value end
        retry_payload.create_if_missing = false
        local retried, retry_err = handlers[constants.COMMAND_TYPES.UPDATE_DATA](tx, dataflow_id, op_id, {
            type = constants.COMMAND_TYPES.UPDATE_DATA,
            payload = retry_payload,
        })
        if retry_err then return nil, retry_err end
        retried.created = inserted.rows_affected > 0
        retried.deduplicated = inserted.rows_affected == 0
        return retried
    end

    return {
        data_id = payload.data_id,
        changes_made = result.rows_affected > 0,
        op_id = op_id,
        rows_affected = result.rows_affected
    }
end

handlers[constants.COMMAND_TYPES.DELETE_DATA] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}

    if not payload.data_id then
        return nil, "Data ID is required"
    end

    local delete_query = sql.builder.delete("dataflow_data")
        :where("data_id = ?", payload.data_id)
        :where("dataflow_id = ?", dataflow_id)

    local executor = delete_query:run_with(tx)
    local result, err = executor:exec()

    if err then
        return nil, "Failed to delete data record: " .. err
    end

    return {
        data_id = payload.data_id,
        changes_made = result.rows_affected > 0,
        op_id = op_id,
        rows_affected = result.rows_affected
    }
end

-- Workflow Operations
handlers[constants.COMMAND_TYPES.CREATE_WORKFLOW] = function(tx, dataflow_id, op_id, command)
    local payload = command.payload or {}

    if not payload.dataflow_id and not dataflow_id then
        return nil, "Workflow ID is required"
    end

    local wf_id = payload.dataflow_id or dataflow_id

    if not payload.actor_id then
        return nil, "User ID is required"
    end

    if not payload.type then
        return nil, "Workflow type is required"
    end

    local now_ts_str = time.now():format(time.RFC3339NANO)

    local meta_json_val_for_db = "{}"
    if payload.metadata ~= nil then
        if type(payload.metadata) == "table" then
            local encoded, err_json = json.encode(payload.metadata)
            if err_json then
                return nil, "Failed to encode metadata: " .. err_json
            end
            meta_json_val_for_db = encoded
        elseif type(payload.metadata) == "string" then
            meta_json_val_for_db = payload.metadata
        else
            return nil, "Invalid metadata type: must be a table or a JSON string"
        end
    end

    local insert_query = sql.builder.insert("dataflows")
        :set_map({
            dataflow_id = wf_id,
            parent_dataflow_id = payload.parent_dataflow_id or sql.as.null(),
            actor_id = payload.actor_id,
            actor_context = payload.actor_context or sql.as.null(),
            type = payload.type,
            status = payload.status or "pending",
            metadata = meta_json_val_for_db,
            created_at = now_ts_str,
            updated_at = now_ts_str
        })

    local executor = insert_query:run_with(tx)
    local result_exec, err_exec = executor:exec()

    if err_exec then
        return nil, "Failed to create dataflow: " .. err_exec
    end

    return {
        dataflow_id = wf_id,
        changes_made = true,
        op_id = op_id
    }
end

handlers[constants.COMMAND_TYPES.UPDATE_WORKFLOW] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}
    local wf_id_to_update = payload.dataflow_id or dataflow_id
    local terminal = payload.status == constants.STATUS.COMPLETED_SUCCESS or
        payload.status == constants.STATUS.COMPLETED_FAILURE or
        payload.status == constants.STATUS.CANCELLED or
        payload.status == constants.STATUS.TERMINATED

    -- A terminal update crosses from the workflow row into activation and wake
    -- rows. Establish the canonical parent-first lock order before UPDATE takes
    -- PostgreSQL's weaker NO KEY UPDATE lock; upgrading that lock afterwards can
    -- deadlock with a concurrent commit holding a foreign-key KEY SHARE lock.
    if terminal then
        local _, lock_err = activation_repo.lock_workflow_tx(tx, wf_id_to_update)
        if lock_err then
            if tostring(lock_err) == "dataflow not found" then
                return nil, "Workflow not found or no changes applied"
            end
            return nil, "Failed to lock workflow lifecycle: " .. tostring(lock_err)
        end
    end

    -- Metadata merge configuration - default is merge=true for consistency
    local merge_metadata = payload.merge_metadata
    if merge_metadata == nil then
        merge_metadata = true -- Default to merge
    end

    local update_query_builder = sql.builder.update("dataflows")
        :where("dataflow_id = ?", wf_id_to_update)

    local has_real_update_field = false

    if payload.type then
        update_query_builder = update_query_builder:set("type", payload.type)
        has_real_update_field = true
    end

    if payload.status then
        update_query_builder = update_query_builder:set("status", payload.status)
        has_real_update_field = true
    end

    if payload.last_commit_id then
        update_query_builder = update_query_builder:set("last_commit_id", payload.last_commit_id)
        has_real_update_field = true
    end

    if payload.metadata ~= nil then
        local meta_val_for_db

        if merge_metadata and payload.metadata then
            -- Read existing metadata first for merging
            local existing_query = sql.builder.select("metadata")
                :from("dataflows")
                :where("dataflow_id = ?", wf_id_to_update)

            local existing_executor = existing_query:run_with(tx)
            local existing_result, existing_err = existing_executor:query()

            if existing_err then
                return nil, "Failed to read existing metadata for merge: " .. existing_err
            end

            -- Parse existing metadata
            local existing_metadata = {}
            if #existing_result > 0 and existing_result[1].metadata then
                local existing_meta_str = existing_result[1].metadata :: string
                if existing_meta_str and existing_meta_str ~= "" and existing_meta_str ~= "{}" then
                    local decoded, decode_err = json.decode(existing_meta_str)
                    if not decode_err and type(decoded) == "table" then
                        existing_metadata = decoded
                    end
                end
            end

            -- Parse new metadata
            local new_metadata = payload.metadata
            if type(new_metadata) == "string" then
                local decoded, decode_err = json.decode(new_metadata)
                if not decode_err and type(decoded) == "table" then
                    new_metadata = decoded
                elseif decode_err then
                    return nil, "Failed to decode new metadata JSON: " .. decode_err
                end
            end

            -- Merge metadata: existing + new (new overwrites existing keys)
            local merged_metadata = {}

            -- Copy existing metadata
            if type(existing_metadata) == "table" then
                for k, v in pairs(existing_metadata) do
                    merged_metadata[k] = v
                end
            end

            -- Overlay new metadata
            if type(new_metadata) == "table" then
                for k, v in pairs(new_metadata) do
                    merged_metadata[k] = v
                end
            end

            -- Encode merged result
            local encoded, err_json = json.encode(merged_metadata)
            if err_json then
                return nil, "Failed to encode merged metadata: " .. err_json
            end
            meta_val_for_db = encoded

        else
            -- Replacement mode (original behavior)
            if payload.metadata == nil then
                meta_val_for_db = sql.as.null()
            elseif type(payload.metadata) == "table" then
                local encoded, err_json = json.encode(payload.metadata)
                if err_json then
                    return nil, "Failed to encode metadata for update: " .. err_json
                end
                meta_val_for_db = encoded
            elseif type(payload.metadata) == "string" then
                meta_val_for_db = payload.metadata
            else
                return nil, "Invalid metadata type for update: must be a table, JSON string, or nil (for SQL NULL)"
            end
        end

        update_query_builder = update_query_builder:set("metadata", encoding.ensure_utf8(meta_val_for_db))
        has_real_update_field = true
    end

    if not has_real_update_field then
        return {
            dataflow_id = wf_id_to_update,
            changes_made = false,
            op_id = op_id,
            message = "No valid fields provided for update"
        }
    end

    local now_ts = time.now():format(time.RFC3339NANO)
    update_query_builder = update_query_builder:set("updated_at", now_ts)

    local executor = update_query_builder:run_with(tx)
    local result_exec, err_exec = executor:exec()

    if err_exec then
        return nil, "Failed to update dataflow: " .. err_exec
    end

    if result_exec.rows_affected == 0 then
        return nil, "Workflow not found or no changes applied"
    end

    if terminal then
        local _, projection_err = activation_repo.disable_terminal_tx(tx, wf_id_to_update, now_ts)
        if projection_err then
            return nil, "Failed to disable terminal activation: " .. tostring(projection_err)
        end
    end

    return {
        dataflow_id = wf_id_to_update,
        changes_made = true,
        op_id = op_id,
        rows_affected = result_exec.rows_affected,
        metadata_merged = merge_metadata,
        terminal = terminal,
        wake_index_changed = terminal,
    }
end

handlers[constants.COMMAND_TYPES.PASSIVATE_WORKFLOW] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}
    local wf_id = payload.dataflow_id or dataflow_id
    local generation = tonumber(payload.activation_generation)
    if not generation or generation < 1 or generation % 1 ~= 0 then
        return nil, "Activation generation must be a positive integer"
    end

    local now_ts = time.now():format(time.RFC3339NANO)
    local release, release_err = activation_repo.release_if_generation_tx(tx, wf_id, generation, now_ts)
    if release_err then return nil, "Failed to release activation: " .. tostring(release_err) end
    if not release.released then
        return {
            dataflow_id = wf_id,
            changes_made = false,
            op_id = op_id,
            released = false,
            terminal = release.terminal == true,
            current_generation = release.generation,
        }
    end

    local update_result, update_err = sql.builder.update("dataflows")
        :set("status", constants.STATUS.WAITING)
        :set("updated_at", now_ts)
        :where("dataflow_id = ?", wf_id)
        :run_with(tx):exec()
    if update_err then return nil, "Failed to passivate workflow: " .. tostring(update_err) end
    if not update_result or (update_result.rows_affected or 0) ~= 1 then
        return nil, "Workflow not found while passivating"
    end

    return {
        dataflow_id = wf_id,
        changes_made = true,
        op_id = op_id,
        released = true,
        generation = generation,
        current_generation = generation,
        status = constants.STATUS.WAITING,
        wake_index_changed = true,
    }
end

handlers[constants.COMMAND_TYPES.COMPLETE_WORKFLOW] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end
    local payload = command.payload or {}
    local wf_id = payload.dataflow_id or dataflow_id
    local generation = tonumber(payload.activation_generation)
    if not generation or generation < 1 or generation % 1 ~= 0 then
        return nil, "Activation generation must be a positive integer"
    end
    local status = payload.status
    local terminal = status == constants.STATUS.COMPLETED_SUCCESS or
        status == constants.STATUS.COMPLETED_FAILURE
    if not terminal then return nil, "Completion status must be completed or failed" end

    local now_ts = time.now():format(time.RFC3339NANO)
    local release, release_err = activation_repo.release_if_generation_tx(tx, wf_id, generation, now_ts)
    if release_err then return nil, "Failed to fence workflow completion: " .. tostring(release_err) end
    if not release.released then
        return {
            dataflow_id = wf_id,
            changes_made = false,
            op_id = op_id,
            completed = false,
            terminal = release.terminal == true,
            current_generation = release.generation,
        }
    end

    local update_result, update_err = handlers[constants.COMMAND_TYPES.UPDATE_WORKFLOW](
        tx, dataflow_id, op_id, {
            type = constants.COMMAND_TYPES.UPDATE_WORKFLOW,
            payload = {
                dataflow_id = wf_id,
                status = status,
                metadata = payload.metadata,
                merge_metadata = payload.merge_metadata,
            },
        })
    if update_err then return nil, update_err end
    update_result.completed = true
    update_result.generation = generation
    update_result.current_generation = generation
    return update_result
end

handlers[constants.COMMAND_TYPES.DELETE_WORKFLOW] = function(tx, dataflow_id, op_id, command)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    local payload = command.payload or {}
    local wf_id_to_delete = payload.dataflow_id or dataflow_id

    local wake_result, wake_err = sql.builder.delete("dataflow_wakes")
        :where("dataflow_id = ?", wf_id_to_delete)
        :run_with(tx)
        :exec()
    if wake_err then return nil, "Failed to clear deleted dataflow wake: " .. tostring(wake_err) end
    local wake_index_changed = (wake_result.rows_affected or 0) > 0

    local delete_query = sql.builder.delete("dataflows")
        :where("dataflow_id = ?", wf_id_to_delete)

    local executor = delete_query:run_with(tx)
    local result_exec, err_exec = executor:exec()

    if err_exec then
        return nil, "Failed to delete dataflow: " .. err_exec
    end

    if result_exec.rows_affected == 0 then
        return nil, "Workflow not found"
    end

    return {
        dataflow_id = wf_id_to_delete,
        changes_made = true,
        op_id = op_id,
        rows_affected = result_exec.rows_affected,
        deleted = true,
        wake_index_changed = wake_index_changed,
    }
end

-- Execute commands within a transaction
-- @param tx (sql.Transaction): Database transaction to use
-- @param dataflow_id (string): ID of the dataflow to operate on
-- @param op_id (string): Operation ID (generated if nil)
-- @param commands (table): Single command or array of commands
-- @return (table, string): Result of operations and error message if failed
function ops.execute(tx, dataflow_id, op_id, commands)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Workflow ID is required"
    end

    -- Generate operation ID if not provided
    op_id = op_id or uuid.v7()

    -- Handle both single command and array of commands
    local command_array = {}
    if type(commands) == "table" and commands.type then
        -- Single command
        table.insert(command_array, commands)
    elseif type(commands) == "table" then
        -- Array of commands
        command_array = commands
    else
        return nil, "Commands must be a table or array of tables"
    end

    local changes_made = false
    local results = {}

    -- Check if any commands are CREATE_WORKFLOW operations for timestamp logic
    local has_workflow_creation = false
    for _, command in ipairs(command_array) do
        if command.type == constants.COMMAND_TYPES.CREATE_WORKFLOW then
            has_workflow_creation = true
            break
        end
    end

    for i, command in ipairs(command_array) do
        local handler = handlers[command.type]

        if not handler then
            return nil, "Unknown command type: " .. (command.type or "nil") .. " at index " .. i
        end

        if type(handler) ~= "function" then
            return nil, "Handler for command type " .. command.type .. " is not a function at index " .. i
        end

        -- Pass the same op_id to all handlers in this batch
        local result, err_handler = handler(tx, dataflow_id, op_id, command)

        if err_handler then
            return nil, "Error executing command at index " .. i .. ": " .. err_handler
        end

        -- Keep the processed command on every successful result, including an
        -- idempotent duplicate. Recovery still needs to reconcile the durable
        -- row with its in-memory state even when this transaction made no
        -- database change.
        if result then
            result.input = command
        end

        -- Track if any command made changes
        if result and result.changes_made then
            changes_made = true
        end

        -- Store command result
        table.insert(results, result)
    end

    -- Update dataflow timestamp for all operations EXCEPT when creating workflows
    -- CREATE_WORKFLOW sets its own timestamps during creation
    -- All other operations (CREATE_NODE, CREATE_DATA, UPDATE_*, DELETE_*) should update workflow timestamp
    if changes_made and not has_workflow_creation then
        local update_ts_sql_builder = sql.builder.update("dataflows")
            :set("updated_at", time.now():format(time.RFC3339NANO))
            :where("dataflow_id = ?", dataflow_id)

        local executor = update_ts_sql_builder:run_with(tx)
        local _, update_err = executor:exec()

        if update_err then
            return nil, "Commands succeeded but failed to update timestamp: " .. update_err
        end
    end

    return { results = results, changes_made = changes_made, op_id = op_id }, nil
end

return ops

