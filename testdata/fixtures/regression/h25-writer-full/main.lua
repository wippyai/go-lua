-- Write side of the projection engine. Registers/removes a projection and its
-- catch-up cursor, advances a cursor as catch-up makes progress, and applies the
-- pause/resume lifecycle transitions. Every entry point takes the caller's db or
-- tx handle and owns only the kickside_projection / kickside_projection_cursor
-- rows — scheduling, access checks, and thread notifications belong to the
-- slices that compose this engine. Timestamps are computed in Lua via
-- time.now() in the dialect's textual shape and bound as params, so the SQL
-- stays portable. Errors are returned as values.

local sql = require("sql")
local json = require("json")
local uuid = require("uuid")
local time = require("time")
local clock = require("clock")
local types = require("types")
local core_types = require("core_types")
local proj_env = require("proj_env")
local execution_identity = require("execution_identity")
local autoinit = require("autoinit")
local proj_worker = require("proj_worker")

local writer = {}

-- A pooled connection or open transaction.
type Executor = sql.DB | sql.Transaction

local RECOVERABLE_INVALID_ERROR = types.CURSOR_ERROR.DISPATCH_ORPHAN
local WORKER_NOT_FOUND_PREFIX = "worker_ref target not found: "

-- The descriptor register_projection returns once the projection + cursor are
-- committed.
type RegisterResult = {
    projection_id: string,
    cursor_id: string,
    kind: string,
    projection_key: string,
    cursor_key: string,
    coalesce_key: string,
    batch_size: integer,
    target_seq: number?,
    worker_ref: string,
    worker_runtime: string,
    worker_input_mode: string,
    generation: number,
    created: boolean,
    window: table,
}

local function trim(value: any): string
    if type(value) ~= "string" then
        return ""
    end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local now_string = clock.now

-- Every transition that puts a cursor back into dispatch rotation clears the
-- whole recorded-failure state: a revived cursor re-enters with a full attempt
-- budget, no recorded error, and no dead-letter stamp. Leaving any of the
-- three behind makes the next claim inherit a failure it never had.
local function clear_cursor_failure(update: any): any
    return update:set("last_error", nil):set("attempts", 0):set("dead_at", nil)
end

-- Decode a stored projection body JSON into a table. Used to read the existing
-- body on a re-register so its fold read-model is preserved.
local function decode_body(raw: any): (table?, string?)
    if type(raw) ~= "string" or raw == "" then
        return {}, nil
    end
    local decoded, err = json.decode(raw :: string)
    if err or type(decoded) ~= "table" then
        return nil, tostring(err or "stored projection body is not an object")
    end
    return decoded :: table, nil
end

-- The projection body contains two different kinds of state: the declaration
-- that says how to run the projection, and the fold state produced by that
-- worker. Keep the declaration vocabulary in one place so normal
-- re-registration and destructive-storage rebuilds cannot drift apart.
local DECLARATION_BODY_KEYS = {
    "meta",
    "worker_config",
    "trigger_policy",
    "events",
    "drop_notify",
}

local function apply_declaration_fields(target: table, declaration: table): table
    for _, key in ipairs(DECLARATION_BODY_KEYS) do
        target[key] = declaration[key]
    end
    return target
end

-- Structural equality over the declaration fields. Used to detect that a module
-- upgrade changed what a stored projection declares, which is the one condition
-- that retires a recorded config fault.
local function same_declaration(a: any, b: any): boolean
    if type(a) ~= type(b) then return false end
    if type(a) ~= "table" then return a == b end
    for key, value in pairs(a :: table) do
        if not same_declaration(value, (b :: table)[key]) then return false end
    end
    for key in pairs(b :: table) do
        if (a :: table)[key] == nil then return false end
    end
    return true
end

local function declaration_only(raw: any): (table?, string?)
    local stored, decode_err = decode_body(raw)
    if decode_err or not stored then
        return nil, decode_err or "stored projection body is not an object"
    end
    return apply_declaration_fields({}, stored :: table), nil
end

-- Clamp value to [min, max], substituting fallback when value is not numeric.
local function clamp(value: any, min_value: integer, max_value: integer, fallback: integer): integer
    local n = tonumber(value)
    if not n then
        return fallback
    end
    n = math.floor(n)
    if n < min_value then
        return min_value
    end
    if n > max_value then
        return max_value
    end
    return n :: integer
end

-- True when err reports a unique-constraint violation.
local function is_unique_error(err: any): boolean
    if not err then
        return false
    end
    return string.find(string.lower(tostring(err)), "unique", 1, true) ~= nil
end

local function identity_row_from_params(params: table): (any?, string?)
    if type(params.execution_identity_row) == "table" then
        local row = params.execution_identity_row :: table
        local identity, row_err = execution_identity.from_row(row.actor_id, row.actor_context)
        if row_err or not identity then
            return nil, row_err or "invalid execution identity row"
        end
        return execution_identity.to_row(identity)
    end

    if type(params.execution_identity) == "table" then
        return execution_identity.to_row(params.execution_identity)
    end

    if params.system_identity == true then
        return nil, nil
    end

    local frozen, capture_err = execution_identity.capture("projection")
    if capture_err or not frozen then
        return nil, tostring(capture_err or "failed to capture execution identity")
    end
    return execution_identity.to_row(frozen)
end

-- register_projection_tx upserts a projection and its cursor within a caller-
-- owned transaction. Validates params, derives runtime/defaults, reads the
-- thread row, upserts the projection (merging the body's read-model on
-- re-register unless reset=true), and upserts the cursor. No tx management:
-- the caller begins and commits; on error the caller rolls back.
function writer.register_projection_tx(tx: sql.Transaction, dbtype: string, thread_id: string, params: table): (RegisterResult?, error?)
    params = type(params) == "table" and params or {}

    local kind = trim(params.kind)
    if kind == "" then
        return nil, (errors.new({ message = "kind is required", kind = errors.INVALID }) :: error)
    end
    if #kind > types.KIND_MAX_LEN then
        return nil, (errors.new({ message = "kind is too long (max " .. tostring(types.KIND_MAX_LEN) .. ")", kind = errors.INVALID }) :: error)
    end

    local worker_ref = trim(params.worker_ref)
    if worker_ref == "" then
        return nil, (errors.new({ message = "worker_ref is required", kind = errors.INVALID }) :: error)
    end
    -- Runtime is part of worker_ref's canonical wire format, never a second
    -- caller-controlled routing decision.
    local worker_runtime
    if string.sub(worker_ref, 1, #types.WORKER_SCHEME.PROC) == types.WORKER_SCHEME.PROC then
        worker_runtime = types.WORKER_RUNTIME.PROC
    elseif string.sub(worker_ref, 1, #types.WORKER_SCHEME.FUNC) == types.WORKER_SCHEME.FUNC then
        worker_runtime = types.WORKER_RUNTIME.FUNC
    else
        return nil, (errors.new({ message = "worker_ref must use func:// or proc:// scheme", kind = errors.INVALID }) :: error)
    end
    local declared_runtime = trim(params.worker_runtime)
    if declared_runtime ~= "" and declared_runtime ~= worker_runtime then
        return nil, (errors.new({
            message = "worker_runtime conflicts with worker_ref scheme: " .. declared_runtime .. " vs " .. worker_runtime,
            kind = errors.INVALID,
        }) :: error)
    end
    local worker_input_mode = trim(params.worker_input_mode)
    if worker_input_mode == "" then
        worker_input_mode = types.WORKER_INPUT_MODE_DEFAULT
    end
    if worker_input_mode ~= types.WORKER_INPUT_MODE.PREFETCH_EVENTS and worker_input_mode ~= types.WORKER_INPUT_MODE.CURSOR_ONLY then
        return nil, (errors.new({ message = "worker_input_mode must be one of: prefetch_events, cursor_only", kind = errors.INVALID }) :: error)
    end

    local projection_key = trim(params.projection_key)
    if projection_key == "" then
        projection_key = types.KEY_DEFAULT
    end
    local cursor_key = trim(params.cursor_key)
    if cursor_key == "" then
        cursor_key = types.KEY_DEFAULT
    end
    local coalesce_key = trim(params.coalesce_key)
    if coalesce_key == "" then
        coalesce_key = cursor_key
    end
    for _, key in ipairs({ projection_key, cursor_key, coalesce_key }) do
        if #key > types.KEY_MAX_LEN then
            return nil, (errors.new({ message = "projection/cursor/coalesce key is too long (max " .. tostring(types.KEY_MAX_LEN) .. ")", kind = errors.INVALID }) :: error)
        end
    end

    local batch_size = clamp(params.batch_size, types.BATCH_SIZE.MIN, types.BATCH_SIZE.MAX, proj_env.batch_size_default())
    local start_seq = clamp(params.start_seq, types.START_SEQ.MIN, types.START_SEQ.MAX, types.START_SEQ.MIN)
    local reset = params.reset == true

    local window_max_events = clamp(params.window_max_events, 0, types.WINDOW_MAX_EVENTS, proj_env.window_max_events())
    local window_idle_timeout_ms = clamp(params.window_idle_timeout_ms, 0, types.WINDOW_MAX_TIMEOUT_MS, proj_env.window_idle_timeout_ms())
    local window_max_duration_ms = clamp(params.window_max_duration_ms, 0, types.WINDOW_MAX_TIMEOUT_MS, proj_env.window_max_duration_ms())

    local now = now_string(dbtype)
    local lock = dbtype == sql.type.SQLITE and "" or "FOR UPDATE"

    local function fail(message: string, kind: string): (nil, error)
        return nil, (errors.new({ message = message, kind = kind }) :: error)
    end

    local thread_query = sql.builder.select("id", "event_count", "hydration_state", "hydration_final_seq")
        :from("kickside_thread")
        :where(sql.builder.eq({ id = thread_id }))
    if lock ~= "" then
        thread_query = thread_query:suffix(lock)
    end
    local thread_rows, thread_err = thread_query:run_with(tx):query()
    if thread_err then
        return fail("failed to load thread: " .. tostring(thread_err), errors.INTERNAL)
    end
    if not thread_rows or #thread_rows == 0 then
        return fail("thread not found", errors.NOT_FOUND)
    end
    local thread = thread_rows[1]
    local event_count = tonumber(thread.event_count) or 0
    local hydration_state = tostring(thread.hydration_state)
    -- start_at_head positions a new cursor at the thread's current head so the
    -- projection processes only events appended after registration, with no
    -- history replay. It resolves in-tx against the freshly read event_count, so
    -- "head" is race-free; an explicit start_seq still wins.
    if params.start_at_head == true and params.start_seq == nil then
        start_seq = event_count
    end
    if hydration_state ~= core_types.HYDRATION_STATE.HYDRATING and start_seq > event_count then
        return fail("start_seq cannot exceed thread event_count when hydration is not active", errors.INVALID)
    end

    -- target_seq is the bounded backfill ceiling; nil means "tail live to head".
    local target_seq: number? = nil
    if hydration_state == core_types.HYDRATION_STATE.CATCHING_UP then
        target_seq = tonumber(thread.hydration_final_seq) or event_count
    end

    local function cursor_status_for(cursor_head: number): string
        if hydration_state == core_types.HYDRATION_STATE.LIVE and event_count <= cursor_head then
            return types.CURSOR_STATUS.LIVE
        end
        return types.CURSOR_STATUS.PENDING
    end

    local function dispatch_after_for(cursor_head: number): string?
        if hydration_state ~= core_types.HYDRATION_STATE.HYDRATING and event_count > cursor_head then
            return now
        end
        return nil
    end

    -- Freeze the registering actor's bounded identity onto the projection definition
    -- (canonical actor_id + actor_context columns). Deferred worker runs reconstruct
    -- it so they act as the owner who registered the projection. Thread autoinit
    -- passes an explicit row captured at the contract boundary; direct writer calls
    -- capture the ambient frame. System identity must be passed explicitly.
    local actor_id_col: string? = nil
    local actor_context_col: string? = nil
    local id_row, id_row_err = identity_row_from_params(params)
    if id_row_err then
        return fail("invalid execution identity: " .. tostring(id_row_err), errors.INVALID)
    end
    if id_row then
        actor_id_col = (id_row :: any).actor_id :: string?
        actor_context_col = (id_row :: any).actor_context :: string?
    end

    local body = {
        meta = {
            worker_ref = worker_ref,
            worker_runtime = worker_runtime,
            worker_input_mode = worker_input_mode,
        },
    }
    if type(params.worker_config) == "table" then
        body.worker_config = params.worker_config
    end
    if type(params.trigger_policy) == "table" then
        body.trigger_policy = params.trigger_policy
    end
    if type(params.events) == "table" then
        body.events = params.events
    end
    -- drop_notify is a subscriber-declared teardown hook: { thread_id, type, body }.
    -- When the thread this projection lives on is torn down, the threads slice appends
    -- the declared typed event to thread_id so the subscriber surfaces the loss of its
    -- source through its own read model. A declaration field, like worker_config.
    if type(params.drop_notify) == "table" then
        body.drop_notify = params.drop_notify
    end
    local body_json, body_encode_err = json.encode(body)
    if body_encode_err or not body_json then
        return fail("failed to encode projection body: " .. tostring(body_encode_err), errors.INVALID)
    end

    local projection_id: string
    local created_projection = false
    local projection_rows, projection_query_err = sql.builder.select("id", "body")
        :from("kickside_projection")
        :where(sql.builder.eq({ thread_id = thread_id, kind = kind, projection_key = projection_key }))
        :limit(1)
        :run_with(tx):query()
    if projection_query_err then
        return fail("failed to load projection: " .. tostring(projection_query_err), errors.INTERNAL)
    end

    if projection_rows and #projection_rows > 0 then
        projection_id = tostring(projection_rows[1].id)
        -- Preserve the fold read-model that lives alongside the declaration in the
        -- body. The worker folds its accumulated public state (counters, status)
        -- into top-level body keys; only meta / worker_config / trigger_policy /
        -- events are declaration fields. A re-register (binding hot-reload,
        -- ensure_thread convergence) refreshes just those declaration fields and
        -- keeps every other key, so a re-registration never drops the read-model.
        -- reset is the explicit rebuild: it replaces the whole body and rewinds the
        -- cursor so the fold reconstructs the read-model from start_seq.
        local body_to_write = body_json
        local declaration_changed = false
        if not reset then
            local merged, decode_err = decode_body(projection_rows[1].body)
            if decode_err or not merged then
                return fail("failed to decode existing projection body: " .. tostring(decode_err), errors.INTERNAL)
            end
            local stored_declaration = apply_declaration_fields({}, merged :: table)
            declaration_changed = not same_declaration(stored_declaration, apply_declaration_fields({}, body))
            apply_declaration_fields(merged :: table, body)
            local merged_json, merged_err = json.encode(merged)
            if merged_err or not merged_json then
                return fail("failed to encode merged projection body: " .. tostring(merged_err), errors.INVALID)
            end
            body_to_write = merged_json :: string
        end
        local projection_update = sql.builder.update("kickside_projection")
            :set("body", body_to_write)
            :set("actor_id", actor_id_col)
            :set("actor_context", actor_context_col)
            :set("updated_at", now)
        if reset then
            -- reset rewinds the projection to start_seq and marks it pending so
            -- catch-up rebuilds it from scratch.
            projection_update = projection_update:set("state", core_types.PROJECTION_STATE.PENDING):set("last_event_seq", start_seq)
        elseif declaration_changed then
            -- INVALID records a fault in the PREVIOUS declaration (an unresolvable
            -- worker_ref, an unsupported runtime). A new declaration retires that
            -- verdict: the projection returns to pending and resumes from the seq it
            -- already reached, so a corrected module upgrade heals without a replay.
            projection_update = projection_update:set("state", core_types.PROJECTION_STATE.PENDING)
        end
        local _, update_err = projection_update
            :where(sql.builder.eq({ id = projection_id }))
            :run_with(tx):exec()
        if update_err then
            return fail("failed to update projection: " .. tostring(update_err), errors.INTERNAL)
        end
    else
        projection_id = uuid.v7()
        created_projection = true
        local _, insert_err = sql.builder.insert("kickside_projection")
            :set_map({
                id = projection_id,
                thread_id = thread_id,
                kind = kind,
                projection_key = projection_key,
                body = body_json,
                state = core_types.PROJECTION_STATE.PENDING,
                last_event_seq = start_seq,
                actor_id = actor_id_col,
                actor_context = actor_context_col,
                created_at = now,
                updated_at = now,
            })
            :run_with(tx):exec()
        if insert_err then
            if is_unique_error(insert_err) then
                return fail("projection already exists", errors.CONFLICT)
            end
            return fail("failed to insert projection: " .. tostring(insert_err), errors.INTERNAL)
        end
    end

    -- A cursor is dispatchable when the thread already has events past the cursor
    -- head and is not still hydrating. dispatch_after stamps it for the scheduler.
    local cursor_id: string
    local generation = 1
    local cursor_rows, cursor_query_err = sql.builder.select("id", "last_seq", "generation")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({ projection_id = projection_id, cursor_key = cursor_key }))
        :limit(1)
        :run_with(tx):query()
    if cursor_query_err then
        return fail("failed to load cursor: " .. tostring(cursor_query_err), errors.INTERNAL)
    end

    local cursor_head = start_seq
    if cursor_rows and #cursor_rows > 0 then
        cursor_id = tostring(cursor_rows[1].id)
        cursor_head = reset and start_seq or (tonumber(cursor_rows[1].last_seq) or 0)
        generation = (tonumber(cursor_rows[1].generation) or 1) + 1
        local status = cursor_status_for(cursor_head)
        local dispatch_after = dispatch_after_for(cursor_head)
        local _, update_err = clear_cursor_failure(sql.builder.update("kickside_projection_cursor"))
            :set("last_seq", cursor_head)
            :set("target_seq", target_seq)
            :set("batch_size", batch_size)
            :set("status", status)
            :set("generation", generation)
            :set("coalesce_key", coalesce_key)
            :set("dispatch_after", dispatch_after)
            :set("window_max_events", window_max_events)
            :set("window_idle_timeout_ms", window_idle_timeout_ms)
            :set("window_max_duration_ms", window_max_duration_ms)
            :set("window_opened_at_ms", nil)
            :set("window_last_event_at_ms", nil)
            :set("locked_by", nil)
            :set("locked_at", nil)
            -- Denormalize only the runtime onto the cursor so the proc-runner claims
            -- WHERE worker_runtime='proc' without reading the body; the binding itself
            -- (worker_ref) is read from the projection body at dispatch. Clear any prior
            -- dispatch token: a new generation invalidates an in-flight attempt.
            :set("worker_runtime", worker_runtime)
            :set("dispatch_token", nil)
            :set("updated_at", now)
            :where(sql.builder.eq({ id = cursor_id }))
            :run_with(tx):exec()
        if update_err then
            return fail("failed to update cursor: " .. tostring(update_err), errors.INTERNAL)
        end
    else
        cursor_id = uuid.v7()
        local status = cursor_status_for(start_seq)
        local dispatch_after = dispatch_after_for(start_seq)
        local _, insert_err = sql.builder.insert("kickside_projection_cursor")
            :set_map({
                id = cursor_id,
                projection_id = projection_id,
                thread_id = thread_id,
                cursor_key = cursor_key,
                coalesce_key = coalesce_key,
                generation = 1,
                last_seq = start_seq,
                target_seq = target_seq,
                batch_size = batch_size,
                status = status,
                dispatch_after = dispatch_after,
                window_max_events = window_max_events,
                window_idle_timeout_ms = window_idle_timeout_ms,
                window_max_duration_ms = window_max_duration_ms,
                worker_runtime = worker_runtime,
                created_at = now,
                updated_at = now,
            })
            :run_with(tx):exec()
        if insert_err then
            return fail("failed to insert cursor: " .. tostring(insert_err), errors.INTERNAL)
        end
    end

    return {
        projection_id = projection_id,
        cursor_id = cursor_id,
        kind = kind,
        projection_key = projection_key,
        cursor_key = cursor_key,
        coalesce_key = coalesce_key,
        batch_size = batch_size,
        target_seq = target_seq,
        worker_ref = worker_ref,
        worker_runtime = worker_runtime,
        worker_input_mode = worker_input_mode,
        generation = generation,
        created = created_projection,
        window = {
            mode = types.WINDOW_MODE_LIVE_WINDOW,
            max_events = window_max_events,
            idle_timeout_ms = window_idle_timeout_ms,
            max_window_ms = window_max_duration_ms,
        },
    }, nil
end

-- suspend_storage_tx fences every cursor whose projection kind materializes into
-- storage owned by a removable module. It is deliberately transaction-scoped:
-- migration rollback also rolls this fence back when the destructive DDL fails.
-- Bumping generation makes any worker claimed before the migration unable to
-- commit after the storage has changed.
function writer.suspend_storage_tx(tx: sql.Transaction, dbtype: string, kind: string): (table?, error?)
    kind = trim(kind)
    if kind == "" then
        return nil, (errors.new({ message = "projection storage kind is required", kind = errors.INVALID }) :: error)
    end

    local rows, query_err = sql.builder.select("id")
        :from("kickside_projection")
        :where(sql.builder.eq({ kind = kind }))
        :run_with(tx):query()
    if query_err then
        return nil, (errors.new({ message = "failed to load projection storage cursors: " .. tostring(query_err), kind = errors.INTERNAL }) :: error)
    end

    local now = now_string(dbtype)
    local projections = 0
    for _, row in ipairs(rows or {}) do
        local projection_id = tostring(row.id)
        local _, cursor_err = sql.builder.update("kickside_projection_cursor")
            :set("status", types.CURSOR_STATUS.PAUSED)
            :set("generation", sql.builder.expr("generation + 1"))
            :set("target_seq", nil)
            :set("dispatch_after", nil)
            :set("window_opened_at_ms", nil)
            :set("window_last_event_at_ms", nil)
            :set("last_error", nil)
            :set("locked_by", nil)
            :set("locked_at", nil)
            :set("dispatch_token", nil)
            :set("updated_at", now)
            :where(sql.builder.eq({ projection_id = projection_id }))
            :run_with(tx):exec()
        if cursor_err then
            return nil, (errors.new({ message = "failed to suspend projection storage: " .. tostring(cursor_err), kind = errors.INTERNAL }) :: error)
        end
        local _, projection_err = sql.builder.update("kickside_projection")
            :set("state", core_types.PROJECTION_STATE.PENDING)
            :set("updated_at", now)
            :where(sql.builder.eq({ id = projection_id }))
            :run_with(tx):exec()
        if projection_err then
            return nil, (errors.new({ message = "failed to mark projection storage pending: " .. tostring(projection_err), kind = errors.INTERNAL }) :: error)
        end
        projections = projections + 1
    end
    return { kind = kind, projections = projections, status = types.CURSOR_STATUS.PAUSED }, nil
end

-- rebuild_storage_tx invalidates only derived state. The authoritative thread
-- and its events remain untouched; declaration fields remain on the projection;
-- every cursor rewinds to sequence zero and is re-armed only after the complete
-- storage migration chain is ready. This is the durable reinstall contract for
-- any externally materialized projection, not a module-specific repair.
function writer.rebuild_storage_tx(tx: sql.Transaction, dbtype: string, kind: string): (table?, error?)
    kind = trim(kind)
    if kind == "" then
        return nil, (errors.new({ message = "projection storage kind is required", kind = errors.INVALID }) :: error)
    end

    local rows, query_err = sql.builder.select("id", "thread_id", "body")
        :from("kickside_projection")
        :where(sql.builder.eq({ kind = kind }))
        :run_with(tx):query()
    if query_err then
        return nil, (errors.new({ message = "failed to load projection storage: " .. tostring(query_err), kind = errors.INTERNAL }) :: error)
    end

    local now = now_string(dbtype)
    local projections = 0
    for _, row in ipairs(rows or {}) do
        local projection_id = tostring(row.id)
        local thread_id = tostring(row.thread_id)
        local declaration, declaration_err = declaration_only(row.body)
        if declaration_err or not declaration then
            return nil, (errors.new({ message = "failed to decode projection storage declaration: " .. tostring(declaration_err), kind = errors.INTERNAL }) :: error)
        end
        local body_json, encode_err = json.encode(declaration)
        if encode_err or not body_json then
            return nil, (errors.new({ message = "failed to encode projection storage declaration: " .. tostring(encode_err), kind = errors.INTERNAL }) :: error)
        end

        local thread_rows, thread_err = sql.builder.select("event_count", "hydration_state", "hydration_final_seq")
            :from("kickside_thread")
            :where(sql.builder.eq({ id = thread_id }))
            :limit(1)
            :run_with(tx):query()
        if thread_err then
            return nil, (errors.new({ message = "failed to load projection storage thread: " .. tostring(thread_err), kind = errors.INTERNAL }) :: error)
        end
        if not thread_rows or #thread_rows == 0 then
            return nil, (errors.new({ message = "projection storage thread not found", kind = errors.NOT_FOUND }) :: error)
        end

        local thread = thread_rows[1]
        local event_count = tonumber(thread.event_count) or 0
        local hydration_state = tostring(thread.hydration_state)
        local cursor_status = types.CURSOR_STATUS.PENDING
        if hydration_state == core_types.HYDRATION_STATE.LIVE and event_count == 0 then
            cursor_status = types.CURSOR_STATUS.LIVE
        end
        local target_seq: number? = nil
        if hydration_state == core_types.HYDRATION_STATE.CATCHING_UP then
            target_seq = tonumber(thread.hydration_final_seq) or event_count
        end
        local dispatch_after: string? = nil
        if hydration_state ~= core_types.HYDRATION_STATE.HYDRATING and event_count > 0 then
            dispatch_after = now
        end

        local _, projection_err = sql.builder.update("kickside_projection")
            :set("body", body_json)
            :set("state", core_types.PROJECTION_STATE.PENDING)
            :set("last_event_seq", 0)
            :set("updated_at", now)
            :where(sql.builder.eq({ id = projection_id }))
            :run_with(tx):exec()
        if projection_err then
            return nil, (errors.new({ message = "failed to reset projection storage: " .. tostring(projection_err), kind = errors.INTERNAL }) :: error)
        end

        local _, cursor_err = clear_cursor_failure(sql.builder.update("kickside_projection_cursor"))
            :set("last_seq", 0)
            :set("target_seq", target_seq)
            :set("status", cursor_status)
            :set("generation", sql.builder.expr("generation + 1"))
            :set("dispatch_after", dispatch_after)
            :set("window_opened_at_ms", nil)
            :set("window_last_event_at_ms", nil)
            :set("locked_by", nil)
            :set("locked_at", nil)
            :set("trigger_state", nil)
            :set("dispatch_token", nil)
            :set("updated_at", now)
            :where(sql.builder.eq({ projection_id = projection_id }))
            :run_with(tx):exec()
        if cursor_err then
            return nil, (errors.new({ message = "failed to rebuild projection storage cursor: " .. tostring(cursor_err), kind = errors.INTERNAL }) :: error)
        end
        projections = projections + 1
    end
    return { kind = kind, projections = projections, status = "rebuilding" }, nil
end

-- register_projection upserts a projection and its cursor, opening and
-- committing its own transaction off db. On error it rolls back and returns
-- the error. Delegates to register_projection_tx for the core SQL.
function writer.register_projection(db: sql.DB, thread_id: string, params: table): (RegisterResult?, error?)
    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local tx, tx_err = db:begin()
    if tx_err then
        return nil, (errors.new({ message = "failed to begin transaction: " .. tostring(tx_err), kind = errors.INTERNAL }) :: error)
    end
    local result, err = writer.register_projection_tx(tx, dbtype, thread_id, params)
    if err then
        tx:rollback()
        return nil, err
    end
    local _, commit_err = tx:commit()
    if commit_err then
        return nil, (errors.new({ message = "failed to commit projection registration: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    return result, nil
end

-- reset_thread_cursors resets all cursors for a thread to pending with cleared
-- window/target state. Called by hydration.start to prepare cursors for re-run.
function writer.reset_thread_cursors(tx: sql.Transaction, thread_id: string, now: string): error?
    local _, err = clear_cursor_failure(sql.builder.update("kickside_projection_cursor"))
        :set("status", types.CURSOR_STATUS.PENDING)
        :set("target_seq", nil)
        :set("dispatch_after", nil)
        :set("window_opened_at_ms", nil)
        :set("window_last_event_at_ms", nil)
        :set("updated_at", now)
        :where(sql.builder.eq({ thread_id = thread_id }))
        :run_with(tx):exec()
    if err then
        return (errors.new({ message = "failed to reset cursors: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    return nil
end

-- count_thread_cursors returns the total cursor count for a thread. Used by
-- hydration.finalize to report how many cursors were armed.
function writer.count_thread_cursors(tx: sql.Transaction, thread_id: string): (integer, error?)
    local rows, err = sql.builder.select("COUNT(*) AS count")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({ thread_id = thread_id }))
        :run_with(tx):query()
    if err then
        return 0, (errors.new({ message = "failed to count cursors: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    return (tonumber(rows and rows[1] and rows[1].count) or 0) :: integer, nil
end

-- arm_thread_cursors sets target_seq and dispatch_after on all cursors for a
-- thread, marking them pending and due. Called by hydration.finalize.
function writer.arm_thread_cursors(tx: sql.Transaction, thread_id: string, final_seq: number, now: string): error?
    local _, err = clear_cursor_failure(sql.builder.update("kickside_projection_cursor"))
        :set("target_seq", final_seq)
        :set("status", types.CURSOR_STATUS.PENDING)
        :set("dispatch_after", now)
        :set("window_opened_at_ms", nil)
        :set("window_last_event_at_ms", nil)
        :set("updated_at", now)
        :where(sql.builder.eq({ thread_id = thread_id }))
        :run_with(tx):exec()
    if err then
        return (errors.new({ message = "failed to arm cursors: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    return nil
end

-- count_not_caught_up_cursors returns the count of cursors for a thread whose
-- status is not caught_up or live. Used by hydration.activate to gate the
-- live transition.
function writer.count_not_caught_up_cursors(tx: sql.Transaction, thread_id: string): (integer, error?)
    local rows, err = sql.builder.select("COUNT(*) AS count")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({ thread_id = thread_id }))
        :where(sql.builder.expr("status NOT IN ('caught_up', 'live')"))
        :run_with(tx):query()
    if err then
        return 0, (errors.new({ message = "failed to count pending cursors: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    return (tonumber(rows and rows[1] and rows[1].count) or 0) :: integer, nil
end

-- promote_caught_up_cursors flips caught_up cursors to live and clears their
-- target/window state. Called by hydration.activate on the live transition.
-- Returns the number of cursors promoted.
function writer.promote_caught_up_cursors(tx: sql.Transaction, thread_id: string, now: string): (integer, error?)
    local result, err = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.LIVE)
        :set("target_seq", nil)
        :set("dispatch_after", nil)
        :set("window_opened_at_ms", nil)
        :set("window_last_event_at_ms", nil)
        :set("updated_at", now)
        :where(sql.builder.eq({ thread_id = thread_id }))
        :where(sql.builder.eq({ status = types.CURSOR_STATUS.CAUGHT_UP }))
        :run_with(tx):exec()
    if err then
        return 0, (errors.new({ message = "failed to flip cursors live: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    return (tonumber(result and result.rows_affected) or 0) :: integer, nil
end

-- remove_projection deletes a projection (addressed by id or by kind +
-- projection_key) and cascades its cursors. Returns the removed projection's
-- identity and the cursor count.
function writer.remove_projection(db: sql.DB, thread_id: string, params: table): (table?, error?)
    params = type(params) == "table" and params or {}

    local projection_id = trim(params.projection_id or params.id)
    local kind = trim(params.kind)
    local projection_key = trim(params.projection_key)
    if projection_key == "" then
        projection_key = types.KEY_DEFAULT
    end
    if projection_id == "" and kind == "" then
        return nil, (errors.new({ message = "projection_id or kind is required", kind = errors.INVALID }) :: error)
    end

    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local lock = tostring(dbtype_raw) == sql.type.SQLITE and "" or "FOR UPDATE"

    local tx, tx_err = db:begin()
    if tx_err then
        return nil, (errors.new({ message = "failed to begin transaction: " .. tostring(tx_err), kind = errors.INTERNAL }) :: error)
    end

    local function fail(message: string, kind_str: string): (nil, error)
        tx:rollback()
        return nil, (errors.new({ message = message, kind = kind_str }) :: error)
    end

    local projection_select = sql.builder.select("id", "kind", "projection_key")
        :from("kickside_projection")
    if projection_id ~= "" then
        projection_select = projection_select:where(sql.builder.eq({ id = projection_id, thread_id = thread_id }))
    else
        projection_select = projection_select:where(sql.builder.eq({ thread_id = thread_id, kind = kind, projection_key = projection_key }))
    end
    if lock ~= "" then
        projection_select = projection_select:suffix(lock)
    end
    local projection_rows, projection_err = projection_select:run_with(tx):query()
    if projection_err then
        return fail("failed to load projection: " .. tostring(projection_err), errors.INTERNAL)
    end
    if not projection_rows or #projection_rows == 0 then
        return fail("projection not found", errors.NOT_FOUND)
    end
    local projection = projection_rows[1]

    local cursor_result, cursor_delete_err = sql.builder.delete("kickside_projection_cursor")
        :where(sql.builder.eq({ projection_id = projection.id, thread_id = thread_id }))
        :run_with(tx):exec()
    if cursor_delete_err then
        return fail("failed to delete cursors: " .. tostring(cursor_delete_err), errors.INTERNAL)
    end

    local _, projection_delete_err = sql.builder.delete("kickside_projection")
        :where(sql.builder.eq({ id = projection.id, thread_id = thread_id }))
        :run_with(tx):exec()
    if projection_delete_err then
        return fail("failed to delete projection: " .. tostring(projection_delete_err), errors.INTERNAL)
    end

    local _, commit_err = tx:commit()
    if commit_err then
        return nil, (errors.new({ message = "failed to commit projection removal: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end

    return {
        projection_id = projection.id,
        kind = projection.kind,
        projection_key = projection.projection_key,
        removed_cursors = tonumber(cursor_result and cursor_result.rows_affected) or 0,
    }, nil
end

-- resolve_cursor loads a cursor for a thread inside tx, by id or by natural
-- address, taking a row lock. Shared by pause/resume.
local function resolve_cursor(tx: sql.Transaction, thread_id: string, params: table, lock: string): (table?, error?)
    local cursor_id = trim(params.cursor_id)
    if cursor_id ~= "" then
        local query = sql.builder.select("id", "projection_id", "thread_id", "status", "generation", "last_error")
            :from("kickside_projection_cursor")
            :where(sql.builder.eq({ id = cursor_id, thread_id = thread_id }))
        if lock ~= "" then
            query = query:suffix(lock)
        end
        local rows, q_err = query:run_with(tx):query()
        if q_err then
            return nil, (errors.new({ message = "failed to load cursor: " .. tostring(q_err), kind = errors.INTERNAL }) :: error)
        end
        if not rows or #rows == 0 then
            return nil, (errors.new({ message = "cursor not found", kind = errors.NOT_FOUND }) :: error)
        end
        return rows[1], nil
    end

    -- Resolve by projection_id: the single durable handle a caller holds after
    -- register (remove_projection resolves the same way). A trigger projection has
    -- one cursor (default cursor_key), so this identifies it without the kind + key
    -- triple. This is the handle the trigger pause/resume path passes.
    local projection_id = trim(params.projection_id)
    if projection_id ~= "" then
        local query = sql.builder.select("id", "projection_id", "thread_id", "status", "generation", "last_error")
            :from("kickside_projection_cursor")
            :where(sql.builder.eq({ projection_id = projection_id, thread_id = thread_id }))
        if lock ~= "" then
            query = query:suffix(lock)
        end
        local rows, q_err = query:run_with(tx):query()
        if q_err then
            return nil, (errors.new({ message = "failed to load cursor: " .. tostring(q_err), kind = errors.INTERNAL }) :: error)
        end
        if not rows or #rows == 0 then
            return nil, (errors.new({ message = "cursor not found", kind = errors.NOT_FOUND }) :: error)
        end
        return rows[1], nil
    end

    local kind = trim(params.kind)
    if kind == "" then
        return nil, (errors.new({ message = "projection_id, kind, or cursor_id is required", kind = errors.INVALID }) :: error)
    end
    local projection_key = trim(params.projection_key)
    if projection_key == "" then projection_key = types.KEY_DEFAULT end
    local cursor_key = trim(params.cursor_key)
    if cursor_key == "" then cursor_key = types.KEY_DEFAULT end
    local query = sql.builder.select("c.id", "c.projection_id", "c.thread_id", "c.status", "c.generation", "c.last_error")
        :from("kickside_projection_cursor c")
        :inner_join("kickside_projection p ON p.id = c.projection_id")
        :where("c.thread_id = ?", thread_id)
        :where("p.kind = ?", kind)
        :where("p.projection_key = ?", projection_key)
        :where("c.cursor_key = ?", cursor_key)
    if lock ~= "" then
        query = query:suffix(lock)
    end
    local rows, q_err = query:run_with(tx):query()
    if q_err then
        return nil, (errors.new({ message = "failed to load cursor: " .. tostring(q_err), kind = errors.INTERNAL }) :: error)
    end
    if not rows or #rows == 0 then
        return nil, (errors.new({ message = "cursor not found", kind = errors.NOT_FOUND }) :: error)
    end
    return rows[1], nil
end

local function recoverable_invalid_cursor(tx: sql.Transaction, cursor: table): (boolean, error?)
    if tostring(cursor.status) ~= types.CURSOR_STATUS.INVALID then
        return false, nil
    end
    local last_error = tostring(cursor.last_error or "")
    local dispatch_orphan = last_error == RECOVERABLE_INVALID_ERROR
    local rows, err = sql.builder.select("c.id", "p.state AS projection_state", "p.body AS projection_body",
            "t.id AS joined_thread_id", "p.id AS joined_projection_id")
        :from("kickside_projection_cursor c")
        :left_join("kickside_thread t ON t.id = c.thread_id")
        :left_join("kickside_projection p ON p.id = c.projection_id")
        :where("c.id = ?", cursor.id)
        :limit(1)
        :run_with(tx):query()
    if err then
        return false, (errors.new({ message = "failed to validate invalid cursor recovery: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    if not rows or #rows == 0 then
        return false, nil
    end
    local row = rows[1]
    if row.joined_thread_id == nil or row.joined_projection_id == nil then
        return false, nil
    end
    if dispatch_orphan then
        return tostring(row.projection_state) ~= core_types.PROJECTION_STATE.INVALID, nil
    end
    -- Any other invalid cursor -- a lost worker target or attempts exhausted on
    -- an external fault -- recovers exactly when its declared binding still
    -- resolves. The projection body is authoritative for the binding, so
    -- re-validate it: an explicit resume then re-enters the normal attempt
    -- cycle, and a fault that persists just dead-letters again.
    local body = select(1, decode_body(row.projection_body))
    local meta = body and type((body :: table).meta) == "table" and (body :: table).meta or {}
    local ref = tostring((meta :: table).worker_ref or "")
    if ref == "" then
        return false, nil
    end
    local binding = select(1, proj_worker.parse(ref, true))
    if not binding then
        return false, nil
    end
    return select(1, proj_worker.validate(binding :: table)) ~= nil, nil
end

-- begin_for resolves the dialect and opens a transaction, returning the now
-- string and row-lock clause alongside it.
local function begin_for(db: sql.DB): (sql.Transaction?, string?, string?, error?)
    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, nil, nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local tx, tx_err = db:begin()
    if tx_err then
        return nil, nil, nil, (errors.new({ message = "failed to begin transaction: " .. tostring(tx_err), kind = errors.INTERNAL }) :: error)
    end
    return tx, now_string(dbtype), dbtype == sql.type.SQLITE and "" or "FOR UPDATE", nil
end

-- pause halts a cursor: it stops being dispatched and any in-flight lease is
-- cleared. Re-pausing an already-paused cursor is idempotent.
function writer.pause(db: sql.DB, thread_id: string, params: table): (table?, error?)
    params = type(params) == "table" and params or {}
    local tx, now, lock, begin_err = begin_for(db)
    if begin_err then
        return nil, begin_err
    end
    local _tx = tx :: sql.Transaction

    local cursor, cursor_err = resolve_cursor(_tx, thread_id, params, lock :: string)
    if cursor_err then
        _tx:rollback()
        return nil, cursor_err
    end
    local _cursor = cursor :: table

    if _cursor.status == types.CURSOR_STATUS.PAUSED then
        _tx:rollback()
        return { cursor_id = _cursor.id, projection_id = _cursor.projection_id, status = types.CURSOR_STATUS.PAUSED, already_paused = true }, nil
    end

    local _, update_err = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.PAUSED)
        :set("dispatch_after", nil)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = _cursor.id }))
        :run_with(_tx):exec()
    if update_err then
        _tx:rollback()
        return nil, (errors.new({ message = "failed to pause cursor: " .. tostring(update_err), kind = errors.INTERNAL }) :: error)
    end

    local _, commit_err = _tx:commit()
    if commit_err then
        return nil, (errors.new({ message = "failed to commit pause: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    return { cursor_id = _cursor.id, projection_id = _cursor.projection_id, status = types.CURSOR_STATUS.PAUSED }, nil
end

-- resume reactivates a paused, failed, or recoverable dispatcher-invalidated
-- cursor: back to pending with an immediate dispatch. Other invalid statuses are
-- permanent projection/config faults and remain conflicts.
function writer.resume(db: sql.DB, thread_id: string, params: table): (table?, error?)
    params = type(params) == "table" and params or {}
    local tx, now, lock, begin_err = begin_for(db)
    if begin_err then
        return nil, begin_err
    end
    local _tx = tx :: sql.Transaction

    local cursor, cursor_err = resolve_cursor(_tx, thread_id, params, lock :: string)
    if cursor_err then
        _tx:rollback()
        return nil, cursor_err
    end
    local _cursor = cursor :: table

    local recoverable_invalid = false
    if _cursor.status == types.CURSOR_STATUS.INVALID then
        local ok, recovery_err = recoverable_invalid_cursor(_tx, _cursor)
        if recovery_err then
            _tx:rollback()
            return nil, recovery_err
        end
        recoverable_invalid = ok
    end
    if _cursor.status ~= types.CURSOR_STATUS.PAUSED
        and _cursor.status ~= types.CURSOR_STATUS.FAILED
        and not recoverable_invalid then
        _tx:rollback()
        return nil, (errors.new({ message = "cursor is not paused, failed, or recoverable invalid (status: " .. tostring(_cursor.status) .. ")", kind = errors.CONFLICT }) :: error)
    end

    -- The attempt budget starts over with the resume: a cursor that kept its
    -- exhausted count would dead-letter again on the next claim without ever
    -- invoking the worker.
    local _, update_err = clear_cursor_failure(sql.builder.update("kickside_projection_cursor"))
        :set("status", types.CURSOR_STATUS.PENDING)
        :set("dispatch_after", now)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = _cursor.id }))
        :run_with(_tx):exec()
    if update_err then
        _tx:rollback()
        return nil, (errors.new({ message = "failed to resume cursor: " .. tostring(update_err), kind = errors.INTERNAL }) :: error)
    end

    -- Invalidation marks the projection alongside the cursor; a recovered cursor
    -- takes its projection back with it, or the runner refuses to dispatch it.
    if recoverable_invalid then
        local _, state_err = sql.builder.update("kickside_projection")
            :set("state", core_types.PROJECTION_STATE.VALID)
            :set("updated_at", now)
            :where(sql.builder.eq({ id = _cursor.projection_id, state = core_types.PROJECTION_STATE.INVALID }))
            :run_with(_tx):exec()
        if state_err then
            _tx:rollback()
            return nil, (errors.new({ message = "failed to restore projection state on resume: " .. tostring(state_err), kind = errors.INTERNAL }) :: error)
        end
    end

    local _, commit_err = _tx:commit()
    if commit_err then
        return nil, (errors.new({ message = "failed to commit resume: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    return { cursor_id = _cursor.id, projection_id = _cursor.projection_id, status = types.CURSOR_STATUS.PENDING }, nil
end

-- declaration_body_for builds the declaration half of a projection body from a
-- kind's ProjectionBinding, in the same shape register writes it, so a stored copy
-- and a fresh declaration compare field for field.
local function declaration_body_for(p: table): table
    local worker_ref = trim(p.worker_ref)
    local worker_runtime = types.WORKER_RUNTIME.FUNC
    if string.sub(worker_ref, 1, #types.WORKER_SCHEME.PROC) == types.WORKER_SCHEME.PROC then
        worker_runtime = types.WORKER_RUNTIME.PROC
    end
    local worker_input_mode = trim(p.input_mode)
    if worker_input_mode == "" then
        worker_input_mode = types.WORKER_INPUT_MODE_DEFAULT
    end
    local declaration: table = {
        meta = {
            worker_ref = worker_ref,
            worker_runtime = worker_runtime,
            worker_input_mode = worker_input_mode,
        },
    }
    if type(p.worker_config) == "table" then declaration.worker_config = p.worker_config end
    if type(p.trigger_policy) == "table" then declaration.trigger_policy = p.trigger_policy end
    if type(p.events) == "table" then declaration.events = p.events end
    if type(p.drop_notify) == "table" then declaration.drop_notify = p.drop_notify end
    return declaration
end

-- apply_declaration rewrites one projection's declaration fields in place and lets
-- its cursor run again. It never touches the fold read-model, the owner identity,
-- or the seq the cursor reached: only the declaration and the verdict recorded
-- against the superseded one.
function writer.apply_declaration(db: sql.DB, projection_id: string, stored_body: table, declaration: table): (boolean, error?)
    local tx, now, _lock, begin_err = begin_for(db)
    if begin_err or not tx then return false, begin_err end
    local _tx = tx :: sql.Transaction

    local merged = apply_declaration_fields(stored_body, declaration)
    local body_json, encode_err = json.encode(merged)
    if encode_err or not body_json then
        _tx:rollback()
        return false, (errors.new({ message = "failed to encode reconciled projection body: " .. tostring(encode_err), kind = errors.INVALID }) :: error)
    end

    local _, update_err = sql.builder.update("kickside_projection")
        :set("body", body_json)
        :set("state", core_types.PROJECTION_STATE.PENDING)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = projection_id }))
        :run_with(_tx):exec()
    if update_err then
        _tx:rollback()
        return false, (errors.new({ message = "failed to write reconciled projection: " .. tostring(update_err), kind = errors.INTERNAL }) :: error)
    end

    local worker_runtime = types.WORKER_RUNTIME.FUNC
    local meta = merged.meta
    if type(meta) == "table" then
        worker_runtime = trim((meta :: table).worker_runtime)
        if worker_runtime == "" then worker_runtime = types.WORKER_RUNTIME.FUNC end
    end

    -- The cursor keeps last_seq: the events it already folded stay folded. A new
    -- generation retires any in-flight dispatch token from the previous binding.
    local _, cursor_err = clear_cursor_failure(sql.builder.update("kickside_projection_cursor"))
        :set("status", types.CURSOR_STATUS.PENDING)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("dispatch_after", now)
        :set("worker_runtime", worker_runtime)
        :set("generation", sql.builder.expr("generation + 1"))
        :set("updated_at", now)
        :where(sql.builder.eq({ projection_id = projection_id }))
        :run_with(_tx):exec()
    if cursor_err then
        _tx:rollback()
        return false, (errors.new({ message = "failed to revive reconciled cursor: " .. tostring(cursor_err), kind = errors.INTERNAL }) :: error)
    end

    local _, commit_err = _tx:commit()
    if commit_err then
        return false, (errors.new({ message = "failed to commit reconciled projection: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    return true, nil
end

-- reconcile_declarations re-materializes stored projection declarations from the
-- component kind that declares them.
--
-- A projection row is derived state: register copies the kind's declaration into
-- the body, and the runner reads the binding from that copy at dispatch. A module
-- upgrade that renames a worker (or changes any declaration field) leaves the copy
-- behind, and the runner then resolves a target that no longer exists and records
-- a permanent config fault -- against a declaration that is, in fact, correct. The
-- declaration is the source of truth, so it is re-read at boot and any drifted copy
-- is rewritten in place: the fold read-model and the owner identity are untouched,
-- the cursor keeps the seq it reached, and a fault recorded against the superseded
-- declaration is retired.
-- component_impl_map loads component_id -> impl_id. The map is joined in Lua:
-- kickside_components.component_id is a UUID column while
-- kickside_thread.component_id is TEXT (componentless threads hold ''), so a
-- SQL join between them is a type error on Postgres.
local function component_impl_map(db: sql.DB): ({ [string]: string }?, error?)
    local components, component_err = sql.builder
        .select("component_id", "impl_id")
        :from("kickside_components")
        :run_with(db):query()
    if component_err then
        return nil, (errors.new({ message = "failed to load components for reconciliation: " .. tostring(component_err), kind = errors.INTERNAL }) :: error)
    end
    local impl_by_component: { [string]: string } = {}
    for _, row in ipairs(components or {}) do
        impl_by_component[tostring(row.component_id)] = tostring(row.impl_id or "")
    end
    return impl_by_component, nil
end

function writer.reconcile_declarations(db: sql.DB): (table, error?)
    local summary = { checked = 0, reconciled = 0 }

    local impl_by_component, map_err = component_impl_map(db)
    if map_err or not impl_by_component then return summary, map_err end

    local rows, query_err = sql.builder
        .select("p.id AS projection_id", "p.kind AS kind", "p.body AS body", "p.state AS state", "t.component_id AS component_id")
        :from("kickside_projection p")
        :join("kickside_thread t ON t.id = p.thread_id")
        :run_with(db):query()
    if query_err then
        return summary, (errors.new({ message = "failed to load projections for reconciliation: " .. tostring(query_err), kind = errors.INTERNAL }) :: error)
    end

    for _, row in ipairs(rows or {}) do
        summary.checked = summary.checked + 1
        local declared = autoinit.projections_for_impl(impl_by_component[tostring(row.component_id)] or "")
        for _, p in ipairs(declared) do
            if tostring(p.kind) == tostring(row.kind) then
                local stored, decode_err = decode_body(row.body)
                if not decode_err and stored then
                    local declaration = declaration_body_for(p :: table)
                    local current = apply_declaration_fields({}, stored :: table)
                    if not same_declaration(current, declaration) then
                        local applied, apply_err = writer.apply_declaration(db, tostring(row.projection_id), stored :: table, declaration)
                        if apply_err then return summary, apply_err end
                        if applied then summary.reconciled = summary.reconciled + 1 end
                    end
                end
                break
            end
        end
    end

    -- Drift repair only: a projection with NO row for a declared kind is
    -- registered by provision.converge at lifecycle boot/install, under the
    -- component owner's identity.
    return summary, nil
end


return writer
