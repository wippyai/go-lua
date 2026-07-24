-- The sequence continues in hash storage, so the array-header length is not
-- the Lua result of #t and no length fact may be published.
local t: {number} = {}
t[1] = 1
t[2] = 2
t[3] = 3
t[100] = 100

return #t
