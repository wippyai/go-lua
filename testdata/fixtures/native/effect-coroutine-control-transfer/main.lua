-- EFFECT ROW: a coroutine yield and the resume that drives it both publish
-- control transfer and suspension. The resume site is a safepoint because its
-- row says so, not because the callee is named "resume".
local co = coroutine.create(function(seed: number): number
    coroutine.yield(seed + 1)
    return seed + 2
end)

local ok, first = coroutine.resume(co, 1)

return ok, first
