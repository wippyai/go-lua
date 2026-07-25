-- A returned any is the opaque top: the declared return type is a concrete
-- contract the body never discharged, so the return is refuted until a runtime
-- validation republishes a proof for the same value.

local function launder(x: any): number
    return x -- expect-error
end

-- A top return type requires no proof.
local function top_return(x: any): any
    return x
end

-- A runtime type test decides the declared return type outright.
local function validated(x: any): number
    if type(x) == "number" then
        return x
    end
    return 0
end

-- A concrete cast is runtime validation on the normal path.
local function casted(x: any): number
    return x as number
end

return launder, top_return, validated, casted
