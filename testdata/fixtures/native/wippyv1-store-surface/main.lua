-- Contract: the Wippy v1 store module as the runtime declares it. Every
-- declared member is called, and every member whose final result is the module
-- error is read on both arms: the arm that observes the error and the arm that
-- goes on to use the value.

local store = require("store")

local backends: string = store.backend.KV_RAFT .. store.backend.KV_CRDT ..
    store.backend.MEMORY .. store.backend.SQL .. store.backend.UNKNOWN
local consistencies: string = store.consistency.LINEARIZABLE .. store.consistency.EVENTUAL ..
    store.consistency.LOCAL .. store.consistency.UNKNOWN

local handle, open_err = store.get("main")
if open_err ~= nil then
    return open_err:kind()
end

local info, info_err = handle:info()
if info_err ~= nil then
    return info_err:message()
end
local described: string = info.id .. ":" .. info.backend .. ":" .. info.consistency
local capable: boolean = info.durable and info.list and info.versioned and
    info.conditional_put and info.ttl

local value, get_err = handle:get("alpha")
if get_err ~= nil then
    return get_err:message()
end

local entry, entry_err = handle:entry("alpha")
if entry_err ~= nil then
    return entry_err:message()
end
local entry_key: string = entry.key
local entry_version: string = entry.version
local entry_value = entry.value

local page, list_err = handle:list({ prefix = "a", after = "a0", limit = 10 })
if list_err ~= nil then
    return list_err:message()
end
local cursor: string = page.cursor
local more: boolean = page.has_more
local rows = page.items

local written, put_err = handle:put("alpha", value, {
    ttl = 30,
    only_if_absent = true,
    if_version = entry_version,
})
if put_err ~= nil then
    return put_err:message()
end
local written_key: string = written.key

local set_ok, set_err = handle:set("beta", entry_value, 60)
if set_err ~= nil then
    return set_err:message()
end

local present, has_err = handle:has("beta")
if has_err ~= nil then
    return has_err:message()
end

local removed, delete_err = handle:delete("beta")
if delete_err ~= nil then
    return delete_err:message()
end

local released: boolean = handle:release()

if not (set_ok and present and removed and released and capable and more) then
    return "incomplete"
end
return backends .. consistencies .. described .. entry_key .. cursor ..
    written_key .. tostring(#rows)
