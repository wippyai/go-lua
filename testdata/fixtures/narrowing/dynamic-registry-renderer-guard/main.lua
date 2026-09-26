local page_registry = require("page_registry")

local function takes_string(name: string)
    return name
end

local function get_page_data(page)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end

    local name: string = page.data_func -- expect-error: cannot assign string | true to string
    takes_string(page.data_func) -- expect-error: argument 1: expected string, got string | true
    return {}, nil
end

local page = page_registry.build_page({
    id = "demo",
    data = { data_func = "load_data" },
})

-- get_page_data is unannotated: its parameter type comes from this call site,
-- so the call is not checked against it. The mismatch is reported in the body,
-- where string is required (lines 12 and 13).
return get_page_data(page)
