-- A record literal built from an any value cannot satisfy a concrete declared
-- field type: the field carries the same unvalidated boundary the value did.

local function launder(x: any): {message: string}
    return { message = x } -- expect-error
end

-- A top-typed field requires no proof.
local function top_field(x: any): {message: any}
    return { message = x }
end

-- A runtime type test proves the field value for the declared type.
local function validated(x: any): {message: string}
    if type(x) == "string" then
        return { message = x }
    end
    return { message = "" }
end

-- A concrete cast is runtime validation on the normal path.
local function casted(x: any): {message: string}
    return { message = x as string }
end

return launder, top_field, validated, casted
