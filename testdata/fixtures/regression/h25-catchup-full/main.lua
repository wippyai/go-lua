-- The projection catch-up engine. One tick advances a single cursor over its
-- thread's event log: it loads the cursor + projection + thread under a row
-- lock, validates the generation (a re-register bumps it, so a tick for the old
-- generation is recognised as stale and dropped), and either parks the cursor as
-- caught up / live when there is nothing past its head, or fetches the next
-- event batch, invokes the projection's bound worker, and advances the cursor +
-- projection. The caller (the scheduler/jobs slice) re-ticks while a tick
-- reports done=false. This module owns no scheduling or thread notifications,
-- only the projection/cursor state transition.
--
-- func:// workers run inline (catchup.tick). proc:// workers are long and
-- side-effecting, so they run on the dedicated runner pool: reserve_proc/
-- claim_proc reserve a due cursor under a fencing token, the runner executes the
-- worker outside any transaction, and apply_proc persists the result under the
-- fence (token + generation + lease + last_seq must all still match). The
-- load-through-build-worker_input prepare phase is shared by both paths.

local sql = require("sql")
local json = require("json")
local clock = require("clock")
local time = require("time")
local uuid = require("uuid")
local types = require("types")
local core_types = require("core_types")
local worker = require("worker")
local proj_env = require("proj_env")
local threads_writer = require("threads_writer")
local threads_notify = require("threads_notify")
local trace_context = require("trace_context")

local catchup = {}

-- The result one tick returns.
type TickResult = {
    cursor_id: string,
    status: string,
    processed: number,
    last_seq: number,
    target_seq: number,
    done: boolean,
    stale: boolean?,
    obsolete: boolean?,
    skipped: boolean?,
    reason: string?,
    error: string?,
}

-- The fencing context a reserved proc dispatch carries from reserve/claim through
-- to apply. It pins the exact attempt that may write: only an apply whose token,
-- generation, lease holder and last_seq all still match the row advances it.
type ApplyCtx = {
    cursor_id: string,
    projection_id: string,
    thread_id: string,
    generation: number,
    dispatch_token: string,
    runner_id: string,
    previous_last_seq: number,
    new_last: number,
    target_seq: number,
    hydration_state: string,
    explicit_target_seq: number?,
    binding: table,
    input_mode: string,
    body: table,
    trace_context: table?,
}

-- What reserve/claim hands the runner: the worker envelope to execute plus the
-- fencing context to apply with. terminal is set instead when the prepare phase
-- reached a no-work/park/invalid/stale outcome and there is nothing to run.
type Reservation = {
    worker_input: table,
    apply_ctx: ApplyCtx,
}

local function trim(value: any): string
    if type(value) ~= "string" then
        return ""
    end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local now_string = clock.now

-- The alias both claim SELECTs give the cursor table, so a claim's row lock can
-- name it (FOR UPDATE OF c) and lock only the cursor row.
local CURSOR_ALIAS = "c"

-- row_lock is the single source of the cursor SELECT row-lock suffix, chosen by
-- intent so the two access patterns never diverge:
--
--   "claim" — the proc claim SCAN that selects a cursor out of the due pool
--   (WHERE status IN (...) ORDER BY dispatch_after LIMIT 1). It MUST NOT block on
--   a peer's in-flight claim: two claimers blocking on each other in plan-dependent
--   order is the 40P01 deadlock that dead-lettered healthy cursors after a restart's
--   thundering herd. FOR UPDATE OF c locks ONLY the cursor row and SKIP LOCKED
--   yields a cursor a peer is already claiming instead of waiting on it.
--
--   "acquire" — the prepare load of ONE already-resolved cursor (WHERE c.id = ?),
--   which the join with projection + thread also touches. Locks ONLY the cursor
--   row (FOR UPDATE OF c — never the joined rows, so it cannot re-enter the claim
--   deadlock) but BLOCKS rather than skips: this load runs the synchronous
--   catch_up path too, where the caller needs THIS cursor and must wait for a
--   concurrent runner's in-flight lock, not read the locked row as absent (a
--   SKIP LOCKED miss here surfaces as a false cursor_not_found). A single-row wait
--   on the already-distributed cursor cannot cycle.
--
--   "fence" — a re-read of ONE cursor row this runner already owns via
--   locked_by/dispatch_token/generation, to re-verify the fence before applying
--   or releasing. Only the owning runner touches a fenced cursor, so a plain
--   blocking FOR UPDATE is correct and cannot deadlock (single uncontended row).
--
-- SQLite serializes writers, so it needs no row lock for any intent.
local function row_lock(dbtype: string, intent: string): string
    if dbtype == sql.type.SQLITE then
        return ""
    end
    if intent == "claim" then
        return "FOR UPDATE OF " .. CURSOR_ALIAS .. " SKIP LOCKED"
    end
    if intent == "acquire" then
        return "FOR UPDATE OF " .. CURSOR_ALIAS
    end
    return "FOR UPDATE"
end
catchup.row_lock = row_lock

-- Decode a projection body JSON string into a table. An absent body (nil / empty
-- string) is a legitimately empty body ({}, no error). A present-but-undecodable
-- body is corruption: it returns the decode failure so the caller surfaces it
-- instead of laundering a corrupt body into an empty one (which then reads as a
-- benign "worker_ref is missing").
local function decode_body(raw: any): (table, error?)
    if type(raw) ~= "string" or raw == "" then
        return {}, nil
    end
    local decoded, err = json.decode(raw)
    if err or type(decoded) ~= "table" then
        return {}, (errors.new({ message = "failed to decode projection body: " .. tostring(err or "not a JSON object"), kind = errors.INTERNAL }) :: error)
    end
    return decoded :: table, nil
end

local function copy_event_for_append(raw_event: any, write_index: integer, event_index: integer): (table?, error?)
    if type(raw_event) ~= "table" then
        return nil, (errors.new({ message = "append_events[" .. tostring(write_index) .. "].events[" .. tostring(event_index) .. "] must be an object", kind = errors.INVALID }) :: error)
    end

    local event: table = {}
    for key, value in pairs(raw_event :: table) do
        event[key] = value
    end

    if type(event.body_json) ~= "string" or event.body_json == "" then
        local body_json, body_err = json.encode(event.body or {})
        if body_err or type(body_json) ~= "string" then
            return nil, (errors.new({ message = "failed to encode append event body: " .. tostring(body_err), kind = errors.INVALID }) :: error)
        end
        event.body_json = body_json
    end
    event.body = nil
    return event, nil
end

local function event_filter_set(event_types: { string }?): { [string]: boolean }?
    if type(event_types) ~= "table" or #event_types == 0 then return nil end
    local set: { [string]: boolean } = table.create(0, #event_types)
    for _, event_type in ipairs(event_types :: { string }) do
        local t = trim(event_type)
        if t ~= "" then set[t] = true end
    end
    return set
end

local function validate_source_append(
    source_thread_id: string?,
    input_types: { [string]: boolean }?,
    write_thread_id: string,
    event: table,
    write_index: integer,
    event_index: integer
): error?
    local source_id = trim(source_thread_id)
    if source_id == "" or write_thread_id ~= source_id then
        return nil
    end

    local event_type = trim(event.type)
    if event_type == "" then
        return (errors.new({ message = "append_events[" .. tostring(write_index) .. "].events[" .. tostring(event_index) .. "].type is required when writing to the source thread", kind = errors.INVALID }) :: error)
    end
    if not input_types then
        return (errors.new({ message = "projection append_events cannot write to its source thread without events.input excluding the appended event type", kind = errors.INVALID }) :: error)
    end
    if input_types[event_type] == true then
        return (errors.new({ message = "projection append_events would retrigger itself on source event type: " .. event_type, kind = errors.INVALID }) :: error)
    end
    return nil
end

local function normalize_append_writes(worker_output: table, source_thread_id: string?, input_event_types: { string }?, inherited_trace: table?): ({ table }?, error?)
    local raw_writes = worker_output.append_events
    if type(raw_writes) ~= "table" or #raw_writes == 0 then
        return nil, nil
    end

    local input_types = event_filter_set(input_event_types)
    local writes: { table } = table.create(#(raw_writes :: { any }), 0)
    for write_index, raw_write in ipairs(raw_writes :: { any }) do
        if type(raw_write) ~= "table" then
            return nil, (errors.new({ message = "append_events[" .. tostring(write_index) .. "] must be an object", kind = errors.INVALID }) :: error)
        end

        local thread_id = trim(raw_write.thread_id)
        if thread_id == "" then
            return nil, (errors.new({ message = "append_events[" .. tostring(write_index) .. "].thread_id is required", kind = errors.INVALID }) :: error)
        end

        local raw_events = raw_write.events
        if type(raw_events) ~= "table" and type(raw_write.event) == "table" then
            raw_events = { raw_write.event }
        end
        if type(raw_events) ~= "table" or #raw_events == 0 then
            return nil, (errors.new({ message = "append_events[" .. tostring(write_index) .. "].events is required", kind = errors.INVALID }) :: error)
        end

        local events: { table } = table.create(#(raw_events :: { any }), 0)
        for event_index, raw_event in ipairs(raw_events :: { any }) do
            local event, event_err = copy_event_for_append(raw_event, write_index, event_index)
            if event_err then return nil, event_err end
            if inherited_trace and type((event :: table).trace_context) ~= "table" then
                (event :: table).trace_context = inherited_trace
            end
            local boundary_err = validate_source_append(source_thread_id, input_types, thread_id, event :: table, write_index, event_index)
            if boundary_err then return nil, boundary_err end
            events[#events + 1] = event :: table
        end
        writes[#writes + 1] = { thread_id = thread_id, events = events }
    end

    return writes, nil
end

local function append_writes_tx(tx: sql.Transaction, dbtype: string, writes: { table }?): ({ string }?, error?)
    if not writes then return {}, nil end

    local wake_thread_ids: { string } = {}
    for _, write in ipairs(writes :: { table }) do
        local thread_id = tostring(write.thread_id)
        local result, append_err = threads_writer.append_system_tx(tx, dbtype, thread_id, write.events :: { table })
        if append_err then return nil, append_err end
        if result and ((result :: any).inserted or 0) > 0 and (result :: any).wake_scheduler then
            wake_thread_ids[#wake_thread_ids + 1] = thread_id
        end
    end
    return wake_thread_ids, nil
end

local function wake_appended_threads(thread_ids: { string }?)
    for _, thread_id in ipairs(thread_ids or {}) do
        threads_notify.wake(thread_id, "projections.catchup.append_events")
    end
end

-- Resolve the worker_ref from the projection body's meta (its canonical home).
local function body_worker_ref(body: table): string?
    local meta = type(body.meta) == "table" and body.meta or {}
    local ref = trim(meta.worker_ref)
    if ref ~= "" then
        return ref
    end
    return nil
end

local function body_worker_input_mode(body: table): string
    local meta = type(body.meta) == "table" and body.meta or {}
    local mode = trim(meta.worker_input_mode)
    if mode == types.WORKER_INPUT_MODE.PREFETCH_EVENTS or mode == types.WORKER_INPUT_MODE.CURSOR_ONLY then
        return mode
    end
    return types.WORKER_INPUT_MODE_DEFAULT
end

local function body_event_filter_types(body: table): { string }?
    local events = type(body.events) == "table" and body.events or nil
    local input = events and type((events :: table).input) == "table" and ((events :: table).input :: { any }) or nil
    if not input or #input == 0 then return nil end

    local out: { string } = table.create(#input, 0)
    local seen: { [string]: boolean } = {}
    for _, shape in ipairs(input) do
        local event_type = type(shape) == "table" and trim((shape :: table).type) or ""
        if event_type ~= "" and not seen[event_type] then
            seen[event_type] = true
            out[#out + 1] = event_type
        end
    end
    if #out == 0 then return nil end
    return out
end

-- mark_invalid flips both the cursor and its projection to invalid with an error
-- message. A permanent worker/config fault uses this so the scheduler stops
-- re-ticking the cursor.
local function mark_invalid(tx: sql.Transaction, now: string, cursor: table, message: string): error?
    local _, cursor_err = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.INVALID)
        :set("dispatch_after", nil)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("window_opened_at_ms", nil)
        :set("window_last_event_at_ms", nil)
        :set("last_error", message)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor.id }))
        :run_with(tx):exec()
    if cursor_err then
        return (errors.new({ message = "failed to mark cursor invalid: " .. tostring(cursor_err), kind = errors.INTERNAL }) :: error)
    end
    local _, projection_err = sql.builder.update("kickside_projection")
        :set("state", core_types.PROJECTION_STATE.INVALID)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor.projection_id }))
        :run_with(tx):exec()
    if projection_err then
        return (errors.new({ message = "failed to mark projection invalid: " .. tostring(projection_err), kind = errors.INTERNAL }) :: error)
    end
    return nil
end

local function clamp_last_error(message: string): string
    if #message <= 500 then
        return message
    end
    return string.sub(message, 1, 500)
end

-- last_error prefix a dead-lettered cursor carries, so an operator can tell a
-- max-attempts dead-letter from a config invalidation. Stable: recovery tooling
-- matches on it.
local DEAD_LETTER_PREFIX = "projection dead-lettered after max attempts: "
catchup.DEAD_LETTER_PREFIX = DEAD_LETTER_PREFIX

-- Build the dead-letter last_error idempotently. The dead-letter gate feeds this
-- the cursor's existing last_error as the reason; when a cursor with work past
-- its head is re-entered after it was already dead-lettered, that reason ALREADY
-- carries the prefix. Strip every leading prefix first so the original failure is
-- preserved as the reason and wrapped exactly once, never nested into
-- "...after max attempts: ...after max attempts: ..." (which also buried the
-- real cause). Clamped to the last_error length like every other write.
local function dead_letter_message(reason: string): string
    local original = reason
    while string.sub(original, 1, #DEAD_LETTER_PREFIX) == DEAD_LETTER_PREFIX do
        original = string.sub(original, #DEAD_LETTER_PREFIX + 1)
    end
    return clamp_last_error(DEAD_LETTER_PREFIX .. original)
end
catchup.dead_letter_message = dead_letter_message

-- A dispatch's attempt is counted before the worker runs so a silent worker
-- death still consumes budget. A failure classified retryable — the worker
-- marked it so, its kind is a transient infrastructure fault, or it is a
-- Postgres deadlock/serialization abort — must NOT consume that budget; the
-- release un-counts it. An unclassified failure stays counted so a genuinely
-- broken worker still reaches the dead-letter ceiling instead of spinning.
local RETRYABLE_KIND = {
    [errors.UNAVAILABLE] = true,
    [errors.TIMEOUT] = true,
    [errors.RATE_LIMITED] = true,
    [errors.CANCELED] = true,
}
local function transient_db_fault(text: string): boolean
    return string.find(text, "40P01", 1, true) ~= nil
        or string.find(text, "40001", 1, true) ~= nil
        or string.find(text, "deadlock detected", 1, true) ~= nil
        or string.find(text, "could not serialize", 1, true) ~= nil
end
local function is_retryable(err: any): boolean
    if err == nil then return false end
    if type(err) == "userdata" then
        local r = err:retryable()
        if r == true then return true end
        if r == false then return false end
        if RETRYABLE_KIND[err:kind()] then return true end
        return transient_db_fault(tostring(err:message()))
    end
    return transient_db_fault(tostring(err))
end
catchup.is_retryable = is_retryable

-- The five columns that pin a dispatch's fence identity on the cursor row. A
-- release/apply re-read compares the live row against the reservation it holds
-- through this one helper so every fence site stays in lockstep. previous_last_seq
-- is optional: the release_proc fence does not advance last_seq (and does not
-- select it), so it leaves it unset and the last_seq leg is skipped.
type FenceExpect = {
    generation: number,
    dispatch_token: string,
    locked_by: string,
    previous_last_seq: number?,
}
local function fence_matches(row: table, expect: FenceExpect): boolean
    if (tonumber(row.generation) or 1) ~= expect.generation then return false end
    if tostring(row.dispatch_token) ~= expect.dispatch_token then return false end
    if tostring(row.locked_by) ~= expect.locked_by then return false end
    if tostring(row.status) ~= types.CURSOR_STATUS.RUNNING then return false end
    if expect.previous_last_seq ~= nil and (tonumber(row.last_seq) or 0) ~= expect.previous_last_seq then return false end
    return true
end
catchup.fence_matches = fence_matches

-- mark_dead retires a cursor whose dispatch has reached the attempt ceiling. It is
-- the terminal dead-letter: the cursor goes invalid (so the scheduler/claim scan
-- never re-selects it), dead_at is stamped, the durable attempt count is preserved
-- as evidence, and the projection is flipped invalid. Unlike a release this never
-- re-arms dispatch_after, so the range stops replaying. Runs in the supplied
-- transaction; the caller commits.
local function mark_dead(tx: sql.Transaction, now: string, cursor: table, attempts: number, message: string): error?
    local _, cursor_err = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.INVALID)
        :set("dispatch_after", nil)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("window_opened_at_ms", nil)
        :set("window_last_event_at_ms", nil)
        :set("last_error", dead_letter_message(message))
        :set("dead_at", now)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor.id }))
        :run_with(tx):exec()
    if cursor_err then
        return (errors.new({ message = "failed to dead-letter cursor: " .. tostring(cursor_err), kind = errors.INTERNAL }) :: error)
    end
    local _, projection_err = sql.builder.update("kickside_projection")
        :set("state", core_types.PROJECTION_STATE.INVALID)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor.projection_id }))
        :run_with(tx):exec()
    if projection_err then
        return (errors.new({ message = "failed to mark projection dead: " .. tostring(projection_err), kind = errors.INTERNAL }) :: error)
    end
    return nil
end

local function retry_expr(dbtype: string, seconds: integer): string
    if dbtype == sql.type.SQLITE then
        return "datetime('now', '+" .. tostring(seconds) .. " seconds')"
    end
    return "NOW() + INTERVAL '" .. tostring(seconds) .. " seconds'"
end

-- The timestamp threshold before which a lease is stale, in the dialect's clock.
local function stale_before_expr(dbtype: string, seconds: integer): string
    if dbtype == sql.type.SQLITE then
        return "datetime('now', '-" .. tostring(seconds) .. " seconds')"
    end
    return "NOW() - INTERVAL '" .. tostring(seconds) .. " seconds'"
end

-- A computed select item that is 1 when the cursor row carries a lease whose
-- locked_at has aged past the stale threshold. A stale lease belongs to a peer
-- that is gone, so a waiting inline caller may take the cursor over rather than
-- wait on it forever.
local function lease_stale_select(dbtype: string, seconds: integer): string
    return "(CASE WHEN " .. CURSOR_ALIAS .. ".locked_at IS NOT NULL AND " ..
        CURSOR_ALIAS .. ".locked_at < " .. stale_before_expr(dbtype, seconds) ..
        " THEN 1 ELSE 0 END) AS lease_stale"
end

local function truthy_flag(value: any): boolean
    if value == true then return true end
    local n = tonumber(value)
    if n ~= nil then return n ~= 0 end
    return false
end

local function inline_runner_id(dispatch_token: string): string
    return "scheduler:inline:" .. dispatch_token
end

local function release_inline_failure(
    db: sql.DB,
    dbtype: string,
    cursor_id: string,
    generation: number,
    dispatch_token: string,
    previous_last_seq: number,
    reason: string,
    retryable: boolean
): (boolean, error?)
    local now = now_string(dbtype)
    local lock = row_lock(dbtype, "fence")
    local rtx, rtx_err = db:begin()
    if rtx_err then
        return false, (errors.new({ message = "failed to begin inline release transaction: " .. tostring(rtx_err), kind = errors.INTERNAL }) :: error)
    end

    local fence_query = sql.builder.select("generation", "dispatch_token", "locked_by", "status", "last_seq")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({ id = cursor_id }))
    if lock ~= "" then
        fence_query = fence_query:suffix(lock)
    end
    local fence_rows, fence_err = fence_query:run_with(rtx):query()
    if fence_err then
        rtx:rollback()
        return false, (errors.new({ message = "failed to re-check inline release fence: " .. tostring(fence_err), kind = errors.INTERNAL }) :: error)
    end
    if not fence_rows or #fence_rows == 0 then
        rtx:rollback()
        return false, nil
    end

    local row = fence_rows[1]
    if not fence_matches(row, {
        generation = generation,
        dispatch_token = dispatch_token,
        locked_by = inline_runner_id(dispatch_token),
        previous_last_seq = previous_last_seq,
    }) then
        rtx:rollback()
        return false, nil
    end

    -- The fence still holds, so this dispatch's pre-run increment is exactly the
    -- one on the row. A retryable failure un-counts it (attempts - 1 returns the
    -- row to its pre-dispatch value) so a transient fault never climbs toward the
    -- dead-letter ceiling; a terminal failure keeps the count.
    local release = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.PENDING)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("dispatch_after", sql.builder.expr(retry_expr(dbtype, proj_env.inline_release_retry_seconds())))
        :set("last_error", clamp_last_error(reason))
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor_id }))
    if retryable then
        release = release:set("attempts", sql.builder.expr("attempts - 1"))
    end
    local _, update_err = release:run_with(rtx):exec()
    if update_err then
        rtx:rollback()
        return false, (errors.new({ message = "failed to release inline cursor: " .. tostring(update_err), kind = errors.INTERNAL }) :: error)
    end

    local _, commit_err = rtx:commit()
    if commit_err then
        return false, (errors.new({ message = "failed to commit inline release: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    return true, nil
end

-- Options a tick accepts. proc_host is forwarded to a proc:// worker.
type TickOpts = {
    cursor_id: string,
    expected_generation: integer?,
    proc_host: string?,
    before_apply_fn: ((table) -> ())?,
    during_wait_fn: ((table) -> ())?,
}

-- The full output of the shared prepare phase. Exactly one of terminal / prepared
-- is set: terminal carries a TickResult (the prepare already committed/rolled the
-- tx and decided the outcome); prepared carries the still-open tx plus the
-- decoded row, resolved binding, prefetched batch and built worker_input that
-- tick/reserve_proc finish off with their own final cursor write + commit.
type PrepareOutcome = {
    terminal: TickResult?,
    terminal_err: error?,
    tx: sql.Transaction?,
    cursor: table?,
    cid: string?,
    status: string?,
    generation: number?,
    hydration_state: string?,
    current_last: number?,
    target_seq: number?,
    new_last: number?,
    processed: number?,
    binding: table?,
    input_mode: string?,
    body: table?,
    worker_input: table?,
    lease_stale: boolean?,
}

-- The shared load → decide → prefetch → build-worker_input phase. Opens its own
-- transaction off db, takes the cursor row lock, and either resolves a terminal
-- outcome (park / invalid / paused / stale / not-found — committing or rolling
-- the tx itself) or returns the open tx and everything tick/reserve need to make
-- their final cursor write. The caller MUST finish a non-terminal outcome: commit
-- (tick) or reserve-and-commit (reserve_proc), then run the worker. The lock is
-- held until that final commit, so the caller writes promptly.
local function prepare(db: sql.DB, opts: TickOpts, dbtype: string, now: string): PrepareOutcome
    local cursor_id = trim(opts.cursor_id)
    -- The prepare load of this one resolved cursor joins projection + thread but
    -- must lock only the cursor row (FOR UPDATE OF c) so concurrent claimers never
    -- acquire the shared projection/thread rows in plan-dependent order -- the 40P01
    -- deadlock ("failed to load cursor") that dead-lettered healthy cursors. It
    -- BLOCKS rather than skips: the synchronous catch_up path runs this same load
    -- and must wait for a concurrent runner's in-flight lock on THIS cursor, not
    -- read the locked row as absent (a SKIP LOCKED miss surfaces as a false
    -- cursor_not_found). A single-row wait on the already-distributed cursor cannot
    -- cycle, so this stays deadlock-free.
    local lock = row_lock(dbtype, "acquire")

    local tx, tx_err = db:begin()
    if tx_err then
        return { terminal_err = (errors.new({ message = "failed to begin transaction: " .. tostring(tx_err), kind = errors.INTERNAL }) :: error) }
    end

    local load_query = sql.builder.select(
        "c.id", "c.projection_id", "c.thread_id", "c.cursor_key", "c.last_seq", "c.target_seq", "c.batch_size",
        "c.status", "c.generation", "c.dispatch_after", "c.locked_by", "c.locked_at", "c.worker_runtime", "c.attempts",
        "c.last_error", "p.kind", "p.body",
        "p.actor_id", "p.actor_context", "t.event_count", "t.hydration_state", "t.hydration_final_seq",
        lease_stale_select(dbtype, proj_env.inline_lease_stale_seconds())
    )
        :from("kickside_projection_cursor c")
        :inner_join("kickside_projection p ON p.id = c.projection_id")
        :inner_join("kickside_thread t ON t.id = c.thread_id")
        :where("c.id = ?", cursor_id)
    if lock ~= "" then
        load_query = load_query:suffix(lock)
    end
    local rows, load_err = load_query:run_with(tx):query()
    if load_err then
        tx:rollback()
        return { terminal_err = (errors.new({ message = "failed to load cursor: " .. tostring(load_err), kind = errors.INTERNAL }) :: error) }
    end
    if not rows or #rows == 0 then
        tx:rollback()
        return { terminal = { cursor_id = cursor_id, status = types.CURSOR_STATUS.INVALID, processed = 0, last_seq = 0, target_seq = 0, done = true, obsolete = true, reason = "cursor_not_found" } }
    end
    local cursor = rows[1]
    local cid = tostring(cursor.id)
    local status = tostring(cursor.status)

    if status == types.CURSOR_STATUS.PAUSED then
        tx:rollback()
        return { terminal = { cursor_id = cid, status = types.CURSOR_STATUS.PAUSED, processed = 0, last_seq = tonumber(cursor.last_seq) or 0, target_seq = tonumber(cursor.target_seq) or 0, done = true, skipped = true, reason = "cursor_paused" } }
    end

    local generation = tonumber(cursor.generation) or 1
    if opts.expected_generation and opts.expected_generation ~= generation then
        tx:rollback()
        return { terminal = { cursor_id = cid, status = status, processed = 0, last_seq = tonumber(cursor.last_seq) or 0, target_seq = tonumber(cursor.target_seq) or 0, done = true, stale = true, reason = "generation_mismatch" } }
    end

    local hydration_state = tostring(cursor.hydration_state)
    local event_count = tonumber(cursor.event_count) or 0
    local current_last = tonumber(cursor.last_seq) or 0
    local batch_size = tonumber(cursor.batch_size) or types.BATCH_SIZE.MAX
    if batch_size < types.BATCH_SIZE.MIN then batch_size = types.BATCH_SIZE.MIN end
    if batch_size > types.BATCH_SIZE.MAX then batch_size = types.BATCH_SIZE.MAX end

    -- target_seq: an explicit ceiling on the cursor; otherwise the thread head.
    -- A live cursor with no explicit target tails the head.
    local target_seq = tonumber(cursor.target_seq)
    if hydration_state == core_types.HYDRATION_STATE.LIVE then
        -- A live cursor tails the thread head. A concrete target can survive an
        -- interrupted catch-up / older writer, but it is only a stale snapshot
        -- once the thread has moved past it; honoring it would park the cursor
        -- live forever without reading the new event.
        target_seq = math.max(target_seq or 0, event_count)
    elseif not target_seq then
        if hydration_state == core_types.HYDRATION_STATE.CATCHING_UP and cursor.hydration_final_seq then
            target_seq = tonumber(cursor.hydration_final_seq) or event_count
        else
            target_seq = event_count
        end
    end

    local next_from = current_last + 1
    local body, body_err = decode_body(cursor.body)
    if body_err then
        tx:rollback()
        return { terminal_err = (errors.new({
            message = "projection " .. tostring(cursor.projection_id) .. " cursor " .. cid .. " body is corrupt: " .. (body_err :: error):message(),
            kind = errors.INTERNAL,
        }) :: error) }
    end

    -- Nothing past the cursor head: park it. live cursors stay live (tailing),
    -- bounded cursors become caught_up, and the projection is marked valid.
    if next_from > target_seq then
        local final_status = hydration_state == core_types.HYDRATION_STATE.LIVE and types.CURSOR_STATUS.LIVE or types.CURSOR_STATUS.CAUGHT_UP
        local final_target: number? = hydration_state == core_types.HYDRATION_STATE.LIVE and nil or target_seq
        local _, cursor_err = sql.builder.update("kickside_projection_cursor")
            :set("status", final_status)
            :set("target_seq", final_target)
            :set("dispatch_after", nil)
            :set("locked_by", nil)
            :set("locked_at", nil)
            :set("dispatch_token", nil)
            :set("window_opened_at_ms", nil)
            :set("window_last_event_at_ms", nil)
            :set("last_error", nil)
            :set("updated_at", now)
            :where(sql.builder.eq({ id = cursor.id }))
            :run_with(tx):exec()
        if cursor_err then
            tx:rollback()
            return { terminal_err = (errors.new({ message = "failed to park cursor: " .. tostring(cursor_err), kind = errors.INTERNAL }) :: error) }
        end
        -- Advance last_event_seq only if it is behind, so a re-tick never rewinds it.
        local _, projection_err = sql.builder.update("kickside_projection")
            :set("state", core_types.PROJECTION_STATE.VALID)
            :set("last_event_seq", sql.builder.expr("CASE WHEN last_event_seq > ? THEN last_event_seq ELSE ? END", current_last, current_last))
            :set("updated_at", now)
            :where(sql.builder.eq({ id = cursor.projection_id }))
            :run_with(tx):exec()
        if projection_err then
            tx:rollback()
            return { terminal_err = (errors.new({ message = "failed to mark projection valid: " .. tostring(projection_err), kind = errors.INTERNAL }) :: error) }
        end
        local _, commit_err = tx:commit()
        if commit_err then
            return { terminal_err = (errors.new({ message = "failed to commit cursor park: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error) }
        end
        return { terminal = { cursor_id = cid, status = final_status, processed = 0, last_seq = current_last, target_seq = target_seq, done = true } }
    end

    -- Dead-letter gate. There IS work past the head, but the durable attempt
    -- counter has reached the ceiling: this many reserves since the last successful
    -- apply have each ended in a genuine terminal fault. Retire the cursor now
    -- instead of dispatching again, so a repeatedly-failing worker cannot replay its
    -- range forever. The counter climbs only on terminal faults: a retryable failure
    -- un-counts on release and a benign fence-out (a peer advanced the cursor)
    -- un-counts on apply, so peer contention never accrues toward this ceiling.
    -- Skip a cursor a live peer already holds (running with dispatch_after cleared):
    -- reserve_proc's already_reserved guard yields to it, and dead-lettering here
    -- would retire its in-flight dispatch one attempt early. Such a cursor is gated
    -- on the next prepare once it is released back to pending.
    local attempts = tonumber(cursor.attempts) or 0
    local max_attempts = proj_env.max_attempts()
    local peer_reserved = cursor.dispatch_after == nil and status == types.CURSOR_STATUS.RUNNING
    if attempts >= max_attempts and not peer_reserved then
        local last_error = trim(cursor.last_error)
        local detail = last_error ~= "" and last_error or ("reserved " .. tostring(attempts) .. " times without a successful apply")
        local dead_err = mark_dead(tx, now, cursor, attempts, detail)
        if dead_err then
            tx:rollback()
            return { terminal_err = dead_err }
        end
        local _, commit_err = tx:commit()
        if commit_err then
            return { terminal_err = (errors.new({ message = "failed to commit dead-letter: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error) }
        end
        return { terminal = { cursor_id = cid, status = types.CURSOR_STATUS.INVALID, processed = 0, last_seq = current_last, target_seq = target_seq, done = true, dead = true, reason = "max_attempts", error = dead_letter_message(detail) } }
    end

    -- Resolve the worker the projection is bound to. A missing or invalid ref is
    -- a permanent config fault: invalidate the cursor + projection.
    local worker_ref = body_worker_ref(body)
    if not worker_ref then
        local invalid_err = mark_invalid(tx, now, cursor, "worker_ref is missing from projection body")
        if invalid_err then
            tx:rollback()
            return { terminal_err = invalid_err }
        end
        local _, commit_err = tx:commit()
        if commit_err then
            return { terminal_err = (errors.new({ message = "failed to commit invalidation: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error) }
        end
        return { terminal = { cursor_id = cid, status = types.CURSOR_STATUS.INVALID, processed = 0, last_seq = current_last, target_seq = target_seq, done = true, error = "worker_ref is missing" } }
    end

    local binding, parse_err = worker.parse(worker_ref, true)
    if parse_err or not binding then
        local invalid_err = mark_invalid(tx, now, cursor, tostring(parse_err and parse_err:message() or "invalid worker_ref"))
        if invalid_err then
            tx:rollback()
            return { terminal_err = invalid_err }
        end
        local _, commit_err = tx:commit()
        if commit_err then
            return { terminal_err = (errors.new({ message = "failed to commit invalidation: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error) }
        end
        return { terminal = { cursor_id = cid, status = types.CURSOR_STATUS.INVALID, processed = 0, last_seq = current_last, target_seq = target_seq, done = true, error = tostring(parse_err and parse_err:message()) } }
    end

    local validated, validate_err = worker.validate(binding)
    if validate_err or not validated then
        -- A blocked daemon is an operational fault (retryable once fixed); any
        -- other validation failure is a permanent config fault.
        if worker.is_blocked_daemon(binding.target_id) then
            tx:rollback()
            return { terminal_err = (validate_err :: error) }
        end
        local invalid_err = mark_invalid(tx, now, cursor, tostring(validate_err and validate_err:message() or "invalid worker binding"))
        if invalid_err then
            tx:rollback()
            return { terminal_err = invalid_err }
        end
        local _, commit_err = tx:commit()
        if commit_err then
            return { terminal_err = (errors.new({ message = "failed to commit invalidation: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error) }
        end
        return { terminal = { cursor_id = cid, status = types.CURSOR_STATUS.INVALID, processed = 0, last_seq = current_last, target_seq = target_seq, done = true, error = tostring(validate_err and validate_err:message()) } }
    end
    binding = validated
    -- The projection's bounded owner identity (canonical actor_id + actor_context,
    -- captured at register). Carried on the binding so worker.run reconstructs and
    -- runs the worker as that owner. Null => system/runner context by design.
    binding.actor_id = cursor.actor_id
    binding.actor_context = cursor.actor_context

    local input_mode = body_worker_input_mode(body)
    local event_filter_types = body_event_filter_types(body)

    -- Prefetch the batch inside the load transaction so the worker sees a
    -- consistent snapshot; compute the new head from the rows.
    local batch_rows, batch_err = worker.query_event_rows(tx, tostring(cursor.thread_id), next_from, target_seq, batch_size, event_filter_types)
    if batch_err then
        tx:rollback()
        return { terminal_err = batch_err }
    end
    local processed = #(batch_rows :: table[])
    local new_last = current_last
    if event_filter_types ~= nil then
        if processed == 0 then
            new_last = target_seq
        else
            for _, row in ipairs(batch_rows :: table[]) do
                local seq = tonumber(row.seq) or new_last
                if seq > new_last then
                    new_last = seq
                end
            end
            if processed < batch_size then
                new_last = target_seq
            end
        end
    else
        for _, row in ipairs(batch_rows :: table[]) do
            local seq = tonumber(row.seq) or new_last
            if seq > new_last then
                new_last = seq
            end
        end
    end

    local inherited_trace = trace_context.common(batch_rows)
    local worker_input = {
        thread_id = cursor.thread_id,
        trace_context = inherited_trace,
        worker_input_mode = input_mode,
        cursor = {
            id = cursor.id,
            cursor_key = cursor.cursor_key,
            last_seq = current_last,
            target_seq = target_seq,
            batch_size = batch_size,
            generation = generation,
        },
        projection = {
            id = cursor.projection_id,
            kind = cursor.kind,
            body = body,
        },
        range = {
            from_seq = next_from,
            to_seq = new_last,
            target_seq = target_seq,
            processed_count = processed,
        },
        events = input_mode == types.WORKER_INPUT_MODE.PREFETCH_EVENTS and batch_rows or nil,
    }

    return {
        tx = tx,
        cursor = cursor,
        cid = cid,
        status = status,
        generation = generation,
        hydration_state = hydration_state,
        current_last = current_last,
        target_seq = target_seq,
        new_last = new_last,
        processed = processed,
        binding = binding,
        input_mode = input_mode,
        body = body,
        worker_input = worker_input,
        lease_stale = truthy_flag(cursor.lease_stale),
    }
end

-- A running cursor with no dispatch stamp is the durable representation of an
-- in-flight lease. Treat even a malformed such row as leased: reconciliation
-- must fail closed rather than changing the lane or re-arming work a worker may
-- still apply under its existing token.
local function has_active_lease(cursor: table): boolean
    return cursor.dispatch_after == nil and tostring(cursor.status) == types.CURSOR_STATUS.RUNNING
end

-- The live progress of a cursor a peer is driving, read without a lock: the
-- synchronous catch_up path polls this while it waits on the peer instead of
-- reserving-and-superseding. active mirrors has_active_lease; stale reports the
-- lease has aged past the takeover threshold.
type PeerProgress = {
    found: boolean,
    last_seq: number,
    status: string,
    active: boolean,
    stale: boolean,
}
local function read_peer_progress(db: sql.DB, dbtype: string, cursor_id: string): (PeerProgress?, error?)
    local rows, err = sql.builder.select(
        CURSOR_ALIAS .. ".last_seq", CURSOR_ALIAS .. ".status", CURSOR_ALIAS .. ".dispatch_after",
        lease_stale_select(dbtype, proj_env.inline_lease_stale_seconds())
    )
        :from("kickside_projection_cursor " .. CURSOR_ALIAS)
        :where(CURSOR_ALIAS .. ".id = ?", cursor_id)
        :run_with(db):query()
    if err then
        return nil, (errors.new({ message = "failed to read peer progress: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    if not rows or #rows == 0 then
        return { found = false, last_seq = 0, status = "", active = false, stale = false }, nil
    end
    local row = rows[1]
    return {
        found = true,
        last_seq = tonumber(row.last_seq) or 0,
        status = tostring(row.status),
        active = row.dispatch_after == nil and tostring(row.status) == types.CURSOR_STATUS.RUNNING,
        stale = truthy_flag(row.lease_stale),
    }, nil
end

-- Un-count the pre-run attempt increment of a dispatch that fenced out because a
-- peer advanced the cursor. A fence-out is benign peer-progress, not a worker
-- fault, so it must never climb toward the dead-letter ceiling; only a genuine
-- terminal fault (kept by the release paths) does. The decrement is guarded by
-- generation so a re-register's fresh counter is never touched, and floored at 0
-- so a peer's successful apply (which resets the counter) cannot drive it
-- negative. Runs in its own transaction; a fence-out over a vanished row is a
-- no-op. The release paths un-count under the fence they still hold; here the
-- fence is already lost, so ownership is proven by the generation guard instead.
local function uncount_superseded_attempt(db: sql.DB, dbtype: string, cursor_id: string, generation: number): error?
    local now = now_string(dbtype)
    local _, update_err = sql.builder.update("kickside_projection_cursor")
        :set("attempts", sql.builder.expr("CASE WHEN attempts > 0 THEN attempts - 1 ELSE 0 END"))
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor_id, generation = generation }))
        :run_with(db):exec()
    if update_err then
        return (errors.new({ message = "failed to un-count superseded attempt: " .. tostring(update_err), kind = errors.INTERNAL }) :: error)
    end
    return nil
end
catchup.uncount_superseded_attempt = uncount_superseded_attempt

-- worker_runtime is only a denormalized discovery index; worker_ref in the
-- projection body is authoritative. Callers hold the cursor row lock and have
-- already rejected active leases. Preserve a non-null schedule (including an
-- intentional future trigger), but re-arm a lost NULL schedule so the newly
-- canonical lane can actually discover the still-due work.
local function reconcile_worker_runtime(tx: sql.Transaction, cursor: table, binding: table, now: string): error?
    local update = sql.builder.update("kickside_projection_cursor")
        :set("worker_runtime", binding.worker_runtime)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor.id }))
    if cursor.dispatch_after == nil then
        update = update:set("dispatch_after", now)
    end
    local _, repair_err = update:run_with(tx):exec()
    if repair_err then
        return (errors.new({ message = "failed to repair projection worker runtime: " .. tostring(repair_err), kind = errors.INTERNAL }) :: error)
    end
    return nil
end

-- Mirror tick's post-run cursor transition: clamp the worker's last_seq, resolve
-- the next status/target, persist the projection body patch + advance, and write
-- the cursor. extra_cursor lets the proc apply also clear its lease columns. Runs
-- inside the supplied transaction; the caller commits.
local function persist_advance(
    apply_tx: sql.Transaction, now: string, dbtype: string,
    projection_id: string, cursor_id: string, body: table, binding: table, input_mode: string,
    current_last: number, target_seq: number, hydration_state: string, explicit_target: number?,
    worker_output: table
): (number, string, error?)
    -- A worker may return a body to persist as the new projection body, and/or a
    -- last_seq to clamp the advance to.
    local next_body = body
    if type(worker_output.body) == "table" then
        next_body = worker_output.body
    elseif type(worker_output.patch) == "table" then
        for key, value in pairs(worker_output.patch :: table) do
            next_body[key] = value
        end
    end
    local worker_last = tonumber(worker_output.last_seq)
    -- new_last starts from the prefetch head the caller threads through
    -- worker_output.new_last; the worker may then clamp it down (never up past
    -- target). This mirrors tick's advance exactly.
    local new_last = tonumber(worker_output.new_last) or current_last
    if worker_last and worker_last > current_last and worker_last <= target_seq then
        new_last = worker_last
    end
    -- Keep the worker_ref pinned in body.meta so a body the worker replaced still
    -- resolves on the next tick.
    next_body.meta = type(next_body.meta) == "table" and next_body.meta or {}
    next_body.meta.worker_ref = binding.worker_ref
    next_body.meta.worker_runtime = binding.worker_runtime
    next_body.meta.worker_input_mode = input_mode
    if type(next_body.worker_config) ~= "table" and type(body.worker_config) == "table" then
        next_body.worker_config = body.worker_config
    end
    if type(next_body.trigger_policy) ~= "table" and type(body.trigger_policy) == "table" then
        next_body.trigger_policy = body.trigger_policy
    end
    if type(next_body.events) ~= "table" and type(body.events) == "table" then
        next_body.events = body.events
    end

    local body_json, body_encode_err = json.encode(next_body)
    if body_encode_err or not body_json then
        return current_last, "", (errors.new({ message = "failed to encode projection body: " .. tostring(body_encode_err), kind = errors.INVALID }) :: error)
    end

    local _, projection_update_err = sql.builder.update("kickside_projection")
        :set("body", body_json)
        :set("state", core_types.PROJECTION_STATE.VALID)
        :set("last_event_seq", new_last)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = projection_id }))
        :run_with(apply_tx):exec()
    if projection_update_err then
        return current_last, "", (errors.new({ message = "failed to persist projection batch: " .. tostring(projection_update_err), kind = errors.INTERNAL }) :: error)
    end

    local done = new_last >= target_seq
    local next_status: string
    local next_target: number?
    if done then
        next_status = hydration_state == core_types.HYDRATION_STATE.LIVE and types.CURSOR_STATUS.LIVE or types.CURSOR_STATUS.CAUGHT_UP
        next_target = hydration_state == core_types.HYDRATION_STATE.LIVE and nil or target_seq
    else
        next_status = types.CURSOR_STATUS.RUNNING
        next_target = hydration_state == core_types.HYDRATION_STATE.LIVE and explicit_target or target_seq
    end

    -- A batch that did not reach the target re-arms dispatch_after to now so the
    -- next batch is claimed immediately; a bulk backlog larger than one batch_size
    -- gets no further event appends to re-trigger it, so without this re-arm it
    -- would stall at the first batch boundary. A done cursor clears dispatch_after.
    local next_dispatch_after: string? = nil
    if not done then
        next_dispatch_after = now
    end
    -- A keyed consumer failure is intentionally terminal for that item: the
    -- caller advances the cursor so one poison event cannot wedge the stream.
    -- It must remain visible, though; clearing last_error here turns a skipped
    -- automation delivery into a silent loss. Workers may provide an exact
    -- message (binding dispatch does); retain a useful fallback for other
    -- envelope consumers.
    local delivery_error = trim(worker_output.last_error)
    if delivery_error == "" and type(worker_output.failed_keys) == "table" and #(worker_output.failed_keys :: { any }) > 0 then
        delivery_error = "projection worker reported " .. tostring(#(worker_output.failed_keys :: { any })) .. " failed item(s)"
    end
    -- A successful apply clears the durable attempt counter: the cursor made
    -- progress, so the next failure/contention starts its climb to the ceiling
    -- fresh rather than inheriting a stale count.
    local _, cursor_update_err = sql.builder.update("kickside_projection_cursor")
        :set("last_seq", new_last)
        :set("target_seq", next_target)
        :set("status", next_status)
        -- This column is a claim index derived from the authoritative worker_ref
        -- in the projection body. Converge it after every successful apply so a
        -- hot declaration change cannot leave the cursor in the wrong lane.
        :set("worker_runtime", binding.worker_runtime)
        :set("dispatch_after", next_dispatch_after)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("last_error", delivery_error ~= "" and clamp_last_error(delivery_error) or nil)
        :set("attempts", 0)
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor_id }))
        :run_with(apply_tx):exec()
    if cursor_update_err then
        return current_last, "", (errors.new({ message = "failed to persist cursor progress: " .. tostring(cursor_update_err), kind = errors.INTERNAL }) :: error)
    end

    return new_last, next_status, nil
end

-- tick advances one cursor by one batch. It opens its own transaction off db for
-- the load/decision phase, commits a snapshot before invoking the worker (so the
-- worker runs outside the lock), then applies the result in a fresh transaction
-- under a scheduler-owned dispatch-token fence. Returns a summary; done=false
-- means the caller should tick again.
function catchup.tick(db: sql.DB, opts: TickOpts): (TickResult?, error?)
    local cursor_id = trim(opts.cursor_id)
    if cursor_id == "" then
        return nil, (errors.new({ message = "cursor_id is required", kind = errors.INVALID }) :: error)
    end

    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local now = now_string(dbtype)
    -- The apply-phase re-read fences a cursor this tick already reserved (owns
    -- its dispatch_token); a plain blocking FOR UPDATE on that single owned row
    -- is correct and uncontended.
    local lock = row_lock(dbtype, "fence")

    local poll_ms = tostring(proj_env.inline_peer_poll_ms()) .. "ms"
    local polls_left = proj_env.inline_peer_wait_polls()

    -- Resolve a cursor this caller may drive. A peer already driving it under a
    -- LIVE lease is waited on, not superseded: this caller polls its last_seq and
    -- returns once the peer reaches the head, so only one driver advances the
    -- cursor at a time (concurrent synchronous catch_up otherwise reserves over
    -- each other until every apply fences out and the cursor dead-letters). A
    -- STALE lease (its peer is gone) is taken over by falling through to drive
    -- under the prepare lock the resolving iteration holds.
    local prep: any = nil
    local tx: sql.Transaction? = nil
    local cursor: any = nil
    local binding: any = nil
    while true do
        now = now_string(dbtype)
        prep = prepare(db, opts, dbtype, now)
        if prep.terminal_err then
            return nil, prep.terminal_err
        end
        if prep.terminal then
            return prep.terminal, nil
        end

        tx = prep.tx :: sql.Transaction
        cursor = prep.cursor :: table
        binding = prep.binding :: table

        if not (has_active_lease(cursor) and not (prep.lease_stale :: boolean)) then
            break
        end

        -- Release the prepare lock before waiting so the peer's fenced apply is
        -- never blocked by this caller.
        tx:rollback()
        local during_wait = opts.during_wait_fn
        if during_wait ~= nil then
            during_wait({ cursor_id = tostring(cursor.id), target_seq = prep.target_seq :: number })
        end
        local progress, progress_err = read_peer_progress(db, dbtype, tostring(cursor.id))
        if progress_err then
            return nil, progress_err
        end
        local prog = progress :: PeerProgress
        if not prog.found then
            return { cursor_id = tostring(cursor.id), status = types.CURSOR_STATUS.INVALID, processed = 0, last_seq = prep.current_last :: number, target_seq = prep.target_seq :: number, done = true, obsolete = true, reason = "cursor_not_found" }, nil
        end
        if prog.last_seq >= (prep.target_seq :: number) then
            -- The peer advanced the cursor to the head this caller needed: the
            -- read-after-write it waited for is satisfied, no second driver.
            return { cursor_id = tostring(cursor.id), status = prog.status, processed = 0, last_seq = prog.last_seq, target_seq = prep.target_seq :: number, done = true, reason = "peer_advanced" }, nil
        end
        if (not prog.active) or prog.stale then
            -- The peer released a batch or its lease went stale: re-prepare and
            -- this caller drives the next batch (or takes the stale cursor over).
        elseif polls_left <= 0 then
            -- The per-tick wait budget is spent and the peer still holds a live
            -- lease. Yield not-done so the caller re-ticks within its own max_ticks
            -- budget instead of superseding the in-flight peer.
            return { cursor_id = tostring(cursor.id), status = types.CURSOR_STATUS.RUNNING, processed = 0, last_seq = prog.last_seq, target_seq = prep.target_seq :: number, done = false, reason = "peer_in_flight" }, nil
        else
            polls_left = polls_left - 1
            time.sleep(poll_ms)
        end
    end

    local active_tx = tx :: sql.Transaction
    if tostring(binding.worker_runtime) ~= types.WORKER_RUNTIME.FUNC then
        -- The inline scheduler discovered this row from its denormalized func
        -- claim index, but the authoritative worker_ref has since moved it to
        -- proc://. Repair that stale index under the prepare lock and, if an
        -- interrupted wake lost its NULL schedule, re-arm it for the proc runner.
        -- A correctly indexed proc row sent to tick directly remains invalid.
        if tostring(cursor.worker_runtime) ~= types.WORKER_RUNTIME.PROC then
            local repair_err = reconcile_worker_runtime(active_tx, cursor, binding, now)
            if repair_err then
                active_tx:rollback()
                return nil, repair_err
            end
            local _, commit_err = active_tx:commit()
            if commit_err then
                return nil, (errors.new({ message = "failed to commit projection worker runtime repair: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
            end
            return {
                cursor_id = tostring(cursor.id),
                status = tostring(cursor.status),
                processed = 0,
                last_seq = prep.current_last :: number,
                target_seq = prep.target_seq :: number,
                done = true,
                skipped = true,
                reason = "worker_runtime_reconciled",
            }, nil
        end
        active_tx:rollback()
        return nil, (errors.new({
            message = "proc:// projection workers are asynchronous and cannot run through inline catch-up",
            kind = errors.INVALID,
        }) :: error)
    end
    local cid = prep.cid :: string
    local status = prep.status :: string
    local generation = prep.generation :: number
    local hydration_state = prep.hydration_state :: string
    local current_last = prep.current_last :: number
    local target_seq = prep.target_seq :: number
    local prefetch_last = prep.new_last :: number
    local processed = prep.processed :: number
    local input_mode = prep.input_mode :: string
    local body = prep.body :: table
    local worker_input = prep.worker_input :: table
    local dispatch_token = uuid.v7()
    local runner_id = inline_runner_id(dispatch_token)

    -- Reserve the inline scheduler attempt durably before the worker runs. The
    -- worker runs outside the DB lock, but the cursor is now fenced by
    -- locked_by/dispatch_token so peers skip it and reclaim can recover it if the
    -- scheduler dies mid-worker.
    -- Count this dispatch durably before the worker runs. The increment lives on
    -- the cursor row (not the projection body), so a silent worker death still
    -- consumes budget. A successful apply resets it; a benign fence-out (a peer
    -- advanced the cursor) un-counts it on apply; only a genuine terminal fault
    -- keeps it, so reaching the ceiling dead-letters the cursor on the next prepare.
    local _, reserve_err = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.RUNNING)
        :set("locked_by", runner_id)
        :set("locked_at", now)
        :set("dispatch_token", dispatch_token)
        :set("dispatch_after", nil)
        :set("attempts", sql.builder.expr("attempts + 1"))
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor.id }))
        :run_with(active_tx):exec()
    if reserve_err then
        active_tx:rollback()
        return nil, (errors.new({ message = "failed to reserve inline cursor: " .. tostring(reserve_err), kind = errors.INTERNAL }) :: error)
    end

    -- Commit the snapshot and release the lock before running the worker, so a
    -- slow worker does not hold the cursor row.
    local _, snapshot_commit_err = active_tx:commit()
    if snapshot_commit_err then
        return nil, (errors.new({ message = "failed to commit pre-worker snapshot: " .. tostring(snapshot_commit_err), kind = errors.INTERNAL }) :: error)
    end

    local function release_worker_failure(reason: string, retryable: boolean): (TickResult?, error?)
        local released, release_err = release_inline_failure(db, dbtype, cid, generation, dispatch_token, current_last, reason, retryable)
        if release_err then
            return nil, release_err
        end
        return {
            cursor_id = cid,
            status = released and types.CURSOR_STATUS.PENDING or types.CURSOR_STATUS.RUNNING,
            processed = 0,
            last_seq = current_last,
            target_seq = target_seq,
            done = false,
            skipped = true,
            stale = not released or nil,
            reason = released and "worker_failed" or "fence_mismatch",
            error = clamp_last_error(reason),
        }, nil
    end

    -- An empty batch (a gap or a filtered range) still advances the head without
    -- invoking the worker.
    local worker_output: table = {}
    if processed > 0 then
        local exec_ok, output, run_err = pcall(worker.run, binding, worker_input)
        if not exec_ok then
            run_err = (errors.new({ message = "projection worker panic: " .. tostring(output), kind = errors.INTERNAL }) :: error)
            output = nil
        end
        if run_err or not output then
            return release_worker_failure((run_err :: error):message() :: string, is_retryable(run_err))
        end
        worker_output = output
    end

    local append_events, append_validate_err = normalize_append_writes(worker_output, tostring(cursor.thread_id), body_event_filter_types(body), worker_input.trace_context :: table?)
    if append_validate_err then
        -- append_events validation failure is always errors.INVALID: a terminal
        -- config/output fault, so it stays counted.
        return release_worker_failure("projection worker append_events invalid: " .. (append_validate_err :: error):message(), false)
    end

    local before_apply = opts.before_apply_fn
    if before_apply ~= nil then
        before_apply({
            cursor_id = cid,
            projection_id = tostring(cursor.projection_id),
            thread_id = tostring(cursor.thread_id),
            generation = generation,
            dispatch_token = dispatch_token,
            last_seq = current_last,
            target_seq = target_seq,
        })
    end

    -- Apply phase: re-open a transaction, re-check the generation/token/head
    -- fence, persist the worker's body patch and the advanced cursor.
    local apply_tx, apply_tx_err = db:begin()
    if apply_tx_err then
        return nil, (errors.new({ message = "failed to begin apply transaction: " .. tostring(apply_tx_err), kind = errors.INTERNAL }) :: error)
    end

    local gen_query = sql.builder.select("generation", "dispatch_token", "locked_by", "status", "last_seq")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({ id = cursor.id }))
    if lock ~= "" then
        gen_query = gen_query:suffix(lock)
    end
    local gen_rows, gen_err = gen_query:run_with(apply_tx):query()
    if gen_err then
        apply_tx:rollback()
        return nil, (errors.new({ message = "failed to re-check cursor generation: " .. tostring(gen_err), kind = errors.INTERNAL }) :: error)
    end
    if not gen_rows or #gen_rows == 0 then
        apply_tx:rollback()
        return { cursor_id = cid, status = types.CURSOR_STATUS.INVALID, processed = processed, last_seq = prefetch_last, target_seq = target_seq, done = true, obsolete = true, reason = "cursor_not_found" }, nil
    end
    local fence = gen_rows[1]
    -- A generation bump is a distinct outcome (a re-register superseded this
    -- dispatch), reported separately from a lost-lease fence mismatch. Either way
    -- the worker ran and only a peer moved the cursor on: un-count this dispatch's
    -- pre-run attempt so benign peer-progress never climbs toward the dead-letter
    -- ceiling. The generation guard no-ops the decrement when a re-register already
    -- owns a fresh counter.
    if (tonumber(fence.generation) or 1) ~= generation then
        apply_tx:rollback()
        local uncount_err = uncount_superseded_attempt(db, dbtype, cid, generation)
        if uncount_err then
            return nil, uncount_err
        end
        return { cursor_id = cid, status = status, processed = processed, last_seq = prefetch_last, target_seq = target_seq, done = true, stale = true, reason = "generation_mismatch" }, nil
    end
    if not fence_matches(fence, {
        generation = generation,
        dispatch_token = dispatch_token,
        locked_by = runner_id,
        previous_last_seq = current_last,
    }) then
        apply_tx:rollback()
        local uncount_err = uncount_superseded_attempt(db, dbtype, cid, generation)
        if uncount_err then
            return nil, uncount_err
        end
        return { cursor_id = cid, status = tostring(fence.status), processed = 0, last_seq = tonumber(fence.last_seq) or 0, target_seq = target_seq, done = true, stale = true, reason = "fence_mismatch" }, nil
    end

    worker_output.new_last = prefetch_last
    local new_last, next_status, persist_err = persist_advance(
        apply_tx, now, dbtype,
        tostring(cursor.projection_id), tostring(cursor.id), body, binding, input_mode,
        current_last, target_seq, hydration_state, tonumber(cursor.target_seq), worker_output
    )
    if persist_err then
        apply_tx:rollback()
        if errors.is(persist_err, errors.INVALID) then
            -- Invalid worker output is a terminal output fault, so it stays counted.
            return release_worker_failure("projection worker output invalid: " .. (persist_err :: error):message(), false)
        end
        return nil, persist_err
    end

    local wake_thread_ids, append_err = append_writes_tx(apply_tx, dbtype, append_events)
    if append_err then
        apply_tx:rollback()
        -- The append apply can fail on a transient DB fault (deadlock/serialization);
        -- classify it so such a failure un-counts rather than climbing the ceiling.
        return release_worker_failure("projection worker append_events apply failed: " .. (append_err :: error):message(), is_retryable(append_err))
    end

    local _, commit_err = apply_tx:commit()
    if commit_err then
        return nil, (errors.new({ message = "failed to commit catchup: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    wake_appended_threads(wake_thread_ids)

    return {
        cursor_id = cid,
        status = next_status,
        processed = processed,
        last_seq = new_last,
        target_seq = target_seq,
        done = new_last >= target_seq,
    }, nil
end

-- Options reserve_proc accepts. runner_id stamps locked_by so apply_proc can fence
-- on the exact runner that reserved the dispatch.
type ReserveOpts = {
    cursor_id: string,
    runner_id: string,
}

-- reserve_proc runs the shared prepare for one cursor and, when there is work,
-- RESERVES it for the runner instead of clearing the lock: status='running',
-- locked_by=runner_id, locked_at=now, dispatch_token=<new uuid>, dispatch_after
-- cleared — all in the prepare transaction. Returns (reservation, terminal,
-- error): reservation carries the worker_input + apply_ctx the runner executes
-- and applies with; terminal carries the prepare's TickResult when it parked /
-- invalidated / was stale / had no work, with no reservation to run.
function catchup.reserve_proc(db: sql.DB, opts: ReserveOpts): (Reservation?, TickResult?, error?)
    local cursor_id = trim(opts.cursor_id)
    if cursor_id == "" then
        return nil, nil, (errors.new({ message = "cursor_id is required", kind = errors.INVALID }) :: error)
    end
    local runner_id = trim(opts.runner_id)
    if runner_id == "" then
        return nil, nil, (errors.new({ message = "runner_id is required", kind = errors.INVALID }) :: error)
    end

    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local now = now_string(dbtype)

    local prep = prepare(db, { cursor_id = cursor_id }, dbtype, now)
    if prep.terminal_err then
        return nil, nil, prep.terminal_err
    end
    if prep.terminal then
        return nil, prep.terminal, nil
    end

    local tx = prep.tx :: sql.Transaction
    local cursor = prep.cursor :: table
    local binding = prep.binding :: table

    -- Never reconcile a derived lane or re-arm a missing schedule underneath an
    -- in-flight worker. A running NULL-dispatch row is owned by its existing
    -- fencing token; reclaim/release decides when it becomes eligible again.
    if has_active_lease(cursor) then
        tx:rollback()
        return nil, { cursor_id = tostring(cursor.id), status = types.CURSOR_STATUS.RUNNING, processed = 0, last_seq = prep.current_last :: number, target_seq = prep.target_seq :: number, done = true, skipped = true, reason = "already_reserved" }, nil
    end

    -- worker_runtime is a denormalized claim index. If a hot declaration change
    -- moved this cursor out of the proc lane, repair the index under the same row
    -- lock without executing it in the wrong topology. Re-arm only a lost NULL
    -- schedule, leaving deliberate future dispatches intact; the inline
    -- dispatcher can then discover the canonical func:// cursor.
    if tostring(binding.worker_runtime) ~= types.WORKER_RUNTIME.PROC then
        local repair_err = reconcile_worker_runtime(tx, cursor, binding, now)
        if repair_err then
            tx:rollback()
            return nil, nil, repair_err
        end
        local _, commit_err = tx:commit()
        if commit_err then
            return nil, nil, (errors.new({ message = "failed to commit projection worker runtime repair: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
        end
        return nil, {
            cursor_id = tostring(cursor.id),
            status = tostring(cursor.status),
            processed = 0,
            last_seq = prep.current_last :: number,
            target_seq = prep.target_seq :: number,
            done = true,
            skipped = true,
            reason = "worker_runtime_reconciled",
        }, nil
    end

    -- Claim-race guard. claim_proc selects a candidate under SKIP LOCKED then
    -- commits before reserve_proc re-locks, so two runners can both target the
    -- same row. prepare re-locks it with FOR UPDATE OF c SKIP LOCKED: a peer still
    -- mid-claim under the lock is skipped there (a terminal outcome the runner
    -- polls past), and once that peer commits its reservation this prepare reads
    -- the row and the has_active_lease check above yields "already_reserved"
    -- rather than issuing a second fencing token — that would let both run the
    -- worker (idempotent, but wasted capacity). A dead peer's reservation is not
    -- seen here because reclaim re-stamps dispatch_after before this row becomes
    -- claimable again.
    local dispatch_token = uuid.v7()

    -- Reserve under the prepare lock: claim the cursor for this runner with a
    -- fresh fencing token. dispatch_after is cleared so the scheduler/claim scan
    -- does not re-select an in-flight cursor.
    -- Count this dispatch durably before the runner executes the worker (see the
    -- inline reserve above). The fenced apply resets it on success; a benign
    -- fenced-out apply un-counts it (a peer advanced the cursor); only a genuine
    -- terminal fault keeps it climbing toward the dead-letter ceiling.
    local _, reserve_err = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.RUNNING)
        :set("locked_by", runner_id)
        :set("locked_at", now)
        :set("dispatch_token", dispatch_token)
        :set("dispatch_after", nil)
        :set("attempts", sql.builder.expr("attempts + 1"))
        :set("updated_at", now)
        :where(sql.builder.eq({ id = cursor.id }))
        :run_with(tx):exec()
    if reserve_err then
        tx:rollback()
        return nil, nil, (errors.new({ message = "failed to reserve proc cursor: " .. tostring(reserve_err), kind = errors.INTERNAL }) :: error)
    end

    local _, commit_err = tx:commit()
    if commit_err then
        return nil, nil, (errors.new({ message = "failed to commit proc reservation: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end

    local apply_ctx: ApplyCtx = {
        cursor_id = tostring(cursor.id),
        projection_id = tostring(cursor.projection_id),
        thread_id = tostring(cursor.thread_id),
        generation = prep.generation :: number,
        dispatch_token = dispatch_token,
        runner_id = runner_id,
        previous_last_seq = prep.current_last :: number,
        new_last = prep.new_last :: number,
        target_seq = prep.target_seq :: number,
        hydration_state = prep.hydration_state :: string,
        explicit_target_seq = tonumber(cursor.target_seq),
        binding = prep.binding :: table,
        input_mode = prep.input_mode :: string,
        body = prep.body :: table,
        trace_context = ((prep.worker_input :: table).trace_context :: table?),
    }

    return { worker_input = prep.worker_input :: table, apply_ctx = apply_ctx }, nil, nil
end

-- claim_proc claims ONE due proc cursor for runner_id: it selects the oldest-due
-- proc cursor (FOR UPDATE OF c SKIP LOCKED on postgres so peers skip a cursor
-- already being claimed; sqlite's single writer needs no lock clause) and
-- reserves it through reserve_proc.
-- Returns (reservation, error): nil reservation with nil error means the queue is
-- empty. A cursor selected whose prepare reached a terminal (parked / no longer
-- due) yields nil so the runner polls again.
function catchup.claim_proc(db: sql.DB, runner_id: string): (Reservation?, error?)
    local rid = trim(runner_id)
    if rid == "" then
        return nil, (errors.new({ message = "runner_id is required", kind = errors.INVALID }) :: error)
    end

    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local now = now_string(dbtype)
    -- Pool scan for the oldest due proc cursor; the claim clause skips a cursor a
    -- peer is already claiming so runners never block/deadlock on each other.
    local lock_suffix = row_lock(dbtype, "claim")

    for _ = 1, 8 do
        local tx, tx_err = db:begin()
        if tx_err then
            return nil, (errors.new({ message = "failed to begin claim transaction: " .. tostring(tx_err), kind = errors.INTERNAL }) :: error)
        end

        local select_query = sql.builder.select("c.id")
            :from("kickside_projection_cursor c")
            :where(sql.builder.expr("c.worker_runtime = ?", types.WORKER_RUNTIME.PROC))
            :where(sql.builder.expr("c.status IN ('pending', 'running', 'live', 'caught_up')"))
            :where("c.locked_by IS NULL")
            :where("c.dispatch_token IS NULL")
            -- Normal appends stamp dispatch_after. Keep a self-healing escape
            -- hatch for an already-live cursor whose stamp was lost (for
            -- example, a process stopped between an event commit and its wake):
            -- it is observably behind its thread head, so it is due even without
            -- a timestamp. reserve_proc then repairs any stale concrete target
            -- before the worker is invoked. Do not include pending/running rows
            -- here: a null stamp on those states can mean an in-flight lease.
            --
            -- Clauses are joined with AND and are not parenthesised by the
            -- builder, so this disjunction carries its own outer parentheses:
            -- without them AND binds tighter than OR and the second branch
            -- stands alone as a top-level alternative, matching cursors of any
            -- runtime and lease state.
            :where(sql.builder.expr(
                "((c.dispatch_after IS NOT NULL AND c.dispatch_after <= ?) OR " ..
                "(c.dispatch_after IS NULL AND c.status IN ('live', 'caught_up') " ..
                "AND c.last_seq < (SELECT t.event_count FROM kickside_thread t WHERE t.id = c.thread_id)))",
                now
            ))
            :order_by("c.dispatch_after ASC")
            :limit(1)
        if lock_suffix ~= "" then
            select_query = select_query:suffix(lock_suffix)
        end
        local rows, select_err = select_query:run_with(tx):query()
        if select_err then
            tx:rollback()
            return nil, (errors.new({ message = "failed to select due proc cursor: " .. tostring(select_err), kind = errors.INTERNAL }) :: error)
        end
        if not rows or #rows == 0 then
            tx:commit()
            return nil, nil
        end
        local candidate_id = tostring(rows[1].id)
        -- Release the candidate-selection lock before reserve_proc opens its own
        -- prepare transaction (a nested begin on the same connection is unsupported);
        -- on postgres the SKIP LOCKED above already steered peers to other rows, and
        -- reserve_proc re-takes the row lock with a generation/token fence so a peer
        -- that slipped in cannot double-reserve.
        local _, commit_err = tx:commit()
        if commit_err then
            return nil, (errors.new({ message = "failed to commit claim selection: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
        end

        local reservation, terminal, reserve_err = catchup.reserve_proc(db, { cursor_id = candidate_id, runner_id = rid })
        if reserve_err then
            return nil, reserve_err
        end
        if reservation then
            return reservation, nil
        end
        if not terminal then
            return nil, nil
        end
        -- The candidate parked / went stale between selection and reservation.
        -- Keep scanning a bounded number of rows so one broken child cannot make
        -- the proc runner report an empty queue while other due work is ready.
    end

    return nil, nil
end

-- apply_proc is the FENCED apply for a proc dispatch the runner executed. It
-- re-opens a transaction, locks the cursor row, and REJECTS (rollback, returning
-- a stale/obsolete TickResult, never an error) unless every fence field still
-- matches the reservation: same row id, generation, dispatch_token, lease holder,
-- running status and last_seq. On a full match it persists the worker body patch,
-- advances last_seq (clamped exactly as tick), sets the next status/target, and
-- clears the lease (locked_by/locked_at/dispatch_token).
function catchup.apply_proc(db: sql.DB, apply_ctx: ApplyCtx, worker_output: table): (TickResult?, error?)
    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local now = now_string(dbtype)
    -- Fence re-read of the single cursor row this runner reserved; plain lock.
    local lock = row_lock(dbtype, "fence")

    local apply_tx, apply_tx_err = db:begin()
    if apply_tx_err then
        return nil, (errors.new({ message = "failed to begin proc apply transaction: " .. tostring(apply_tx_err), kind = errors.INTERNAL }) :: error)
    end

    local fence_query = sql.builder.select("id", "generation", "dispatch_token", "locked_by", "status", "last_seq")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({ id = apply_ctx.cursor_id }))
    if lock ~= "" then
        fence_query = fence_query:suffix(lock)
    end
    local fence_rows, fence_err = fence_query:run_with(apply_tx):query()
    if fence_err then
        apply_tx:rollback()
        return nil, (errors.new({ message = "failed to re-check proc fence: " .. tostring(fence_err), kind = errors.INTERNAL }) :: error)
    end
    if not fence_rows or #fence_rows == 0 then
        apply_tx:rollback()
        return { cursor_id = apply_ctx.cursor_id, status = types.CURSOR_STATUS.INVALID, processed = 0, last_seq = apply_ctx.new_last, target_seq = apply_ctx.target_seq, done = true, obsolete = true, reason = "cursor_not_found" }, nil
    end
    local row = fence_rows[1]
    if not fence_matches(row, {
        generation = apply_ctx.generation,
        dispatch_token = apply_ctx.dispatch_token,
        locked_by = apply_ctx.runner_id,
        previous_last_seq = apply_ctx.previous_last_seq,
    }) then
        apply_tx:rollback()
        -- The worker ran and a peer moved the cursor on: un-count this dispatch's
        -- pre-run attempt so a benign supersede never climbs toward the dead-letter
        -- ceiling. The generation guard no-ops the decrement when a re-register
        -- already owns a fresh counter.
        local uncount_err = uncount_superseded_attempt(db, dbtype, apply_ctx.cursor_id, apply_ctx.generation)
        if uncount_err then
            return nil, uncount_err
        end
        return { cursor_id = apply_ctx.cursor_id, status = tostring(row.status), processed = 0, last_seq = tonumber(row.last_seq) or 0, target_seq = apply_ctx.target_seq, done = true, stale = true, reason = "fence_mismatch" }, nil
    end

    local append_events, append_validate_err = normalize_append_writes(worker_output, apply_ctx.thread_id, body_event_filter_types(apply_ctx.body), apply_ctx.trace_context)
    if append_validate_err then
        apply_tx:rollback()
        return nil, append_validate_err
    end

    worker_output.new_last = apply_ctx.new_last
    local new_last, next_status, persist_err = persist_advance(
        apply_tx, now, dbtype,
        apply_ctx.projection_id, apply_ctx.cursor_id, apply_ctx.body, apply_ctx.binding, apply_ctx.input_mode,
        apply_ctx.previous_last_seq, apply_ctx.target_seq, apply_ctx.hydration_state, apply_ctx.explicit_target_seq,
        worker_output
    )
    if persist_err then
        apply_tx:rollback()
        return nil, persist_err
    end

    local wake_thread_ids, append_err = append_writes_tx(apply_tx, dbtype, append_events)
    if append_err then
        apply_tx:rollback()
        return nil, append_err
    end

    local _, commit_err = apply_tx:commit()
    if commit_err then
        return nil, (errors.new({ message = "failed to commit proc apply: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    wake_appended_threads(wake_thread_ids)

    return {
        cursor_id = apply_ctx.cursor_id,
        status = next_status,
        processed = 0,
        last_seq = new_last,
        target_seq = apply_ctx.target_seq,
        done = new_last >= apply_ctx.target_seq,
    }, nil
end

-- release_proc hands a reserved cursor back for retry after the runner's worker
-- failed (error or panic), under the SAME fence as apply_proc so a runner that
-- already lost its lease cannot disturb a reassigned dispatch. On a fence match it
-- clears the lease (locked_by/locked_at/dispatch_token), parks the cursor back to
-- pending, and stamps dispatch_after with a configured backoff so the next claim
-- picks it up promptly but not instantly. The cursor head is NOT advanced — the
-- batch is retried. A fence miss is a no-op (reclaim/another runner owns it).
function catchup.release_proc(db: sql.DB, apply_ctx: ApplyCtx, reason: any): (boolean, error?)
    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return false, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local now = now_string(dbtype)
    -- Fence re-read of the single cursor row this runner reserved; plain lock.
    local lock = row_lock(dbtype, "fence")

    local rtx, rtx_err = db:begin()
    if rtx_err then
        return false, (errors.new({ message = "failed to begin proc release transaction: " .. tostring(rtx_err), kind = errors.INTERNAL }) :: error)
    end

    local fence_query = sql.builder.select("generation", "dispatch_token", "locked_by", "status")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({ id = apply_ctx.cursor_id }))
    if lock ~= "" then
        fence_query = fence_query:suffix(lock)
    end
    local fence_rows, fence_err = fence_query:run_with(rtx):query()
    if fence_err then
        rtx:rollback()
        return false, (errors.new({ message = "failed to re-check proc release fence: " .. tostring(fence_err), kind = errors.INTERNAL }) :: error)
    end
    if not fence_rows or #fence_rows == 0 then
        rtx:rollback()
        return false, nil
    end
    local row = fence_rows[1]
    -- The release fence does not advance last_seq (and this SELECT omits it), so
    -- previous_last_seq is left unset and only the four lease-identity legs match.
    if not fence_matches(row, {
        generation = apply_ctx.generation,
        dispatch_token = apply_ctx.dispatch_token,
        locked_by = apply_ctx.runner_id,
    }) then
        rtx:rollback()
        return false, nil
    end

    -- The fence still holds, so this dispatch's pre-run increment is the one on
    -- the row. A retryable failure un-counts it so a transient fault never climbs
    -- toward the dead-letter ceiling; a terminal failure keeps the count. The
    -- reason arrives as the wrapped worker error so its kind/retryable survive the
    -- runner hop; its message text is what the cursor records.
    local reason_text: string
    if reason == nil then
        reason_text = "proc worker failed"
    elseif type(reason) == "userdata" then
        reason_text = ((reason :: any):message() :: string)
    else
        reason_text = tostring(reason)
    end
    local release = sql.builder.update("kickside_projection_cursor")
        :set("status", types.CURSOR_STATUS.PENDING)
        :set("locked_by", nil)
        :set("locked_at", nil)
        :set("dispatch_token", nil)
        :set("dispatch_after", sql.builder.expr(retry_expr(dbtype, proj_env.proc_release_retry_seconds())))
        :set("last_error", clamp_last_error(reason_text))
        :set("updated_at", now)
        :where(sql.builder.eq({ id = apply_ctx.cursor_id }))
    if is_retryable(reason) then
        release = release:set("attempts", sql.builder.expr("attempts - 1"))
    end
    local _, update_err = release:run_with(rtx):exec()
    if update_err then
        rtx:rollback()
        return false, (errors.new({ message = "failed to release proc cursor: " .. tostring(update_err), kind = errors.INTERNAL }) :: error)
    end

    local _, commit_err = rtx:commit()
    if commit_err then
        return false, (errors.new({ message = "failed to commit proc release: " .. tostring(commit_err), kind = errors.INTERNAL }) :: error)
    end
    return true, nil
end

-- fence_alive is a read-only liveness check the supervised runner polls between
-- worker stages: it reports whether the cursor still belongs to this dispatch
-- (same generation + dispatch_token + runner, still running). Unlike heartbeat it
-- does NOT refresh the lease (the supervisor owns DB heartbeats); it only answers
-- whether the in-flight worker still holds the fence, so a stolen/reclaimed lease
-- lets the runner terminate the now-orphaned child instead of applying its result.
function catchup.fence_alive(db: sql.DB, apply_ctx: ApplyCtx): (boolean, error?)
    local rows, err = sql.builder.select("status")
        :from("kickside_projection_cursor")
        :where(sql.builder.eq({
            id = apply_ctx.cursor_id,
            generation = apply_ctx.generation,
            dispatch_token = apply_ctx.dispatch_token,
            locked_by = apply_ctx.runner_id,
            status = types.CURSOR_STATUS.RUNNING,
        }))
        :limit(1)
        :run_with(db):query()
    if err then
        return false, (errors.new({ message = "failed to check proc fence liveness: " .. tostring(err), kind = errors.INTERNAL }) :: error)
    end
    return rows ~= nil and #rows > 0, nil
end

-- heartbeat_proc extends a reserved proc cursor's lease: it stamps locked_at=now
-- only while the fence still holds (matching id + generation + dispatch_token +
-- runner_id and still running). Returns (true) while the runner still owns the
-- lease, (false) once the guarded update touches no rows — the lease was lost
-- (reclaimed or the cursor advanced/re-registered) and the runner must abandon
-- its attempt.
function catchup.heartbeat_proc(db: sql.DB, cursor_id: string, generation: number, dispatch_token: string, runner_id: string): (boolean, error?)
    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return false, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local now = now_string(dbtype)

    local result, update_err = sql.builder.update("kickside_projection_cursor")
        :set("locked_at", now)
        :set("updated_at", now)
        :where(sql.builder.eq({
            id = cursor_id,
            generation = generation,
            dispatch_token = dispatch_token,
            locked_by = runner_id,
            status = types.CURSOR_STATUS.RUNNING,
        }))
        :run_with(db):exec()
    if update_err then
        return false, (errors.new({ message = "failed to heartbeat proc lease: " .. tostring(update_err), kind = errors.INTERNAL }) :: error)
    end
    local affected = result and (tonumber(result.rows_affected) or 0) or 0
    return affected > 0, nil
end

local HEARTBEAT_KEY_SEP = string.char(31)

local function heartbeat_key(cursor_id: string, generation: number, dispatch_token: string, runner_id: string): string
    return cursor_id .. HEARTBEAT_KEY_SEP .. tostring(generation) .. HEARTBEAT_KEY_SEP .. dispatch_token .. HEARTBEAT_KEY_SEP .. runner_id
end

local function normalized_heartbeat_leases(leases: { table }): { table }
    local normalized: { table } = {}
    for index, raw in ipairs(leases or {}) do
        local lease = type(raw) == "table" and raw or {}
        local cursor_id = trim(lease.cursor_id)
        local generation = math.floor(tonumber(lease.generation) or 0)
        local dispatch_token = trim(lease.dispatch_token)
        local runner_id = trim(lease.runner_id)
        if cursor_id ~= "" and generation > 0 and dispatch_token ~= "" and runner_id ~= "" then
            local key = heartbeat_key(cursor_id, generation, dispatch_token, runner_id)
            normalized[#normalized + 1] = {
                cursor_id = cursor_id,
                generation = generation,
                dispatch_token = dispatch_token,
                runner_id = runner_id,
                key = key,
                index = index,
            }
        end
    end
    return normalized
end

local function heartbeat_proc_many_postgres(db: sql.DB, leases: { table }, now: string): ({ [integer]: boolean }?, error?)
    local values_sql: { string } = {}
    local params: { any } = {}
    for i, lease in ipairs(leases) do
        local base = (i - 1) * 4
        values_sql[#values_sql + 1] = "($" .. tostring(base + 1) .. "::uuid, $" .. tostring(base + 2) .. "::bigint, $" .. tostring(base + 3) .. "::text, $" .. tostring(base + 4) .. "::text)"
        params[#params + 1] = tostring(lease.cursor_id)
        params[#params + 1] = tonumber(lease.generation) or 0
        params[#params + 1] = tostring(lease.dispatch_token)
        params[#params + 1] = tostring(lease.runner_id)
    end

    local now_param = #params + 1
    params[now_param] = now
    local status_param = #params + 1
    params[status_param] = types.CURSOR_STATUS.RUNNING

    local query = [[
        WITH heartbeat_input(cursor_id, generation, dispatch_token, runner_id) AS (
            VALUES ]] .. table.concat(values_sql, ", ") .. [[
        ),
        updated AS (
            UPDATE kickside_projection_cursor c
            SET locked_at = $]] .. tostring(now_param) .. [[::timestamptz,
                updated_at = $]] .. tostring(now_param) .. [[::timestamptz
            FROM heartbeat_input h
            WHERE c.id = h.cursor_id
              AND c.generation = h.generation
              AND c.dispatch_token = h.dispatch_token
              AND c.locked_by = h.runner_id
              AND c.status = $]] .. tostring(status_param) .. [[
            RETURNING c.id, c.generation, c.dispatch_token, c.locked_by
        )
        SELECT id, generation, dispatch_token, locked_by FROM updated
    ]]

    local rows, query_err = db:query(query, params)
    if query_err then
        return nil, (errors.new({ message = "failed to heartbeat proc leases: " .. tostring(query_err), kind = errors.INTERNAL }) :: error)
    end

    local alive_by_key: { [string]: boolean } = {}
    for _, row in ipairs(rows or {}) do
        alive_by_key[heartbeat_key(tostring(row.id), tonumber(row.generation) or 0, tostring(row.dispatch_token), tostring(row.locked_by))] = true
    end

    local alive_by_index: { [integer]: boolean } = {}
    for _, lease in ipairs(leases) do
        alive_by_index[tonumber(lease.index) or 0] = alive_by_key[tostring(lease.key)] == true
    end
    return alive_by_index, nil
end

-- heartbeat_proc_many extends a batch of reserved proc leases through one
-- storage contract. Postgres uses one fenced UPDATE...FROM(VALUES) round trip;
-- sqlite deliberately keeps the simple per-row path because local profiles run
-- tiny worker pools and benefit more from portable behavior than SQL cleverness.
function catchup.heartbeat_proc_many(db: sql.DB, leases: { table }): ({ [integer]: boolean }?, error?)
    local dbtype_raw, dbtype_err = db:type()
    if dbtype_err then
        return nil, (errors.new({ message = "failed to resolve db type: " .. tostring(dbtype_err), kind = errors.INTERNAL }) :: error)
    end
    local dbtype = tostring(dbtype_raw)
    local now = now_string(dbtype)
    local normalized = normalized_heartbeat_leases(leases)
    local alive_by_index: { [integer]: boolean } = {}
    for index = 1, #(leases or {}) do
        alive_by_index[index] = false
    end
    if #normalized == 0 then
        return alive_by_index, nil
    end

    if dbtype == sql.type.POSTGRES then
        local postgres_alive, postgres_err = heartbeat_proc_many_postgres(db, normalized, now)
        if postgres_err then
            return nil, postgres_err
        end
        for index = 1, #leases do
            alive_by_index[index] = postgres_alive and postgres_alive[index] == true
        end
        return alive_by_index, nil
    end

    for _, lease in ipairs(normalized) do
        local alive, hb_err = catchup.heartbeat_proc(
            db,
            tostring(lease.cursor_id),
            tonumber(lease.generation) or 0,
            tostring(lease.dispatch_token),
            tostring(lease.runner_id)
        )
        if hb_err then
            return nil, hb_err
        end
        alive_by_index[tonumber(lease.index) or 0] = alive == true
    end
    return alive_by_index, nil
end

return catchup
