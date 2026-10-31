-- HTTP shell for the market surface: start a hunt, accept a candidate, and read
-- the configuration the form needs. Hunt listing is not proxied -- the page
-- reads the published /research/cases API directly under the same session.
local http = require("http")
local security = require("security")
local hunt = require("hunt")
local accept = require("accept")
local deepen = require("deepen")
local dismiss = require("dismiss")
local stats = require("stats")
local config = require("config")

local M = {}

M._hunt = hunt :: any
M._accept = accept :: any
M._deepen = deepen :: any
M._dismiss = dismiss :: any
M._stats = stats :: any
M._config = config :: any

local function guarded(handler: any)
    return function()
        local req, res = http.request(), http.response()
        if not req or not res then return nil, "no http context" end
        res:set_content_type(http.CONTENT.JSON)
        if not security.actor() then
            res:set_status(http.STATUS.UNAUTHORIZED)
            res:write_json({ success = false, error = "authentication required" })
            return
        end
        handler(req, res)
    end
end

local function body_of(req: any): any
    local json = require("json")
    local decoded, err = json.decode(tostring(req:body() or ""))
    if err or type(decoded) ~= "table" then return {} end
    return decoded
end

M.start_hunt = guarded(function(req: any, res: any)
    local result, err = M._hunt.start(body_of(req))
    if err then
        res:set_status(http.STATUS.BAD_REQUEST)
        res:write_json({ success = false, error = tostring(err) })
        return
    end
    res:write_json({ success = true, hunt = result })
end)

M.accept_candidate = guarded(function(req: any, res: any)
    local result, err = M._accept.accept(body_of(req))
    if err then
        res:set_status(http.STATUS.BAD_REQUEST)
        res:write_json({ success = false, error = tostring(err) })
        return
    end
    res:write_json({ success = true, lead = result })
end)

M.deepen_candidate = guarded(function(req: any, res: any)
    local result, err = M._deepen.deepen(body_of(req))
    if err then
        res:set_status(http.STATUS.BAD_REQUEST)
        res:write_json({ success = false, error = tostring(err) })
        return
    end
    res:write_json({ success = true, hunt = result })
end)

M.dismiss_candidate = guarded(function(req: any, res: any)
    local result, err = M._dismiss.dismiss(body_of(req))
    if err then
        res:set_status(http.STATUS.BAD_REQUEST)
        res:write_json({ success = false, error = tostring(err) })
        return
    end
    res:write_json({ success = true, dismissed = result })
end)

M.get_stats = guarded(function(req: any, res: any)
    local hours = tonumber(req:query("hours"))
    local result, err = M._stats.yield({ hours = hours })
    if err then
        res:set_status(http.STATUS.BAD_REQUEST)
        res:write_json({ success = false, error = tostring(err) })
        return
    end
    res:write_json({ success = true, yield = result })
end)

M.get_config = guarded(function(_req: any, res: any)
    local snapshot = M._config.snapshot()
    res:write_json({
        success = true,
        configured = snapshot.workspace_ref ~= "" and snapshot.profile_ref ~= "",
        default_target_count = snapshot.default_target_count,
        default_depth = snapshot.default_depth,
        ecosystem_seeds = snapshot.ecosystem_seeds,
    })
end)

return M

