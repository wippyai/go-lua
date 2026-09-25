local test = require("test")
local json = require("json")
local gmail = require("outreach_gmail")

local function define_tests()
    test.describe("gmail.parse_created", function()
        test.it("reads the draft id off the tool's created answer", function()
            local id, err = gmail.parse_created("Draft created: id r-12345")
            test.is_nil(err)
            test.eq(id, "r-12345")
        end)
        test.it("reads the tool's own error text through", function()
            local id, err = gmail.parse_created("Error: Token expired and refresh failed")
            test.is_nil(id)
            test.eq(err, "Token expired and refresh failed")
        end)
        test.it("fails loudly on an answer it does not recognise", function()
            local id, err = gmail.parse_created("Draft saved somewhere")
            test.is_nil(id)
            test.matches(err, "unexpected")
        end)
    end)

    test.describe("gmail.create", function()
        test.after_each(function()
            gmail._available = nil
            gmail._tool_call = nil
        end)

        test.it("calls the published write tool under the named connection", function()
            local seen = {}
            gmail._available = function() return true end
            gmail._tool_call = function(id, args, ctx)
                seen.id, seen.args, seen.ctx = id, args, ctx
                return "Draft created: id d-9", nil
            end
            local id = assert(gmail.create("conn-1", "a@b.co", "Subject", "Body"))
            test.eq(id, "d-9")
            test.eq(seen.id, "kickside.google.traits:gmail_write_tool")
            test.eq(seen.args.action, "create_draft")
            test.eq(seen.args.to, "a@b.co")
            test.eq(seen.ctx.connection_id, "conn-1")
        end)

        test.it("creates the draft without a To when no recipient resolved", function()
            local seen = {}
            gmail._available = function() return true end
            gmail._tool_call = function(_, args) seen.args = args return "Draft created: id d-1", nil end
            assert(gmail.create("conn-1", "", "S", "B"))
            test.is_nil(seen.args.to)
        end)

        test.it("refuses an empty connection and an absent connector as caller errors", function()
            local _, no_conn = gmail.create("", "a@b.co", "S", "B")
            test.matches(no_conn, "no Gmail connection")
            gmail._available = function() return false end
            local _, absent = gmail.create("conn-1", "a@b.co", "S", "B")
            test.matches(absent, "not installed")
        end)
    end)

    test.describe("gmail.recipient", function()
        test.after_each(function() gmail._call = nil end)

        test.it("resolves the first linked contact to its address and name", function()
            gmail._call = function(_, args)
                test.eq(args.record_id, "person:hubspot:9")
                return json.encode({ ok = true, data = { values = { email = "l@x.co", name = "Liudmila" } } }), nil
            end
            local email, name = gmail.recipient("crm-1", {
                contacts = { { kind = "record", type = "person", id = "person:hubspot:9" } },
            })
            test.eq(email, "l@x.co")
            test.eq(name, "Liudmila")
        end)

        test.it("answers empty for a deal with no linked contact", function()
            local email = gmail.recipient("crm-1", { contacts = {} })
            test.eq(email, "")
        end)
    end)

    test.describe("gmail.link", function()
        test.it("addresses the draft in the drafts view", function()
            test.eq(gmail.link("d-9"), "https://mail.google.com/mail/u/0/#draft=d-9")
            test.eq(gmail.link(""), "https://mail.google.com/mail/u/0/#drafts")
        end)
    end)
end

local run_cases = test.run_cases(define_tests)
return { run = function(options) return run_cases(options) end }

