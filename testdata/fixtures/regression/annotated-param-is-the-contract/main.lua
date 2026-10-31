-- An explicit parameter annotation is the contract: call-site hints never
-- replace it, even when it contains any.
local function take_any(m: {[string]: any}): number
    local n = 0
    for _ in pairs(m) do n = n + 1 end
    return n
end

local ok = take_any({ a = 1 })
local bad = take_any(42) -- expect-error: argument 1
return ok + bad
