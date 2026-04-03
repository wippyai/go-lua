local logger = require("logger")
local cache = require("cache")

type Services = {
    logger: Logger,
    cache: Cache,
}

local M = {}

local _services: Services? = nil

function M.init(log_level: LogLevel?): Services
    local s: Services = {
        logger = logger.new(log_level),
        cache = cache.new(),
    }
    _services = s
    return s
end

function M.get(): Services
    if not _services then
        return M.init()
    end
    return _services
end

function M.logger(): Logger
    return M.get().logger
end

function M.cache(): Cache
    return M.get().cache
end

return M
