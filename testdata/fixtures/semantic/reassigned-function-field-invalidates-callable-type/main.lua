local M = {}

function M.f(): string
    return "ok"
end

M.f = 42
local g: () -> string = M.f -- expect-error

return "ok"
