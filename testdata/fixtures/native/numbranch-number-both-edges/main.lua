-- Mirrors flow/return-early-in-if. A normalized branch bound is not numeric execution
-- authority: the carrier and its representation must ride BOTH edges of the branch.

local function f(x: number): number
    if x < 0 then
        return 0
    end
    return x
end

return f
