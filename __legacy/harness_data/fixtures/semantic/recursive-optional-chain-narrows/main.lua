type Link = {
    id: string,
    next: Link?,
}

local tail: Link = {id = "tail", next = nil}
local mid: Link = {id = "mid", next = tail}
local head: Link = {id = "head", next = mid}

local first = head.next
if first then
    local first_id: string = first.id
    local second = first.next
    if second then
        local second_id: string = second.id
    end
end

local missing: Link = tail.next -- expect-error

return "ok"
