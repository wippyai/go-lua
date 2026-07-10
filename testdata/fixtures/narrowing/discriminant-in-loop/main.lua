type Cmd = { kind: "inc", by: number } | { kind: "reset" }
local function run(cmds: {Cmd}): number
    local total = 0
    for _, c in ipairs(cmds) do
        if c.kind == "inc" then
            total = total + c.by
        else
            total = 0
        end
    end
    return total
end
return run({ { kind = "inc", by = 3 }, { kind = "reset" } })
