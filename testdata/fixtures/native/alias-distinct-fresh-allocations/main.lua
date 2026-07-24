-- Two fresh, non-escaping table allocations in the same scope are provably disjoint, which
-- is what licenses forwarding a load across a store into the other one.

local function build(): number
    local a = { n = 1 }
    local b = { n = 2 }
    local first = a.n
    b.n = 9
    return first + a.n
end

return build
