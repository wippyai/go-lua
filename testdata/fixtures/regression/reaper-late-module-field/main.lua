-- reaper: the lifecycle thread's projection worker. Each tick prefetches the new
-- deletion reports (kickside.lifecycle.events:principal.deleted / kickside.lifecycle.events:component.deleted) and, for every named owner,
-- deletes each component that owner still owns via the trusted component.system_delete
-- (which runs the impl's own deletable cleanup). Ownership is nested-safe: deleting a
-- component recurses into anything IT owns, so a single principal report tears the
-- whole owned subtree down to its leaves' backing data.
--
-- Idempotent + retriable: list_system shrinks as components go, system_delete no-ops on
-- an already-gone component, and the cursor only advances when the tick returns -- so a
-- failed tick safely re-runs the same window.

local component = require("component")
local consts = require("lifecycle_consts")
local json = require("json")
local registry = require("registry")
local funcs = require("funcs")

local M = {}

-- Bound on nested-ownership recursion: a guard against a pathological ownership cycle,
-- far above any real component tree depth.
local MAX_DEPTH = 64
local CLEANUP_META_TYPE = "kickside.lifecycle.principal_cleanup"

local function decode_body(body: any): { [string]: any }
    if type(body) == "table" then return body :: { [string]: any } end
    if type(body) ~= "string" or body == "" then return {} end
    local decoded, err = json.decode(body :: string)
    if err or type(decoded) ~= "table" then return {} end
    return decoded :: { [string]: any }
end

-- reap_owner deletes every component owned by owner_id, recursing into each deleted
-- component's own ownership subtree. Returns the count deleted at this level + below.
local function reap_owner(owner_id: string, depth: integer): integer
    if depth > MAX_DEPTH then return 0 end
    if type(owner_id) ~= "string" or owner_id == "" then return 0 end

    local rows = component.list_system({
        meta = { [consts.OWNER_META_KEY] = owner_id },
        include = { meta = false },
    })
    local deleted = 0
    for _, row in ipairs(rows or {}) do
        local cid = (row :: any).component_id
        if type(cid) == "string" and cid ~= "" then
            local ok = component.system_delete(cid :: string, "reparent_root")
            if ok then
                deleted = deleted + 1
                -- Recurse: anything the just-deleted component owned is now orphaned.
                deleted = deleted + reap_owner(cid :: string, depth + 1)
            end
        end
    end
    return deleted
end

local function cleanup_target(entry: any): string
    if type(entry) ~= "table" then return "" end
    if type((entry :: any).target) == "string" and (entry :: any).target ~= "" then
        return (entry :: any).target :: string
    end
    if type((entry :: any).id) == "string" then return (entry :: any).id :: string end
    return ""
end

local function run_principal_cleanups(owner_id: string): error?
    if type(owner_id) ~= "string" or owner_id == "" then return nil end
    local entries, find_err = M._registry.find({ ["meta.type"] = CLEANUP_META_TYPE })
    if find_err then
        return (errors.new({ message = "principal cleanup discovery failed: " .. tostring(find_err), kind = errors.INTERNAL }) :: error)
    end
    for _, entry in ipairs(entries or {}) do
        local target = cleanup_target(entry)
        if target ~= "" then
            local result, call_err = M._funcs.call(target, {
                principal_id = owner_id,
                user_id = owner_id,
            })
            if call_err then
                return (errors.new({ message = "principal cleanup failed (" .. target .. "): " .. tostring(call_err), kind = errors.INTERNAL }) :: error)
            end
            if type(result) == "table" and (result :: any).success == false then
                return (errors.new({ message = "principal cleanup failed (" .. target .. "): " .. tostring((result :: any).error or "failed"), kind = errors.INTERNAL }) :: error)
            end
        end
    end
    return nil
end

-- run is the func:// projection worker entry. input.events holds the prefetched batch;
-- input.range.to_seq is the cursor advance. Returns the patched body + the new last_seq.
local function run(input: table): (table?, error?)
    local input_tbl: table = type(input) == "table" and input or {}
    local events: { any } = type(input_tbl.events) == "table" and input_tbl.events or {}
    local range: table = type(input_tbl.range) == "table" and input_tbl.range or {}
    local projection: table = type(input_tbl.projection) == "table" and input_tbl.projection or {}
    local body: table = type(projection.body) == "table" and projection.body or {}

    local reaped = 0
    for _, ev in ipairs(events) do
        local etype = type((ev :: any).type) == "string" and (ev :: any).type or ""
        if etype == consts.EVENT.PRINCIPAL_DELETED or etype == consts.EVENT.COMPONENT_DELETED then
            local payload = decode_body((ev :: any).body)
            local owner_id = tostring(payload.subject_id or "")
            if etype == consts.EVENT.PRINCIPAL_DELETED then
                local cleanup_err = run_principal_cleanups(owner_id)
                if cleanup_err then return nil, cleanup_err end
            end
            reaped = reaped + reap_owner(owner_id, 0)
        end
    end

    local stats: table = type(body.stats) == "table" and body.stats or {}
    stats.reaped = (tonumber(stats.reaped) or 0) + reaped
    body.stats = stats

    return {
        body = body,
        last_seq = tonumber(range.to_seq) or nil,
    }
end

M._registry = registry
M._funcs = funcs
M.run = run

return M

