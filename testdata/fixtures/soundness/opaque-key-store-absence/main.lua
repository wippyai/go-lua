-- Lua indexes a dotted name by its string key like any other index, so a store
-- whose key this analysis never resolved to a slot can land on any slot of the
-- table -- including one a later read spells exactly. From that store on, the
-- constructor's closure no longer proves an omitted slot absent, and it no
-- longer proves a recorded slot still current. The keyed component the stores
-- do establish is what types the read in its place: their key type is the key
-- domain, their value type is what every unnamed slot holds, and presence stays
-- unproven because no store named the slot the read selects.
--
-- The obligations run both ways. A closed constructor with no such store keeps
-- its closure. A store this analysis cannot classify leaves the container with
-- no keyed component at all, and the lost closure still stands: the read is
-- unknown, not absent.

-- Control: no store at an unresolved key, so the closure stands and the omitted
-- member is proven absent.
local sealed = { present = 1 }
local sealed_missing: nil = sealed.absent
local sealed_wrong: number = sealed.absent -- expect-error

-- The loop stores at a key drawn from the string array. That key can be "x" or
-- "y", so neither the computed read nor the spelled one is nil. The component
-- states integer at a string key, and states nothing about presence.
local keys: {string} = {}
local suites = {}
for _, key in ipairs(keys) do
    suites[key] = 1
end
local computed: nil = suites["x"] -- expect-error
local spelled: nil = suites.y -- expect-error
local element: integer? = suites["x"]
local present: integer = suites["x"] -- expect-error
local other: string? = suites["x"] -- expect-error

-- A table is not a key domain a read can be decided against, so these stores
-- classify nothing and the container keeps no keyed component. The unresolved
-- store still stands, so the read is unknown rather than absent.
local table_keys = { {}, {} }
local sink = {}
for _, table_key in ipairs(table_keys) do
    sink[table_key] = 1
end
local sink_absent: nil = sink["x"] -- expect-error

-- The same container crossing a call boundary. The callee's unresolved store is
-- part of the authority its return carries, so the caller reads the identical
-- answer the producing body reads.
local function build(source: {string})
    local out = {}
    for _, key in ipairs(source) do
        out[key] = 1
    end
    return out
end

local returned = build(keys)
local returned_computed: nil = returned["x"] -- expect-error
local returned_spelled: nil = returned.y -- expect-error
local returned_element: integer? = returned["x"]
local returned_present: integer = returned["x"] -- expect-error
local returned_other: string? = returned["x"] -- expect-error

return sealed_missing, sealed_wrong, computed, spelled, element, present, other,
    sink_absent, returned_computed, returned_spelled, returned_element,
    returned_present, returned_other
