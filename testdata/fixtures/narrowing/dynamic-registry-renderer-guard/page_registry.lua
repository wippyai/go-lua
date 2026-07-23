type Entry = {
    id: string,
    data: {[string]: any},
}

local pages = {}

local function qualify_id(entry_id: string, relative_id: string): string
    return entry_id .. ":" .. relative_id
end

function pages.build_page(entry: Entry)
    local raw_data_func = entry.data.data_func
    local data_func: string? = nil
    if type(raw_data_func) == "string" and raw_data_func ~= "" then
        data_func = qualify_id(entry.id, raw_data_func)
    end

    local page = {}
    page.data_func = data_func
    return page
end

return pages
