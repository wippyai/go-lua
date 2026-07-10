-- A local predicate returning a builtin type guard (a conjunction) narrows its
-- argument to the tested kind on the TRUE edge of the calling guard.
local function is_positive_number(value)
    return type(value) == "number" and value > 0
end

local function run(value: any): number
    if is_positive_number(value) then
        local n: number = value
        return n + 1
    end
    return 0
end

return run
