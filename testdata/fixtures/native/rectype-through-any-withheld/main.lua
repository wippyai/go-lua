-- The recursive link passes through any: the successor's shape identity is not
-- derivable from the subject, so there is no fixpoint and no stable identity.

type LooseNode = { value: number, next: any }

local function total(start: LooseNode): number
    local sum = 0
    local cur: LooseNode = start
    while true do
        sum = sum + cur.value
        local nxt = cur.next
        if nxt == nil then break end
        cur = nxt :: LooseNode
    end
    return sum
end

return total({ value = 1, next = nil })
