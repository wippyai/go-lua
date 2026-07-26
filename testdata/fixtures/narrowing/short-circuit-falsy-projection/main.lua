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

-- A nested body states the same projection. The front threads the result of a
-- value-position short-circuit through a cell it writes once before the guard
-- and once on the edge that evaluates the right operand, so no single claim
-- names either operand; the cell is filled from this declaration's own terms
-- alone, so the contract stated on it is decided where the formals are seeded.
local function shown_of(u: User): string
    local nested_or: string = u.nick or u.name
    local nested_or_is_not_a_number: number = u.nick or u.name -- expect-error
    return nested_or
end
local called: string = shown_of(user)

-- The and-form inside the same body: the record side is refuted by the edge
-- that carries the result, so the chain stays optional and a total annotation
-- on it is refused.
local function suite_of(e: Entry): string?
    local nested_and: string? = e.meta and e.meta.suite
    local nested_and_is_not_total: string = e.meta and e.meta.suite -- expect-error
    return nested_and
end
local suite_of_entry: string? = suite_of(acyclic)

-- An uncalled declaration carries the same obligation: no invocation can supply
-- a formal more precise than the declared type the projection is proven from.
local function uncalled_shown(u: User, e: Entry): string
    local uncalled_or: string = u.nick or u.name
    local uncalled_or_is_not_a_number: number = u.nick or u.name -- expect-error
    local uncalled_and: string? = e.meta and e.meta.suite
    local uncalled_and_is_not_total: string = e.meta and e.meta.suite -- expect-error
    return uncalled_or
end
