-- Lua's `a and b` yields a only when a is falsy and `a or b` yields a only when
-- a is truthy, so a short-circuit result is falsy(a) | b and truthy(a) | b. The
-- side of the operand the taken edge refutes is unreachable through the
-- expression and is therefore not part of its type.

type Entry = {id: string, meta: {type: string, suite: string?}?}
type User = {name: string, nick: string?}

-- The record is the truthy side of the left operand: `and` never yields it, so
-- the chain is nil | string? and the whole result is string?.
local acyclic: Entry = {id = "a"}
local suite: string? = acyclic.meta and acyclic.meta.suite
local suite_is_not_a_record: {type: string, suite: string?} = suite -- expect-error

-- The same chain under a recurrence, and the guard that consumes it: with the
-- record gone the truthy edge holds a string alone.
local entries: {Entry} = {}
local grouped = {}
for _, entry in ipairs(entries) do
    local looped: string? = entry.meta and entry.meta.suite
    if looped then
        local narrowed: string = looped
        grouped[looped] = entry
    end
end

-- The chain's result is a string key, so the container it addresses carries an
-- inferred keyed component and its read states that component.
local group: Entry? = grouped["x"]

-- `or` is the mirror: the falsy side of the left operand is what the taken edge
-- refutes, so an optional left operand beside a total right operand states no
-- nil at all.
local user: User = {name = "n"}
local shown: string = user.nick or user.name

-- false is falsy and stays in the projection: falsy(boolean) is false, not
-- empty. The and-chain therefore admits false beside the right operand and the
-- or-chain admits true.
local ran = pcall(function() return 1 end)
local guarded: false | string = ran and user.name
local guarded_is_not_a_string: string = ran and user.name -- expect-error
local defaulted: true | string = ran or user.name
local defaulted_is_not_a_string: string = ran or user.name -- expect-error
