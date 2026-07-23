type User = { name: string, nick: string? }
local function display(u: User): string
    local shown: string = u.nick or u.name
    return shown
end
return display({ name = "alice", nick = nil })
