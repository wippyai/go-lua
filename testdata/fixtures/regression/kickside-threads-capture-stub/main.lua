-- Capture contract stub for the REAL thread_bridge. get("kickside.core:threads")
-- returns a threads instance that mirrors the core threads dedup contract: an
-- append_event whose external identity (external_source, external_id,
-- external_version, type) has been seen before is counted as a duplicate and does
-- NOT advance the head. This lets a test prove that append_inbound witnesses a
-- retried message exactly once. Appends without an external identity are always
-- inserted (the pre-fix behavior a keyless witness fell back to).
local M = {}

M.appended = ({} :: { any })
M.seen_external = ({} :: { [string]: boolean })

function M.reset()
    M.appended = ({} :: { any })
    M.seen_external = ({} :: { [string]: boolean })
end

local function external_key(event)
    local ext = type(event) == "table" and event.external or nil
    if type(ext) ~= "table" then return nil end
    local id = type(ext.external_id) == "string" and ext.external_id or ""
    if id == "" then return nil end
    local source = type(ext.external_source) == "string" and ext.external_source or ""
    local version = type(ext.external_version) == "string" and ext.external_version or ""
    local etype = type(event.type) == "string" and event.type or ""
    return source .. "\0" .. id .. "\0" .. version .. "\0" .. etype
end

local function threads_instance()
    local inst = {}
    function inst:with_actor(_actor) return inst end
    function inst:with_scope(_scope) return inst end
    function inst:open() return inst, nil end
    function inst:append_event(args)
        local event = type(args) == "table" and args.event or {}
        local key = external_key(event)
        local duplicate = false
        if key ~= nil then
            if M.seen_external[key] then
                duplicate = true
            else
                M.seen_external[key] = true
            end
        end
        M.appended[#M.appended + 1] = {
            thread_id = type(args) == "table" and args.thread_id or nil,
            event = event,
            external_key = key,
            duplicate = duplicate,
        }
        if duplicate then return { duplicate = true, inserted = 0 }, nil end
        return { duplicate = false, inserted = 1 }, nil
    end
    return inst
end

function M.get(id)
    if id == "kickside.core:threads" then return threads_instance(), nil end
    return nil, "unknown contract: " .. tostring(id)
end

return M

