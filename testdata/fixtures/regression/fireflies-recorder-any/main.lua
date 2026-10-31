local test = require("test")
local api = require("api")
local writer = require("writer")

-- A Fireflies that never leaves the process. The transport is faked at the seam
-- the writer uses, and the mutation itself is asserted one layer deeper: the
-- fake calls the real api.update_meeting_title with api.graphql recording what
-- would have gone over the wire, so the GraphQL document and its variables are
-- the ones a live call would carry.
local function recording(answer: any): (any, any)
    local seen: any = { calls = 0 }
    local restore = api.graphql
    api.graphql = function(conn: any, query: string, variables: any): any
        seen.calls = seen.calls + 1
        seen.conn, seen.query, seen.variables = conn, query, variables
        return answer
    end
    local transport: any = {
        connect = function(component_id: string?): (any, string?)
            seen.component_id = component_id
            return { component_id = component_id or "c1", api_key = "ff-key" }, nil
        end,
        update_meeting_title = api.update_meeting_title,
    }
    return transport, { seen = seen, done = function() api.graphql = restore end }
end

local function ok_answer(title: string): any
    return { success = true, data = { updateMeetingTitle = { title = title } } }
end

local function define_tests()
    test.describe("Fireflies record writer", function()
        -- What this connector may be asked for, stated by the connector. It is
        -- the whole of the Fireflies write surface: there is no mutation for a
        -- speaker name and none for transcript text, so claiming either would be
        -- promising a write that never happens.
        test.it("describes one system, one record kind and one writable field", function()
            local described = writer.describe({})
            test.eq(described.system, "fireflies")
            test.eq(#described.kinds, 1)
            test.eq(described.kinds[1], "transcript")
            test.eq(#described.fields, 1)
            test.eq(described.fields[1], "title")
        end)

        test.it("writes a title through updateMeetingTitle with the transcript's id", function()
            local transport, recorder = recording(ok_answer("Acme renewal call"))
            local result = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-9",
                fields = { title = "Acme renewal call" },
            }, { transport = transport, component_id = "conn-1" })
            recorder.done()

            test.is_true(result.success, tostring(result.error))
            test.eq(#(result.updated_fields or {}), 1)
            test.eq((result.updated_fields or {})[1], "title")

            local seen = recorder.seen
            test.eq(seen.calls, 1)
            test.eq(seen.component_id, "conn-1")
            test.eq(seen.query, api.UPDATE_TITLE_MUTATION)
            test.is_true(seen.query:find("updateMeetingTitle(input: $input)", 1, true) ~= nil, seen.query)
            test.eq(seen.variables.input.id, "tr-9")
            test.eq(seen.variables.input.title, "Acme renewal call")
            test.eq(seen.conn.api_key, "ff-key")
        end)

        -- No connection named, so the transport resolves the single Fireflies
        -- connection in the caller's own scope. A push therefore writes through
        -- the credential the calls were pulled with.
        test.it("writes through the caller's own connection when none is named", function()
            local transport, recorder = recording(ok_answer("Acme sync"))
            local result = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-1",
                fields = { title = "Acme sync" },
            }, { transport = transport })
            recorder.done()
            test.is_true(result.success, tostring(result.error))
            test.eq(recorder.seen.component_id, nil)
        end)

        test.it("refuses a field this API cannot write instead of writing part of it", function()
            local transport, recorder = recording(ok_answer("Acme sync"))
            local result = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-2",
                fields = { speaker_name = "Dana" },
            }, { transport = transport })
            recorder.done()
            test.is_false(result.success)
            test.is_true(tostring(result.error):find("speaker_name", 1, true) ~= nil, tostring(result.error))
            test.eq(recorder.seen.calls, 0, "a refused field still reached the API")
            -- The request is the reason, so sending it again unchanged is a
            -- wasted attempt against the same answer.
            test.is_false(result.retriable)
        end)

        -- system and kind are required by the contract. Reading an absent one as
        -- this writer's own default answers a request that addressed no
        -- connector as though it had addressed Fireflies, so the caller least
        -- able to notice its mistake is the one told nothing about it.
        test.it("refuses a write that names no system and one that names no record kind", function()
            local transport, recorder = recording(ok_answer("x"))
            local no_system = writer.update_record({
                kind = "transcript", external_id = "tr-7", fields = { title = "x" },
            }, { transport = transport })
            local no_kind = writer.update_record({
                system = "fireflies", external_id = "tr-7", fields = { title = "x" },
            }, { transport = transport })
            recorder.done()

            test.is_false(no_system.success)
            test.is_true(tostring(no_system.error):find("names the system", 1, true) ~= nil,
                tostring(no_system.error))
            test.is_false(no_kind.success)
            test.is_true(tostring(no_kind.error):find("names the record kind", 1, true) ~= nil,
                tostring(no_kind.error))
            test.eq(recorder.seen.calls, 0, "a misaddressed write still reached the API")
        end)

        test.it("refuses another system and another record kind", function()
            local transport, recorder = recording(ok_answer("x"))
            local other_system = writer.update_record({
                system = "zoom", kind = "transcript", external_id = "tr-3",
                fields = { title = "x" },
            }, { transport = transport })
            local other_kind = writer.update_record({
                system = "fireflies", kind = "meeting", external_id = "tr-3",
                fields = { title = "x" },
            }, { transport = transport })
            recorder.done()
            test.is_false(other_system.success)
            test.is_true(tostring(other_system.error):find("zoom", 1, true) ~= nil, tostring(other_system.error))
            test.is_false(other_kind.success)
            test.eq(recorder.seen.calls, 0)
        end)

        test.it("refuses a write that names no transcript and one that carries no title", function()
            local transport, recorder = recording(ok_answer("x"))
            local no_record = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "",
                fields = { title = "x" },
            }, { transport = transport })
            local no_title = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-4",
                fields = { title = "   " },
            }, { transport = transport })
            recorder.done()
            test.is_false(no_record.success)
            test.is_false(no_title.success)
            test.eq(recorder.seen.calls, 0)
        end)

        -- Fireflies answers HTTP 200 with an errors array for a rename it will
        -- not perform -- a non-admin key, or a meeting owned outside the team.
        -- The client turns that into an unsuccessful result, and the writer
        -- carries the reason out rather than reporting a write that did not
        -- happen.
        test.it("carries the API's own refusal out as the reason", function()
            local transport, recorder = recording({
                success = false, error = "Only users with admin privileges can update meeting titles",
                status_code = 200,
            })
            local result = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-5",
                fields = { title = "Acme renewal call" },
            }, { transport = transport })
            recorder.done()
            test.is_false(result.success)
            test.is_true(tostring(result.error):find("admin privileges", 1, true) ~= nil, tostring(result.error))
            -- The key never becomes an admin key by being asked twice. Marking
            -- this retriable would spend every attempt the caller has on a
            -- decision Fireflies already made.
            test.is_false(result.retriable)
            test.eq(result.status_code, 200)
        end)

        -- A rate limit reads exactly like the refusal above -- free text under a
        -- status the message never mentions. The status is what separates them,
        -- and without that separation a rename is lost to a 429 that would have
        -- cleared on the next attempt.
        test.it("marks a rate-limited write retriable rather than refused on its merits", function()
            local transport, recorder = recording({
                success = false, error = "rate limited", status_code = 429,
            })
            local result = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-429",
                fields = { title = "Acme renewal call" },
            }, { transport = transport })
            recorder.done()

            test.is_false(result.success)
            test.is_true(result.retriable, "a 429 was reported as a decision about this write")
            test.eq(result.status_code, 429)
        end)

        test.it("marks a service error retriable and a bad key permanent", function()
            local transport, recorder = recording({
                success = false, error = "request failed", status_code = 503,
            })
            local unavailable = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-503",
                fields = { title = "Acme renewal call" },
            }, { transport = transport })
            recorder.done()

            local bad_key_transport, bad_key_recorder = recording({
                success = false, error = "unauthorized (check the Fireflies API key)",
                status_code = 401,
            })
            local unauthorized = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-401",
                fields = { title = "Acme renewal call" },
            }, { transport = bad_key_transport })
            bad_key_recorder.done()

            test.is_true(unavailable.retriable, "a 503 was reported as permanent")
            test.eq(unavailable.status_code, 503)
            test.is_false(unauthorized.retriable)
            test.eq(unauthorized.status_code, 401)
        end)

        -- A request that never reached Fireflies carries status 0. Nothing about
        -- the write was judged, so the caller has not been refused anything and
        -- is free to ask again.
        test.it("marks a request that never reached the API retriable", function()
            local transport, recorder = recording({
                success = false, error = "dial tcp 1.2.3.4:443: connect: connection refused",
                status_code = 0,
            })
            local result = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-0",
                fields = { title = "Acme renewal call" },
            }, { transport = transport })
            recorder.done()

            test.is_false(result.success)
            test.is_true(result.retriable, "a transport failure was recorded as a refusal")
            test.eq(result.status_code, 0)
            test.is_true(tostring(result.error):find("connection refused", 1, true) ~= nil,
                tostring(result.error))
        end)

        test.it("refuses when no connection can be resolved", function()
            local result = writer.update_record({
                system = "fireflies", kind = "transcript", external_id = "tr-6",
                fields = { title = "Acme renewal call" },
            }, { transport = { connect = function(_id: string?): (any, string?)
                return nil, "no Fireflies connection selected"
            end } })
            test.is_false(result.success)
            test.is_true(tostring(result.error):find("no Fireflies connection", 1, true) ~= nil,
                tostring(result.error))
        end)
    end)
end

local run_cases = test.run_cases(define_tests)
return { run = function(options) return run_cases(options) end }
