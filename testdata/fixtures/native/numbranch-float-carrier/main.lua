-- `/` always yields the VM float arm. With a non-zero literal divisor and an integer
-- numerator the quotient is a finite float, so the branch publishes the float
-- representation and a total order.

local function half_below(n: integer): number
    local q = n / 2
    if q < 1.0 then
        return 0.0
    end
    return q
end

return half_below
