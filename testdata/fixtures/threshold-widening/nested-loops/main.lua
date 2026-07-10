-- Nested threshold widening.
--
-- The outer loop is bounded by #grid (4 rows). The inner loop is bounded by
-- #first (8 columns). Threshold widening should preserve both thresholds so the
-- row and cell reads are non-optional.

local grid: {{number}} = {
	{ 1, 2, 3, 4, 5, 6, 7, 8 },
	{ 9, 10, 11, 12, 13, 14, 15, 16 },
	{ 17, 18, 19, 20, 21, 22, 23, 24 },
	{ 25, 26, 27, 28, 29, 30, 31, 32 },
}

local first: {number} = grid[1]
local total: number = 0
local i: number = 1
while i <= #grid do
	local row: {number} = grid[i]
	local j: number = 1
	while j <= #first do
		local cell: number = row[j]
		total = total + cell
		j = j + 1
	end
	i = i + 1
end

return total
