-- The same freshly built child reaches two fields: both edges are closed
-- explicitly and the source evaluation order of the construction is preserved.

type Leaf = { id: string }

local function build(id: string): { primary: Leaf, mirror: Leaf }
    local child: Leaf = { id = id }
    return { primary = child, mirror = child }
end

return build
