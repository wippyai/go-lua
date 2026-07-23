-- A declared-`any` value (unknown_id: any) stored into a record field and read
-- back is a concrete declared `any`, not a gradual top from an untyped source.
-- Writing it unguarded into a {[string]: string} map violates the map's element
-- domain and must be rejected; the gradual-top admission applies only to a read
-- whose path root is an unannotated parameter, never to a declared-`any` field.
local function can_access(_page)
    return true
end

local unknown_id: any = nil
local all_pages = {
    { id = unknown_id, mount_route = "/ok/:part(.*)*", secure = false },
}
local routes_map: {[string]: string} = {
    ["/ok/:part(.*)*"] = "page:ok",
}
local accessible: {[string]: string} = {}

for _, page in ipairs(all_pages) do
    local mr = page.mount_route
    if mr and routes_map[mr] == page.id and (not page.secure or can_access(page)) then
        accessible[mr] = page.id -- expect-error
    end
end

return accessible
