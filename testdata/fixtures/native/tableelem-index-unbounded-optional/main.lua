-- With no upper bound the read stays T?: presence must not be published and
-- the row must state maybe_nil.
local function at(xs: {number}, i: number): number?
    if i >= 1 then
        return xs[i]
    end
    return nil
end

return at
