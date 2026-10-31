local json = require("json")
local security = require("security")

local M = {}

M.VERSION = 1
M.KIND = "dataflow.execution_frame"
M._deps = {
    security = security,
}

local MAX_DEPTH = 12
local MAX_NODES = 2048
local MAX_BYTES = 256 * 1024
local MAX_KEY_BYTES = 256
local MAX_ID_BYTES = 512
local MAX_POLICIES = 256

local DATAFLOW_CONTEXT_FIELDS = {
    kind = true,
    version = true,
    actor_meta = true,
    policy_ids = true,
}

local LEGACY_POLICY_CONTEXT_FIELDS = {
    policies = true,
}

local KICKSIDE_CONTEXT_FIELDS = {
    version = true,
    scope_id = true,
    claims = true,
    captured_at = true,
    captured_by = true,
}

local function fail(message)
    return nil, tostring(message or "execution frame operation failed")
end

local function call(label, fn)
    local ok, first, second = pcall(fn)
    if not ok then
        return nil, label .. ": " .. tostring(first)
    end
    if second ~= nil then
        return first, tostring(second)
    end
    return first, nil
end

local function clone_plain(value, path, depth, state)
    if depth > MAX_DEPTH then
        return fail(path .. " exceeds maximum depth")
    end

    state.nodes = state.nodes + 1
    if state.nodes > MAX_NODES then
        return fail(path .. " exceeds maximum item count")
    end

    local value_type = type(value)
    if value_type == "string" then
        state.bytes = state.bytes + #value
        if state.bytes > MAX_BYTES then
            return fail(path .. " exceeds maximum size")
        end
        return value, nil
    end
    if value_type == "boolean" then
        return value, nil
    end
    if value_type == "number" then
        if value ~= value or value == math.huge or value == -math.huge then
            return fail(path .. " contains a non-finite number")
        end
        return value, nil
    end
    if value_type ~= "table" then
        return fail(path .. " contains unsupported value type " .. value_type)
    end
    if getmetatable(value) ~= nil then
        return fail(path .. " contains a table with a metatable")
    end
    if state.active[value] then
        return fail(path .. " contains a cycle")
    end

    state.active[value] = true
    local copy = {}
    local numeric_keys = 0
    local string_keys = 0
    local largest_index = 0

    for key, item in pairs(value) do
        local key_type = type(key)
        if key_type == "string" then
            string_keys = string_keys + 1
            if #key > MAX_KEY_BYTES then
                state.active[value] = nil
                return fail(path .. " contains an oversized key")
            end
            state.bytes = state.bytes + #key
            if state.bytes > MAX_BYTES then
                state.active[value] = nil
                return fail(path .. " exceeds maximum size")
            end
        elseif key_type == "number" and key > 0 and key % 1 == 0 then
            numeric_keys = numeric_keys + 1
            if key > largest_index then largest_index = key end
        else
            state.active[value] = nil
            return fail(path .. " contains an unsupported table key")
        end

        if numeric_keys > 0 and string_keys > 0 then
            state.active[value] = nil
            return fail(path .. " mixes object and array keys")
        end

        local key_path = key_type == "string" and (path .. "." .. key) or (path .. "[" .. tostring(key) .. "]")
        local cloned, clone_err = clone_plain(item, key_path, depth + 1, state)
        if clone_err then
            state.active[value] = nil
            return nil, clone_err
        end
        copy[key] = cloned
    end

    state.active[value] = nil
    if numeric_keys > 0 and largest_index ~= numeric_keys then
        return fail(path .. " contains a sparse array")
    end
    return copy, nil
end

local function bounded_plain_table(value: any, path)
    if type(value) ~= "table" then
        return fail(path .. " must be a table")
    end
    return clone_plain(value, path, 1, {
        active = {},
        nodes = 0,
        bytes = 0,
    })
end

local function validate_id(value, path)
    if type(value) ~= "string" or value == "" then
        return fail(path .. " must be a non-empty string")
    end
    if #value > MAX_ID_BYTES then
        return fail(path .. " exceeds maximum size")
    end
    return value, nil
end

local function validate_policy_ids(value)
    if type(value) ~= "table" or getmetatable(value) ~= nil then
        return fail("execution frame policy_ids must be an array")
    end

    local count = 0
    for key, _ in pairs(value) do
        if type(key) ~= "number" or key <= 0 or key % 1 ~= 0 then
            return fail("execution frame policy_ids must be an array")
        end
        count = count + 1
    end
    if count ~= #value then
        return fail("execution frame policy_ids must be a dense array")
    end
    if count > MAX_POLICIES then
        return fail("execution frame contains too many policies")
    end

    local copy = {}
    local previous = nil
    for index = 1, count do
        local id, id_err = validate_id(value[index] :: string?, "execution frame policy_ids[" .. tostring(index) .. "]")
        if id_err then return nil, id_err end
        if previous ~= nil and id <= previous then
            return fail("execution frame policy_ids must be sorted and unique")
        end
        copy[index] = id
        previous = id
    end
    return copy, nil
end

local function reject_unknown_fields(value, allowed, label)
    for key, _ in pairs(value) do
        if not allowed[key] then
            return label .. " contains unknown field " .. tostring(key)
        end
    end
    return nil
end

local function validate_dataflow_context(value: any)
    if type(value) ~= "table" or getmetatable(value) ~= nil then
        return fail("execution frame context must be a plain table")
    end
    local fields_err = reject_unknown_fields(value, DATAFLOW_CONTEXT_FIELDS, "execution frame")
    if fields_err then return fail(fields_err) end
    if value.kind ~= M.KIND then
        return fail("unsupported execution frame kind " .. tostring(value.kind))
    end
    if value.version ~= M.VERSION then
        return fail("unsupported execution frame version " .. tostring(value.version))
    end

    local actor_meta, meta_err = bounded_plain_table(value.actor_meta, "execution frame actor_meta")
    if meta_err then return nil, meta_err end
    local policy_ids, policies_err = validate_policy_ids(value.policy_ids)
    if policies_err then return nil, policies_err end

    return {
        kind = M.KIND,
        version = M.VERSION,
        actor_meta = actor_meta,
        policy_ids = policy_ids,
    }, nil
end

local function validate_legacy_policy_context(value)
    local fields_err = reject_unknown_fields(value, LEGACY_POLICY_CONTEXT_FIELDS, "legacy policy context")
    if fields_err then return fail(fields_err) end
    local policy_ids, policies_err = validate_policy_ids(value.policies)
    if policies_err then return nil, policies_err end
    return {
        format = "policy_ids",
        actor_meta = {},
        policy_ids = policy_ids,
    }, nil
end

local function validate_kickside_context(value)
    local fields_err = reject_unknown_fields(value, KICKSIDE_CONTEXT_FIELDS, "Kickside identity context")
    if fields_err then return fail(fields_err) end
    if value.version ~= 1 then
        return fail("unsupported Kickside identity version " .. tostring(value.version))
    end
    local scope_id, scope_id_err = validate_id(value.scope_id :: string?, "Kickside identity scope_id")
    if scope_id_err then return nil, scope_id_err end
    local claims, claims_err = bounded_plain_table(value.claims, "Kickside identity claims")
    if claims_err then return nil, claims_err end
    if value.captured_at ~= nil then
        local captured_at = value.captured_at
        if type(captured_at) ~= "number" or captured_at ~= captured_at
            or captured_at == math.huge or captured_at == -math.huge then
            return fail("Kickside identity captured_at must be a finite number")
        end
    end
    if value.captured_by ~= nil then
        local _, captured_by_err = validate_id(value.captured_by :: string?, "Kickside identity captured_by")
        if captured_by_err then return nil, captured_by_err end
    end
    return {
        format = "named_scope",
        actor_meta = claims,
        scope_id = scope_id,
    }, nil
end

local function validate_context(value: any)
    if type(value) ~= "table" or getmetatable(value) ~= nil then
        return fail("execution frame context must be a plain table")
    end
    if value.kind ~= nil then
        local context, context_err = validate_dataflow_context(value)
        if context_err then return nil, context_err end
        context.format = "policy_ids"
        return context, nil
    end
    if value.policies ~= nil then
        return validate_legacy_policy_context(value)
    end
    if value.scope_id ~= nil or value.claims ~= nil or value.version ~= nil then
        return validate_kickside_context(value)
    end
    return fail("unrecognized execution identity context")
end

local function encode_context(context)
    local ok, encoded, encode_err = pcall(json.encode, context)
    if not ok then return fail("execution frame encoding failed: " .. tostring(encoded)) end
    if encode_err or type(encoded) ~= "string" then
        return fail("execution frame encoding failed: " .. tostring(encode_err or "no encoded value"))
    end
    if #encoded > MAX_BYTES then
        return fail("encoded execution frame exceeds maximum size")
    end
    return encoded, nil
end

local function decode_context(actor_context)
    if type(actor_context) == "table" then
        return validate_context(actor_context)
    end
    if type(actor_context) ~= "string" or actor_context == "" then
        return fail("actor_context must be a non-empty string or table")
    end
    if #actor_context > MAX_BYTES then
        return fail("encoded execution frame exceeds maximum size")
    end

    local ok, decoded, decode_err = pcall(json.decode, actor_context)
    if not ok then return fail("execution frame decoding failed: " .. tostring(decoded)) end
    if decode_err then return fail("execution frame decoding failed: " .. tostring(decode_err)) end
    return validate_context(decoded)
end

local function plain_equal(left, right)
    if type(left) ~= type(right) then return false end
    if type(left) ~= "table" then return left == right end
    for key, value in pairs(left) do
        if not plain_equal(value, right[key]) then return false end
    end
    for key, _ in pairs(right) do
        if left[key] == nil then return false end
    end
    return true
end

local function current_actor(deps)
    return call("current actor lookup failed", function()
        return deps.security.actor()
    end)
end

local function current_scope(deps)
    return call("current scope lookup failed", function()
        return deps.security.scope()
    end)
end

local function scope_policy_ids(scope, label)
    local policies, list_err = call(label .. " policy lookup failed", function()
        return scope:policies()
    end)
    if list_err then return nil, list_err end
    if type(policies) ~= "table" then return fail(label .. " policies must be an array") end

    local policy_ids = {}
    local seen = {}
    local count = 0
    for key, policy in pairs(policies) do
        if type(key) ~= "number" or key <= 0 or key % 1 ~= 0 then
            return fail(label .. " policies must be an array")
        end
        count = count + 1
        local policy_id, policy_err = call(label .. " policy id lookup failed", function()
            return policy:id()
        end)
        if policy_err then return nil, policy_err end
        policy_id, policy_err = validate_id(policy_id, label .. " policy id")
        if policy_err then return nil, policy_err end
        if seen[policy_id] then return fail(label .. " contains a duplicate policy") end
        seen[policy_id] = true
        policy_ids[#policy_ids + 1] = policy_id
    end
    if count ~= #policies then return fail(label .. " policies must be a dense array") end
    if count > MAX_POLICIES then return fail(label .. " contains too many policies") end
    table.sort(policy_ids)
    return policy_ids, nil
end

function M.capture()
    local deps = M._deps
    local actor, actor_err = current_actor(deps)
    if actor_err then return nil, actor_err end
    if not actor then return fail("current actor is unavailable") end

    local actor_id, id_err = call("current actor id lookup failed", function()
        return actor:id()
    end)
    if id_err then return nil, id_err end
    actor_id, id_err = validate_id(actor_id, "current actor id")
    if id_err then return nil, id_err end

    local actor_meta: any, meta_err = call("current actor metadata lookup failed", function()
        return actor:meta()
    end)
    if meta_err then return nil, meta_err end
    actor_meta, meta_err = bounded_plain_table(actor_meta, "current actor metadata")
    if meta_err then return nil, meta_err end

    local scope, scope_err = current_scope(deps)
    if scope_err then return nil, scope_err end
    if not scope then return fail("current scope is unavailable") end
    local policy_ids, list_err = scope_policy_ids(scope, "current scope")
    if list_err then return nil, list_err end

    local context, context_err = validate_dataflow_context({
        kind = M.KIND,
        version = M.VERSION,
        actor_meta = actor_meta,
        policy_ids = policy_ids,
    })
    if context_err then return nil, context_err end
    local encoded, encode_err = encode_context(context)
    if encode_err then return nil, encode_err end
    return {
        actor_id = actor_id,
        actor_context = encoded,
    }, nil
end

function M.reconstruct(actor_id, actor_context)
    local valid_actor_id, actor_id_err = validate_id(actor_id, "persisted actor id")
    if actor_id_err then return nil, nil, actor_id_err end
    local context, context_err = decode_context(actor_context)
    if context_err then return nil, nil, context_err end
    if not context then return nil, nil, "execution frame decoding returned no context" end
    local restored = context :: any

    local deps = M._deps
    local scope = nil
    if restored.format == "named_scope" then
        local status = restored.actor_meta.status
        if type(status) == "string" then
            local normalized_status = status:lower()
            if normalized_status == "disabled" or normalized_status == "suspended"
                or normalized_status == "deleted" or normalized_status == "inactive" then
                return nil, nil, "actor status is " .. status .. "; refusing to reconstruct"
            end
        end
        local named_scope, named_scope_err = call("named scope reconstruction failed", function()
            return deps.security.named_scope(restored.scope_id :: string)
        end)
        if named_scope_err or not named_scope then
            return nil, nil, named_scope_err or ("named scope is unavailable: " .. tostring(restored.scope_id))
        end
        scope = named_scope
    else
        local policies: { security.Policy } = {}
        for index, policy_id in ipairs(restored.policy_ids) do
            local policy, policy_err = call("persisted policy lookup failed", function()
                return deps.security.policy(policy_id :: string)
            end)
            if policy_err or not policy then
                return nil, nil, policy_err or ("persisted policy is unavailable: " .. policy_id)
            end
            policies[index] = policy :: security.Policy
        end
        local rebuilt_scope, scope_err = call("scope reconstruction failed", function()
            return deps.security.new_scope(policies)
        end)
        if scope_err or not rebuilt_scope then
            return nil, nil, scope_err or "scope reconstruction failed: no scope returned"
        end
        scope = rebuilt_scope
    end

    local actor, actor_err = call("actor reconstruction failed", function()
        return deps.security.new_actor(valid_actor_id, restored.actor_meta)
    end)
    if actor_err or not actor then
        return nil, nil, actor_err or "actor reconstruction failed: no actor returned"
    end
    return actor, scope, nil
end

-- Resolve a frozen frame without requiring reconstruction capabilities when a
-- synchronous caller already runs under that exact actor and scope. Actor ID
-- alone is insufficient: metadata and canonical policy IDs are part of the
-- frozen execution identity and must match byte-for-structure semantics.
function M.resolve(actor_id, actor_context)
    local valid_actor_id, actor_id_err = validate_id(actor_id, "persisted actor id")
    if actor_id_err then return nil, nil, actor_id_err end
    local persisted_raw, context_err = decode_context(actor_context)
    if context_err then return nil, nil, context_err end
    local persisted = persisted_raw :: any

    local deps = M._deps
    local actor, actor_err = current_actor(deps)
    local scope, scope_err = current_scope(deps)
    if not actor_err and actor and not scope_err and scope then
        local current_id, current_id_err = call("current actor id lookup failed", function()
            return actor:id()
        end)
        if not current_id_err and current_id == valid_actor_id then
            local captured, capture_err = M.capture()
            if not capture_err and captured then
                local current_raw, decode_err = decode_context(captured.actor_context)
                local current = current_raw :: any
                if not decode_err and current and current.format == "policy_ids" and
                    plain_equal(current.actor_meta, persisted.actor_meta) then
                    if persisted.format == "policy_ids" and
                        plain_equal(current.policy_ids, persisted.policy_ids) then
                        return actor, scope, nil
                    end
                    if persisted.format == "named_scope" then
                        local status = persisted.actor_meta.status
                        local disabled = type(status) == "string" and ({
                            disabled = true,
                            suspended = true,
                            deleted = true,
                            inactive = true,
                        })[string.lower(status)] == true
                        if not disabled then
                            local named_scope = select(1, call("named scope comparison failed", function()
                                return deps.security.named_scope(persisted.scope_id :: string)
                            end))
                            if named_scope then
                                local named_policy_ids = select(1, scope_policy_ids(
                                    named_scope, "named scope comparison"))
                                if named_policy_ids and plain_equal(
                                    current.policy_ids, named_policy_ids) then
                                    return actor, scope, nil
                                end
                            end
                        end
                    end
                end
            end
        end
    end

    return M.reconstruct(valid_actor_id, actor_context)
end

return M
