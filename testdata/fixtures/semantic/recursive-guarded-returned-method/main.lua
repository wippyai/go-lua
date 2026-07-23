type Node = {
    name: string,
    child: Node?,
    label: (self: Node) -> string,
}

local function make_node(name: string): Node
    return {
        name = name,
        child = nil,
        label = function(self: Node): string
            return self.name
        end,
    }
end

local root = make_node("root")

if root.child then
    local name: string = root.child:label()
end

return "ok"
