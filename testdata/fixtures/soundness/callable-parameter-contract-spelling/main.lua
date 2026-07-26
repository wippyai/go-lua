-- A parameter is omittable when its declared type admits nil. That is a
-- property of the contract, not of how the contract renders: a callable whose
-- own result is optional renders with a trailing question mark and still
-- requires an argument, while an optional parameter accepts the omission.

local function each(visit: fun(item: string): string?): number
    return 1
end

local function tagged(name: string, suffix: string?): number
    return 1
end

-- The callback is required: its type states a function, and the question mark
-- belongs to that function's result.
local function omits_required_callback(): number
    return each() -- expect-error: each expects 1 arguments, got 0
end

-- Supplying it satisfies the same contract.
local function supplies_callback(): number
    return each(function(item: string): string? return item end)
end

-- A parameter whose own type admits nil stays omittable.
local function omits_optional_parameter(): number
    return tagged("head")
end

return {
    omits_required_callback = omits_required_callback,
    supplies_callback = supplies_callback,
    omits_optional_parameter = omits_optional_parameter,
}
