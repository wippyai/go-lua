-- PUBLICATION IDENTITY: for a fixed input the published fact set is byte-identical
-- across runs -- same rows, same order, same keys. A JIT that caches compiled
-- artifacts keyed by fact digests cannot function without it.
type Sample = {
    id: string,
    weight: number,
}

local function total(samples: {Sample}): number
    local sum: number = 0
    for i = 1, #samples do
        sum = sum + samples[i].weight
    end
    return sum
end

local rows: {Sample} = {
    { id = "a", weight = 1 },
    { id = "b", weight = 2 },
}

return total(rows)
