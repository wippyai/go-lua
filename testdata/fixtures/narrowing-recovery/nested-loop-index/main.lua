-- Doubly-nested numeric loop indexing a 2D array.
--
-- The outer counter `i` ranges over [1,rows] and the inner counter `j` over
-- [1,cols]. Under widening both smear to [1,+inf); the narrowing pass must
-- re-bound `i` to [1,rows] and `j` to [1,cols] so the 2D access `grid[i][j]`
-- on a `{{number}}` is proven in-bounds at both levels.
--
-- Downstream consumer: `local cell: number = row[j]` requires both the outer
-- read `grid[i]` (bounded by i in [1,rows]) and the inner read `row[j]`
-- (bounded by j in [1,cols]) to be non-optional; that annotation is the
-- consumption point of the two recovered bounds.

local function sum2d(grid: {{number}}, rows: number, cols: number): number
	local total: number = 0
	for i = 1, rows do
		local row: {number} = grid[i]
		for j = 1, cols do
			local cell: number = row[j]
			total = total + cell
		end
	end
	return total
end

local grid: {{number}} = {
	{ 1, 2, 3 },
	{ 4, 5, 6 },
}

return sum2d(grid, 2, 3)
