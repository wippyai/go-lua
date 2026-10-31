-- From app:trigger_mode_poll_test: with_alert_seam invokes fn with captures,
-- while several call sites deliberately ignore that argument.
type Map = { [string]: any }
type AlertCaptures = {
    raised: { Map },
    cleared: { Map },
}

local function with_alert_seam(fn: (captures: AlertCaptures) -> ())
    local raised: { Map } = {}
    local cleared: { Map } = {}
    local ok, err = pcall(function() fn({ raised = raised, cleared = cleared }) end)
    if not ok then error(err) end
end

with_alert_seam(function()
    local paced: Map = { code = "rate_limited", message = "slow down", retriable = true, scope = "connection", retry_after_ms = 45000 }
    assert(paced.retry_after_ms == 45000)
end)

with_alert_seam(function(captures: AlertCaptures)
    assert(#captures.raised == 0)
end)

with_alert_seam(function(first: AlertCaptures, second: string) -- expect-error
    assert(first ~= nil and second ~= nil)
end)
