-- From app:accept_test: wire installs accept._component and reset clears it.
local accept: { _component: { ACCESS: { READ: string } }? } = {}

local function wire()
    accept._component = { ACCESS = { READ = "read" } }
end

local function reset()
    accept._component = nil
end

wire()
local first: string = accept._component.ACCESS.READ
reset()
local second: string = accept._component.ACCESS.READ
return first, second
