local raw: any = {
    id = "cfg",
    retries = "three",
}

if type(raw.id) == "string" then
    local id: string = raw.id
    local retries: number = raw.retries -- expect-error
end

return "ok"
