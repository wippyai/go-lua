-- A constructor whose instance's metatable carries methods converges to one
-- instance type: return summaries of such higher-order values join the
-- approximations of successive iterations field by field instead of keeping
-- each as a union member (session reader: session.open(id) then
-- reader:contexts()).
local session = {}

local reader = {
    session_id = nil :: string?,
    user_id = nil :: string?,
    actor = nil :: any,
    _session_data = nil :: any,
    cache = nil :: any,
}
reader.__index = reader

local function get(id: string): ({[string]: any}?, string?)
    return { config = {} }, nil
end

function session.open(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end
    local actor = { id = "a" }
    if not actor then
        return nil, "no actor"
    end
    local user_id = actor.id
    local data, err = get(session_id)
    if err then
        return nil, "Failed: " .. err
    end
    if not data then
        return nil, "not found"
    end
    local self = setmetatable({}, reader)
    self.session_id = session_id
    self.user_id = user_id
    self.actor = actor
    self._session_data = data
    self.cache = nil
    return self, nil
end

local query = {
    _session_id = nil :: string?,
}
query.__index = query

function reader:contexts()
    local q = setmetatable({}, query)
    q._session_id = self.session_id
    return q
end

function reader:name(): string
    return "x"
end

local function use(id: string): string?
    local r, err = session.open(id)
    if not r then
        return err
    end
    return r:name()
end

return { use = use, session = session }
