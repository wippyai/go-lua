local protocol = require("protocol")

local M = {}

function M.accept(request: protocol.Request): protocol.Accepted
    return {
        id = request.id,
        attempt = request.retries + 1,
        source = request.tags["source"],
    }
end

function M.reject(request: protocol.Request, reason: string): protocol.Rejected
    return {
        id = request.id,
        reason = reason,
    }
end

function M.decide(request: protocol.Request): protocol.Decision
    if request.retries > 3 then
        return M.reject(request, "retry_limit")
    end
    return M.accept(request)
end

return M
