-- Contract: recursion routed through a mutable table field closes no provable
-- call cycle; the SCC row must be withheld rather than assumed.

local m = {}

function m.step(n: number): number
    if n <= 0 then
        return 0
    end
    return m.step(n - 1) + 1
end

return m
