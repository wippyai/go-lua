-- A pointer-bearing record entry carries an ownership mode and a write barrier;
-- the edge is bound to the producing call, never to lexical position.

type Node = { id: string }

local function make_node(id: string): Node
    return { id = id }
end

local function wrap(id: string): { node: Node }
    return { node = make_node(id) }
end

return wrap
