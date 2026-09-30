-- Bee runtime-pin checker regression: bee.gov:activation_profile_decoder:619
-- Expected vs actual: Guarded initialization yields Vocabulary; actual argument expected Vocabulary, got Vocabulary?.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type Vocabulary = {revision: integer}
local M = {}
function M.decode(raw: unknown): Vocabulary?
    if type(raw) ~= "table" or type(raw.revision) ~= "number" then return nil end
    return {revision = math.floor(raw.revision)}
end
local function use(v: Vocabulary): integer return v.revision end
local function run(entries: {unknown}): integer
    local vocabulary: Vocabulary? = nil
    local result = 0
    for _, entry in ipairs(entries) do
        if entry == nil then goto continue end
        if not vocabulary then
            local raw = entry
            local decoded = raw and M.decode(raw) or nil
            if not decoded then return -1 end
            vocabulary = decoded
        end
        result = use(vocabulary)
        ::continue::
    end
    return result
end
return run
