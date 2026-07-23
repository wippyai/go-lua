type Node = {
    name: string,
    child: Node?,
    set_child: (self: Node, child: Node) -> Node,
    label: (self: Node) -> string,
}

local function make_node(name: string): Node
    local node: Node = {
        name = name,
        child = nil,
        set_child = function(self: Node, child: Node): Node
            self.child = child
            return self
        end,
        label = function(self: Node): string
            return self.name
        end,
    }
    return node
end

local root = make_node("root")
local child = make_node("child")
root:set_child(child)

if root.child then
    local name: string = root.child:label()
end

local direct: Node = root.child -- expect-error: cannot assign root.child because it may be nil

return "ok"
