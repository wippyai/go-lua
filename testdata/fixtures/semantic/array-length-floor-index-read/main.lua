local function numeric_for_clean(arr: {number}): number
    local total = 0
    for i = 1, #arr do
        local v: number = arr[i]
        total = total + v
    end
    return total
end

local function guarded_by_length(arr: {string}, i: integer): string
    if i >= 1 and i <= #arr then
        local v: string = arr[i]
        return v
    end
    return "fallback"
end

local function guarded_by_floor_and_ceil(arr: {string}, i: integer): string
    if #arr >= 3 and i >= 1 and i <= 3 then
        local v: string = arr[i]
        return v
    end
    return "fallback"
end

local literal = {"a", "b", "c"}
local first: string = literal[1]
local second: string = literal[2]
local third: string = literal[3]
local fourth: string = literal[4] -- expect-error

local function table_insert_new_index(): string
    local arr = {"a", "b"}
    table.insert(arr, "c")
    local c: string = arr[3]
    return c
end

local function direct_append_new_index(): string
    local arr: {string} = {"a", "b"}
    arr[#arr + 1] = "c"
    local c: string = arr[3]
    return c
end

local function escaped_after_unknown_writer(arr: {string}): string
    if #arr >= 1 then
        local unknown_writer = (function(_) end) :: any
        unknown_writer(arr)
        local stale: string = arr[1] -- expect-error
        return stale
    end
    return "fallback"
end

local n = numeric_for_clean({1, 2, 3})
local a = guarded_by_length({"x", "y"}, 2)
local b = guarded_by_floor_and_ceil({"x", "y", "z"}, 3)
local c = table_insert_new_index()
local d = direct_append_new_index()
local e = escaped_after_unknown_writer({"safe"})

return first .. second .. third .. a .. b .. c .. d .. e .. tostring(n) .. tostring(fourth)
