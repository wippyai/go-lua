-- A guard excluding both 0 and -1 discharges both integer floor-division traps:
-- the divide-by-zero error and the minimum-integer overflow.

local function idiv(a: integer, b: integer)
    if b ~= 0 and b ~= -1 then
        return a // b
    end
    return 0
end

return idiv
