local provider = require("provider")

local decoded = provider.decode({id = "cfg", retries = 2})
if decoded.ok then
    local id: string = decoded.value.id
    local retries: number = decoded.value.retries + 1
end

return "ok"
