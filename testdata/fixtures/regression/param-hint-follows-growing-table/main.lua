local methods = {}

local function resolve(self, value)
    if value then
        return resolve(self, nil)
    end
    return value
end

function methods:route(content)
    local r = resolve(self, content)
    return r
end

function methods:command(cmd)
    return self, nil
end

return methods
