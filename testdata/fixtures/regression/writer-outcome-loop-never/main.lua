-- The public write seam. writer.execute(estimate_id, command) is the ONLY entry the
-- API/MCP layers and tests use to mutate an estimate. It refuses malformed or oversized
-- commands, refuses an unknown estimate, durably admits the command (payload + canonical
-- payload_hash keyed by (estimate_id, command_id), with a per-estimate in-flight cap),
-- then WAKES the estimate's single command processor with only the command REFERENCE and
-- awaits the durable outcome by polling _outcome. The mailbox only wakes the processor;
-- the result is read from the durable ledger, so the caller needs no routable reply
-- mailbox and a resend of the same command_id observes the same recorded outcome.
-- Conflict (same command_id, different payload) is detected at admission against the
-- stored payload_hash; the slot is released by the processor on a recorded outcome.

local process = require("process")
local time = require("time")
local sql = require("sql")
local security = require("security")
local contract = require("contract")
local ctx = require("ctx")
local json = require("json")
local uuid = require("uuid")
local types = require("types")
local canon = require("canon")
local reader = require("reader")
local component = require("component")

local M = {}

local OUTCOME_ATTEMPTS = 600
local OUTCOME_SLEEP = "20ms"
local DISPATCHER_LOOKUP_ATTEMPTS = 80
local DISPATCHER_LOOKUP_SLEEP = "25ms"
-- claim_next: default lease window, frontier page size, and re-read bound for lost races.
local CLAIM_LEASE_SECONDS = 300
local CLAIM_FRONTIER_LIMIT = 16
local CLAIM_REREAD_TRIES = 4

local function refusal(reason: string, flag: string?): any
    local out: any = { ok = false, error = reason }
    if flag then out[flag] = true end
    return out
end

local function trim(v: any): string
    if type(v) ~= "string" then return "" end
    return (v:gsub("^%s*(.-)%s*$", "%1"))
end

local function actor_identity(): (string, { string })
    local actor = security.actor()
    if not actor then return "", {} end
    local id = ""
    local ok, resolved = pcall(function() return actor:id() end)
    if ok then id = trim(tostring(resolved or "")) end
    local groups: { string } = {}
    pcall(function()
        local g = actor:groups()
        if type(g) == "table" then for _, x in ipairs(g :: { any }) do groups[#groups + 1] = tostring(x) end end
    end)
    return id, groups
end

local function scope_name(): string
    local ok, name = pcall(function()
        local s = security.scope()
        return s and s:name() or ""
    end)
    if ok and trim(tostring(name)) ~= "" then return trim(tostring(name)) end
    return ""
end

local function origin_for(): string
    local rt = trim(ctx.get("runtime_type"))
    if rt == "agent" or rt == "dataflow" or rt == "workflow" then return types.ORIGIN.AGENT end
    return types.ORIGIN.HUMAN
end

-- The processor reconstructs only the durable actor frame, which is insufficient for a
-- component-service ACL lookup. Capture the ambient caller's WRITE entitlement at
-- admission so proposal validation can distinguish a self-authored owner/editor proposal
-- from an attributed scoped-contributor proposal.
local function has_component_write(estimate_id: string): boolean
    local called, _, access_err = pcall(component.validate_access, estimate_id, component.ACCESS.WRITE)
    return called and access_err == nil
end

local function thread_exists(estimate_id: string): (boolean, string?)
    local def, err = contract.get(types.THREADS)
    if err or not def then return false, "threads contract unavailable: " .. tostring(err) end
    local conn, oerr = def:with_actor(security.actor()):with_scope(security.scope()):open()
    if oerr or not conn then return false, "threads open failed: " .. tostring(oerr) end
    local found, ferr = conn:find({ thread_id = estimate_id })
    if ferr then return false, "estimate lookup failed: " .. tostring(ferr) end
    return found ~= nil, nil
end


-- admit inserts the durable admission row under the per-estimate in-flight cap, or reads
-- the payload_hash already admitted for this command_id. Returns
-- (admitted_hash?, cap_refusal?, err?): a non-nil admitted_hash that differs from the
-- caller's hash is a conflict, resolved by the caller.
local function admit(estimate_id: string, command_id: string, payload_hash: string, payload: string): (string?, any?, string?)
    local db, db_err = sql.get(types.db_id())
    if not db then return nil, nil, tostring(db_err or "estimation: db unavailable") end
    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, nil, tostring(tx_err) end

    local existing, eerr = tx:query("SELECT payload_hash FROM " .. types.T.ADMISSION .. " WHERE estimate_id=$1 AND command_id=$2", { estimate_id, command_id })
    if eerr then tx:rollback(); db:release(); return nil, nil, tostring(eerr) end
    if #(existing :: { any }) > 0 then
        local admitted = tostring((existing[1] :: any).payload_hash or "")
        local _, cerr = tx:commit()
        db:release()
        if cerr then return nil, nil, tostring(cerr) end
        return admitted, nil, nil
    end

    local counts, cerr = tx:query("SELECT COUNT(*) AS n FROM " .. types.T.ADMISSION .. " WHERE estimate_id=$1 AND state='pending'", { estimate_id })
    if cerr then tx:rollback(); db:release(); return nil, nil, tostring(cerr) end
    if (tonumber((counts[1] :: any).n) or 0) >= types.MAX_INFLIGHT then
        tx:rollback(); db:release()
        return nil, refusal("estimate is busy: too many commands in flight", "busy"), nil
    end
    local _, ierr = tx:execute(
        "INSERT INTO " .. types.T.ADMISSION .. " (estimate_id, command_id, payload_hash, payload, state, created_at) VALUES ($1,$2,$3,$4,'pending',$5)",
        { estimate_id, command_id, payload_hash, payload, types.now() })
    if ierr then tx:rollback(); db:release(); return nil, nil, tostring(ierr) end

    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, nil, tostring(commit_err) end
    return payload_hash, nil, nil
end

local function read_outcome(estimate_id: string, command_id: string): (any?, string?)
    local db, db_err = sql.get(types.db_id())
    if not db then return nil, tostring(db_err or "estimation: db unavailable") end
    local rows, err = db:query("SELECT payload_hash, status, result_json FROM " .. types.T.OUTCOME .. " WHERE estimate_id=$1 AND command_id=$2", { estimate_id, command_id })
    db:release()
    if err then return nil, tostring(err) end
    return (rows or {})[1], nil
end

local function reply_from_outcome(row: any, replayed: boolean): any
    local status = tostring((row :: any).status or "")
    local result = json.decode(tostring((row :: any).result_json or "{}"))
    if type(result) ~= "table" then result = {} end
    if status == "refused" then
        return { ok = false, refused = true, error = tostring((result :: any).reason or "refused"), replayed = replayed }
    end
    if status ~= "done" then
        return { ok = false, error = tostring((result :: any).reason or ("command outcome: " .. status)), replayed = replayed }
    end
    return { ok = true, status = status, affected = type((result :: any).affected) == "table" and (result :: any).affected or {}, replayed = replayed }
end

-- with_history augments a successful command reply with the caller's undo/redo availability
-- (cheap: one indexed read), so the surface can enable its controls straight off the reply.
local function with_history(reply: any, estimate_id: string, actor_id: string): any
    if type(reply) == "table" and (reply :: any).ok then
        local flags = reader.history_flags(estimate_id, actor_id)
        if type(flags) == "table" then
            ;(reply :: any).can_undo = (flags :: any).can_undo == true
            ;(reply :: any).can_redo = (flags :: any).can_redo == true
        end
    end
    return reply
end

local function lookup_dispatcher(): string?
    for _ = 1, DISPATCHER_LOOKUP_ATTEMPTS do
        local pid = process.registry.lookup(types.DISPATCHER_NAME)
        if pid then return tostring(pid) end
        time.sleep(DISPATCHER_LOOKUP_SLEEP)
    end
    return nil
end

-- wake sends the command reference to the estimate's processor (spawning it via the
-- dispatcher on the cold path). It does not await a reply.
local function wake(estimate_id: string, msg: any): string?
    local name = types.REGISTRY_PREFIX .. estimate_id
    local pid = process.registry.lookup(name)
    if pid and process.send(tostring(pid), "command", msg) then return nil end
    local disp = lookup_dispatcher()
    if not disp then return "estimation dispatcher unavailable" end
    if not process.send(disp, "spawn", { estimate_id = estimate_id, bootstrap = { topic = "command", message = msg } }) then
        return "estimation dispatcher send failed"
    end
    return nil
end

-- execute admits and runs one command against estimate_id. command = { command_id, ops }.
function M.execute(estimate_id: string, command: any): any
    if type(estimate_id) ~= "string" or estimate_id == "" then return refusal("estimate_id is required") end
    if type(command) ~= "table" then return refusal("command is required") end
    local command_id = trim((command :: any).command_id)
    if command_id == "" then return refusal("command_id is required") end
    local ops = (command :: any).ops
    if type(ops) ~= "table" or #ops == 0 then return refusal("command requires a non-empty ops array") end
    if #ops > types.MAX_COMMAND_OPS then return refusal("command exceeds MAX_COMMAND_OPS", "too_large") end

    local actor_id, groups = actor_identity()
    if actor_id == "" then return refusal("an authenticated actor is required") end

    local exists, texists_err = thread_exists(estimate_id)
    if texists_err then return refusal(texists_err) end
    if not exists then return refusal("unknown estimate: " .. estimate_id, "unknown") end

    local payload_hash = canon.hash({ ops = ops })
    local payload = json.encode({
        ops = ops, actor_id = actor_id, origin = origin_for(), groups = groups, scope_name = scope_name(),
        write_authorized = has_component_write(estimate_id),
    })
    if #payload > types.MAX_COMMAND_BYTES then return refusal("command exceeds MAX_COMMAND_BYTES", "too_large") end

    local admitted_hash, cap_refusal, admit_err = admit(estimate_id, command_id, payload_hash, payload)
    if admit_err then return refusal("admission failed: " .. admit_err) end
    if cap_refusal then return cap_refusal end
    if admitted_hash ~= payload_hash then
        return refusal("command_id conflict: submitted payload differs from the admitted command", "conflict")
    end

    -- Already committed: replay the recorded outcome without waking the processor.
    local committed, cerr = read_outcome(estimate_id, command_id)
    if cerr then return refusal(cerr) end
    if committed then return with_history(reply_from_outcome(committed, true), estimate_id, actor_id) end

    local wake_err = wake(estimate_id, { command_id = command_id, payload_hash = payload_hash })
    if wake_err then return refusal(wake_err) end

    for _ = 1, OUTCOME_ATTEMPTS do
        local outcome, oerr = read_outcome(estimate_id, command_id)
        if oerr then return refusal(oerr) end
        if outcome then return with_history(reply_from_outcome(outcome, false), estimate_id, actor_id) end
        time.sleep(OUTCOME_SLEEP)
    end
    return refusal("processor timeout awaiting outcome for command " .. command_id, "timeout")
end

local function lease_deadline(seconds: number): string
    return os.date("!%Y-%m-%dT%H:%M:%SZ", os.time() + seconds) :: string
end

-- claim_next reads the frontier and acquires the first node exclusively through the processor.
-- A lost race (a live claim already present) is refused by the processor, so it advances to the
-- next frontier node; the loop is bounded and re-reads the frontier a few times before yielding
-- an empty result.
function M.claim_next(estimate_id: string, holder: any, opts: any): any
    if type(estimate_id) ~= "string" or estimate_id == "" then return refusal("estimate_id is required") end
    local holder_id = trim(holder)
    if holder_id == "" then return refusal("holder is required") end
    opts = type(opts) == "table" and opts or {}
    local lease_seconds = tonumber((opts :: any).lease_seconds) or CLAIM_LEASE_SECONDS

    for _ = 1, CLAIM_REREAD_TRIES do
        local front, ferr = reader.frontier(estimate_id, { limit = CLAIM_FRONTIER_LIMIT })
        if not front then return refusal(tostring(ferr)) end
        local nodes = type((front :: any).nodes) == "table" and (front :: any).nodes or {}
        if #nodes == 0 then return { ok = true, claimed = false } end
        for _, n in ipairs(nodes :: { any }) do
            local lease_until = lease_deadline(lease_seconds)
            local command = { command_id = uuid.v7(), ops = { {
                op = "claim.acquire", node_id = tostring((n :: any).node_id), holder_id = holder_id, lease_until = lease_until,
            } } }
            local reply = M.execute(estimate_id, command)
            if (reply :: any).ok then
                return { ok = true, claimed = true, node_id = tostring((n :: any).node_id), holder = holder_id, lease_until = lease_until }
            end
        end
    end
    return { ok = true, claimed = false }
end

return M

