-- Bee runtime-pin checker regression: bee.hive.supervisor:owner_stop_test:11
-- Expected vs actual: The returned record satisfies Call, including its unknown-valued input map; actual record-to-Call return rejected after typed call-site specialization.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type OwnerRef = {node_id: string, service_id: string, resource_ref: string?}
type Call = {protocol_revision: string, request_id: string, idempotency_key: string, deadline: string?, owner_ref: OwnerRef, target: {operation_ref: string?, interface_ref: string?}, input: {[string]: unknown}}
local types = {REVISION = "bee.hive@1"}
local owner_stop = {SERVICE = "bee.hive.owner", STOP = "bee.hive.owner:stop"}
function owner_stop.decode(call: Call): boolean return true end
local test = {}
function test.it(name: string, run: () -> ()) run() end
local function call(input: {[string]: unknown}, operation: string?, owner: OwnerRef?): Call
    return {protocol_revision = types.REVISION, request_id = "request-1", idempotency_key = "key-1", owner_ref = owner or {node_id = "owner-node", service_id = owner_stop.SERVICE},
        target = {operation_ref = operation or owner_stop.STOP}, input = input}
end
local function run(): ()
    test.it("decode", function()
        owner_stop.decode(call({alone = true}))
        owner_stop.decode(call({alone = false, force = true}))
    end)
end
return run
