type Inner = { value: number }
type Outer = { inner: Inner? }
local function get(o: Outer?): number
    if o ~= nil and o.inner ~= nil then
        return o.inner.value
    end
    return 0
end
return get({ inner = { value = 7 } })
