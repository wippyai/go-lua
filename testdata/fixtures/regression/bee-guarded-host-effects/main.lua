-- Bee runtime-pin checker regression: bee.launch:hosts:63
-- Expected: A guarded Host retains its required leases map before hold executes.
-- Actual: argument 1: expected Host, got {leases?: {[string]: string}, ...} (Bee argument 2).
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type Host = {phase: string, leases: {[string]: string}}
type State = {hosts: {[string]: Host}}
local function hold(host: Host, lease: string, holder: string)
 host.leases[lease] = holder
end
local function acquire(state: State, id: string, lease: string, holder: string)
 local host = state.hosts[id]
 if host then
  if host.phase == "stopping" then return end
  hold(host, lease, holder)
 end
end
return acquire
