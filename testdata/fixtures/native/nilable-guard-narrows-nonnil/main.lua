-- A truthiness guard on an optional parameter publishes nilability per edge: non_nil on the
-- then edge, nil on the else edge, because nil is the only falsy inhabitant of string?.

local function label(x: string?): string
    if x then
        return x
    end
    return "none"
end

return label
