-- #t is admitted only from a proved dense prefix with __len proved absent.
-- The published row authorizes the VM border algorithm; it never predicts a
-- runtime length.
local function fill(n: number): number
    local t: {number} = {}
    for i = 1, n do
        t[i] = i
    end
    return #t
end

return fill(4)
