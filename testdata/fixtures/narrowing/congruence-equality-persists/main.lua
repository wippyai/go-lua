-- Proven equality is a fact about the two symbols, not about the condition that
-- established it: once p == q holds on this path, a narrowing of p.f reaches
-- q.f from an inner arm as much as from the same condition.
local function nested_arms(p: { f: number? }, q: { f: number? }): number
    if p == q then
        if p.f ~= nil then
            local v: number = q.f
            return v
        end
    end
    return 0
end

-- The equality can also be established by leaving the unequal path: after the
-- early return only p == q survives, so the narrowing still carries.
local function after_early_return(p: { f: number? }, q: { f: number? }): number
    if p ~= q then
        return 0
    end
    if p.f ~= nil then
        local v: number = q.f
        return v
    end
    return 0
end

-- Transitive chain across arms: a == b outside, b == c inside, so narrowing a.f
-- reaches c.f.
local function chain_across_arms(a: { f: number? }, b: { f: number? }, c: { f: number? }): number
    if a == b then
        if b == c and a.f ~= nil then
            local v: number = c.f
            return v
        end
    end
    return 0
end

-- Soundness: the read target is reassigned after the equality, so the new q is
-- unrelated and the read stays optional.
local function reassigned(p: { f: number? }, q: { f: number? }, other: { f: number? }): number
    if p == q then
        if p.f ~= nil then
            q = other
            local v: number = q.f -- expect-error
            return v
        end
    end
    return 0
end

-- Soundness: the equality holds only on the path through the boolean guard, so
-- it says nothing where the guard was not taken.
local function equality_not_on_this_path(p: { f: number? }, q: { f: number? }, b: boolean): number
    if b then
        if p == q then
            return 0
        end
    end
    if p.f ~= nil then
        local v: number = q.f -- expect-error
        return v
    end
    return 0
end

-- Soundness: inside the inequality arm the two objects are known to differ, so
-- no narrowing carries.
local function inequality_arm(p: { f: number? }, q: { f: number? }): number
    if p ~= q then
        if p.f ~= nil then
            local v: number = q.f -- expect-error
            return v
        end
    end
    return 0
end

return nested_arms, after_early_return, chain_across_arms, reassigned, equality_not_on_this_path, inequality_arm
