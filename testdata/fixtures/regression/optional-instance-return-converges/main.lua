-- A constructor returning an optional class instance converges to one
-- instance type: iterations that resolve the instance's fields replace the
-- earlier approximations instead of accumulating beside them
-- (session reader: session.open(id) then reader:contexts()).
local session = {}

local reader = {
    session_id = nil :: string?,
    cache = nil :: any,
}
reader.__index = reader

function session.open(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end
    local self = setmetatable({}, reader)
    self.session_id = session_id
    self.cache = nil
    return self, nil
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
