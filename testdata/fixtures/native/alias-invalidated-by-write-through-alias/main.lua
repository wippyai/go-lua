-- A store through a proven alias revokes the load-forwarding fact taken on the original
-- binding: the two names denote one allocation, so write.field through either revokes it.

local function run(): number
    local box = { n = 1 }
    local alias = box
    local first = box.n
    alias.n = 9
    return first + box.n
end

return run
