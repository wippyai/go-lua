type Boxed = {
    tag: string,
    body: string,
}

local M = {}
M.Boxed = Boxed

function M.wrap(payload: string): M.Boxed
    local box: M.Boxed = { tag = "boxed", body = payload }
    return box
end

return M
