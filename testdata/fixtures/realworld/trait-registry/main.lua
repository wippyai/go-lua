local types = require("types")
local processor = require("processor")

local entry: types.TraitRegistryEntry = {
    id = "search-trait",
    meta = {type = types.TRAIT_TYPE, name = "Search", comment = "Web search capability"},
    data = {
        prompt = "You can search the web using the search tool.",
        tools = {
            "tool:web-search",
            {id = "tool:scrape", description = "Scrape a URL", alias = "fetch"},
            {id = "tool:summarize", context = {max_length = 500}},
        },
        context = {api_key = "sk-123"},
    },
}

local spec, err = processor.build_trait(entry)
if err == nil and spec then
    local name: string = spec.name
    local prompt: string = spec.prompt
    local tool_count: number = #spec.tools
    -- spec.tools is the typed sequence {TraitToolEntry}: it has unknown runtime
    -- length, so spec.tools[1] is TraitToolEntry? and its .id field is string?;
    -- assigning that to a non-optional string is soundly rejected.
    local first_tool_id: string = spec.tools[1].id -- expect-error: cannot assign spec.tools[1].id because it may be nil
end

local normalized = processor.normalize_tool("tool:simple")
local simple_id: string = normalized.id

local complex = processor.normalize_tool({id = "tool:complex", alias = "cx"})
local complex_alias: string? = complex.alias
