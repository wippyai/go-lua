type Node = {
    value: number,
    next: Node?,
}

local function sum(node: Node?, limit: number): number
    local total = 0
    local current = node
    local remaining = limit
    while current and remaining > 0 do
        total = total + current.value
        current = current.next
        remaining = remaining - 1
    end
    return total
end

local third: Node = {value = 3, next = nil}
local second: Node = {value = 2, next = third}
local first: Node = {value = 1, next = second}

local total: number = sum(first, 4)
local bad: string = sum(first, 2) -- expect-error

return "ok"
