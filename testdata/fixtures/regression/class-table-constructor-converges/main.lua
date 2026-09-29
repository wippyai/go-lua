-- A constructor stores a guarded optional parameter on an instance whose
-- metatable is its class table. Every nested function sees the class through
-- one recursion identity, so the class and the instance type converge.
local Plain = {}
Plain.__index = Plain

function Plain.new(name: string?)
    if not name then
        return nil
    end
    local self = setmetatable({}, Plain)
    self.name = name
    return self
end

local Class = {}
Class.__index = Class

function Class.new(name: string)
    local self = setmetatable({}, Class)
    self.name = name
    return self
end

function Class:get(): string
    return self.name
end

local function use(name: string): string?
    local c = Class.new(name)
    return c:get()
end

return { Plain = Plain, Class = Class, use = use }
