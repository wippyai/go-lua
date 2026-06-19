local raw: any = {
    tags = {owner = "ops", retry = 3},
}

if type(raw.tags) == "table" and type(raw.tags.owner) == "string" then
    local owner: string = raw.tags.owner
    local tags: {[string]: string} = raw.tags -- expect-error: cannot assign raw.tags because it is any, not {[string]: string}
end

return "ok"
