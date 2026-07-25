type A = {tag: "a", value: string}
type B = {tag: "b", value: number}

-- Proven equality relates the whole value, not only its nil-ness: p == q makes
-- the discriminant read on p a discriminant read on q, so selecting the "a" arm
-- on p selects it on q too.
local function forward(p: A | B, q: A | B): string
    if p == q and p.tag == "a" then
        local s: string = q.value
        return s
    end
    return ""
end

-- The relation is symmetric: discriminating q selects the same arm for p.
local function reverse(p: A | B, q: A | B): string
    if p == q and q.tag == "a" then
        local s: string = p.value
        return s
    end
    return ""
end

-- Soundness: without a proven equality the two values are unrelated and the
-- discriminant on one says nothing about the other.
local function unrelated(p: A | B, q: A | B): string
    if p.tag == "a" then
        local s: string = q.value -- expect-error
        return s
    end
    return ""
end

-- Soundness: the read target is reassigned after the equality, so the proof is
-- stale and the union stays open.
local function reassigned(p: A | B, q: A | B, other: A | B): string
    if p == q and p.tag == "a" then
        q = other
        local s: string = q.value -- expect-error
        return s
    end
    return ""
end

return forward, reverse, unrelated, reassigned
