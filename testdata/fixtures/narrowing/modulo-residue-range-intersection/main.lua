-- A residue class intersected with an integer range leaves fewer values than
-- the range alone: 1 <= i <= 2 with i % 2 == 1 leaves i == 1, so a length floor
-- of one is enough to put xs[i] in range.
local function residue_pins_range(xs: {number}, i: integer): number
    if #xs >= 1 and i >= 1 and i <= 2 and i % 2 == 1 then
        local v: number = xs[i]
        return v
    end
    return 0
end

-- The same residue over a wider range leaves 1 and 3, and 3 exceeds a floor of
-- one, so the read stays optional and the declaration that admits no nil is
-- refuted.
local function residue_leaves_range_open(zs: {number}, i: integer): number
    if #zs >= 1 and i >= 1 and i <= 4 and i % 2 == 1 then
        local v: number = zs[i] -- expect-error
        return v
    end
    return 0
end

-- Without the residue the range still admits 2, which exceeds a floor of one,
-- so the read stays optional here too.
local function range_without_residue(ws: {number}, i: integer): number
    if #ws >= 1 and i >= 1 and i <= 2 then
        local v: number = ws[i] -- expect-error
        return v
    end
    return 0
end

return residue_pins_range, residue_leaves_range_open, range_without_residue
