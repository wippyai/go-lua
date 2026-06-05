type User = {
    id: string,
    retries: number,
}

local raw: any = {
    id = 42,
    retries = "three",
}

local user: User = raw -- expect-error

if raw.id then
    local id: string = raw.id -- expect-error
end

return "ok"
