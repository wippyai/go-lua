local function describe(value: {[string]: any} | integer): string
    if type(value) == "number" then
        return "count"
    end
    return "table"
end

local function first_kind(values: {[string]: any} | integer | nil): {[string]: any} | integer | nil
    return values
end

local shape = { kind = "node" }
local counted: {[string]: any} | integer = shape
local described = describe(shape)
local passed = first_kind(shape)

return { counted = counted, described = described, passed = passed }
