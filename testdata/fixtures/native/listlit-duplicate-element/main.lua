-- The same freshly built child is referenced twice: both edges are closed
-- explicitly and the edge count is edge occurrences, not distinct children.
type Row = { id: number }

local shared: Row = { id = 1 }
local rows: {Row} = { shared, shared }

return rows
