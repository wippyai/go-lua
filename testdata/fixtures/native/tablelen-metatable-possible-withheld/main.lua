-- setmetatable flips the __len disposition from proved-absent to unknown; the
-- raw length fact is withheld, never silently kept as raw.
local t: {number} = {}
for i = 1, 4 do
    t[i] = i
end

setmetatable(t, {})

return #t
