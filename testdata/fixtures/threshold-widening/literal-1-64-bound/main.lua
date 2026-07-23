-- Literal threshold widening.
--
-- The loop guard exposes the numeric threshold 64. Threshold widening should
-- keep the body counter `i` in [1,64] instead of widening directly to [1,+inf),
-- allowing the 64-element array read to be proven non-optional.

local values: {number} = {
	1, 2, 3, 4, 5, 6, 7, 8,
	9, 10, 11, 12, 13, 14, 15, 16,
	17, 18, 19, 20, 21, 22, 23, 24,
	25, 26, 27, 28, 29, 30, 31, 32,
	33, 34, 35, 36, 37, 38, 39, 40,
	41, 42, 43, 44, 45, 46, 47, 48,
	49, 50, 51, 52, 53, 54, 55, 56,
	57, 58, 59, 60, 61, 62, 63, 64,
}

local total: number = 0
local i: number = 1
while i <= 64 do
	local v: number = values[i]
	total = total + v
	i = i + 1
end

return total
