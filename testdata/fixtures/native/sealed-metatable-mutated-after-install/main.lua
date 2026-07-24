-- A metatable is installed and then the installed __index table itself gains a
-- method: the seal is revoked at the mutation, not at the install.

type Item = { id: string }

local methods = {}

function methods.name(self: Item): string
    return self.id
end

local mt = { __index = methods }
local obj: Item = { id = "a" }
setmetatable(obj, mt)

function methods.alias(self: Item): string
    return self.id
end

return obj.id
