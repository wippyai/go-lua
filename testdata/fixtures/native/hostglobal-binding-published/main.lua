-- Reading a declared host global publishes the whole binding row: identity,
-- type, use order and value carrier. Type ingress alone is not the binding.

local handle = stream.open("input")
local id: string = handle.id
return id
