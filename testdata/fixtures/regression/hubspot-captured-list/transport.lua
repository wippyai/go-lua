local component = require("component")
local api = require("api")
local config = require("config")
local types = require("types")

local M = {}

type Context = {
    api_key: unknown?,
    component_id: unknown?,
}

-- Resolve the connection's stored context: an explicit component_id, else the
-- single HubSpot connection in scope (trusted read of its private context).
local function context(component_id: string?): (Context?, string?)
    if component_id and component_id ~= "" then
        local ctx, err = component.get_context(component_id, component.ACCESS.READ)
        if err then return nil, tostring(err) end
        if type(ctx) ~= "table" then return nil, "HubSpot connection context is invalid" end
        return ctx :: Context, nil
    end
    local ctx, err = component.get_context_by_meta({
        class = types.CONNECTION_CLASS,
        provider = types.PROVIDER,
    }, component.ACCESS.READ)
    if err then
        return nil, "no HubSpot connection selected; configure the trait connection when there is no single default"
    end
    if type(ctx) ~= "table" then return nil, "HubSpot connection context is invalid" end
    return ctx :: Context, nil
end

function M.connect(component_id: string?): (types.Conn?, string?)
    local ctx, err = context(component_id)
    if err or not ctx then return nil, err end
    local api_key = ctx.api_key
    if type(api_key) ~= "string" or api_key == "" then
        return nil, "HubSpot connection is missing api_key"
    end
    local cid = type(ctx.component_id) == "string" and (ctx.component_id :: string) or nil
    return ({ component_id = cid, api_key = api_key } :: types.Conn), nil
end

-- The config-owned canonical CRM object-type lists. All types are readable;
-- engagement objects (meetings, notes, emails) are pull-only.
M.OBJECT_TYPES = config.OBJECT_TYPES
M.WRITE_OBJECT_TYPES = config.WRITE_OBJECT_TYPES

-- Whether object_type is one of the supported CRM objects.
function M.is_object_type(object_type: unknown): boolean
    return config.is_object_type(object_type)
end

-- Whether object_type accepts writes.
function M.is_writable_object_type(object_type: unknown): boolean
    return config.is_writable_object_type(object_type)
end

M.account_info = api.account_info
M.test_connection = api.test_connection
M.list_objects = api.list_objects
M.get_object = api.get_object
M.batch_read_objects = api.batch_read_objects
M.batch_read_associations = api.batch_read_associations
M.search_objects = api.search_objects
M.search_incremental = api.search_incremental
M.watermark_value = api.watermark_value
M.list_owners = api.list_owners
M.list_pipelines = api.list_pipelines
M.list_properties = api.list_properties
M.create_object = api.create_object
M.update_object = api.update_object
M.archive_object = api.archive_object
M.create_association = api.create_association

return M

