local test = {}
function test.is_nil(value: any)
    if value ~= nil then error("expected nil") end
end
function test.not_nil(value: any)
    if value == nil then error("expected value") end
end
return test
