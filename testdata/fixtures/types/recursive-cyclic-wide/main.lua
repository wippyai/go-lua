type ListNode = { value: number, next: ListNode? }
type Tree = { root: TreeNode? }
type TreeNode = { label: string, owner: Tree, children: {TreeNode}, parent: TreeNode? }
type DeepOptional = {a: {b: {c: {d: {e: number?}?}?}?}?}
type Shape =
    "circle" | "square" | "triangle" | "hexagon" | "pentagon" |
    "octagon" | "ellipse" | "rhombus" | "trapezoid" | "star"

local function sum_list(node: ListNode?): number
    if node == nil then return 0 end
    return node.value + sum_list(node.next)
end

local function depth_of(node: TreeNode?): number
    if node == nil then return 0 end
    local best = 0
    for _, child in ipairs(node.children) do
        local d = depth_of(child)
        if d > best then best = d end
    end
    return best + 1
end

local head: ListNode = {value = 1, next = {value = 2, next = nil}}
local tree: Tree = {root = nil}
local node: TreeNode = {label = "root", owner = tree, children = {}, parent = nil}
tree.root = node
local nested: DeepOptional = {a = {b = {c = {d = {e = 42}}}}}
local kind: Shape = "hexagon"
return sum_list(head) + depth_of(tree.root)
