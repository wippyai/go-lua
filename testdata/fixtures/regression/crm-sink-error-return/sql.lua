local M = {}
function M.get(_id: string): (any?, string?)
    return { query = function() return {}, nil end }, nil
end
return M
