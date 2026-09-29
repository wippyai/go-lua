-- Lua's length can be a positive border even when an earlier slot is a hole.
local sparse = { [2] = { label = "present" } }
assert(#sparse == 2)
local missing = sparse[1].label -- expect-error
local present = sparse[2].label
