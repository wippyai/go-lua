-- Numeric for-loop counter recovery.
--
-- The counter `i` ranges over [1,10] inside the body. Under widening alone the
-- solver smears `i` to [1,+inf); a bounded decreasing (narrowing) pass must
-- re-descend to the true loop bound [1,10].
--
-- Downstream consumer: `k` copies `i` and is documented as bounded [1,10]; the
-- annotated `local k: number = i` records the point where the recovered bound
-- is consumed.

local last: number = 0

for i = 1, 10 do
	local k: number = i
	last = k
end

return last
