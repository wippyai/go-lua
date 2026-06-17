type ListNode = { value: number, next: ListNode? }

local function sum_list(node: ListNode?): number
    if node == nil then
        return 0
    end
    return node.value + sum_list(node.next)
end

local head: ListNode = {value = 1, next = nil}
return sum_list(head)
