-- Extracted from kickside.agents.binding:user_agents_func and kickside.component:component.
local user_agents = {}
local component = {}
function component.set_meta(component_id: string, fields: { [string]: any }, opts: { [string]: any }?): (boolean, error?)
    return true, nil
end
local function discovery_meta_fields(args: table): table
    local fields = {}
    -- Public discovery is opt-in and strictly boolean. Missing or malformed
    -- values remain private rather than being coerced truthy.
    if args.public ~= nil then fields.public = args.public == true end
    if args.palette_hidden ~= nil then fields.palette_hidden = args.palette_hidden == true end
    if args.environment ~= nil then fields.environment = tostring(args.environment) end
    return fields
end
user_agents.discovery_meta_fields = discovery_meta_fields

local function update_meta_fields(args: table): table
    local fields = discovery_meta_fields(args)
    -- A metadata-only maintenance patch (for example palette visibility) must not
    -- rewrite the capability summary. Only recompute it when delegates are present.
    if args.delegates ~= nil then fields.delegate_count = #(args.delegates or {}) end
    return fields
end
user_agents.update_meta_fields = update_meta_fields
function user_agents.update(component_id: string, args: table): any
    local meta_fields = update_meta_fields(args)
    if args.title ~= nil then meta_fields.title = args.title end
    if args.description ~= nil then meta_fields.comment = args.description end
    if args.icon ~= nil then meta_fields.icon = args.icon end
    if args.model ~= nil then meta_fields.model = args.model end
    for key, value in pairs(discovery_meta_fields(args)) do meta_fields[key] = value end
    local meta_ok, meta_err = component.set_meta(component_id, meta_fields)
    if not meta_ok then
        return {
            success = false,
            error = meta_err or "update failed",
            status = "server_error",
        }
    end
end
return user_agents
