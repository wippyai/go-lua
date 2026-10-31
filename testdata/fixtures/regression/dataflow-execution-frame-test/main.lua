local json = require("json")
local test = require("test")
local execution_frame = require("execution_frame")

local original_security = execution_frame._deps.security

local function policy(id)
    return {
        id = function() return id end,
    }
end

local function capture_security(meta, policies)
    return {
        actor = function()
            return {
                id = function() return "actor:one" end,
                meta = function() return meta end,
            }
        end,
        scope = function()
            return {
                policies = function() return policies end,
            }
        end,
    }
end

local function valid_context(overrides)
    local context = {
        kind = execution_frame.KIND,
        version = execution_frame.VERSION,
        actor_meta = { tenant = "acme", nested = { enabled = true } },
        policy_ids = { "policy:a", "policy:b" },
    }
    for key, value in pairs(overrides or {}) do context[key] = value end
    return context
end

local function define_tests()
    describe("execution_frame", function()
        after_each(function()
            execution_frame._deps.security = original_security
        end)

        it("captures actor metadata and canonical policy ids in a versioned context", function()
            local meta = { tenant = "acme", flags = { "one", "two" } }
            execution_frame._deps.security = capture_security(meta, {
                policy("policy:z"),
                policy("policy:a"),
            })

            local row, capture_err = execution_frame.capture()
            test.is_nil(capture_err)
            test.eq(row.actor_id, "actor:one")

            local context, decode_err = json.decode(((row :: any).actor_context :: string))
            test.is_nil(decode_err)
            test.eq(context.version, execution_frame.VERSION)
            test.eq(context.kind, execution_frame.KIND)
            test.eq(context.actor_meta.tenant, "acme")
            test.eq(context.actor_meta.flags[2], "two")
            test.eq(context.policy_ids[1], "policy:a")
            test.eq(context.policy_ids[2], "policy:z")

            meta.tenant = "changed"
            test.eq(context.actor_meta.tenant, "acme")
        end)

        it("reconstructs the exact actor metadata and policy set", function()
            local looked_up = {}
            local actor_value = { kind = "actor" }
            local scope_value = { kind = "scope" }
            execution_frame._deps.security = {
                policy = function(id)
                    looked_up[#looked_up + 1] = id
                    return policy(id)
                end,
                new_actor = function(id, meta)
                    test.eq(id, "actor:one")
                    test.eq(meta.tenant, "acme")
                    test.is_true(meta.nested.enabled)
                    return actor_value
                end,
                new_scope = function(policies)
                    test.eq(#policies, 2)
                    return scope_value
                end,
            }

            local actor, scope, reconstruct_err = execution_frame.reconstruct("actor:one", valid_context())
            test.is_nil(reconstruct_err)
            test.eq(actor, actor_value)
            test.eq(scope, scope_value)
            test.eq(looked_up[1], "policy:a")
            test.eq(looked_up[2], "policy:b")
        end)

        it("reuses the current actor and scope only when the frozen frame matches exactly", function()
            local actor_value = {
                id = function() return "actor:one" end,
                meta = function() return { tenant = "acme", nested = { enabled = true } } end,
            }
            local scope_value = {
                policies = function()
                    return { policy("policy:b"), policy("policy:a") }
                end,
            }
            execution_frame._deps.security = {
                actor = function() return actor_value end,
                scope = function() return scope_value end,
                policy = function() error("exact current frame must not reconstruct policies") end,
                new_scope = function() error("exact current frame must not reconstruct scope") end,
                new_actor = function() error("exact current frame must not reconstruct actor") end,
            }

            local actor, scope, resolve_err = execution_frame.resolve("actor:one", valid_context())

            test.is_nil(resolve_err)
            test.eq(actor, actor_value)
            test.eq(scope, scope_value)
        end)

        it("does not reuse current identity when frozen metadata or policies differ", function()
            local reconstructed_actor = { kind = "reconstructed_actor" }
            local reconstructed_scope = { kind = "reconstructed_scope" }
            local reconstruction_calls = 0
            execution_frame._deps.security = {
                actor = function()
                    return {
                        id = function() return "actor:one" end,
                        meta = function() return { tenant = "different", nested = { enabled = true } } end,
                    }
                end,
                scope = function()
                    return {
                        policies = function() return { policy("policy:a"), policy("policy:b") } end,
                    }
                end,
                policy = function(id) return policy(id) end,
                new_scope = function()
                    reconstruction_calls = reconstruction_calls + 1
                    return reconstructed_scope
                end,
                new_actor = function()
                    reconstruction_calls = reconstruction_calls + 1
                    return reconstructed_actor
                end,
            }

            local actor, scope, resolve_err = execution_frame.resolve("actor:one", valid_context())

            test.is_nil(resolve_err)
            test.eq(actor, reconstructed_actor)
            test.eq(scope, reconstructed_scope)
            test.eq(reconstruction_calls, 2)
        end)

        it("fails closed before actor or scope construction when a policy is unavailable", function()
            local actor_calls = 0
            local scope_calls = 0
            execution_frame._deps.security = {
                policy = function(id)
                    if id == "policy:b" then return nil, "policy was removed" end
                    return policy(id)
                end,
                new_actor = function()
                    actor_calls = actor_calls + 1
                    return {}
                end,
                new_scope = function()
                    scope_calls = scope_calls + 1
                    return {}
                end,
            }

            local actor, scope, reconstruct_err = execution_frame.reconstruct("actor:one", valid_context())
            test.is_nil(actor)
            test.is_nil(scope)
            test.contains(reconstruct_err, "policy was removed")
            test.eq(actor_calls, 0)
            test.eq(scope_calls, 0)
        end)

        it("reconstructs the strict legacy Dataflow policy context", function()
            local actor_value = { kind = "legacy_actor" }
            local scope_value = { kind = "legacy_scope" }
            execution_frame._deps.security = {
                policy = function(id) return policy(id) end,
                new_scope = function(policies)
                    test.eq(#policies, 2)
                    return scope_value
                end,
                new_actor = function(id, meta)
                    test.eq(id, "actor:legacy")
                    test.eq(next(meta), nil)
                    return actor_value
                end,
            }

            local actor, scope, reconstruct_err = execution_frame.reconstruct(
                "actor:legacy",
                '{"policies":["policy:a","policy:b"]}'
            )
            test.is_nil(reconstruct_err)
            test.eq(actor, actor_value)
            test.eq(scope, scope_value)
        end)

        it("reconstructs the canonical Kickside named-scope context", function()
            local actor_value = { kind = "kickside_actor" }
            local scope_value = { kind = "kickside_scope" }
            local actor_calls = 0
            execution_frame._deps.security = {
                named_scope = function(scope_id)
                    test.eq(scope_id, "app.security:user")
                    return scope_value
                end,
                new_actor = function(id, claims)
                    actor_calls = actor_calls + 1
                    test.eq(id, "user:one")
                    test.eq(claims.email, "one@example.test")
                    return actor_value
                end,
            }

            local actor, scope, reconstruct_err = execution_frame.reconstruct("user:one", {
                version = 1,
                scope_id = "app.security:user",
                claims = { email = "one@example.test", status = "active", security_groups = { "members" } },
                captured_at = 123,
                captured_by = "kickside.core",
            })
            test.is_nil(reconstruct_err)
            test.eq(actor, actor_value)
            test.eq(scope, scope_value)
            test.eq(actor_calls, 1)
        end)

        it("reuses an exact current Kickside named scope without actor reconstruction", function()
            local claims = { email = "one@example.test", status = "active" }
            local actor_value = {
                id = function() return "user:one" end,
                meta = function() return claims end,
            }
            local current_scope = {
                policies = function() return { policy("policy:b"), policy("policy:a") } end,
            }
            execution_frame._deps.security = {
                actor = function() return actor_value end,
                scope = function() return current_scope end,
                named_scope = function()
                    return {
                        policies = function() return { policy("policy:a"), policy("policy:b") } end,
                    }
                end,
                new_actor = function() error("exact named frame must not reconstruct actor") end,
            }

            local actor, scope, resolve_err = execution_frame.resolve("user:one", {
                version = 1,
                scope_id = "app.security:user",
                claims = claims,
            })

            test.is_nil(resolve_err)
            test.eq(actor, actor_value)
            test.eq(scope, current_scope)
        end)

        it("fails closed when a current named-scope policy set differs and reconstruction is denied", function()
            execution_frame._deps.security = {
                actor = function()
                    return {
                        id = function() return "user:one" end,
                        meta = function() return { email = "one@example.test", status = "active" } end,
                    }
                end,
                scope = function()
                    return { policies = function() return { policy("policy:other") } end }
                end,
                named_scope = function()
                    return { policies = function() return { policy("policy:a") } end }
                end,
                new_actor = function() return nil, "actor creation denied" end,
            }

            local actor, scope, resolve_err = execution_frame.resolve("user:one", {
                version = 1,
                scope_id = "app.security:user",
                claims = { email = "one@example.test", status = "active" },
            })

            test.is_nil(actor)
            test.is_nil(scope)
            test.contains(resolve_err, "actor creation denied")
        end)

        it("fails closed when a canonical Kickside named scope is unavailable", function()
            local actor_calls = 0
            execution_frame._deps.security = {
                named_scope = function() return nil, "scope was removed" end,
                new_actor = function()
                    actor_calls = actor_calls + 1
                    return {}
                end,
            }

            local actor, scope, reconstruct_err = execution_frame.reconstruct("user:one", {
                version = 1,
                scope_id = "app.security:removed",
                claims = {},
            })
            test.is_nil(actor)
            test.is_nil(scope)
            test.contains(reconstruct_err, "scope was removed")
            test.eq(actor_calls, 0)
        end)

        it("rejects unknown versions and non-canonical policy sets", function()
            execution_frame._deps.security = {}

            local actor, scope, version_err = execution_frame.reconstruct("actor:one", valid_context({ version = 2 }))
            test.is_nil(actor)
            test.is_nil(scope)
            test.contains(version_err, "unsupported execution frame version")

            actor, scope, version_err = execution_frame.reconstruct("actor:one", valid_context({
                policy_ids = { "policy:b", "policy:a" },
            }))
            test.is_nil(actor)
            test.is_nil(scope)
            test.contains(version_err, "sorted and unique")
        end)

        it("does not confuse unmarked or hybrid identity shapes", function()
            execution_frame._deps.security = {}
            local contexts = {
                { version = 1, actor_meta = {}, policy_ids = {} },
                { policies = {}, claims = {} },
                { kind = "another.execution_frame", version = 1, actor_meta = {}, policy_ids = {} },
                { version = 1, scope_id = "scope", claims = {}, extra = true },
            }
            for _, context in ipairs(contexts) do
                local actor, scope, reconstruct_err = execution_frame.reconstruct("actor:one", context)
                test.is_nil(actor)
                test.is_nil(scope)
                test.not_nil(reconstruct_err)
            end
        end)

        it("rejects runtime values, cycles, sparse arrays, and excessive depth", function()
            execution_frame._deps.security = {}

            local cases = {
                { meta = { callback = function() end }, message = "unsupported value type" },
                { meta = { values = { [1] = "a", [3] = "c" } }, message = "sparse array" },
            }
            local cyclic = {}
            cyclic.self = cyclic
            cases[#cases + 1] = { meta = cyclic, message = "cycle" }

            local deep = {}
            local cursor = deep
            for _ = 1, 14 do
                cursor.next = {}
                cursor = cursor.next
            end
            cases[#cases + 1] = { meta = deep, message = "maximum depth" }

            for _, case in ipairs(cases) do
                local actor, scope, reconstruct_err = execution_frame.reconstruct("actor:one", valid_context({
                    actor_meta = case.meta,
                }))
                test.is_nil(actor)
                test.is_nil(scope)
                test.contains(reconstruct_err, case.message)
            end
        end)

        it("rejects oversized frames before reconstruction", function()
            execution_frame._deps.security = {}
            local actor, scope, reconstruct_err = execution_frame.reconstruct("actor:one", valid_context({
                actor_meta = { payload = string.rep("x", 256 * 1024 + 1) },
            }))
            test.is_nil(actor)
            test.is_nil(scope)
            test.contains(reconstruct_err, "maximum size")
        end)

        it("rejects malformed capture inputs rather than weakening identity", function()
            local meta = {}
            meta.self = meta
            execution_frame._deps.security = capture_security(meta, { policy("policy:a") })
            local row, capture_err = execution_frame.capture()
            test.is_nil(row)
            test.contains(capture_err, "cycle")

            execution_frame._deps.security = capture_security({}, {
                policy("policy:a"),
                policy("policy:a"),
            })
            row, capture_err = execution_frame.capture()
            test.is_nil(row)
            test.contains(capture_err, "duplicate policy")
        end)
    end)
end

return test.run_cases(define_tests)
