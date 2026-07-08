local M = {}

function M.make(ok: boolean)
    if ok then
        return { kind = "ok", value = 42 }
    end
    return { kind = "err", message = "boom" }
end

return M
