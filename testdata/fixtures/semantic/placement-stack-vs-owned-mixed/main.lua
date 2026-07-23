local box = { items = {} }
local owned = { id = "o", child = { v = 1 } }
local stacked = { id = "s", child = { v = 2 } }
ownership.store(owned, box)
local a: number = owned.child.v
local b: number = stacked.child.v
print(tostring(a) .. tostring(b))
