-- keeper.components.ui:supervisor
--
-- Long-lived process that owns a single Playwright docker container and
-- serves RPC calls from HTTP handlers / MCP tools. Operations queue
-- naturally through the actor mailbox. The container is spawned on the
-- first op and killed after IDLE_TIMEOUT of inactivity.
--
-- Protocol with the container (JSONL over stdin/stdout):
--   request  : {id, op, ...args}
--   response : {id, ok, ...data, error?}
--   event    : {type:"event", event:"console"|"pageerror"|"http_error", data:...}
--   ready    : {type:"ready"}  -- sent once on startup
--
-- Client calls this process via:
--   process.send("keeper.components.ui.supervisor", "ui.command",
--                {op, args, respond_to, id})
-- and waits on its own listen channel named by respond_to.

local process = require("process")
local channel = require("channel")
local exec = require("exec")
local env = require("env")
local fs = require("fs")
local http_client = require("http_client")
local json = require("json")
local system = require("system")
local time = require("time")
local uuid = require("uuid")
local log = require("logger"):named("keeper.components.ui.supervisor")

local consts = require("consts")
local filenames = require("filenames")
local scanner = require("scanner")

local SERVICE_NAME = "keeper.components.ui.supervisor"
local COMMAND_TOPIC = "ui.command"

type Closable = { close: (Closable) -> unknown }
type RunnerProc = {
    write_stdin: (RunnerProc, string) -> unknown,
    wait: (RunnerProc) -> unknown,
}
type ShellHandle = { release: (ShellHandle) -> unknown }
type RunnerArgs = {[string]: unknown}

-- Magic-number knobs, all env-overridable. Defaults below match the
-- original inline constants so nothing changes for the common case.
local function env_int(name, fallback)
    local v = env.get(name)
    if v and v ~= "" then
        local n = tonumber(v)
        if n and n > 0 then return n end
    end
    return fallback
end

local function env_str(name, fallback)
    local v = env.get(name)
    if v and v ~= "" then return v end
    return fallback
end

local DEFAULT_IDLE_S        = env_int("KEEPER_UI_IDLE_S", 180)
local DEFAULT_CALL_TIMEOUT_S = env_int("KEEPER_UI_CALL_TIMEOUT_S", 30)
local GOTO_TIMEOUT_S         = env_int("KEEPER_UI_GOTO_TIMEOUT_S", 45)
local SHUTDOWN_WATCHDOG_S    = env_int("KEEPER_UI_SHUTDOWN_WATCHDOG_S", 5)
local TOKEN_REFRESH_S        = env_int("KEEPER_UI_TOKEN_REFRESH_S", 12 * 60 * 60)
local PW_UI_RETENTION_DAYS   = env_int("PW_UI_RETENTION_DAYS", 7)
local STDOUT_CHANNEL_BUFFER  = 64
local EVENT_RING_CAPACITY    = env_int("KEEPER_UI_EVENT_RING", 200)
local INSTALL_TIMEOUT_S      = env_int("KEEPER_UI_INSTALL_TIMEOUT_S", 180)
local PLAYWRIGHT_VERSION     = env_str("KEEPER_UI_PLAYWRIGHT_VERSION", "1.59.1")

local PW_RUNTIME_REL = ".wippy/fe-playwright-runtime"
local RUNNER_FILENAME = "ui_runner.cjs"
local PACKAGE_JSON = '{"name":"keeper-ui-runtime","private":true,"dependencies":{"playwright":"' .. PLAYWRIGHT_VERSION .. '"}}'

-- The runner is a persistent Node process that owns one Playwright
-- browser + page and handles JSONL commands forever. Kept inline so
-- updating the supervisor hot-applies in one entry write.
local RUNNER_JS = [[
'use strict';
const { chromium } = require('playwright');
const readline = require('readline');
const fs = require('fs');
const path = require('path');

let browser = null;
let context = null;
let page = null;
let origin = null;
// Remember the last explicit context options so ensurePage() can
// recreate a matching context if an op is called without an open first.
let lastContextOpts = { viewport: { width: 1280, height: 800 }, colorScheme: 'dark' };

function send(obj) {
  process.stdout.write(JSON.stringify(obj) + '\n');
}

function safeFilenameStem(value) {
  let s = String(value || '').trim();
  if (s.toLowerCase().endsWith('.png')) s = s.slice(0, -4);
  s = s.replace(/[\/\\:]+/g, '_').replace(/[^a-zA-Z0-9_-]+/g, '_');
  s = s.replace(/_+/g, '_').replace(/^[_-]+|[_-]+$/g, '');
  if (!s) s = 'screenshot';
  if (/^(con|prn|aux|nul|com[1-9]|lpt[1-9])$/i.test(s)) s = 'file_' + s;
  if (s.length > 96) s = s.slice(0, 96).replace(/[_-]+$/g, '');
  return s || 'screenshot';
}

function attachPageListeners(p) {
  p.on('console', (msg) => {
    send({ type: 'event', event: 'console', data: { type: msg.type(), text: msg.text() } });
  });
  p.on('pageerror', (err) => {
    send({ type: 'event', event: 'pageerror', data: { text: String(err && err.stack || err) } });
  });
  p.on('requestfailed', (req) => {
    const f = req.failure && req.failure();
    send({ type: 'event', event: 'request_failed', data: { url: req.url(), error: f && f.errorText } });
  });
  p.on('response', (resp) => {
    const s = resp.status();
    if (s >= 400) {
      send({ type: 'event', event: 'http_error', data: { url: resp.url(), status: s } });
    }
  });
}

async function ensurePage() {
  if (browser && context && page && !page.isClosed()) return;
  if (!browser) browser = await chromium.launch({ headless: true });
  if (!context) {
    // Fallback path when an op runs before ui_open. Mirror the last
    // known context options (default dark + 1280x800) so rendering
    // stays consistent with the keeper UI.
    context = await browser.newContext({
      viewport: lastContextOpts.viewport || { width: 1280, height: 800 },
      colorScheme: lastContextOpts.colorScheme || 'dark',
      ignoreHTTPSErrors: true,
    });
  }
  if (!page || page.isClosed()) {
    page = await context.newPage();
    attachPageListeners(page);
  }
}

async function openSession(args) {
  if (browser) {
    try { await browser.close(); } catch (_) {}
    browser = null;
    context = null;
    page = null;
  }
  browser = await chromium.launch({ headless: true });
  const ctxOpts = {
    viewport: args.viewport || { width: 1280, height: 800 },
    colorScheme: args.color_scheme || 'dark',
    ignoreHTTPSErrors: true,
  };
  if (args.storage_state) ctxOpts.storageState = args.storage_state;
  lastContextOpts = { viewport: ctxOpts.viewport, colorScheme: ctxOpts.colorScheme };
  context = await browser.newContext(ctxOpts);
  page = await context.newPage();
  origin = args.origin || null;
  attachPageListeners(page);
  if (args.start_url) {
    await page.goto(args.start_url, {
      waitUntil: args.wait_until || 'networkidle',
      timeout: args.goto_timeout || 30000,
    });
  }
  return currentState();
}

async function currentState() {
  if (!page || page.isClosed()) return { url: null, title: null, snapshot: null };
  let title = null;
  try { title = await page.title(); } catch (_) {}
  let snapshot = null;
  try {
    snapshot = await page.accessibility.snapshot({ interestingOnly: true });
  } catch (_) {}
  return {
    url: page.url(),
    title,
    snapshot,
    viewport: page.viewportSize(),
  };
}

async function handle(req) {
  const { id, op } = req;
  const to = req.timeout || 10000;
  try {
    switch (op) {
      case 'open':
        return { id, ok: true, ...(await openSession(req)) };
      case 'snapshot':
        await ensurePage();
        return { id, ok: true, ...(await currentState()) };
      case 'goto':
        await ensurePage();
        await page.goto(req.url, { waitUntil: req.wait_until || 'networkidle', timeout: req.timeout || 30000 });
        return { id, ok: true, ...(await currentState()) };
      case 'click':
        await ensurePage();
        await page.click(req.selector, { timeout: to });
        return { id, ok: true };
      case 'fill':
        await ensurePage();
        await page.fill(req.selector, req.value || '', { timeout: to });
        return { id, ok: true };
      case 'type':
        await ensurePage();
        await page.type(req.selector, req.value || '', { delay: req.delay || 0, timeout: to });
        return { id, ok: true };
      case 'press':
        await ensurePage();
        if (req.selector) {
          await page.press(req.selector, req.key, { timeout: to });
        } else {
          await page.keyboard.press(req.key);
        }
        return { id, ok: true };
      case 'hover':
        await ensurePage();
        await page.hover(req.selector, { timeout: to });
        return { id, ok: true };
      case 'select':
        await ensurePage();
        await page.selectOption(req.selector, req.value, { timeout: to });
        return { id, ok: true };
      case 'wait_for':
        await ensurePage();
        await page.waitForSelector(req.selector, { timeout: to, state: req.state || 'visible' });
        return { id, ok: true };
      case 'wait':
        await ensurePage();
        await page.waitForTimeout(req.ms || 500);
        return { id, ok: true };
      case 'eval': {
        await ensurePage();
        const value = await page.evaluate(req.js);
        return { id, ok: true, value };
      }
      case 'screenshot': {
        await ensurePage();
        const dir = req.dir;
        if (!dir) throw new Error('screenshot dir required');
        fs.mkdirSync(dir, { recursive: true });
        const rawName = req.name || ('shot-' + Date.now());
        const name = safeFilenameStem(rawName);
        const out = path.join(dir, name + '.png');
        if (req.selector) {
          await page.locator(req.selector).first().screenshot({ path: out });
        } else {
          await page.screenshot({ path: out, fullPage: req.full === true });
        }
        return { id, ok: true, name, path: out };
      }
      case 'assert_visible': {
        await ensurePage();
        const visible = await page.locator(req.selector).first().isVisible({ timeout: to });
        if (!visible) return { id, ok: false, error: 'not visible: ' + req.selector };
        return { id, ok: true };
      }
      case 'assert_text': {
        await ensurePage();
        const text = await page.locator(req.selector).first().textContent({ timeout: to });
        if (text == null || (req.text && text.indexOf(req.text) === -1)) {
          return { id, ok: false, error: 'text mismatch: expected ' + JSON.stringify(req.text) + ' in ' + JSON.stringify(text) };
        }
        return { id, ok: true, text };
      }
      case 'expect_url': {
        await ensurePage();
        const cur = page.url();
        if (req.equals && cur !== req.equals) return { id, ok: false, error: 'url not equal: ' + cur };
        if (req.contains && cur.indexOf(req.contains) === -1) return { id, ok: false, error: 'url does not contain: ' + cur };
        if (req.matches && !new RegExp(req.matches).test(cur)) return { id, ok: false, error: 'url does not match: ' + cur };
        return { id, ok: true, url: cur };
      }
      case 'close':
        if (page && !page.isClosed()) { try { await page.close(); } catch (_) {} }
        if (context) { try { await context.close(); } catch (_) {} }
        if (browser) { try { await browser.close(); } catch (_) {} }
        page = null; context = null; browser = null;
        return { id, ok: true };
      case 'ping':
        return { id, ok: true, pong: Date.now() };
      case 'quit':
        send({ id, ok: true });
        if (browser) { try { await browser.close(); } catch (_) {} }
        process.exit(0);
        return;
      default:
        return { id, ok: false, error: 'unknown op: ' + op };
    }
  } catch (err) {
    return { id, ok: false, error: String(err && err.message || err) };
  }
}

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on('line', async (line) => {
  if (!line || !line.trim()) return;
  let req;
  try { req = JSON.parse(line); } catch (err) {
    send({ id: null, ok: false, error: 'invalid json: ' + String(err) });
    return;
  }
  const reply = await handle(req);
  if (reply) send(reply);
});
rl.on('close', async () => {
  if (browser) { try { await browser.close(); } catch (_) {} }
  process.exit(0);
});

send({ type: 'ready' });
]]

local function shell_escape(s)
    if s == nil then return "''" end
    return "'" .. tostring(s):gsub("'", [['\'']]) .. "'"
end

local function host_base_url()
    local url = env.get(consts.PUBLIC_HOST_ENV)
    if not url or url == "" then
        return nil, consts.PUBLIC_HOST_ENV .. " is not configured by the host app"
    end
    return (url:gsub("/+$", ""))
end

local function host_needs_host_network(url)
    return url:find("//localhost") ~= nil or url:find("//127%.0%.0%.1") ~= nil
end

local function project_root()
    return system.process.cwd() or ""
end

local function resolve_host_uid_gid()
    local uid = env.get("HOST_UID")
    if not uid or uid == "" then uid = "1000" end
    local gid = env.get("HOST_GID")
    if not gid or gid == "" then gid = "1000" end
    return uid, gid
end

local function mint_admin_token()
    local email = env.get("USERSPACE_USER_DEFAULT_ADMIN_EMAIL")
    local password = env.get("USERSPACE_USER_DEFAULT_ADMIN_PASSWORD")
    if not email or email == "" or not password or password == "" then
        return nil, "admin credentials not in env"
    end
    local base, base_err = host_base_url()
    if not base then return nil, base_err end
    local res, err = http_client.post(base .. "/api/public/user/token", {
        headers = { ["Content-Type"] = "application/json" },
        body = json.encode({ email = email, password = password }),
        timeout = "10s",
    })
    if not res or err then return nil, "login failed: " .. (err or "unknown") end
    local ok, parsed = pcall(json.decode, res.body or "")
    if not ok or not parsed or not parsed.token then return nil, "login response missing token" end
    return parsed.token
end

local function build_storage_state(origin, token)
    if not token or token == "" then return nil end
    local stored = json.encode({
        token = token,
        expiresAt = os.date("!%Y-%m-%dT%H:%M:%S.000Z", os.time() + 24 * 60 * 60),
    })
    return {
        cookies = {},
        origins = {
            {
                origin = origin,
                localStorage = {
                    { name = "@wippy_token_info", value = stored },
                },
            },
        },
    }
end

-- File system helpers. The runtime dir holds the npm-installed playwright
-- module, the runner.cjs file, and the package.json that pins the version.
-- We talk to the host dir through the pw_runtime_fs wippy volume so we
-- don't depend on io/os (not available in wippy's sandboxed Lua).
local PW_RUNTIME_FS = "keeper.components:pw_runtime_fs"

local function runtime_dir_host(root)
    return root .. "/" .. PW_RUNTIME_REL
end

local function previews_dir_host(root)
    return root .. "/" .. consts.PATHS.PREVIEWS_REL
end

local function get_runtime_vol()
    local vol, err = fs.get(PW_RUNTIME_FS)
    if not vol then return nil, "pw_runtime_fs unavailable: " .. (err or "unknown") end
    return vol
end

local function write_runtime_assets()
    local vol, err = get_runtime_vol()
    if not vol then return false, err end
    local ok1 = pcall(function() vol:writefile("package.json", PACKAGE_JSON) end)
    local ok2 = pcall(function() vol:writefile(RUNNER_FILENAME, RUNNER_JS) end)
    if not ok1 or not ok2 then return false, "failed to write runtime assets" end
    return true
end

local function has_playwright_installed()
    local vol, err = get_runtime_vol()
    if not vol then return false end
    local ok, stat = pcall(function() return vol:stat("node_modules/playwright/package.json") end)
    if ok and stat then return true end
    return false
end

local function docker_base_args(root, uid, gid, target_url_for_network)
    local parts = { "docker", "run", "--rm" }
    local network_url = target_url_for_network or env.get(consts.PUBLIC_HOST_ENV) or ""
    if network_url ~= "" and host_needs_host_network(network_url) then
        table.insert(parts, "--network")
        table.insert(parts, "host")
    else
        table.insert(parts, "--add-host")
        table.insert(parts, "host.docker.internal:host-gateway")
    end
    table.insert(parts, "--user")
    table.insert(parts, uid .. ":" .. gid)
    table.insert(parts, "-e")
    table.insert(parts, "HOME=/tmp")
    table.insert(parts, "-v")
    table.insert(parts, shell_escape(runtime_dir_host(root)) .. ":/pw:rw")
    table.insert(parts, "-v")
    table.insert(parts, shell_escape(previews_dir_host(root)) .. ":/out:rw")
    return parts
end

local function install_playwright(root)
    log:info("installing playwright into runtime volume")
    local shell, err = exec.get(consts.HOST_SHELL_ID)
    if not shell then return false, "host shell unavailable: " .. (err or "unknown") end
    local uid, gid = resolve_host_uid_gid()
    local install_name = "keeper-ui-install-" .. uuid.v7():gsub("-", ""):sub(1, 12)
    local parts = docker_base_args(root, uid, gid, nil)
    table.insert(parts, "--name")
    table.insert(parts, install_name)
    table.insert(parts, "-w")
    table.insert(parts, "/pw")
    table.insert(parts, consts.PLAYWRIGHT_IMAGE)
    table.insert(parts, "npm")
    table.insert(parts, "install")
    table.insert(parts, "--prefer-offline")
    table.insert(parts, "--no-audit")
    table.insert(parts, "--no-fund")
    local cmd = table.concat(parts, " ")
    local proc, perr = shell:exec(cmd)
    if not proc then
        if shell.release then shell:release() end
        return false, "install spawn failed: " .. (perr or "unknown")
    end
    local stderr_stream = proc:stderr_stream()
    local stderr_buf = {}
    local ok_start = proc:start()
    if not ok_start then
        if shell.release then shell:release() end
        return false, "install start failed"
    end
    coroutine.spawn(function()
        local sc = stderr_stream:scanner("lines")
        while sc:scan() do table.insert(stderr_buf, sc:text()) end
    end)

    -- Bound the install with a watchdog so a broken registry / network
    -- hang doesn't stall wippy forever.
    local done = channel.new(1)
    local exit_code = nil
    coroutine.spawn(function()
        local c = proc:wait()
        exit_code = c
        pcall(function() done:send(true) end)
    end)
    local deadline = time.after(tostring(INSTALL_TIMEOUT_S) .. "s")
    local r = channel.select({
        done:case_receive(),
        deadline:case_receive(),
    })
    if r.channel == deadline then
        log:warn("install watchdog fired, force-killing container", { name = install_name })
        local ks, _ = exec.get(consts.HOST_SHELL_ID)
        if ks then
            local p, _ = ks:exec("docker kill " .. install_name)
            if p then pcall(function() p:start() end); pcall(function() p:wait() end) end
            if ks.release then pcall(function() ks:release() end) end
        end
        if stderr_stream.close then pcall(function() stderr_stream:close() end) end
        if shell.release then pcall(function() shell:release() end) end
        return false, "install timed out after " .. INSTALL_TIMEOUT_S .. "s"
    end

    if stderr_stream.close then pcall(function() stderr_stream:close() end) end
    if shell.release then shell:release() end
    if exit_code and exit_code ~= 0 then
        return false, "install exited " .. tostring(exit_code) .. ": " .. table.concat(stderr_buf, "\n"):sub(-500)
    end
    return true
end

-- Supervisor state owned by the main loop.
local function new_state()
    return {
        root = nil,
        shell = nil,
        proc = nil,
        proc_name = nil,       -- docker --name used for spawn; used by shutdown watchdog
        stdout_ch = nil,       -- buffered channel of parsed runner messages
        next_id = 0,
        ready = false,
        -- Last successful open args, replayed automatically when the container
        -- is lazy-respawned (e.g. after an idle kill) so agents don't have to
        -- re-authenticate on every call.
        last_open = nil,
        -- Bounded ring buffer of async runner events. Each entry has
        -- {seq, type, event, data, op}. Drained by ui_snapshot responses.
        events = { list = {}, capacity = EVENT_RING_CAPACITY, next = 1, seq = 0 },
        -- Track currently-in-flight op so async events can be tagged to it.
        current_op = nil,
    }
end

local function close_streams(state)
    if state.stdout_stream then
        local stream = state.stdout_stream :: Closable
        pcall(function() stream:close() end)
    end
    if state.stderr_stream then
        local stream = state.stderr_stream :: Closable
        pcall(function() stream:close() end)
    end
    state.stdout_stream = nil
    state.stderr_stream = nil
end

-- Bounded wait: give the runner a chance to exit cleanly, then force-kill
-- the docker container from the host side so shutdown can't hang on a stuck
-- child. Relies on state.proc_name being set in spawn_container.
local function wait_with_watchdog(state)
    local proc = state.proc
    if not proc then return end
    local done = channel.new(1)
    coroutine.spawn(function()
        pcall(function() proc:wait() end)
        pcall(function() done:send(true) end)
    end)
    local deadline = time.after(tostring(SHUTDOWN_WATCHDOG_S) .. "s")
    local r = channel.select({
        done:case_receive(),
        deadline:case_receive(),
    })
    if r.channel == deadline then
        log:warn("proc wait watchdog fired, force-killing container", { name = state.proc_name })
        if state.proc_name and state.proc_name ~= "" then
            local kill_done = channel.new(1)
            local proc_name = state.proc_name
            coroutine.spawn(function()
                local s, _ = exec.get(consts.HOST_SHELL_ID)
                if s then
                    local p, _ = s:exec("docker kill " .. proc_name)
                    if p then
                        pcall(function() p:start() end)
                        pcall(function() p:wait() end)
                    end
                    if s.release then pcall(function() s:release() end) end
                end
                pcall(function() kill_done:send(true) end)
            end)
            local kill_deadline = time.after(tostring(SHUTDOWN_WATCHDOG_S) .. "s")
            channel.select({ kill_done:case_receive(), kill_deadline:case_receive() })
        end
        local grace = time.after("1s")
        channel.select({ done:case_receive(), grace:case_receive() })
    end
end

local function kill_container(state)
    if not state.proc then return end
    log:info("stopping ui container", { name = state.proc_name })
    local proc = state.proc :: RunnerProc
    pcall(function() proc:write_stdin(json.encode({ id = -1, op = "quit" }) .. "\n") end)
    pcall(function() proc:write_stdin("") end)  -- EOF
    wait_with_watchdog(state)
    close_streams(state)
    if state.shell then
        local shell = state.shell :: ShellHandle
        pcall(function() shell:release() end)
    end
    state.proc = nil
    state.proc_name = nil
    state.shell = nil
    state.stdout_ch = nil
    state.ready = false
    state.current_op = nil
end

local function spawn_container(state)
    if state.proc then return true end
    local root = state.root or project_root()
    state.root = root
    if not root or root == "" then return false, "project root unavailable" end

    local ok_assets, aerr = write_runtime_assets()
    if not ok_assets then return false, aerr end
    if not has_playwright_installed() then
        local ok, err = install_playwright(root)
        if not ok then return false, err end
    end

    local shell, serr = exec.get(consts.HOST_SHELL_ID)
    if not shell then return false, "host shell unavailable: " .. (serr or "unknown") end

    -- Give each spawn a unique --name so the shutdown watchdog can
    -- force-kill by name if proc:wait() hangs.
    local name_suffix = uuid.v7():gsub("-", ""):sub(1, 12)
    local proc_name = "keeper-ui-runtime-" .. name_suffix

    local uid, gid = resolve_host_uid_gid()
    local parts = docker_base_args(root, uid, gid, nil)
    table.insert(parts, "--name")
    table.insert(parts, proc_name)
    table.insert(parts, "-i")
    table.insert(parts, "-w")
    table.insert(parts, "/pw")
    table.insert(parts, "-e")
    table.insert(parts, "NODE_PATH=/pw/node_modules")
    table.insert(parts, consts.PLAYWRIGHT_IMAGE)
    table.insert(parts, "node")
    table.insert(parts, "/pw/" .. RUNNER_FILENAME)
    local cmd = table.concat(parts, " ")
    log:info("spawning ui container", { cmd = cmd, name = proc_name })

    local proc, perr = shell:exec(cmd)
    if not proc then
        if shell.release then shell:release() end
        return false, "ui container spawn failed: " .. (perr or "unknown")
    end
    state.shell = shell
    state.proc = proc
    state.proc_name = proc_name
    state.stdout_stream = proc:stdout_stream()
    state.stderr_stream = proc:stderr_stream()

    local ok_start = proc:start()
    if not ok_start then
        close_streams(state)
        if shell.release then shell:release() end
        state.shell = nil
        state.proc = nil
        state.proc_name = nil
        return false, "ui container start failed"
    end
    state.stdout_ch = channel.new(STDOUT_CHANNEL_BUFFER)

    local ch = state.stdout_ch
    local stream = state.stdout_stream
    coroutine.spawn(function()
        local sc = stream:scanner("lines")
        while sc:scan() do
            local line = sc:text()
            if line and line ~= "" then
                local ok, parsed = pcall(json.decode, line)
                if ok and parsed then
                    pcall(function() ch:send(parsed) end)
                else
                    log:warn("runner stdout non-json", { line = line })
                end
            end
        end
        pcall(function() ch:close() end)
    end)

    local stderr_stream = state.stderr_stream
    coroutine.spawn(function()
        local sc = stderr_stream:scanner("lines")
        while sc:scan() do
            local line = sc:text()
            if line and line ~= "" then log:warn("ui runner stderr", { line = line }) end
        end
    end)

    -- Wait for the {type:"ready"} handshake before accepting real ops.
    local ready_timeout = time.after("15s")
    while true do
        local r = channel.select({
            ch:case_receive(),
            ready_timeout:case_receive(),
        })
        if r.channel == ready_timeout then
            kill_container(state)
            return false, "ui runner did not become ready"
        end
        local msg = r.value
        if msg and msg.type == "ready" then
            state.ready = true
            return true
        end
        -- drop pre-ready events (shouldn't happen)
    end
end

-- Push an async runner event into the bounded ring buffer, tagging it with
-- the currently-in-flight op so later responses can report "this event
-- happened during step X".
local function push_event(state, raw)
    local ring = state.events
    if not ring then return end
    ring.seq = ring.seq + 1
    local entry = {
        seq = ring.seq,
        type = raw.event,
        data = raw.data,
        op = state.current_op,
        at = os.time(),
    }
    ring.list[ring.next] = entry
    ring.next = (ring.next % ring.capacity) + 1
end

-- Return all events with seq > after_seq in order, plus the latest seq.
-- Used by snapshot responses and the dedicated drain_events op.
local function drain_events(state, after_seq)
    after_seq = after_seq or 0
    local ring = state.events
    if not ring then return {}, 0 end
    local out = {}
    -- Walk the ring from next (oldest) forward, skipping nil slots.
    for i = 0, ring.capacity - 1 do
        local idx = ((ring.next - 1 + i) % ring.capacity) + 1
        local e = ring.list[idx]
        if e and e.seq > after_seq then table.insert(out, e) end
    end
    table.sort(out, function(a, b) return a.seq < b.seq end)
    return out, ring.seq
end

local function send_one(state, op, args: RunnerArgs?, timeout_s)
    state.next_id = state.next_id + 1
    local id = state.next_id
    state.current_op = op
    local req = { id = id, op = op }
    if args then
        for k, v in pairs(args) do
            if k ~= "id" and k ~= "op" then req[k] = v end
        end
    end
    local payload = json.encode(req)
    local ok_write = pcall(function() state.proc:write_stdin(payload .. "\n") end)
    if not ok_write then
        kill_container(state)
        return nil, "write_stdin failed"
    end

    local deadline = time.after(tostring(timeout_s or DEFAULT_CALL_TIMEOUT_S) .. "s")
    while true do
        local r = channel.select({
            state.stdout_ch:case_receive(),
            deadline:case_receive(),
        })
        if r.channel == deadline then
            log:warn("ui op timed out", { op = op, id = id })
            kill_container(state)
            return nil, "op timed out: " .. op
        end
        if not r.ok then
            kill_container(state)
            return nil, "runner channel closed"
        end
        local msg = r.value
        if msg then
            if msg.type == "event" then
                push_event(state, msg)
            elseif msg.id == id then
                return msg
            end
        end
    end
end

local function call_container(state, op, args: RunnerArgs?, timeout_s)
    local fresh = false
    if not state.proc then
        local ok, err = spawn_container(state)
        if not ok then return nil, err end
        fresh = true
    end
    -- If we just spawned a fresh container AND we have a saved session from
    -- a prior open() call, replay it so auth + current page survive idle kills.
    if fresh and state.last_open and op ~= "open" then
        log:info("replaying last open on fresh container")
        state.last_open = refresh_replay_token(state.last_open)
        local replay, rerr = send_one(state, "open", state.last_open, 60)
        if rerr or (replay and not replay.ok) then
            log:warn("open replay failed", { error = rerr or (replay and replay.error) })
            state.last_open = nil
        end
    end
    return send_one(state, op, args, timeout_s)
end

local function compute_start_url(component_id, route)
    local cid = component_id or "@wippy/app-keeper"
    local desc, derr = scanner.get(cid)
    if not desc then
        -- Agents frequently pass `keeper:main` (a route prefix / view.page id) as
        -- the component id. Hint the right value so they can self-correct.
        if cid == "keeper:main" or cid:match("^[%w_%.]+:main$") then
            return nil, "component not found: " .. cid ..
                " (did you mean '@wippy/app-keeper'? '" .. cid ..
                "' is a route prefix, not a component id)"
        end
        return nil, derr or "component not found: " .. cid
    end
    local r = route or ""
    if desc.is_main_app then
        if r == "" then r = "/" end
    elseif desc.kind == "app" then
        local slug = desc.path:match("[^/]+$") or "main"
        local prefix = "/c/" .. slug .. ":main"
        if r == "" then
            r = prefix .. "/"
        elseif r:sub(1, #prefix) ~= prefix then
            if r:sub(1, 1) ~= "/" then r = "/" .. r end
            r = prefix .. r
        end
    else
        if r == "" then r = "/" end
    end
    local base, base_err = host_base_url()
    if not base then return nil, base_err end
    return base .. r, nil, base
end

-- Inject auth + start_url for open op; route screenshots into per-request dir.
local function preprocess(op, payload, root)
    if op == "open" then
        local start_url, err, origin = compute_start_url(payload.component_id, payload.route)
        if not start_url then return nil, err end
        payload.start_url = start_url
        payload.origin = origin
        local token = payload.auth_token
        local token_source
        if token and token ~= "" then
            token_source = "caller"
        else
            token = mint_admin_token()
            token_source = token and "minted" or nil
        end
        payload.storage_state = build_storage_state(payload.origin, token)
        payload.token_source = token_source
        payload.auth_token = nil
    elseif op == "screenshot" then
        -- raw_root = true tells the runner to write directly to /out/<name>.png
        -- (i.e. previews/<name>.png on the host). Used by the legacy
        -- capture_fe_screenshot path so the thumbnail scanner keeps
        -- finding PNGs at the slug locations it expects.
        if payload.raw_root then
            payload.dir = "/out"
            payload.raw_root = nil
        else
            local req_id = filenames.safe_stem(payload.request_id or "current", { fallback = "current" })
            payload.dir = "/out/ui/" .. req_id
        end
        payload.request_id = nil
    end
    return payload
end

-- Rebuild storage_state with a freshly minted admin token. Only used on
-- replay after idle-kill when the original token source was "minted".
-- Caller-provided tokens are never auto-refreshed; the caller owns them.
local function refresh_replay_token(last_open)
    if not last_open or last_open.token_source ~= "minted" then return last_open end
    local age = os.time() - (last_open.minted_at or 0)
    if age < TOKEN_REFRESH_S then return last_open end
    local token, err = mint_admin_token()
    if not token then
        log:warn("token refresh skipped, mint failed", { error = err })
        return last_open
    end
    local origin = last_open.origin
    if not origin or origin == "" then
        local base, base_err = host_base_url()
        if not base then
            log:warn("token refresh skipped, host URL unavailable", { error = base_err })
            return last_open
        end
        origin = base
    end
    last_open.storage_state = build_storage_state(origin, token)
    last_open.minted_at = os.time()
    log:info("refreshed minted token on replay", { age_s = age })
    return last_open
end

local function postprocess(op, response, request_id, state, client_last_seen_seq)
    if response and op == "screenshot" and response.ok and response.path then
        local p = tostring(response.path)  -- /out/ui/<req>/<name>.png
        local rel = p:match("^/out/(.+)$") or p
        response.url = consts.URLS.PREVIEWS_PUBLIC .. "/" .. tostring(rel)
        response.path = nil
    end
    -- Attach any async runner events (console/pageerror/http_error) that
    -- have accumulated since the caller last saw the event stream. Snapshot
    -- always returns the full buffer; other ops return only new events.
    if response and state and state.events then
        if op == "snapshot" then
            local evs, latest = drain_events(state, 0)
            response.events = evs
            response.events_seq = latest
        else
            local evs, latest = drain_events(state, client_last_seen_seq or (state.events.seq or 0))
            if #evs > 0 then
                response.events = evs
                response.events_seq = latest
            end
        end
    end
    return response
end

local function handle_command(state, msg)
    local payload = msg:payload():data()
    local from = tostring(msg:from())
    local reply_topic = payload.respond_to
    local client_id = payload.id
    local op = payload.op
    local args = payload.args or {}
    local timeout_s = payload.timeout_s

    local function reply(data)
        if reply_topic and type(reply_topic) == "string" then
            data.request_id = client_id
            process.send(from, reply_topic, data)
        end
    end

    if not op or op == "" then
        reply({ success = false, error = "op required" })
        return
    end

    local root = state.root or project_root()
    state.root = root

    -- Supervisor-internal ops that don't hit the container.
    if op == "drain_events" then
        local after = tonumber(args.after_seq) or 0
        local evs, latest = drain_events(state, after)
        reply({ success = true, ok = true, events = evs, events_seq = latest })
        return
    end

    local processed, perr = preprocess(op, args, root)
    if not processed then
        reply({ success = false, error = perr })
        return
    end

    -- Baseline: events seq at the moment the caller's request enters the
    -- supervisor. If the caller provides their own events_seq (tracking a
    -- multi-call session) we honour it, otherwise we use this baseline so
    -- the response surfaces everything that fires during this op.
    local baseline_seq = (state.events and state.events.seq) or 0
    local last_seen = tonumber(args.events_seq) or baseline_seq

    local response, err = call_container(state, tostring(op), processed :: RunnerArgs, tonumber(timeout_s))
    if err then
        reply({ success = false, error = err })
        return
    end

    -- Remember the last successful open so we can replay it when the
    -- container is respawned after an idle kill.
    if op == "open" and response and response.ok then
        processed.minted_at = os.time()
        state.last_open = processed
    elseif op == "close" then
        state.last_open = nil
    end

    response = postprocess(op, response, args.request_id or "current", state, last_seen)
    if response.ok then
        response.success = true
    else
        response.success = false
    end
    reply(response)
end

-- Recursively delete a path via the fs volume. Used for pruning old
-- ui/<request_id>/ subdirs. Missing methods or errors are swallowed so
-- a flaky filesystem can never crash the supervisor.
local function remove_recursive(vol, path)
    local ok_is, is_dir = pcall(function() return vol:isdir(path) end)
    if ok_is and is_dir then
        local ok_iter, it = pcall(function() return vol:readdir(path) end)
        if ok_iter and it then
            for entry in it do
                local name
                if type(entry) == "table" then
                    name = entry.name or entry[1]
                else
                    name = tostring(entry)
                end
                if name and name ~= "" and name ~= "." and name ~= ".." then
                    remove_recursive(vol, path .. "/" .. name)
                end
            end
        end
    end
    pcall(function() vol:remove(path) end)
end

-- Best-effort prune of /out/ui/<request_id>/ subdirectories older than
-- PW_UI_RETENTION_DAYS. Called from the main loop whenever the idle timer
-- fires (naturally rate-limited to once every DEFAULT_IDLE_S). Pure fs
-- operations, never throws. Missing stat.mtime is treated as "keep".
local function prune_ui_runs()
    local vol, err = fs.get(consts.FS.PREVIEWS_FS_ID)
    if not vol or err then return end
    local ok_exists, exists = pcall(function() return vol:exists("ui") end)
    if not ok_exists or not exists then return end
    local cutoff = os.time() - (PW_UI_RETENTION_DAYS * 86400)
    local ok_iter, it = pcall(function() return vol:readdir("ui") end)
    if not ok_iter or not it then return end
    local candidates = {}
    for entry in it do
        local name = (type(entry) == "table" and (entry.name or entry[1])) or tostring(entry)
        if name and name ~= "" and name ~= "." and name ~= ".." then
            table.insert(candidates, "ui/" .. name)
        end
    end
    local removed = 0
    for _, rel in ipairs(candidates) do
        local ok_stat, stat = pcall(function() return vol:stat(rel) end)
        local mtime = nil
        if ok_stat and type(stat) == "table" then
            mtime = stat.mtime or stat.modified or stat.mtime_unix
        end
        if mtime and tonumber(mtime) and tonumber(mtime) < cutoff then
            remove_recursive(vol, rel)
            removed = removed + 1
        end
    end
    if removed > 0 then
        log:info("pruned ui screenshot runs", { removed = removed, days = PW_UI_RETENTION_DAYS })
    end
end

local function run()
    log:info("ui supervisor starting")
    process.registry.register(SERVICE_NAME)

    local state = new_state()
    local inbox = process.inbox()
    local proc_events = process.events()

    while true do
        local idle = time.after(tostring(DEFAULT_IDLE_S) .. "s")
        local result = channel.select({
            inbox:case_receive(),
            proc_events:case_receive(),
            idle:case_receive(),
        })

        if not result.ok then
            log:warn("channel closed, exiting")
            break
        end

        if result.channel == inbox then
            local ok, err = pcall(handle_command, state, result.value)
            if not ok then log:error("handle_command crashed", { err = tostring(err) }) end
        elseif result.channel == idle then
            if state.proc then
                log:info("idle timeout, killing ui container")
                kill_container(state)
            end
            pcall(prune_ui_runs)
        elseif result.channel == proc_events then
            local ev = result.value
            if ev and ev.kind == process.event.CANCEL then
                log:info("shutdown requested")
                kill_container(state)
                return { status = "cancelled" }
            end
        end
    end

    kill_container(state)
    return { status = "completed" }
end

return { run = run }

