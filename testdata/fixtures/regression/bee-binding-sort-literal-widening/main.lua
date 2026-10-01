-- Bee runtime-pin checker regression: bee.harness.catalog:catalog:105
-- Expected: Appending a Binding preserves its literal state union for the typed sort comparator.
-- Actual: argument 2: expected comparator over state:string, got comparator over Binding.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type Binding = {binding_id: string, state: "compatible" | "incompatible"}
type Snapshot = {bindings: {Binding}}
local function read(binding: Binding): Snapshot
 local snapshot: Snapshot = {bindings = {}}
 snapshot.bindings[#snapshot.bindings + 1] = binding
 table.sort(snapshot.bindings, function(left: Binding, right: Binding): boolean return left.binding_id < right.binding_id end)
 return snapshot
end
return read
