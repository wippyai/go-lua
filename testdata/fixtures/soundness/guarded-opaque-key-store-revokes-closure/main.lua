-- A store at a key this analysis never resolved is a may-fact: the arm that
-- performed it is one of the executions arriving at every point past the
-- decision, so the container's constructor stops proving an omitted slot absent
-- there. Reading the marker only inside the arm that published it would let a
-- guarded store leave the closure standing at the join, where the store may
-- already have happened.
--
-- The obligations run both ways. A container with no such store anywhere keeps
-- its closure, and the arm's own value is what the component types the read
-- with, so a read at an unnamed slot is that value rather than a permissive one.

-- Control: no store at an unresolved key, so the closure stands.
local sealed = { present = 1 }
local sealed_missing: nil = sealed.absent
local sealed_wrong: number = sealed.absent -- expect-error

-- The store sits on one arm of a decision. Past the decision the slot it may
-- have written is no longer proven absent, and the component the store
-- establishes is what types the read.
local guarded = {}
local key = tostring(1)
if key ~= "" then
    guarded[key] = 1
end
local guarded_absent: nil = guarded["x"] -- expect-error
local guarded_element: integer? = guarded["x"]
local guarded_present: integer = guarded["x"] -- expect-error
local guarded_other: string? = guarded["x"] -- expect-error

-- The same decision inside a loop body: the arm is still one of the executions
-- reaching the read.
type Entry = {id: string}
local entries: {Entry} = {}
local looped = {}
for _, entry in ipairs(entries) do
    if entry.id ~= "" then
        looped[entry.id] = 1
    end
end
local looped_absent: nil = looped["x"] -- expect-error
local looped_element: integer? = looped["x"]

return sealed_missing, sealed_wrong, guarded_absent, guarded_element,
    guarded_present, guarded_other, looped_absent, looped_element
