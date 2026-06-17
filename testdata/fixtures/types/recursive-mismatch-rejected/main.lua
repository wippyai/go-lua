type Tree = { root: TreeNode? }
type TreeNode = { label: string, owner: Tree, children: {TreeNode}, parent: TreeNode? }

local tree: Tree = {root = nil}
-- label must be string; number is a real error that must NOT be masked by recursion handling
local node: TreeNode = {label = 123, owner = tree, children = {}, parent = nil}
return node
