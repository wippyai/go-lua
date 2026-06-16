local protocol = require("protocol")

type Registry = {
    primary: protocol.Handler?,
    backup: protocol.Handler?,
}

local registry: Registry = {}

local imported = protocol.accept
registry.primary = imported
registry.backup = protocol.reject

local first = registry.primary
if first then
    local result = first({id = "r1", retries = 2})
    if result.ok then
        local label: string = result.label
        local bad_label: number = result.label -- expect-error
        print(label)
    else
        local reason: string = result.reason
        local bad_reason: number = result.reason -- expect-error
        print(reason)
    end
end

registry.primary = function(req: protocol.Request): protocol.Response
    return {
        ok = true,
        label = tostring(req.retries),
    }
end

local second = registry.primary
if second then
    local result = second({id = "r2", retries = 3})
    if result.ok then
        local label: string = result.label
        local bad_label: number = result.label -- expect-error
        print(label)
    end

    local wrong_arg = second({id = 42, retries = "bad"}) -- expect-error
    print(wrong_arg)
end

local bad_handler: protocol.Handler = function(req: {id: number, retries: number}): protocol.Response -- expect-error
    return {
        ok = true,
        label = tostring(req.id),
    }
end

print(bad_handler)
