-- A container addressed only at keys this analysis never resolved carries no
-- declaration and no type witness, and its allocation value states nothing
-- about those slots. The keyed component its own stores establish is that
-- container's key domain and element, and it is already the authority an index
-- read of it consumes. An enumeration of the same container binds exactly what
-- that component describes, so the key it produces is the component's key
-- domain and the read at that key is the component's element.
--
-- The obligations run both ways. A container whose stores establish no
-- component states nothing about its slots, so an enumeration of it binds an
-- unknown key exactly as before, and a declared container keeps answering from
-- its declaration.

type Entry = {id: string}

local entries: {Entry} = {}

local suites = {}
for _, entry in ipairs(entries) do
    suites[entry.id] = suites[entry.id] or {}
    table.insert(suites[entry.id], entry)
end

-- The read outside the loop and the read inside the enumeration answer with the
-- same component.
local direct: {Entry}? = suites["alpha"]
for key in pairs(suites) do
    local spelled: string = key
    local wrong_key: number = key -- expect-error
    local tests: {Entry} = suites[key]
    local wrong_element: {string} = suites[key] -- expect-error
end

-- The enumerated key names a slot the container holds, so the read at it is the
-- element without Lua's missing-slot nil.
local counts = {}
local keys: {string} = {}
for _, key in ipairs(keys) do
    counts[key] = 1
end
for key in pairs(counts) do
    local hit: integer = counts[key]
    local absent: nil = counts[key] -- expect-error
end

-- A store whose value carries no type leaves no component, so the enumeration
-- binds an unknown key exactly as it does without one.
local untyped = {}
for _, entry in ipairs(entries) do
    untyped[entry.id] = unresolved_source()
end
for key in pairs(untyped) do
    local spelled: string = key -- expect-error
end

-- A declared container answers from its declaration, unchanged.
local declared: {[string]: {Entry}} = {}
for key in pairs(declared) do
    local spelled: string = key
    local tests: {Entry} = declared[key]
end

return suites, direct, counts, untyped, declared
