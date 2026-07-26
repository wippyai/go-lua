-- A callee whose body this analysis never evaluates has an unmodeled effect on
-- every table it receives. The declared function type states the signature, so
-- the argument is admissible, but it states nothing about what the body does
-- with it: clearing xs[1] or truncating xs are both inside the contract.

local sink: fun(m: {number})

-- The guard proves a length floor and the literal publishes its slots. The call
-- invalidates both: after it neither the floor nor the cell bounds the live
-- sequence.
local xs: {number} = {1, 2, 3}
if #xs >= 1 then
    sink(xs)
    local v: number = xs[1] -- expect-error
    print(v)
end

-- The same conclusion for an opaque array, where the length floor is the only
-- proof the read has. This is the cross-lane case: the guard is consumed after
-- the escape, not before it.
local function floor_consumed_after_escape(arr: {string}, escape: fun(m: {string})): string
    if #arr >= 3 then
        escape(arr)
        local third: string = arr[3] -- expect-error
        return third
    end
    return "fallback"
end

-- A proof established after the escape is the current one. The call before it
-- revokes nothing it did not precede.
local function guard_after_escape(arr: {string}, escape: fun(m: {string})): string
    escape(arr)
    if #arr >= 3 then
        local third: string = arr[3]
        return third
    end
    return "fallback"
end

-- A container an argument carries is reached by the same callee, so a proof
-- about the nested sequence goes stale with the root's.
local function nested_escape(escape: fun(m: {items: {number}})): number
    local box = { items = {1, 2, 3} }
    if #box.items >= 1 then
        escape(box)
        local first: number = box.items[1] -- expect-error
        return first
    end
    return 0
end

-- A read-only contract is the exception that keeps precision. Each of these
-- providers publishes a borrow, a borrow-all or an iterator source for the
-- container, so the proofs it holds survive the call.
local borrowed: {number} = {1, 2, 3}
if #borrowed >= 1 then
    local joined: string = table.concat(borrowed, ",")
    print(joined)
    print(borrowed)
    for _, item in ipairs(borrowed) do
        print(item)
    end
    local first: number = borrowed[1]
    print(first)
end

-- An append is a contract too: the border can only rise and no proven slot is
-- emptied, so both the floor and the appended cell stand.
local appended: {string} = {"a", "b"}
if #appended >= 2 then
    table.insert(appended, "c")
    local third: string = appended[3]
    print(third)
end

-- A contract that shrinks the container, and one that mutates it without
-- stating what survives, discharge nothing.
local shrunk: {number} = {1, 2, 3}
if #shrunk >= 1 then
    table.remove(shrunk)
    local first: number = shrunk[1] -- expect-error
    print(first)
end

-- A provider with no published contract at all is the fail-closed case.
local unnamed: {number} = {1, 2, 3}
if #unnamed >= 1 then
    unpublished_writer(unnamed)
    local first: number = unnamed[1] -- expect-error
    print(first)
end

-- A Lua string is immutable and its position bound rides the same length-floor
-- family, so no callee that receives one can invalidate a covered position.
local function need_integer(v: integer): integer return v end
local text: string = string.rep("a", 3)
if #text >= 3 then
    unpublished_writer(text)
    need_integer(string.byte(text, 2))
end

return floor_consumed_after_escape, guard_after_escape, nested_escape
