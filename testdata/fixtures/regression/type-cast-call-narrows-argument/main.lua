type Point = {x: number, y: number}

-- A type-cast call narrows a bare-symbol argument to the cast type on the
-- continuation (a failed cast raises, so past the call the argument IS Point).
local function validate(data: any)
    Point(data)
    local p: {x: number, y: number} = data
    return p
end

-- The cast result and the narrowed argument agree under assignment.
local function validate_assign(data: any)
    local v = Point(data)
    local q: {x: number, y: number} = v
    local r: {x: number, y: number} = data
    return r
end

-- A wrapper that returns a cast forwards the narrowing to its callers.
local function expect_point(x)
    return Point(x)
end

local function validate_wrapped(data: any)
    expect_point(data)
    local p: {x: number, y: number} = data
    return p
end

return validate, validate_assign, validate_wrapped
