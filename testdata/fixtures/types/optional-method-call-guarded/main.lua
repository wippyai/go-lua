type Logger = { log: (self: Logger, msg: string) -> () }
local function maybe_log(l: Logger?, msg: string)
    if l ~= nil then
        l:log(msg)
    end
end
maybe_log(nil, "x")
