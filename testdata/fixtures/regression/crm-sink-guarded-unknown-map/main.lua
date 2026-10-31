local component = require("component")
local hash = require("hash")
local json = require("json")
local sql = require("sql")
local storage = require("storage")
local types = require("types")
local writer = require("writer")

local M = {}

type UnknownMap = { [string]: unknown }
type DynamicTable = { [string | number]: unknown }
type Executor = sql.DB | sql.Transaction
type ErrorResponse = {
    success: boolean,
    error: {
        code: string,
        message: string,
        retriable: boolean,
        scope: string,
        dependency_key: string?,
    },
}
type SuccessResponse = {
    success: boolean,
    result: unknown,
    dest_ref: UnknownMap,
}
type SinkReceipt = {
    payload_hash: unknown,
    status: unknown,
    outcome: unknown,
    dest_ref: unknown,
    idempotency_key: unknown,
}
type Command = {
    type: string,
    body: UnknownMap,
    external: UnknownMap?,
}

local function trim(value: unknown): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function response_error(code: string, message: unknown, retriable: boolean, scope: string): ErrorResponse
    return {
        success = false,
        error = {
            code = code,
            message = tostring(message),
            retriable = retriable,
            scope = scope,
        },
    }
end

local function validation_error(message: string): ErrorResponse
    return response_error("invalid_request", message, false, "item")
end

local function idempotency_conflict(): ErrorResponse
    return response_error("conflict", "sink idempotency conflict", false, "item")
end

local function stable(value: unknown): string
    local value_type = type(value)
    if value == nil then return "null" end
    if value_type == "boolean" then return value and "true" or "false" end
    if value_type == "number" then return tostring(value) end
    if value_type == "string" then
        local encoded, err = json.encode(value)
        if err then error(err, 0) end
        return encoded
    end
    if value_type ~= "table" then error("sink request contains unsupported value type: " .. value_type, 0) end

    local source = value :: DynamicTable
    local count, array = 0, true
    for key in pairs(source) do
        count = count + 1
        if type(key) ~= "number" or key % 1 ~= 0 or key < 1 then array = false end
    end
    if array then
        for index = 1, count do if source[index] == nil then array = false; break end end
    end
    if array then
        local parts = {}
        for index = 1, count do parts[#parts + 1] = stable(source[index]) end
        return "[" .. table.concat(parts, ",") .. "]"
    end

    local keys = {}
    for key in pairs(source) do
        if type(key) ~= "string" then error("sink request object keys must be strings", 0) end
        keys[#keys + 1] = key
    end
    table.sort(keys)
    local parts = {}
    for _, key in ipairs(keys) do parts[#parts + 1] = stable(key) .. ":" .. stable(source[key]) end
    return "{" .. table.concat(parts, ",") .. "}"
end

-- The digest covers what a delivery writes: its config, input, and operation.
-- dest_ref is where the write landed, an address the sink itself returned, and
-- an at-least-once redelivery carries it back. The same write under the same
-- key replays whether or not the caller already learned its destination.
local function payload_hash(config: UnknownMap, input: UnknownMap, operation: string): (string?, string?)
    local ok, encoded = pcall(stable, { config = config, input = input, operation = operation })
    if not ok then return nil, tostring(encoded) end
    return tostring(hash.sha256(encoded)), nil
end

local function open_db(): (sql.DB?, string?)
    local db, err = sql.get(types.db_id())
    if not db then return nil, tostring(err or "crm: app:db unavailable") end
    return db, nil
end

local function decode_dest_ref(value: unknown): UnknownMap
    if type(value) == "table" then return value :: UnknownMap end
    if type(value) ~= "string" or value == "" then return {} end
    local decoded, err = json.decode(value)
    if err or type(decoded) ~= "table" then return {} end
    return decoded :: UnknownMap
end

local function replay(crm_id: string, origin: string, operation: string, idempotency_key: string, digest: string): (SuccessResponse?, ErrorResponse?)
    local db, db_err = open_db()
    if not db then return nil, response_error("provider_unavailable", db_err, true, "provider") end
    local receipt, receipt_err = storage.sink_receipt(db, crm_id, origin, operation, idempotency_key)
    db:release()
    if receipt_err then return nil, response_error("provider_unavailable", receipt_err, true, "provider") end
    if not receipt then return nil, nil end
    if type(receipt) ~= "table" then return nil, response_error("provider_unavailable", "invalid sink receipt", true, "provider") end
    local row = receipt :: SinkReceipt
    if row.payload_hash ~= digest then return nil, idempotency_conflict() end
    if row.status ~= "committed" then return nil, nil end
    return {
        success = true,
        result = row.outcome,
        dest_ref = decode_dest_ref(row.dest_ref),
    }, nil
end

local function reserve_receipt(crm_id: string, origin: string, operation: string, idempotency_key: string, digest: string): (unknown, string?)
    local db, db_err = open_db()
    if not db then return nil, db_err end
    local receipt, receipt_err = storage.reserve_sink_receipt(db, crm_id, {
        origin = origin,
        sink_op = operation,
        idempotency_key = idempotency_key,
        payload_hash = digest,
        created_at = types.now(),
    })
    db:release()
    return receipt, receipt_err
end

local function persist_receipt(crm_id: string, origin: string, operation: string, idempotency_key: string, digest: string, outcome: string, dest_ref: UnknownMap): (unknown, string?)
    local encoded, encode_err = json.encode(dest_ref)
    if encode_err then return nil, tostring(encode_err) end
    local db, db_err = open_db()
    if not db then return nil, db_err end
    local receipt, receipt_err = storage.complete_sink_receipt(db, crm_id, {
        origin = origin,
        sink_op = operation,
        idempotency_key = idempotency_key,
        payload_hash = digest,
        outcome = outcome,
        dest_ref = encoded,
        completed_at = types.now(),
    })
    db:release()
    return receipt, receipt_err
end

-- A schema change to the receipt table reads this lease and refuses while it is
-- live, so a delivery announces itself before it writes a receipt. A writer that
-- cannot renew is a writer the next migration cannot see, so the delivery waits.
local WRITER_ID = "spiralscout.crm.sink:crm_write"

local function renew_writer_lease(): ErrorResponse?
    local db, db_err = open_db()
    if not db then return response_error("provider_unavailable", db_err, true, "provider") end
    local _, lease_err = storage.renew_sink_writer_lease(db, WRITER_ID)
    db:release()
    if lease_err then return response_error("provider_unavailable", lease_err, true, "provider") end
    return nil
end

-- Canonical source attribution a writer may attach to any write. Optional:
-- the durable write protocol keys identity on origin + idempotency, and
-- provenance rides the event body as the record of where the data came from.
type Provenance = {
    source_type: string,
    source_id: string,
    source_event_id: string?,
    confidence: number?,
}

local PROVENANCE_FIELDS = { source_type = true, source_id = true, source_event_id = true, confidence = true }

local function validate_provenance(input: UnknownMap): (Provenance?, string?)
    local raw = input.provenance
    if raw == nil then return nil, nil end
    if type(raw) ~= "table" then return nil, "input.provenance must be an object" end
    local provenance = raw :: UnknownMap
    for key in pairs(provenance) do
        if not PROVENANCE_FIELDS[key] then return nil, "unknown input.provenance field: " .. tostring(key) end
    end
    if trim(provenance.source_type) == "" then return nil, "input.provenance.source_type is required" end
    if trim(provenance.source_id) == "" then return nil, "input.provenance.source_id is required" end
    if provenance.confidence ~= nil and type(provenance.confidence) ~= "number" then
        return nil, "input.provenance.confidence must be a number"
    end
    return provenance :: Provenance, nil
end

local COMMON_INPUT = { kind = true, provenance = true }
local KIND_INPUT = {
    record = { object_type = true, record_id = true, values = true, set = true,
        set_if_absent = true, append_unique = true, unset = true },
    relation = { object_type = true, record_id = true, attr = true, reference = true, relation = true },
    activity = { activity_id = true, record_id = true, participant_record_id = true, activity_type = true, occurred_at = true,
        due_at = true, done = true, payload = true, source = true, external_id = true, correlation_key = true,
        direction = true, duration_sec = true, outcome = true, recording_url = true, has_transcript = true },
    reference = { from_ref = true, edge_type = true, to_ref = true, attrs = true, active = true },
    membership = { collection_id = true, record_id = true, stage_id = true, from_stage_id = true, position = true },
}

local function validate_shape(config: UnknownMap, input: UnknownMap): string?
    for key in pairs(config) do
        if key ~= "crm_id" and key ~= "field_map" then return "unknown config field: " .. tostring(key) end
    end
    local kind = trim(input.kind)
    local allowed = KIND_INPUT[kind]
    if not allowed then return "unsupported CRM sink kind: " .. kind end
    for key in pairs(input) do
        if not COMMON_INPUT[key] and not allowed[key] then return "unknown input field for " .. kind .. ": " .. tostring(key) end
    end
    return nil
end

local function mapped_fields(config: UnknownMap, fields: unknown, field_name: string): (UnknownMap?, string?)
    if type(fields) ~= "table" then return nil, "input." .. field_name .. " is required" end
    local source = fields :: UnknownMap
    if config.field_map == nil then return source, nil end
    if type(config.field_map) ~= "table" then return nil, "config.field_map must be an object" end
    local values: UnknownMap = {}
    for source_attr, target_attr in pairs(config.field_map) do
        if type(source_attr) ~= "string" or type(target_attr) ~= "string" or target_attr == "" then
            return nil, "config.field_map must map field names to non-empty CRM field names"
        end
        if source[source_attr] ~= nil then values[target_attr] = source[source_attr] end
    end
    return values, nil
end

local function mapped_unset(config: UnknownMap, unset: unknown): ({ string }?, string?)
    if type(unset) ~= "table" then return nil, "input.unset must be an array" end
    local source = unset :: { unknown }
    if config.field_map == nil then
        local copied: { string } = {}
        for _, source_attr in ipairs(source) do
            if type(source_attr) ~= "string" then return nil, "input.unset entries must be strings" end
            copied[#copied + 1] = source_attr
        end
        return copied, nil
    end
    if type(config.field_map) ~= "table" then return nil, "config.field_map must be an object" end
    local field_map = config.field_map :: UnknownMap
    local mapped: { string } = {}
    for _, source_attr in ipairs(source) do
        if type(source_attr) ~= "string" then return nil, "input.unset entries must be strings" end
        local target_attr = field_map[source_attr]
        if target_attr ~= nil then
            if type(target_attr) ~= "string" or target_attr == "" then return nil, "config.field_map values must be non-empty strings" end
            mapped[#mapped + 1] = target_attr
        end
    end
    return mapped, nil
end

-- Create-vs-update resolves on the record itself, not on delivery history: sources
-- re-identify a changed item with a new item_key per version, so a redelivery
-- legitimately arrives with no dest_ref for a record that already exists. The one
-- exception is a crash-window replay of this delivery, which must re-emit the exact
-- shape of the event that already committed for the writer to recognize the batch.
local function record_upsert_shape(crm_id: string, origin: string, operation: string, record_id: string,
    idempotency_key: string): (boolean?, ErrorResponse?)
    local durable, durable_err = writer.durable_event_type(crm_id, writer.sink_external(origin, operation, idempotency_key),
        { types.EVENTS.RECORD_CREATED, types.EVENTS.RECORD_UPDATED })
    if durable_err then return nil, response_error("provider_unavailable", durable_err, true, "provider") end
    if durable ~= nil then return durable == types.EVENTS.RECORD_UPDATED, nil end
    local db, db_err = open_db()
    if not db then return nil, response_error("provider_unavailable", db_err, true, "provider") end
    local existing, locate_err = storage.locate(db, crm_id, record_id)
    db:release()
    if locate_err then return nil, response_error("provider_unavailable", locate_err, true, "provider") end
    return existing ~= nil, nil
end

-- These kinds all name a record the delete must resolve against, and the domain
-- refuses the command when it is missing. A destination that does not hold the
-- record is already in the state the delivery asks for, so the delete has
-- converged; answering noop settles it instead of failing the item on every pass.
local DELETE_NEEDS_RECORD: { [string]: boolean } = { record = true, relation = true, membership = true }

local function optional_input(input: UnknownMap, field: string): string?
    local value = trim(input[field])
    if value == "" then return nil end
    return value
end

local function delete_target_absent(crm_id: string, input: UnknownMap): (boolean?, ErrorResponse?)
    local kind = trim(input.kind)
    if kind == "activity" then
        local db, db_err = open_db()
        if not db then return nil, response_error("provider_unavailable", db_err, true, "provider") end
        local found, locate_err = storage.locate_activity(db, crm_id,
            optional_input(input, "activity_id"), optional_input(input, "external_id"),
            optional_input(input, "correlation_key"))
        db:release()
        if locate_err then return nil, response_error("provider_unavailable", locate_err, true, "provider") end
        return found == nil, nil
    end
    if not DELETE_NEEDS_RECORD[kind] then return false, nil end
    local record_id = trim(input.record_id)
    if record_id == "" then return false, nil end
    local db, db_err = open_db()
    if not db then return nil, response_error("provider_unavailable", db_err, true, "provider") end
    local existing, locate_err = storage.locate(db, crm_id, record_id)
    db:release()
    if locate_err then return nil, response_error("provider_unavailable", locate_err, true, "provider") end
    return existing == nil, nil
end

local function command_for(operation: string, input: UnknownMap,
    current_ref: unknown): (Command?, string?, UnknownMap?, string?)
    local kind = trim(input.kind)
    if kind == "record" then
        local object_type = trim(input.object_type)
        local record_id = trim(input.record_id)
        if object_type == "" then return nil, nil, nil, "record writes require input.object_type" end
        if record_id == "" then return nil, nil, nil, "record writes require input.record_id" end
        local body: UnknownMap = { object_type = object_type, record_id = record_id }
        local event_type, outcome
        if operation == "delete" then
            event_type, outcome = types.EVENTS.RECORD_DELETED, "deleted"
        elseif operation == "update" then
            if input.set ~= nil and type(input.set) ~= "table" then return nil, nil, nil, "input.set must be an object" end
            if input.set_if_absent ~= nil and type(input.set_if_absent) ~= "table" then return nil, nil, nil, "input.set_if_absent must be an object" end
            if input.append_unique ~= nil and type(input.append_unique) ~= "table" then return nil, nil, nil, "input.append_unique must be an object" end
            if input.unset ~= nil and type(input.unset) ~= "table" then return nil, nil, nil, "input.unset must be an array" end
            if input.set == nil and input.unset == nil and input.set_if_absent == nil and input.append_unique == nil then
                return nil, nil, nil, "record updates require input.set, input.set_if_absent, input.append_unique, or input.unset"
            end
            body.set, body.unset = input.set, input.unset
            body.set_if_absent, body.append_unique = input.set_if_absent, input.append_unique
            event_type, outcome = types.EVENTS.RECORD_UPDATED, "updated"
        elseif operation == "upsert" then
            if current_ref == nil then
                -- record_id is the natural key either way; _update_shape carries the
                -- resolution the caller already made.
                if input._update_shape then
                    body.set, body.unset = input.values, {}
                    event_type, outcome = types.EVENTS.RECORD_UPDATED, "updated"
                else
                    body.values = input.values
                    event_type, outcome = types.EVENTS.RECORD_CREATED, "created"
                end
            else
                if type(current_ref) ~= "table" then return nil, nil, nil, "dest_ref must be an object" end
                local ref = current_ref :: UnknownMap
                if trim(ref.crm_id) ~= trim(input._crm_id)
                    or trim(ref.object_type) ~= object_type
                    or trim(ref.record_id) ~= record_id then
                    return nil, nil, nil, "dest_ref must match config.crm_id, input.object_type, and input.record_id"
                end
                body.set, body.unset = input.values, {}
                event_type, outcome = types.EVENTS.RECORD_UPDATED, "updated"
            end
        else
            return nil, nil, nil, "unsupported record sink operation: " .. operation
        end
        local command: Command = { type = event_type, body = body }
        return command, outcome, { crm_id = input._crm_id, object_type = object_type, record_id = record_id }, nil
    end

    if kind == "relation" then
        local object_type = trim(input.object_type)
        local record_id = trim(input.record_id)
        local attr = trim(input.attr)
        if object_type == "" then return nil, nil, nil, "relation writes require input.object_type" end
        if record_id == "" then return nil, nil, nil, "relation writes require input.record_id" end
        if attr == "" then return nil, nil, nil, "relation writes require input.attr" end
        if type(input.reference) ~= "table" then return nil, nil, nil, "relation writes require input.reference" end
        local event_type = operation == "delete" and types.EVENTS.RELATION_UNLINKED or types.EVENTS.RELATION_LINKED
        if operation ~= "delete" and operation ~= "upsert" then return nil, nil, nil, "unsupported relation sink operation: " .. operation end
        local body: UnknownMap = {
            object_type = object_type,
            record_id = record_id,
            attr = attr,
            reference = input.reference,
            relation = input.relation,
        }
        local command: Command = { type = event_type, body = body }
        return command, operation == "delete" and "deleted" or "created",
            { crm_id = input._crm_id, object_type = object_type, record_id = record_id, attr = attr, reference = input.reference }, nil
    end

    if kind == "activity" then
        if operation == "delete" then
            local body: UnknownMap = {
                activity_id = optional_input(input, "activity_id"),
                external_id = optional_input(input, "external_id"),
                correlation_key = optional_input(input, "correlation_key"),
            }
            if body.activity_id == nil and body.external_id == nil and body.correlation_key == nil then
                return nil, nil, nil,
                    "activity deletes require input.activity_id, input.external_id, or input.correlation_key"
            end
            local command: Command = { type = types.EVENTS.ACTIVITY_DELETED, body = body }
            return command, "deleted",
                { crm_id = input._crm_id, activity_id = body.activity_id, external_id = body.external_id }, nil
        end
        if operation ~= "upsert" then return nil, nil, nil, "unsupported activity sink operation: " .. operation end
        local activity_id = trim(input.activity_id)
        local record_id = trim(input.record_id)
        local activity_type = trim(input.activity_type)
        if activity_id == "" or record_id == "" or activity_type == "" then
            return nil, nil, nil, "activity writes require input.activity_id, input.record_id, and input.activity_type"
        end
        local body: UnknownMap = {
            activity_id = activity_id,
            record_id = record_id,
            activity_type = activity_type,
            participant_record_id = input.participant_record_id,
            occurred_at = input.occurred_at,
            due_at = input.due_at,
            done = input.done,
            source = input.source,
            external_id = input.external_id,
            correlation_key = input.correlation_key,
            direction = input.direction,
            duration_sec = input.duration_sec,
            outcome = input.outcome,
            recording_url = input.recording_url,
            has_transcript = input.has_transcript,
            payload = input.payload,
        }
        local command: Command = { type = types.EVENTS.ACTIVITY_LOGGED, body = body }
        return command, "created", { crm_id = input._crm_id, activity_id = activity_id, record_id = record_id }, nil
    end

    if kind == "reference" then
        local from_ref = trim(input.from_ref)
        local edge_type = trim(input.edge_type)
        local to_ref = trim(input.to_ref)
        if from_ref == "" or edge_type == "" or to_ref == "" then
            return nil, nil, nil, "reference writes require input.from_ref, input.edge_type, and input.to_ref"
        end
        if operation ~= "upsert" and operation ~= "delete" then return nil, nil, nil, "unsupported reference sink operation: " .. operation end
        if input.attrs ~= nil and type(input.attrs) ~= "table" then return nil, nil, nil, "input.attrs must be an object" end
        local body: UnknownMap = {
            from_ref = from_ref,
            edge_type = edge_type,
            to_ref = to_ref,
            attrs = input.attrs,
            active = input.active,
        }
        local event_type = operation == "delete" and types.EVENTS.REFERENCE_UNLINKED or types.EVENTS.REFERENCE_LINKED
        local command: Command = { type = event_type, body = body }
        return command, operation == "delete" and "deleted" or "created",
            { crm_id = input._crm_id, from_ref = from_ref, edge_type = edge_type, to_ref = to_ref }, nil
    end

    if kind == "membership" then
        local collection_id = trim(input.collection_id)
        local record_id = trim(input.record_id)
        if collection_id == "" or record_id == "" then return nil, nil, nil, "membership writes require input.collection_id and input.record_id" end
        if operation ~= "upsert" and operation ~= "delete" then return nil, nil, nil, "unsupported membership sink operation: " .. operation end
        local body: UnknownMap = {
            collection_id = collection_id,
            record_id = record_id,
            stage_id = input.stage_id,
            from_stage_id = input.from_stage_id,
            position = input.position,
        }
        local event_type = operation == "delete" and types.EVENTS.MEMBERSHIP_REMOVED or types.EVENTS.MEMBERSHIP_SET
        local command: Command = { type = event_type, body = body }
        return command, operation == "delete" and "deleted" or "updated",
            { crm_id = input._crm_id, collection_id = collection_id, record_id = record_id }, nil
    end
    return nil, nil, nil, "unsupported CRM sink kind: " .. kind
end

function M.write(req: unknown): UnknownMap
    if type(req) ~= "table" then return validation_error("request must be an object") end
    local request = req :: UnknownMap
    local config = type(request.config) == "table" and request.config :: UnknownMap or nil
    local input = type(request.input) == "table" and request.input :: UnknownMap or nil
    if not config then return validation_error("config is required") end
    if not input then return validation_error("input is required") end
    local crm_id = trim(config.crm_id)
    if crm_id == "" then return validation_error("config.crm_id is required") end
    local operation = trim(request.sink_op)
    if operation == "" then return validation_error("sink_op is required") end
    local idempotency_key = trim(request.idempotency_key)
    if idempotency_key == "" then return validation_error("idempotency_key is required") end
    -- idempotency_key identifies the source item, which two syncs reading one
    -- source share; origin identifies the writer, so their receipts stay apart.
    local origin = trim(request.origin)
    if trim(input.kind) == "" then return validation_error("input.kind is required") end
    local shape_err = validate_shape(config, input)
    if shape_err then return validation_error(shape_err) end
    local provenance, provenance_err = validate_provenance(input)
    if provenance_err then return validation_error(provenance_err) end
    local allowed, access_err = writer.require_access(crm_id, component.ACCESS.WRITE)
    if not allowed then return response_error("permission_denied", access_err, false, "item") end
    local lease_err = renew_writer_lease()
    if lease_err then return lease_err end
    local digest, digest_err = payload_hash(config, input, operation)
    if not digest then return validation_error(digest_err :: string) end
    local prior, replay_err = replay(crm_id, origin, operation, idempotency_key, digest)
    if replay_err then return replay_err end
    if prior then
        -- A replayed commit answers with the same slot-keyed dest_refs shape a
        -- fresh delivery does: the sync stores the map on every non-delete
        -- write, and a retried item is exactly the caller most likely to land
        -- here.
        local prior_slot = trim(request.map_slot)
        if prior_slot == "" then prior_slot = "default" end
        local prior_map: UnknownMap = type((prior :: UnknownMap).dest_ref) == "table"
            and (prior :: UnknownMap).dest_ref :: UnknownMap or {}
        (prior :: UnknownMap).dest_refs = { [prior_slot] = { dest_ref = prior_map } }
        return prior
    end

    local command_input: UnknownMap = {}
    for key, value in pairs(input) do
        if key ~= "provenance" then command_input[key] = value end
    end
    command_input._crm_id = crm_id
    if operation == "upsert" and trim(input.kind) == "record" then
        local values, values_err = mapped_fields(config, input.values, "values")
        if not values then return validation_error(values_err :: string) end
        command_input.values = values
    elseif operation == "update" and trim(input.kind) == "record" and input.set ~= nil then
        local set, set_err = mapped_fields(config, input.set, "set")
        if not set then return validation_error(set_err :: string) end
        command_input.set = set
    end
    if operation == "update" and trim(input.kind) == "record" and input.set_if_absent ~= nil then
        local conditional, conditional_err = mapped_fields(config, input.set_if_absent, "set_if_absent")
        if not conditional then return validation_error(conditional_err :: string) end
        command_input.set_if_absent = conditional
    end
    if operation == "update" and trim(input.kind) == "record" and input.append_unique ~= nil then
        local appended, append_err = mapped_fields(config, input.append_unique, "append_unique")
        if not appended then return validation_error(append_err :: string) end
        command_input.append_unique = appended
    end
    if operation == "update" and trim(input.kind) == "record" and input.unset ~= nil then
        local unset, unset_err = mapped_unset(config, input.unset)
        if not unset then return validation_error(unset_err :: string) end
        command_input.unset = unset
    end
    if operation == "upsert" and trim(input.kind) == "record"
        and request.dest_ref == nil and trim(input.record_id) ~= "" then
        local update_shape, shape_err = record_upsert_shape(crm_id, origin, operation, trim(input.record_id), idempotency_key)
        if shape_err then return shape_err end
        command_input._update_shape = update_shape
    end
    local command, outcome, dest_ref, command_err = command_for(operation, command_input, request.dest_ref)
    if not command then return validation_error(command_err :: string) end
    if provenance ~= nil and type((command :: Command).body) == "table" then
        ((command :: Command).body :: UnknownMap).provenance = provenance
    end
    -- Checked once the delivery is known to be well formed, so a malformed delete
    -- still reports why rather than converging on a record it never named. No
    -- effect is applied, so no receipt is retained: a redelivery re-derives the
    -- same answer from the destination itself.
    if operation == "delete" then
        local absent, absent_err = delete_target_absent(crm_id, input)
        if absent_err then return absent_err end
        if absent then return { success = true, result = "noop" } end
    end
    command.external = writer.sink_external(origin, operation, idempotency_key)
    local reservation, reservation_err = reserve_receipt(crm_id, origin, operation, idempotency_key, digest)
    if not reservation then
        if reservation_err == "sink idempotency conflict" then return idempotency_conflict() end
        return response_error("provider_error", reservation_err, true, "provider")
    end
    if type(reservation) ~= "table" then return response_error("provider_error", "invalid sink reservation", true, "provider") end
    local reserved = reservation :: SinkReceipt
    -- The sync's writable ABI reads dest_refs as a map keyed by the delivery's
    -- map_slot: that map is what the destination bookkeeping stores per item.
    -- The bare dest_ref stays alongside for callers that address a single ref.
    local map_slot = trim(request.map_slot)
    if map_slot == "" then map_slot = "default" end
    -- A landed record answers to the dependency key other writes may be
    -- blocked on; the delivery carries it so the caller can release them.
    local announced: { string } = {}
    if trim(input.kind) == "record" and operation ~= "delete" then
        announced[1] = writer.record_dependency_key(crm_id, trim(input.record_id))
    end
    return { success = true }
end

return M
