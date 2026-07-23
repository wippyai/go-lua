local protocol = require("protocol")

local M = {}

local function read_payload(raw: any): {[string]: string}
    local out: {[string]: string} = {}
    if type(raw) ~= "table" then
        return out
    end
    for key, value in pairs(raw) do
        if type(key) == "string" and type(value) == "string" then
            out[key] = value
        end
    end
    return out
end

function M.decode(raw: any): protocol.EnvelopeResult
    if type(raw) ~= "table" then
        return {ok = false, error = protocol.err("bad_root", "message must be table")}
    end
    if type(raw.kind) ~= "string" then
        return {ok = false, error = protocol.err("bad_kind", "missing kind")}
    end
    if type(raw.id) ~= "string" then
        return {ok = false, error = protocol.err("bad_id", "missing id")}
    end

    if raw.kind == "task" then
        if type(raw.route_id) ~= "string" then
            return {ok = false, error = protocol.err("bad_route", "missing route")}
        end
        return {
            ok = true,
            value = {
                kind = "task",
                id = raw.id,
                route_id = raw.route_id,
                payload = read_payload(raw.payload),
            },
        }
    end

    if raw.kind == "timer" then
        if type(raw.due_at) ~= "number" then
            return {ok = false, error = protocol.err("bad_due", "missing due_at")}
        end
        return {
            ok = true,
            value = {
                kind = "timer",
                id = raw.id,
                due_at = raw.due_at,
            },
        }
    end

    return {ok = false, error = protocol.err("unknown_kind", raw.kind)}
end

return M
