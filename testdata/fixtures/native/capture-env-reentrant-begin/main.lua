-- The closure is constructed inside a loop, so several concrete environments
-- exist and no single epoch root can be proved.

local function counters(): {() -> number}
    local fns: {() -> number} = {}
    for i = 1, 4 do
        local n: number = i
        fns[i] = function(): number
            n = n + 1
            return n
        end
    end
    return fns
end

return counters()
