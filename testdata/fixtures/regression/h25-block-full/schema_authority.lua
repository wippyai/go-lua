-- Dispatches schema mutations to the sole authority actor for a CRM component.
-- Callers authorize before dispatching; the actor receives no caller-supplied
-- identity or scope and is the only runtime entry that imports mutation methods.

local channel = require("channel")
local crypto = require("crypto")
local process = require("process")
local time = require("time")
local types = require("types")

local M = {}

local REQUEST_TIMEOUT = "30s"

type Payload = { [string]: unknown }
type AuthorityResponse = {
    success: boolean,
    result: unknown?,
    error: unknown?,
}

local function payload(value: unknown): Payload
    if type(value) ~= "table" then return {} end
    local out: Payload = {}
    for key, item in pairs(value) do
        if type(key) == "string" then out[key] = item end
    end
    return out
end

local function spawn(crm_id: string, topic: string, message: Payload): (string?, string?)
    local host, host_err = types.schema_authority_host()
    if not host then return nil, host_err end
    local pid, err = process.spawn(types.SCHEMA_AUTHORITY_PROCESS, host, {
        crm_id = crm_id,
        bootstrap = { topic = topic, message = message },
    })
    if not pid then return nil, "CRM schema authority spawn failed: " .. tostring(err) end
    return tostring(pid), nil
end

function M.dispatch(crm_id: string, topic: string, input: unknown): (unknown?, string?)
    if type(crm_id) ~= "string" or crm_id == "" then return nil, "crm_id is required" end
    if type(topic) ~= "string" or topic == "" then return nil, "schema operation is required" end

    local reply_topic = "spiralscout.crm.schema.reply:" .. tostring(crypto.random.uuid())
    local message = payload(input)
    message.reply_pid = tostring(process.pid())
    message.reply_topic = reply_topic

    local replies = process.listen(reply_topic)
    local pid = process.registry.lookup(types.SCHEMA_AUTHORITY_REGISTRY_PREFIX .. crm_id)
    local delivered = pid and process.send(tostring(pid), topic, message) or false
    if not delivered then
        local _, spawn_err = spawn(crm_id, topic, message)
        if spawn_err then
            process.unlisten(replies)
            return nil, spawn_err
        end
    end

    local timeout = time.after(REQUEST_TIMEOUT)
    local result = channel.select({
        replies:case_receive(),
        timeout:case_receive(),
    })
    process.unlisten(replies)
    if result.channel == timeout then return nil, "CRM schema authority timeout" end

    local message_result: unknown = result.value
    local response: unknown = message_result
    if type(response) ~= "table" then
        local ok, data = pcall(function() return message_result:payload():data() end)
        if not ok then return nil, "invalid CRM schema authority response" end
        response = data
    end
    if type(response) ~= "table" then return nil, "invalid CRM schema authority response" end
    local typed = response :: AuthorityResponse
    if typed.success ~= true then return nil, tostring(typed.error or "CRM schema mutation failed") end
    return typed.result, nil
end

function M.apply(crm_id: string, declaration: unknown, expected_revision: unknown): (unknown?, string?)
    return M.dispatch(crm_id, "apply", { declaration = declaration, expected_revision = expected_revision })
end

function M.import_config(crm_id: string, artifact: unknown, expected_revision: unknown): (unknown?, string?)
    return M.dispatch(crm_id, "import", { artifact = artifact, expected_revision = expected_revision })
end

function M.provision(crm_id: string, declaration: unknown): (unknown?, string?)
    return M.dispatch(crm_id, "provision", { declaration = declaration })
end

function M.rollback_provision(crm_id: string): (boolean?, string?)
    local result, err = M.dispatch(crm_id, "rollback", {})
    if err then return nil, err end
    return result == true, nil
end

function M.ensure_object_type(crm_id: string, definition: unknown, expected_revision: unknown): (unknown?, string?)
    return M.dispatch(crm_id, "ensure_object_type", {
        definition = definition,
        expected_revision = expected_revision,
    })
end

return M
