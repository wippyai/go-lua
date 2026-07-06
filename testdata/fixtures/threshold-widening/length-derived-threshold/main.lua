-- Length-derived threshold widening.
--
-- The guard threshold is `#thresholds`, whose constructor size is 8. Threshold
-- widening should keep `i` in [1,#thresholds] inside the loop body.

local thresholds: {number} = { 3, 5, 8, 13, 21, 34, 55, 89 }

local total: number = 0
local i: number = 1
while i <= #thresholds do
	local v: number = thresholds[i]
	total = total + v
	i = i + 1
end

return total
