-- From app:component_packaging_test, the guarded item-type-to-view lookup.
local registry = { get = function(_id: string): unknown return {} end }
local types = { ITEM_TYPE = { CRM_UPDATE = "crm_update", FOLLOWUP = "followup" } }
type Map = { [string]: unknown }

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

local value: string = "crm_update"
local want = expected[value]
if want ~= nil then
    return registry.get(want.view)
end
return nil
