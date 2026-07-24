type Point = { x: number, y: number }

local M = {}

function M.origin(): Point
    return { x = 0, y = 0 }
end

return M
