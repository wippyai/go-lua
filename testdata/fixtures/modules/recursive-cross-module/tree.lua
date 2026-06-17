type TreeNode = { label: string, children: {TreeNode}, parent: TreeNode? }
local M = {}
M.TreeNode = TreeNode
function M.leaf(label: string): TreeNode
    return { label = label, children = {}, parent = nil }
end
return M
