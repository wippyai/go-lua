-- process.send answers one boolean: the transfer was delivered, or it was
-- rejected. Both arms are read here, so the rejection the declaration states is
-- observed rather than discarded at the call.
local process = require("process")

type Notice = {id: string, body: string}

local function publish(pid: string, notice: Notice): string
    local delivered = process.send(pid, "notice.published", notice)
    if not delivered then
        return "rejected:" .. notice.id
    end
    return "delivered:" .. notice.id
end

local function publish_all(pid: string, notices: Notice[]): number
    local rejected = 0
    for _, notice in ipairs(notices) do
        if not process.send(pid, "notice.published", notice) then
            rejected = rejected + 1
        end
    end
    return rejected
end

return {publish = publish, publish_all = publish_all}
