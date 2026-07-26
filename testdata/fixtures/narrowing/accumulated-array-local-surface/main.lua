-- An append advances the member cells of the array's heap identity rather than
-- the aggregate value the allocation published, so a reader inside the body
-- that filled the array sees the allocation shape and nothing the body put
-- there. The aggregate a return boundary rebuilds from those member cells is
-- the same aggregate the local read needs, and it is what an iteration over the
-- array reads its element from.
--
-- The rebuild carries the authority the member cells carry and no more. A
-- container handed to a callee this analysis never read may hold slots that
-- callee wrote, and a store at a key the analysis never resolved leaves the
-- member set short of the container's slots; both leave the allocation shape
-- standing.

local counts: {[string]: number} = {}

-- The elements the loop appended are what the iteration binds, so the element
-- type is the appended one and the read at that key is the enumeration's own.
local names = {}
for key in pairs(counts) do
    table.insert(names, key)
end
for _, name in ipairs(names) do
    local spelled: string = name
    local wrong: number = name -- expect-error
    local hits: number = counts[name]
end

-- A container a callee this analysis never read received may hold slots that
-- callee put there, so the member cells no longer describe it.
local escaped = {}
for key in pairs(counts) do
    table.insert(escaped, key)
end
unresolved_sink(escaped)
for _, name in ipairs(escaped) do
    local spelled: string = name -- expect-error
end

-- A store at a key the analysis never resolved leaves the member set short of
-- the slots the container holds.
local unresolved = {}
for key in pairs(counts) do
    table.insert(unresolved, key)
    unresolved[key] = key
end
for _, name in ipairs(unresolved) do
    local spelled: string = name -- expect-error
end

return names, escaped, unresolved
