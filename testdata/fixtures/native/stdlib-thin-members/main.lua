-- The declared stdlib members no other fixture calls. Each is an independent
-- one-liner, so one pass exercises them all: the two table length readers, the
-- three raw/metatable readers, the two binary string members, the three
-- coroutine control readers, and the two error readers.
local function lengths(rows: any): number
    return table.getn(rows) + table.maxn(rows)
end

local function raw_reads(): string
    local left = {id = "left"}
    local right = {id = "right"}
    local shadow = setmetatable({}, {__index = left})

    local same = rawequal(left, left) and not rawequal(left, right)
    local direct = rawget(shadow, "id")
    local meta = getmetatable(shadow)
    if meta == nil then
        return tostring(same) .. "/" .. tostring(direct) .. "/no metatable"
    end
    return tostring(same) .. "/" .. tostring(direct) .. "/" .. type(meta)
end

local function binary(): number
    local packed = string.pack("i4i4", 1, 2)
    return #packed + string.packsize("i4i4")
end

local function coroutines(): string
    local body = function(): string return "body" end
    local co = coroutine.create(body)
    local wrapped = coroutine.wrap(body)
    local status = coroutine.status(co)
    local current = coroutine.running()
    return status .. "/" .. tostring(wrapped()) .. "/" .. type(current)
end

local function failures(): string
    local failure = errors.new({message = "unreachable", kind = errors.UNAVAILABLE})
    local matched = errors.is(failure, errors.UNAVAILABLE)
    local stack = errors.call_stack(failure)
    if stack == nil then
        return tostring(matched) .. "/no stack"
    end
    return tostring(matched) .. "/" .. stack.thread .. "/" .. tostring(#stack.frames)
end

return {
    lengths = lengths,
    raw_reads = raw_reads,
    binary = binary,
    coroutines = coroutines,
    failures = failures,
}
