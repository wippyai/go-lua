-- A store inside a loop is a repeatable mutation site, not a one-shot
-- lifecycle row: the growth row carries the occurrence mode, the array-vs-hash
-- predecessor retirement arm, the heap-exhaustion rollback disposition and a
-- complete throw inventory.
local function fill(n: number): ()
    local t: {number} = {}
    for i = 1, n do
        t[i] = i * 3
    end
end

return fill
