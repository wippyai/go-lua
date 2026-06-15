local raw: any = {
    items = {"ok", 42},
}

if type(raw.items) == "table" and type(raw.items[1]) == "string" then
    local first: string = raw.items[1]
    local all_items: {string} = raw.items -- expect-error: cannot assign any to string[]
end

return "ok"
