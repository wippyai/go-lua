local function get(): { [string]: { id: string }? }
    return { value = { id = "x" } }
end

local function run(key: string)
    local holder = get()
    local first: string = holder.value.id
    holder[key] = nil
    local second: string = holder.value.id
    return first, second
end
return run
