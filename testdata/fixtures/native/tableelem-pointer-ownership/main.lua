-- Elements are freshly constructed records, so the element row carries the
-- per-element ownership mode and the write-barrier obligation.
type Node = { id: number }

local nodes: {Node} = {}
for i = 1, 4 do
    nodes[i] = { id = i }
end

return nodes
