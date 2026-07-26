-- A keyed iteration visits the slots the table holds, so the variable it binds
-- each key to names a slot that holds a value. A map read at that key is
-- therefore present, while every other read of the same map keeps Lua's
-- missing-slot nil.

local counts: {[string]: number} = {}
counts["alpha"] = 1
counts["beta"] = 2

-- The key came from the iteration of this table, so the slot it names holds a
-- value and the read is the declared element without nil.
for name in pairs(counts) do
    local hits: number = counts[name]
    local shown: string = name
end

-- A key the iteration never produced is an ordinary map read: the declared
-- element with the missing-slot nil.
local elsewhere = tostring(1)
for name in pairs(counts) do
    local hits: number = counts[elsewhere] -- expect-error
end

-- The relation names one table. Another map with the same key type was never
-- enumerated, so it proves nothing about that key.
local mirror: {[string]: number} = {}
for name in pairs(counts) do
    local hits: number = mirror[name] -- expect-error
end

-- Rebinding the key leaves the proof stated about a key the body has left.
for name in pairs(counts) do
    name = tostring(2)
    local hits: number = counts[name] -- expect-error
end

-- A dynamic write can clear the enumerated slot, so the read after it is
-- optional again.
for name in pairs(counts) do
    counts[elsewhere] = nil
    local hits: number = counts[name] -- expect-error
end

-- A static write names its slot exactly, and that slot may be the enumerated
-- key, so it drops the proof exactly as a dynamic write does.
for name in pairs(counts) do
    counts.alpha = nil
    local hits: number = counts[name] -- expect-error
end

-- A callee whose body this analysis reads keeps the proof when that body makes
-- no write: the writes a local callee does make are already projected back onto
-- the table it received, and there are none here.
local function peek(m: {[string]: number}) end

for name in pairs(counts) do
    peek(counts)
    local hits: number = counts[name]
end

-- A callee with no body to read may have cleared the enumerated slot, and no
-- write of its own was ever projected back, so the proof drops.
local sink: fun(m: {[string]: number})

for name in pairs(counts) do
    sink(counts)
    local hits: number = counts[name] -- expect-error
end

-- A metatable answers a read Lua's raw slot never held, so an enumerated key
-- proves nothing about what the read returns.
local shadowed: {[string]: number} = {}
setmetatable(shadowed, { __index = function(_, _) return nil end })

for name in pairs(shadowed) do
    local hits: number = shadowed[name] -- expect-error
end
