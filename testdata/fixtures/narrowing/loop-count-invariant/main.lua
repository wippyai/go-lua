-- The counter and the array it fills advance together. Each iteration raises n by
-- one from zero and writes exactly buf[n], so the written prefix 1..n is dense and
-- no slot in it is ever cleared. n >= 1 therefore witnesses buf[1].

local function collect(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end

    if n >= 1 then
        local first: string = buf[1]
        return first
    end
    return ""
end

-- The invariant reaches the last written slot and stops there: buf[n + 1] is the
-- slot the next iteration would have filled.
local function past_last(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end

    if n >= 1 then
        local beyond: string = buf[n + 1] -- expect-error
        return beyond
    end
    return ""
end

-- A conditional write breaks density: n counts every element while buf receives
-- only some, so buf[1] may be a hole even when n >= 1.
local function sparse(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        if s ~= "" then
            buf[n] = s
        end
    end

    if n >= 1 then
        local first: string = buf[1] -- expect-error
        return first
    end
    return ""
end

return collect, past_last, sparse
