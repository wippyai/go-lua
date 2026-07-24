-- Contract: a function whose result depends on a mutable captured local is not
-- caller invariant; a store through the captured cell revokes the summary and
-- must appear in its deopt-trigger set.

local offset: number = 1

local function shift(v: number): number
    return v + offset
end

local before: number = shift(1)
offset = offset + 1
local after: number = shift(1)

return before + after
