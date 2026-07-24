-- The coinductive fixpoint of a self-linked record: the shape identity of
-- node.next equals the identity of node, so one traversal cache serves the loop.

type Node = { value: number, next: Node? }

local function total(head: Node?): number
    local sum = 0
    local cur = head
    while cur ~= nil do
        sum = sum + cur.value
        cur = cur.next
    end
    return sum
end

return total({ value = 1, next = { value = 2, next = nil } })
