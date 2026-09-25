-- kickside.data:writable sink for writing generic provider/automation envelopes into
-- the workspace CRM event log. The sink owns no direct table writes: every upsert,
-- delete, association, membership, or activity becomes a crm.* event through writer.
local component = require("component")
local writer = require("writer")
local types = require("types")

local M = {}

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function fail(message: string, retriable: boolean?): any
    return { success = false, result = "noop", error = { code = "sink_error", message = tostring(message), retriable = retriable ~= false, scope = "sink" } }
end

local function validation_fail(message: string): any
    return fail(message, false)
end

local function ok(result: string, dest_ref: any): any
    return { success = true, result = result, dest_ref = dest_ref }
end

local function shallow_copy(input: any): any
    local out: any = {}
    if type(input) == "table" then
        for k, v in pairs(input :: any) do out[k] = v end
    end
    return out
end

local function mapped_properties(config: any, props: any): any
    local out: any = {}
    if type(props) ~= "table" then return out end
    local map = type(config.field_map) == "table" and config.field_map or nil
    if not map then
        for k, v in pairs(props :: any) do out[k] = v end
        return out
    end
    for source_key, dest_key in pairs(map :: any) do
        if type(dest_key) == "string" and dest_key ~= "" then
            local v = (props :: any)[source_key]
            if v ~= nil then out[dest_key] = v end
        end
    end
    return out
end

local function open_db(): (any?, string?)
    local db, err = require("sql").get(types.db_id())
    if not db then return nil, tostring(err or "crm: app:db unavailable") end
    return db, nil
end

local function config_input(req: any): (any, any)
    local config = type(req.config) == "table" and req.config or {}
    local input = type(req.input) == "table" and req.input or {}
    return config, input
end

local function crm_id_of(config: any, input: any): string
    return trim(input.crm_id) ~= "" and trim(input.crm_id) or trim(config.crm_id)
end

local function reserved_human_source(value: any): boolean
    local s = trim(value):lower()
    return s == "human" or s == "owner"
end

local function first_provider_source(...): string
    for i = 1, select("#", ...) do
        local value = select(i, ...)
        local s = trim(value)
        if s ~= "" and not reserved_human_source(s) then return s end
    end
    return ""
end

local function external_source_of(config: any, input: any): string
    local source = type(input.source) == "table" and input.source or {}
    local metadata = type(input.metadata) == "table" and input.metadata or {}
    local source_ref = type((metadata :: any).source_ref) == "table" and (metadata :: any).source_ref or {}
    local s = first_provider_source(
        input.external_source,
        (source :: any).external_source,
        (source :: any).origin,
        (source :: any).provider,
        (source_ref :: any).sync_id,
        (metadata :: any).sync_origin,
        config.external_source,
        config.source_label
    )
    return s ~= "" and s or "spiralscout.crm.sink"
end

local function external_id_of(req: any, input: any): string
    local source = type(input.source) == "table" and input.source or {}
    local id = trim(input.external_id)
    if id == "" then id = trim(input.source_event_id) end
    if id == "" then id = trim(input.dedup_key) end
    if id == "" then id = trim(input.item_key) end
    if id == "" then id = trim(type(input.metadata) == "table" and (input.metadata :: any).item_key or nil) end
    if id == "" then id = trim(req.idempotency_key) end
    if id == "" then id = trim((source :: any).external_id) end
    if id == "" then id = trim((source :: any).source_event_id) end
    if id == "" then id = trim((source :: any).id) end
    return id
end

local function external_version_of(input: any): string
    local source = type(input.source) == "table" and input.source or {}
    local metadata = type(input.metadata) == "table" and input.metadata or {}
    local v = trim(input.external_version)
    if v == "" then v = trim(input.version) end
    if v == "" then v = trim((source :: any).external_version) end
    if v == "" then v = trim((source :: any).version) end
    if v == "" then v = trim((metadata :: any).source_version) end
    return v
end

local function event_identity(req: any, config: any, input: any): any
    local metadata = type(input.metadata) == "table" and input.metadata or {}
    local source_ref = type((metadata :: any).source_ref) == "table" and (metadata :: any).source_ref or {}
    local external_id = external_id_of(req, input)
    local out: any = {
        external_source = external_source_of(config, input),
        external_id = external_id ~= "" and external_id or nil,
        external_version = nil,
        correlation_id = trim(input.correlation_id) ~= "" and trim(input.correlation_id) or trim(req.correlation_id),
        sync_id = trim((source_ref :: any).sync_id) ~= "" and trim((source_ref :: any).sync_id) or trim((metadata :: any).sync_id),
        workflow_run_id = trim((source_ref :: any).workflow_run_id) ~= "" and trim((source_ref :: any).workflow_run_id) or trim((metadata :: any).workflow_run_id),
        approval_item_id = trim((source_ref :: any).approval_item_id) ~= "" and trim((source_ref :: any).approval_item_id) or trim((metadata :: any).approval_item_id),
    }
    local version = external_version_of(input)
    if version ~= "" then out.external_version = version end
    if trim(input.source_event_id) ~= "" then out.source_event_id = trim(input.source_event_id) end
    if trim(input.dedup_key) ~= "" then out.dedup_key = trim(input.dedup_key) end
    return out
end

local function stamp_origin(body: any, identity: any)
    if type(body) ~= "table" or type(identity) ~= "table" then return end
    (body :: any).origin = { type = identity.external_source, id = identity.external_id }
    if trim(identity.sync_id) ~= "" then (body :: any).sync_id = trim(identity.sync_id) end
    if trim(identity.workflow_run_id) ~= "" then (body :: any).workflow_run_id = trim(identity.workflow_run_id) end
    if trim(identity.approval_item_id) ~= "" then (body :: any).approval_item_id = trim(identity.approval_item_id) end
end

local function outcome_noop(result: any): boolean
    local outcomes = type(result) == "table" and (result :: any).outcomes or nil
    if type(outcomes) ~= "table" or #outcomes == 0 then return false end
    for _, outcome in ipairs(outcomes :: { any }) do
        if type(outcome) == "table" and (outcome :: any).outcome == "applied" then return false end
    end
    return true
end

local function append_one(crm_id: string, etype: string, body: any, identity: any): any
    stamp_origin(body, identity)
    local cmd: any = {
        type = etype,
        body = body,
        external_source = identity.external_source,
        external_id = identity.external_id,
        external_version = identity.external_version,
        source_event_id = identity.source_event_id,
        dedup_key = identity.dedup_key,
        correlation_id = identity.correlation_id,
    }
    local ok_append, err, result = writer.append(crm_id, { cmd })
    if not ok_append then return nil, err end
    return result or {}, nil
end

local function write_record(req: any, config: any, input: any, op: string): any
    local crm_id = crm_id_of(config, input)
    if crm_id == "" then return fail("config.crm_id or input.crm_id is required") end

    local identity = event_identity(req, config, input)
    if op ~= "delete" and trim(identity.external_id) == "" then
        return validation_fail("record field write requires a stable provider identity: external_id, source_event_id, dedup_key, item_key, or idempotency_key")
    end
    local record_id = trim(input.record_id)
    if record_id == "" then record_id = trim((type(req.dest_ref) == "table" and (req.dest_ref :: any).record_id) or nil) end
    if record_id == "" then record_id = trim(identity.external_id) end
    if record_id == "" then return fail("record write requires record_id, external_id, dedup_key, item_key, or idempotency_key") end

    local object_type = trim(input.object_type)
    if object_type == "" then object_type = trim(config.object_type) end
    if object_type == "" then return fail("record write requires object_type") end

    if op == "delete" then
        local result, err = append_one(crm_id, types.EVENTS.RECORD_DELETED, {
            record_id = record_id,
            object_type = object_type,
            source = { external_source = identity.external_source, external_id = identity.external_id },
            source_event_id = identity.source_event_id,
            correlation_id = identity.correlation_id,
        }, identity)
        if err then return fail(err) end
        local r = outcome_noop(result) and "noop"
            or ((result :: any).inserted or 0) == 0 and ((result :: any).duplicates or 0) > 0 and "noop" or "deleted"
        return ok(r, { crm_id = crm_id, record_id = record_id, object_type = object_type })
    end

    local values = type(input.values) == "table" and input.values or mapped_properties(config, input.properties)
    local changed = type(input.changed) == "table" and input.changed or nil
    local etype = (op == "update" or changed ~= nil) and types.EVENTS.RECORD_UPDATED or types.EVENTS.RECORD_CREATED
    local body: any = {
        record_id = record_id,
        object_type = object_type,
        source = { external_source = identity.external_source, external_id = identity.external_id, external_version = identity.external_version },
        source_event_id = identity.source_event_id,
        dedup_key = identity.dedup_key,
        correlation_id = identity.correlation_id,
    }
    if etype == types.EVENTS.RECORD_UPDATED then body.changed = changed or values else body.values = values end

    local result, err = append_one(crm_id, etype, body, identity)
    if err then return fail(err) end
    local r = outcome_noop(result) and "noop"
        or ((result :: any).inserted or 0) == 0 and ((result :: any).duplicates or 0) > 0 and "noop"
        or (etype == types.EVENTS.RECORD_UPDATED and "updated" or "created")
    return ok(r, { crm_id = crm_id, record_id = record_id, object_type = object_type })
end

local function write_activity(req: any, config: any, input: any, _op: string): any
    local crm_id = crm_id_of(config, input)
    if crm_id == "" then return fail("config.crm_id or input.crm_id is required") end
    local identity = event_identity(req, config, input)
    local body = shallow_copy(input)
    body.activity_id = trim(body.activity_id) ~= "" and body.activity_id or identity.external_id
    if trim(body.activity_id) == "" then return fail("activity write requires activity_id or source identity") end
    body.source = { external_source = identity.external_source, external_id = identity.external_id, external_version = identity.external_version }
    body.source_event_id = identity.source_event_id
    body.correlation_id = identity.correlation_id
    local result, err = append_one(crm_id, types.EVENTS.ACTIVITY_LOGGED, body, identity)
    if err then return fail(err) end
    local r = outcome_noop(result) and "noop"
        or ((result :: any).inserted or 0) == 0 and ((result :: any).duplicates or 0) > 0 and "noop" or "created"
    return ok(r, { crm_id = crm_id, activity_id = body.activity_id, record_id = body.record_id })
end

local function write_relation(req: any, config: any, input: any, op: string): any
    local crm_id = crm_id_of(config, input)
    if crm_id == "" then return fail("config.crm_id or input.crm_id is required") end
    local identity = event_identity(req, config, input)
    local etype = op == "delete" and types.EVENTS.RELATION_UNLINKED or types.EVENTS.RELATION_LINKED
    local body = {
        from_record_id = input.from_record_id,
        to_record_id = input.to_record_id,
        relation = input.relation,
        source = { external_source = identity.external_source, external_id = identity.external_id, external_version = identity.external_version },
        source_event_id = identity.source_event_id,
        correlation_id = identity.correlation_id,
    }
    local result, err = append_one(crm_id, etype, body, identity)
    if err then return fail(err) end
    local r = outcome_noop(result) and "noop"
        or ((result :: any).inserted or 0) == 0 and ((result :: any).duplicates or 0) > 0 and "noop"
        or (op == "delete" and "deleted" or "created")
    return ok(r, { crm_id = crm_id, from_record_id = input.from_record_id, to_record_id = input.to_record_id, relation = input.relation })
end

local function write_membership(req: any, config: any, input: any, op: string): any
    local crm_id = crm_id_of(config, input)
    if crm_id == "" then return fail("config.crm_id or input.crm_id is required") end
    local identity = event_identity(req, config, input)
    local etype = op == "delete" and types.EVENTS.MEMBERSHIP_REMOVED or types.EVENTS.MEMBERSHIP_SET
    local body = {
        collection_id = input.collection_id,
        record_id = input.record_id,
        stage_id = input.stage_id,
        from_stage_id = input.from_stage_id,
        position = input.position,
        source = { external_source = identity.external_source, external_id = identity.external_id, external_version = identity.external_version },
        source_event_id = identity.source_event_id,
        correlation_id = identity.correlation_id,
    }
    local result, err = append_one(crm_id, etype, body, identity)
    if err then return fail(err) end
    local r = outcome_noop(result) and "noop"
        or ((result :: any).inserted or 0) == 0 and ((result :: any).duplicates or 0) > 0 and "noop"
        or (op == "delete" and "deleted" or "updated")
    return ok(r, { crm_id = crm_id, collection_id = input.collection_id, record_id = input.record_id })
end

function M.write(req: any): any
    req = type(req) == "table" and req or {}
    local config, input = config_input(req)
    local op = trim(req.sink_op)
    if op == "" then op = "upsert" end
    local kind = trim(input.kind)
    if kind == "" then kind = trim(input.entity) end
    if kind == "" then kind = trim(config.kind) end
    if kind == "" then kind = "record" end

    if kind == "record" then return write_record(req, config, input, op) end
    if kind == "activity" then return write_activity(req, config, input, op) end
    if kind == "association" or kind == "relation" then return write_relation(req, config, input, op) end
    if kind == "membership" then return write_membership(req, config, input, op) end
    return fail("unsupported CRM sink kind: " .. kind)
end

function M.list_keys(req: any): any
    req = type(req) == "table" and req or {}
    local config = type(req.config) == "table" and req.config or {}
    local crm_id = trim(req.crm_id) ~= "" and trim(req.crm_id) or trim(config.crm_id)
    if crm_id == "" then return fail("config.crm_id is required") end
    local allowed, aerr = writer.require_access(crm_id, component.ACCESS.READ)
    if not allowed then return fail(aerr) end
    local origin = trim(req.origin)
    if origin == "" then origin = trim(config.external_source) end
    if origin == "" then origin = trim(config.source_label) end
    if origin == "" then return fail("origin is required") end

    local db, derr = open_db()
    if not db then return fail(derr) end
    local limit = math.floor(tonumber(req.limit) or 500)
	local rows, qerr = db:query([[
	    SELECT
	        a.alias_display AS external_id,
	        a.record_id AS alias_record_id,
	        COALESCE(r.object_type, a.object_scope) AS object_type,
	        CASE
	            WHEN r.canonical_id IS NOT NULL AND r.canonical_id <> '' THEN r.canonical_id
	            ELSE a.record_id
	        END AS canonical_record_id
	    FROM spiralscout_crm_alias a
	    LEFT JOIN spiralscout_crm_record r
	      ON r.crm_id = a.crm_id AND r.record_id = a.record_id
	    WHERE a.crm_id = $1
	      AND a.alias_type = $2
	      AND a.alias_display IS NOT NULL
	    ORDER BY a.updated_at, a.record_id
	    LIMIT $3
	]], { crm_id, "external:" .. origin, limit })
    local recon_rows: any = {}
    local rerr: any = nil
    if not qerr then
        recon_rows, rerr = db:query([[
            SELECT
                external_id,
                record_id,
                object_type,
                CASE
                    WHEN canonical_id IS NOT NULL AND canonical_id <> '' THEN canonical_id
                    ELSE record_id
                END AS canonical_record_id
            FROM spiralscout_crm_reconciliation
            WHERE crm_id = $1
              AND external_source = $2
              AND external_id IS NOT NULL
              AND external_id <> ''
            ORDER BY updated_at, record_id
            LIMIT $3
        ]], { crm_id, origin, limit })
    end
    db:release()
    if qerr then return fail(tostring(qerr)) end
    if rerr then return fail(tostring(rerr)) end

    local keys: { any } = {}
    local seen = {}
    for _, row in ipairs(type(rows) == "table" and rows or {}) do
        local item_key = tostring((row :: any).external_id or "")
        seen[item_key] = true
	        keys[#keys + 1] = {
	            item_key = item_key,
	            dedup_key = item_key,
	            dest_ref = {
	                crm_id = crm_id,
	                record_id = (row :: any).canonical_record_id or (row :: any).alias_record_id,
	                object_type = (row :: any).object_type,
	            },
	        }
    end
    for _, row in ipairs(type(recon_rows) == "table" and recon_rows or {}) do
        local item_key = tostring((row :: any).external_id or "")
        if #keys >= limit then break end
        if item_key ~= "" and not seen[item_key] then
            keys[#keys + 1] = {
                item_key = item_key,
                dedup_key = item_key,
                dest_ref = {
                    crm_id = crm_id,
                    record_id = (row :: any).canonical_record_id or (row :: any).record_id,
                    object_type = (row :: any).object_type,
                },
            }
        end
    end
    return { success = true, keys = keys, next_cursor = nil, has_more = false }
end

return M

