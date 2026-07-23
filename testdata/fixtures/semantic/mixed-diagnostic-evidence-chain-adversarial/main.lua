type Handler = {
    run: fun(self: Handler, id: string): number
}

local function parse_count(raw: string): number
    return "bad"
end

local function load_name(): string
    return "alice"
end

local function uppercase(value: string | number)
    value:upper()
end

local got_count: number = load_name()

local maybe_handler: Handler? = nil
maybe_handler:run("job-1")

for i = "first", 3 do
    got_count = i
end

uppercase(10)

return parse_count("10")
