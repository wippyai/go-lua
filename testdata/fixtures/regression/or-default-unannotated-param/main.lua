-- keeper.components.tools:ui
--
-- Action-dispatched UI driving tool for MCP / agents. Thin wrapper over
-- keeper.components.ui:client which talks to the long-lived Playwright supervisor.
-- One persistent browser page survives across calls, so a session typically
-- begins with `open` (for a keeper component) or `goto` (for any URL) and
-- ends with `close` when done.

local fs: any = nil
local base64: any = nil
local security: any = nil
local audit: any = nil
local ui: any = nil
local keeper_config: any = nil
local filenames: any = nil

local PREVIEWS_FS_ID = "keeper.components:previews_fs"
local PREVIEWS_URL_PREFIX = "/app/public/previews/"

local function mint_token()
    local actor = security.actor()
    local scope = security.scope()
    if not actor or not scope then
        return nil, "No security context available"
    end
    local token_store_id = keeper_config.auth_token_store()
    local store, err = security.token_store(token_store_id)
    if not store then return nil, "Token store failed: " .. tostring(err) end
    local token, terr = store:create(actor, scope, {
        expiration = "5m",
        meta = { source = "keeper.components.tools:ui" },
    })
    store:close()
    if not token then return nil, "Token creation failed: " .. tostring(terr) end
    return token
end

local function wrap(res, label)
    if type(res) ~= "table" then
        return nil, (label or "ui") .. ": unexpected response type"
    end
    if res.success == false then
        return nil, (label or "ui") .. ": " .. tostring(res.error or "unknown")
    end
    return res
end

local ACTIONS = {}

function ACTIONS.open(params)
    if type(params.component_id) ~= "string" or params.component_id == "" then
        return nil, "component_id is required for open"
    end
    local token, terr = mint_token()
    if terr then return nil, terr end
    return wrap(ui.open({
        component_id = params.component_id,
        route = params.route,
        auth_token = token,
    }), "open")
end

function ACTIONS.goto_url(params)
    if type(params.url) ~= "string" or params.url == "" then
        return nil, "url is required for goto"
    end
    return wrap(ui.goto_url(params.url, params.wait_until), "goto")
end

function ACTIONS.snapshot(_)
    return wrap(ui.snapshot(), "snapshot")
end

function ACTIONS.click(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for click"
    end
    return wrap(ui.click(params.selector, { timeout = params.timeout_ms }), "click")
end

function ACTIONS.fill(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for fill"
    end
    return wrap(ui.fill(params.selector, tostring(params.value or ""), { timeout = params.timeout_ms }), "fill")
end

function ACTIONS.type_text(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for type"
    end
    return wrap(ui.type_text(params.selector, tostring(params.value or ""), {
        delay = params.delay_ms,
        timeout = params.timeout_ms,
    }), "type")
end

function ACTIONS.press(params)
    if type(params.key) ~= "string" or params.key == "" then
        return nil, "key is required for press"
    end
    return wrap(ui.press(params.key, params.selector), "press")
end

function ACTIONS.hover(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for hover"
    end
    return wrap(ui.hover(params.selector), "hover")
end

function ACTIONS.select_option(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for select"
    end
    return wrap(ui.select_option(params.selector, params.value), "select")
end

function ACTIONS.wait_for(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for wait_for"
    end
    return wrap(ui.wait_for(params.selector, {
        state = params.state,
        timeout = params.timeout_ms,
    }), "wait_for")
end

function ACTIONS.wait(params)
    local ms = tonumber(params.ms) or 0
    if ms <= 0 then return nil, "ms must be a positive integer for wait" end
    return wrap(ui.wait(ms), "wait")
end

function ACTIONS.eval(params)
    if type(params.js) ~= "string" or params.js == "" then
        return nil, "js is required for eval"
    end
    return wrap(ui.eval(params.js), "eval")
end

local function read_preview_png(url)
    if type(url) ~= "string" or url == "" then return nil end
    if url:sub(1, #PREVIEWS_URL_PREFIX) ~= PREVIEWS_URL_PREFIX then return nil end
    local rel: string = url:sub(#PREVIEWS_URL_PREFIX + 1)
    local storage = fs.get(PREVIEWS_FS_ID)
    if not storage then return nil end
    if not storage:exists(rel) then return nil end
    return storage:readfile(rel)
end

function ACTIONS.screenshot(params)
    local name
    if params.name ~= nil and params.name ~= "" then
        name = filenames.with_random_seed(params.name)
    end
    local shot, err = wrap(ui.screenshot({
        name = name,
        selector = params.selector,
        full = params.full == true,
        request_id = "screenshots",
    }), "screenshot")
    if err then return nil, err end

    local content = {
        { type = "text", text = "screenshot: " .. tostring(shot.url or "") },
    }
    local png = read_preview_png(shot.url)
    if png then
        local encoded = base64.encode(png)
        if encoded then
            table.insert(content, { type = "image", data = encoded, mimeType = "image/png" })
        end
    end

    return {
        url = shot.url,
        _mcp_content = content,
    }
end

function ACTIONS.assert_visible(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for assert_visible"
    end
    return wrap(ui.assert_visible(params.selector), "assert_visible")
end

function ACTIONS.assert_text(params)
    if type(params.selector) ~= "string" or params.selector == "" then
        return nil, "selector is required for assert_text"
    end
    if type(params.text) ~= "string" then
        return nil, "text is required for assert_text"
    end
    return wrap(ui.assert_text(params.selector, params.text), "assert_text")
end

function ACTIONS.expect_url(params)
    return wrap(ui.expect_url({
        url = params.url,
        contains = params.contains,
        regex = params.regex,
    }), "expect_url")
end

function ACTIONS.close(_)
    return wrap(ui.close(), "close")
end

function ACTIONS.ping(_)
    return wrap(ui.ping(), "ping")
end

local ALIASES: {[string]: string} = {
    ["goto"] = "goto_url",
    ["type"] = "type_text",
    ["select"] = "select_option",
}

local function do_handler(params)
    params = params or {}
    local action = params.action
    if type(action) ~= "string" or action == "" then
        return nil, "action is required"
    end
    local resolved = ALIASES[action] or action
    local fn = ACTIONS[resolved]
    if not fn then return nil, "unknown action: " .. tostring(action) end

    return fn(params)
end

local function handler(params)
    params = params or {}
    local action = params.action or "?"
    return audit.wrap({
        tool          = "ui",
        discriminator = "ui." .. action,
        target        = params.url or params.selector,
        params        = params,
        summarise = function(result, err)
            if err then return "ui " .. action .. " failed: " .. tostring(err) end
            return "ui." .. action
        end,
    }, function()
        return do_handler(params)
    end)
end

return { handler = handler }

