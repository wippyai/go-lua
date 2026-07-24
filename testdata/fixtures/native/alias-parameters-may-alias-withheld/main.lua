-- Two parameters of the same record type may be the same table: the caller is free to pass
-- one table twice, so disjointness is not provable and no alias_disjoint row may be granted.
type Cell = { n: number }

local function combine(a: Cell, b: Cell): number
    local first = a.n
    b.n = 9
    return first + a.n
end

return combine
