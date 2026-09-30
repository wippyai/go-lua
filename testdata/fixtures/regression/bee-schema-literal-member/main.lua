-- Bee runtime-pin checker regression: bee.gov:migration_work:106/107/388
-- Expected vs actual: Unmodified module schema literals retain singleton types; actual string-to-Schema? assignments and string record field-to-Payload argument.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
local M = {}
M.SCHEMA = "work@3"
M.LEGACY_SCHEMA = "work@2"
type Schema = "work@3" | "work@2"
type Payload = {schema_revision: Schema}
local function seal(value: Payload): Payload return value end
function M.decode(value: unknown): Schema?
    if type(value) ~= "table" then return nil end
    local schema: Schema? = nil
    if value.schema_revision == M.SCHEMA then schema = M.SCHEMA
    elseif value.schema_revision == M.LEGACY_SCHEMA then schema = M.LEGACY_SCHEMA end
    return schema
end
function M.build(): Payload return seal({schema_revision = M.SCHEMA}) end
return M
