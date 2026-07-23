-- Array filled in a loop, then read under an in-bounds guard.
--
-- `a` is grown to length `n` by the fill loop. Widening loses the exact length
-- relationship (the length floor smears), so a guarded read `a[k]` is typed as
-- optional `number?`. The narrowing pass recovers the length floor so the guard
-- `k >= 1 and k <= #a` proves the read in-bounds, yielding a non-optional
-- `number`.
--
-- Downstream consumer: `local v: number = a[k]` requires the read to be
-- non-optional; that annotation is the consumption point of the recovered
-- length relationship.

local function fill(n: number): {number}
	local a: {number} = {}
	for i = 1, n do
		a[i] = i * 2
	end
	return a
end

local a: {number} = fill(5)

local total: number = 0
local k: number = 3
if k >= 1 and k <= #a then
	local v: number = a[k]
	total = v
end

return total
