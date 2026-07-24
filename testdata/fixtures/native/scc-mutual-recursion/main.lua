-- Contract: two mutually recursive functions form a single call SCC; one row
-- closes both edges rather than two rows each closing one.

local is_odd: (number) -> boolean

local function is_even(n: number): boolean
    if n == 0 then
        return true
    end
    return is_odd(n - 1)
end

is_odd = function(n: number): boolean
    if n == 0 then
        return false
    end
    return is_even(n - 1)
end

return is_even(10)
