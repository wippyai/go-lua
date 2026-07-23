type Tree = { root: TreeNode? }
type TreeNode = { label: string, owner: Tree, children: {TreeNode}, parent: TreeNode? }

local function make(label: string, owner: Tree): TreeNode
    return { label = label, owner = owner, children = {}, parent = nil }
end

local function first_label(t: Tree): string?
    local r = t.root
    if r == nil then return nil end
    if r.parent ~= nil then
        return r.parent.label
    end
    return r.label
end

local tree: Tree = { root = nil }
tree.root = make("a", tree)
return first_label(tree)
