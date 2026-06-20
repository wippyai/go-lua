-- Direct congruence over a field: p == q carries p.f ~= nil to q.f.
local function direct(p: { f: number? }, q: { f: number? }): number
    if p == q and p.f ~= nil then
        local v: number = q.f
        return v
    end
    return 0
end

-- Transitive chain: a == b == c, narrowing a.f reaches c.f.
local function chain(a: { f: number? }, b: { f: number? }, c: { f: number? }): number
    if a == b and b == c and a.f ~= nil then
        local v: number = c.f
        return v
    end
    return 0
end

-- Nested member: p == q carries p.inner.x ~= nil to q.inner.x.
local function nested(p: { inner: { x: number? } }, q: { inner: { x: number? } }): number
    if p == q and p.inner.x ~= nil then
        local v: number = q.inner.x
        return v
    end
    return 0
end

-- Movable symbol soundness: q is reassigned after the equality, so the proof is
-- stale and q.f must stay optional.
local function moved(p: { f: number? }, q: { f: number? }, other: { f: number? }): number
    if p == q and p.f ~= nil then
        q = other
        local v: number = q.f -- expect-error
        return v
    end
    return 0
end

-- Movable target in a chain: the read target c is reassigned after the a==b==c
-- equality, so the new c is unrelated and c.f stays optional.
local function moved_chain(a: { f: number? }, b: { f: number? }, c: { f: number? }, other: { f: number? }): number
    if a == b and b == c and a.f ~= nil then
        c = other
        local v: number = c.f -- expect-error
        return v
    end
    return 0
end

-- No equality proven: distinct objects, q.f stays optional.
local function unrelated(p: { f: number? }, q: { f: number? }): number
    if p.f ~= nil then
        local v: number = q.f -- expect-error
        return v
    end
    return 0
end

return direct, chain, nested, moved, moved_chain, unrelated
