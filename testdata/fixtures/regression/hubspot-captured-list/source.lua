local batch = require("association_batch")
local data_error = require("data_error")
local transport = require("transport")
local types = require("types")

local M = {}

local DEFAULT_LIMIT = 100
local MAX_OBJECT_PAGE = batch.MAX_BATCH_INPUTS

type ObjectRow = { id: string? }
type ObjectPageData = {
    results: { ObjectRow }?,
    paging: types.Paging?,
}
type ObjectQuery = {
    limit: number,
    archived: string,
    after: string?,
}
type ProviderResult = {
    success: boolean,
    error: string?,
    status_code: integer?,
    retry_after_ms: number?,
}
type ClientResult = {
    success: boolean,
    data: ObjectPageData?,
    error: string?,
    status_code: integer?,
    retry_after_ms: number?,
}
type Transport = {
    connect: fun(component_id: string?): (types.Conn?, string?),
    list_objects: fun(conn: types.Conn, object_type: string, query: ObjectQuery): ClientResult,
    batch_read_associations: fun(
        conn: types.Conn,
        from_type: string,
        to_type: string,
        inputs: { types.AssociationInput }
    ): types.AssociationBatchResult,
}
type Deps = { transport: Transport? }
type Config = {
    connection_id: string?,
    from_object_type: string?,
    to_object_type: string?,
}
type Cursor = {
    object_after: string?,
    association_inputs: { types.AssociationInput }?,
}
type Request = {
    config: Config?,
    cursor: Cursor?,
    limit: number?,
    component_id: string?,
}
type DataError = {
    code: string,
    message: string,
    retriable: boolean,
    scope: string,
    auth_expired: boolean?,
}
type ErrorEnvelope = {
    success: boolean,
    error: DataError,
    retry_after_ms: number?,
}
type EdgeEndpoint = { object_type: string, id: string }
type Edge = {
    source: { type: string, id: string },
    from: EdgeEndpoint,
    to: EdgeEndpoint,
    association_type: { category: string, type_id: number, label: string? },
}
type Provenance = {
    external_source: string,
    external_id: string,
    external_version: string,
}
type Payload = {
    kind: string,
    edge: Edge,
    provenance: Provenance,
    raw: types.AssociationTarget,
}
type Item = {
    item_key: string,
    dedup_key: string,
    op: string,
    source_version: string,
    payload: Payload,
    ref: string?,
}
type Key = {
    item_key: string,
    dedup_key: string,
    source_version: string,
}
type PullResult = {
    success: boolean,
    items: { Item }?,
    keys: { Key }?,
    next_cursor: Cursor?,
    has_more: boolean?,
    retry_after_ms: number?,
    error: DataError?,
}

local DEFAULT_TRANSPORT: Transport = {
    connect = function(component_id: string?): (types.Conn?, string?)
        return transport.connect(component_id)
    end,
    list_objects = function(conn: types.Conn, object_type: string, query: ObjectQuery): ClientResult
        return transport.list_objects(conn, object_type, query) :: ClientResult
    end,
    batch_read_associations = function(
        conn: types.Conn,
        from_type: string,
        to_type: string,
        inputs: { types.AssociationInput }
    ): types.AssociationBatchResult
        return transport.batch_read_associations(conn, from_type, to_type, inputs)
    end,
}

local function trim(value: unknown): string
    if type(value) ~= "string" and type(value) ~= "number" then return "" end
    return (tostring(value):gsub("^%s+", ""):gsub("%s+$", ""))
end

local function encode_identity(value: string): string
    return (value:gsub("[^%w%-%_%.%~]", function(char: string): string
        return string.format("%%%02X", string.byte(char))
    end))
end

local next_after = batch.next_after
local ignorable_partial_errors = batch.ignorable_partial_errors

local function failure(result: ProviderResult, operation: string): ErrorEnvelope
    return data_error.from_result(result, operation)
end

local function validate_config(config: Config): (string?, string?, ErrorEnvelope?)
    local from_type = trim(config.from_object_type)
    local to_type = trim(config.to_object_type)
    if from_type == "" then return nil, nil, data_error.invalid_config("config.from_object_type is required") end
    if to_type == "" then return nil, nil, data_error.invalid_config("config.to_object_type is required") end
    return from_type, to_type, nil
end

local function association_item(
    connection_id: string,
    from_type: string,
    from_id: string,
    to_type: string,
    association: types.AssociationTarget,
    association_type: types.AssociationType
): (Item?, string?)
    local to_id = trim(association.toObjectId)
    local category = trim(association_type.category)
    local type_id = association_type.typeId
    local label = trim(association_type.label)
    if to_id == "" then return nil, "association is missing toObjectId" end
    if category == "" then return nil, "association type is missing category" end
    if type_id == nil then return nil, "association type is missing typeId" end

    local key = table.concat({
        "hubspot",
        "association",
        encode_identity(connection_id),
        encode_identity(from_type),
        encode_identity(from_id),
        encode_identity(to_type),
        encode_identity(to_id),
        encode_identity(category),
        tostring(type_id),
    }, ":")
    local source_version = category .. ":" .. tostring(type_id) .. ":" .. label
    local edge: Edge = {
        source = { type = "hubspot.crm", id = connection_id },
        from = { object_type = from_type, id = from_id },
        to = { object_type = to_type, id = to_id },
        association_type = {
            category = category,
            type_id = type_id,
            label = label ~= "" and label or nil,
        },
    }
    return {
        item_key = key,
        dedup_key = key .. ":upsert:" .. encode_identity(source_version),
        op = "upsert",
        source_version = source_version,
        payload = {
            kind = "association",
            edge = edge,
            provenance = {
                external_source = "hubspot",
                external_id = from_type .. "/" .. from_id .. "/" .. to_type .. "/" .. to_id,
                external_version = source_version,
            },
            raw = association,
        },
        ref = nil,
    }, nil
end

-- One from-object fans out to arbitrarily many association rows (every to-record
-- times every association type), so a page of N objects can flatten far past the
-- engine's item limit -- and the engine refuses an over-limit answer outright
-- rather than advancing past unconsumed items. The flatten therefore caps at the
-- limit and records WHERE it stopped: the interrupted object re-enters the
-- cursor with the SAME page `after` it was read with plus a skip count, so the
-- next run re-reads that page (HubSpot answers the same (id, after) page in the
-- same order) and resumes at the first unemitted row; objects the cap never
-- reached re-enter untouched.
local function flatten_results(
    data: types.AssociationBatchData,
    connection_id: string,
    from_type: string,
    to_type: string,
    limit: number?,
    input_by_id: { [string]: types.AssociationInput }?
): ({ Item }?, { types.AssociationInput }?, string?)
    if type(data.results) ~= "table" then return nil, nil, "association response is missing results" end
    local items: { Item } = {}
    local continuations: { types.AssociationInput } = {}
    local cap = type(limit) == "number" and limit > 0 and limit or nil
    local capped = false
    for _, result in ipairs(data.results) do
        local from_id = trim(result.from and result.from.id or nil)
        if from_id == "" then return nil, nil, "association result is missing from.id" end
        if type(result.to) ~= "table" then return nil, nil, "association result is missing to" end
        local input = input_by_id and input_by_id[from_id] or nil
        if capped then
            -- The cap landed on an earlier object; this one was read but not
            -- consumed at all -- it re-enters exactly as it was requested.
            continuations[#continuations + 1] = {
                id = from_id,
                after = input and input.after or nil,
                skip = input and input.skip or nil,
            } :: types.AssociationInput
        else
            local rows: { Item } = {}
            for _, association in ipairs(result.to) do
                if type(association.associationTypes) ~= "table" or #association.associationTypes == 0 then
                    return nil, nil, "association is missing associationTypes"
                end
                for _, association_type in ipairs(association.associationTypes) do
                    local item, err = association_item(
                        connection_id,
                        from_type,
                        from_id,
                        to_type,
                        association,
                        association_type
                    )
                    if err then return nil, nil, err end
                    rows[#rows + 1] = item :: Item
                end
            end
            local skip = input and tonumber(input.skip) or 0
            if skip < 0 then skip = 0 end
            local emitted = 0
            for index = skip + 1, #rows do
                if cap and #items >= cap then break end
                items[#items + 1] = rows[index]
                emitted = emitted + 1
            end
            if skip + emitted < #rows then
                -- Interrupted mid-object: resume this same page at the next row.
                capped = true
                continuations[#continuations + 1] = {
                    id = from_id,
                    after = input and input.after or nil,
                    skip = skip + emitted,
                } :: types.AssociationInput
            else
                local after = next_after(result.paging)
                if after then continuations[#continuations + 1] = { id = from_id, after = after } end
            end
        end
    end
    return items, continuations, nil
end

function M.pull(request: Request, _opts: nil?, deps: Deps?): PullResult
    local config: Config = request.config or {}
    local from_type, to_type, config_err = validate_config(config)
    if config_err then return config_err end

    local tp: Transport = (deps and deps.transport) or DEFAULT_TRANSPORT
    -- The reconcile key listing carries the connection instance on the request;
    -- a binding open carries it in config.
    local selected_connection = trim(request.component_id)
    if selected_connection == "" then selected_connection = trim(config.connection_id) end
    local conn, connection_err = tp.connect(selected_connection ~= "" and selected_connection or nil)
    if connection_err or not conn then
        return data_error.connection(tostring(connection_err or "no connection"))
    end
    local connection_id = trim(conn.component_id)
    if connection_id == "" then
        return data_error.invalid_config("HubSpot connection has no stable component id")
    end

    local cursor: Cursor = request.cursor or {}
    local inputs = cursor.association_inputs
    local object_after = trim(cursor.object_after)
    local next_object_after = object_after ~= "" and object_after or nil

    if not inputs or #inputs == 0 then
        local requested_limit = math.floor(request.limit or DEFAULT_LIMIT)
        if requested_limit < 1 then requested_limit = 1 end
        if requested_limit > MAX_OBJECT_PAGE then requested_limit = MAX_OBJECT_PAGE end
        local query: ObjectQuery = {
            limit = requested_limit,
            archived = "false",
        }
        if object_after ~= "" then query.after = object_after end
        local object_page = tp.list_objects(conn, from_type :: string, query)
        if object_page.success ~= true then
            return failure(object_page, "list HubSpot " .. tostring(from_type))
        end
        if not object_page.data or type(object_page.data.results) ~= "table" then
            return data_error.invalid_request("invalid HubSpot object response: missing results")
        end
        inputs = {}
        for _, row in ipairs(object_page.data.results) do
            local id = trim(row.id)
            if id == "" then
                return data_error.invalid_request("invalid HubSpot object response: object is missing id")
            end
            inputs[#inputs + 1] = { id = id }
        end
        next_object_after = next_after(object_page.data.paging)
        if #inputs == 0 then
            return {
                success = true,
                items = {},
                next_cursor = { object_after = next_object_after },
                has_more = next_object_after ~= nil,
                retry_after_ms = next_object_after and 1 or nil,
            }
        end
    end

    local input_by_id: { [string]: types.AssociationInput } = {}
    local transport_inputs: { types.AssociationInput } = {}
    for _, input in ipairs(inputs :: { types.AssociationInput }) do
        input_by_id[trim(input.id)] = input
        -- skip is flatten-side resume state; the transport request carries only
        -- the page address.
        transport_inputs[#transport_inputs + 1] = { id = input.id, after = input.after } :: types.AssociationInput
    end
    local association_page = tp.batch_read_associations(
        conn,
        from_type :: string,
        to_type :: string,
        transport_inputs
    )
    if association_page.success ~= true then
        return failure(association_page, "read HubSpot associations")
    end
    local data = association_page.data
    if not data then
        return data_error.invalid_request("invalid HubSpot association response: missing data")
    end
    local ignorable, partial_error_count = ignorable_partial_errors(data)
    if partial_error_count > 0 and not ignorable then
        return failure({
            success = false,
            status_code = association_page.status_code,
            error = "HubSpot association batch returned " .. tostring(partial_error_count) .. " partial errors",
        }, "read HubSpot associations")
    end

    local flatten_limit = math.floor(request.limit or DEFAULT_LIMIT)
    if flatten_limit < 1 then flatten_limit = 1 end
    local items, continuations, flatten_err = flatten_results(
        data,
        connection_id,
        from_type :: string,
        to_type :: string,
        flatten_limit,
        input_by_id
    )
    if flatten_err then
        return data_error.invalid_request("invalid HubSpot association response: " .. flatten_err)
    end
    local typed_items = items :: { Item }
    local typed_continuations = continuations :: { types.AssociationInput }
    return {
        success = true,
        items = typed_items,
        next_cursor = {
            object_after = next_object_after,
            association_inputs = #typed_continuations > 0 and typed_continuations or nil,
        },
        has_more = #typed_continuations > 0 or next_object_after ~= nil,
    }
end

function M.pull_keys(request: Request, opts: nil?, deps: Deps?): PullResult
    local page = M.pull(request, opts, deps)
    if page.success ~= true then return page end
    local keys: { Key } = {}
    for _, item in ipairs(page.items or {}) do
        keys[#keys + 1] = {
            item_key = item.item_key,
            dedup_key = item.dedup_key,
            source_version = item.source_version,
        }
    end
    return {
        success = true,
        keys = keys,
        next_cursor = page.next_cursor,
        has_more = page.has_more,
        retry_after_ms = page.retry_after_ms,
    }
end

return M

