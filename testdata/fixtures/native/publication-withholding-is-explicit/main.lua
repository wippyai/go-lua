-- PUBLICATION: withholding is EXPLICIT. Both operands are number, so the scalar
-- operator row publishes; the divisor cannot be proven nonzero, so that row must
-- be withheld with a reason. Silent absence is indistinguishable from a bug.
local function ratio(n: number, d: number): number
    return n / d
end

return ratio
