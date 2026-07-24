-- Contract: the right-hand point of a short-circuit or publishes an evaluation
-- node carrying the length operation it performs, not a noop.

type Counted = { n: number?, items: {number} }

local c: Counted = { items = {1, 2, 3} }
local n = c.n or #c.items

return n
