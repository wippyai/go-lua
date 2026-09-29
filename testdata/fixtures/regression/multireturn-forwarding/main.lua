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

-- Values a trailing call yields beyond the callee's parameters are dropped.
local function decode(s: string): string
    return s
end
local function body(): (string, string?)
    return "{}", nil
end
local decoded: string = decode(body())

-- A trailing call's error value forwarded into an optional message parameter
-- is passed by Lua's adjustment, not written by the caller.
local function fetch(): (string?, string?)
    return "v", nil
end
local function expect(v: any, msg: string?): any
    return v
end
local fetched = assert(fetch())
local expected = expect(fetch())

return { handler = handler }
