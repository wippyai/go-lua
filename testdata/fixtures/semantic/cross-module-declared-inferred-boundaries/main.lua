local decoder = require("decoder")
local decision = require("decision")
local protocol = require("protocol")

local raw: any = {
    id = "job-1",
    retries = 1,
    tags = {source = "api"},
}

local request_from_raw: protocol.Request = raw -- expect-error

local decoded = decoder.decode(raw)
if decoded.ok then
    local request: protocol.Request = decoded.value
    local id: string = decoded.value.id
    local bad_id: number = decoded.value.id -- expect-error

    local accepted = decision.accept(request)
    local attempt: number = accepted.attempt
    local bad_attempt: string = accepted.attempt -- expect-error

    local accepted_again = decision.accept(request)
    local accepted_attempt: number = accepted_again.attempt
    local accepted_attempt_text: string = accepted_again.attempt -- expect-error

    local outcome = decision.decide(request)
    if outcome.reason then
        local reason: string = outcome.reason
        local attempt_from_rejected: number = outcome.attempt -- expect-error
        print(reason)
    else
        local accepted_id: string = outcome.id
        local inferred_attempt: number = outcome.attempt -- expect-error
        local reason_from_accepted: string = outcome.reason -- expect-error
        print(accepted_id .. ":" .. tostring(accepted_attempt))
    end

    local wrong_decision: protocol.Decision = { -- expect-error
        id = id,
        attempt = "two",
    }
end

local failed = decoder.decode({id = "job-2", retries = "bad"})
if not failed.ok then
    local message: string = failed.error
    print(message)
end
