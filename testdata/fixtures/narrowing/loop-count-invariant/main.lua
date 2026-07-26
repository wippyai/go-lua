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

-- A step of two skips the slots the count still names: after one iteration n is
-- 2 and only buf[2] is written, so buf[1] is a hole.
local function striding(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 2
        buf[n] = s
    end

    if n >= 1 then
        local first: string = buf[1] -- expect-error
        return first
    end
    return ""
end

-- The counter enters the loop at one while the container enters it empty, so
-- the first write lands at buf[2] and the count stays one ahead of the length.
local function offset_seed(src: {string}): string
    local n: integer = 1
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end

    if n >= 1 then
        local first: string = buf[1] -- expect-error
        return first
    end
    return ""
end

-- A write to the counter outside the pair raises the count with no write
-- reaching the sequence, so n >= 1 no longer states that buf holds a slot.
local function counted_twice(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end
    n = n + 1

    if n >= 1 then
        local first: string = buf[1] -- expect-error
        return first
    end
    return ""
end

-- A write into the sequence outside the pair lands at a slot the count does not
-- name, so the count no longer states the sequence's length.
local function extra_slot(src: {string}, at: integer): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        n = n + 1
        buf[n] = s
    end
    buf[at] = "extra"

    if n >= 1 then
        local first: string = buf[1] -- expect-error
        return first
    end
    return ""
end

return collect, past_last, sparse, striding, offset_seed, counted_twice, extra_slot
