local M = {}

function M.pick(which: "num" | "str")
    if which == "num" then
        return { kind = "num", value = 10 }
    end
    return { kind = "str", text = "remote" }
end

return M
