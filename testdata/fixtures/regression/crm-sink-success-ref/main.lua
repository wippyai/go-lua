-- Extracted from spiralscout.crm.sink:crm_sink_implementation.
type UnknownMap = { [string]: unknown }
type Command = { type: string, body: UnknownMap, external: UnknownMap? }
local types: any = { EVENTS = {} }
local function trim(value: unknown): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end
local function optional_input(input: UnknownMap, field: string): string?
    local value = trim(input[field])
    if value == "" then return nil end
    return value
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
local function persist_receipt(ref: UnknownMap): boolean return true end
local function delivered(ref: UnknownMap): UnknownMap return { dest_ref = ref } end
local function write(operation: string, input: UnknownMap, current_ref: unknown): UnknownMap
    local command, outcome, dest_ref, command_err = command_for(operation, input, current_ref)
    if not command then return { error = command_err } end
    persist_receipt(dest_ref)
    return delivered(dest_ref)
end
return write
