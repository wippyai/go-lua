type Mapper<T, U> = fun(x: T): U
local function map<T, U>(arr: {T}, f: Mapper<T, U>): {U}
    local result: {U} = {}
    for i, v in ipairs(arr) do
        result[i] = f(v)
    end
    return result
end
local nums = map({"1", "2", "3"}, function(s: string): number
    return tonumber(s) or 0
end)
-- The mapper result is typed {U}: a typed sequence has unknown runtime length,
-- so an arbitrary index read nums[1] is U? and assigning it to a non-optional
-- number is soundly rejected (no length proof eliminates nil).
local n: number = nums[1] -- expect-error: cannot assign number? to number
