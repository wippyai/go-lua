-- A branch inside a loop owns its arms per evaluation. The back edge lets the
-- other arm reach this point again, but only by evaluating the same branch a
-- second time, and that is a later iteration rather than an alternate edge of
-- the decision this point is under. The arm a point sits in is therefore the
-- arm of the last evaluation before it, exactly as outside a loop.

local xs: {string?} = {}

-- Truthiness inside the loop body narrows the bound element.
for _, x in ipairs(xs) do
    if x then
        local kk: string = x
    end
end

-- The nil comparison is the same decision.
for _, x in ipairs(xs) do
    if x ~= nil then
        local kk: string = x
    end
end

-- The non-loop control: the same guard on the same declared type.
local single: string? = xs[1]
if single then
    local kk: string = single
end

-- The false arm keeps the other side of the decision, so the element there is
-- still optional and the annotation does not hold.
for _, x in ipairs(xs) do
    if x then
        local ok: string = x
    else
        local bad: string = x -- expect-error
    end
end

-- Past the branch both arms rejoin, so no arm owns the point and the element
-- keeps its declared type.
for _, x in ipairs(xs) do
    if x then
        local ok: string = x
    end
    local after: string = x -- expect-error
end

-- A nested branch composes: the inner arm is owned under the outer one.
local ys: {string?} = {}
for _, y in ipairs(ys) do
    if y ~= nil then
        if #y > 0 then
            local inner: string = y
        end
    end
end

-- The guard of one iteration does not carry into the next: the element the
-- next iteration binds is a fresh read of the declared optional element.
for _, z in ipairs(xs) do
    if z then
        local held: string = z
    end
    for _, w in ipairs(xs) do
        local nested: string = w -- expect-error
    end
end

return xs, ys
