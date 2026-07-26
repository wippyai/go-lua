-- A local's declared type is the contract for every write to that local, not
-- only for its initializer. A reassignment that proves a value outside the
-- declaration is refuted exactly as a mismatched initializer is, so a reader of
-- the local may rely on the declaration having governed whatever wrote it.

type Rec = {id: string}
type StringOrNumber = string | number

local function sink(v: any) end
local function need_number(n: number) end

-- A scalar declaration refutes a reassignment of the wrong primitive.
local function scalar_reassign_wrong()
    local s: string = "x"
    s = 5 -- expect-error
    sink(s)
end

-- A record declaration refutes a reassignment whose member type is wrong.
local function record_reassign_wrong_member()
    local a: Rec = {id = "x"}
    a = {id = 5} -- expect-error
    sink(a)
end

-- A record declaration refutes a reassignment that omits a required member.
local function record_reassign_missing_member()
    local a: Rec = {id = "x"}
    a = {} -- expect-error
    sink(a)
end

-- A declaration without an initializer still contracts its first write.
local function declared_without_initializer()
    local b: Rec
    b = {id = 7} -- expect-error
    sink(b)
end

-- A non-optional declaration refutes a nil reassignment.
local function nil_into_required()
    local m: number = 1
    m = nil -- expect-error
    sink(m)
end

-- An authored nil initializer is a value the reader supplied, so the
-- declaration contracts it exactly as it contracts a nil reassignment.
local function nil_initializer()
    local m: number = nil -- expect-error
    sink(m)
end

-- A declaration with no initializer at all writes no value of the reader's,
-- so there is nothing yet to refute.
local function no_initializer_writes_nothing()
    local m: number
    sink(m)
end

-- A write under a branch guard carries the same contract as a straight-line
-- write: the declaration governs the arm that executes.
local function branch_guarded_write()
    local flag = true
    local s: string = "ok"
    if flag then
        s = 5 -- expect-error
    end
    sink(s)
end

-- A closure writing a captured declared local is contracted at that write.
local function upvalue_write()
    local s: string = "ok"
    local function writer()
        s = 5 -- expect-error
    end
    writer()
    sink(s)
end

-- Every target of a multiple assignment carries its own declaration.
local function multi_target_write()
    local a: string = "x"
    local b: number = 1
    a, b = 1, "y" -- expect-error
    sink(a)
    sink(b)
end

-- A refuted write is reported at the write itself and analysis continues past
-- it: a later independent violation in the same body is still reported.
local function refuted_write_does_not_halt_analysis()
    local s: string = "ok"
    s = 5 -- expect-error
    need_number("not a number") -- expect-error
end

-- A conforming record reassignment discharges the declaration.
local function record_reassign_conforming(): Rec
    local a: Rec = {id = "x"}
    a = {id = "y"}
    return a
end

-- A conforming scalar reassignment discharges the declaration.
local function scalar_reassign_conforming(): string
    local s: string = "x"
    s = "y"
    return s
end

-- A declared union accepts any of its arms, including after narrowing has
-- pinned the local to one of them.
local function union_narrow_then_widen(): StringOrNumber
    local u: StringOrNumber = "hello"
    if type(u) == "string" then
        u = 42
    end
    return u
end

-- Loop rebinding with a conforming value stays clean across iterations.
local function loop_rebind_conforming(): Rec
    local a: Rec = {id = "seed"}
    for _ = 1, 3 do
        a = {id = "step"}
    end
    return a
end

-- An optional declaration accepts a nil reassignment.
local function nil_into_optional(): number?
    local n: number? = 1
    n = nil
    return n
end

-- An undeclared local carries no write contract: rebinding is free.
local function undeclared_rebind_free()
    local v = "x"
    v = 5
    sink(v)
end

return scalar_reassign_wrong,
    record_reassign_wrong_member,
    record_reassign_missing_member,
    declared_without_initializer,
    nil_into_required,
    nil_initializer,
    no_initializer_writes_nothing,
    branch_guarded_write,
    upvalue_write,
    multi_target_write,
    refuted_write_does_not_halt_analysis,
    record_reassign_conforming,
    scalar_reassign_conforming,
    union_narrow_then_widen,
    loop_rebind_conforming,
    nil_into_optional,
    undeclared_rebind_free
