local conformance = require("conformance")
local source = require("source")
local test = require("test")

local function fake_transport(): any
    local fake: any = { lists = {}, batches = {} }
    function fake.connect(component_id: string?): (any, string?)
        return { component_id = component_id, api_key = "pat" }, nil
    end
    function fake.list_objects(_conn: any, object_type: string, query: any): any
        fake.lists[#fake.lists + 1] = { object_type = object_type, query = query }
        if query.after == "objects-2" then
            return { success = true, data = { results = { { id = "30" } } } }
        end
        return {
            success = true,
            data = {
                results = { { id = "10" }, { id = "20" } },
                paging = { next = { after = "objects-2" } },
            },
        }
    end
    function fake.batch_read_associations(_conn: any, from_type: string, to_type: string, inputs: any): any
        fake.batches[#fake.batches + 1] = { from_type = from_type, to_type = to_type, inputs = inputs }
        if inputs[1].after == "assoc-2" then
            return {
                success = true,
                status_code = 200,
                data = { results = {
                    {
                        from = { id = "10" },
                        to = {
                            { toObjectId = "200", associationTypes = {
                                { category = "USER_DEFINED", typeId = 42, label = "Decision maker" },
                            } },
                        },
                    },
                } },
            }
        end
        if inputs[1].id == "30" then
            return { success = true, status_code = 200, data = { results = {} } }
        end
        return {
            success = true,
            status_code = 207,
            data = { numErrors = 0, results = {
                {
                    from = { id = "10" },
                    to = {
                        { toObjectId = "100", associationTypes = {
                            { category = "HUBSPOT_DEFINED", typeId = 1, label = "Primary" },
                            { category = "USER_DEFINED", typeId = 42, label = "Decision maker" },
                        } },
                    },
                    paging = { next = { after = "assoc-2" } },
                },
                { from = { id = "20" }, to = {} },
            } },
        }
    end
    return fake
end

local function define_tests()
    test.describe("HubSpot association source", function()
        test.it("never returns more items than the requested limit and loses nothing across pulls", function()
            -- Three objects fanning out five association rows each: a page of
            -- objects can flatten far past the engine limit, and the engine
            -- refuses an over-limit answer outright.
            local fake: any = { batches = {} }
            function fake.connect(_id: any): (any, any)
                return { component_id = "conn-1" }, nil
            end
            function fake.list_objects(_conn: any, _from: string, _query: any): any
                return { success = true, data = { results = { { id = "1" }, { id = "2" }, { id = "3" } } } }
            end
            function fake.batch_read_associations(_conn: any, _from: string, _to: string, inputs: any): any
                local results = {}
                for _, input in ipairs(inputs) do
                    local to = {}
                    for n = 1, 5 do
                        to[#to + 1] = { toObjectId = tostring(input.id) .. "0" .. tostring(n), associationTypes = {
                            { category = "HUBSPOT_DEFINED", typeId = 1, label = "Primary" },
                        } }
                    end
                    results[#results + 1] = { from = { id = input.id }, to = to }
                end
                return { success = true, status_code = 200, data = { results = results } }
            end

            local config = { from_object_type = "contacts", to_object_type = "companies", connection_id = "conn-1" }
            local seen: { [string]: boolean } = {}
            local total = 0
            local cursor: any = nil
            for _ = 1, 12 do
                local page = source.pull({ config = config, limit = 4, cursor = cursor }, nil, { transport = fake })
                test.is_true(page.success, tostring(page.error))
                local items = page.items or {}
                test.is_true(#items <= 4, "page exceeded the limit: " .. tostring(#items))
                for _, item in ipairs(items) do
                    test.is_true(seen[item.dedup_key] ~= true, "duplicate emission: " .. tostring(item.dedup_key))
                    seen[item.dedup_key] = true
                    total = total + 1
                end
                if page.has_more ~= true then break end
                cursor = page.next_cursor
            end
            test.eq(total, 15)
        end)

        test.it("preserves object and per-record association pagination", function()
            local fake = fake_transport()
            local config = {
                connection_id = "component:hubspot/one",
                from_object_type = "contacts",
                to_object_type = "companies",
            }
            local first = source.pull({ config = config, limit = 2 }, nil, { transport = fake })
            test.eq(first.success, true)
            test.eq(first.has_more, true)
            test.eq(#first.items, 2)
            test.eq(first.next_cursor.object_after, "objects-2")
            test.eq(first.next_cursor.association_inputs[1].id, "10")
            test.eq(first.next_cursor.association_inputs[1].after, "assoc-2")
            test.eq(first.items[1].item_key, "hubspot:association:component%3Ahubspot%2Fone:contacts:10:companies:100:HUBSPOT_DEFINED:1")
            test.eq(first.items[1].payload.kind, "association")
            test.eq(first.items[1].payload.edge.from.object_type, "contacts")
            test.eq(first.items[1].payload.edge.to.id, "100")
            test.eq(first.items[2].payload.edge.association_type.label, "Decision maker")

            local second = source.pull({ config = config, cursor = first.next_cursor }, nil, { transport = fake })
            test.eq(second.success, true)
            test.eq(second.has_more, true)
            test.eq(second.items[1].payload.edge.to.id, "200")
            test.eq(second.next_cursor.object_after, "objects-2")
            test.is_nil(second.next_cursor.association_inputs)
            test.eq(fake.lists[2], nil)

            local third = source.pull({ config = config, cursor = second.next_cursor }, nil, { transport = fake })
            test.eq(third.success, true)
            test.eq(third.has_more, false)
            test.eq(fake.lists[2].query.after, "objects-2")
            test.eq(fake.batches[3].inputs[1].id, "30")
        end)

        test.it("normalizes numeric HubSpot object identifiers at the transport boundary", function()
            local fake = fake_transport()
            function fake.list_objects(_conn: any, _object_type: string, _query: any): any
                return { success = true, data = { results = { { id = 10 } } } }
            end
            function fake.batch_read_associations(_conn: any, _from: string, _to: string, inputs: any): any
                test.eq(inputs[1].id, "10")
                return {
                    success = true,
                    status_code = 200,
                    data = {
                        results = {
                            {
                                from = { id = 10 },
                                to = {
                                    {
                                        toObjectId = 100,
                                        associationTypes = {
                                            { category = "HUBSPOT_DEFINED", typeId = 1 },
                                        },
                                    },
                                },
                            },
                        },
                    },
                }
            end
            local result = source.pull({
                config = { connection_id = "conn", from_object_type = "contacts", to_object_type = "companies" },
            }, nil, { transport = fake })
            test.eq(result.success, true)
            test.eq(result.items[1].payload.edge.from.id, "10")
            test.eq(result.items[1].payload.edge.to.id, "100")
        end)

        test.it("uses a versioned dedup key and connection-scoped identity", function()
            local first_fake = fake_transport()
            local a = source.pull({
                config = { connection_id = "conn-a", from_object_type = "contacts", to_object_type = "companies" },
            }, nil, { transport = first_fake })
            local second_fake = fake_transport()
            local b = source.pull({
                config = { connection_id = "conn-b", from_object_type = "contacts", to_object_type = "companies" },
            }, nil, { transport = second_fake })
            test.is_true(a.items[1].item_key ~= b.items[1].item_key)
            test.is_true(a.items[1].source_version ~= "")
            test.is_true(a.items[1].dedup_key ~= a.items[1].item_key .. ":upsert")
        end)

        test.it("does not advance a partial 207 response", function()
            local fake = fake_transport()
            function fake.batch_read_associations(_conn: any, _from: string, _to: string, _inputs: any): any
                return {
                    success = true,
                    status_code = 207,
                    data = { numErrors = 1, results = {}, errors = { { message = "one failed" } } },
                }
            end
            local result = source.pull({
                config = { connection_id = "conn", from_object_type = "contacts", to_object_type = "companies" },
            }, nil, { transport = fake })
            test.eq(result.success, false)
            test.is_true(result.error.message:find("partial errors", 1, true) ~= nil)
        end)

        test.it("keeps successful associations when missing objects race a batch read", function()
            local fake = fake_transport()
            function fake.batch_read_associations(_conn: any, _from: string, _to: string, _inputs: any): any
                return {
                    success = true,
                    status_code = 207,
                    data = {
                        numErrors = 1,
                        errors = {
                            {
                                category = "OBJECT_NOT_FOUND",
                                message = "source object disappeared",
                                context = { id = { "20" } },
                            },
                        },
                        results = {
                            {
                                from = { id = "10" },
                                to = {
                                    {
                                        toObjectId = "100",
                                        associationTypes = {
                                            { category = "HUBSPOT_DEFINED", typeId = 1, label = "Primary" },
                                        },
                                    },
                                },
                            },
                        },
                    },
                }
            end
            local result = source.pull({
                config = { connection_id = "conn", from_object_type = "contacts", to_object_type = "companies" },
            }, nil, { transport = fake })
            test.eq(result.success, true)
            test.eq(#result.items, 1)
            test.eq(result.items[1].payload.edge.from.id, "10")
            test.eq(result.items[1].payload.edge.to.id, "100")
        end)

        test.it("enumerates the same stable identities for reconciliation", function()
            local result = source.pull_keys({
                config = {
                    connection_id = "conn",
                    from_object_type = "contacts",
                    to_object_type = "companies",
                },
                limit = 2,
            }, nil, { transport = fake_transport() })
            test.eq(result.success, true)
            test.eq(#result.keys, 2)
            test.eq(result.keys[1].item_key, "hubspot:association:conn:contacts:10:companies:100:HUBSPOT_DEFINED:1")
            test.is_true(result.keys[1].dedup_key ~= result.keys[1].item_key)
            test.eq(result.has_more, true)
            test.eq(result.next_cursor.association_inputs[1].after, "assoc-2")
        end)

        test.it("pull_keys connects with the request-carried connection component", function()
            local fake = fake_transport()
            local result = source.pull_keys({
                component_id = "conn-reconcile",
                config = {
                    from_object_type = "contacts",
                    to_object_type = "companies",
                },
                limit = 2,
            }, nil, { transport = fake })
            test.eq(result.success, true)
            test.eq(result.keys[1].item_key, "hubspot:association:conn-reconcile:contacts:10:companies:100:HUBSPOT_DEFINED:1")
        end)

        test.it("conforms to the pullable envelope", function()
            local fake = fake_transport()
            local result = conformance.run({
                pull = function(request: any): any
                    return source.pull(request, nil, { transport = fake })
                end,
                config = function(_name: string): any
                    return {
                        connection_id = "conn",
                        from_object_type = "contacts",
                        to_object_type = "companies",
                    }
                end,
                failure_config = {
                    connection_id = "conn",
                    from_object_type = "",
                    to_object_type = "companies",
                },
                pull_keys = function(request: any): any
                    return source.pull_keys(request, nil, { transport = fake_transport() })
                end,
                backfill_since = { mode = "ignored", reason = "Associations are enumerated from the current object graph." },
                limit = 2,
            })
            test.eq(result.success, true, conformance.format_failures(result))
        end)
    end)
end

local run_cases = test.run_cases(define_tests)
return { run = function(options) return run_cases(options) end }
