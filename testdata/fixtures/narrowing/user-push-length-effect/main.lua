-- push writes at the border: for a dense sequence #t + 1 is the first free slot,
-- so one call extends the caller's array by exactly one element. The effect is the
-- callee's, and the caller's index proof has to carry it across the call.

local function push(t: {string}, v: string)
    t[#t + 1] = v
end

local function one(): string
    local xs: {string} = {}
    push(xs, "a")
    local first: string = xs[1]
    return first
end

local function two(): string
    local xs: {string} = {}
    push(xs, "a")
    push(xs, "b")
    local second: string = xs[2]
    return second
end

-- One call fills slot 1 only, so slot 2 is past the length the effect established.
local function past_pushes(): string
    local xs: {string} = {}
    push(xs, "a")
    local second: string = xs[2] -- expect-error
    return second
end

return one, two, past_pushes
