-- Accumulator bounded by the iteration count.
--
-- `count` is incremented at most once per element of `xs`, so it is bounded by
-- [0, #xs]. Under widening `count` smears to [0,+inf); the narrowing pass must
-- recover the upper bound `count <= #xs`.
--
-- Downstream consumer: the guarded read `xs[count]` needs the upper bound
-- `count <= #xs` (together with the `count >= 1` guard) to be proven in-bounds
-- and typed non-optional `number`. The `local v: number = xs[count]` annotation
-- is the consumption point of the recovered upper bound.

local function pred(v: number): boolean
	return v > 0
end

local function count_pos(xs: {number}): number
	local count: number = 0
	for _, v in ipairs(xs) do
		if pred(v) then
			count = count + 1
		end
	end

	local last: number = 0
	if count >= 1 and count <= #xs then
		local v: number = xs[count]
		last = v
	end

	return last
end

return count_pos({ 1, -2, 3, 4 })
