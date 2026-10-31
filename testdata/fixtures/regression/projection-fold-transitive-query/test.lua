local test: { [string]: any } = {}
function test.is_nil(val: any)
    if val ~= nil then error("expected nil", 2) end
end
return test
