-- Assertion body from wippy.test:test.
local test = {}
function test.is_nil(val: any, msg: string?)
    if val ~= nil then error(msg or "expected nil") end
end
return test
