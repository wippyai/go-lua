-- From spiralscout.estimation:rank in estimation/test/hub.
local M = {}

function M.between(lo: string?, hi: string?): string
    local a = type(lo) == "string" and lo :: string or ""
    local b = type(hi) == "string" and hi :: string or ""
    local result = ""
    local i = 1
    local upper_open = (b == "")
    while true do
        if upper_open then
            result = result .. a:sub(i, i)
            i = i + 1
        elseif i <= #b then
            return result .. b:sub(i, i)
        else
            upper_open = true
        end
    end
end

local function can_break(stop: boolean): string -- expect-error: missing return
    while true do
        if stop then break end
    end
end

return M
