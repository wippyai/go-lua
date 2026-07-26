-- The whole chain a grouped container travels: a body fills it at keys this
-- analysis never resolves, returns it, a helper with an undeclared formal
-- enumerates it into an array of its keys, and the caller reads the container
-- back at one of those keys.
--
-- Each link states one thing. An append through a read at an unresolved key
-- belongs to the container's keyed component wherever that append stands,
-- including inside an arm of a decision. A call whose published effect changes
-- neither the container's shape nor its length rewrites it in place and
-- accounts for itself. A formal with no declaration holds the component of the
-- argument bound to it. A returned container needs a heap identity of its own
-- before anything about its slots can be stated, because that identity is the
-- subject every later write publishes its revocation under.
--
-- The obligations run both ways: a store into the container, an element the
-- array received from anywhere but the enumeration, and a container handed to a
-- callee this analysis never read each leave the relation describing a state
-- the container has left.

type Entry = {id: string, meta: {type: string, suite: string?}?}

local function group_by_suite(entries: {Entry})
    local suites = {}
    for _, entry in ipairs(entries) do
        local suite = entry.meta and entry.meta.suite
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        end
    end
    return suites
end

local function sorted_keys(t)
    local out = {}
    for key in pairs(t) do
        table.insert(out, key)
    end
    table.sort(out)
    return out
end

local entries: {Entry} = {}
local suites = group_by_suite(entries)

-- The element the guarded append established crosses the return boundary.
local one: {Entry}? = suites["alpha"]
local one_wrong: {string}? = suites["alpha"] -- expect-error

-- The undeclared formal holds the component, so the enumeration inside the
-- helper binds its key domain and the returned array carries the relation.
local names = sorted_keys(suites)
local first: string = names[1]
for _, name in ipairs(names) do
    local tests: {Entry} = suites[name]
    local tests_wrong: {string} = suites[name] -- expect-error
end

-- A store into the container leaves the array naming slots it no longer
-- accounts for.
local mutated = group_by_suite(entries)
local mutated_names = sorted_keys(mutated)
mutated["added"] = {}
for _, name in ipairs(mutated_names) do
    local tests: {Entry} = mutated[name] -- expect-error
end

-- An element the array received from anywhere but the enumeration is a key no
-- enumeration produced.
local mixed = group_by_suite(entries)
local mixed_names = sorted_keys(mixed)
table.insert(mixed_names, "literal")
for _, name in ipairs(mixed_names) do
    local tests: {Entry} = mixed[name] -- expect-error
end

-- A container handed to a callee this analysis never read may hold slots that
-- callee put there.
local escaped = group_by_suite(entries)
local escaped_names = sorted_keys(escaped)
unresolved_sink(escaped)
for _, name in ipairs(escaped_names) do
    local tests: {Entry} = escaped[name] -- expect-error
end

return suites, one, one_wrong, names, first, mutated, mixed, escaped
