type Box<T> = {value: T}
type StringBox = {value: string}

local function make<T>(value: T): Box<T>
    return {value = value}
end

-- No gradual `any` stands between the inferred T = boolean and the StringBox
-- (value: string) context, so the mismatch is a genuine static error and stays
-- rejected.
local function build(): StringBox
    return make(true) -- expect-error
end

return build
