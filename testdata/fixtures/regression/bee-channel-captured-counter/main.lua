-- Bee runtime-pin checker regression: bee.harness.window:picker:93
-- Expected vs actual: Integer counter and valid sends preserve Channel<Activation>; actual serial becomes integer | unresolved at the captured return.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
-- Requires only the standard channel manifest supplied by the driver, no Bee imports.
local channel = require("channel")
type Channel = channel.Channel
type Activation = {serial: integer, error: string?, title: string?}
local function run(): Channel<Activation>?
    local activations = channel.new(1) :: Channel<Activation>
    local serial = 0
    local function finish(): Channel<Activation>?
        serial = serial + 1
        return activations
    end
    activations:send({serial = serial, error = "setup failed"})
    activations:send({serial = serial, title = "app"})
    return finish()
end
return run
