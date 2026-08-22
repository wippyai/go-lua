-- Every declared method of time.Time, time.Duration and time.Location is
-- called on a value the module itself produced, and each answer is read as the
-- type the declaration states: an instant answers an instant, the difference of
-- two instants answers a duration, and a reading off either answers a scalar.
local time = require("time")

local function instants(): boolean
    local now = time.now()
    local built = time.date(2026, 8, 22, 13, 45, 30, 0)
    local epoch = time.unix(0, 0)

    local later = now:add(time.HOUR)
    local anniversary = now:add_date(1, 0, 0)
    local elapsed = later:sub(now)

    local ordered = now:before(later) and later:after(now) and now:equal(now)
    local zeroed = epoch:is_zero() and built:is_zero()

    local stamp = now:format(time.RFC3339) .. now:format_rfc3339()
    local seconds = now:unix() + now:unix_nano()
    local year, month, day = now:date()
    local hour, minute, second = now:clock()
    local fields = now:year() + now:month() + now:day() +
        now:hour() + now:minute() + now:second() +
        now:nanosecond() + now:weekday() + now:year_day()

    return ordered and zeroed and #stamp > 0 and
        seconds > 0 and year + month + day > 0 and
        hour + minute + second >= 0 and fields > 0 and
        anniversary:after(now) and elapsed:seconds() >= 0
end

local function durations(): number
    local start = time.now()
    local finish = start:add(time.MINUTE)
    local span = finish:sub(start)
    return span:nanoseconds() + span:microseconds() + span:milliseconds() +
        span:seconds() + span:minutes() + span:hours()
end

local function zones(name: string): string
    local fallback = time.fixed_zone(name, 0)
    local zone = fallback
    local loaded = time.load_location(name)
    if loaded ~= nil then
        zone = loaded
    end

    local now = time.now()
    local moved = now:in_location(zone)
    local resolved = moved:location()
    local universal = now:utc()
    local here = now:in_local()

    local unit = universal:sub(here)
    local rounded = now:round(unit)
    local truncated = now:truncate(unit)

    return resolved:string() .. "/" .. fallback:string() ..
        "/" .. tostring(rounded:unix()) .. "/" .. tostring(truncated:unix())
end

time.sleep(time.MILLISECOND)

return {instants = instants, durations = durations, zones = zones}
