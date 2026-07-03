local x: any = "hello"
if type(x) ~= "string" then
    error("expected string")
end
local s: string = x
local upper = string.upper(s)
