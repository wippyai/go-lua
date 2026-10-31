-- Bee runtime-pin checker regression: bee.threads.service:authority:465
-- Expected: The decoder returns a Message with Outcome?, also when captured by a callback.
-- Actual: argument 1: expected Payload, got {body: {message_id: string, outcome?: string}, kind: "message"} (Bee argument 5).
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type Outcome = "succeeded" | "failed"
type Message = {message_id: string, outcome: Outcome?}
type Payload = {kind: "message", body: Message} | {kind: "other", body: string}
local function outcome(value: unknown): Outcome?
 if value == "succeeded" or value == "failed" then return value end
 return nil
end
local function decode(value: unknown): Message?
 local result: Message = {message_id = "id"}
 if value ~= nil then
  local decoded = outcome(value)
  if not decoded then return nil end
  result.outcome = decoded
 end
 return result
end
local function commit(payload: Payload) end
local function run(callback: () -> ()) callback() end
local function submit(value: unknown)
 local decoded = decode(value)
 if not decoded then return end
 run(function() commit({kind = "message", body = decoded}) end)
end
return submit
