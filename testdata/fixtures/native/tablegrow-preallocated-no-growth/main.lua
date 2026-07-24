-- A static capacity with a fill that stays inside it: the non-growing arm is
-- published distinctly from the growth arms.
local function fill(): ()
    local t: {number} = table.create(8, 0)
    for i = 1, 8 do
        t[i] = i
    end
end

return fill
