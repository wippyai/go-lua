-- The table is sent to another process before the fill, so the growth site
-- cannot own the backing store and growth authority is withheld.
local function fill(n: number): ()
    local t: {number} = {}
    process.send("worker", "batch", t)
    for i = 1, n do
        t[i] = i
    end
end

return fill
