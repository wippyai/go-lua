type Point = {x: number, y: number}

-- A regular function is not a type-cast: calling it asserts nothing about its
-- argument's type, so the argument stays gradual and is not assignable to Point.
local function consume(x: any)
    return x
end

local function validate(data: any)
    consume(data)
    local p: {x: number, y: number} = data -- expect-error
    return p
end

return validate
