-- Reduced from app:journal_span_identity_test in kickside/chestor/journal/test.
local test = {}
function test.is_true(value: any, message: string?)
    if not value then error(message or "assertion failed") end
end
local function query(): { [number]: { seq: number } }
    return { { seq = 4 } }
end
local stored = query()
test.is_true(#stored >= 1, "protocol events stay in the durable index")
local seq: number = stored[#stored].seq

-- Without the assertion, a declared map element remains optional.
local other = query()
local unchecked: number = other[#other].seq -- expect-error

-- Removing the tail invalidates the earlier length proof.
stored[#stored] = nil
local removed: number = stored[#stored].seq -- expect-error
