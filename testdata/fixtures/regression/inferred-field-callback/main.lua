local test = require("test")
local accept = require("market_accept")

local CANDIDATE = {
    name = "Impact Data", domain = "impactdata.example", region = "US",
    employee_count_min = 20, employee_count_max = 80,
    complexity_signal = "They run four disconnected reporting stacks.",
    person_name = "Kjell Thorvaldsen", person_role = "CEO",
    person_url = "https://linkedin.example/kjell",
    contact_kind = "email", contact_value = "Kjell@ImpactData.example",
    origin_name = "HubSpot Solutions Directory", origin_url = "https://ecosystem.example/solutions",
}

local function wire(opts)
    local o = opts or {}
    local seen = {}
    accept._component = {
        ACCESS = { READ = "read" },
        validate_access = function(id, mode)
            seen.access = { id = id, mode = mode }
            if o.denied then return nil, "forbidden" end
            return {}, nil
        end,
    }
    accept._cases = { get = function(id)
        seen.case_id = id
        if o.missing then return nil, "not found" end
        return {
            purpose = { hypothesis = "US B2B agencies buying AI help" },
            result = { case_output = {
                companies = o.companies or { CANDIDATE },
                partial_companies = o.partials,
            } },
        }, nil
    end }
    accept._leads_available = function() return o.no_leads ~= true end
    accept._call = function(id, payload)
        seen.submit = { id = id, payload = payload }
        if o.submit_err then return nil, o.submit_err end
        return { ok = true, lead_id = "actor:kjell@impactdata.example", thread_id = "t-1", status = "observed" }, nil
    end
    return seen
end

local function reset()
    accept._component = nil; accept._cases = nil; accept._leads_available = nil; accept._call = nil
end

local function define_tests()
    test.describe("accept.accept", function()
        test.after_each(reset)

        test.it("reads the case under its own access check and submits one warm lead", function()
            local seen = wire({})
            local result = assert(accept.accept({ case_id = "case-1" }))
            test.eq(seen.access.id, "case-1")
            test.eq(seen.submit.id, "spiralscout.leads.binding:submit_func")
            local lead = seen.submit.payload.lead
            test.eq(lead.person_ref, "kjell@impactdata.example")
            test.eq(lead.email, "kjell@impactdata.example")
            test.eq(lead.name, "Kjell Thorvaldsen")
            test.eq(lead.company, "Impact Data")
            test.eq(lead.source, "market_acquisition")
            test.matches(lead.project_details, "four disconnected reporting stacks")
            test.matches(lead.project_details, "HubSpot Solutions Directory")
            test.eq(result.thread_id, "t-1")
        end)

        test.it("identifies a person with no email by a stable company-and-name slug", function()
            local seen = wire({ companies = { {
                name = "Impact Data", domain = "ImpactData.example",
                person_name = "Kjell Thorvaldsen", person_role = "CEO",
                contact_kind = "profile", contact_value = "https://linkedin.example/kjell",
            } } })
            assert(accept.accept({ case_id = "case-1" }))
            local lead = seen.submit.payload.lead
            test.eq(lead.person_ref, "market:impactdata.example:kjell-thorvaldsen")
            test.is_nil(lead.email)
        end)

        test.it("accepts the named company position and refuses one that is not there", function()
            wire({ companies = { CANDIDATE, CANDIDATE } })
            assert(accept.accept({ case_id = "case-1", company_index = 2 }))
            local _, err = accept.accept({ case_id = "case-1", company_index = 5 })
            test.matches(err, "no company at that position")
        end)

        test.it("refuses an unreadable case before touching anything", function()
            local seen = wire({ denied = true })
            local _, err = accept.accept({ case_id = "case-1" })
            test.matches(err, "not readable")
            test.is_nil(seen.submit)
        end)

        test.it("refuses an empty case and an absent Leads component in words", function()
            wire({ companies = {} })
            local _, empty = accept.accept({ case_id = "case-1" })
            test.matches(empty, "no qualified company")
            wire({ no_leads = true })
            local _, absent = accept.accept({ case_id = "case-1" })
            test.matches(absent, "Leads component is not installed")
        end)

        test.it("carries a Leads refusal through as the acceptance failure", function()
            wire({ submit_err = "a lead needs a person_ref" })
            local _, err = accept.accept({ case_id = "case-1" })
            test.matches(err, "could not be submitted")
        end)

        test.it("accepts a partly proven candidate with its gaps named in the dossier", function()
            local seen = wire({ companies = {}, partials = { {
                name = "Salted Stone", domain = "saltedstone.example",
                current_work_signal = "Salted Stone provides strategy and consulting services.",
                missing = { "company.person", "company.contact" },
            } } })
            local result = assert(accept.accept({ case_id = "case-1", partial = true }))
            local lead = seen.submit.payload.lead
            test.eq(lead.company, "Salted Stone")
            test.eq(lead.person_ref, "market:saltedstone.example:company")
            test.matches(lead.project_details, "Still unproven from public pages: person, contact")
            test.eq(result.thread_id, "t-1")
        end)

        test.it("keeps partly proven candidates out of the default acceptance pool", function()
            wire({ companies = {}, partials = { { name = "Salted Stone", domain = "saltedstone.example" } } })
            local _, err = accept.accept({ case_id = "case-1" })
            test.matches(err, "no qualified company")
        end)

        test.it("identifies a domain-less partial by the company name slug", function()
            local seen = wire({ companies = {}, partials = { {
                name = "Media Junction", missing = { "company.person" },
            } } })
            assert(accept.accept({ case_id = "case-1", partial = true }))
            test.eq(seen.submit.payload.lead.person_ref, "market:media-junction:company")
        end)
    end)
end

local run_cases = test.run_cases(define_tests)
return { run = function(options) return run_cases(options) end }

