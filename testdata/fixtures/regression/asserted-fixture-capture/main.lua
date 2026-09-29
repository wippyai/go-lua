-- From app:accept_test (the wire fixture records calls in captured `seen`).
local test = { eq = function(_actual: any, _expected: any) end }

local function wire()
    local seen = {}
    local function accept()
        seen.submit = {
            id = "spiralscout.leads.binding:submit_func",
            payload = { lead = { person_ref = "kjell@impactdata.example" } },
        }
        return { thread_id = "t-1" }
    end
    return seen, accept
end

local seen, accept = wire()
local result = assert(accept())
test.eq(seen.submit.id, "spiralscout.leads.binding:submit_func")
local lead = seen.submit.payload.lead
test.eq(lead.person_ref, "kjell@impactdata.example")
test.eq(result.thread_id, "t-1")

-- A declared optional field still needs a presence check.
local declared: { submit: { id: string }? } = {}
local declared_id: string = declared.submit.id -- expect-error: cannot assign string? to string
