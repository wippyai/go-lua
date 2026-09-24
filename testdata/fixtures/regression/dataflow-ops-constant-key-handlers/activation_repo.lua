local sql = require("sql")
local json = require("json")
local consts = require("dataflow_consts")

local activation_repo = {}

local TERMINAL_STATUS = {
    [consts.STATUS.COMPLETED_SUCCESS] = true,
    [consts.STATUS.COMPLETED_FAILURE] = true,
    [consts.STATUS.CANCELLED] = true,
    [consts.STATUS.TERMINATED] = true,
}

local TERMINAL_VALUES = {
    consts.STATUS.COMPLETED_SUCCESS,
    consts.STATUS.COMPLETED_FAILURE,
    consts.STATUS.CANCELLED,
    consts.STATUS.TERMINATED,
}

local function rebind(query, db_type)
    if db_type ~= sql.type.POSTGRES and db_type ~= "postgres" then return query end
    local index = 0
    return (query:gsub("%?", function()
        index = index + 1
        return "$" .. index
    end))
end

local function tx_query(tx, query, params)
    local db_type, type_err = tx:db_type()
    if type_err then return nil, type_err end
    return tx:query(rebind(query, db_type), params or {})
end

local function tx_execute(tx, query, params)
    local db_type, type_err = tx:db_type()
    if type_err then return nil, type_err end
    return tx:execute(rebind(query, db_type), params or {})
end

local function db_query(db, query, params)
    local db_type, type_err = db:type()
    if type_err then return nil, type_err end
    return db:query(rebind(query, db_type), params or {})
end

local function validate_id(dataflow_id)
    if type(dataflow_id) ~= "string" or dataflow_id == "" then
        return nil, "dataflow_id is required"
    end
    return true, nil
end

local function validate_timestamp(value, field)
    if type(value) ~= "string" or value == "" then
        return nil, field .. " is required"
    end
    return true, nil
end

local function validate_json_value(value: any, seen: any, path: string)
    local kind = type(value)
    if kind == "nil" or kind == "string" or kind == "boolean" then return true, nil end
    if kind == "number" then
        if value ~= value or value == math.huge or value == -math.huge then
            return nil, path .. " contains a non-finite number"
        end
        return true, nil
    end
    if kind ~= "table" then return nil, path .. " contains unsupported " .. kind end
    if getmetatable(value) ~= nil then return nil, path .. " must not have a metatable" end
    if seen[value] then return nil, path .. " contains a cycle" end
    seen[value] = true

    local key_kind = nil
    local count = 0
    local max_index = 0
    for key, item in pairs(value) do
        local current_kind = type(key)
        if current_kind == "number" then
            local array_index = tonumber(key) or 0
            if array_index < 1 or array_index % 1 ~= 0 then
                seen[value] = nil
                return nil, path .. " contains an invalid array index"
            end
            max_index = math.max(max_index, array_index)
            current_kind = "array"
        elseif current_kind == "string" then
            current_kind = "object"
        else
            seen[value] = nil
            return nil, path .. " contains an unsupported key"
        end
        if key_kind and key_kind ~= current_kind then
            seen[value] = nil
            return nil, path .. " mixes object and array keys"
        end
        key_kind = current_kind
        count = count + 1
        local ok, err = validate_json_value(item, seen, path .. "." .. tostring(key))
        if not ok then
            seen[value] = nil
            return nil, err
        end
    end
    seen[value] = nil
    if key_kind == "array" and max_index ~= count then
        return nil, path .. " contains a sparse array"
    end
    return true, nil
end

local function encode_launch_args(launch_args: any)
    if launch_args == nil then return nil, nil end
    if type(launch_args) ~= "table" or getmetatable(launch_args) ~= nil then
        return nil, "launch_args must be a plain object"
    end
    for key in pairs(launch_args) do
        if type(key) ~= "string" then return nil, "launch_args must be a plain object" end
    end
    local valid, validation_err = validate_json_value(launch_args, {}, "launch_args")
    if not valid then return nil, validation_err end
    local encoded, encode_err = json.encode(launch_args)
    if encode_err then return nil, "failed to encode launch_args: " .. tostring(encode_err) end
    return encoded, nil
end

local function decode_launch_args(value: any)
    if value == nil then return nil, nil end
    local decoded = value
    if type(value) == "string" then
        local decode_err
        decoded, decode_err = json.decode(value)
        if decode_err then return nil, "failed to decode launch_args: " .. tostring(decode_err) end
    end
    if type(decoded) ~= "table" then return nil, "launch_args is not an object" end
    for key in pairs(decoded) do
        if type(key) ~= "string" then return nil, "launch_args is not an object" end
    end
    return decoded, nil
end

local function normalize_row(row: any)
    if not row then return nil, nil end
    local launch_args, decode_err = decode_launch_args(row.launch_args)
    if decode_err then return nil, decode_err end
    return {
        dataflow_id = tostring(row.dataflow_id),
        generation = tonumber(row.generation),
        desired_active = row.desired_active == true or tonumber(row.desired_active) == 1,
        owner_epoch = row.owner_epoch and tostring(row.owner_epoch) or nil,
        launch_args = launch_args,
        requested_at = tostring(row.requested_at),
        updated_at = tostring(row.updated_at),
    }, nil
end

-- The workflow row is the first lock in every transaction that also mutates
-- activation or wake rows. PostgreSQL foreign-key checks can hold KEY SHARE on
-- this parent row, so acquiring a weaker UPDATE lock and upgrading it later can
-- deadlock with a concurrent commit. Callers that cross the workflow/lifecycle
-- boundary must establish this lock before either side is changed.
function activation_repo.lock_workflow_tx(tx, dataflow_id)
    local db_type, type_err = tx:db_type()
    if type_err then return nil, type_err end
    if db_type ~= sql.type.POSTGRES and db_type ~= "postgres" then
        -- SQLite has no row-level SELECT FOR UPDATE. Make the parent row the
        -- first write so concurrent activation/terminal transactions serialize
        -- before either can inspect lifecycle state or touch a wake.
        local lock_result, lock_err = tx:execute(
            "UPDATE dataflows SET updated_at = updated_at WHERE dataflow_id = ?",
            { dataflow_id })
        if lock_err then return nil, lock_err end
        if not lock_result or (lock_result.rows_affected or 0) == 0 then
            return nil, "dataflow not found"
        end
    end
    local query = "SELECT status FROM dataflows WHERE dataflow_id = ? LIMIT 1"
    if db_type == sql.type.POSTGRES or db_type == "postgres" then
        query = query .. " FOR UPDATE"
    end
    local rows, query_err = tx:query(rebind(query, db_type), { dataflow_id })
    if query_err then return nil, query_err end
    if not rows or not rows[1] then return nil, "dataflow not found" end
    return tostring(rows[1].status), nil
end

local function get_tx(tx, dataflow_id)
    local rows, query_err = tx_query(tx, [[
        SELECT dataflow_id, generation, desired_active, owner_epoch,
               launch_args, requested_at, updated_at
        FROM dataflow_activations WHERE dataflow_id = ? LIMIT 1
    ]], { dataflow_id })
    if query_err then return nil, query_err end
    return normalize_row(rows and rows[1] or nil)
end

local function terminal_result_from_status(status)
    if TERMINAL_STATUS[status] then
        return { changed = false, terminal = true, status = status }
    end
    return nil
end

-- The workflow row is already lifecycle-locked by the caller. Terminal state
-- owns both durable activation intent and its wake index, so converge them in
-- the same transaction before returning the terminal observation.
local function cleanup_terminal_tx(tx, dataflow_id, status, now_value)
    local activation_result, activation_err = tx_execute(tx, [[
        UPDATE dataflow_activations
        SET desired_active = ?, launch_args = NULL, updated_at = ?
        WHERE dataflow_id = ? AND (desired_active = ? OR launch_args IS NOT NULL)
    ]], { false, now_value, dataflow_id, true })
    if activation_err then return nil, "failed to disable terminal activation: " .. tostring(activation_err) end

    local wake_result, wake_err = tx_execute(tx,
        "DELETE FROM dataflow_wakes WHERE dataflow_id = ?", { dataflow_id })
    if wake_err then return nil, "failed to clear terminal wakes: " .. tostring(wake_err) end

    local activation_disabled = activation_result and (activation_result.rows_affected or 0) > 0
    local wake_index_changed = wake_result and (wake_result.rows_affected or 0) > 0
    return {
        changed = activation_disabled or wake_index_changed,
        terminal = true,
        status = status,
        activation_disabled = activation_disabled,
        wake_index_changed = wake_index_changed,
    }, nil
end

local function advance_activation_tx(tx, dataflow_id, launch_args: any, now_value, preserve_launch_args)
    local encoded_args, encode_err = encode_launch_args(launch_args)
    if encode_err then return nil, encode_err end

    local update_args = preserve_launch_args and "dataflow_activations.launch_args" or "excluded.launch_args"
    local result, write_err = tx_execute(tx, ([[
        INSERT INTO dataflow_activations(
            dataflow_id, generation, desired_active, owner_epoch,
            launch_args, requested_at, updated_at
        )
        SELECT ?, 1, ?, NULL, ?, ?, ? FROM dataflows
        WHERE dataflow_id = ? AND status NOT IN (?, ?, ?, ?)
        ON CONFLICT(dataflow_id) DO UPDATE SET
            generation = dataflow_activations.generation + 1,
            desired_active = excluded.desired_active,
            owner_epoch = NULL,
            launch_args = %s,
            requested_at = excluded.requested_at,
            updated_at = excluded.updated_at
        WHERE EXISTS (
            SELECT 1 FROM dataflows
            WHERE dataflow_id = excluded.dataflow_id AND status NOT IN (?, ?, ?, ?)
        )
    ]]):format(update_args), {
        dataflow_id, true, encoded_args or sql.as.null(), now_value, now_value, dataflow_id,
        TERMINAL_VALUES[1], TERMINAL_VALUES[2], TERMINAL_VALUES[3], TERMINAL_VALUES[4],
        TERMINAL_VALUES[1], TERMINAL_VALUES[2], TERMINAL_VALUES[3], TERMINAL_VALUES[4],
    })
    if write_err then return nil, "failed to advance activation: " .. tostring(write_err) end
    if not result or (result.rows_affected or 0) == 0 then
        return nil, "activation request made no change"
    end

    local row, row_err = get_tx(tx, dataflow_id)
    if row_err then return nil, row_err end
    if not row then return nil, "activation row missing after advance" end
    row.changed = true
    row.terminal = false
    return row, nil
end

function activation_repo.request_activation_tx(tx, dataflow_id, launch_args, now_value)
    if not tx then return nil, "transaction is required" end
    local valid, id_err = validate_id(dataflow_id)
    if not valid then return nil, id_err end
    valid, id_err = validate_timestamp(now_value, "requested_at")
    if not valid then return nil, id_err end
    local status, status_err = activation_repo.lock_workflow_tx(tx, dataflow_id)
    if status_err then return nil, status_err end
    local terminal = terminal_result_from_status(status)
    if terminal then return terminal, nil end
    return advance_activation_tx(tx, dataflow_id, launch_args, now_value, false)
end

function activation_repo.activate_for_signal_tx(tx, dataflow_id, wake_key, wake_at, now_value)
    if not tx then return nil, "transaction is required" end
    local valid, validation_err = validate_id(dataflow_id)
    if not valid then return nil, validation_err end
    if type(wake_key) ~= "string" or not wake_key:match("^signal:.+") then
        return nil, "signal wake_key is required"
    end
    valid, validation_err = validate_timestamp(wake_at, "wake_at")
    if not valid then return nil, validation_err end
    valid, validation_err = validate_timestamp(now_value, "requested_at")
    if not valid then return nil, validation_err end

    local status, status_err = activation_repo.lock_workflow_tx(tx, dataflow_id)
    if status_err then return nil, status_err end
    local terminal = terminal_result_from_status(status)
    if terminal then
        terminal.wake_inserted = false
        return terminal, nil
    end

    local insert_result, insert_err = tx_execute(tx, [[
        INSERT INTO dataflow_wakes(dataflow_id, wake_key, wake_at, activation_generation)
        SELECT ?, ?, ?, NULL FROM dataflows
        WHERE dataflow_id = ? AND status NOT IN (?, ?, ?, ?)
        ON CONFLICT(dataflow_id, wake_key) DO NOTHING
    ]], {
        dataflow_id, wake_key, wake_at, dataflow_id,
        TERMINAL_VALUES[1], TERMINAL_VALUES[2], TERMINAL_VALUES[3], TERMINAL_VALUES[4],
    })
    if insert_err then return nil, "failed to insert signal wake: " .. tostring(insert_err) end

    if not insert_result or (insert_result.rows_affected or 0) == 0 then
        local rows, row_err = tx_query(tx, [[
            SELECT activation_generation FROM dataflow_wakes
            WHERE dataflow_id = ? AND wake_key = ? LIMIT 1
        ]], { dataflow_id, wake_key })
        if row_err then return nil, row_err end
        return {
            changed = false,
            terminal = false,
            wake_inserted = false,
            generation = rows and rows[1] and tonumber(rows[1].activation_generation) or nil,
        }, nil
    end

    local activation, activation_err = advance_activation_tx(tx, dataflow_id, nil, now_value, true)
    if activation_err then return nil, activation_err end
    if activation.terminal then return nil, "signal wake inserted for terminal dataflow" end

    local stamp_result, stamp_err = tx_execute(tx, [[
        UPDATE dataflow_wakes SET activation_generation = ?
        WHERE dataflow_id = ? AND wake_key = ? AND activation_generation IS NULL
    ]], { activation.generation, dataflow_id, wake_key })
    if stamp_err then return nil, "failed to fence signal wake: " .. tostring(stamp_err) end
    if not stamp_result or (stamp_result.rows_affected or 0) ~= 1 then
        return nil, "signal wake generation fence was not written"
    end

    activation.wake_inserted = true
    return activation, nil
end

function activation_repo.activate_due_tx(tx, dataflow_id, wake_key, now_value)
    if not tx then return nil, "transaction is required" end
    local valid, validation_err = validate_id(dataflow_id)
    if not valid then return nil, validation_err end
    if type(wake_key) ~= "string" or wake_key == "" then return nil, "wake_key is required" end
    valid, validation_err = validate_timestamp(now_value, "now")
    if not valid then return nil, validation_err end

    local status, status_err = activation_repo.lock_workflow_tx(tx, dataflow_id)
    if status_err then return nil, status_err end
    local terminal = terminal_result_from_status(status)
    if terminal then
        local cleaned, cleanup_err = cleanup_terminal_tx(tx, dataflow_id, status, now_value)
        if cleanup_err then return nil, cleanup_err end
        cleaned.promoted = false
        return cleaned, nil
    end

    -- This conditional no-op update is the row lock/CAS. On PostgreSQL a
    -- concurrent scanner waits and then rechecks activation_generation; on
    -- SQLite it acquires the database writer lock before generation advances.
    local lock_result, lock_err = tx_execute(tx, [[
        UPDATE dataflow_wakes SET wake_at = wake_at
        WHERE dataflow_id = ? AND wake_key = ? AND wake_at <= ?
          AND activation_generation IS NULL
          AND EXISTS (
              SELECT 1 FROM dataflows
              WHERE dataflow_id = ? AND status NOT IN (?, ?, ?, ?)
          )
    ]], {
        dataflow_id, wake_key, now_value, dataflow_id,
        TERMINAL_VALUES[1], TERMINAL_VALUES[2], TERMINAL_VALUES[3], TERMINAL_VALUES[4],
    })
    if lock_err then return nil, "failed to lock due wake: " .. tostring(lock_err) end

    if lock_result and (lock_result.rows_affected or 0) > 0 then
        local activation, activation_err = advance_activation_tx(tx, dataflow_id, nil, now_value, true)
        if activation_err then return nil, activation_err end
        if activation.terminal then return nil, "due wake promoted for terminal dataflow" end
        local stamp_result, stamp_err = tx_execute(tx, [[
            UPDATE dataflow_wakes SET activation_generation = ?
            WHERE dataflow_id = ? AND wake_key = ? AND activation_generation IS NULL
        ]], { activation.generation, dataflow_id, wake_key })
        if stamp_err then return nil, "failed to fence due wake: " .. tostring(stamp_err) end
        if not stamp_result or (stamp_result.rows_affected or 0) ~= 1 then
            return nil, "due wake generation fence was not written"
        end
        activation.promoted = true
        return activation, nil
    end

    local rows, row_err = tx_query(tx, [[
        SELECT wake_at, activation_generation FROM dataflow_wakes
        WHERE dataflow_id = ? AND wake_key = ? LIMIT 1
    ]], { dataflow_id, wake_key })
    if row_err then return nil, row_err end
    local row = rows and rows[1] or nil
    if not row then
        return { changed = false, terminal = false, promoted = false, missing = true }, nil
    end
    if row.activation_generation ~= nil then
        return {
            changed = false,
            terminal = false,
            promoted = false,
            already_promoted = true,
            generation = tonumber(row.activation_generation),
        }, nil
    end
    return { changed = false, terminal = false, promoted = false, due = false }, nil
end

function activation_repo.release_if_generation_tx(tx, dataflow_id, generation, now_value)
    if not tx then return nil, "transaction is required" end
    local valid, validation_err = validate_id(dataflow_id)
    if not valid then return nil, validation_err end
    generation = tonumber(generation)
    if not generation or generation < 1 or generation % 1 ~= 0 then
        return nil, "generation must be a positive integer"
    end
    valid, validation_err = validate_timestamp(now_value, "updated_at")
    if not valid then return nil, validation_err end

    local status, status_err = activation_repo.lock_workflow_tx(tx, dataflow_id)
    if status_err then return nil, status_err end
    local terminal = terminal_result_from_status(status)
    if terminal then
        terminal.released = false
        return terminal, nil
    end

    local result, update_err = tx_execute(tx, [[
        UPDATE dataflow_activations
        SET desired_active = ?, launch_args = NULL, updated_at = ?
        WHERE dataflow_id = ? AND generation = ? AND desired_active = ?
          AND EXISTS (
              SELECT 1 FROM dataflows
              WHERE dataflow_id = ? AND status NOT IN (?, ?, ?, ?)
          )
    ]], {
        false, now_value, dataflow_id, generation, true, dataflow_id,
        TERMINAL_VALUES[1], TERMINAL_VALUES[2], TERMINAL_VALUES[3], TERMINAL_VALUES[4],
    })
    if update_err then return nil, "failed to release activation: " .. tostring(update_err) end
    if result and (result.rows_affected or 0) > 0 then
        return { changed = true, released = true, generation = generation, terminal = false }, nil
    end

    local current, current_err = get_tx(tx, dataflow_id)
    if current_err then return nil, current_err end
    return {
        changed = false,
        released = false,
        terminal = false,
        generation = current and current.generation or nil,
    }, nil
end

-- Fence process ownership before spawn. A generation can be claimed only from
-- the exact epoch observed by the overseer. The write happens before process
-- creation, so an overseer crash between claim and spawn is classified as a
-- same-runtime loss rather than retried into a process flood.
function activation_repo.claim_epoch_tx(
    tx, dataflow_id, generation, observed_epoch, runtime_epoch, now_value)
    if not tx then return nil, "transaction is required" end
    local valid, validation_err = validate_id(dataflow_id)
    if not valid then return nil, validation_err end
    generation = tonumber(generation)
    if not generation or generation < 1 or generation % 1 ~= 0 then
        return nil, "generation must be a positive integer"
    end
    if type(runtime_epoch) ~= "string" or runtime_epoch == "" then
        return nil, "runtime_epoch is required"
    end
    valid, validation_err = validate_timestamp(now_value, "updated_at")
    if not valid then return nil, validation_err end

    local status, status_err = activation_repo.lock_workflow_tx(tx, dataflow_id)
    if status_err then return nil, status_err end
    local terminal = terminal_result_from_status(status)
    if terminal then
        terminal.claimed = false
        return terminal, nil
    end

    local epoch_predicate = "owner_epoch IS NULL"
    local params = { runtime_epoch, now_value, dataflow_id, generation, true }
    if observed_epoch ~= nil then
        if type(observed_epoch) ~= "string" or observed_epoch == "" then
            return nil, "observed_epoch must be nil or a non-empty string"
        end
        epoch_predicate = "owner_epoch = ?"
        table.insert(params, observed_epoch)
    end
    local result, update_err = tx_execute(tx, [[
        UPDATE dataflow_activations
        SET owner_epoch = ?, updated_at = ?
        WHERE dataflow_id = ? AND generation = ? AND desired_active = ?
          AND ]] .. epoch_predicate, params)
    if update_err then return nil, "failed to claim activation epoch: " .. tostring(update_err) end

    local current, current_err = get_tx(tx, dataflow_id)
    if current_err then return nil, current_err end
    if not current then return nil, "activation row missing after epoch claim" end
    current.claimed = result ~= nil and (result.rows_affected or 0) == 1
    current.terminal = false
    return current, nil
end

function activation_repo.consume_wake_tx(tx, dataflow_id, wake_key, generation)
    if not tx then return nil, "transaction is required" end
    local valid, validation_err = validate_id(dataflow_id)
    if not valid then return nil, validation_err end
    if type(wake_key) ~= "string" or wake_key == "" then return nil, "wake_key is required" end

    local status, status_err = activation_repo.lock_workflow_tx(tx, dataflow_id)
    if status_err then return nil, status_err end
    local terminal = terminal_result_from_status(status)
    if terminal then
        terminal.consumed = false
        return terminal, nil
    end

    local query = "DELETE FROM dataflow_wakes WHERE dataflow_id = ? AND wake_key = ?"
    local params = { dataflow_id, wake_key }
    if generation ~= nil then
        generation = tonumber(generation)
        if not generation or generation < 1 or generation % 1 ~= 0 then
            return nil, "generation must be a positive integer"
        end
        query = query .. " AND activation_generation = ?"
        table.insert(params, generation)
    end
    local result, delete_err = tx_execute(tx, query, params)
    if delete_err then return nil, "failed to consume wake: " .. tostring(delete_err) end
    return { changed = result and (result.rows_affected or 0) > 0, consumed = result and (result.rows_affected or 0) > 0 }, nil
end

-- Register or re-arm a durable yield deadline. Reusing the same logical yield
-- is a new wait episode, so any activation fence left by the previous episode
-- must be cleared atomically with the new deadline.
function activation_repo.register_yield_wake_tx(tx, dataflow_id, yield_id, wake_at)
    if not tx then return nil, "transaction is required" end
    local valid, validation_err = validate_id(dataflow_id)
    if not valid then return nil, validation_err end
    if type(yield_id) ~= "string" or yield_id == "" then return nil, "yield_id is required" end
    valid, validation_err = validate_timestamp(wake_at, "wake_at")
    if not valid then return nil, validation_err end

    local result, write_err = tx_execute(tx, [[
        INSERT INTO dataflow_wakes(dataflow_id, wake_key, wake_at, activation_generation)
        VALUES (?, ?, ?, NULL)
        ON CONFLICT(dataflow_id, wake_key) DO UPDATE SET
            wake_at = excluded.wake_at,
            activation_generation = NULL
    ]], { dataflow_id, "yield:" .. yield_id, wake_at })
    if write_err then return nil, "failed to register yield wake: " .. tostring(write_err) end
    return {
        changed = result ~= nil and (result.rows_affected or 0) > 0,
    }, nil
end

function activation_repo.disable_terminal_tx(tx, dataflow_id, now_value)
    if not tx then return nil, "transaction is required" end
    local valid, validation_err = validate_id(dataflow_id)
    if not valid then return nil, validation_err end
    valid, validation_err = validate_timestamp(now_value, "updated_at")
    if not valid then return nil, validation_err end
    local status, status_err = activation_repo.lock_workflow_tx(tx, dataflow_id)
    if status_err then return nil, status_err end
    if not TERMINAL_STATUS[status] then return nil, "dataflow is not terminal" end
    return cleanup_terminal_tx(tx, dataflow_id, status, now_value)
end

function activation_repo.get(dataflow_id)
    local valid, id_err = validate_id(dataflow_id)
    if not valid then return nil, id_err end
    local db, db_err = sql.get(consts.APP_DB)
    if db_err then return nil, db_err end
    local rows, query_err = db_query(db, [[
        SELECT dataflow_id, generation, desired_active, owner_epoch,
               launch_args, requested_at, updated_at
        FROM dataflow_activations WHERE dataflow_id = ? LIMIT 1
    ]], { dataflow_id })
    db:release()
    if query_err then return nil, query_err end
    return normalize_row(rows and rows[1] or nil)
end

function activation_repo.list_active()
    local db, db_err = sql.get(consts.APP_DB)
    if db_err then return nil, db_err end
    local rows, query_err = db_query(db, [[
        SELECT a.dataflow_id, a.generation, a.desired_active, a.owner_epoch, a.launch_args,
               a.requested_at, a.updated_at
        FROM dataflow_activations a
        JOIN dataflows d ON d.dataflow_id = a.dataflow_id
        WHERE a.desired_active = ? AND d.status NOT IN (?, ?, ?, ?)
        ORDER BY a.updated_at ASC, a.dataflow_id ASC
    ]], {
        true,
        TERMINAL_VALUES[1], TERMINAL_VALUES[2], TERMINAL_VALUES[3], TERMINAL_VALUES[4],
    })
    db:release()
    if query_err then return nil, query_err end
    local result = {}
    for _, row in ipairs(rows or {}) do
        local normalized, normalize_err = normalize_row(row)
        if normalize_err then return nil, normalize_err end
        table.insert(result, normalized)
    end
    return result, nil
end

return activation_repo

