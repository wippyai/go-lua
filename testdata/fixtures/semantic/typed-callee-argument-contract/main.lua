type Fn = (string) -> number

local function reject_wrong_argument(cb: Fn): number
    return cb(42) -- expect-error
end

local function accept_conforming_argument(cb: Fn): number
    return cb("key")
end

print(reject_wrong_argument)
print(accept_conforming_argument)
