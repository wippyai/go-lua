-- Write facade over the event-sourced inbox. Each mutation appends an immutable
-- thread event. Public summary counts and realtime invalidation are maintained by
-- the inbox projection via canonical component public meta.

local inbox_types = require("inbox_types")
local inbox_events = require("inbox_events")
local inbox_delivery = require("inbox_delivery")

local writer = {}

local function events()
    return writer._events or inbox_events
end

local function delivery()
    return writer._delivery or inbox_delivery
end

function writer.create(params: any): (any, string?)
    local id, err = events().create(params)
    if not id then return nil, err end
    local data = { id = id, item_type = params.item_type, title = params.title }
    return data, nil
end

-- deliver posts one delivery of a source item: params carries the item fields plus
-- origin / item_key / dedup_key. Returns { id, outcome } where outcome is "created"
-- for the delivery that minted the item and "updated" for a later one.
function writer.deliver(params: any): (any, string?)
    local result, err = delivery().deliver(params)
    if not result then return nil, err end
    return { id = (result :: any).item_id, outcome = (result :: any).outcome }, nil
end

-- resolve_delivery answers the item a delivery identity names, reading the log so
-- a retraction never depends on the read model having caught up.
function writer.resolve_delivery(origin: string, item_key: string, dedup_key: string): (string?, string?)
    return delivery().resolve(origin, item_key, dedup_key)
end

local function resolve_to(id: string, status: string, decision: string, meta: any?): (any, string?)
    local payload: { [string]: any } = {}
    if type(meta) == "table" then
        for k, v in pairs(meta) do payload[k] = v end
    end
    payload.decision = decision
    local ok, err = events().resolve(id, status, payload)
    if not ok then return nil, err end
    local data = { id = id, status = status, decision = payload.decision, resolved_by = payload.resolved_by }
    return data, nil
end

function writer.approve(id: string, meta: any?): (any, string?)
    return resolve_to(id, inbox_types.STATUS.APPROVED, "approve", meta)
end

function writer.reject(id: string, meta: any?): (any, string?)
    return resolve_to(id, inbox_types.STATUS.REJECTED, "reject", meta)
end

function writer.dismiss(id: string, meta: any?): (any, string?)
    return resolve_to(id, inbox_types.STATUS.DISMISSED, "dismiss", meta)
end

function writer.delete(id: string): (any, string?)
    local ok, err = events().delete(id)
    if not ok then return nil, err end
    local data = { id = id }
    return data, nil
end

-- Discard pending items whose metadata contains every supplied key/value. This is
-- a lifecycle cleanup seam for owners that created correlated inbox work (for
-- example, a deleted import session). It folds the owner's current projection and
-- emits the normal item.deleted event for each match; it never reaches across
-- inbox owners or mutates a resolved item.
function writer.discard_by_metadata(match: any): (any, string?)
    if type(match) ~= "table" or next(match) == nil then return nil, "metadata match is required" end
    local all, read_err = events().reduce()
    if read_err then return nil, read_err end
    local discarded = 0
    for id, item in pairs(type(all) == "table" and all or {}) do
        local metadata = type((item :: any).metadata) == "table" and (item :: any).metadata or {}
        local matches = (item :: any).status == inbox_types.STATUS.PENDING
        if matches then
            for key, value in pairs(match) do
                if metadata[key] ~= value then matches = false; break end
            end
        end
        if matches then
            local ok, err = events().delete(id)
            if not ok then return nil, err end
            discarded = discarded + 1
        end
    end
    return { discarded = discarded }, nil
end

return writer

