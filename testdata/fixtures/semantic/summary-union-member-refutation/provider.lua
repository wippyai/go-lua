local M = {}

-- Every return site is a sealed closed literal, so the inferred result union
-- enumerates every result this call can produce.
function M.finite(flag: boolean)
    if flag then
        return {kind = "event", name = "a"}
    end
    return {kind = "timer", elapsed = 1}
end

-- One return site is an opaque call result, so no result summary is inferred
-- and the caller keeps no member surface to decide against.
function M.opaque(flag: boolean, make: () -> {kind: string})
    if flag then
        return {kind = "event", name = "a"}
    end
    return make()
end

return M
