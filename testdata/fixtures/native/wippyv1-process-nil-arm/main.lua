-- Contract: the Wippy v1 process surface declares four results that carry a
-- nil the caller must handle - Event.payload, which answers an optional value,
-- and the result, error and reason fields of an Event, which the record
-- declares as optional because which of them an event carries depends on the
-- kind that raised it. Each is read here twice: once unguarded, where the nil
-- reaches a consumer that cannot take it, and once behind the test that rules
-- it out. The guarded read is what the declaration exists to require, so it
-- must be silent.

local process = require("process")

local function consume(value: {[string]: any}): number
    local total: number = 0
    for _ in pairs(value) do
        total = total + 1
    end
    return total
end

local events = process.events()
local event, event_ok = events:receive()
if not event_ok then
    return "no event"
end

local kind: string = event.kind
local from: string = event.from

-- An event that did not complete a call carries no result.
local raw_result: number = consume(event.result) -- expect-error
local result_count: number = 0
if event.result ~= nil then
    result_count = consume(event.result)
end

-- Only a failing event carries an error.
local raw_error: number = consume(event.error) -- expect-error
local error_count: number = 0
if event.error ~= nil then
    error_count = consume(event.error)
end

-- Only an exit or a cancel event carries a reason.
local raw_reason: string = event.reason -- expect-error: may be nil
local reason_text: string = ""
if event.reason ~= nil then
    reason_text = event.reason
end

-- Event.payload answers an optional value, so the nil is part of its result.
local raw_payload: number = consume(event:payload()) -- expect-error
local payload_count: number = 0
local payload = event:payload()
if payload ~= nil then
    payload_count = consume(payload)
end

return kind .. from .. reason_text .. raw_reason ..
    tostring(result_count + error_count + payload_count) ..
    tostring(raw_result + raw_error + raw_payload)
