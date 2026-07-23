type A = { b: B?, tag: "a" }
type B = { c: C?, tag: "b" }
type C = { a: A?, tag: "c" }

local function walk(a: A?): number
    if a == nil then return 0 end
    if a.b == nil then return 1 end
    if a.b.c == nil then return 2 end
    return 3 + walk(a.b.c.a)
end

local a: A = { b = nil, tag = "a" }
return walk(a)
