-- The delivery protocol for inbox items. A receipt is not a side table: the
-- delivery IS the external identity of the event it appends --
--   external_source  the writing origin
--   external_id      the stable source item_key
--   external_version the delivery dedup_key (the pullable contract's idempotency
--                    key for one delivery, stable across cursor replay)
-- so the durable log is the only receipt store, and the engine's uniqueness on
-- that identity is the only arbiter.
--
-- Every write reads the log FIRST, across both event types an item's identity can
-- carry (its genesis and its later versions), and answers from what stands:
--   * the same delivery again -> the standing item id, nothing appended;
--   * a new delivery of the same source item -> a version event the projection
--     folds onto the item (fold order is sequence order, so A -> B -> A is three
--     deliveries, three events, and the last body stands);
--   * one delivery key carrying a different body -> a refusal, because the source
--     broke its own idempotency contract and adopting the collision would attach a
--     body to an item it does not describe;
--   * an ambiguous append (the identity landed under a peer) -> the same lookup
--     again, and the answer is whatever now stands.
-- The visible item id is minted here and rides the genesis body; it is never the
-- source key, so two writers delivering one source key keep separate items.

local json = require("json")
local hash = require("hash")
local uuid = require("uuid")

local inbox_types = require("inbox_types")
local events = require("inbox_events")

local M = {}

local EV = inbox_types.EVENT

-- The legacy identity: before the delivery identity the item id WAS the delivery
-- key, recorded under the module's own origin as "item:<key>".
local LEGACY_PREFIX = "item:"

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function body_of(event: any): { [string]: any }
    local raw = type(event) == "table" and (event :: any).body or nil
    if type(raw) == "string" then
        local decoded, err = json.decode(raw :: string)
        if err or type(decoded) ~= "table" then return {} end
        return decoded :: { [string]: any }
    end
    return (type(raw) == "table" and raw or {}) :: { [string]: any }
end

-- A stable rendering of a value: map keys are emitted in sorted order so two
-- equal bodies render equally regardless of how their tables were built.
local function canonical(value: any): string
    if value == nil then return "~" end
    if type(value) ~= "table" then return tostring(value) end
    local keys: { string } = {}
    for key in pairs(value :: table) do keys[#keys + 1] = tostring(key) end
    table.sort(keys)
    local parts: { string } = {}
    for _, key in ipairs(keys) do
        parts[#parts + 1] = key .. "=" .. canonical((value :: any)[key])
    end
    return "{" .. table.concat(parts, ",") .. "}"
end

-- The delivered body, digested. One delivery key names one body; this is what
-- proves a second delivery under that key carries the same one.
local function fingerprint_of(params: any): string
    local digest, err = hash.sha256(canonical({
        item_type = params.item_type,
        title = params.title,
        content = params.content,
        priority = params.priority,
        source_id = params.source_id,
        metadata = params.metadata,
    }))
    if err or type(digest) ~= "string" then return "" end
    return digest
end

local function conflict(): (any?, string?)
    return nil, inbox_types.ERROR.DELIVERY_CONFLICT
end

-- find_legacy reads the log under the identity the pre-delivery writer used. Only
-- two keys could have named such an item: the delivery key the sink projected as
-- the item id, and the source key it falls back to when a source states no
-- delivery key of its own.
local function find_legacy(item_key: string, dedup_key: string): (any, string?)
    local candidates: { string } = { dedup_key }
    if item_key ~= dedup_key then candidates[#candidates + 1] = item_key end
    for _, key in ipairs(candidates) do
        local event, err = events.find_event(EV.CREATED, inbox_types.ORIGIN, LEGACY_PREFIX .. key)
        if err then return nil, err end
        if event then return event, nil end
    end
    return nil, nil
end

-- standing answers a delivery whose source item already has a genesis event.
local function standing(genesis: any, delivery: any, params: any): (any?, string?)
    local body = body_of(genesis)
    local item_id = trim(body.id)
    if item_id == "" then return nil, "the standing item event carries no item id" end

    -- This delivery minted the item: the same delivery again is a replay.
    if tostring((genesis :: any).external_version) == delivery.dedup_key then
        if trim(body.fingerprint) ~= "" and trim(body.fingerprint) ~= delivery.fingerprint then return conflict() end
        return { item_id = item_id, outcome = "created" }, nil
    end

    -- A version already recorded under this delivery key is a replay of it.
    local version, verr = events.find_event(EV.VERSION, delivery.origin, delivery.item_key, delivery.dedup_key)
    if verr then return nil, verr end
    if version then
        local recorded = body_of(version)
        if trim(recorded.fingerprint) ~= "" and trim(recorded.fingerprint) ~= delivery.fingerprint then return conflict() end
        return { item_id = item_id, outcome = "updated" }, nil
    end

    local _, err = events.emit(EV.VERSION, {
        id = item_id,
        item_type = params.item_type,
        title = params.title,
        content = params.content,
        metadata = params.metadata,
        source_id = params.source_id,
        priority = params.priority,
        origin = delivery.origin,
        item_key = delivery.item_key,
        dedup_key = delivery.dedup_key,
        fingerprint = delivery.fingerprint,
        delivered_at = os.time(),
    }, {
        external_source = delivery.origin,
        external_id = delivery.item_key,
        external_version = delivery.dedup_key,
    }, params.trace_context)
    if err then return nil, err end
    return { item_id = item_id, outcome = "updated" }, nil
end

-- adopt records that a delivery identity names an item that was written under the
-- legacy raw-key identity. The mapping is itself a genesis event, so every later
-- delivery of that source item resolves through the one lookup and the item is
-- never minted a second time. It materializes nothing: the adopted item already
-- stands (or was deleted, and a mapping must not resurrect it).
local function adopt(legacy: any, delivery: any, params: any): (any?, string?)
    local item_id = trim(body_of(legacy).id)
    if item_id == "" then return nil, "the legacy item event carries no item id" end
    local _, err = events.emit(EV.CREATED, {
        id = item_id,
        adopted = true,
        origin = delivery.origin,
        item_key = delivery.item_key,
        dedup_key = delivery.dedup_key,
        fingerprint = delivery.fingerprint,
        created_at = os.time(),
    }, {
        external_source = delivery.origin,
        external_id = delivery.item_key,
        external_version = delivery.dedup_key,
    }, params.trace_context)
    if err then return nil, err end
    return { item_id = item_id, outcome = "created" }, nil
end

-- deliver(params) -> ({ item_id, outcome }, err). params carries the item fields
-- plus origin / item_key / dedup_key.
function M.deliver(params: any): (any?, string?)
    params = type(params) == "table" and params or {}
    local origin = trim(params.origin)
    if origin == "" then origin = inbox_types.ORIGIN end
    local item_key = trim(params.item_key)
    local dedup_key = trim(params.dedup_key)
    if item_key == "" or dedup_key == "" then
        return nil, "a delivery requires a source item_key and a dedup_key"
    end

    local delivery = {
        origin = origin,
        item_key = item_key,
        dedup_key = dedup_key,
        fingerprint = fingerprint_of(params),
    }

    local genesis, gerr = events.find_event(EV.CREATED, origin, item_key)
    if gerr then return nil, gerr end
    if genesis then return standing(genesis, delivery, params) end

    local legacy, lerr = find_legacy(item_key, dedup_key)
    if lerr then return nil, lerr end
    if legacy then return adopt(legacy, delivery, params) end

    local create_params: { [string]: any } = {}
    for key, value in pairs(params) do create_params[key] = value end
    create_params.id = uuid.v7()
    create_params.delivery = delivery

    local id, cerr, duplicate = events.create(create_params)
    if cerr then
        -- The identity may have landed under a peer between the lookup and this
        -- append; ask the log again and answer from what stands.
        local won, werr = events.find_event(EV.CREATED, origin, item_key)
        if werr then return nil, werr end
        if won then return standing(won, delivery, params) end
        return nil, cerr
    end
    if duplicate then
        local won, werr = events.find_event(EV.CREATED, origin, item_key)
        if werr then return nil, werr end
        if won then return standing(won, delivery, params) end
    end
    return { item_id = tostring(id), outcome = "created" }, nil
end

-- resolve answers the item standing under a delivery identity, or nil when that
-- identity has none. It reads the LOG, never the read model, so a retraction
-- converges whether or not the projection has caught up.
function M.resolve(origin: string, item_key: string, dedup_key: string): (string?, string?)
    local source = trim(origin)
    if source == "" then source = inbox_types.ORIGIN end
    local key = trim(item_key)
    if key == "" then return nil, "a retraction requires a source item_key" end

    local genesis, gerr = events.find_event(EV.CREATED, source, key)
    if gerr then return nil, gerr end
    if genesis then
        local item_id = trim(body_of(genesis).id)
        return item_id ~= "" and item_id or nil, nil
    end

    local legacy, lerr = find_legacy(key, trim(dedup_key) ~= "" and trim(dedup_key) or key)
    if lerr then return nil, lerr end
    if not legacy then return nil, nil end
    local legacy_id = trim(body_of(legacy).id)
    return legacy_id ~= "" and legacy_id or nil, nil
end

return M

