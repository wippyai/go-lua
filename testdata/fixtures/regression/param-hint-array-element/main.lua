-- Unannotated params typed from call-site hints must admit the arguments of every contributing call site.
local function parse_field(v: string)
    local values = {}
    if v == "*" then
        for i = 0, 5 do table.insert(values, i) end
        return values, nil
    end
    local n = tonumber(v)
    if not n then return nil, "bad" end
    table.insert(values, n)
    return values, nil
end

local function parse(expr: string)
    local parsed = {}
    local days, err = parse_field(expr)
    if err then return nil, err end
    parsed.days = days
    return parsed, nil
end

local function contains(array, value)
    for _, v in ipairs(array) do
        if v == value then return true end
    end
    return false
end

local function matches(t: number, spec)
    return contains(spec.days, t)
end

local function find_next(spec, from: number)
    for i = 1, 10 do
        if matches(from + i, spec) then return from + i end
    end
    return nil
end

local function next_run(expr: string): number?
    local spec, err = parse(expr)
    if err or not spec then return nil end
    return find_next(spec, 0)
end

return { next_run = next_run }
