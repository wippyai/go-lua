-- An argument reached through an any boundary proves no concrete parameter
-- contract: whatever shape the value carries crossed that boundary without
-- validation, so it cannot discharge the callee's declared parameter type.

local function need(s: string): number return 1 end
local function sink(v: any): number return 1 end

-- The declared any formal cannot satisfy a concrete parameter.
local function launder(x: any): number
    return need(x) -- expect-error
end

-- A top parameter accepts a top argument: nothing concrete is required.
local function top_into_top(x: any): number
    return sink(x)
end

-- A runtime type test is the boundary's own validator.
local function validated(x: any): number
    if type(x) == "string" then
        return need(x)
    end
    return 0
end

-- A concrete cast is runtime validation on the normal path.
local function casted(x: any): number
    return need(x as string)
end

return launder, top_into_top, validated, casted
