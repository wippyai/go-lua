type Pair<A, B> = { first: A, second: B, nest: Pair<B, A>? }

local function depth<A, B>(p: Pair<A, B>?): number
    if p == nil then return 0 end
    return 1 + depth(p.nest)
end

local p: Pair<number, string> = { first = 1, second = "x", nest = nil }
return depth(p)
