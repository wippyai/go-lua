local tree = require("tree")

local function count(node: tree.TreeNode): number
    local n = 1
    for _, child in ipairs(node.children) do
        n = n + count(child)
    end
    return n
end

local root = tree.leaf("root")
return count(root)
