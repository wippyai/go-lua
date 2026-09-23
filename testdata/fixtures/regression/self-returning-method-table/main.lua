local methods = {}

local function describe(self, value)
    if type(value) ~= "table" then
        return nil
    end
    return value
end

function methods:route(content)
    describe(self, content)
    for _, target in ipairs(self.targets) do
        local _, err = self:data(target.kind)
        if err then
            return nil, "route failed: " .. tostring(err)
        end
    end
    table.insert(self.queue, content)
    return self, nil
end

function methods:ids()
    return self.created
end

return methods
