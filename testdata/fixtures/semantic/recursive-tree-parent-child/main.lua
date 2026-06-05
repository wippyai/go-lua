type Tree = {
    id: string,
    parent: Tree?,
    children: {Tree},
}

local root: Tree = {
    id = "root",
    parent = nil,
    children = {},
}

local child: Tree = {
    id = "child",
    parent = root,
    children = {},
}

table.insert(root.children, child)

local first = root.children[1]
if first then
    local child_id: string = first.id
    if first.parent then
        local parent_id: string = first.parent.id
    end
end

local missing: Tree = root.children[2] -- expect-error

return "ok"
