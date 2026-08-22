-- Every fallible time member answers two normal arms: the value with no error,
-- or nil with the module's error. Each call below reads both arms, so the nil
-- the module actually answers is observed on one path and the value is used on
-- the other.
local time = require("time")

local function parse_instant(text: string): string
    local parsed, err = time.parse(time.RFC3339, text)
    if err ~= nil then
        return "unparsed"
    end
    return parsed:format_rfc3339()
end

local function parse_instant_in_zone(text: string, zone: time.Location): string
    local parsed, err = time.parse(time.RFC3339, text, zone)
    if parsed == nil then
        return tostring(err)
    end
    return parsed:format(time.DATE_ONLY)
end

local function span_seconds(text: string): number
    local span, err = time.parse_duration(text)
    if err ~= nil then
        return -1
    end
    return span:seconds()
end

local function zone_name(name: string): string
    local zone, err = time.load_location(name)
    if zone == nil then
        return tostring(err)
    end
    return zone:string()
end

local function timer_started(delay: string): boolean
    local pending, err = time.timer(delay)
    if err ~= nil then
        return false
    end
    return pending:stop()
end

local function ticker_started(interval: number): boolean
    local repeating, err = time.ticker(interval)
    if repeating == nil then
        return false
    end
    return repeating:stop()
end

return {
    parse_instant = parse_instant,
    parse_instant_in_zone = parse_instant_in_zone,
    span_seconds = span_seconds,
    zone_name = zone_name,
    timer_started = timer_started,
    ticker_started = ticker_started,
}
