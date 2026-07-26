-- A store the body places inside a cycle runs once per trip, and running none
-- of the trips is an execution of that cycle too. The slot one trip names holds
-- either the value that trip wrote or whatever the slot held on the way in, so
-- a slot the cycle itself introduces admits nil. The key a trip resolves is that
-- trip's iterate and names no slot the fixed point holds occupied.
--
-- The counter advanced after the append is one ahead of the slot the append
-- filled: the first trip writes buf[0] and leaves n at 1, so n >= 1 states
-- nothing about buf[1].

local function trip_one(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        buf[n] = s
        n = n + 1
    end

    if n >= 1 then
        local first: string = buf[1] -- expect-error
        return first
    end
    return ""
end

-- The slot the first trip does fill is not established either: the execution
-- that runs no trip reaches this read with buf still empty.
local function trip_one_first_slot(src: {string}): string
    local n: integer = 0
    local buf: {string} = {}
    for _, s in ipairs(src) do
        buf[n] = s
        n = n + 1
    end

    if n >= 1 then
        local first: string = buf[0] -- expect-error
        return first
    end
    return ""
end

-- A key this body cannot name carries the same reading: the stores establish
-- the element contract of the slots they fill and no slot's occupancy.
local function dynamic_key(src: {string}, at: integer): string
    local buf: {string} = {}
    for _, s in ipairs(src) do
        buf[at] = s
    end

    local first: string = buf[1] -- expect-error
    return first
end

-- A constant key names one slot on every trip, and the execution that runs no
-- trip leaves that slot empty.
local function constant_key(src: {string}): string
    local buf: {string} = {}
    local k: integer = 1
    for _, s in ipairs(src) do
        buf[k] = s
    end

    local first: string = buf[1] -- expect-error
    return first
end

-- A slot established before the cycle keeps its type: the write on the way in
-- is the value the zero-trip execution reaches, and every trip stores the same
-- element contract over it.
local function pre_loop_seed(src: {string}, seed: string): string
    local buf: {string} = {}
    local k: integer = 1
    buf[k] = seed
    local n: integer = 1
    for _, s in ipairs(src) do
        buf[n] = s
        n = n + 1
    end

    local first: string = buf[1]
    return first
end

-- The paired counter is the occupancy proof the read rests on: the increment
-- precedes the append, so every trip fills the slot the count reaches and
-- n >= 1 witnesses buf[1].
local function density_paired(src: {string}): string
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

return trip_one, trip_one_first_slot, dynamic_key, constant_key, pre_loop_seed, density_paired
