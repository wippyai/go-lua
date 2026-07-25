-- The declared contracts are the whole basis here. string.sub is
-- sub(s: string, i: integer, j: integer?) -> string, total in every argument, so
-- its result is a string with or without a guard. string.byte is
-- byte(s: string, i: integer?, j: integer?) -> integer?, optional because the
-- position may be past the end, and a length floor is what discharges that
-- optionality for the positions the floor covers.

local function need_integer(value: integer): integer
    return value
end

-- The guard is not what makes sub total; the contract is.
local function head(s: string): string
    if #s >= 3 then
        local t: string = s:sub(1, 3)
        return t
    end
    return ""
end

-- A floor of 3 puts positions 1 through 3 inside the string, so the declared
-- optional cannot be nil there.
local function first_byte(s: string): integer
    if #s >= 3 then
        local b = s:byte(1)
        return need_integer(b)
    end
    return 0
end

-- The floor covers position 3 and stops there: position 4 may be past the end.
local function past_floor(s: string): integer
    if #s >= 3 then
        local b = s:byte(4)
        return need_integer(b) -- expect-error
    end
    return 0
end

-- Without the floor the string may be empty and position 1 is past its end.
local function unguarded(s: string): integer
    local b = s:byte(1)
    return need_integer(b) -- expect-error
end

-- The optionality belongs to the contract, not to the position that consumes it:
-- an annotated local carries the same obligation as an argument.
local function unguarded_assigned(s: string): integer
    local b: integer = s:byte(1) -- expect-error
    return b
end

-- Nor does nesting the call inside the argument discharge it.
local function unguarded_nested(s: string): integer
    return need_integer(s:byte(1)) -- expect-error
end

return head, first_byte, past_floor, unguarded, unguarded_assigned,
    unguarded_nested
