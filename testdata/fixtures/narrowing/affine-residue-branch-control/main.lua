-- A constant shift moves the whole residue window: (i % 3) + 10 occupies
-- [10, 12], so a ceiling of five refutes the arm this affine-derived control
-- guards.
local function shifted_window(zs: {number}, i: integer): number
    i = (i % 3) + 10
    if i >= 0 and #zs >= 3 and i <= 5 then
        local v: number = zs[i + 1]
        return v
    end
    return 0
end

-- The shifted window is the whole proof: a ceiling above it decides nothing,
-- so the arm stays reachable and its read is still an obligation.
local function shifted_window_admits(zs: {number}, i: integer): number
    i = (i % 3) + 10
    if i >= 0 and #zs >= 3 and i <= 20 then
        local v: number = zs[i + 1] -- expect-error
        return v
    end
    return 0
end

-- A modulo control over a subject that already carries a window is decided by
-- the intersection: i % 2 occupies [0, 1] and the class 3 (mod 4) has no member
-- there, so the arm never runs.
local function class_outside_window(zs: {number}, i: integer): number
    i = i % 2
    if i >= 0 and #zs >= 3 and i % 4 == 3 then
        local v: number = zs[i + 5]
        return v
    end
    return 0
end

-- The intersection is the whole proof: the class 1 (mod 4) does hold a member
-- of [0, 1], so this control decides nothing and the read stays an obligation.
local function class_inside_window(zs: {number}, i: integer): number
    i = i % 2
    if i >= 0 and #zs >= 3 and i % 4 == 1 then
        local v: number = zs[i + 5] -- expect-error
        return v
    end
    return 0
end

return shifted_window, shifted_window_admits, class_outside_window, class_inside_window
