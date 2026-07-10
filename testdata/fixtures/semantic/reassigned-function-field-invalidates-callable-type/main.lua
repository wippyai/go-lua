local M = {}

function M.f(): string
    return "ok"
end

M.f = 42 -- expect-error
local g: () -> string = M.f

return "ok"
