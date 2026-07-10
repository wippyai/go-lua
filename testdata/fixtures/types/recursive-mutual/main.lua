type Tree = { root: TreeNode? }
type TreeNode = {
    label: string,
    owner: Tree,
    children: {TreeNode},
    parent: TreeNode?,
}

local function depth_of(node: TreeNode?): number
    if node == nil then
        return 0
    end
    local best = 0
    for _, child in ipairs(node.children) do
        local d = depth_of(child)
        if d > best then
            best = d
        end
    end
    return best + 1
end

local tree: Tree = {root = nil}
local node: TreeNode = {label = "root", owner = tree, children = {}, parent = nil}
tree.root = node
return depth_of(tree.root)
