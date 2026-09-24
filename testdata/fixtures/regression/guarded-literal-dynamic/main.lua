-- From app:component_packaging_test, meeting inbox card packaging.
local registry = { get = function(_id: string): unknown return {} end }
local types = { ITEM_TYPE = { CRM_UPDATE = "crm_update", FOLLOWUP = "followup" } }
type Map = { [string]: unknown }

local function packaging(entries: { unknown })
    local expected: { [string]: Map } = {}
    expected[tostring(types.ITEM_TYPE.CRM_UPDATE)] = {
        view = "spiralscout.meetings.inbox:crm_review_view",
        tag = "spiralscout-meetings-crm-review",
        entry_point = "crm_review.js",
    }
    expected[tostring(types.ITEM_TYPE.FOLLOWUP)] = {
        view = "spiralscout.meetings.inbox:followup_review_view",
        tag = "spiralscout-meetings-followup-review",
        entry_point = "followup_review.js",
    }
    for _, raw in ipairs(entries) do
        local value = tostring(raw)
        local want = expected[value]
        if want ~= nil then
            local view = registry.get(want.view)
            return view
        end
    end
    for value in pairs(expected) do
        if value == "" then return nil end
    end
    return nil
end

return packaging
