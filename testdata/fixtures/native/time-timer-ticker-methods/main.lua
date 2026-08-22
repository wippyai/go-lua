-- time.Ticker and time.Timer are only reachable through their fallible
-- constructors, so each is built, its nil arm is read, and every declared
-- method of the constructed object is then called. A tick is delivered as the
-- instant it fired, so both delivery channels carry time.Time.
local time = require("time")

local function drive_ticker(interval: number): number
    local repeating, err = time.ticker(interval)
    if err ~= nil then
        return 0
    end

    local delivered = repeating:channel()
    local answered = repeating:response()
    local tick, tick_ok = delivered:receive()
    local echo, echo_ok = answered:receive()
    if not tick_ok or not echo_ok or not repeating:stop() then
        return 0
    end
    return tick:unix() + echo:unix()
end

local function drive_timer(delay: string): number
    local pending, err = time.timer(delay)
    if pending == nil then
        return #tostring(err)
    end

    local delivered = pending:channel()
    local answered = pending:response()
    if not pending:reset(time.SECOND) then
        return 0
    end
    local fired, fired_ok = delivered:receive()
    local echo, echo_ok = answered:receive()
    if not fired_ok or not echo_ok or not pending:stop() then
        return 0
    end
    return fired:unix() + echo:unix()
end

return {drive_ticker = drive_ticker, drive_timer = drive_timer}
