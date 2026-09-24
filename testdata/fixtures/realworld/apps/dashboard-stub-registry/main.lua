-- kickside/spiralscout/dashboard/src/overview_test.lua: a stub registry built
-- from canned entry lists, some of them empty.
local NS_DEFINITION_KIND = "ns.definition"
local NAV_ITEM_TYPE = "ui.nav_item"
local VIEW_COMPONENT_TYPE = "view.component"

local function stub_registry(nav_items, view_components, definitions)
    return {
        find = function(q)
            if q[".kind"] == NS_DEFINITION_KIND then return definitions end
            if q["meta.type"] == NAV_ITEM_TYPE then return nav_items end
            if q["meta.type"] == VIEW_COMPONENT_TYPE then return view_components end
            return {}
        end,
    }
end

local first = stub_registry(
    {
        { id = "kickside.uploads:nav_item", meta = {
            type = "ui.nav_item", title = "Uploads", icon = "tabler:files",
            route_name = "uploads", order = 60, component_tag = "kickside-uploads",
            comment = "Upload and convert files.",
        } },
    },
    {}, {}
)
local second = stub_registry(
    {},
    { { id = "kickside.dm:view", meta = { type = "view.component", tag_name = "kickside-dm", icon = "tabler:message", comment = "Direct messages with agents." } } },
    { { id = "kickside.skills:definition", kind = "ns.definition", meta = { title = "Kickside Skills", comment = "Reusable agent procedures as versioned skills." } } }
)
return { first, second }
