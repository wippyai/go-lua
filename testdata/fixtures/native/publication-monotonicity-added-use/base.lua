local M = {}

function M.scale(n: number): number
    return n * 2
end

M.first = M.scale(1)

return M
