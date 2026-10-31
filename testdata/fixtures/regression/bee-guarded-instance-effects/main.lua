-- Bee runtime-pin checker regression: bee.apps:broker:1537
-- Expected: A guarded Instance remains complete when stop appends a typed waiter.
-- Actual: argument 1: expected Instance, got {waiters?: Waiter[], ...}.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type Waiter = {recipient: string}
type Instance = {pid: string, waiters: {Waiter}}
local function stop(item: Instance, waiter: Waiter)
 item.waiters[#item.waiters + 1] = waiter
end
local function control(find: (string) -> Instance?, pid: string, waiter: Waiter)
 local item = find(pid)
 if not item then return end
 stop(item, waiter)
end
return control
