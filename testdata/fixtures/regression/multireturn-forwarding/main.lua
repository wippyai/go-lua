-- From spiralscout.estimation.traits:read_tool in kickside/estimation/test/hub.
-- The reader stub has the public result/error shape used by this call.
local reader = {
    tree_page = function(eid: string, opts: any): (any, string?)
        return {}, nil
    end,
}

local function fail(message: any): string
    return tostring(message)
end

local function encode(data: any): string
    return tostring(data)
end

local function reply(data: any, err: any): string
    if not data then return fail(err) end
    return encode(data)
end

local function handler(args: any): string
    args = type(args) == "table" and args or {}
    local eid = tostring((args :: any).estimate_id or "")
    return reply(reader.tree_page(eid, { parent_id = (args :: any).parent_id, cursor = (args :: any).cursor, limit = (args :: any).limit }))
end

return { handler = handler }
