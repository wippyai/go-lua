-- Fields a callee writes through its parameter are visible to the caller,
-- also when the write happens one call deeper (kickside uploads view_files:
-- apply_content(result, ...) then apply_text_window(result, ...), which
-- copies a window's fields with result[key] = value).
local function read_window(text: string)
    return { content = text, content_length = #text }
end

local function apply_window(result: any, text: string)
    local window = read_window(text)
    for key, value in pairs(window) do
        result[key] = value
    end
end

local function apply_content(result: any, text: string)
    if text == "" then
        result.content_omitted = true
        return
    end
    apply_window(result, text)
end

local function apply_image(result: any, data: string): boolean
    if #data > 10 then
        result.image_omitted = true
        return false
    end
    result.content = data
    return true
end

local function build(items: {string}): {string}
    local results = {}
    for _, item in ipairs(items) do
        if item == "" then
            table.insert(results, { error = "empty" })
        else
            local result = { name = item }
            if #item > 3 then
                apply_image(result, item)
            else
                apply_content(result, item)
            end
            table.insert(results, result)
        end
    end
    local out: {string} = {}
    for _, r in ipairs(results) do
        if r.error then
            table.insert(out, r.error)
        elseif r.content then
            table.insert(out, r.content)
        end
    end
    return out
end

return { build = build }
