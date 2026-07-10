type Handler = { run: (self: Handler, n: number) -> Handler, value: number }

local function step(h: Handler): Handler
    return h:run(1)
end

local h: Handler = {
    value = 0,
    run = function(self: Handler, n: number): Handler return self end,
}
return step(h).value
