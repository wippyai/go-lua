package regression

import "testing"

const unknownInitializerPrelude = `
type Vocabulary = {revision: integer}
local function decode(raw: unknown): Vocabulary?
    if type(raw) ~= "table" then return nil end
    return {revision = 1}
end
local function use(v: Vocabulary): integer return v.revision end
`

// A local initialized from an unknown value has the converged type unknown,
// so an expression over it gets a final type the flow can narrow.
func TestUnknownInitializedLocalGuardedDecode(t *testing.T) {
	checkBothModes(t, unknownInitializerPrelude+`
local function run(entry: unknown): integer
    local raw = entry
    local decoded = raw and decode(raw) or nil
    if not decoded then return -1 end
    local vocabulary: Vocabulary? = decoded
    return use(vocabulary)
end
return run
`, "")
}

func TestLazyOptionalCatalogInitializedInLoop(t *testing.T) {
	checkBothModes(t, unknownInitializerPrelude+`
local function run(entries: {unknown}): integer
    local vocabulary: Vocabulary? = nil
    local result = 0
    for _, entry in ipairs(entries) do
        if entry == nil then goto continue end
        if not vocabulary then
            local raw = entry
            local decoded = raw and decode(raw) or nil
            if not decoded then return -1 end
            vocabulary = decoded
        end
        result = use(vocabulary)
        ::continue::
    end
    return result
end
return run
`, "")
}

func TestUnknownInitializedLocalUnguardedDecodeRejected(t *testing.T) {
	checkBothModes(t, unknownInitializerPrelude+`
local function run(entry: unknown): integer
    local raw = entry
    local decoded = raw and decode(raw)
    return use(decoded)
end
return run
`, "argument 1: expected Vocabulary, got")
}

func TestLazyOptionalCatalogMissingGuardRejected(t *testing.T) {
	checkBothModes(t, unknownInitializerPrelude+`
local function run(entries: {unknown}): integer
    local vocabulary: Vocabulary? = nil
    local result = 0
    for _, entry in ipairs(entries) do
        if not vocabulary then
            local raw = entry
            vocabulary = raw and decode(raw) or nil
        end
        result = use(vocabulary)
    end
    return result
end
return run
`, "argument 1: expected Vocabulary, got Vocabulary?")
}
