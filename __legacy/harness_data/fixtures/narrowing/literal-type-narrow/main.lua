type Status = "active" | "paused" | "stopped"
local function tick(s: Status): number
    if s == "active" then return 1 end
    if s == "paused" then return 0 end
    return -1
end
return tick("active")
