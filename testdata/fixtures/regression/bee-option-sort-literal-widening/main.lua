-- Bee runtime-pin checker regression: bee.harness.window:profile_editor:289
-- Expected: Appending an enum row to OptionRow[] preserves its literal kind union.
-- Actual: argument 2: expected comparator over kind:string, got comparator over OptionRow.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.2 fails (1 diagnostic).
-- Check using pinlint2-driver/v1524/checker or pinlint2-driver/v162/checker.
type OptionRow = {name: string, kind: "enum" | "text"}
local function options(names: {string}): {OptionRow}
 local rows: {OptionRow} = {}
 for _, name in ipairs(names) do
  rows[#rows + 1] = {name = name, kind = "enum"}
 end
 table.sort(rows, function(left: OptionRow, right: OptionRow): boolean return left.name < right.name end)
 return rows
end
return options
