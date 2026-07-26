-- A store at a key this analysis never resolved establishes the container's
-- keyed component, and the component is what answers every read of an unnamed
-- slot. A write through such a read therefore reaches that component and
-- nothing else: the empty literal the store placed there is an empty array as
-- much as an empty record, and the elements appended through the slot are what
-- decide which. The component's value is the array those elements prove.
--
-- The obligations run both ways. An appended element carrying no type states
-- nothing about the slot, so the component is withheld rather than left at the
-- store's own empty literal. A slot handed to a callee that accounts for
-- nothing it put there is withheld on the same ground. A store of something no
-- element could have been appended to leaves the writes describing two
-- different slots, and the component describes neither.

type Entry = {id: string, weight: number}

local entries: {Entry} = {}

-- The component's value follows the appends: every slot holds an array of the
-- appended element type.
local groups = {}
for _, entry in ipairs(entries) do
    groups[entry.id] = groups[entry.id] or {}
    table.insert(groups[entry.id], entry)
end
local group: {Entry}? = groups["alpha"]
local group_absent: nil = groups["alpha"] -- expect-error
local group_wrong: {string}? = groups["alpha"] -- expect-error

-- The same relation across a call boundary: the callee's component is the
-- authority its return carries.
local function group_by_id(source: {Entry})
    local built = {}
    for _, entry in ipairs(source) do
        built[entry.id] = built[entry.id] or {}
        table.insert(built[entry.id], entry)
    end
    return built
end
local returned = group_by_id(entries)
local returned_group: {Entry}? = returned["alpha"]
local returned_wrong: {number}? = returned["alpha"] -- expect-error

-- An element whose type is unpublished leaves the slot undescribed, so the
-- component is withheld instead of standing on the store's empty literal.
local untyped = {}
for _, entry in ipairs(entries) do
    untyped[entry.id] = untyped[entry.id] or {}
    table.insert(untyped[entry.id], unresolved_source())
end
local untyped_group: {Entry}? = untyped["alpha"] -- expect-error

-- A slot handed to a callee this analysis never read may hold anything it put
-- there, and the call accounts for nothing.
local escaped = {}
for _, entry in ipairs(entries) do
    escaped[entry.id] = escaped[entry.id] or {}
    table.insert(escaped[entry.id], entry)
    unresolved_sink(escaped[entry.id])
end
local escaped_group: {Entry}? = escaped["alpha"] -- expect-error

-- A store of a value no element could have been appended to leaves the two
-- writes describing different slots.
local mixed = {}
for _, entry in ipairs(entries) do
    mixed[entry.id] = entry.weight
    table.insert(mixed[entry.id], entry)
end
local mixed_group: {Entry}? = mixed["alpha"] -- expect-error

return group, group_absent, group_wrong, returned_group, returned_wrong,
    untyped_group, escaped_group, mixed_group
