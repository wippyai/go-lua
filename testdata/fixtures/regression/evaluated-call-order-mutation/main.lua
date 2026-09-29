local function get(): { value: { id: string }? }
    return { value = { id = "x" } }
end
local holder = get()
local function clear()
    holder.value = nil
end
local function consume(_first: string, _second: any) end
consume(holder.value.id, clear())
local second: string = holder.value.id
return second
