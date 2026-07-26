-- No number is both at least five and at most two, so the conjunction guarding
-- this arm has no model: the arm is an instruction stream no execution enters
-- and the optional element read inside it is nobody's obligation.
local function unreachable_arm(zs: {number}, i: integer): number
    if i >= 5 and i <= 2 and #zs >= 3 then
        local v: number = zs[(i % 5) + 1]
        return v
    end
    return 0
end

-- The same shape over a satisfiable range decides nothing. Both arms stay
-- reachable, so the read whose index can exceed the proven length floor is
-- still an obligation the guard does not discharge.
local function reachable_arm(zs: {number}, i: integer): number
    if i >= 2 and i <= 5 and #zs >= 3 then
        local v: number = zs[(i % 5) + 1] -- expect-error
        return v
    end
    return 0
end

-- The two disjuncts exclude each other, but a disjunction states neither of
-- them: every value of i reaches this arm through one side or the other, so the
-- arm is reachable and its read is still an obligation.
local function disjunctive_arm(zs: {number}, i: integer): number
    if i >= 0 and #zs >= 3 and (i >= 5 or i <= 2) then
        local v: number = zs[(i % 5) + 1] -- expect-error
        return v
    end
    return 0
end

return unreachable_arm, reachable_arm, disjunctive_arm
