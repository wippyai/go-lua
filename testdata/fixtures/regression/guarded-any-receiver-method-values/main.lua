-- A method call on a value typed any, narrowed to string by a type() guard,
-- resolves on the string library: its values are the method's stated returns,
-- not dynamic ones, and a guard on a value past them does not type it number.
local function canonical_error(err_msg: any): any
    if type(err_msg) ~= "string" then return err_msg end
    local outer_id, inner_at, inner_id = err_msg:match("^Node %[([^%]]+)%] failed:%s*()Node %[([^%]]+)%] failed")
    if outer_id ~= nil and inner_id ~= nil and outer_id == inner_id and type(inner_at) == "number" then
        return err_msg:sub(inner_at)
    end
    return err_msg
end

local function method_values(m: any)
    if type(m) ~= "string" then return end
    local upper = m:upper()
    local first, second = m:match("(%w+)")
    local u: boolean = upper -- expect-error: cannot assign string to boolean
    local f: boolean = first -- expect-error: cannot assign string? to boolean
    local s: boolean = second -- expect-error: cannot assign nil to boolean
end

return { canonical_error = canonical_error, method_values = method_values }
