local M = {}
function M.mutate(t: { [string]: { [string]: unknown } })
    t.other = { id = "without-view" }
end
return M
