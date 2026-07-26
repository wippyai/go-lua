-- The residue of a positive constant modulus occupies [0, modulus-1], and that
-- window is a fact about the binding holding it. A test against a value the
-- window excludes is refuted, so this arm never runs and the optional element
-- read inside it is nobody's obligation.
local function outside_window(zs: {number}, i: integer): number
    i = i % 4
    if i >= 0 and #zs >= 3 and i == 5 then
        local v: number = zs[i + 1]
        return v
    end
    return 0
end

-- A value the window admits decides nothing: 1 is a residue of 4, so the arm
-- stays reachable and the read whose index can exceed the proven length floor
-- is still an obligation.
local function inside_window(zs: {number}, i: integer): number
    i = i % 4
    if i >= 0 and #zs >= 3 and i == 1 then
        local v: number = zs[i + 1] -- expect-error
        return v
    end
    return 0
end

return outside_window, inside_window
